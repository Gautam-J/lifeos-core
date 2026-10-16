package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

// testNow is 2026-10-10 20:00 UTC, which is 2026-10-11 01:30 in IST.
var testNow = time.Date(2026, 10, 10, 20, 0, 0, 0, time.UTC)

func newTestService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.New(t)
	s := New(pool)
	s.now = func() time.Time { return testNow }
	return s, pool
}

func ptr[T any](v T) *T { return &v }

// dune is a valid input. Tests copy it and change one field.
func dune() AddItemParams {
	return AddItemParams{
		Kind:       "movie",
		Title:      "Dune: Part Two",
		Rating:     ptr(4.5),
		OccurredOn: "2026-10-09",
		Attrs:      map[string]any{"director": "Denis Villeneuve", "year": 2024},
		Provenance: Provenance{Source: SourceAgent},
	}
}

func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestAddItemStoresTheItem(t *testing.T) {
	s, pool := newTestService(t)

	got, err := s.AddItem(context.Background(), dune())
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	if got.ID == 0 {
		t.Error("AddItem returned ID 0")
	}
	if got.Kind != "movie" || got.Title != "Dune: Part Two" {
		t.Errorf("kind, title = %q, %q", got.Kind, got.Title)
	}
	if got.Rating == nil || *got.Rating != 4.5 {
		t.Errorf("rating = %v, want 4.5", got.Rating)
	}
	if got.OccurredOn != "2026-10-09" {
		t.Errorf("occurred_on = %q, want 2026-10-09", got.OccurredOn)
	}
	if got.Attrs["director"] != "Denis Villeneuve" || got.Attrs["year"] != float64(2024) {
		t.Errorf("attrs = %v", got.Attrs)
	}
	if got.Source != SourceAgent {
		t.Errorf("source = %q", got.Source)
	}
	if got.CreatedAt.IsZero() || !got.CreatedAt.Equal(got.UpdatedAt) {
		t.Errorf("created_at, updated_at = %v, %v", got.CreatedAt, got.UpdatedAt)
	}
	if got.CreatedAt.Location() != time.UTC {
		t.Errorf("created_at is in %v, want UTC", got.CreatedAt.Location())
	}
	if n := countRows(t, pool, "catalog_items"); n != 1 {
		t.Errorf("catalog_items has %d rows, want 1", n)
	}
}

func TestAddItemAcceptsTheMinimumInput(t *testing.T) {
	s, _ := newTestService(t)

	item, err := s.AddItem(context.Background(), AddItemParams{
		Kind:       "book",
		Title:      "  Dune  ",
		Provenance: Provenance{Source: SourceAgent},
	})
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if item.Title != "Dune" {
		t.Errorf("title = %q, want the spaces trimmed", item.Title)
	}
	if item.Rating != nil || item.OccurredOn != "" {
		t.Errorf("rating, occurred_on = %v, %q, want none", item.Rating, item.OccurredOn)
	}
	if item.Attrs == nil || len(item.Attrs) != 0 {
		t.Errorf("attrs = %v, want an empty object", item.Attrs)
	}
}

func TestAddItemAppendsACreateEvent(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	item, err := s.AddItem(ctx, dune())
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	var (
		action, recordType, source string
		recordID                   int64
		hasOld                     bool
		newTitle                   string
	)
	err = pool.QueryRow(ctx, `
		SELECT action, record_type, record_id, source, old_value IS NOT NULL, new_value->>'title'
		FROM events`).Scan(&action, &recordType, &recordID, &source, &hasOld, &newTitle)
	if err != nil {
		t.Fatalf("read the event (want exactly one row): %v", err)
	}
	if action != "create" || recordType != "catalog_items" || recordID != item.ID {
		t.Errorf("event = %s %s %d, want create catalog_items %d", action, recordType, recordID, item.ID)
	}
	if source != "agent" {
		t.Errorf("event source = %q, want agent", source)
	}
	if hasOld {
		t.Error("a create event has an old value")
	}
	if newTitle != "Dune: Part Two" {
		t.Errorf("event new_value title = %q", newTitle)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events has %d rows, want 1", n)
	}
}

