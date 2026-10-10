package service

import (
	"context"
	"errors"
	"testing"
)

func addItem(t *testing.T, s *Service, kind, title string) Item {
	t.Helper()
	item, err := s.AddItem(context.Background(), AddItemParams{
		Kind:       kind,
		Title:      title,
		Provenance: Provenance{Source: SourceManual, Verified: true},
	})
	if err != nil {
		t.Fatalf("AddItem %q: %v", title, err)
	}
	return item
}

func titles(items []Item) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Title
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGetItemReturnsTheStoredItem(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added, err := s.AddItem(ctx, dune())
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	got, err := s.GetItem(ctx, added.ID)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if got.ID != added.ID || got.Title != "Dune: Part Two" || got.OccurredOn != "2026-10-09" {
		t.Errorf("GetItem = %+v", got)
	}
	if got.Rating == nil || *got.Rating != 4.5 || got.Attrs["year"] != float64(2024) {
		t.Errorf("rating, attrs = %v, %v", got.Rating, got.Attrs)
	}
}

func TestGetItemReturnsNotFound(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	deleted := addItem(t, s, "book", "Deleted book")
	if _, err := pool.Exec(ctx, "UPDATE catalog_items SET deleted_at = now() WHERE id = $1", deleted.ID); err != nil {
		t.Fatal(err)
	}

	for name, id := range map[string]int64{"unknown ID": 9999, "zero": 0, "negative": -1, "deleted item": deleted.ID} {
		t.Run(name, func(t *testing.T) {
			if _, err := s.GetItem(ctx, id); !errors.Is(err, ErrNotFound) {
				t.Errorf("GetItem(%d) error = %v, want ErrNotFound", id, err)
			}
		})
	}
}

func TestListItemsReturnsTheNewestFirst(t *testing.T) {
	s, _ := newTestService(t)
	addItem(t, s, "book", "First")
	addItem(t, s, "movie", "Second")
	addItem(t, s, "book", "Third")

	items, err := s.ListItems(context.Background(), ListItemsParams{})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if want := []string{"Third", "Second", "First"}; !equal(titles(items), want) {
		t.Errorf("titles = %v, want %v", titles(items), want)
	}
}

func TestListItemsFiltersByKind(t *testing.T) {
	s, _ := newTestService(t)
	addItem(t, s, "book", "First")
	addItem(t, s, "movie", "Second")
	addItem(t, s, "book", "Third")

	items, err := s.ListItems(context.Background(), ListItemsParams{Kind: "book"})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if want := []string{"Third", "First"}; !equal(titles(items), want) {
		t.Errorf("titles = %v, want %v", titles(items), want)
	}
}

func TestListItemsLeavesOutDeletedItems(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	addItem(t, s, "book", "Kept")
	deleted := addItem(t, s, "book", "Deleted")
	if _, err := pool.Exec(ctx, "UPDATE catalog_items SET deleted_at = now() WHERE id = $1", deleted.ID); err != nil {
		t.Fatal(err)
	}

	items, err := s.ListItems(ctx, ListItemsParams{})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if want := []string{"Kept"}; !equal(titles(items), want) {
		t.Errorf("titles = %v, want %v", titles(items), want)
	}
}

func TestListItemsAppliesTheLimit(t *testing.T) {
	s, _ := newTestService(t)
	addItem(t, s, "book", "First")
	addItem(t, s, "book", "Second")
	addItem(t, s, "book", "Third")

	items, err := s.ListItems(context.Background(), ListItemsParams{Limit: 2})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if want := []string{"Third", "Second"}; !equal(titles(items), want) {
		t.Errorf("titles = %v, want %v", titles(items), want)
	}
}

func TestListItemsReturnsAnEmptyListNotNil(t *testing.T) {
	s, _ := newTestService(t)

	items, err := s.ListItems(context.Background(), ListItemsParams{})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if items == nil || len(items) != 0 {
		t.Errorf("items = %#v, want an empty non-nil slice so JSON shows []", items)
	}
}

func TestListItemsRejectsBadInput(t *testing.T) {
	s, _ := newTestService(t)
	tests := []struct {
		name      string
		params    ListItemsParams
		wantField string
	}{
		{"unknown kind", ListItemsParams{Kind: "film"}, "kind"},
		{"limit above the maximum", ListItemsParams{Limit: 201}, "limit"},
		{"negative limit", ListItemsParams{Limit: -1}, "limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.ListItems(context.Background(), tt.params)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
}
