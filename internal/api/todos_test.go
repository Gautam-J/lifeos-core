package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const domainJSON = `{"title":"Buy a Cloudflare domain","due_on":"2026-10-20","project":"lifeos"}`

// addTodo stores one todo through the API and returns its path.
func addTodo(t *testing.T, h http.Handler, body string) string {
	t.Helper()
	status, added, raw := do(t, h, "POST", "/api/todos", body)
	if status != http.StatusCreated {
		t.Fatalf("POST /api/todos: status = %d, want 201\n%s", status, raw)
	}
	return fmt.Sprintf("/api/todos/%d", int64(added["id"].(float64)))
}

func TestAddTodoThenReadIt(t *testing.T) {
	h, _ := newTestAPI(t)

	status, added, raw := do(t, h, "POST", "/api/todos", domainJSON)

	if status != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201\n%s", status, raw)
	}
	if added["title"] != "Buy a Cloudflare domain" || added["status"] != "open" || added["due_on"] != "2026-10-20" || added["project"] != "lifeos" {
		t.Errorf("todo = %s", raw)
	}
	if added["source"] != "manual" {
		t.Errorf("source = %v, want manual", added["source"])
	}

	path := fmt.Sprintf("/api/todos/%d", int64(added["id"].(float64)))
	status, got, raw := do(t, h, "GET", path, "")
	if status != http.StatusOK || got["title"] != "Buy a Cloudflare domain" {
		t.Errorf("GET: status = %d, todo = %s", status, raw)
	}
}

func TestAddTodoRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantInBody string
	}{
		{"title of spaces", `{"title":"  "}`, 422, "body.title"},
		{"missing title", `{"project":"lifeos"}`, 422, "title"},
		{"date that is not a date", `{"title":"x","due_on":"soon"}`, 422, "body.due_on"},
		// A new todo is open. A caller cannot set the status or done_at.
		{"status", `{"title":"x","status":"done"}`, 422, "status"},
		{"done_at", `{"title":"x","done_at":"2026-10-11T00:00:00Z"}`, 422, "done_at"},
		{"request that is not JSON", `title=x`, 400, ""},
	}
	h, pool := newTestAPI(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, raw := do(t, h, "POST", "/api/todos", tt.body)

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d\n%s", status, tt.wantStatus, raw)
			}
			if !strings.Contains(raw, tt.wantInBody) {
				t.Errorf("body does not name %q:\n%s", tt.wantInBody, raw)
			}
		})
	}
	if n := countEvents(t, pool); n != 0 {
		t.Errorf("events = %d after rejected requests, want 0", n)
	}
}

func TestListTodos(t *testing.T) {
	h, _ := newTestAPI(t)

	status, _, raw := do(t, h, "GET", "/api/todos", "")
	if status != http.StatusOK || !strings.Contains(raw, `"todos":[]`) {
		t.Fatalf("empty list: status = %d, body = %s; want 200 and an empty array", status, raw)
	}

	done := addTodo(t, h, `{"title":"done"}`)
	addTodo(t, h, `{"title":"open"}`)
	if status, _, raw := do(t, h, "PATCH", done, `{"status":"done"}`); status != http.StatusOK {
		t.Fatalf("PATCH: status = %d\n%s", status, raw)
	}

	titles := func(query string) string {
		t.Helper()
		status, list, raw := do(t, h, "GET", "/api/todos"+query, "")
		if status != http.StatusOK {
			t.Fatalf("GET /api/todos%s: status = %d\n%s", query, status, raw)
		}
		var out []string
		for _, todo := range list["todos"].([]any) {
			out = append(out, todo.(map[string]any)["title"].(string))
		}
		return strings.Join(out, ",")
	}
	if got := titles(""); got != "open,done" {
		t.Errorf("all = %q, want open,done (the open todos first)", got)
	}
	if got := titles("?status=done"); got != "done" {
		t.Errorf("status=done = %q, want done", got)
	}
	if got := titles("?status=open&limit=1"); got != "open" {
		t.Errorf("status=open = %q, want open", got)
	}
	for _, query := range []string{"?status=later", "?limit=500"} {
		if status, _, raw := do(t, h, "GET", "/api/todos"+query, ""); status != http.StatusUnprocessableEntity {
			t.Errorf("GET /api/todos%s: status = %d, want 422\n%s", query, status, raw)
		}
	}
}

