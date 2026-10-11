package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

// addJournal stores one journal entry as the MCP tool does: the words are
// Gautam's own, so the row is verified.
func addJournal(t *testing.T, s *Service, body, entryDate string) JournalEntry {
	t.Helper()
	entry, err := s.AddJournalEntry(context.Background(), AddJournalEntryParams{
		Body:       body,
		EntryDate:  entryDate,
		Provenance: Provenance{Source: SourceAgent, Verified: true},
	})
	if err != nil {
		t.Fatalf("AddJournalEntry(%q): %v", body, err)
	}
	return entry
}

// The journal holds Gautam's words. The service must not trim, correct or
// shorten them.
func TestAddJournalEntryStoresTheBodyExactlyAsWritten(t *testing.T) {
	s, pool := newTestService(t)
	const body = "  spent the day on LifeOS\n\n- planned the schema\n- no gym :(  \n"

	got, err := s.AddJournalEntry(context.Background(), AddJournalEntryParams{
		Body:       body,
		Provenance: Provenance{Source: SourceAgent, Verified: true},
	})
	if err != nil {
		t.Fatalf("AddJournalEntry: %v", err)
	}

	if got.ID == 0 {
		t.Error("AddJournalEntry returned ID 0")
	}
	if got.Body != body {
		t.Errorf("body = %q, want it unchanged: %q", got.Body, body)
	}
	var stored string
	if err := pool.QueryRow(context.Background(), "SELECT body FROM journal_entries").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != body {
		t.Errorf("stored body = %q, want it unchanged: %q", stored, body)
	}
	if got.Source != SourceAgent || !got.Verified {
		t.Errorf("source, verified = %q, %v", got.Source, got.Verified)
	}
	if got.CreatedAt.IsZero() || got.CreatedAt.Location() != time.UTC {
		t.Errorf("created_at = %v, want a time in UTC", got.CreatedAt)
	}
}

// Between 00:00 and 05:30 IST the UTC date is one day behind. An entry
// with no date is about today in IST.
func TestAddJournalEntryUsesTheISTDateAsToday(t *testing.T) {
	s, _ := newTestService(t) // the clock is 2026-10-10 20:00 UTC = 2026-10-11 01:30 IST
	ctx := context.Background()

	if got := addJournal(t, s, "no date", ""); got.EntryDate != "2026-10-11" {
		t.Errorf("entry_date = %q, want 2026-10-11 (today in IST)", got.EntryDate)
	}
	if got := addJournal(t, s, "about last week", "2026-10-03"); got.EntryDate != "2026-10-03" {
		t.Errorf("entry_date = %q, want 2026-10-03", got.EntryDate)
	}

	_, err := s.AddJournalEntry(ctx, AddJournalEntryParams{
		Body: "about tomorrow", EntryDate: "2026-10-12", Provenance: Provenance{Source: SourceManual, Verified: true},
	})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "entry_date" {
		t.Errorf("entry for tomorrow in IST: error = %v, want a ValidationError on entry_date", err)
	}
}

func TestAddJournalEntryAppendsACreateEvent(t *testing.T) {
	s, pool := newTestService(t)

	entry := addJournal(t, s, "planned the schema", "")

	events := allEvents(t, pool)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Action != "create" || e.RecordType != "journal_entries" || e.RecordID != entry.ID || e.Source != "agent" {
		t.Errorf("event = %s %s %d by %s, want create journal_entries %d by agent", e.Action, e.RecordType, e.RecordID, e.Source, entry.ID)
	}
	if e.Old != nil {
		t.Errorf("old value = %v, want none", e.Old)
	}
	if e.New["body"] != "planned the schema" || e.New["entry_date"] != "2026-10-11" {
		t.Errorf("new value = %v, want the stored entry", e.New)
	}
}

