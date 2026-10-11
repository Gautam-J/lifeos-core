package api_test

import (
	"net/http"
	"strings"
	"testing"
)

// hitTypes returns the type of each hit, in order.
func hitTypes(body map[string]any) []string {
	types := []string{}
	for _, hit := range body["hits"].([]any) {
		types = append(types, hit.(map[string]any)["type"].(string))
	}
	return types
}

func TestSearchFindsRecordsOfEachType(t *testing.T) {
	h, _ := newTestAPI(t)
	do(t, h, "POST", "/api/items", `{"kind":"place","title":"Lonavala"}`)
	do(t, h, "POST", "/api/journal", `{"body":"Went to Lonavala. It rained."}`)
	do(t, h, "POST", "/api/todos", `{"title":"Book a room in Lonavala"}`)
	do(t, h, "POST", "/api/ingest", `{"url":"https://example.com/lonavala-guide"}`)

	status, body, raw := do(t, h, "GET", "/api/search?query=lonav", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", status, raw)
	}
	// The newest record is first.
	if got := strings.Join(hitTypes(body), " "); got != "source todo journal item" {
		t.Errorf("types of the hits = %q, want source todo journal item\n%s", got, raw)
	}
	first := body["hits"].([]any)[0].(map[string]any)
	if first["summary"] != "link: https://example.com/lonavala-guide" || first["verified"] != true || first["id"] == nil {
		t.Errorf("first hit = %v", first)
	}

	_, body, raw = do(t, h, "GET", "/api/search?query=lonavala&types=journal,todo&limit=1", "")
	if got := strings.Join(hitTypes(body), " "); got != "todo" {
		t.Errorf("types with a filter and a limit = %q, want todo\n%s", got, raw)
	}
	_, body, raw = do(t, h, "GET", "/api/search?query=rained+lonavala", "")
	if got := strings.Join(hitTypes(body), " "); got != "journal" {
		t.Errorf("types for two words = %q, want journal\n%s", got, raw)
	}
	if _, _, raw := do(t, h, "GET", "/api/search?query=snow", ""); !strings.Contains(raw, `"hits":[]`) {
		t.Errorf("no hit: body = %s, want an empty list", raw)
	}
}

func TestSearchRejectsBadRequests(t *testing.T) {
	h, _ := newTestAPI(t)
	tests := []struct {
		name, path, wantInBody string
	}{
		{"no query", "/api/search", "query.query"},
		{"a query of spaces", "/api/search?query=+++", "query.query"},
		{"a type that search does not look in", "/api/search?query=gym&types=habit_log", "query.types"},
		{"a limit over the maximum", "/api/search?query=gym&limit=201", "query.limit"},
		{"a limit that is not a number", "/api/search?query=gym&limit=many", "query.limit"},
	}
	for _, tt := range tests {
		status, _, raw := do(t, h, "GET", tt.path, "")
		if status != http.StatusUnprocessableEntity || !strings.Contains(raw, tt.wantInBody) {
			t.Errorf("%s: status = %d, want 422 with %q\n%s", tt.name, status, tt.wantInBody, raw)
		}
	}
}
