package admin_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// postFromToday sends a form as the summary page does: htmx names the part
// of the page that the response replaces.
func postFromToday(h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Target", "today")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// The address that Gautam types has no path. It must open the summary page.
func TestTheBareAddressOpensTheSummaryPage(t *testing.T) {
	h, _ := newTestAdmin(t)

	for _, path := range []string{"/", "/admin/"} {
		rec := get(h, path)
		if rec.Code != http.StatusSeeOther {
			t.Errorf("GET %s: status = %d, want 303", path, rec.Code)
		}
		if got := rec.Header().Get("Location"); got != "/admin" {
			t.Errorf("GET %s: Location = %q, want /admin", path, got)
		}
	}

	rec := get(h, "/admin")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin: status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		time.Now().In(service.IST).Format("Monday, 2 January"),
		`<a href="/admin" aria-current="page">Today</a>`,
		`hx-post="/admin/journal"`,
		`hx-post="/admin/todos"`,
		"Nothing is written for today or yesterday.",
		"No habit is active.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %q", want)
		}
	}
}

func TestSummaryPageShowsTheTwoDaysAndTheWorkThatWaits(t *testing.T) {
	h, pool := newTestAdmin(t)
	svc := service.New(pool)
	ctx := t.Context()
	yesterday := time.Now().In(service.IST).AddDate(0, 0, -1).Format(time.DateOnly)
	addEntry(t, pool, "walked to the <b>lake</b>", "")
	addEntry(t, pool, "slept late", yesterday)
	addEntry(t, pool, "an old entry", "2026-01-05")
	addStoredHabit(t, pool, "gym")
	addStoredHabit(t, pool, "read")
	logStored(t, pool, "gym", true)
	late, err := svc.AddTodo(ctx, service.AddTodoParams{
		Title: "Renew the policy", DueOn: "2026-01-05",
		Provenance: service.Provenance{Source: service.SourceManual},
	})
	if err != nil {
		t.Fatal(err)
	}
	addStoredTodo(t, pool, "Due later", true)
	done := addStoredTodo(t, pool, "Done before", true)
	if rec := postForm(h, fmt.Sprintf("/admin/todos/%d/done", done.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("mark the todo done: status = %d", rec.Code)
	}
	waits := addStored(t, pool, "Inferred", false)

	body := get(h, "/admin").Body.String()

	for _, want := range []string{
		"walked to the &lt;b&gt;lake&lt;/b&gt;",
		"slept late",
		"1 of 2 habits done",
		`name="habit" value="read"`,
		"Renew the policy",
		fmt.Sprintf(`hx-post="/admin/todos/%d/done"`, late.ID),
		"2 todos open, 1 due",
		"Due later",
		fmt.Sprintf(`<a href="/admin/items/%d/edit">item %d</a>`, waits.ID, waits.ID),
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %q", want)
		}
	}
	for _, not := range []string{"an old entry", `name="habit" value="gym"`, "<b>lake</b>", "Done before"} {
		if strings.Contains(body, not) {
			t.Errorf("the page contains %q", not)
		}
	}
}

// A form of the summary page gets the summary back, not the list of the
// page that owns the route.
func TestAWriteFromTheSummaryPageShowsTheSummaryAgain(t *testing.T) {
	h, pool := newTestAdmin(t)
	addStoredHabit(t, pool, "gym")
	todo := addStoredTodo(t, pool, "Buy a domain", true)

	clicks := []struct {
		path     string
		form     url.Values
		wantText string
		trigger  string
	}{
		{"/admin/journal", url.Values{"body": {"planned the week"}}, "planned the week", "entry-added"},
		{"/admin/todos", url.Values{"title": {"Call the bank"}, "due_on": {todayIST()}}, "Call the bank", "todo-added"},
		{"/admin/habits/log", url.Values{"habit": {"gym"}}, "1 of 1 habit done", ""},
		{fmt.Sprintf("/admin/todos/%d/done", todo.ID), nil, "1 todo open, 1 due", ""},
	}
	for _, click := range clicks {
		rec := postFromToday(h, click.path, click.form)
		body := strings.TrimSpace(rec.Body.String())
		if rec.Code != http.StatusOK || !strings.HasPrefix(body, `<div id="today">`) {
			t.Errorf("POST %s: status = %d, and the response is not the summary:\n%.200s", click.path, rec.Code, body)
		}
		if !strings.Contains(body, click.wantText) {
			t.Errorf("POST %s: the summary does not contain %q", click.path, click.wantText)
		}
		if got := rec.Header().Get("HX-Trigger"); got != click.trigger {
			t.Errorf("POST %s: HX-Trigger = %q, want %q", click.path, got, click.trigger)
		}
	}
}

