package admin_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// addStoredTodo stores one todo through the service and returns it.
// verified false gives a todo as an agent proposes it.
func addStoredTodo(t *testing.T, pool *pgxpool.Pool, title string, verified bool) service.Todo {
	t.Helper()
	source := service.SourceManual
	if !verified {
		source = service.SourceAgent
	}
	todo, err := service.New(pool).AddTodo(t.Context(), service.AddTodoParams{
		Title:      title,
		DueOn:      "2026-10-20",
		Project:    "lifeos",
		Provenance: service.Provenance{Source: source, Verified: verified},
	})
	if err != nil {
		t.Fatalf("add %q: %v", title, err)
	}
	return todo
}

func TestTodosPageShowsTheFormAndAnEmptyList(t *testing.T) {
	h, _ := newTestAdmin(t)

	rec := get(h, "/admin/todos")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`hx-post="/admin/todos"`, `name="title"`, `name="project"`, `name="due_on"`, "No todos yet.", `href="/admin/todos"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %q", want)
		}
	}
}

func TestAddTodoStoresTheRowAndShowsIt(t *testing.T) {
	h, pool := newTestAdmin(t)

	rec := postForm(h, "/admin/todos", url.Values{"title": {"Buy a <b>domain</b>"}, "project": {"lifeos"}, "due_on": {""}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("HX-Trigger"); got != "todo-added" {
		t.Errorf("HX-Trigger = %q, want todo-added so that the form resets", got)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(body), `<div id="list">`) {
		t.Errorf("the response is not the #list fragment:\n%s", body)
	}
	for _, want := range []string{"Buy a &lt;b&gt;domain&lt;/b&gt;", "lifeos", "open", "manual", "yes"} {
		if !strings.Contains(body, want) {
			t.Errorf("the list does not contain %q", want)
		}
	}
	if strings.Contains(body, "<b>domain</b>") {
		t.Error("the list contains the title as raw HTML")
	}

	todos, err := service.New(pool).ListTodos(t.Context(), service.ListTodosParams{})
	if err != nil || len(todos) != 1 {
		t.Fatalf("todos = %v, %v, want one todo", todos, err)
	}
	if got := todos[0]; got.Source != service.SourceManual || !got.Verified || got.Status != "open" || got.DueOn != "" {
		t.Errorf("todo = %+v, want manual, verified, open, no date", got)
	}
}

func TestAddTodoShowsWhyTheServiceRefusedTheInput(t *testing.T) {
	h, pool := newTestAdmin(t)

	rec := postForm(h, "/admin/todos", url.Values{"title": {" "}})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200: htmx does not show a 4xx response", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "title: must not be empty") {
		t.Errorf("the response does not show the reason:\n%s", rec.Body)
	}
	if got := rec.Header().Get("HX-Trigger"); got != "" {
		t.Errorf("HX-Trigger = %q, want none so that the form keeps its input", got)
	}
	if n := countEvents(t, pool); n != 0 {
		t.Errorf("events = %d after refused input, want 0", n)
	}
}

// An open todo has a Done button. A todo that is done has none.
func TestMarkTodoDone(t *testing.T) {
	h, pool := newTestAdmin(t)
	todo := addStoredTodo(t, pool, "Buy a domain", true)
	path := fmt.Sprintf("/admin/todos/%d/done", todo.ID)
	if page := get(h, "/admin/todos").Body.String(); !strings.Contains(page, `hx-post="`+path+`"`) {
		t.Fatalf("the page has no Done button for the open todo:\n%s", page)
	}

	rec := postForm(h, path, nil)

	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(body), `<div id="list">`) {
		t.Fatalf("status = %d, want 200 with the #list fragment\n%s", rec.Code, body)
	}
	if strings.Contains(body, `hx-post="`+path+`"`) {
		t.Errorf("the list still has a Done button for the todo:\n%s", body)
	}
	got, err := service.New(pool).GetTodo(t.Context(), todo.ID)
	if err != nil || got.Status != "done" || got.DoneAt == nil || got.Title != "Buy a domain" || got.Project != "lifeos" || got.DueOn != "2026-10-20" {
		t.Errorf("todo = %+v, %v, want it done with its other fields unchanged", got, err)
	}

	// A second click, for example from a page that was open in two tabs.
	postForm(h, path, nil)
	if n := countEvents(t, pool); n != 2 {
		t.Errorf("events = %d, want 2 (create, one update)", n)
	}
	gone := postForm(h, "/admin/todos/9999/done", nil)
	if gone.Code != http.StatusOK || !strings.Contains(gone.Body.String(), "Todo 9999 is not in the list") {
		t.Errorf("unknown ID: status = %d, want 200 with a message\n%s", gone.Code, gone.Body)
	}
}

func TestEditTodoPageShowsTheStoredValuesAsText(t *testing.T) {
	h, pool := newTestAdmin(t)
	todo := addStoredTodo(t, pool, `Buy "a" <b>domain`, true)

	rec := get(h, fmt.Sprintf("/admin/todos/%d/edit", todo.ID))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		fmt.Sprintf(`action="/admin/todos/%d"`, todo.ID),
		`value="Buy &#34;a&#34; &lt;b&gt;domain"`,
		`<option value="open" selected>`,
		`value="lifeos"`,
		`value="2026-10-20"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %s", want)
		}
	}
	for _, path := range []string{"/admin/todos/9999/edit", "/admin/todos/abc/edit"} {
		if rec := get(h, path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, rec.Code)
		}
	}
}

