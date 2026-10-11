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

	for _, table := range []string{"events", "catalog_items", "journal_entries", "todos", "habits", "habit_logs"} {
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

	for _, table := range []string{"catalog_items", "journal_entries", "todos", "habits", "habit_logs"} {
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
