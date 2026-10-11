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

// addProposedTodo stores a todo as an agent proposes it from a source: not
// verified.
func addProposedTodo(t *testing.T, s *Service, p AddTodoParams) Todo {
	t.Helper()
	p.Provenance = Provenance{Source: SourceAgent}
	todo, err := s.AddTodo(context.Background(), p)
	if err != nil {
		t.Fatalf("AddTodo(%q): %v", p.Title, err)
	}
	return todo
}

func TestReviewQueueListsAProposedTodo(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	item := addUnverified(t, s, "Dune")
	full := addProposedTodo(t, s, AddTodoParams{Title: "Renew the insurance", DueOn: "2026-12-01", Project: "home"})
	bare := addProposedTodo(t, s, AddTodoParams{Title: "Call the bank"})
	addTodo(t, s, "Stated by Gautam") // verified

	got, err := s.ReviewQueue(ctx, 0)
	if err != nil {
		t.Fatalf("ReviewQueue: %v", err)
	}
	if len(got) != 3 || got[0].ID != item.ID || got[0].Type != "item" {
		t.Fatalf("queue = %+v, want the item, then the two todos", got)
	}
	if got[1].Type != "todo" || got[1].ID != full.ID || got[1].Summary != "todo: Renew the insurance, project home, due 2026-12-01" {
		t.Errorf("entry 1 = %+v", got[1])
	}
	if got[2].Type != "todo" || got[2].ID != bare.ID || got[2].Summary != "todo: Call the bank" || got[2].Source != SourceAgent {
		t.Errorf("entry 2 = %+v", got[2])
	}
}

func TestVerifyApprovesATodo(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	todo := addProposedTodo(t, s, AddTodoParams{Title: "Call the bank"})

	n, err := s.Verify(ctx, []RecordRef{{"todo", todo.ID}}, SourceManual)
	if err != nil || n != 1 {
		t.Fatalf("Verify = %d, %v, want 1", n, err)
	}
	if got, _ := s.GetTodo(ctx, todo.ID); !got.Verified {
		t.Error("the todo is not verified")
	}
	events := allEvents(t, pool)
	e := events[len(events)-1]
	if e.Action != "verify" || e.RecordType != "todos" || e.RecordID != todo.ID || e.Old["verified"] != false || e.New["verified"] != true {
		t.Errorf("event = %+v, want a verify of the todo", e)
	}
	if queue, _ := s.ReviewQueue(ctx, 0); len(queue) != 0 {
		t.Errorf("queue = %+v, want it empty", queue)
	}
}
