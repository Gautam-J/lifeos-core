package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUpdateTodoChangesOnlyTheGivenFields(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added := addTodo(t, s, "Buy a domain")

	got, err := s.UpdateTodo(ctx, added.ID, UpdateTodoParams{DueOn: ptr("2026-11-01"), Actor: SourceManual})
	if err != nil {
		t.Fatalf("UpdateTodo: %v", err)
	}
	if got.DueOn != "2026-11-01" || got.Title != "Buy a domain" || got.Project != "lifeos" || got.Status != "open" {
		t.Errorf("after a change of the date: %+v", got)
	}

	got, err = s.UpdateTodo(ctx, added.ID, UpdateTodoParams{Title: ptr("  Buy two domains "), Project: ptr("home"), Actor: SourceManual})
	if err != nil {
		t.Fatalf("UpdateTodo: %v", err)
	}
	if got.Title != "Buy two domains" || got.Project != "home" || got.DueOn != "2026-11-01" {
		t.Errorf("after a change of the title and the project: %+v", got)
	}
	if got.Source != SourceAgent || !got.Verified {
		t.Errorf("source, verified = %q, %v, want them unchanged", got.Source, got.Verified)
	}
	if stored, err := s.GetTodo(ctx, added.ID); err != nil || stored != got {
		t.Errorf("stored = %+v, %v, want %+v", stored, err, got)
	}
}

// done_at is not a field that a caller sets. It follows the status.
func TestUpdateTodoSetsDoneAtWhenTheStatusBecomesDone(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added := addTodo(t, s, "Buy a domain")
	update := func(p UpdateTodoParams) Todo {
		t.Helper()
		p.Actor = SourceManual
		todo, err := s.UpdateTodo(ctx, added.ID, p)
		if err != nil {
			t.Fatalf("UpdateTodo: %v", err)
		}
		return todo
	}

	done := update(UpdateTodoParams{Status: ptr("done")})
	if done.Status != "done" || done.DoneAt == nil || !done.DoneAt.Equal(testNow) {
		t.Fatalf("status, done_at = %q, %v, want done at %v", done.Status, done.DoneAt, testNow)
	}

	// A change of a different field of a done todo keeps the moment.
	s.now = func() time.Time { return testNow.Add(time.Hour) }
	renamed := update(UpdateTodoParams{Title: ptr("Buy the domain")})
	if renamed.DoneAt == nil || !renamed.DoneAt.Equal(testNow) {
		t.Errorf("done_at = %v after a change of the title, want %v", renamed.DoneAt, testNow)
	}

	if reopened := update(UpdateTodoParams{Status: ptr("open")}); reopened.DoneAt != nil {
		t.Errorf("done_at = %v after the todo is open again, want none", reopened.DoneAt)
	}
	if dropped := update(UpdateTodoParams{Status: ptr("dropped")}); dropped.Status != "dropped" || dropped.DoneAt != nil {
		t.Errorf("status, done_at = %q, %v, want dropped and none", dropped.Status, dropped.DoneAt)
	}
}

func TestUpdateTodoClearsAnOptionalField(t *testing.T) {
	s, _ := newTestService(t)
	added := addTodo(t, s, "Buy a domain")

	got, err := s.UpdateTodo(context.Background(), added.ID, UpdateTodoParams{Clear: []string{"due_on", "project"}, Actor: SourceManual})

	if err != nil {
		t.Fatalf("UpdateTodo: %v", err)
	}
	if got.DueOn != "" || got.Project != "" || got.Title != "Buy a domain" {
		t.Errorf("todo = %+v, want no date and no project", got)
	}
}

func TestUpdateTodoAppendsAnUpdateEventWithTheOldValue(t *testing.T) {
	s, pool := newTestService(t)
	added := addTodo(t, s, "Buy a domain")

	if _, err := s.UpdateTodo(context.Background(), added.ID, UpdateTodoParams{Status: ptr("done"), Actor: SourceManual}); err != nil {
		t.Fatalf("UpdateTodo: %v", err)
	}

	events := allEvents(t, pool)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (create, update)", len(events))
	}
	e := events[1]
	if e.Action != "update" || e.RecordType != "todos" || e.RecordID != added.ID || e.Source != "manual" {
		t.Errorf("event = %s %s %d by %s", e.Action, e.RecordType, e.RecordID, e.Source)
	}
	if e.Old["status"] != "open" || e.New["status"] != "done" || e.New["done_at"] == nil {
		t.Errorf("old, new = %v, %v", e.Old, e.New)
	}
}

