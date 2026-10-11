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
		_, err := s.Verify(ctx, []RecordRef{{recordType, 9999}}, SourceManual)
		var invalid *ValidationError
		if !errors.As(err, &invalid) || !strings.Contains(invalid.Message, recordType+" 9999 does not exist") {
			t.Errorf("Verify %s: error = %v, want it to say that the record does not exist", recordType, err)
		}
		// A field that no type has: each type must refuse it by name.
		_, err = s.Update(ctx, recordType, 9999, UpdateParams{Fields: map[string]any{"verified": true}, Actor: SourceManual})
		if !errors.As(err, &invalid) {
			t.Errorf("Update %s: error = %v, want a ValidationError", recordType, err)
		}
	}
}

// addInferredJournal stores a journal entry that is not verified. No
// adapter writes such a row; the queue and Verify must still handle it.
func addInferredJournal(t *testing.T, s *Service, body string) JournalEntry {
	t.Helper()
	entry, err := s.AddJournalEntry(context.Background(), AddJournalEntryParams{Body: body, Provenance: Provenance{Source: SourceAgent}})
	if err != nil {
		t.Fatalf("AddJournalEntry(%q): %v", body, err)
	}
	return entry
}

func TestReviewQueueListsEveryTypeOldestFirst(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	first := addInferredJournal(t, s, "went to the gym.\n\n"+strings.Repeat("and then more words ", 10))
	second := addUnverified(t, s, "Dune")
	third := addInferredJournal(t, s, "short")
	addJournal(t, s, "verified", "")

	got, err := s.ReviewQueue(ctx, 0)
	if err != nil {
		t.Fatalf("ReviewQueue: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("queue = %+v, want 3 entries", got)
	}
	want := []RecordRef{{"journal", first.ID}, {"item", second.ID}, {"journal", third.ID}}
	for i, w := range want {
		if got[i].Type != w.Type || got[i].ID != w.ID {
			t.Errorf("entry %d = %s %d, want %s %d", i, got[i].Type, got[i].ID, w.Type, w.ID)
		}
	}
	const wantSummary = "journal 2026-10-11: went to the gym. and then more words and then more words and then more words and…"
	if got[0].Summary != wantSummary {
		t.Errorf("summary = %q, want %q", got[0].Summary, wantSummary)
	}
	if got[2].Summary != "journal 2026-10-11: short" {
		t.Errorf("summary = %q", got[2].Summary)
	}

	limited, err := s.ReviewQueue(ctx, 2)
	if err != nil || len(limited) != 2 || limited[1].Type != "item" {
		t.Errorf("queue with limit 2 = %+v, %v, want the two oldest", limited, err)
	}
}

func TestVerifyApprovesAJournalEntry(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	entry := addInferredJournal(t, s, "a day")
	item := addUnverified(t, s, "Dune")

	n, err := s.Verify(ctx, []RecordRef{{"journal", entry.ID}, {"item", item.ID}}, SourceManual)
	if err != nil || n != 2 {
		t.Fatalf("Verify = %d, %v, want 2", n, err)
	}
	if got, _ := s.GetJournalEntry(ctx, entry.ID); !got.Verified {
		t.Error("the journal entry is not verified")
	}
	events := allEvents(t, pool)
	e := events[len(events)-2]
	if e.Action != "verify" || e.RecordType != "journal_entries" || e.RecordID != entry.ID || e.Old["verified"] != false || e.New["verified"] != true {
		t.Errorf("event = %+v, want a verify of the journal entry", e)
	}

	// A second approval changes nothing and appends no event.
	before := countRows(t, pool, "events")
	if n, err := s.Verify(ctx, []RecordRef{{"journal", entry.ID}}, SourceManual); err != nil || n != 0 {
		t.Errorf("second Verify = %d, %v, want 0", n, err)
	}
	if after := countRows(t, pool, "events"); after != before {
		t.Errorf("events = %d, want %d", after, before)
	}
}
