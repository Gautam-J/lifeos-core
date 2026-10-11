package mcp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

// connectWithQuery returns a session for a service that has a query pool.
func connectWithQuery(t *testing.T) (*sdk.ClientSession, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.New(t)
	svc := service.New(pool)
	if err := svc.UseQueryPool(context.Background(), testdb.QueryPool(t, pool)); err != nil {
		t.Fatalf("UseQueryPool: %v", err)
	}
	return connectTo(t, svc), pool
}

func TestQueryAnswersAQuestionWithACount(t *testing.T) {
	session, pool := connectWithQuery(t)
	addHabit(t, pool, "gym")
	for _, day := range []string{"2026-01-05", "2026-01-06", "2026-01-08"} {
		call(t, session, "log_habit", map[string]any{"habit": "gym", "logged_on": day, "dictated": true})
	}
	call(t, session, "log_habit", map[string]any{"habit": "gym", "logged_on": "2026-01-09"}) // waits for approval

	got := structured(t, call(t, session, "query", map[string]any{
		"sql": "SELECT h.name, count(*) AS days, min(l.logged_on) AS first_day FROM habit_logs l JOIN habits h ON h.id = l.habit_id WHERE l.deleted_at IS NULL AND l.verified GROUP BY h.name",
	}))

	columns := got["columns"].([]any)
	rows := got["rows"].([]any)
	if len(columns) != 3 || columns[1] != "days" || len(rows) != 1 {
		t.Fatalf("result = %v, want three columns and one row", got)
	}
	if row := rows[0].([]any); row[0] != "gym" || row[1] != "3" || row[2] != "2026-01-05" {
		t.Errorf("row = %v, want gym, 3, 2026-01-05 as text", row)
	}
	if got["truncated"] != false {
		t.Errorf("truncated = %v, want false", got["truncated"])
	}

	// A NULL is a JSON null.
	null := structured(t, call(t, session, "query", map[string]any{"sql": "SELECT NULL::text AS nothing"}))
	if row := null["rows"].([]any)[0].([]any); row[0] != nil {
		t.Errorf("NULL = %v, want null", row[0])
	}
}

func TestQueryCannotChangeData(t *testing.T) {
	session, pool := connectWithQuery(t)
	call(t, session, "todo", map[string]any{"title": "Buy a domain", "dictated": true})

	for name, tt := range map[string]struct{ sql, want string }{
		"a delete":           {"DELETE FROM todos", "sql: PostgreSQL refused the statement: cannot execute DELETE in a read-only transaction"},
		"a second statement": {"SELECT 1; UPDATE todos SET title = 'x'", "cannot insert multiple commands into a prepared statement"},
		"a syntax error":     {"SELEC 1", "syntax error"},
		"no statement":       {"", "sql: must not be empty"},
	} {
		result := call(t, session, "query", map[string]any{"sql": tt.sql})
		if !result.IsError || !strings.Contains(text(result), tt.want) {
			t.Errorf("%s: result = %q, want an error with %q", name, text(result), tt.want)
		}
		// The agent wrote the SQL. It must not ask the user to correct it.
		if !strings.Contains(text(result), "Correct the statement, then call query again.") || strings.Contains(text(result), "Ask the user") {
			t.Errorf("%s: result = %q, want it to tell the agent to correct the statement", name, text(result))
		}
	}
	var title string
	if err := pool.QueryRow(context.Background(), "SELECT title FROM todos").Scan(&title); err != nil || title != "Buy a domain" {
		t.Errorf("the todo = %q, %v, want it as it was", title, err)
	}
}

// The description is how the model knows the tables. A migration that adds
// a table must add its name to the description.
func TestQueryDescriptionNamesEveryTable(t *testing.T) {
	session, pool := connectWithQuery(t)
	ctx := context.Background()

	list, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	description := ""
	for _, tool := range list.Tools {
		if tool.Name == "query" {
			description = tool.Description
		}
	}
	rows, err := pool.Query(ctx, "SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
	if err != nil {
		t.Fatalf("list the tables: %v", err)
	}
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(tables) < 8 {
		t.Fatalf("tables = %v, %v, want 8 or more", tables, err)
	}
	for _, table := range tables {
		if !strings.Contains(description, table) {
			t.Errorf("the description of query does not name the table %s", table)
		}
	}
}