func TestAddJournalEntryRejectsBadInput(t *testing.T) {
	valid := AddJournalEntryParams{Body: "a day", Provenance: Provenance{Source: SourceManual, Verified: true}}
	tests := []struct {
		name      string
		change    func(*AddJournalEntryParams)
		wantField string
	}{
		{"empty body", func(p *AddJournalEntryParams) { p.Body = "" }, "body"},
		{"body of spaces", func(p *AddJournalEntryParams) { p.Body = " \t\n" }, "body"},
		{"body with a NUL character", func(p *AddJournalEntryParams) { p.Body = "a\x00b" }, "body"},
		{"body that is not UTF-8", func(p *AddJournalEntryParams) { p.Body = "a\xffb" }, "body"},
		{"date that is not a date", func(p *AddJournalEntryParams) { p.EntryDate = "yesterday" }, "entry_date"},
		{"date in the wrong order", func(p *AddJournalEntryParams) { p.EntryDate = "11-10-2026" }, "entry_date"},
		{"day that does not exist", func(p *AddJournalEntryParams) { p.EntryDate = "2026-02-30" }, "entry_date"},
		{"unknown source", func(p *AddJournalEntryParams) { p.Source = "robot" }, "source"},
	}
	s, pool := newTestService(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := valid
			tt.change(&p)
			_, err := s.AddJournalEntry(context.Background(), p)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
	if n := countRows(t, pool, "journal_entries"); n != 0 {
		t.Errorf("journal_entries has %d rows after refused input, want 0", n)
	}
	if n := countRows(t, pool, "events"); n != 0 {
		t.Errorf("events has %d rows after refused input, want 0", n)
	}
}

func TestAddJournalEntryStoresNothingWhenTheEventFails(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "DROP TABLE events"); err != nil {
		t.Fatal(err)
	}

	_, err := s.AddJournalEntry(ctx, AddJournalEntryParams{Body: "a day", Provenance: Provenance{Source: SourceManual, Verified: true}})

	if err == nil {
		t.Fatal("AddJournalEntry returned no error although the event insert must fail")
	}
	var invalid *ValidationError
	if errors.As(err, &invalid) {
		t.Errorf("a database failure came back as a ValidationError: %v", err)
	}
	if n := countRows(t, pool, "journal_entries"); n != 0 {
		t.Errorf("journal_entries has %d rows, want 0: the entry must roll back with its event", n)
	}
}

func TestGetJournalEntry(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added := addJournal(t, s, "a day", "2026-10-09")

	got, err := s.GetJournalEntry(ctx, added.ID)
	if err != nil {
		t.Fatalf("GetJournalEntry: %v", err)
	}
	if got != added {
		t.Errorf("GetJournalEntry = %+v, want %+v", got, added)
	}

	if _, err := s.GetJournalEntry(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Errorf("absent entry: error = %v, want ErrNotFound", err)
	}
}

func bodies(entries []JournalEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Body
	}
	return out
}

// The list is by the day that an entry is about, not by the day it was
// typed: an entry written late for last week must not show first.
func TestListJournalEntriesReturnsTheNewestDayFirst(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	addJournal(t, s, "friday", "2026-10-09")
	addJournal(t, s, "sunday morning", "2026-10-11")
	addJournal(t, s, "late entry for last week", "2026-10-03")
	addJournal(t, s, "sunday evening", "2026-10-11")

	all, err := s.ListJournalEntries(ctx, 0)
	if err != nil {
		t.Fatalf("ListJournalEntries: %v", err)
	}
	if want := []string{"sunday evening", "sunday morning", "friday", "late entry for last week"}; !equal(bodies(all), want) {
		t.Errorf("bodies = %v, want %v", bodies(all), want)
	}

	two, err := s.ListJournalEntries(ctx, 2)
	if err != nil {
		t.Fatalf("ListJournalEntries with a limit: %v", err)
	}
	if want := []string{"sunday evening", "sunday morning"}; !equal(bodies(two), want) {
		t.Errorf("bodies with limit 2 = %v, want %v", bodies(two), want)
	}
}

func TestListJournalEntriesReturnsAnEmptyListNotNil(t *testing.T) {
	s, _ := newTestService(t)

	entries, err := s.ListJournalEntries(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListJournalEntries: %v", err)
	}
	if entries == nil || len(entries) != 0 {
		t.Errorf("entries = %#v, want an empty non-nil slice so JSON shows []", entries)
	}
}

func TestListJournalEntriesRejectsABadLimit(t *testing.T) {
	s, _ := newTestService(t)

	for _, limit := range []int{-1, 201} {
		_, err := s.ListJournalEntries(context.Background(), limit)
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != "limit" {
			t.Errorf("limit %d: error = %v, want a ValidationError on limit", limit, err)
		}
	}
}
