package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/Gautam-J/lifeos-core/internal/testdb"
	"github.com/Gautam-J/lifeos-core/migrations"
)

func TestUpCreatesTheTables(t *testing.T) {
	pool := testdb.New(t) // testdb.New already ran migrations.Up once
	ctx := context.Background()

	for _, table := range []string{"events", "catalog_items", "journal_entries", "todos", "habits", "habit_logs", "sources"} {
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = $1)",
			table).Scan(&exists)
		if err != nil {
			t.Fatalf("query %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %s does not exist after Up", table)
		}
	}
}

// Every record table has the provenance columns, deleted_at and the
// trigger that keeps updated_at current.
func TestRecordTablesHaveTheProvenanceColumns(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	for _, table := range []string{"catalog_items", "journal_entries", "todos", "habits", "habit_logs", "sources"} {
		for _, column := range []string{"source", "source_ref", "verified", "created_at", "updated_at", "deleted_at"} {
			var exists bool
			err := pool.QueryRow(ctx,
				"SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2)",
				table, column).Scan(&exists)
			if err != nil {
				t.Fatalf("query %s.%s: %v", table, column, err)
			}
			if !exists {
				t.Errorf("table %s has no column %s", table, column)
			}
		}
		var triggers int
		err := pool.QueryRow(ctx,
			"SELECT count(*) FROM pg_trigger WHERE tgrelid = $1::regclass AND tgname = $2",
			table, table+"_set_updated_at").Scan(&triggers)
		if err != nil {
			t.Fatalf("query the trigger of %s: %v", table, err)
		}
		if triggers != 1 {
			t.Errorf("table %s has no trigger %s_set_updated_at", table, table)
		}
	}
}

// The database itself keeps one live log for each habit and day, and one
// live habit for each name. A deleted row does not count.
func TestHabitTablesRefuseADuplicate(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	const addHabit = "INSERT INTO habits (name, source) VALUES ('gym', 'manual') RETURNING id"
	const addLog = "INSERT INTO habit_logs (habit_id, logged_on, source) VALUES ($1, '2026-10-11', 'manual')"

	var habit int64
	if err := pool.QueryRow(ctx, addHabit).Scan(&habit); err != nil {
		t.Fatalf("first habit: %v", err)
	}
	if _, err := pool.Exec(ctx, addHabit); err == nil {
		t.Error("a second live habit with the same name was stored")
	}
	if _, err := pool.Exec(ctx, addLog, habit); err != nil {
		t.Fatalf("first log: %v", err)
	}
	if _, err := pool.Exec(ctx, addLog, habit); err == nil {
		t.Error("a second live log for the same habit and day was stored")
	}

	if _, err := pool.Exec(ctx, "UPDATE habit_logs SET deleted_at = now() WHERE habit_id = $1", habit); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, addLog, habit); err != nil {
		t.Errorf("a new log after a deleted log was refused: %v", err)
	}
}

func TestTodosRefuseAnUnknownStatus(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	const add = "INSERT INTO todos (title, status, source) VALUES ('Buy a domain', $1, 'manual')"

	for _, status := range []string{"open", "done", "dropped"} {
		if _, err := pool.Exec(ctx, add, status); err != nil {
			t.Errorf("status %s: %v", status, err)
		}
	}
	if _, err := pool.Exec(ctx, add, "later"); err == nil {
		t.Error("a todo with the status later was stored")
	}
}

func TestUpAppliesNothingTheSecondTime(t *testing.T) {
	pool := testdb.New(t)
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	applied, err := migrations.Up(context.Background(), db)
	if err != nil {
		t.Fatalf("second Up: %v", err)
	}
	if len(applied) != 0 {
		t.Errorf("second Up applied %v, want nothing", applied)
	}
}

