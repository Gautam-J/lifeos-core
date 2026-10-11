package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// addInferred stores one unverified item, as an agent does, and returns
// its ID. The REST API itself stores only verified rows.
func addInferred(t *testing.T, pool *pgxpool.Pool, title string) int64 {
	t.Helper()
	item, err := service.New(pool).AddItem(t.Context(), service.AddItemParams{
		Kind:       "book",
		Title:      title,
		Provenance: service.Provenance{Source: service.SourceAgent, Verified: false},
	})
	if err != nil {
		t.Fatalf("add %q: %v", title, err)
	}
	return item.ID
}

func TestReviewQueueThenVerify(t *testing.T) {
	h, pool := newTestAPI(t)
	first := addInferred(t, pool, "First")
	second := addInferred(t, pool, "Second")
	addDune(t, h) // verified, so not in the queue

	status, queue, raw := do(t, h, "GET", "/api/review", "")
	if status != http.StatusOK {
		t.Fatalf("GET /api/review: status = %d, want 200\n%s", status, raw)
	}
	entries := queue["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("queue has %d entries, want 2\n%s", len(entries), raw)
	}
	entry := entries[0].(map[string]any)
	if entry["type"] != "item" || entry["id"] != float64(first) || entry["summary"] != "book: First" || entry["source"] != "agent" {
		t.Errorf("first entry = %v", entry)
	}

	body := fmt.Sprintf(`{"records":[{"type":"item","id":%d},{"type":"item","id":%d}]}`, first, second)
	status, result, raw := do(t, h, "POST", "/api/review/verify", body)
	if status != http.StatusOK || result["verified"] != float64(2) {
		t.Fatalf("POST /api/review/verify: status = %d, body = %s; want 200 and verified 2", status, raw)
	}

	// An empty queue must be a JSON list, not null.
	if _, _, raw := do(t, h, "GET", "/api/review", ""); !strings.Contains(raw, `"entries":[]`) {
		t.Errorf("queue after verify = %s, want an empty list", raw)
	}
	var source string
	if err := pool.QueryRow(t.Context(), "SELECT source FROM events WHERE action = 'verify' LIMIT 1").Scan(&source); err != nil {
		t.Fatal(err)
	}
	if source != "manual" {
		t.Errorf("verify event source = %q, want manual", source)
	}
}

func TestReviewRejectsBadRequests(t *testing.T) {
	h, pool := newTestAPI(t)
	id := addInferred(t, pool, "Waits")

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantInBody string
	}{
		{"limit above the maximum", "GET", "/api/review?limit=500", "", "query.limit"},
		{"no records", "POST", "/api/review/verify", `{"records":[]}`, "body.records"},
		{"record that does not exist", "POST", "/api/review/verify", fmt.Sprintf(`{"records":[{"type":"item","id":%d},{"type":"item","id":9999}]}`, id), "item 9999 does not exist"},
		{"unknown type", "POST", "/api/review/verify", fmt.Sprintf(`{"records":[{"type":"song","id":%d}]}`, id), "body.records"},
		{"misspelled field", "POST", "/api/review/verify", `{"record":[]}`, "record"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, raw := do(t, h, tt.method, tt.path, tt.body)
			if status != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want 422\n%s", status, raw)
			}
			if !strings.Contains(raw, tt.wantInBody) {
				t.Errorf("body does not name %q:\n%s", tt.wantInBody, raw)
			}
		})
	}
	var verified bool
	if err := pool.QueryRow(t.Context(), "SELECT verified FROM catalog_items WHERE id = $1", id).Scan(&verified); err != nil {
		t.Fatal(err)
	}
	if verified {
		t.Error("a refused request verified the item")
	}
}

func TestDatabaseFailureOnAChangeGivesAPlain500(t *testing.T) {
	h, pool := newTestAPI(t)
	pool.Close()

	requests := []struct{ method, path, body string }{
		{"PATCH", "/api/items/1", `{"rating":3}`},
		{"DELETE", "/api/items/1", ""},
		{"GET", "/api/review", ""},
		{"POST", "/api/review/verify", `{"records":[{"type":"item","id":1}]}`},
	}
	for _, r := range requests {
		status, body, raw := do(t, h, r.method, r.path, r.body)
		if status != http.StatusInternalServerError {
			t.Errorf("%s %s: status = %d, want 500\n%s", r.method, r.path, status, raw)
		}
		if body["detail"] != "internal error" || body["errors"] != nil {
			t.Errorf("%s %s: the response is not the plain error:\n%s", r.method, r.path, raw)
		}
	}
}
