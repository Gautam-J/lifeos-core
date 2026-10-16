package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRecordFunctionsWorkOnAJournalEntry(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added := addJournal(t, s, "a day", "2026-10-09")

	got, err := s.Get(ctx, "journal", added.ID)
	if entry, ok := got.(JournalEntry); err != nil || !ok || entry != added {
		t.Fatalf("Get = %#v, %v, want the entry", got, err)
	}

	got, err = s.Update(ctx, "journal", added.ID, UpdateParams{Fields: map[string]any{"body": "a good day"}, Actor: SourceManual})
	if entry, ok := got.(JournalEntry); err != nil || !ok || entry.Body != "a good day" || entry.EntryDate != "2026-10-09" {
		t.Fatalf("Update = %#v, %v, want the changed entry", got, err)
	}

	refused := map[string]UpdateParams{
		"verified":   {Fields: map[string]any{"verified": false}, Actor: SourceManual},
		"id":         {Fields: map[string]any{"id": 7}, Actor: SourceManual},
		"title":      {Fields: map[string]any{"title": "a field of an item"}, Actor: SourceManual},
		"body":       {Fields: map[string]any{"body": 5}, Actor: SourceManual},
		"entry_date": {Fields: map[string]any{"entry_date": nil}, Actor: SourceManual},
		"clear":      {Fields: map[string]any{"body": "x"}, Clear: []string{"entry_date"}, Actor: SourceManual},
	}
	for field, p := range refused {
		_, err := s.Update(ctx, "journal", added.ID, p)
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != field {
			t.Errorf("%s: error = %v, want a ValidationError on %s", field, err, field)
		}
	}

	if err := s.Delete(ctx, "journal", added.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "journal", added.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: error = %v, want ErrNotFound", err)
	}
}

// A type in RecordTypes with no code in one entry point is a bug that only
// shows when an agent uses that type. Each entry point must answer "not
// found" for an ID that does not exist, for every type.
func TestEveryRecordTypeHasEveryEntryPoint(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	for _, recordType := range RecordTypes {
		if _, err := s.Get(ctx, recordType, 9999); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get %s: error = %v, want ErrNotFound", recordType, err)
		}
		if err := s.Delete(ctx, recordType, 9999, DeleteParams{Actor: SourceManual}); !errors.Is(err, ErrNotFound) {
			t.Errorf("Delete %s: error = %v, want ErrNotFound", recordType, err)
		}
		// A field that no type has: each type must refuse it by name.
		var invalid *ValidationError
		_, err := s.Update(ctx, recordType, 9999, UpdateParams{Fields: map[string]any{"verified": true}, Actor: SourceManual})
		if !errors.As(err, &invalid) {
			t.Errorf("Update %s: error = %v, want a ValidationError", recordType, err)
		}
	}
}

// addInferredJournal stores a journal entry that is not verified. No
// adapter writes such a row.
func addInferredJournal(t *testing.T, s *Service, body string) JournalEntry {
	t.Helper()
	entry, err := s.AddJournalEntry(context.Background(), AddJournalEntryParams{Body: body, Provenance: Provenance{Source: SourceAgent}})
	if err != nil {
		t.Fatalf("AddJournalEntry(%q): %v", body, err)
	}
	return entry
}

// The one line of a journal entry, which search and the list of deleted
// records show: the day and the start of the body, on one line.
func TestAJournalSummaryIsOneShortLine(t *testing.T) {
	s, _ := newTestService(t)
	long := addInferredJournal(t, s, "went to the gym.\n\n"+strings.Repeat("and then more words ", 10))
	short := addInferredJournal(t, s, "short")

	const want = "journal 2026-10-11: went to the gym. and then more words and then more words and then more words and…"
	if got := long.summary(); got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
	if got := short.summary(); got != "journal 2026-10-11: short" {
		t.Errorf("summary = %q", got)
	}
}
