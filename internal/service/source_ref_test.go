package service

import (
	"context"
	"errors"
	"testing"
)

// addWithSourceRef calls each function that takes a source_ref. It returns
// the name of each function with the source_ref of the stored row, or the
// error.
func addWithSourceRef(s *Service, ref int64) map[string]func() (*int64, error) {
	ctx := context.Background()
	p := Provenance{Source: SourceAgent, SourceRef: &ref}
	return map[string]func() (*int64, error){
		"AddItem": func() (*int64, error) {
			item, err := s.AddItem(ctx, AddItemParams{Kind: "book", Title: "Dune", Provenance: p})
			return item.SourceRef, err
		},
		"AddJournalEntry": func() (*int64, error) {
			entry, err := s.AddJournalEntry(ctx, AddJournalEntryParams{Body: "a day", Provenance: p})
			return entry.SourceRef, err
		},
		"AddTodo": func() (*int64, error) {
			todo, err := s.AddTodo(ctx, AddTodoParams{Title: "Renew the policy", Provenance: p})
			return todo.SourceRef, err
		},
		"AddHabit": func() (*int64, error) {
			habit, err := s.AddHabit(ctx, AddHabitParams{Name: "read", Provenance: p})
			return habit.SourceRef, err
		},
		"LogHabit": func() (*int64, error) {
			log, err := s.LogHabit(ctx, LogHabitParams{Habit: "gym", Provenance: p})
			return log.SourceRef, err
		},
	}
}

func TestARecordCanNameTheSourceThatItCameFrom(t *testing.T) {
	s, _ := newTestService(t)
	addHabit(t, s, "gym")
	source := captureText(t, s, "Read Dune. Renew the policy. Went to the gym.")

	for name, add := range addWithSourceRef(s, source.ID) {
		ref, err := add()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if ref == nil || *ref != source.ID {
			t.Errorf("%s: source_ref = %v, want %d", name, ref, source.ID)
		}
	}
}

func TestARecordCannotNameASourceThatDoesNotExist(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	addHabit(t, s, "gym")
	deleted := captureText(t, s, "a note")
	if err := s.DeleteCapture(ctx, deleted.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("DeleteCapture: %v", err)
	}
	before := countRows(t, pool, "events")

	for what, ref := range map[string]int64{"an unknown source": 9999, "a deleted source": deleted.ID} {
		for name, add := range addWithSourceRef(s, ref) {
			_, err := add()
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != "source_ref" {
				t.Errorf("%s with %s: error = %v, want a ValidationError on source_ref", name, what, err)
			}
		}
	}
	for _, table := range []string{"catalog_items", "journal_entries", "todos", "habit_logs"} {
		if n := countRows(t, pool, table); n != 0 {
			t.Errorf("%s has %d rows, want 0", table, n)
		}
	}
	if after := countRows(t, pool, "events"); after != before {
		t.Errorf("events = %d, want %d: a refused call appends no event", after, before)
	}
}

func TestAnUpdateKeepsTheSourceRef(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	source := captureText(t, s, "Read Dune.")
	item, err := s.AddItem(ctx, AddItemParams{Kind: "book", Title: "Dune", Provenance: Provenance{Source: SourceAgent, SourceRef: &source.ID}})
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	changed, err := s.UpdateItem(ctx, item.ID, UpdateItemParams{Rating: ptr(4.0), Actor: SourceManual})
	if err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}
	if changed.SourceRef == nil || *changed.SourceRef != source.ID {
		t.Errorf("source_ref after the update = %v, want %d", changed.SourceRef, source.ID)
	}
}