// The database keeps one live capture for each kind and content. A deleted
// capture does not count, and the same hash can be a text and a link.
func TestSourcesRefuseASecondLiveCaptureOfTheSameContent(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	const addText = "INSERT INTO sources (kind, content, sha256, source) VALUES ('text', 'a note', 'abc', 'manual') RETURNING id"

	var first int64
	if err := pool.QueryRow(ctx, addText).Scan(&first); err != nil {
		t.Fatalf("first capture: %v", err)
	}
	if _, err := pool.Exec(ctx, addText); err == nil {
		t.Error("a second live capture of the same text was stored")
	}
	if _, err := pool.Exec(ctx, "INSERT INTO sources (kind, url, sha256, source) VALUES ('link', 'https://example.com', 'abc', 'manual')"); err != nil {
		t.Errorf("a link with the hash of a text: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE sources SET deleted_at = now() WHERE id = $1", first); err != nil {
		t.Fatalf("delete the first capture: %v", err)
	}
	if _, err := pool.Exec(ctx, addText); err != nil {
		t.Errorf("a capture of the text of a deleted capture: %v", err)
	}
}

// A row holds what its kind needs: a text has content, a link has a URL, a
// file has a path.
func TestSourcesRefuseARowThatDoesNotFitItsKind(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	for name, insert := range map[string]string{
		"an unknown kind":       "INSERT INTO sources (kind, content, sha256, source) VALUES ('video', 'x', 'a', 'manual')",
		"a text with no text":   "INSERT INTO sources (kind, sha256, source) VALUES ('text', 'b', 'manual')",
		"a text with a URL":     "INSERT INTO sources (kind, content, url, sha256, source) VALUES ('text', 'x', 'https://example.com', 'c', 'manual')",
		"a link with no URL":    "INSERT INTO sources (kind, sha256, source) VALUES ('link', 'd', 'manual')",
		"a file with no path":   "INSERT INTO sources (kind, sha256, source) VALUES ('file', 'e', 'manual')",
		"a file with a URL":     "INSERT INTO sources (kind, file_path, url, sha256, source) VALUES ('file', 'ab/abc', 'https://example.com', 'f', 'manual')",
		"a row with no hash":    "INSERT INTO sources (kind, content, source) VALUES ('text', 'x', 'manual')",
		"an unknown provenance": "INSERT INTO sources (kind, content, sha256, source) VALUES ('text', 'x', 'g', 'robot')",
	} {
		if _, err := pool.Exec(ctx, insert); err == nil {
			t.Errorf("%s was stored", name)
		}
	}
	for name, insert := range map[string]string{
		"a text": "INSERT INTO sources (kind, content, sha256, source) VALUES ('text', 'x', 'h', 'manual')",
		"a link": "INSERT INTO sources (kind, url, sha256, source) VALUES ('link', 'https://example.com', 'i', 'manual')",
		"a file": "INSERT INTO sources (kind, title, file_path, media_type, size_bytes, sha256, source) VALUES ('file', 'policy.pdf', 'ab/abc', 'application/pdf', 3, 'j', 'manual')",
	} {
		if _, err := pool.Exec(ctx, insert); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// source_ref on each record table names a row of sources.
func TestSourceRefMustNameASource(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	var source int64
	err := pool.QueryRow(ctx, "INSERT INTO sources (kind, content, sha256, source) VALUES ('text', 'a note', 'abc', 'manual') RETURNING id").Scan(&source)
	if err != nil {
		t.Fatalf("add a source: %v", err)
	}
	// The habit of the habit log. It gets the ID 1.
	if _, err := pool.Exec(ctx, "INSERT INTO habits (name, source) VALUES ('gym', 'manual')"); err != nil {
		t.Fatalf("add a habit: %v", err)
	}

	inserts := map[string]string{
		"catalog_items":   "INSERT INTO catalog_items (kind, title, source, source_ref) VALUES ('book', 'Dune', 'agent', $1)",
		"journal_entries": "INSERT INTO journal_entries (body, entry_date, source, source_ref) VALUES ('a day', '2026-10-11', 'agent', $1)",
		"todos":           "INSERT INTO todos (title, source, source_ref) VALUES ('Buy a domain', 'agent', $1)",
		"habits":          "INSERT INTO habits (name, source, source_ref) VALUES ('read', 'agent', $1)",
		"habit_logs":      "INSERT INTO habit_logs (habit_id, logged_on, source, source_ref) VALUES (1, '2026-10-11', 'agent', $1)",
		"sources":         "INSERT INTO sources (kind, content, sha256, source, source_ref) VALUES ('text', 'a part', 'def', 'agent', $1)",
	}
	for table, insert := range inserts {
		if _, err := pool.Exec(ctx, insert, source+1000); err == nil {
			t.Errorf("%s: a row with a source_ref that names no source was stored", table)
		}
		if _, err := pool.Exec(ctx, insert, source); err != nil {
			t.Errorf("%s: a row with the source_ref of a source: %v", table, err)
		}
	}
}
