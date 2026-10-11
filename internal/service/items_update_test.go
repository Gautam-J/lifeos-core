package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// event is one row of the events table, with the JSON values decoded.
type event struct {
	Action, RecordType, Source string
	RecordID                   int64
	Old, New                   map[string]any
}

// allEvents returns every event, oldest first.
func allEvents(t *testing.T, pool *pgxpool.Pool) []event {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		"SELECT action, record_type, record_id, source, old_value, new_value FROM events ORDER BY id")
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	defer rows.Close()
	var events []event
	for rows.Next() {
		var e event
		var oldJSON, newJSON []byte
		if err := rows.Scan(&e.Action, &e.RecordType, &e.RecordID, &e.Source, &oldJSON, &newJSON); err != nil {
			t.Fatalf("scan event: %v", err)
		}
		if oldJSON != nil {
			if err := json.Unmarshal(oldJSON, &e.Old); err != nil {
				t.Fatalf("old value: %v", err)
			}
		}
		if newJSON != nil {
			if err := json.Unmarshal(newJSON, &e.New); err != nil {
				t.Fatalf("new value: %v", err)
			}
		}
		events = append(events, e)
	}
	return events
}

// A correction such as "that was 3 stars, not 4" names one field. The other
// fields must stay as they are.
func TestUpdateItemChangesOnlyTheGivenFields(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added, err := s.AddItem(ctx, dune())
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.UpdateItem(ctx, added.ID, UpdateItemParams{Rating: ptr(3.0), Actor: SourceManual})
	if err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}

	if got.Rating == nil || *got.Rating != 3 {
		t.Errorf("rating = %v, want 3", got.Rating)
	}
	if got.ID != added.ID || got.Kind != "movie" || got.Title != "Dune: Part Two" || got.OccurredOn != "2026-10-09" {
		t.Errorf("the update changed a field that was not given: %+v", got)
	}
	if got.Attrs["director"] != "Denis Villeneuve" || got.Attrs["year"] != float64(2024) {
		t.Errorf("attrs = %v, want the stored attrs", got.Attrs)
	}
	if got.Source != SourceAgent || !got.Verified {
		t.Errorf("source, verified = %q, %v; want the stored values", got.Source, got.Verified)
	}
	stored, err := s.GetItem(ctx, added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Rating == nil || *stored.Rating != 3 || stored.Title != "Dune: Part Two" {
		t.Errorf("stored item = %+v", stored)
	}
}

func TestUpdateItemChangesEveryFieldThatIsGiven(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added, err := s.AddItem(ctx, dune())
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.UpdateItem(ctx, added.ID, UpdateItemParams{
		Kind:       ptr("book"),
		Title:      ptr("  Dune  "),
		Rating:     ptr(5.0),
		OccurredOn: ptr("2026-10-11"), // today in IST
		Attrs:      map[string]any{"author": "Frank Herbert"},
		Actor:      SourceManual,
	})
	if err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}

	if got.Kind != "book" || got.Title != "Dune" || *got.Rating != 5 || got.OccurredOn != "2026-10-11" {
		t.Errorf("item = %+v", got)
	}
	if len(got.Attrs) != 1 || got.Attrs["author"] != "Frank Herbert" {
		t.Errorf("attrs = %v, want only the new attrs", got.Attrs)
	}
}

func TestUpdateItemClearsAnOptionalField(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added, err := s.AddItem(ctx, dune())
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.UpdateItem(ctx, added.ID, UpdateItemParams{Clear: []string{"rating", "occurred_on"}, Actor: SourceManual})
	if err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}

	if got.Rating != nil || got.OccurredOn != "" {
		t.Errorf("rating, occurred_on = %v, %q; want both empty", got.Rating, got.OccurredOn)
	}
	if got.Title != "Dune: Part Two" {
		t.Errorf("title = %q, want the stored title", got.Title)
	}
}

func TestUpdateItemAppendsAnUpdateEventWithTheOldValue(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added, err := s.AddItem(ctx, dune())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.UpdateItem(ctx, added.ID, UpdateItemParams{Rating: ptr(3.0), Actor: SourceManual}); err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}

	events := allEvents(t, pool)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (create, update)", len(events))
	}
	e := events[1]
	if e.Action != "update" || e.RecordType != "catalog_items" || e.RecordID != added.ID {
		t.Errorf("event = %+v", e)
	}
	// The row came from an agent. The event names who changed it.
	if e.Source != "manual" {
		t.Errorf("event source = %q, want manual", e.Source)
	}
	if e.Old["rating"] != 4.5 || e.New["rating"] != float64(3) {
		t.Errorf("old, new rating = %v, %v; want 4.5, 3", e.Old["rating"], e.New["rating"])
	}
	if e.Old["title"] != "Dune: Part Two" || e.New["title"] != "Dune: Part Two" {
		t.Errorf("old, new title = %v, %v", e.Old["title"], e.New["title"])
	}
}

