package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestUpdateJournalEntryChangesOnlyTheGivenFields(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added := addJournal(t, s, "a day", "2026-10-09")

	got, err := s.UpdateJournalEntry(ctx, added.ID, UpdateJournalEntryParams{EntryDate: ptr("2026-10-08"), Actor: SourceManual})
	if err != nil {
		t.Fatalf("UpdateJournalEntry: %v", err)
	}
	if got.EntryDate != "2026-10-08" || got.Body != "a day" {
		t.Errorf("after a change of the date: %+v", got)
	}

	// The new body is stored as written, the same as a new entry.
	const body = "  a long day\n\nwith two paragraphs \n"
	got, err = s.UpdateJournalEntry(ctx, added.ID, UpdateJournalEntryParams{Body: ptr(body), Actor: SourceManual})
	if err != nil {
		t.Fatalf("UpdateJournalEntry: %v", err)
	}
	if got.Body != body || got.EntryDate != "2026-10-08" {
		t.Errorf("after a change of the body: %+v", got)
	}
	if got.Source != SourceAgent {
		t.Errorf("source = %q, want it unchanged", got.Source)
	}
	stored, err := s.GetJournalEntry(ctx, added.ID)
	if err != nil || stored != got {
		t.Errorf("stored = %+v, %v, want %+v", stored, err, got)
	}
}

func TestUpdateJournalEntryAppendsAnUpdateEventWithTheOldValue(t *testing.T) {
	s, pool := newTestService(t)
	added := addJournal(t, s, "a day", "2026-10-09")

	if _, err := s.UpdateJournalEntry(context.Background(), added.ID, UpdateJournalEntryParams{Body: ptr("a good day"), Actor: SourceManual}); err != nil {
		t.Fatalf("UpdateJournalEntry: %v", err)
	}

	events := allEvents(t, pool)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (create, update)", len(events))
	}
	e := events[1]
	if e.Action != "update" || e.RecordType != "journal_entries" || e.RecordID != added.ID || e.Source != "manual" {
		t.Errorf("event = %s %s %d by %s, want update journal_entries %d by manual", e.Action, e.RecordType, e.RecordID, e.Source, added.ID)
	}
	if e.Old["body"] != "a day" || e.New["body"] != "a good day" {
		t.Errorf("old, new body = %v, %v", e.Old["body"], e.New["body"])
	}
}

func TestUpdateJournalEntryRejectsBadInput(t *testing.T) {
	s, pool := newTestService(t)
	added := addJournal(t, s, "a day", "2026-10-09")
	tests := []struct {
		name      string
		params    UpdateJournalEntryParams
		wantField string
	}{
		{"no field", UpdateJournalEntryParams{Actor: SourceManual}, "fields"},
		{"empty body", UpdateJournalEntryParams{Body: ptr(" \n"), Actor: SourceManual}, "body"},
		{"body with a NUL character", UpdateJournalEntryParams{Body: ptr("a\x00b"), Actor: SourceManual}, "body"},
		{"empty date", UpdateJournalEntryParams{EntryDate: ptr(""), Actor: SourceManual}, "entry_date"},
		{"date in the future", UpdateJournalEntryParams{EntryDate: ptr("2026-10-12"), Actor: SourceManual}, "entry_date"},
		{"unknown actor", UpdateJournalEntryParams{Body: ptr("x"), Actor: "robot"}, "source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.UpdateJournalEntry(context.Background(), added.ID, tt.params)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want 1 (the create): refused input appended an event", n)
	}
}

func TestUpdateJournalEntryReturnsNotFound(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	deleted := addJournal(t, s, "a day", "")
	if err := s.DeleteJournalEntry(ctx, deleted.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	for name, id := range map[string]int64{"deleted": deleted.ID, "absent": 9999} {
		_, err := s.UpdateJournalEntry(ctx, id, UpdateJournalEntryParams{Body: ptr("x"), Actor: SourceManual})
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", name, err)
		}
	}
}

func TestUpdateJournalEntryWritesNothingWhenNoValueChanges(t *testing.T) {
	s, pool := newTestService(t)
	added := addJournal(t, s, "a day", "2026-10-09")

	got, err := s.UpdateJournalEntry(context.Background(), added.ID, UpdateJournalEntryParams{
		Body: ptr("a day"), EntryDate: ptr("2026-10-09"), Actor: SourceManual,
	})

	if err != nil || got != added {
		t.Errorf("UpdateJournalEntry = %+v, %v, want the stored entry and no error", got, err)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want 1 (the create)", n)
	}
}

