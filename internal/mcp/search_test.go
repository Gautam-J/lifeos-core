package mcp_test

import (
	"strings"
	"testing"
)

func TestSearchFindsTheRecordToChange(t *testing.T) {
	session, _ := connect(t)
	call(t, session, "todo", map[string]any{"title": "Renew the car insurance", "dictated": true})
	call(t, session, "journal", map[string]any{"body": "Paid the insurance today."})
	proposed := structured(t, call(t, session, "todo", map[string]any{"title": "Call the insurance agent"}))

	found := structured(t, call(t, session, "search", map[string]any{"query": "INSURANCE"}))
	hits := found["hits"].([]any)
	if len(hits) != 3 {
		t.Fatalf("hits = %v, want 3", hits)
	}
	// The newest record is first. It waits for approval.
	first := hits[0].(map[string]any)
	if first["type"] != "todo" || first["id"] != proposed["id"] || first["summary"] != "todo: Call the insurance agent" || first["verified"] != false {
		t.Errorf("first hit = %v", first)
	}

	// The agent finds the todo, then marks it done with its ID.
	todos := structured(t, call(t, session, "search", map[string]any{"query": "renew insurance", "types": []string{"todo"}}))
	hits = todos["hits"].([]any)
	if len(hits) != 1 {
		t.Fatalf("hits for two words in todos = %v, want 1", hits)
	}
	done := structured(t, call(t, session, "update", map[string]any{
		"type": "todo", "id": hits[0].(map[string]any)["id"], "fields": map[string]any{"status": "done"}, "dictated": true,
	}))
	if done["status"] != "done" {
		t.Errorf("todo after the update = %v", done)
	}

	none := structured(t, call(t, session, "search", map[string]any{"query": "snow"}))
	if list, ok := none["hits"].([]any); !ok || len(list) != 0 {
		t.Errorf("hits = %v, want an empty list", none["hits"])
	}
}

func TestSearchRejectsBadInput(t *testing.T) {
	session, _ := connect(t)

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"a query of spaces", map[string]any{"query": "  "}, "query: must not be empty"},
		{"a type that search does not look in", map[string]any{"query": "gym", "types": []string{"habit_log"}}, `types: "habit_log" is not a type that search looks in`},
		{"a limit over the maximum", map[string]any{"query": "gym", "limit": 201}, "limit: must be from 1 to 200"},
		{"no query", map[string]any{}, "query"},
	}
	for _, tt := range tests {
		result := call(t, session, "search", tt.args)
		if !result.IsError || !strings.Contains(text(result), tt.want) {
			t.Errorf("%s: result = %q, want an error with %q", tt.name, text(result), tt.want)
		}
	}
}
