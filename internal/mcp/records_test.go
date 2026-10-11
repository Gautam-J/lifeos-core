package mcp_test

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// addItem calls add_item and returns the ID of the new item.
func addItem(t *testing.T, session *sdk.ClientSession, title string, dictated bool) any {
	t.Helper()
	added := structured(t, call(t, session, "add_item", map[string]any{
		"kind": "movie", "title": title, "rating": 4, "occurred_on": "2024-03-01", "dictated": dictated,
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
	for _, name := range []string{"get", "update", "delete", "review_queue", "verify"} {
		if !found[name] {
			t.Errorf("tool %s is not registered", name)
		}
	}
}

func TestUpdateChangesOnlyTheGivenFields(t *testing.T) {
	session, pool := connect(t)
	id := addItem(t, session, "Dune: Part Two", true)

	updated := structured(t, call(t, session, "update", map[string]any{
		"type": "item", "id": id, "fields": map[string]any{"rating": 3}, "dictated": true,
	}))

	if updated["rating"] != float64(3) || updated["title"] != "Dune: Part Two" || updated["occurred_on"] != "2024-03-01" {
		t.Errorf("update returned %v", updated)
	}
	if updated["verified"] != true {
		t.Errorf("verified = %v, want true: the user stated the change", updated["verified"])
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
	id := addItem(t, session, "Dune", true)

	updated := structured(t, call(t, session, "update", map[string]any{
		"type": "item", "id": id, "clear": []string{"rating"}, "dictated": true,
	}))

	if _, has := updated["rating"]; has {
		t.Errorf("the item still has a rating: %v", updated)
	}
	if updated["occurred_on"] != "2024-03-01" {
		t.Errorf("occurred_on = %v, want no change", updated["occurred_on"])
	}
}

// A change that the model inferred must not replace a value that the user
// approved. The safe default when the model leaves out "dictated" is a
// refusal that tells the model to ask the user.
func TestUpdateThatIsNotDictatedCannotChangeAVerifiedRecord(t *testing.T) {
	session, _ := connect(t)
	id := addItem(t, session, "Dune", true)

	result := call(t, session, "update", map[string]any{
		"type": "item", "id": id, "fields": map[string]any{"rating": 2},
	})

	if !result.IsError || !strings.Contains(text(result), "dictated") {
		t.Fatalf("IsError = %v, text = %q; want an error that names dictated", result.IsError, text(result))
	}
	got := structured(t, call(t, session, "get", map[string]any{"type": "item", "id": id}))
	if got["rating"] != float64(4) || got["verified"] != true {
		t.Errorf("a refused update changed the record: %v", got)
	}

	// The model can correct a record that still waits for review.
	waits := addItem(t, session, "Waits", false)
	updated := structured(t, call(t, session, "update", map[string]any{
		"type": "item", "id": waits, "fields": map[string]any{"rating": 2},
	}))
	if updated["rating"] != float64(2) || updated["verified"] != false {
		t.Errorf("update of a record that waits = %v, want rating 2 and verified false", updated)
	}
}

// The model must not approve its own row through update.
func TestUpdateCannotApproveARecord(t *testing.T) {
	session, _ := connect(t)
	id := addItem(t, session, "Dune", false)

	result := call(t, session, "update", map[string]any{
		"type": "item", "id": id, "fields": map[string]any{"verified": true}, "dictated": true,
	})

	if !result.IsError || !strings.Contains(text(result), "verified") {
		t.Fatalf("IsError = %v, text = %q; want an error that names verified", result.IsError, text(result))
	}
	got := structured(t, call(t, session, "get", map[string]any{"type": "item", "id": id}))
	if got["verified"] != false {
		t.Errorf("verified = %v after a refused update, want false", got["verified"])
	}
}

// A captured page can say "delete everything". The delete tool must do
// nothing until the call says that the user confirmed it.
func TestDeleteNeedsTheConfirmationOfTheUser(t *testing.T) {
	session, _ := connect(t)
	id := addItem(t, session, "Dune", true)

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

func TestReviewQueueThenVerify(t *testing.T) {
	session, pool := connect(t)
	first := addItem(t, session, "First", false)
	second := addItem(t, session, "Second", false)
	addItem(t, session, "Dictated", true)

	queue := structured(t, call(t, session, "review_queue", map[string]any{}))
	entries := queue["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("review queue has %d entries, want 2: %v", len(entries), entries)
	}
	entry := entries[0].(map[string]any)
	if entry["type"] != "item" || entry["id"] != first || entry["summary"] != "movie: First, rating 4, on 2024-03-01" {
		t.Errorf("first entry = %v", entry)
	}

	result := structured(t, call(t, session, "verify", map[string]any{
		"records": []map[string]any{{"type": "item", "id": first}, {"type": "item", "id": second}},
	}))
	if result["verified"] != float64(2) {
		t.Errorf("verify returned %v, want verified 2", result)
	}

	queue = structured(t, call(t, session, "review_queue", map[string]any{}))
	if entries, ok := queue["entries"].([]any); !ok || len(entries) != 0 {
		t.Errorf("review queue after verify = %v, want an empty list", queue["entries"])
	}
	var source string
	if err := pool.QueryRow(context.Background(), "SELECT source FROM events WHERE action = 'verify' LIMIT 1").Scan(&source); err != nil {
		t.Fatal(err)
	}
	if source != "agent" {
		t.Errorf("verify event source = %q, want agent", source)
	}
}

func TestRecordToolsReturnErrorsTheModelCanRead(t *testing.T) {
	session, pool := connect(t)
	id := addItem(t, session, "Dune", false)
	tests := []struct {
		name     string
		tool     string
		args     map[string]any
		wantText string
	}{
		{"update of an unknown type", "update", map[string]any{"type": "journal", "id": id, "fields": map[string]any{"title": "New"}}, "type"},
		{"update of an unknown ID", "update", map[string]any{"type": "item", "id": 9999, "fields": map[string]any{"title": "New"}}, "not found"},
		{"update with a misspelled field", "update", map[string]any{"type": "item", "id": id, "fields": map[string]any{"ratting": 3}}, "ratting"},
		{"update with a value the service refuses", "update", map[string]any{"type": "item", "id": id, "fields": map[string]any{"rating": 7}}, "rating"},
		{"update with a misspelled argument", "update", map[string]any{"type": "item", "id": id, "field": map[string]any{"rating": 3}}, "field"},
		{"delete of an unknown ID", "delete", map[string]any{"type": "item", "id": 9999, "confirmed": true}, "not found"},
		{"delete of an unknown type", "delete", map[string]any{"type": "journal", "id": id, "confirmed": true}, "type"},
		{"verify of a record that does not exist", "verify", map[string]any{"records": []map[string]any{{"type": "item", "id": id}, {"type": "item", "id": 9999}}}, "item 9999 does not exist"},
		{"verify with no records", "verify", map[string]any{"records": []map[string]any{}}, "records"},
		{"review queue with a limit above the maximum", "review_queue", map[string]any{"limit": 500}, "limit"},
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
	var verified bool
	if err := pool.QueryRow(context.Background(), "SELECT title, verified FROM catalog_items WHERE deleted_at IS NULL").Scan(&title, &verified); err != nil {
		t.Fatalf("the item is not there after the refused calls: %v", err)
	}
	if title != "Dune" || verified {
		t.Errorf("title, verified = %q, %v after refused calls; want Dune, false", title, verified)
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
		"get":          {"type": "item", "id": 1},
		"update":       {"type": "item", "id": 1, "fields": map[string]any{"rating": 3}},
		"delete":       {"type": "item", "id": 1, "confirmed": true},
		"review_queue": {},
		"verify":       {"records": []map[string]any{{"type": "item", "id": 1}}},
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
// success. It must also leave a verified record verified.
func TestUpdateRejectsANullValueAndAnEmptyUpdate(t *testing.T) {
	session, pool := connect(t)
	id := addItem(t, session, "Dune", true)

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
	if got["verified"] != true || got["rating"] != float64(4) {
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