// "Done" is one field. The title, the project and the date must stay.
func TestPatchTodoMarksItDoneAndClearsAField(t *testing.T) {
	h, _ := newTestAPI(t)
	path := addTodo(t, h, domainJSON)

	status, patched, raw := do(t, h, "PATCH", path, `{"status":"done"}`)
	if status != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200\n%s", status, raw)
	}
	if patched["status"] != "done" || patched["done_at"] == nil || patched["title"] != "Buy a Cloudflare domain" || patched["project"] != "lifeos" || patched["due_on"] != "2026-10-20" {
		t.Errorf("todo after PATCH = %s", raw)
	}

	status, patched, raw = do(t, h, "PATCH", path, `{"status":"open","clear":["due_on","project"]}`)
	if status != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200\n%s", status, raw)
	}
	for _, field := range []string{"done_at", "due_on", "project"} {
		if _, has := patched[field]; has {
			t.Errorf("the todo still has %s:\n%s", field, raw)
		}
	}
}

func TestPatchTodoRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		body       string
		wantStatus int
		wantInBody string
	}{
		{"unknown status", "", `{"status":"later"}`, 422, "body.status"},
		{"empty title", "", `{"title":" "}`, 422, "body.title"},
		{"clear of a required field", "", `{"clear":["title"]}`, 422, "body.clear"},
		{"done_at", "", `{"done_at":"2026-10-11T00:00:00Z"}`, 422, "done_at"},
		{"field that a caller cannot change", "", `{"verified":false}`, 422, "verified"},
		{"null", "", `{"title":"New","due_on":null}`, 422, "due_on"},
		{"empty object", "", `{}`, 422, "body.fields"},
		{"body that is not JSON", "", `status=done`, 400, ""},
		{"unknown ID", "/api/todos/9999", `{"status":"done"}`, 404, ""},
		{"ID that is not a number", "/api/todos/abc", `{"status":"done"}`, 422, ""},
	}
	h, pool := newTestAPI(t)
	path := addTodo(t, h, domainJSON)
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
	if got["status"] != "open" || got["title"] != "Buy a Cloudflare domain" || got["due_on"] != "2026-10-20" {
		t.Errorf("a refused PATCH changed the todo: %s", raw)
	}
	if n := countEvents(t, pool); n != 1 {
		t.Errorf("events = %d after refused requests, want 1 (the create)", n)
	}
}

func TestDeleteTodo(t *testing.T) {
	h, pool := newTestAPI(t)
	path := addTodo(t, h, domainJSON)

	status, _, raw := do(t, h, "DELETE", path, "")

	if status != http.StatusNoContent || raw != "" {
		t.Fatalf("DELETE: status = %d, body = %q; want 204 and no body", status, raw)
	}
	if status, _, _ := do(t, h, "GET", path, ""); status != http.StatusNotFound {
		t.Errorf("GET after DELETE: status = %d, want 404", status)
	}
	if _, _, raw := do(t, h, "GET", "/api/todos", ""); !strings.Contains(raw, `"todos":[]`) {
		t.Errorf("the list still shows the todo:\n%s", raw)
	}
	for _, target := range []string{path, "/api/todos/9999"} {
		if status, _, raw := do(t, h, "DELETE", target, ""); status != http.StatusNotFound {
			t.Errorf("DELETE %s: status = %d, want 404\n%s", target, status, raw)
		}
	}
	if n := countEvents(t, pool); n != 2 {
		t.Errorf("events = %d, want 2 (create, one delete)", n)
	}
}

func TestDatabaseFailureOnATodoRouteGivesAPlain500(t *testing.T) {
	h, pool := newTestAPI(t)
	pool.Close()

	requests := []struct{ method, path, body string }{
		{"POST", "/api/todos", `{"title":"x"}`},
		{"GET", "/api/todos", ""},
		{"GET", "/api/todos/1", ""},
		{"PATCH", "/api/todos/1", `{"status":"done"}`},
		{"DELETE", "/api/todos/1", ""},
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