func TestDeleteJournalEntry(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added := addJournal(t, s, "a day", "")

	err := s.DeleteJournalEntry(ctx, added.ID, DeleteParams{Actor: SourceAgent})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "confirmed" {
		t.Fatalf("delete by an agent with no confirmation: error = %v, want a ValidationError on confirmed", err)
	}

	if err := s.DeleteJournalEntry(ctx, added.ID, DeleteParams{Actor: SourceAgent, Confirmed: true}); err != nil {
		t.Fatalf("DeleteJournalEntry: %v", err)
	}
	if _, err := s.GetJournalEntry(ctx, added.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get after delete: error = %v, want ErrNotFound", err)
	}
	if entries, _ := s.ListJournalEntries(ctx, 0); len(entries) != 0 {
		t.Errorf("list after delete = %v, want it empty", entries)
	}
	if n := countRows(t, pool, "journal_entries"); n != 1 {
		t.Errorf("journal_entries has %d rows, want 1: a delete keeps the row", n)
	}
	events := allEvents(t, pool)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (create, delete)", len(events))
	}
	e := events[1]
	if e.Action != "delete" || e.RecordType != "journal_entries" || e.RecordID != added.ID || e.Source != "agent" {
		t.Errorf("event = %s %s %d by %s", e.Action, e.RecordType, e.RecordID, e.Source)
	}
	if e.Old["body"] != "a day" || e.New != nil {
		t.Errorf("old, new = %v, %v, want the entry and none", e.Old, e.New)
	}

	if err := s.DeleteJournalEntry(ctx, added.ID, DeleteParams{Actor: SourceManual}); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: error = %v, want ErrNotFound", err)
	}
	if n := countRows(t, pool, "events"); n != 2 {
		t.Errorf("events = %d after a second delete, want 2", n)
	}
}

// #51: the service keeps rule 10 for an agent. The words of an entry are
// the words of Gautam, so an agent does not change them.
func TestUpdateJournalEntryRefusesABodyChangeByAnAgent(t *testing.T) {
	s, pool := newTestService(t)
	added := addJournal(t, s, "a day", "2026-10-09")

	_, err := s.UpdateJournalEntry(context.Background(), added.ID, UpdateJournalEntryParams{Body: ptr("a better day"), Actor: SourceAgent})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "body" || !strings.Contains(err.Error(), "an agent does not change the words of a journal entry") {
		t.Fatalf("error = %v, want a ValidationError on body that says an agent does not change the words", err)
	}
	stored, err := s.GetJournalEntry(context.Background(), added.ID)
	if err != nil || stored != added {
		t.Errorf("stored = %+v, %v, want the entry unchanged", stored, err)
	}
	if n := len(allEvents(t, pool)); n != 1 {
		t.Errorf("events = %d, want 1 (create only)", n)
	}

	// The refusal does not depend on the old text: the same body is refused too.
	_, err = s.UpdateJournalEntry(context.Background(), added.ID, UpdateJournalEntryParams{Body: ptr("a day"), Actor: SourceAgent})
	if !errors.As(err, &invalid) || invalid.Field != "body" {
		t.Errorf("same body: error = %v, want a ValidationError on body", err)
	}
	// A body with a date is refused as a whole.
	_, err = s.UpdateJournalEntry(context.Background(), added.ID, UpdateJournalEntryParams{Body: ptr("x"), EntryDate: ptr("2026-10-08"), Actor: SourceAgent})
	if !errors.As(err, &invalid) {
		t.Errorf("body and date: error = %v, want a ValidationError", err)
	}
	if got, _ := s.GetJournalEntry(context.Background(), added.ID); got.EntryDate != "2026-10-09" {
		t.Errorf("entry_date = %s, want it unchanged after a refused call", got.EntryDate)
	}
}

func TestUpdateJournalEntryLetsAnAgentChangeTheDate(t *testing.T) {
	s, pool := newTestService(t)
	added := addJournal(t, s, "a day", "2026-10-09")

	got, err := s.UpdateJournalEntry(context.Background(), added.ID, UpdateJournalEntryParams{EntryDate: ptr("2026-10-08"), Actor: SourceAgent})
	if err != nil || got.EntryDate != "2026-10-08" || got.Body != "a day" {
		t.Fatalf("UpdateJournalEntry = %+v, %v, want the new date and the same body", got, err)
	}
	events := allEvents(t, pool)
	if len(events) != 2 || events[1].Action != "update" || events[1].Source != "agent" {
		t.Errorf("events = %+v, want create and one update by agent", events)
	}
}
