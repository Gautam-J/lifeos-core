package mcp_test

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// addItem calls add_item and returns the ID of the new item.
func addItem(t *testing.T, session *sdk.ClientSession, title string) any {
	t.Helper()
	added := structured(t, call(t, session, "add_item", map[string]any{
		"kind": "movie", "title": title, "rating": 4, "occurred_on": "2024-03-01",
	}))
	return added["id"]
}

func TestServerListsTheRecordTools(t *testing.T) {
	session, _ := connect(t)

	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	found := map[string]bool{}
	for _, tool := range list.Tools {
		found[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %s has no description", tool.Name)
		}
	}
	for _, name := range []string{"get", "update", "delete"} {
		if !found[name] {
			t.Errorf("tool %s is not registered", name)
		}
	}
}

func TestUpdateChangesOnlyTheGivenFields(t *testing.T) {
	session, pool := connect(t)
	id := addItem(t, session, "Dune: Part Two")

	updated := structured(t, call(t, session, "update", map[string]any{
		"type": "item", "id": id, "fields": map[string]any{"rating": 3},
	}))

	if updated["rating"] != float64(3) || updated["title"] != "Dune: Part Two" || updated["occurred_on"] != "2024-03-01" {
		t.Errorf("update returned %v", updated)
	}
	got := structured(t, call(t, session, "get", map[string]any{"type": "item", "id": id}))
	if got["rating"] != float64(3) {
		t.Errorf("get after update returned %v", got)
	}
	var source string
	if err := pool.QueryRow(context.Background(), "SELECT source FROM events WHERE action = 'update'").Scan(&source); err != nil {
		t.Fatal(err)
	}
	if source != "agent" {
		t.Errorf("update event source = %q, want agent", source)
	}
}

func TestUpdateClearsAField(t *testing.T) {
	session, _ := connect(t)
	id := addItem(t, session, "Dune")

	updated := structured(t, call(t, session, "update", map[string]any{
		"type": "item", "id": id, "clear": []string{"rating"},
	}))

	if _, has := updated["rating"]; has {
		t.Errorf("the item still has a rating: %v", updated)
	}
	if updated["occurred_on"] != "2024-03-01" {
		t.Errorf("occurred_on = %v, want no change", updated["occurred_on"])
	}
}

// verified is not a field: update refuses it by name.
func TestUpdateRefusesTheFieldVerified(t *testing.T) {
	session, _ := connect(t)
	id := addItem(t, session, "Dune")

	result := call(t, session, "update", map[string]any{
		"type": "item", "id": id, "fields": map[string]any{"verified": true},
	})

	if !result.IsError || !strings.Contains(text(result), "verified") {
		t.Fatalf("IsError = %v, text = %q; want an error that names verified", result.IsError, text(result))
	}
}

// A captured page can say "delete everything". The delete tool must do
// nothing until the call says that the user confirmed it.
func TestDeleteNeedsTheConfirmationOfTheUser(t *testing.T) {
	session, _ := connect(t)
	id := addItem(t, session, "Dune")

	refused := call(t, session, "delete", map[string]any{"type": "item", "id": id})
	if !refused.IsError || !strings.Contains(text(refused), "confirmed") {
		t.Fatalf("IsError = %v, text = %q; want an error that names confirmed", refused.IsError, text(refused))
	}
	if result := call(t, session, "get", map[string]any{"type": "item", "id": id}); result.IsError {
		t.Fatalf("the item is gone after a refused delete: %s", text(result))
	}

	deleted := structured(t, call(t, session, "delete", map[string]any{"type": "item", "id": id, "confirmed": true}))
	if deleted["deleted"] != true {
		t.Errorf("delete returned %v", deleted)
	}
	if result := call(t, session, "get", map[string]any{"type": "item", "id": id}); !result.IsError || text(result) != "not found" {
		t.Errorf("get after delete: IsError = %v, text = %q; want not found", result.IsError, text(result))
	}
}