func TestAddItemRejectsBadInput(t *testing.T) {
	tests := []struct {
		name      string
		change    func(*AddItemParams)
		wantField string
	}{
		{"no kind", func(p *AddItemParams) { p.Kind = "" }, "kind"},
		{"unknown kind", func(p *AddItemParams) { p.Kind = "film" }, "kind"},
		{"empty title", func(p *AddItemParams) { p.Title = "" }, "title"},
		{"title of spaces", func(p *AddItemParams) { p.Title = " \t\n" }, "title"},
		{"title too long", func(p *AddItemParams) { p.Title = strings.Repeat("a", 301) }, "title"},
		{"rating above 5", func(p *AddItemParams) { p.Rating = ptr(7.0) }, "rating"},
		{"rating below 0", func(p *AddItemParams) { p.Rating = ptr(-0.5) }, "rating"},
		{"rating not a half step", func(p *AddItemParams) { p.Rating = ptr(4.25) }, "rating"},
		{"rating not a number", func(p *AddItemParams) { p.Rating = ptr(math.NaN()) }, "rating"},
		{"date in the wrong format", func(p *AddItemParams) { p.OccurredOn = "09-10-2026" }, "occurred_on"},
		{"date that does not exist", func(p *AddItemParams) { p.OccurredOn = "2026-02-30" }, "occurred_on"},
		{"date in the future", func(p *AddItemParams) { p.OccurredOn = "2099-01-01" }, "occurred_on"},
		{"unknown source", func(p *AddItemParams) { p.Source = "robot" }, "source"},
		{"no source", func(p *AddItemParams) { p.Source = "" }, "source"},
	}

	s, pool := newTestService(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := dune()
			tt.change(&p)

			_, err := s.AddItem(context.Background(), p)

			var invalid *ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("error = %v, want a *ValidationError", err)
			}
			if invalid.Field != tt.wantField {
				t.Errorf("field = %q, want %q (message: %s)", invalid.Field, tt.wantField, invalid.Message)
			}
		})
	}
	if n := countRows(t, pool, "catalog_items"); n != 0 {
		t.Errorf("catalog_items has %d rows after rejected input, want 0", n)
	}
	if n := countRows(t, pool, "events"); n != 0 {
		t.Errorf("events has %d rows after rejected input, want 0", n)
	}
}

func TestAddItemAcceptsATitleOfTheMaximumLength(t *testing.T) {
	s, _ := newTestService(t)
	p := dune()
	p.Title = strings.Repeat("é", 300) // 300 characters, 600 bytes

	if _, err := s.AddItem(context.Background(), p); err != nil {
		t.Errorf("AddItem with a 300-character title: %v", err)
	}
}

// Between 00:00 and 05:30 IST the UTC date is one day behind. "Today" for
// Gautam is the IST date, so the service must accept it.
func TestAddItemUsesTheISTDateAsToday(t *testing.T) {
	s, _ := newTestService(t) // the clock is 2026-10-10 20:00 UTC = 2026-10-11 01:30 IST
	ctx := context.Background()

	today := dune()
	today.OccurredOn = "2026-10-11"
	if _, err := s.AddItem(ctx, today); err != nil {
		t.Errorf("AddItem for today in IST: %v", err)
	}

	tomorrow := dune()
	tomorrow.OccurredOn = "2026-10-12"
	var invalid *ValidationError
	if _, err := s.AddItem(ctx, tomorrow); !errors.As(err, &invalid) || invalid.Field != "occurred_on" {
		t.Errorf("AddItem for tomorrow in IST: error = %v, want a ValidationError on occurred_on", err)
	}
}

func TestAddItemStoresNothingWhenTheEventFails(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "DROP TABLE events"); err != nil {
		t.Fatal(err)
	}

	if _, err := s.AddItem(ctx, dune()); err == nil {
		t.Fatal("AddItem returned no error although the event insert must fail")
	}
	if n := countRows(t, pool, "catalog_items"); n != 0 {
		t.Errorf("catalog_items has %d rows, want 0: the item must roll back with its event", n)
	}
}

func TestAddItemReportsADatabaseFailure(t *testing.T) {
	s, pool := newTestService(t)
	pool.Close()

	_, err := s.AddItem(context.Background(), dune())

	if err == nil {
		t.Fatal("AddItem returned no error with a closed pool")
	}
	var invalid *ValidationError
	if errors.As(err, &invalid) {
		t.Errorf("a database failure came back as a ValidationError: %v", err)
	}
}
