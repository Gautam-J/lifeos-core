package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

// newQueryService returns a service with a query pool that connects as the
// read-only user.
func newQueryService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	s, pool := newTestService(t)
	if err := s.UseQueryPool(context.Background(), testdb.QueryPool(t, pool)); err != nil {
		t.Fatalf("UseQueryPool: %v", err)
	}
	return s, pool
}

// cells returns the values of a result as text. A NULL is "NULL".
func cells(result QueryResult) [][]string {
	out := [][]string{}
	for _, row := range result.Rows {
		texts := []string{}
		for _, value := range row {
			if value == nil {
				texts = append(texts, "NULL")
			} else {
				texts = append(texts, *value)
			}
		}
		out = append(out, texts)
	}
	return out
}

func wantSQLRefused(t *testing.T, name string, err error, wantInMessage string) {
	t.Helper()
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "sql" || !strings.Contains(invalid.Message, wantInMessage) {
		t.Errorf("%s: error = %v, want a ValidationError on sql with %q", name, err, wantInMessage)
	}
}

func TestQueryReturnsTheColumnsAndTheRowsAsText(t *testing.T) {
	s, _ := newQueryService(t)
	ctx := context.Background()
	addTodo(t, s, "Buy a domain") // project lifeos, due 2026-10-20
	done := addTodo(t, s, "Call the bank")
	if _, err := s.UpdateTodo(ctx, done.ID, UpdateTodoParams{Status: ptr("done"), Actor: SourceManual}); err != nil {
		t.Fatalf("UpdateTodo: %v", err)
	}

	got, err := s.Query(ctx, "SELECT title, status, due_on, done_at IS NULL AS waits, verified, deleted_at FROM todos ORDER BY id")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !equal(got.Columns, []string{"title", "status", "due_on", "waits", "verified", "deleted_at"}) {
		t.Errorf("columns = %v", got.Columns)
	}
	rows := cells(got)
	if len(rows) != 2 ||
		!equal(rows[0], []string{"Buy a domain", "open", "2026-10-20", "t", "t", "NULL"}) ||
		!equal(rows[1], []string{"Call the bank", "done", "2026-10-20", "f", "t", "NULL"}) {
		t.Errorf("rows = %v", rows)
	}
	if got.Truncated {
		t.Error("truncated = true for two rows")
	}

	count, err := s.Query(ctx, "SELECT status, count(*) AS n FROM todos WHERE deleted_at IS NULL GROUP BY status ORDER BY status")
	if err != nil {
		t.Fatalf("Query with GROUP BY: %v", err)
	}
	if rows := cells(count); len(rows) != 2 || !equal(rows[0], []string{"done", "1"}) || !equal(rows[1], []string{"open", "1"}) {
		t.Errorf("counts = %v", rows)
	}

	// A value of each type comes as the text that psql shows.
	typed, err := s.Query(ctx, `SELECT 1.50::numeric, '\xdeadbeef'::bytea, '{"a": [1, 2]}'::jsonb, ARRAY['x', 'y'], '2026-10-11 07:30:00+05:30'::timestamptz AT TIME ZONE 'UTC', NULL::int`)
	if err != nil {
		t.Fatalf("Query with a value of each type: %v", err)
	}
	if rows := cells(typed); len(rows) != 1 || !equal(rows[0], []string{"1.50", `\xdeadbeef`, `{"a": [1, 2]}`, "{x,y}", "2026-10-11 02:00:00", "NULL"}) {
		t.Errorf("typed values = %q", rows)
	}

	none, err := s.Query(ctx, "SELECT id FROM todos WHERE false")
	if err != nil || none.Rows == nil || len(none.Rows) != 0 || !equal(none.Columns, []string{"id"}) {
		t.Errorf("no row: result = %+v, %v, want the column and an empty list", none, err)
	}
}

// The database refuses a change. The Go code does not look at the SQL. The
// read-only transaction refuses first; the migration tests show that the
// rights of lifeos_query refuse the same statements without it.
func TestQueryCannotChangeData(t *testing.T) {
	s, pool := newQueryService(t)
	ctx := context.Background()
	todo := addTodo(t, s, "Buy a domain")
	before := countRows(t, pool, "events")

	for name, c := range map[string]struct{ sql, want string }{
		"an insert":               {"INSERT INTO todos (title, source) VALUES ('x', 'manual')", "read-only transaction"},
		"an update":               {"UPDATE todos SET title = 'x'", "read-only transaction"},
		"a delete":                {"DELETE FROM todos", "read-only transaction"},
		"a delete in a WITH":      {"WITH gone AS (DELETE FROM todos RETURNING id) SELECT * FROM gone", "read-only transaction"},
		"a drop":                  {"DROP TABLE todos", "read-only transaction"},
		"a temporary table":       {"CREATE TEMP TABLE notes (id int)", "read-only transaction"},
		"a second statement":      {"SELECT 1; DELETE FROM todos", "multiple commands"},
		"a new role":              {"CREATE ROLE helper SUPERUSER", "read-only transaction"},
		"a file of the server":    {"SELECT pg_read_file('/etc/passwd')", "permission denied"},
		"a program on the server": {"COPY (SELECT 1) TO PROGRAM 'id'", "permission denied"},
	} {
		_, err := s.Query(ctx, c.sql)
		wantSQLRefused(t, name, err, c.want)
	}

	got, err := s.GetTodo(ctx, todo.ID)
	if err != nil || got.Title != "Buy a domain" {
		t.Errorf("the todo = %+v, %v, want it as it was", got, err)
	}
	if after := countRows(t, pool, "events"); after != before {
		t.Errorf("events = %d, want %d", after, before)
	}
}

