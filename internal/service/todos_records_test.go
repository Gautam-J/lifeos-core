package service

import (
	"context"
	"errors"
	"testing"
)

func TestRecordFunctionsWorkOnATodo(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added := addTodo(t, s, "Buy a domain")

	got, err := s.Get(ctx, "todo", added.ID)
	if todo, ok := got.(Todo); err != nil || !ok || todo != added {
		t.Fatalf("Get = %#v, %v, want the todo", got, err)
	}

	got, err = s.Update(ctx, "todo", added.ID, UpdateParams{
		Fields: map[string]any{"status": "done"}, Clear: []string{"due_on"}, Actor: SourceManual,
	})
	if todo, ok := got.(Todo); err != nil || !ok || todo.Status != "done" || todo.DoneAt == nil || todo.DueOn != "" || todo.Project != "lifeos" {
		t.Fatalf("Update = %#v, %v, want the todo done, with no date", got, err)
	}

	// done_at follows the status. A caller must not set it.
	refused := map[string]map[string]any{
		"done_at":  {"done_at": "2026-10-11T00:00:00Z"},
		"verified": {"verified": true},
		"source":   {"source": "manual"},
		"status":   {"status": 1},
		"project":  {"project": nil},
	}
	for field, fields := range refused {
		_, err := s.Update(ctx, "todo", added.ID, UpdateParams{Fields: fields, Actor: SourceManual})
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != field {
			t.Errorf("%s: error = %v, want a ValidationError on %s", field, err, field)
		}
	}

	if err := s.Delete(ctx, "todo", added.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "todo", added.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: error = %v, want ErrNotFound", err)
	}
}

// addProposedTodo stores a todo as an agent adds it from a journal entry.
func addProposedTodo(t *testing.T, s *Service, p AddTodoParams) Todo {
	t.Helper()
	p.Provenance = Provenance{Source: SourceAgent}
	todo, err := s.AddTodo(context.Background(), p)
	if err != nil {
		t.Fatalf("AddTodo(%q): %v", p.Title, err)
	}
	return todo
}
