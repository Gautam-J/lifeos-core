package service

import (
	"context"
	"errors"
	"testing"
)

func TestDeleteItemHidesTheItemAndKeepsTheRow(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	kept := addItem(t, s, "movie", "Kept")
	gone := addItem(t, s, "movie", "Gone")

	if err := s.DeleteItem(ctx, gone.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}

	if _, err := s.GetItem(ctx, gone.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetItem after delete: error = %v, want ErrNotFound", err)
	}
	items, err := s.ListItems(ctx, ListItemsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != kept.ID {
		t.Errorf("ListItems after delete = %v, want only the kept item", titles(items))
	}
	// The delete is soft: the row is still in the table.
	var deleted bool
	if err := pool.QueryRow(ctx, "SELECT deleted_at IS NOT NULL FROM catalog_items WHERE id = $1", gone.ID).Scan(&deleted); err != nil {
		t.Fatalf("the row is gone from the table: %v", err)
	}
	if !deleted {
		t.Error("deleted_at is not set")
	}
}

func TestDeleteItemAppendsADeleteEventWithTheOldValue(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added, err := s.AddItem(ctx, dune())
	if err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteItem(ctx, added.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}

	events := allEvents(t, pool)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (create, delete)", len(events))
	}
	e := events[1]
	if e.Action != "delete" || e.RecordType != "catalog_items" || e.RecordID != added.ID || e.Source != "manual" {
		t.Errorf("event = %+v", e)
	}
	if e.Old["title"] != "Dune: Part Two" || e.Old["rating"] != 4.5 {
		t.Errorf("old value = %v, want the deleted row", e.Old)
	}
	if e.New != nil {
		t.Errorf("new value = %v, want none", e.New)
	}
}

// A second delete of one row, or a delete of a row that never existed,
// must report not found and must not append an event.
func TestDeleteItemReturnsNotFound(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added := addItem(t, s, "movie", "Dune")
	if err := s.DeleteItem(ctx, added.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	for name, id := range map[string]int64{"deleted already": added.ID, "absent": 9999} {
		if err := s.DeleteItem(ctx, id, DeleteParams{Actor: SourceManual}); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", name, err)
		}
	}
	if n := countRows(t, pool, "events"); n != 2 {
		t.Errorf("events = %d, want 2 (create, one delete)", n)
	}
}

// A captured page can tell an agent to delete things. The service must
// refuse a delete from an agent unless Gautam confirmed it.
func TestDeleteItemNeedsAConfirmationFromAnAgent(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added := addItem(t, s, "movie", "Dune")

	err := s.DeleteItem(ctx, added.ID, DeleteParams{Actor: SourceAgent})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "confirmed" {
		t.Fatalf("error = %v, want a ValidationError for confirmed", err)
	}
	if _, err := s.GetItem(ctx, added.ID); err != nil {
		t.Errorf("the item is gone after a refused delete: %v", err)
	}

	if err := s.DeleteItem(ctx, added.ID, DeleteParams{Actor: SourceAgent, Confirmed: true}); err != nil {
		t.Errorf("confirmed delete: %v", err)
	}
}

func TestDeleteItemRejectsAnUnknownActor(t *testing.T) {
	s, _ := newTestService(t)
	added := addItem(t, s, "movie", "Dune")

	err := s.DeleteItem(context.Background(), added.ID, DeleteParams{})

	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "source" {
		t.Errorf("error = %v, want a ValidationError for source", err)
	}
}
