package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// buyDomain is a valid input. Tests copy it and change one field.
func buyDomain() AddTodoParams {
	return AddTodoParams{
		Title:      "Buy a Cloudflare domain",
		DueOn:      "2026-10-20",
		Project:    "lifeos",
		Provenance: Provenance{Source: SourceAgent, Verified: true},
	}
}

func addTodo(t *testing.T, s *Service, title string) Todo {
	t.Helper()
	p := buyDomain()
	p.Title = title
	todo, err := s.AddTodo(context.Background(), p)
	if err != nil {
		t.Fatalf("AddTodo(%q): %v", title, err)
	}
	return todo
}

func TestAddTodoStoresAnOpenTodo(t *testing.T) {
	s, pool := newTestService(t)

	got, err := s.AddTodo(context.Background(), buyDomain())
	if err != nil {
		t.Fatalf("AddTodo: %v", err)
	}

	if got.ID == 0 {
		t.Error("AddTodo returned ID 0")
	}
	if got.Title != "Buy a Cloudflare domain" || got.Status != "open" || got.DueOn != "2026-10-20" || got.Project != "lifeos" {
		t.Errorf("todo = %+v", got)
	}
	if got.DoneAt != nil {
		t.Errorf("done_at = %v, want none for an open todo", got.DoneAt)
	}
	if got.Source != SourceAgent || !got.Verified {
		t.Errorf("source, verified = %q, %v", got.Source, got.Verified)
	}
	events := allEvents(t, pool)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Action != "create" || e.RecordType != "todos" || e.RecordID != got.ID || e.Source != "agent" || e.Old != nil {
		t.Errorf("event = %+v, want a create of the todo by agent", e)
	}
	if e.New["title"] != "Buy a Cloudflare domain" || e.New["status"] != "open" {
		t.Errorf("new value = %v", e.New)
	}
}

// "todo - buy cloudflare domain" has no date and can have no project. A
// date in the past is valid too: it is a todo that is late.
func TestAddTodoAcceptsTheMinimumInputAndAnyDay(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	got, err := s.AddTodo(ctx, AddTodoParams{Title: "  set up the tunnel  ", Provenance: Provenance{Source: SourceAgent}})
	if err != nil {
		t.Fatalf("AddTodo: %v", err)
	}
	if got.Title != "set up the tunnel" || got.DueOn != "" || got.Project != "" || got.Verified {
		t.Errorf("todo = %+v, want the trimmed title, no date, no project, not verified", got)
	}

	for _, day := range []string{"2026-01-01", "2027-06-30"} {
		p := buyDomain()
		p.DueOn = day
		if got, err := s.AddTodo(ctx, p); err != nil || got.DueOn != day {
			t.Errorf("due_on %s: todo = %+v, error = %v", day, got, err)
		}
	}
}

func TestAddTodoRejectsBadInput(t *testing.T) {
	tests := []struct {
		name      string
		change    func(*AddTodoParams)
		wantField string
	}{
		{"empty title", func(p *AddTodoParams) { p.Title = "" }, "title"},
		{"title of spaces", func(p *AddTodoParams) { p.Title = " \t\n" }, "title"},
		{"title too long", func(p *AddTodoParams) { p.Title = strings.Repeat("a", 301) }, "title"},
		{"title with a NUL character", func(p *AddTodoParams) { p.Title = "a\x00b" }, "title"},
		{"date that is not a date", func(p *AddTodoParams) { p.DueOn = "next week" }, "due_on"},
		{"day that does not exist", func(p *AddTodoParams) { p.DueOn = "2026-02-30" }, "due_on"},
		{"project too long", func(p *AddTodoParams) { p.Project = strings.Repeat("a", 101) }, "project"},
		{"project with a NUL character", func(p *AddTodoParams) { p.Project = "a\x00b" }, "project"},
		{"unknown source", func(p *AddTodoParams) { p.Source = "robot" }, "source"},
	}
	s, pool := newTestService(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := buyDomain()
			tt.change(&p)
			_, err := s.AddTodo(context.Background(), p)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
	if n := countRows(t, pool, "todos"); n != 0 {
		t.Errorf("todos has %d rows after refused input, want 0", n)
	}
}

func TestAddTodoStoresNothingWhenTheEventFails(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "DROP TABLE events"); err != nil {
		t.Fatal(err)
	}

	_, err := s.AddTodo(ctx, buyDomain())

	var invalid *ValidationError
	if err == nil || errors.As(err, &invalid) {
		t.Fatalf("error = %v, want a database failure", err)
	}
	if n := countRows(t, pool, "todos"); n != 0 {
		t.Errorf("todos has %d rows, want 0: the todo must roll back with its event", n)
	}
}

