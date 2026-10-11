package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// One day of records: a journal entry, a todo that the agent proposes from
// it, and a habit. Each step uses a different adapter, and each adapter
// must see what the others did.
func TestServeTakesADayOfRecordsThroughEveryAdapter(t *testing.T) {
	cfg, pool, access := startServer(t)
	ctx := context.Background()
	rest := "http://" + cfg.HTTPAddr
	const form = "application/x-www-form-urlencoded"
	const json = "application/json"

	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, mcpTransport(cfg.MCPAddr, access), nil)
	if err != nil {
		t.Fatalf("MCP connect: %v", err)
	}
	defer session.Close()
	tool := func(name string, args map[string]any) map[string]any {
		t.Helper()
		result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("MCP %s: %v", name, err)
		}
		if result.IsError {
			t.Fatalf("MCP %s returned an error: %+v", name, result.Content)
		}
		return result.StructuredContent.(map[string]any)
	}

	// 1. MCP: the journal entry, in Gautam's words. The journal page of
	// the admin shows it, verified.
	entry := tool("journal", map[string]any{"body": "planned the tunnel.\nmust buy a domain"})
	if entry["verified"] != true {
		t.Fatalf("journal: %+v, want a verified entry", entry)
	}
	if status, body := send(t, "GET", rest+"/admin/journal", "", ""); status != 200 || !strings.Contains(body, "planned the tunnel.\nmust buy a domain") {
		t.Fatalf("GET /admin/journal: status %d, body %s", status, body)
	}

	// 2. MCP: the agent proposes a todo from the entry. REST shows it in
	// the review queue.
	todo := tool("todo", map[string]any{"title": "Buy a domain", "project": "lifeos"})
	todoID := int64(todo["id"].(float64))
	if status, body := send(t, "GET", rest+"/api/review", "", ""); status != 200 || !strings.Contains(body, "todo: Buy a domain, project lifeos") {
		t.Fatalf("GET /api/review: status %d, body %s", status, body)
	}

	// 3. Admin: Gautam approves the todo. REST marks it done.
	status, body := send(t, "POST", rest+"/admin/review/verify", form, fmt.Sprintf("type=todo&id=%d", todoID))
	if status != 200 || !strings.Contains(body, "No record waits for review.") {
		t.Fatalf("POST /admin/review/verify: status %d, body %s", status, body)
	}
	status, body = send(t, "PATCH", fmt.Sprintf("%s/api/todos/%d", rest, todoID), json, `{"status":"done"}`)
	if status != 200 || !strings.Contains(body, `"done_at"`) || !strings.Contains(body, `"verified":true`) {
		t.Fatalf("PATCH todo: status %d, body %s", status, body)
	}
	if got := tool("get", map[string]any{"type": "todo", "id": todoID}); got["status"] != "done" {
		t.Fatalf("MCP get of the todo: %+v, want it done", got)
	}

	// 4. Admin: Gautam adds a habit. REST logs it, as the Shortcut does.
	if status, body := send(t, "POST", rest+"/admin/habits", form, "name=Gym"); status != 200 || !strings.Contains(body, "not done") {
		t.Fatalf("POST /admin/habits: status %d, body %s", status, body)
	}
	status, body = send(t, "POST", rest+"/api/habits/log", json, `{"habit":"gym"}`)
	if status != 200 || !strings.Contains(body, `"habit":"gym"`) {
		t.Fatalf("POST /api/habits/log: status %d, body %s", status, body)
	}
	if status, body := send(t, "GET", rest+"/api/habits/today", "", ""); status != 200 || !strings.Contains(body, `"name":"gym","done":true`) {
		t.Fatalf("GET /api/habits/today: status %d, body %s", status, body)
	}

	// 5. MCP: the same habit and day again is the same log. A delete
	// that Gautam confirmed removes it, and the checklist shows that.
	log := tool("log_habit", map[string]any{"habit": "gym", "dictated": true})
	if log["source"] != "manual" {
		t.Fatalf("log_habit: %+v, want the log that the REST request stored", log)
	}
	tool("delete", map[string]any{"type": "habit_log", "id": log["id"], "confirmed": true})
	if _, body := send(t, "GET", rest+"/api/habits/today", "", ""); !strings.Contains(body, `"name":"gym","done":false`) {
		t.Errorf("GET /api/habits/today after the delete: %s", body)
	}

	// 6. Each write has its event, with the adapter that made it.
	rows, err := pool.Query(ctx, "SELECT action || ' ' || record_type || ' by ' || source FROM events ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	want := []string{
		"create journal_entries by agent",
		"create todos by agent",
		"verify todos by manual",
		"update todos by manual",
		"create habits by manual",
		"create habit_logs by manual",
		"delete habit_logs by agent",
	}
	if got := strings.Join(events, "\n"); got != strings.Join(want, "\n") {
		t.Errorf("events =\n%s\nwant\n%s", got, strings.Join(want, "\n"))
	}
}
