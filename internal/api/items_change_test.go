package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// addDune adds the item of duneJSON and returns its path, /api/items/<id>.
func addDune(t *testing.T, h http.Handler) string {
	t.Helper()
	status, added, raw := do(t, h, "POST", "/api/items", duneJSON)
	if status != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201\n%s", status, raw)
	}
	return fmt.Sprintf("/api/items/%d", int64(added["id"].(float64)))
}

func countEvents(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM events").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A PATCH names the fields to change. A field that the body leaves out
// must keep its value.
func TestPatchItemChangesOnlyTheGivenFields(t *testing.T) {
	h, _ := newTestAPI(t)
	path := addDune(t, h)

	status, patched, raw := do(t, h, "PATCH", path, `{"rating":3}`)

	if status != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200\n%s", status, raw)
	}
	if patched["rating"] != float64(3) {
		t.Errorf("rating = %v, want 3", patched["rating"])
	}
	_, got, raw := do(t, h, "GET", path, "")
	if got["rating"] != float64(3) || got["title"] != "Dune: Part Two" || got["occurred_on"] != "2024-03-01" || got["kind"] != "movie" {
		t.Errorf("item after PATCH = %s", raw)
	}
	if got["verified"] != true || got["source"] != "manual" {
		t.Errorf("verified, source = %v, %v; want no change", got["verified"], got["source"])
	}
}

func TestPatchItemClearsAField(t *testing.T) {
	h, _ := newTestAPI(t)
	path := addDune(t, h)

	status, patched, raw := do(t, h, "PATCH", path, `{"title":"Dune 2","clear":["rating","occurred_on"]}`)

	if status != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200\n%s", status, raw)
	}
	if patched["title"] != "Dune 2" {
		t.Errorf("title = %v, want Dune 2", patched["title"])
	}
	if _, has := patched["rating"]; has {
		t.Errorf("the item still has a rating:\n%s", raw)
	}
	if _, has := patched["occurred_on"]; has {
		t.Errorf("the item still has a date:\n%s", raw)
	}
}

func TestPatchItemRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		body       string
		wantStatus int
		wantInBody string
	}{
		{"rating the service refuses", "", `{"rating":7}`, 422, "body.rating"},
		{"empty title", "", `{"title":" "}`, 422, "body.title"},
		{"clear of a required field", "", `{"clear":["title"]}`, 422, "body.clear"},
		{"misspelled field", "", `{"ratting":3}`, 422, "ratting"},
		{"field that a caller cannot change", "", `{"verified":false}`, 422, "verified"},
		{"rating of the wrong type", "", `{"rating":"three"}`, 422, "rating"},
		{"body that is not JSON", "", `rating=3`, 400, ""},
		{"unknown ID", "/api/items/9999", `{"rating":3}`, 404, ""},
		{"ID that is not a number", "/api/items/abc", `{"rating":3}`, 422, ""},
	}
	h, pool := newTestAPI(t)
	path := addDune(t, h)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := path
			if tt.path != "" {
				target = tt.path
			}
			status, _, raw := do(t, h, "PATCH", target, tt.body)

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d\n%s", status, tt.wantStatus, raw)
			}
			if !strings.Contains(raw, tt.wantInBody) {
				t.Errorf("body does not name %q:\n%s", tt.wantInBody, raw)
			}
		})
	}
	_, got, raw := do(t, h, "GET", path, "")
	if got["rating"] != 4.5 || got["title"] != "Dune: Part Two" || got["verified"] != true {
		t.Errorf("a refused PATCH changed the item: %s", raw)
	}
	if n := countEvents(t, pool); n != 1 {
		t.Errorf("events = %d after refused requests, want 1 (the create)", n)
	}
}

func TestDeleteItem(t *testing.T) {
	h, pool := newTestAPI(t)
	path := addDune(t, h)

	status, _, raw := do(t, h, "DELETE", path, "")

	if status != http.StatusNoContent || raw != "" {
		t.Fatalf("DELETE: status = %d, body = %q; want 204 and no body", status, raw)
	}
	if status, _, _ := do(t, h, "GET", path, ""); status != http.StatusNotFound {
		t.Errorf("GET after DELETE: status = %d, want 404", status)
	}
	if _, list, raw := do(t, h, "GET", "/api/items", ""); len(list["items"].([]any)) != 0 {
		t.Errorf("the list still shows the item:\n%s", raw)
	}
	var action, source string
	if err := pool.QueryRow(t.Context(), "SELECT action, source FROM events ORDER BY id DESC LIMIT 1").Scan(&action, &source); err != nil {
		t.Fatal(err)
	}
	if action != "delete" || source != "manual" {
		t.Errorf("last event = %s by %s, want delete by manual", action, source)
	}

	// A second DELETE, and a DELETE of an ID that never existed.
	for _, target := range []string{path, "/api/items/9999"} {
		if status, _, raw := do(t, h, "DELETE", target, ""); status != http.StatusNotFound {
			t.Errorf("DELETE %s: status = %d, want 404\n%s", target, status, raw)
		}
	}
	if n := countEvents(t, pool); n != 2 {
		t.Errorf("events = %d, want 2 (create, one delete)", n)
	}
}