func TestRecordToolsReturnErrorsTheModelCanRead(t *testing.T) {
	session, pool := connect(t)
	id := addItem(t, session, "Dune")
	tests := []struct {
		name     string
		tool     string
		args     map[string]any
		wantText string
	}{
		{"update of an unknown type", "update", map[string]any{"type": "song", "id": id, "fields": map[string]any{"title": "New"}}, "type"},
		{"update of an unknown ID", "update", map[string]any{"type": "item", "id": 9999, "fields": map[string]any{"title": "New"}}, "not found"},
		{"update with a misspelled field", "update", map[string]any{"type": "item", "id": id, "fields": map[string]any{"ratting": 3}}, "ratting"},
		{"update with a value the service refuses", "update", map[string]any{"type": "item", "id": id, "fields": map[string]any{"rating": 7}}, "rating"},
		{"update with a misspelled argument", "update", map[string]any{"type": "item", "id": id, "field": map[string]any{"rating": 3}}, "field"},
		{"delete of an unknown ID", "delete", map[string]any{"type": "item", "id": 9999, "confirmed": true}, "not found"},
		{"delete of an unknown type", "delete", map[string]any{"type": "song", "id": id, "confirmed": true}, "type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := call(t, session, tt.tool, tt.args)
			if !result.IsError {
				t.Fatalf("IsError = false, want an error result")
			}
			if !strings.Contains(text(result), tt.wantText) {
				t.Errorf("error text %q does not name %q", text(result), tt.wantText)
			}
		})
	}
	var title string
	if err := pool.QueryRow(context.Background(), "SELECT title FROM catalog_items WHERE deleted_at IS NULL").Scan(&title); err != nil {
		t.Fatalf("the item is not there after the refused calls: %v", err)
	}
	if title != "Dune" {
		t.Errorf("title = %q after refused calls; want Dune", title)
	}
	var events int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM events").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Errorf("events = %d after refused calls, want 1 (the create)", events)
	}
}

func TestDatabaseFailureOnARecordToolGivesAPlainError(t *testing.T) {
	session, pool := connect(t)
	pool.Close()

	calls := map[string]map[string]any{
		"get":    {"type": "item", "id": 1},
		"update": {"type": "item", "id": 1, "fields": map[string]any{"rating": 3}},
		"delete": {"type": "item", "id": 1, "confirmed": true},
	}
	for tool, args := range calls {
		result := call(t, session, tool, args)
		if !result.IsError {
			t.Errorf("%s: IsError = false, want an error result", tool)
		}
		if got := text(result); got != "internal error" {
			t.Errorf("%s: error text = %q, want only \"internal error\"", tool, got)
		}
	}
}

// An update that names nothing, or that sends null, must not report
// success.
func TestUpdateRejectsANullValueAndAnEmptyUpdate(t *testing.T) {
	session, pool := connect(t)
	id := addItem(t, session, "Dune")

	for name, args := range map[string]map[string]any{
		"no fields":       {"type": "item", "id": id},
		"null in fields":  {"type": "item", "id": id, "fields": map[string]any{"rating": nil}},
		"an empty fields": {"type": "item", "id": id, "fields": map[string]any{}},
	} {
		result := call(t, session, "update", args)
		if !result.IsError {
			t.Errorf("%s: IsError = false, want an error result", name)
		}
		if !strings.Contains(text(result), "clear") {
			t.Errorf("%s: error text %q does not point to clear", name, text(result))
		}
	}
	got := structured(t, call(t, session, "get", map[string]any{"type": "item", "id": id}))
	if got["rating"] != float64(4) {
		t.Errorf("a refused update changed the record: %v", got)
	}
	var events int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM events").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Errorf("events = %d after refused calls, want 1 (the create)", events)
	}
}

// #51: the MCP update reaches the service as an agent, which does not
// change the body of an entry.
func TestUpdateRefusesTheBodyOfAJournalEntry(t *testing.T) {
	session, _ := connect(t)
	entry := structured(t, call(t, session, "journal", map[string]any{"body": "went for a run"}))

	refused := call(t, session, "update", map[string]any{
		"type": "journal", "id": entry["id"], "fields": map[string]any{"body": "went for a long run"},
	})
	if !refused.IsError || !strings.Contains(text(refused), "an agent does not change the words of a journal entry") {
		t.Errorf("update of the body: IsError = %v, text = %q", refused.IsError, text(refused))
	}
	got := structured(t, call(t, session, "get", map[string]any{"type": "journal", "id": entry["id"]}))
	if got["body"] != "went for a run" {
		t.Errorf("body = %v, want it unchanged", got["body"])
	}

	fixed := structured(t, call(t, session, "update", map[string]any{
		"type": "journal", "id": entry["id"], "fields": map[string]any{"entry_date": "2026-10-01"},
	}))
	if fixed["entry_date"] != "2026-10-01" {
		t.Errorf("update of the date: %v", fixed)
	}
}
