package service

import (
	"context"
	"errors"
	"testing"
)

// recordRef names one record by its type and its ID.
type recordRef struct {
	Type string
	ID   int64
}

// deletedRefs returns the type and the ID of each record, in order.
func deletedRefs(records []DeletedRecord) []recordRef {
	refs := []recordRef{}
	for _, r := range records {
		refs = append(refs, recordRef{Type: r.Type, ID: r.ID})
	}
	return refs
}

func TestListDeletedGivesTheDeletedRecordsOfEachTypeWithTheLastDeletedFirst(t *testing.T) {
	s, _ := newTestService(t)
	addHabit(t, s, "gym")
	item := addItem(t, s, "book", "Dune")
	entry := addJournal(t, s, "a private day", "2026-10-09")
	todo := addTodo(t, s, "Buy a domain")
	log := logHabit(t, s, "gym", "2026-10-09")
	source := captureText(t, s, "a note")
	addItem(t, s, "book", "Emma")
	order := []recordRef{
		{"todo", todo.ID}, {"item", item.ID}, {"source", source.ID}, {"habit_log", log.ID}, {"journal", entry.ID},
	}
	for _, ref := range order {
		deleteByHand(t, s, ref.Type, ref.ID)
	}

	got, err := s.ListDeleted(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListDeleted: %v", err)
	}

	want := []recordRef{
		{"journal", entry.ID}, {"habit_log", log.ID}, {"source", source.ID}, {"item", item.ID}, {"todo", todo.ID},
	}
	refs := deletedRefs(got)
	if len(refs) != len(want) {
		t.Fatalf("ListDeleted gave %v, want %v", refs, want)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("ListDeleted gave %v, want %v", refs, want)
		}
	}
	summaries := map[string]string{
		"journal":   "journal 2026-10-09: a private day",
		"habit_log": "habit: gym on 2026-10-09",
		"source":    "text: a note",
		"item":      "book: Dune",
		"todo":      "todo: Buy a domain, project lifeos, due 2026-10-20",
	}
	for _, r := range got {
		if r.Summary != summaries[r.Type] {
			t.Errorf("the summary of the %s is %q, want %q", r.Type, r.Summary, summaries[r.Type])
		}
		if r.DeletedAt.IsZero() {
			t.Errorf("the %s has no time of the delete", r.Type)
		}
	}
}

func TestListDeletedLeavesOutARecordThatIsPurged(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	first := addItem(t, s, "book", "Dune")
	second := addItem(t, s, "book", "Emma")
	deleteByHand(t, s, "item", first.ID)
	deleteByHand(t, s, "item", second.ID)
	if err := s.Purge(ctx, "item", first.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	got, err := s.ListDeleted(ctx, 0)
	if err != nil {
		t.Fatalf("ListDeleted: %v", err)
	}

	if refs := deletedRefs(got); len(refs) != 1 || refs[0] != (recordRef{"item", second.ID}) {
		t.Errorf("ListDeleted gave %v, want only item %d", refs, second.ID)
	}
}

func TestListDeletedObeysTheLimit(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	first := addItem(t, s, "book", "Dune")
	todo := addTodo(t, s, "Buy a domain")
	last := addItem(t, s, "book", "Emma")
	for _, ref := range []recordRef{{"item", first.ID}, {"todo", todo.ID}, {"item", last.ID}} {
		deleteByHand(t, s, ref.Type, ref.ID)
	}

	got, err := s.ListDeleted(ctx, 2)
	if err != nil {
		t.Fatalf("ListDeleted: %v", err)
	}
	if refs := deletedRefs(got); len(refs) != 2 || refs[0] != (recordRef{"item", last.ID}) || refs[1] != (recordRef{"todo", todo.ID}) {
		t.Errorf("ListDeleted with limit 2 gave %v, want item %d, then todo %d", refs, last.ID, todo.ID)
	}

	_, err = s.ListDeleted(ctx, -1)
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "limit" {
		t.Errorf("limit -1: error = %v, want a ValidationError for limit", err)
	}

	none, err := newEmptyList(t)
	if err != nil || none == nil || len(none) != 0 {
		t.Errorf("an empty database gave %v, %v; want an empty list that is not nil", none, err)
	}
}

// newEmptyList returns the deleted records of a database that has none.
func newEmptyList(t *testing.T) ([]DeletedRecord, error) {
	t.Helper()
	s, _ := newTestService(t)
	return s.ListDeleted(context.Background(), 0)
}

func TestListDeletedShowsTheLogOfAHabitThatIsRetired(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	habit := addHabit(t, s, "gym")
	log := logHabit(t, s, "gym", "2026-10-09")
	deleteByHand(t, s, "habit_log", log.ID)
	if _, err := s.UpdateHabit(ctx, habit.ID, UpdateHabitParams{Active: ptr(false), Actor: SourceManual}); err != nil {
		t.Fatalf("UpdateHabit: %v", err)
	}

	got, err := s.ListDeleted(ctx, 0)
	if err != nil {
		t.Fatalf("ListDeleted: %v", err)
	}

	if len(got) != 1 || got[0].Summary != "habit: gym on 2026-10-09" {
		t.Errorf("ListDeleted gave %+v, want the log of gym", got)
	}
}
