package mcp_test

import (
	"context"
	"strings"
	"testing"
)

// "todo - buy cloudflare domain and set up tunnel": the user dictated the
// two actions, so the two rows are verified.
func TestTodoStoresATodoThatTheUserDictated(t *testing.T) {
	session, pool := connect(t)

	first := structured(t, call(t, session, "todo", map[string]any{"title": "Buy a Cloudflare domain", "project": "lifeos", "dictated": true}))
	second := structured(t, call(t, session, "todo", map[string]any{"title": "Set up the tunnel", "project": "lifeos", "dictated": true}))

	if first["id"] == second["id"] {
		t.Errorf("the two todos have one ID: %v", first["id"])
	}
	if first["title"] != "Buy a Cloudflare domain" || first["status"] != "open" || first["project"] != "lifeos" {
		t.Errorf("todo = %v", first)
	}
	if _, has := first["due_on"]; has {
		t.Errorf("the todo has a date: %v", first)
	}
	if first["source"] != "agent" || first["verified"] != true {
		t.Errorf("source, verified = %v, %v, want agent, true", first["source"], first["verified"])
	}
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM events WHERE action = 'create' AND record_type = 'todos' AND source = 'agent'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("create events = %d, want 2", n)
	}
}

// The tool todo adds. The tool update changes a todo and marks it done.
func TestUpdateMarksATodoDone(t *testing.T) {
	session, _ := connect(t)
	added := structured(t, call(t, session, "todo", map[string]any{"title": "Buy a domain", "due_on": "2026-10-20", "dictated": true}))

	refused := call(t, session, "update", map[string]any{"type": "todo", "id": added["id"], "fields": map[string]any{"status": "done"}})
	if !refused.IsError || !strings.Contains(text(refused), "dictated") {
		t.Fatalf("update with no dictated: IsError = %v, text = %q; want an error that names dictated", refused.IsError, text(refused))
	}

	updated := structured(t, call(t, session, "update", map[string]any{
		"type": "todo", "id": added["id"], "fields": map[string]any{"status": "done"}, "clear": []string{"due_on"}, "dictated": true,
	}))
	if updated["status"] != "done" || updated["done_at"] == nil || updated["title"] != "Buy a domain" {
		t.Errorf("update returned %v", updated)
	}
	if _, has := updated["due_on"]; has {
		t.Errorf("the todo still has a date: %v", updated)
	}

	got := structured(t, call(t, session, "get", map[string]any{"type": "todo", "id": added["id"]}))
	if got["status"] != "done" {
		t.Errorf("get returned %v", got)
	}
	unknown := call(t, session, "update", map[string]any{"type": "todo", "id": added["id"], "fields": map[string]any{"status": "later"}, "dictated": true})
	if !unknown.IsError || !strings.Contains(text(unknown), "status: must be one of open, done, dropped") {
		t.Errorf("update with an unknown status: IsError = %v, text = %q", unknown.IsError, text(unknown))
	}
	if result := call(t, session, "delete", map[string]any{"type": "todo", "id": added["id"]}); !result.IsError || !strings.Contains(text(result), "confirmed") {
		t.Errorf("delete with no confirmation: IsError = %v, text = %q", result.IsError, text(result))
	}
}

func TestTodoReturnsErrorsTheModelCanRead(t *testing.T) {
	session, pool := connect(t)
	tests := []struct {
		name     string
		args     map[string]any
		wantText string
	}{
		{"empty title", map[string]any{"title": " "}, "title"},
		{"missing title", map[string]any{"project": "lifeos"}, "title"},
		{"date that is not a date", map[string]any{"title": "x", "due_on": "next week"}, "due_on"},
		{"misspelled argument", map[string]any{"title": "x", "due": "2026-12-01"}, "due"},
		// A new todo is open. The tool has no status argument.
		{"status", map[string]any{"title": "x", "status": "done"}, "status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := call(t, session, "todo", tt.args)
			if !result.IsError {
				t.Fatalf("IsError = false, want an error result")
			}
			if !strings.Contains(text(result), tt.wantText) {
				t.Errorf("error text %q does not name %q", text(result), tt.wantText)
			}
		})
	}
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM todos").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("todos has %d rows after rejected calls, want 0", n)
	}

	pool.Close()
	result := call(t, session, "todo", map[string]any{"title": "x"})
	if !result.IsError || text(result) != "internal error" {
		t.Errorf("database failure: IsError = %v, text = %q; want only \"internal error\"", result.IsError, text(result))
	}
}