func TestQueryRejectsBadInput(t *testing.T) {
	s, _ := newQueryService(t)
	ctx := context.Background()

	for name, c := range map[string]struct{ sql, want string }{
		"no statement":              {"  \n", "must not be empty"},
		"a statement too long":      {"SELECT '" + strings.Repeat("a", maxSQLLength) + "'", "at most"},
		"a NUL":                     {"SELECT 'a\x00b'", "valid text"},
		"a syntax error":            {"SELEC 1", `syntax error at or near "SELEC"`},
		"a table that is not there": {"SELECT * FROM songs", `relation "songs" does not exist`},
		"a parameter":               {"SELECT $1", "bind message supplies 0 parameters"},
	} {
		_, err := s.Query(ctx, c.sql)
		wantSQLRefused(t, name, err, c.want)
	}
}

func TestQueryCutsALargeResult(t *testing.T) {
	s, _ := newQueryService(t)
	ctx := context.Background()

	full, err := s.Query(ctx, "SELECT generate_series(1, 200)")
	if err != nil || len(full.Rows) != maxQueryRows || full.Truncated {
		t.Errorf("200 rows: rows = %d, truncated = %v, %v, want 200, false", len(full.Rows), full.Truncated, err)
	}
	cut, err := s.Query(ctx, "SELECT generate_series(1, 201)")
	if err != nil || len(cut.Rows) != maxQueryRows || !cut.Truncated {
		t.Errorf("201 rows: rows = %d, truncated = %v, %v, want 200, true", len(cut.Rows), cut.Truncated, err)
	}
	// Ten rows of 100 kB are more than the limit on the size.
	wide, err := s.Query(ctx, "SELECT repeat('x', 100000) FROM generate_series(1, 10)")
	if err != nil || len(wide.Rows) >= 10 || len(wide.Rows) == 0 || !wide.Truncated {
		t.Errorf("wide rows: rows = %d, truncated = %v, %v, want some rows, fewer than 10, true", len(wide.Rows), wide.Truncated, err)
	}
}

func TestQueryStopsAStatementThatRunsTooLong(t *testing.T) {
	s, _ := newQueryService(t)
	s.queryTimeout = 300 * time.Millisecond

	start := time.Now()
	_, err := s.Query(context.Background(), "SELECT pg_sleep(30)")
	wantSQLRefused(t, "a long statement", err, "took longer than 300ms")
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Query came back after %s, want less than 5 seconds", elapsed)
	}
	// The next query works.
	if got, err := s.Query(context.Background(), "SELECT 1"); err != nil || len(got.Rows) != 1 {
		t.Errorf("query after the timeout: %+v, %v", got, err)
	}
}

// One query must not leave a setting or a lock for the next query.
func TestQueryStartsEachStatementInACleanSession(t *testing.T) {
	s, _ := newQueryService(t)
	ctx := context.Background()

	for _, sql := range []string{
		"SELECT set_config('lifeos.note', 'left behind', false)",
		"SET statement_timeout = 1",
		"SELECT pg_advisory_lock(42)",
	} {
		if _, err := s.Query(ctx, sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	got, err := s.Query(ctx, "SELECT current_setting('lifeos.note', true), current_setting('statement_timeout'), (SELECT count(*) FROM pg_locks WHERE locktype = 'advisory')")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if rows := cells(got); len(rows) != 1 || !equal(rows[0], []string{"NULL", "0", "0"}) {
		t.Errorf("the session of the next query = %v, want no note, no timeout, no lock", rows)
	}
}

func TestUseQueryPoolRefusesAUserThatCanWrite(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	// pool connects as the owner of the tables.
	err := s.UseQueryPool(ctx, pool)
	if err == nil || !strings.Contains(err.Error(), "read-only user") {
		t.Fatalf("UseQueryPool with the owner: error = %v, want it refused", err)
	}
	_, err = s.Query(ctx, "SELECT 1")
	var invalid *ValidationError
	if err == nil || errors.As(err, &invalid) {
		t.Errorf("Query with no query pool: error = %v, want an error that is not a ValidationError", err)
	}
}

// One row can be very large. Without a limit, one statement can use all
// the memory of the Pi.
func TestQueryRefusesARowThatIsTooLarge(t *testing.T) {
	s, _ := newQueryService(t)
	ctx := context.Background()

	_, err := s.Query(ctx, "SELECT repeat('x', 30000000)")
	wantSQLRefused(t, "a row of 30 MB", err, "a row is too large")

	// A row of 1 MB comes back whole: that is the largest text of a source.
	got, err := s.Query(ctx, "SELECT repeat('x', 1048576)")
	if err != nil || len(got.Rows) != 1 || len(*got.Rows[0][0]) != 1048576 {
		t.Errorf("a row of 1 MB: %d rows, %v, want the full value", len(got.Rows), err)
	}
}
