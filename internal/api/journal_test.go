package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// todayIST is today's date for Gautam, as the service computes it.
func todayIST() string {
	return time.Now().In(service.IST).Format(time.DateOnly)
}

// addJournal stores one journal entry through the API and returns its path.
func addJournal(t *testing.T, h http.Handler, body string) string {
	t.Helper()
	status, added, raw := do(t, h, "POST", "/api/journal", body)
	if status != http.StatusCreated {
		t.Fatalf("POST /api/journal: status = %d, want 201\n%s", status, raw)
	}
	return fmt.Sprintf("/api/journal/%d", int64(added["id"].(float64)))
}

func TestAddJournalEntryThenReadIt(t *testing.T) {
	h, _ := newTestAPI(t)

	status, added, raw := do(t, h, "POST", "/api/journal", `{"body":"  planned the schema\n\nno gym :(  \n"}`)

	if status != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201\n%s", status, raw)
	}
	if added["body"] != "  planned the schema\n\nno gym :(  \n" {
		t.Errorf("body = %q, want it exactly as it was sent", added["body"])
	}
	if added["entry_date"] != todayIST() {
		t.Errorf("entry_date = %v, want today in IST (%s)", added["entry_date"], todayIST())
	}
	if added["source"] != "manual" || added["verified"] != true {
		t.Errorf("source, verified = %v, %v, want manual, true", added["source"], added["verified"])
	}

	path := fmt.Sprintf("/api/journal/%d", int64(added["id"].(float64)))
	status, got, raw := do(t, h, "GET", path, "")
	if status != http.StatusOK || got["body"] != added["body"] || got["entry_date"] != added["entry_date"] {
		t.Errorf("GET: status = %d, entry = %s", status, raw)
	}
}

func TestAddJournalEntryRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantInBody string
	}{
		{"body of spaces", `{"body":"  "}`, 422, "body.body"},
		{"missing body", `{"entry_date":"2026-01-01"}`, 422, "body"},
		{"date in the future", `{"body":"x","entry_date":"2999-01-01"}`, 422, "body.entry_date"},
		{"date that is not a date", `{"body":"x","entry_date":"yesterday"}`, 422, "body.entry_date"},
		{"field that a caller cannot set", `{"body":"x","verified":false}`, 422, "verified"},
		{"body of the wrong type", `{"body":5}`, 422, "body"},
		{"request that is not JSON", `body=x`, 400, ""},
	}
	h, pool := newTestAPI(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, raw := do(t, h, "POST", "/api/journal", tt.body)

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d\n%s", status, tt.wantStatus, raw)
			}
			if !strings.Contains(raw, tt.wantInBody) {
				t.Errorf("body does not name %q:\n%s", tt.wantInBody, raw)
			}
		})
	}
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM journal_entries").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("journal_entries has %d rows after rejected requests, want 0", n)
	}
}

func TestPatchJournalEntry(t *testing.T) {
	h, pool := newTestAPI(t)
	path := addJournal(t, h, `{"body":"a day","entry_date":"2026-01-05"}`)

	status, patched, raw := do(t, h, "PATCH", path, `{"entry_date":"2026-01-04"}`)
	if status != http.StatusOK || patched["entry_date"] != "2026-01-04" || patched["body"] != "a day" {
		t.Fatalf("PATCH of the date: status = %d\n%s", status, raw)
	}

	refused := []struct{ name, body, wantInBody string }{
		{"null", `{"body":null}`, "body.body"},
		{"empty object", `{}`, "body.fields"},
		{"empty body text", `{"body":" "}`, "body.body"},
		{"field that a caller cannot change", `{"verified":false}`, "verified"},
		{"clear", `{"body":"x","clear":["entry_date"]}`, "clear"},
	}
	for _, tt := range refused {
		status, _, raw := do(t, h, "PATCH", path, tt.body)
		if status != http.StatusUnprocessableEntity || !strings.Contains(raw, tt.wantInBody) {
			t.Errorf("%s: status = %d, want 422 that names %q\n%s", tt.name, status, tt.wantInBody, raw)
		}
	}
	if status, _, raw := do(t, h, "PATCH", "/api/journal/9999", `{"body":"x"}`); status != http.StatusNotFound {
		t.Errorf("unknown ID: status = %d, want 404\n%s", status, raw)
	}

	_, got, raw := do(t, h, "GET", path, "")
	if got["body"] != "a day" || got["entry_date"] != "2026-01-04" || got["verified"] != true {
		t.Errorf("entry after the refused requests = %s", raw)
	}
	if n := countEvents(t, pool); n != 2 {
		t.Errorf("events = %d, want 2 (create, one update)", n)
	}
}

func TestDeleteJournalEntry(t *testing.T) {
	h, pool := newTestAPI(t)
	path := addJournal(t, h, `{"body":"a day"}`)

	status, _, raw := do(t, h, "DELETE", path, "")

	if status != http.StatusNoContent || raw != "" {
		t.Fatalf("DELETE: status = %d, body = %q; want 204 and no body", status, raw)
	}
	if status, _, _ := do(t, h, "GET", path, ""); status != http.StatusNotFound {
		t.Errorf("GET after DELETE: status = %d, want 404", status)
	}
	for _, target := range []string{path, "/api/journal/9999"} {
		if status, _, raw := do(t, h, "DELETE", target, ""); status != http.StatusNotFound {
			t.Errorf("DELETE %s: status = %d, want 404\n%s", target, status, raw)
		}
	}
	if status, _, raw := do(t, h, "GET", "/api/journal/abc", ""); status != http.StatusUnprocessableEntity {
		t.Errorf("ID that is not a number: status = %d, want 422\n%s", status, raw)
	}
	if n := countEvents(t, pool); n != 2 {
		t.Errorf("events = %d, want 2 (create, one delete)", n)
	}
}

func TestDatabaseFailureOnAJournalRouteGivesAPlain500(t *testing.T) {
	h, pool := newTestAPI(t)
	pool.Close()

	requests := []struct{ method, path, body string }{
		{"POST", "/api/journal", `{"body":"a day"}`},
		{"GET", "/api/journal/1", ""},
		{"PATCH", "/api/journal/1", `{"body":"x"}`},
		{"DELETE", "/api/journal/1", ""},
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