func TestUpdateItemRejectsBadInput(t *testing.T) {
	tests := []struct {
		name      string
		params    UpdateItemParams
		wantField string
	}{
		{"rating above 5", UpdateItemParams{Rating: ptr(7.0)}, "rating"},
		{"empty title", UpdateItemParams{Title: ptr("   ")}, "title"},
		{"unknown kind", UpdateItemParams{Kind: ptr("song")}, "kind"},
		{"date in the future", UpdateItemParams{OccurredOn: ptr("2026-10-12")}, "occurred_on"},
		{"date that is not a date", UpdateItemParams{OccurredOn: ptr("yesterday")}, "occurred_on"},
		{"empty date", UpdateItemParams{OccurredOn: ptr("")}, "occurred_on"},
		{"clear of a required field", UpdateItemParams{Clear: []string{"title"}}, "clear"},
		{"rating set and cleared", UpdateItemParams{Rating: ptr(3.0), Clear: []string{"rating"}}, "rating"},
		{"date set and cleared", UpdateItemParams{OccurredOn: ptr("2026-10-01"), Clear: []string{"occurred_on"}}, "occurred_on"},
	}
	s, pool := newTestService(t)
	ctx := context.Background()
	added, err := s.AddItem(ctx, dune())
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.params.Actor = SourceManual
			_, err := s.UpdateItem(ctx, added.ID, tt.params)

			var invalid *ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("error = %v, want a ValidationError", err)
			}
			if invalid.Field != tt.wantField {
				t.Errorf("field = %q, want %q (%v)", invalid.Field, tt.wantField, err)
			}
		})
	}

	t.Run("no actor", func(t *testing.T) {
		_, err := s.UpdateItem(ctx, added.ID, UpdateItemParams{Rating: ptr(3.0)})
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != "source" {
			t.Errorf("error = %v, want a ValidationError for source", err)
		}
	})

	stored, err := s.GetItem(ctx, added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if *stored.Rating != 4.5 || stored.Title != "Dune: Part Two" || stored.OccurredOn != "2026-10-09" || !stored.UpdatedAt.Equal(added.UpdatedAt) {
		t.Errorf("a refused update changed the row: %+v", stored)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d after refused updates, want 1 (the create)", n)
	}
}

func TestUpdateItemReturnsNotFound(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	deleted := addItem(t, s, "movie", "Deleted")
	if _, err := pool.Exec(ctx, "UPDATE catalog_items SET deleted_at = now() WHERE id = $1", deleted.ID); err != nil {
		t.Fatal(err)
	}

	for name, id := range map[string]int64{"absent": 9999, "deleted": deleted.ID, "zero": 0} {
		_, err := s.UpdateItem(ctx, id, UpdateItemParams{Title: ptr("New"), Actor: SourceManual})
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", name, err)
		}
	}
	var title string
	if err := pool.QueryRow(ctx, "SELECT title FROM catalog_items WHERE id = $1", deleted.ID).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "Deleted" {
		t.Errorf("title of the deleted row = %q, want no change", title)
	}
}

// A change that an agent inferred is not Gautam's word. It must not
// replace a value that he approved: the service refuses it, and the agent
// must ask him. No update approves a row.
func TestUpdateItemRefusesAnInferredChangeToAVerifiedRow(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	verified, err := s.AddItem(ctx, dune())
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.UpdateItem(ctx, verified.ID, UpdateItemParams{Rating: ptr(3.0), Actor: SourceAgent, Inferred: true})

	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "dictated" {
		t.Fatalf("error = %v, want a ValidationError for dictated", err)
	}
	stored, err := s.GetItem(ctx, verified.ID)
	if err != nil {
		t.Fatal(err)
	}
	if *stored.Rating != 4.5 || !stored.Verified || !stored.UpdatedAt.Equal(verified.UpdatedAt) {
		t.Errorf("a refused update changed the row: %+v", stored)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want 1 (the create)", n)
	}

	// A change that Gautam stated goes through, and the row stays verified.
	stated, err := s.UpdateItem(ctx, verified.ID, UpdateItemParams{Rating: ptr(3.0), Actor: SourceAgent})
	if err != nil {
		t.Fatalf("stated change: %v", err)
	}
	if *stated.Rating != 3 || !stated.Verified {
		t.Errorf("stated change: rating, verified = %v, %v; want 3, true", *stated.Rating, stated.Verified)
	}
}

