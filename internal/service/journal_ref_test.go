package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestARecordFromAnEntryNamesTheEntry(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	entry := addJournal(t, s, "Went for a run. Booked the bike service.", "")
	addHabit(t, s, "run")
	todo := addTodo(t, s, "Book the bike service")

	log, err := s.LogHabit(ctx, LogHabitParams{Habit: "run", LoggedOn: entry.EntryDate, JournalRef: entry.ID, Provenance: Provenance{Source: SourceAgent}})
	if err != nil || log.JournalRef != entry.ID {
		t.Errorf("LogHabit = %+v, %v; want journal_ref %d", log, err, entry.ID)
	}
	closed, err := s.UpdateTodo(ctx, todo.ID, UpdateTodoParams{Status: ptr("done"), JournalRef: &entry.ID, Actor: SourceAgent})
	if err != nil || closed.Status != "done" || closed.JournalRef != entry.ID {
		t.Errorf("UpdateTodo = %+v, %v; want done with journal_ref %d", closed, err, entry.ID)
	}
	item, err := s.AddItem(ctx, AddItemParams{Kind: "movie", Title: "Dune", JournalRef: entry.ID, Provenance: Provenance{Source: SourceAgent}})
	if err != nil || item.JournalRef != entry.ID {
		t.Errorf("AddItem = %+v, %v", item, err)
	}
	added, err := s.AddTodo(ctx, AddTodoParams{Title: "Pay the bike service", JournalRef: entry.ID, Provenance: Provenance{Source: SourceAgent}})
	if err != nil || added.JournalRef != entry.ID {
		t.Errorf("AddTodo = %+v, %v", added, err)
	}
}

// Review Focus 3: an unknown entry and a deleted entry are refused alike.
func TestJournalRefMustNameALiveEntry(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	gone := addJournal(t, s, "An entry that goes.", "")
	if err := s.DeleteJournalEntry(ctx, gone.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("DeleteJournalEntry: %v", err)
	}
	for _, ref := range []int64{gone.ID, 999999} {
		_, err := s.AddTodo(ctx, AddTodoParams{Title: "x", JournalRef: ref, Provenance: Provenance{Source: SourceAgent}})
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != "journal_ref" {
			t.Errorf("journal_ref %d: err = %v, want a ValidationError of journal_ref", ref, err)
		}
	}
	todo := addTodo(t, s, "Close it")
	_, err := s.UpdateTodo(ctx, todo.ID, UpdateTodoParams{Status: ptr("done"), JournalRef: &gone.ID, Actor: SourceAgent})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "journal_ref" {
		t.Errorf("UpdateTodo with a deleted entry: err = %v, want a ValidationError of journal_ref", err)
	}
}

// Review Focus 1: the same close a second time is not a write.
func TestTheSameCloseTwiceWritesOnce(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	entry := addJournal(t, s, "Paid the bill.", "")
	todo := addTodo(t, s, "Pay the bill")
	p := UpdateTodoParams{Status: ptr("done"), JournalRef: &entry.ID, Actor: SourceAgent}

	if _, err := s.UpdateTodo(ctx, todo.ID, p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateTodo(ctx, todo.ID, p); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM events WHERE record_type = 'todos' AND record_id = $1 AND action = 'update'", todo.ID).Scan(&n); err != nil || n != 1 {
		t.Errorf("update events = %d, %v; want 1", n, err)
	}
}

// Review Focus 2: a log for a day that has one already stays as it is.
func TestALogFromAnEntryOverAStoredLogChangesNothing(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	addHabit(t, s, "run")
	entry := addJournal(t, s, "Ran again.", "")
	stored, err := s.LogHabit(ctx, LogHabitParams{Habit: "run", Provenance: Provenance{Source: SourceManual}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.LogHabit(ctx, LogHabitParams{Habit: "run", JournalRef: entry.ID, Provenance: Provenance{Source: SourceAgent}})
	if err != nil || got != stored {
		t.Errorf("second log = %+v, %v; want the stored log %+v", got, err, stored)
	}
}

// Review Focus 4: an entry that a record names cannot go for good.
func TestPurgeRefusesAnEntryThatARecordNames(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	entry := addJournal(t, s, "Booked it.", "")
	todo, err := s.AddTodo(ctx, AddTodoParams{Title: "Book it", JournalRef: entry.ID, Provenance: Provenance{Source: SourceAgent}})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []struct {
		kind string
		id   int64
	}{{"todo", todo.ID}, {"journal", entry.ID}} {
		if err := s.Delete(ctx, d.kind, d.id, DeleteParams{Actor: SourceManual}); err != nil {
			t.Fatalf("delete %s: %v", d.kind, err)
		}
	}
	err = s.Purge(ctx, "journal", entry.ID)
	var invalid *ValidationError
	if !errors.As(err, &invalid) || !strings.Contains(invalid.Error(), "journal_ref") {
		t.Errorf("Purge of a named entry: err = %v, want a refusal that names journal_ref", err)
	}
	if err := s.Purge(ctx, "todo", todo.ID); err != nil {
		t.Fatalf("Purge todo: %v", err)
	}
	if err := s.Purge(ctx, "journal", entry.ID); err != nil {
		t.Errorf("Purge of the entry after the todo: %v", err)
	}
}

// Final review: an entry written after midnight is about the day before.
// A log from that entry with no day takes the day of the entry, not today.
func TestALogFromAnEntryTakesTheDayOfTheEntry(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	addHabit(t, s, "run")
	entry := addJournal(t, s, "Ran in the evening.", "2026-10-10") // today is 2026-10-11 in IST

	log, err := s.LogHabit(ctx, LogHabitParams{Habit: "run", JournalRef: entry.ID, Provenance: Provenance{Source: SourceAgent}})
	if err != nil || log.LoggedOn != "2026-10-10" {
		t.Errorf("LogHabit = %+v, %v; want the day of the entry, 2026-10-10", log, err)
	}
	stated, err := s.LogHabit(ctx, LogHabitParams{Habit: "run", LoggedOn: "2026-10-11", JournalRef: entry.ID, Provenance: Provenance{Source: SourceAgent}})
	if err != nil || stated.LoggedOn != "2026-10-11" {
		t.Errorf("LogHabit with a day = %+v, %v; want the day that the call gives", stated, err)
	}
}