func TestSummaryPageShowsWhyTheServiceRefusedTheInput(t *testing.T) {
	h, _ := newTestAdmin(t)

	rec := postFromToday(h, "/admin/todos", url.Values{"title": {" "}})

	body := strings.TrimSpace(rec.Body.String())
	if !strings.HasPrefix(body, `<div id="today">`) || !strings.Contains(body, `role="alert">title: must not be empty`) {
		t.Errorf("the summary does not show the reason:\n%.300s", body)
	}
	if got := rec.Header().Get("HX-Trigger"); got != "" {
		t.Errorf("HX-Trigger = %q, want none so that the form keeps its input", got)
	}
}

// htmx does not show the body of a 5xx response, and the summary page
// must keep its own part of the page for the next click.
func TestSummaryPageShowsADatabaseFailureInItsOwnPart(t *testing.T) {
	h, pool := newTestAdmin(t)
	pool.Close()

	if rec := get(h, "/admin"); rec.Code != http.StatusInternalServerError || strings.TrimSpace(rec.Body.String()) != "internal error" {
		t.Errorf("page: status = %d, body = %q; want 500 and only \"internal error\"", rec.Code, rec.Body)
	}
	clicks := map[string]url.Values{
		"/admin/journal":      {"body": {"a day"}},
		"/admin/todos":        {"title": {"Buy a domain"}},
		"/admin/todos/1/done": nil,
		"/admin/habits/log":   {"habit": {"gym"}},
	}
	for path, form := range clicks {
		rec := postFromToday(h, path, form)
		if rec.Code != http.StatusOK {
			t.Errorf("POST %s: status = %d, want 200: htmx does not show a 5xx response", path, rec.Code)
		}
		page := strings.TrimSpace(rec.Body.String())
		if !strings.HasPrefix(page, `<div id="today"><p class="error" role="alert">`) || !strings.Contains(page, "internal error") {
			t.Errorf("POST %s: the response is not the summary with the error:\n%s", path, page)
		}
	}
}

// Cloudflare and the browser keep a static file. The address of the
// stylesheet has the hash of its content, so a deploy shows the new one.
func TestPagesLinkTheStylesheetByItsVersion(t *testing.T) {
	h, _ := newTestAdmin(t)

	page := get(h, "/admin/items").Body.String()
	link := regexp.MustCompile(`href="(/admin/static/app\.css\?v=[0-9a-f]{12})"`).FindStringSubmatch(page)
	if link == nil {
		t.Fatal("the page does not link the stylesheet with a version")
	}
	rec := get(h, link[1])
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "text/css") {
		t.Errorf("stylesheet: status = %d, Content-Type = %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q", got)
	}
	if font := get(h, "/admin/static/fonts/inter.woff2"); font.Code != http.StatusOK || font.Header().Get("Content-Type") != "font/woff2" {
		t.Errorf("font: status = %d, Content-Type = %q", font.Code, font.Header().Get("Content-Type"))
	}
	// A file that has no version in its address must not stay for a year.
	if got := get(h, "/admin/static/app.css").Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control without a version = %q, want no-cache", got)
	}
}

func TestTheMenuMarksThePageThatIsOpen(t *testing.T) {
	h, _ := newTestAdmin(t)

	for path, name := range map[string]string{
		"/admin/journal": "Journal",
		"/admin/todos":   "Todos",
		"/admin/habits":  "Habits",
		"/admin/items":   "Items",
		"/admin/sources": "Sources",
	} {
		want := fmt.Sprintf(`<a href="%s" aria-current="page">%s</a>`, path, name)
		page := get(h, path).Body.String()
		if !strings.Contains(page, want) || strings.Count(page, `aria-current="page"`) != 1 {
			t.Errorf("GET %s: the menu does not mark only %s", path, name)
		}
	}
}

// #40: the review page, its count and its approval route are gone, and
// the menu does not link to them.
func TestTheReviewPageIsGone(t *testing.T) {
	h, _ := newTestAdmin(t)

	for _, path := range []string{"/admin/review", "/admin/review/count"} {
		if code := get(h, path).Code; code != http.StatusNotFound {
			t.Errorf("GET %s: status %d, want 404", path, code)
		}
	}
	if page := get(h, "/admin").Body.String(); strings.Contains(page, "/admin/review") {
		t.Error("the summary page links to /admin/review")
	}
}