func TestGetTodo(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added := addTodo(t, s, "Buy a domain")

	got, err := s.GetTodo(ctx, added.ID)
	if err != nil || got != added {
		t.Errorf("GetTodo = %+v, %v, want %+v", got, err, added)
	}
	if _, err := s.GetTodo(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Errorf("absent todo: error = %v, want ErrNotFound", err)
	}
}

func todoTitles(todos []Todo) []string {
	out := make([]string, len(todos))
	for i, todo := range todos {
		out[i] = todo.Title
	}
	return out
}

// setStatus changes the status of a todo with SQL, because this file is
// about the reads. UpdateTodo has its own tests.
func setStatus(t *testing.T, s *Service, id int64, status string) {
	t.Helper()
	if _, err := s.pool.Exec(context.Background(), "UPDATE todos SET status = $2 WHERE id = $1", id, status); err != nil {
		t.Fatal(err)
	}
}

// The open todos are what Gautam looks for. They come first, so that the
// first page of a long list does not hide them behind todos that are done.
func TestListTodosReturnsTheOpenTodosFirst(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	addTodo(t, s, "first open")
	done := addTodo(t, s, "done")
	dropped := addTodo(t, s, "dropped")
	addTodo(t, s, "second open")
	setStatus(t, s, done.ID, "done")
	setStatus(t, s, dropped.ID, "dropped")

	all, err := s.ListTodos(ctx, ListTodosParams{})
	if err != nil {
		t.Fatalf("ListTodos: %v", err)
	}
	if want := []string{"second open", "first open", "dropped", "done"}; !equal(todoTitles(all), want) {
		t.Errorf("titles = %v, want %v", todoTitles(all), want)
	}

	open, err := s.ListTodos(ctx, ListTodosParams{Status: "open", Limit: 1})
	if err != nil {
		t.Fatalf("ListTodos: %v", err)
	}
	if want := []string{"second open"}; !equal(todoTitles(open), want) {
		t.Errorf("open titles with limit 1 = %v, want %v", todoTitles(open), want)
	}

	finished, err := s.ListTodos(ctx, ListTodosParams{Status: "done"})
	if err != nil || !equal(todoTitles(finished), []string{"done"}) {
		t.Errorf("done titles = %v, %v, want [done]", todoTitles(finished), err)
	}
}

func TestListTodosReturnsAnEmptyListNotNil(t *testing.T) {
	s, _ := newTestService(t)

	todos, err := s.ListTodos(context.Background(), ListTodosParams{})
	if err != nil {
		t.Fatalf("ListTodos: %v", err)
	}
	if todos == nil || len(todos) != 0 {
		t.Errorf("todos = %#v, want an empty non-nil slice so JSON shows []", todos)
	}
}

func TestListTodosRejectsBadInput(t *testing.T) {
	s, _ := newTestService(t)
	tests := []struct {
		name      string
		params    ListTodosParams
		wantField string
	}{
		{"unknown status", ListTodosParams{Status: "later"}, "status"},
		{"limit above the maximum", ListTodosParams{Limit: 201}, "limit"},
		{"negative limit", ListTodosParams{Limit: -1}, "limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.ListTodos(context.Background(), tt.params)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
}
