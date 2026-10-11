package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// addUnverified stores one item that an agent inferred.
func addUnverified(t *testing.T, s *Service, title string) Item {
	t.Helper()
	p := dune()
	p.Title = title
	p.Verified = false
	item, err := s.AddItem(context.Background(), p)
	if err != nil {
		t.Fatalf("AddItem(%q): %v", title, err)
	}
	return item
}

func TestReviewQueueListsTheUnverifiedRecordsOldestFirst(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	first := addUnverified(t, s, "First")
	addItem(t, s, "movie", "Verified") // addItem stores a verified row
	second := addUnverified(t, s, "Second")
	deleted := addUnverified(t, s, "Deleted")
	if err := s.DeleteItem(ctx, deleted.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ReviewQueue(ctx, 0)
	if err != nil {
		t.Fatalf("ReviewQueue: %v", err)
	}

	if len(got) != 2 || got[0].ID != first.ID || got[1].ID != second.ID {
		t.Fatalf("queue = %+v, want First then Second", got)
	}
	e := got[0]
	if e.Type != "item" || e.Source != SourceAgent {
		t.Errorf("type, source = %q, %q", e.Type, e.Source)
	}
	if e.Summary != "movie: First, rating 4.5, on 2026-10-09" {
		t.Errorf("summary = %q", e.Summary)
	}
	if !e.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("created_at = %v, want %v", e.CreatedAt, first.CreatedAt)
	}
}

func TestReviewQueueReturnsAnEmptyListNotNil(t *testing.T) {
	s, _ := newTestService(t)

	got, err := s.ReviewQueue(context.Background(), 0)
	if err != nil {
		t.Fatalf("ReviewQueue: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("queue = %#v, want an empty list that is not nil", got)
	}
}

func TestReviewQueueAppliesTheLimit(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	for _, title := range []string{"A", "B", "C"} {
		addUnverified(t, s, title)
	}

	got, err := s.ReviewQueue(ctx, 2)
	if err != nil {
		t.Fatalf("ReviewQueue: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("queue has %d entries, want 2", len(got))
	}

	for _, limit := range []int{-1, 201} {
		_, err := s.ReviewQueue(ctx, limit)
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != "limit" {
			t.Errorf("limit %d: error = %v, want a ValidationError for limit", limit, err)
		}
	}
}

func TestVerifyMarksTheRecordsAndAppendsEvents(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	a := addUnverified(t, s, "A")
	b := addUnverified(t, s, "B")
	done := addItem(t, s, "movie", "Verified already")

	n, err := s.Verify(ctx, []RecordRef{{"item", a.ID}, {"item", done.ID}, {"item", b.ID}}, SourceManual)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if n != 2 {
		t.Errorf("Verify changed %d records, want 2", n)
	}
	for _, id := range []int64{a.ID, b.ID} {
		item, err := s.GetItem(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if !item.Verified {
			t.Errorf("item %d is not verified", id)
		}
	}
	queue, err := s.ReviewQueue(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 0 {
		t.Errorf("the review queue still has %d entries", len(queue))
	}
	// Three creates, then one verify event for each changed record. The
	// record that was verified already gets no event.
	events := allEvents(t, pool)
	if len(events) != 5 {
		t.Fatalf("events = %d, want 5", len(events))
	}
	for i, id := range []int64{a.ID, b.ID} {
		e := events[3+i]
		if e.Action != "verify" || e.RecordID != id || e.Source != "manual" {
			t.Errorf("event = %+v, want a verify of item %d by manual", e, id)
		}
		if e.Old["verified"] != false || e.New["verified"] != true {
			t.Errorf("old, new verified = %v, %v", e.Old["verified"], e.New["verified"])
		}
	}
}

// One wrong ID in a batch must not leave the batch half done: Gautam
// would not know which rows count in a report.
func TestVerifyChangesNothingWhenOneRecordDoesNotExist(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	a := addUnverified(t, s, "A")
	deleted := addUnverified(t, s, "Deleted")
	if err := s.DeleteItem(ctx, deleted.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}
	eventsBefore := countRows(t, pool, "events")

	for name, id := range map[string]int64{"absent": 9999, "deleted": deleted.ID} {
		_, err := s.Verify(ctx, []RecordRef{{"item", a.ID}, {"item", id}}, SourceManual)

		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != "records" {
			t.Fatalf("%s: error = %v, want a ValidationError for records", name, err)
		}
		if !strings.Contains(invalid.Message, "does not exist") {
			t.Errorf("%s: message = %q, want it to name the record that does not exist", name, invalid.Message)
		}
	}

	item, err := s.GetItem(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Verified {
		t.Error("item A is verified although the batch failed")
	}
	if n := countRows(t, pool, "events"); n != eventsBefore {
		t.Errorf("events = %d, want %d: a failed batch appended an event", n, eventsBefore)
	}
}

func TestVerifyRejectsBadInput(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	a := addUnverified(t, s, "A")
	tooMany := make([]RecordRef, 201)
	for i := range tooMany {
		tooMany[i] = RecordRef{"item", a.ID}
	}

	tests := []struct {
		name      string
		refs      []RecordRef
		actor     Source
		wantField string
	}{
		{"no records", nil, SourceManual, "records"},
		{"too many records", tooMany, SourceManual, "records"},
		{"unknown type", []RecordRef{{"song", a.ID}}, SourceManual, "records"},
		{"unknown actor", []RecordRef{{"item", a.ID}}, "robot", "source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.Verify(ctx, tt.refs, tt.actor)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError for %s", err, tt.wantField)
			}
		})
	}
	item, err := s.GetItem(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Verified {
		t.Error("refused input verified the item")
	}
}
