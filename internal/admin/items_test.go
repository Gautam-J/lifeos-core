package admin_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/admin"
	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

func newTestAdmin(t *testing.T) (http.Handler, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.New(t)
	router := chi.NewRouter()
	admin.Register(router, service.New(pool))
	return router, pool
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func postForm(h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestItemsPageShowsTheFormAndAnEmptyList(t *testing.T) {
	h, _ := newTestAdmin(t)

	rec := get(h, "/admin/items")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{`hx-post="/admin/items"`, `name="title"`, `<option value="movie">`, "No items yet.", "/admin/static/htmx.min.js"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %q", want)
		}
	}
}

func TestAddItemStoresTheRowAndShowsIt(t *testing.T) {
	h, pool := newTestAdmin(t)

	rec := postForm(h, "/admin/items", url.Values{
		"kind": {"movie"}, "title": {"Dune: Part Two"}, "rating": {"4.5"}, "occurred_on": {"2024-03-01"},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("HX-Trigger"); got != "item-added" {
		t.Errorf("HX-Trigger = %q, want item-added so that the form resets", got)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(body), `<div id="list">`) {
		t.Errorf("the response is not the #list fragment:\n%s", body)
	}
	for _, want := range []string{"Dune: Part Two", "4.5", "2024-03-01", "manual", "yes"} {
		if !strings.Contains(body, want) {
			t.Errorf("the list does not contain %q", want)
		}
	}

	var source string
	var verified bool
	if err := pool.QueryRow(t.Context(), "SELECT source, verified FROM catalog_items").Scan(&source, &verified); err != nil {
		t.Fatalf("read the row (want exactly one): %v", err)
	}
	if source != "manual" || !verified {
		t.Errorf("source, verified = %q, %v, want manual, true", source, verified)
	}

	if page := get(h, "/admin/items").Body.String(); !strings.Contains(page, "Dune: Part Two") {
		t.Error("the full page does not list the new item")
	}
}

func TestAddItemShowsWhyTheServiceRefusedTheInput(t *testing.T) {
	tests := []struct {
		name     string
		form     url.Values
		wantText string
	}{
		{"rating above 5", url.Values{"kind": {"movie"}, "title": {"Dune"}, "rating": {"7"}}, "rating: must be from 0 to 5"},
		{"rating that is not a number", url.Values{"kind": {"movie"}, "title": {"Dune"}, "rating": {"four"}}, "rating: must be a number"},
		{"no title", url.Values{"kind": {"movie"}, "title": {""}}, "title: must not be empty"},
		{"no kind", url.Values{"title": {"Dune"}}, "kind: must be one of"},
		{"date in the future", url.Values{"kind": {"movie"}, "title": {"Dune"}, "occurred_on": {"2099-01-01"}}, "occurred_on: must not be in the future"},
	}
	h, pool := newTestAdmin(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postForm(h, "/admin/items", tt.form)

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200: htmx does not show a 4xx response", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), tt.wantText) {
				t.Errorf("the response does not show %q:\n%s", tt.wantText, rec.Body)
			}
			if got := rec.Header().Get("HX-Trigger"); got != "" {
				t.Errorf("HX-Trigger = %q, want none so that the form keeps its input", got)
			}
		})
	}
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM catalog_items").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("catalog_items has %d rows after refused input, want 0", n)
	}
}

// A title is text that Gautam or an import supplied. The page must show
// it as text and must not run it as HTML.
func TestItemsPageEscapesTheTitle(t *testing.T) {
	h, _ := newTestAdmin(t)
	postForm(h, "/admin/items", url.Values{"kind": {"movie"}, "title": {`<script>alert("x")</script>`}})

	body := get(h, "/admin/items").Body.String()

	if strings.Contains(body, `<script>alert(`) {
		t.Error("the page contains the title as raw HTML")
	}
	if !strings.Contains(body, "&lt;script&gt;alert(") {
		t.Errorf("the page does not contain the escaped title")
	}
}

func TestStaticServesHtmx(t *testing.T) {
	h, _ := newTestAdmin(t)

	rec := get(h, "/admin/static/htmx.min.js")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want JavaScript", ct)
	}
	if !strings.Contains(rec.Body.String(), "htmx") {
		t.Error("the file does not look like htmx")
	}
}

func TestDatabaseFailureGivesAPlain500(t *testing.T) {
	h, pool := newTestAdmin(t)
	pool.Close()

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"page": get(h, "/admin/items"),
		"post": postForm(h, "/admin/items", url.Values{"kind": {"movie"}, "title": {"Dune"}}),
	} {
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status = %d, want 500", name, rec.Code)
		}
		if got := strings.TrimSpace(rec.Body.String()); got != "internal error" {
			t.Errorf("%s: body = %q, want only \"internal error\"", name, got)
		}
	}
}