// A row that waits for review is the agent's own guess. The agent can
// correct it, with or without Gautam's words, and the row keeps waiting.
func TestUpdateItemKeepsAnUnverifiedRowUnverified(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	p := dune()
	p.Verified = false
	added, err := s.AddItem(ctx, p)
	if err != nil {
		t.Fatal(err)
	}

	for _, inferred := range []bool{true, false} {
		rating := 1.0
		if inferred {
			rating = 2.0
		}
		got, err := s.UpdateItem(ctx, added.ID, UpdateItemParams{Rating: &rating, Actor: SourceAgent, Inferred: inferred})
		if err != nil {
			t.Fatalf("inferred %v: UpdateItem: %v", inferred, err)
		}
		if *got.Rating != rating || got.Verified {
			t.Errorf("inferred %v: rating, verified = %v, %v; want %v, false", inferred, *got.Rating, got.Verified, rating)
		}
	}
}

// A Shortcut and an agent can change one row at the same moment. Each
// change names a different field, and the two changes must stay.
func TestUpdateItemKeepsAChangeThatAnotherWriterMakesAtTheSameTime(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added, err := s.AddItem(ctx, dune())
	if err != nil {
		t.Fatal(err)
	}

	// The other writer locks the row and changes the rating.
	other, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Rollback(ctx)
	if _, err := other.Exec(ctx, "UPDATE catalog_items SET rating = 2 WHERE id = $1", added.ID); err != nil {
		t.Fatal(err)
	}

	// UpdateItem changes the title while the other writer holds the row.
	done := make(chan error, 1)
	go func() {
		_, err := s.UpdateItem(ctx, added.ID, UpdateItemParams{Title: ptr("Dune 2"), Actor: SourceManual})
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("UpdateItem returned (%v) while a different writer held the row", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := other.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}

	stored, err := s.GetItem(ctx, added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Title != "Dune 2" {
		t.Errorf("title = %q, want Dune 2", stored.Title)
	}
	if stored.Rating == nil || *stored.Rating != 2 {
		t.Error("rating is not 2: the update wrote the old rating back")
	}
}

func TestUpdateItemReportsADatabaseFailure(t *testing.T) {
	s, pool := newTestService(t)
	pool.Close()

	_, err := s.UpdateItem(context.Background(), 1, UpdateItemParams{Title: ptr("New"), Actor: SourceManual})

	var invalid *ValidationError
	if err == nil || errors.As(err, &invalid) || errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want a plain error", err)
	}
	if !strings.HasPrefix(err.Error(), "update item:") {
		t.Errorf("error = %q, want the prefix \"update item:\"", err)
	}
}

// "Remove the rating" with no field named must not report success. It
// must also not take a verified row out of the reports.
func TestUpdateItemRejectsAnUpdateThatNamesNoField(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added, err := s.AddItem(ctx, dune())
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.UpdateItem(ctx, added.ID, UpdateItemParams{Actor: SourceAgent, Inferred: true})

	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "fields" {
		t.Fatalf("error = %v, want a ValidationError for fields", err)
	}
	stored, err := s.GetItem(ctx, added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.Verified || !stored.UpdatedAt.Equal(added.UpdatedAt) {
		t.Errorf("a refused update changed the row: %+v", stored)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want 1 (the create)", n)
	}
}

// A Save on the edit form with no change, or an agent that sends the
// stored values again, is not a write: no event, no new updated_at, and
// the row stays verified.
func TestUpdateItemWritesNothingWhenNoValueChanges(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added, err := s.AddItem(ctx, dune())
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.UpdateItem(ctx, added.ID, UpdateItemParams{
		Kind:       ptr("movie"),
		Title:      ptr("  Dune: Part Two "),
		Rating:     ptr(4.5),
		OccurredOn: ptr("2026-10-09"),
		Actor:      SourceAgent,
		Inferred:   true,
	})
	if err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}

	if !got.Verified || !got.UpdatedAt.Equal(added.UpdatedAt) || got.Title != "Dune: Part Two" {
		t.Errorf("an update with no change wrote the row: %+v", got)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want 1 (the create)", n)
	}

	// The same holds for a clear of a field that is empty already.
	if _, err := s.UpdateItem(ctx, added.ID, UpdateItemParams{Clear: []string{"rating"}, Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateItem(ctx, added.ID, UpdateItemParams{Clear: []string{"rating"}, Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, pool, "events"); n != 2 {
		t.Errorf("events = %d, want 2 (create, one update)", n)
	}
}
