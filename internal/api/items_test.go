package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/api"
	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

func newTestAPI(t *testing.T) (http.Handler, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.New(t)
	router := chi.NewRouter()
	api.Register(router, service.New(pool))
	return router, pool
}

// do sends one request and returns the status and the decoded JSON body.
func do(t *testing.T, h http.Handler, method, path, body string) (int, map[string]any, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	raw := rec.Body.String()
	var decoded map[string]any
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			t.Fatalf("%s %s: body is not a JSON object: %v\n%s", method, path, err, raw)
		}
	}
	return rec.Code, decoded, raw
}

const duneJSON = `{"kind":"movie","title":"Dune: Part Two","rating":4.5,"occurred_on":"2024-03-01","attrs":{"year":2024}}`

func TestAddItemThenReadIt(t *testing.T) {
	h, _ := newTestAPI(t)

	status, added, raw := do(t, h, "POST", "/api/items", duneJSON)
	if status != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201\n%s", status, raw)
	}
	if added["source"] != "manual" || added["verified"] != true {
		t.Errorf("source, verified = %v, %v, want manual, true", added["source"], added["verified"])
	}
	id, ok := added["id"].(float64)
	if !ok || id < 1 {
		t.Fatalf("id = %v, want a positive number", added["id"])
	}

	status, got, raw := do(t, h, "GET", fmt.Sprintf("/api/items/%d", int64(id)), "")
	if status != http.StatusOK {
		t.Fatalf("GET status = %d, want 200\n%s", status, raw)
	}
	if got["title"] != "Dune: Part Two" || got["rating"] != 4.5 || got["occurred_on"] != "2024-03-01" {
		t.Errorf("item = %s", raw)
	}
}

func TestAddItemRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantInBody string
	}{
		{"rating the service refuses", `{"kind":"movie","title":"Dune","rating":7}`, 422, "body.rating"},
		{"kind the service refuses", `{"kind":"film","title":"Dune"}`, 422, "body.kind"},
		{"missing title", `{"kind":"movie"}`, 422, "title"},
		{"misspelled field", `{"kind":"movie","title":"Dune","ratting":4}`, 422, "ratting"},
		{"rating of the wrong type", `{"kind":"movie","title":"Dune","rating":"four"}`, 422, "rating"},
		{"body that is not JSON", `kind=movie`, 400, ""},
		{"empty body", ``, 400, ""},
	}
	h, pool := newTestAPI(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/items", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d\n%s", rec.Code, tt.wantStatus, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), tt.wantInBody) {
				t.Errorf("body does not name %q:\n%s", tt.wantInBody, rec.Body)
			}
		})
	}
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM catalog_items").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("catalog_items has %d rows after rejected requests, want 0", n)
	}
}

func TestGetItemErrors(t *testing.T) {
	h, _ := newTestAPI(t)

	if status, _, raw := do(t, h, "GET", "/api/items/9999", ""); status != http.StatusNotFound {
		t.Errorf("unknown ID: status = %d, want 404\n%s", status, raw)
	}
	if status, _, raw := do(t, h, "GET", "/api/items/abc", ""); status != http.StatusUnprocessableEntity {
		t.Errorf("ID that is not a number: status = %d, want 422\n%s", status, raw)
	}
}

func TestListItems(t *testing.T) {
	h, _ := newTestAPI(t)

	status, empty, raw := do(t, h, "GET", "/api/items", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", status, raw)
	}
	if items, ok := empty["items"].([]any); !ok || len(items) != 0 {
		t.Errorf("empty list = %s, want {\"items\":[]}", raw)
	}

	do(t, h, "POST", "/api/items", `{"kind":"book","title":"First"}`)
	do(t, h, "POST", "/api/items", `{"kind":"movie","title":"Second"}`)
	do(t, h, "POST", "/api/items", `{"kind":"book","title":"Third"}`)

	_, books, raw := do(t, h, "GET", "/api/items?kind=book&limit=1", "")
	items, _ := books["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["title"] != "Third" {
		t.Errorf("kind=book&limit=1 = %s, want only Third", raw)
	}

	status, _, raw = do(t, h, "GET", "/api/items?kind=film", "")
	if status != http.StatusUnprocessableEntity || !strings.Contains(raw, "query.kind") {
		t.Errorf("kind=film: status = %d, want 422 that names query.kind\n%s", status, raw)
	}
}

// A database failure must give a plain 500. The response must not show
// internal error text.
func TestDatabaseFailureGivesAPlain500(t *testing.T) {
	h, pool := newTestAPI(t)
	pool.Close()

	status, _, raw := do(t, h, "POST", "/api/items", duneJSON)

	if status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", status)
	}
	if strings.Contains(raw, "pool") || strings.Contains(raw, "add item") {
		t.Errorf("the response shows internal error text:\n%s", raw)
	}
}