func TestSaveTodoChangesTheTodoAndGoesBackToTheList(t *testing.T) {
	h, pool := newTestAdmin(t)
	todo := addStoredTodo(t, pool, "Buy a domain", false)

	// An empty project and an empty date remove the stored values.
	rec := postForm(h, fmt.Sprintf("/admin/todos/%d", todo.ID), url.Values{
		"title": {"Buy two domains"}, "status": {"dropped"}, "project": {""}, "due_on": {""},
	})

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/todos" {
		t.Fatalf("status = %d, Location = %q; want 303 to /admin/todos\n%s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	got, err := service.New(pool).GetTodo(t.Context(), todo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Buy two domains" || got.Status != "dropped" || got.Project != "" || got.DueOn != "" {
		t.Errorf("todo = %+v", got)
	}
	// An edit is not an approval: the todo stays in the review queue.
	if got.Verified || got.Source != service.SourceAgent {
		t.Errorf("verified, source = %v, %q; want no change", got.Verified, got.Source)
	}

	for _, path := range []string{"/admin/todos/9999", "/admin/todos/abc"} {
		if rec := postForm(h, path, url.Values{"title": {"x"}, "status": {"open"}}); rec.Code != http.StatusNotFound {
			t.Errorf("POST %s: status = %d, want 404", path, rec.Code)
		}
	}
}

func TestSaveTodoShowsWhyTheServiceRefusedTheInput(t *testing.T) {
	h, pool := newTestAdmin(t)
	todo := addStoredTodo(t, pool, "Buy a domain", true)

	rec := postForm(h, fmt.Sprintf("/admin/todos/%d", todo.ID), url.Values{
		"title": {"Typed title"}, "status": {"later"}, "project": {"typed project"}, "due_on": {"2026-11-01"},
	})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 with the form", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "status: must be one of open, done, dropped") {
		t.Errorf("the page does not show the reason:\n%s", body)
	}
	for _, want := range []string{`value="Typed title"`, `value="typed project"`, `value="2026-11-01"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the form lost the typed input %s", want)
		}
	}
	if got, _ := service.New(pool).GetTodo(t.Context(), todo.ID); got.Title != "Buy a domain" {
		t.Errorf("refused input changed the todo: %+v", got)
	}
}

// A click on Save with no change is not a write.
func TestSaveTodoWithNoChangeWritesNoEvent(t *testing.T) {
	h, pool := newTestAdmin(t)
	todo := addStoredTodo(t, pool, "Buy a domain", true)

	rec := postForm(h, fmt.Sprintf("/admin/todos/%d", todo.ID), url.Values{
		"title": {"Buy a domain"}, "status": {"open"}, "project": {"lifeos"}, "due_on": {"2026-10-20"},
	})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303\n%s", rec.Code, rec.Body)
	}
	if n := countEvents(t, pool); n != 1 {
		t.Errorf("events = %d, want 1 (the create)", n)
	}
}

func TestDeleteTodoRemovesTheTodoFromTheList(t *testing.T) {
	h, pool := newTestAdmin(t)
	addStoredTodo(t, pool, "Kept", true)
	gone := addStoredTodo(t, pool, "Gone", true)
	path := fmt.Sprintf("/admin/todos/%d/delete", gone.ID)
	if page := get(h, "/admin/todos").Body.String(); !strings.Contains(page, `hx-confirm="Delete Gone?"`) {
		t.Errorf("the Delete button does not ask for a confirmation:\n%s", page)
	}

	rec := postForm(h, path, nil)

	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(body), `<div id="list">`) {
		t.Fatalf("status = %d, want 200 with the #list fragment\n%s", rec.Code, body)
	}
	if strings.Contains(body, "Gone") || !strings.Contains(body, "Kept") {
		t.Errorf("the list must show Kept and not Gone:\n%s", body)
	}

	again := postForm(h, path, nil)
	if again.Code != http.StatusOK || !strings.Contains(again.Body.String(), fmt.Sprintf("Todo %d is not in the list", gone.ID)) {
		t.Errorf("second delete: status = %d, want 200 with a message\n%s", again.Code, again.Body)
	}
	if n := countEvents(t, pool); n != 3 {
		t.Errorf("events = %d, want 3 (two creates, one delete)", n)
	}
}

func TestDatabaseFailureOnTheTodoPages(t *testing.T) {
	h, pool := newTestAdmin(t)
	pool.Close()

	if rec := get(h, "/admin/todos"); rec.Code != http.StatusInternalServerError || strings.TrimSpace(rec.Body.String()) != "internal error" {
		t.Errorf("page: status = %d, body = %q; want 500 and only \"internal error\"", rec.Code, rec.Body)
	}
	clicks := map[string]url.Values{
		"/admin/todos":          {"title": {"x"}},
		"/admin/todos/1/done":   nil,
		"/admin/todos/1/delete": nil,
	}
	for path, form := range clicks {
		rec := postForm(h, path, form)
		if rec.Code != http.StatusOK {
			t.Errorf("POST %s: status = %d, want 200: htmx does not show a 5xx response", path, rec.Code)
		}
		page := strings.TrimSpace(rec.Body.String())
		if !strings.HasPrefix(page, `<div id="list"><p class="error" role="alert">`) || !strings.Contains(page, "internal error") {
			t.Errorf("POST %s: the response is not the list with the error:\n%s", path, page)
		}
	}
}

// A todo that an agent proposed waits in the review queue. Its Edit link
// must go to the todo.
func TestReviewPageLinksATodoToItsEditPage(t *testing.T) {
	h, pool := newTestAdmin(t)
	todo := addStoredTodo(t, pool, "Renew the insurance", false)

	body := get(h, "/admin/review").Body.String()

	for _, want := range []string{
		"todo: Renew the insurance, project lifeos, due 2026-10-20",
		`name="type" value="todo"`,
		fmt.Sprintf(`href="/admin/todos/%d/edit"`, todo.ID),
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %s", want)
		}
	}
	if strings.Contains(body, fmt.Sprintf(`href="/admin/items/%d/edit"`, todo.ID)) {
		t.Error("the Edit link of the todo goes to the item with the same ID")
	}

	rec := postForm(h, "/admin/review/verify", url.Values{"type": {"todo"}, "id": {fmt.Sprint(todo.ID)}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "No record waits for review.") {
		t.Errorf("after the approval the queue must be empty:\n%s", rec.Body)
	}
}