func TestUpdateTodoRejectsBadInput(t *testing.T) {
	s, pool := newTestService(t)
	added := addTodo(t, s, "Buy a domain")
	tests := []struct {
		name      string
		params    UpdateTodoParams
		wantField string
	}{
		{"no field", UpdateTodoParams{}, "fields"},
		{"empty title", UpdateTodoParams{Title: ptr(" ")}, "title"},
		{"unknown status", UpdateTodoParams{Status: ptr("later")}, "status"},
		{"date that is not a date", UpdateTodoParams{DueOn: ptr("soon")}, "due_on"},
		{"empty date", UpdateTodoParams{DueOn: ptr("")}, "due_on"},
		{"empty project", UpdateTodoParams{Project: ptr(" ")}, "project"},
		{"clear of a required field", UpdateTodoParams{Clear: []string{"title"}}, "clear"},
		{"clear of done_at", UpdateTodoParams{Clear: []string{"done_at"}}, "clear"},
		{"date with a value and in clear", UpdateTodoParams{DueOn: ptr("2026-11-01"), Clear: []string{"due_on"}}, "due_on"},
		{"project with a value and in clear", UpdateTodoParams{Project: ptr("home"), Clear: []string{"project"}}, "project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.params.Actor = SourceManual
			_, err := s.UpdateTodo(context.Background(), added.ID, tt.params)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
	_, err := s.UpdateTodo(context.Background(), added.ID, UpdateTodoParams{Title: ptr("x"), Actor: "robot"})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "source" {
		t.Errorf("unknown actor: error = %v, want a ValidationError on source", err)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want 1 (the create): refused input appended an event", n)
	}
}

func TestUpdateTodoReturnsNotFound(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	deleted := addTodo(t, s, "Deleted")
	if err := s.DeleteTodo(ctx, deleted.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	for name, id := range map[string]int64{"deleted": deleted.ID, "absent": 9999} {
		_, err := s.UpdateTodo(ctx, id, UpdateTodoParams{Title: ptr("x"), Actor: SourceManual})
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", name, err)
		}
	}
}

// An agent that reads "done with the domain" in a captured page must not
// close a todo that Gautam stated. It can change a todo that it proposed.
func TestUpdateTodoRefusesAnInferredChangeToAVerifiedTodo(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	verified := addTodo(t, s, "Stated by Gautam")
	p := buyDomain()
	p.Verified = false
	proposed, err := s.AddTodo(ctx, p)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.UpdateTodo(ctx, verified.ID, UpdateTodoParams{Status: ptr("done"), Actor: SourceAgent, Inferred: true})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "dictated" {
		t.Fatalf("error = %v, want a ValidationError on dictated", err)
	}
	if got, _ := s.GetTodo(ctx, verified.ID); got.Status != "open" {
		t.Errorf("status = %q, want open", got.Status)
	}
	if n := countRows(t, pool, "events"); n != 2 {
		t.Errorf("events = %d, want 2 (the two creates)", n)
	}

	got, err := s.UpdateTodo(ctx, proposed.ID, UpdateTodoParams{DueOn: ptr("2026-11-01"), Actor: SourceAgent, Inferred: true})
	if err != nil || got.DueOn != "2026-11-01" || got.Verified {
		t.Errorf("inferred change to a proposed todo = %+v, %v; want the change, and the todo not verified", got, err)
	}
}

func TestUpdateTodoWritesNothingWhenNoValueChanges(t *testing.T) {
	s, pool := newTestService(t)
	added := addTodo(t, s, "Buy a domain")

	got, err := s.UpdateTodo(context.Background(), added.ID, UpdateTodoParams{
		Title: ptr("Buy a domain "), Status: ptr("open"), DueOn: ptr("2026-10-20"), Project: ptr("lifeos"),
		Actor: SourceAgent, Inferred: true,
	})

	if err != nil || got != added {
		t.Errorf("UpdateTodo = %+v, %v, want the stored todo and no error", got, err)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want 1 (the create)", n)
	}
}

func TestDeleteTodo(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added := addTodo(t, s, "Buy a domain")

	err := s.DeleteTodo(ctx, added.ID, DeleteParams{Actor: SourceAgent})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "confirmed" {
		t.Fatalf("delete by an agent with no confirmation: error = %v, want a ValidationError on confirmed", err)
	}

	if err := s.DeleteTodo(ctx, added.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("DeleteTodo: %v", err)
	}
	if _, err := s.GetTodo(ctx, added.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get after delete: error = %v, want ErrNotFound", err)
	}
	if todos, _ := s.ListTodos(ctx, ListTodosParams{}); len(todos) != 0 {
		t.Errorf("list after delete = %v, want it empty", todos)
	}
	if n := countRows(t, pool, "todos"); n != 1 {
		t.Errorf("todos has %d rows, want 1: a delete keeps the row", n)
	}
	events := allEvents(t, pool)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (create, delete)", len(events))
	}
	if e := events[1]; e.Action != "delete" || e.RecordType != "todos" || e.RecordID != added.ID || e.Old["title"] != "Buy a domain" || e.New != nil {
		t.Errorf("event = %+v, want a delete with the old todo", e)
	}

	if err := s.DeleteTodo(ctx, added.ID, DeleteParams{Actor: SourceManual}); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: error = %v, want ErrNotFound", err)
	}
}
