package service

import (
	"context"
	"errors"
	"testing"
)

// deleteByHand marks one record as deleted, as the admin pages do.
func deleteByHand(t *testing.T, s *Service, recordType string, id int64) {
	t.Helper()
	if err := s.Delete(context.Background(), recordType, id, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("Delete %s %d: %v", recordType, id, err)
	}
}

func TestPurgeRemovesADeletedRecordOfEachType(t *testing.T) {
	s, pool := newTestService(t)
	addHabit(t, s, "gym")
	records := []struct {
		recordType, table string
		id                int64
	}{
		{"item", "catalog_items", addItem(t, s, "book", "Dune").ID},
		{"journal", "journal_entries", addJournal(t, s, "a private day", "2026-10-09").ID},
		{"todo", "todos", addTodo(t, s, "Buy a domain").ID},
		{"habit_log", "habit_logs", logHabit(t, s, "gym", "2026-10-09").ID},
	}

	for _, r := range records {
		deleteByHand(t, s, r.recordType, r.id)
		if err := s.Purge(context.Background(), r.recordType, r.id); err != nil {
			t.Errorf("Purge %s: %v", r.recordType, err)
			continue
		}
		if n := countRows(t, pool, r.table); n != 0 {
			t.Errorf("%s has %d rows after the purge, want 0", r.table, n)
		}
	}
}

func TestPurgeEmptiesTheValuesOfTheEarlierEvents(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	secret := addItem(t, s, "book", "A private title")
	kept := addItem(t, s, "book", "Dune")
	_, err := s.Update(ctx, "item", secret.ID, UpdateParams{Fields: map[string]any{"title": "A second private title"}, Actor: SourceManual})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	deleteByHand(t, s, "item", secret.ID)

	if err := s.Purge(ctx, "item", secret.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	var actions []string
	for _, e := range allEvents(t, pool) {
		if e.RecordID == kept.ID {
			if e.New["title"] != "Dune" {
				t.Errorf("the event of the other record lost its value: %v", e.New)
			}
			continue
		}
		actions = append(actions, e.Action)
		if e.Old != nil || e.New != nil {
			t.Errorf("the %s event keeps a value: old %v, new %v", e.Action, e.Old, e.New)
		}
		if e.RecordType != "catalog_items" {
			t.Errorf("the %s event has the record type %q, want catalog_items", e.Action, e.RecordType)
		}
	}
	if want := []string{"create", "update", "delete", "purge"}; !equal(actions, want) {
		t.Errorf("the events of the record are %v, want %v", actions, want)
	}
	events := allEvents(t, pool)
	if last := events[len(events)-1]; last.Action != "purge" || last.Source != "manual" {
		t.Errorf("the last event is %s by %s, want purge by manual", last.Action, last.Source)
	}
}

func TestPurgeRefusesARecordThatIsNotDeleted(t *testing.T) {
	s, pool := newTestService(t)
	item := addItem(t, s, "book", "Dune")

	err := s.Purge(context.Background(), "item", item.ID)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if n := countRows(t, pool, "catalog_items"); n != 1 {
		t.Errorf("catalog_items has %d rows, want 1", n)
	}
	events := allEvents(t, pool)
	if len(events) != 1 || events[0].New["title"] != "Dune" {
		t.Errorf("the events changed: %+v", events)
	}
}

func TestPurgeLeavesTheOtherDeletedRecords(t *testing.T) {
	s, pool := newTestService(t)
	first := addItem(t, s, "book", "Dune")
	second := addItem(t, s, "book", "Emma")
	deleteByHand(t, s, "item", first.ID)
	deleteByHand(t, s, "item", second.ID)

	if err := s.Purge(context.Background(), "item", first.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	if n := countRows(t, pool, "catalog_items"); n != 1 {
		t.Errorf("catalog_items has %d rows, want 1", n)
	}
	// A todo with the same ID is a different record.
	todo := addTodo(t, s, "Buy a domain")
	deleteByHand(t, s, "todo", todo.ID)
	if err := s.Purge(context.Background(), "item", second.ID); err != nil {
		t.Fatalf("Purge the second item: %v", err)
	}
	if n := countRows(t, pool, "todos"); n != 1 {
		t.Errorf("todos has %d rows, want 1", n)
	}
	for _, e := range allEvents(t, pool) {
		if e.RecordType == "todos" && e.Action == "create" && e.New["title"] != "Buy a domain" {
			t.Errorf("the create event of the todo lost its value: %v", e.New)
		}
	}
}

func TestPurgeRefusesATypeThatIsNotARecordTypeAndARecordThatDoesNotExist(t *testing.T) {
	s, _ := newTestService(t)

	err := s.Purge(context.Background(), "habit", 1)
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "type" {
		t.Errorf("type habit: error = %v, want a ValidationError for type", err)
	}

	if err := s.Purge(context.Background(), "item", 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("item 999: error = %v, want ErrNotFound", err)
	}
}

// A double click sends the same request two times.
func TestPurgeOfTheSameRecordAtTheSameMomentRemovesItOneTime(t *testing.T) {
	s, pool := newTestService(t)
	item := addItem(t, s, "book", "Dune")
	deleteByHand(t, s, "item", item.ID)

	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- s.Purge(context.Background(), "item", item.ID) }()
	}
	var done, missing int
	for range 2 {
		switch err := <-errs; {
		case err == nil:
			done++
		case errors.Is(err, ErrNotFound):
			missing++
		default:
			t.Errorf("Purge: %v", err)
		}
	}

	if done != 1 || missing != 1 {
		t.Errorf("%d calls removed the record and %d found none, want 1 and 1", done, missing)
	}
	var purges int
	for _, e := range allEvents(t, pool) {
		if e.Action == "purge" {
			purges++
		}
	}
	if purges != 1 {
		t.Errorf("the log has %d purge events, want 1", purges)
	}
}
