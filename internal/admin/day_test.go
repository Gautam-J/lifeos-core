package admin_test

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// #50: the browser draws a date input in the language of the system, so a
// MacBook with a US setting shows MM-DD-YYYY. A field of a day is a text
// field that reads DD-MM-YYYY, the same in each browser.
func TestNoPageHasANativeDateInput(t *testing.T) {
	h, pool := newTestAdmin(t)
	todo := addStoredTodo(t, pool, "Buy a domain", true)
	item := addStored(t, pool, "Dune", true)
	entry := addEntry(t, pool, "a day", "2026-01-05")
	paths := []string{
		"/admin", "/admin/todos", "/admin/journal", "/admin/items",
		fmt.Sprintf("/admin/todos/%d/edit", todo.ID),
		fmt.Sprintf("/admin/items/%d/edit", item.ID),
		fmt.Sprintf("/admin/journal/%d/edit", entry.ID),
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			body := get(h, path).Body.String()
			if strings.Contains(body, `type="date"`) {
				t.Error("the page has an input of type date, which the browser draws in its own format")
			}
			if path != "/admin" && !strings.Contains(body, `placeholder="DD-MM-YYYY"`) {
				t.Error("the page has no field that asks for DD-MM-YYYY")
			}
		})
	}
}

func TestEditPagesShowADayAsDDMMYYYY(t *testing.T) {
	h, pool := newTestAdmin(t)
	todo := addStoredTodo(t, pool, "Buy a domain", true)
	item := addStored(t, pool, "Dune", true)
	entry := addEntry(t, pool, "a day", "2026-01-05")
	tests := []struct{ path, want string }{
		{fmt.Sprintf("/admin/todos/%d/edit", todo.ID), `value="20-10-2026"`},
		{fmt.Sprintf("/admin/items/%d/edit", item.ID), `value="01-03-2024"`},
		{fmt.Sprintf("/admin/journal/%d/edit", entry.ID), `value="05-01-2026"`},
	}
	for _, tt := range tests {
		if body := get(h, tt.path).Body.String(); !strings.Contains(body, tt.want) {
			t.Errorf("GET %s does not contain %s", tt.path, tt.want)
		}
	}
}

func TestFormsReadADayAsDDMMYYYY(t *testing.T) {
	h, pool := newTestAdmin(t)
	svc := service.New(pool)

	postForm(h, "/admin/todos", url.Values{"title": {"New todo"}, "due_on": {"16-11-2026"}})
	postForm(h, "/admin/journal", url.Values{"body": {"new entry"}, "entry_date": {"03-01-2026"}})
	postForm(h, "/admin/items", url.Values{"kind": {"movie"}, "title": {"New item"}, "occurred_on": {"02-01-2026"}})
	var due, entryDate, occurred string
	for query, dest := range map[string]*string{
		"SELECT due_on::text FROM todos WHERE title = 'New todo'":              &due,
		"SELECT entry_date::text FROM journal_entries WHERE body = 'new entry'": &entryDate,
		"SELECT occurred_on::text FROM catalog_items WHERE title = 'New item'": &occurred,
	} {
		if err := pool.QueryRow(t.Context(), query).Scan(dest); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	if due != "2026-11-16" || entryDate != "2026-01-03" || occurred != "2026-01-02" {
		t.Errorf("stored days = %s, %s, %s; want 2026-11-16, 2026-01-03, 2026-01-02", due, entryDate, occurred)
	}

	todo := addStoredTodo(t, pool, "Buy a domain", true)
	rec := postForm(h, fmt.Sprintf("/admin/todos/%d", todo.ID), url.Values{"title": {"Buy a domain"}, "status": {"open"}, "due_on": {"21-12-2026"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save todo: status = %d\n%s", rec.Code, rec.Body)
	}
	if got, _ := svc.GetTodo(t.Context(), todo.ID); got.DueOn != "2026-12-21" {
		t.Errorf("saved due_on = %q, want 2026-12-21", got.DueOn)
	}
	entry := addEntry(t, pool, "a day", "2026-01-05")
	postForm(h, fmt.Sprintf("/admin/journal/%d", entry.ID), url.Values{"body": {"a day"}, "entry_date": {"04-01-2026"}})
	if got, _ := svc.GetJournalEntry(t.Context(), entry.ID); got.EntryDate != "2026-01-04" {
		t.Errorf("saved entry_date = %q, want 2026-01-04", got.EntryDate)
	}
	item := addStored(t, pool, "Dune", true)
	postForm(h, fmt.Sprintf("/admin/items/%d", item.ID), url.Values{"kind": {"movie"}, "title": {"Dune"}, "occurred_on": {"05-03-2024"}})
	if got, _ := svc.GetItem(t.Context(), item.ID); got.OccurredOn != "2024-03-05" {
		t.Errorf("saved occurred_on = %q, want 2024-03-05", got.OccurredOn)
	}
}

// A day in YYYY-MM-DD still works: a script or an old form can send it.
func TestFormsStillReadAnISODay(t *testing.T) {
	h, pool := newTestAdmin(t)

	postForm(h, "/admin/todos", url.Values{"title": {"New todo"}, "due_on": {"2026-11-16"}})

	var due string
	if err := pool.QueryRow(t.Context(), "SELECT due_on::text FROM todos").Scan(&due); err != nil || due != "2026-11-16" {
		t.Errorf("due_on = %q, %v; want 2026-11-16", due, err)
	}
}

func TestFormsRefuseADayThatDoesNotExistAndNameTheFormat(t *testing.T) {
	h, pool := newTestAdmin(t)
	todo := addStoredTodo(t, pool, "Buy a domain", true)

	for name, rec := range map[string]string{
		"add todo":  postForm(h, "/admin/todos", url.Values{"title": {"x"}, "due_on": {"31-02-2026"}}).Body.String(),
		"save todo": postForm(h, fmt.Sprintf("/admin/todos/%d", todo.ID), url.Values{"title": {"x"}, "status": {"open"}, "due_on": {"31-02-2026"}}).Body.String(),
	} {
		if !strings.Contains(rec, "DD-MM-YYYY") || strings.Contains(rec, "must be a date as YYYY-MM-DD") {
			t.Errorf("%s: the error does not name DD-MM-YYYY:\n%s", name, rec)
		}
	}
	if n := countEvents(t, pool); n != 1 {
		t.Errorf("events = %d, want 1 (the todo of the setup)", n)
	}
}

// The calendar button is an aid. Its script is a static file that the
// pages load with a hashed address.
func TestPagesLoadTheScriptOfTheCalendarButton(t *testing.T) {
	h, _ := newTestAdmin(t)

	page := get(h, "/admin/todos").Body.String()

	m := regexp.MustCompile(`<script src="(/admin/static/day\.js\?v=[0-9a-f]{12})"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatal("the page does not load day.js with a version")
	}
	if rec := get(h, m[1]); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "showPicker") {
		t.Errorf("day.js: status = %d, or it does not open the picker", rec.Code)
	}
	if !strings.Contains(page, `class="day-pick"`) {
		t.Error("the page has no calendar button")
	}
}
