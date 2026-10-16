package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGetReadsARecordByType(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added := addItem(t, s, "book", "Dune")

	got, err := s.Get(ctx, "item", added.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	item, ok := got.(Item)
	if !ok || item.ID != added.ID || item.Title != "Dune" {
		t.Errorf("Get = %#v, want the item", got)
	}

	if _, err := s.Get(ctx, "item", 9999); !errors.Is(err, ErrNotFound) {
		t.Errorf("absent record: error = %v, want ErrNotFound", err)
	}
}

func TestRecordFunctionsRejectAnUnknownType(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added := addItem(t, s, "book", "Dune")

	_, getErr := s.Get(ctx, "song", added.ID)
	_, updateErr := s.Update(ctx, "song", added.ID, UpdateParams{Fields: map[string]any{"title": "New"}, Actor: SourceManual})
	deleteErr := s.Delete(ctx, "", added.ID, DeleteParams{Actor: SourceManual})

	for name, err := range map[string]error{"Get": getErr, "Update": updateErr, "Delete": deleteErr} {
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != "type" {
			t.Errorf("%s: error = %v, want a ValidationError for type", name, err)
			continue
		}
		if !strings.Contains(invalid.Message, "item") {
			t.Errorf("%s: message = %q, want it to list the types", name, invalid.Message)
		}
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want 1 (the create)", n)
	}
}

func TestUpdateChangesTheNamedFields(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added, err := s.AddItem(ctx, dune())
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.Update(ctx, "item", added.ID, UpdateParams{
		Fields: map[string]any{"title": "Dune 2", "attrs": map[string]any{"year": 2024}},
		Clear:  []string{"rating"},
		Actor:  SourceAgent,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	item, ok := got.(Item)
	if !ok {
		t.Fatalf("Update returned %T, want an Item", got)
	}
	if item.Title != "Dune 2" || item.Rating != nil || item.OccurredOn != "2026-10-09" {
		t.Errorf("item = %+v", item)
	}
	if len(item.Attrs) != 1 || item.Attrs["year"] != float64(2024) {
		t.Errorf("attrs = %v", item.Attrs)
	}
	events := allEvents(t, pool)
	if last := events[len(events)-1]; last.Action != "update" || last.Source != "agent" {
		t.Errorf("last event = %+v, want an update by agent", last)
	}
}

// A model can invent a field, or try to approve its own row with
// "verified": true. Update must refuse the call and name the field.
func TestUpdateRejectsAFieldThatACallerCannotChange(t *testing.T) {
	tests := []struct {
		name        string
		fields      map[string]any
		wantField   string
		wantMessage string
	}{
		{"misspelled field", map[string]any{"ratting": 3}, "ratting", "is not a field of item"},
		{"verified", map[string]any{"verified": true}, "verified", "is not a field of item"},
		{"source", map[string]any{"source": "manual"}, "source", "is not a field of item"},
		{"id", map[string]any{"id": 7}, "id", "is not a field of item"},
		{"clear inside fields", map[string]any{"clear": []string{"rating"}}, "clear", "is not a field of item"},
		{"text for a number", map[string]any{"rating": "four"}, "rating", "must be a number"},
		{"number for text", map[string]any{"title": 5}, "title", "must be text"},
		{"text for an object", map[string]any{"attrs": "year 2024"}, "attrs", "must be an object"},
		{"good field and bad field", map[string]any{"title": "New", "verified": true}, "verified", "is not a field of item"},
	}
	s, pool := newTestService(t)
	ctx := context.Background()
	added, err := s.AddItem(ctx, dune())
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.Update(ctx, "item", added.ID, UpdateParams{Fields: tt.fields, Actor: SourceAgent})

			var invalid *ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("error = %v, want a ValidationError", err)
			}
			if invalid.Field != tt.wantField || !strings.Contains(invalid.Message, tt.wantMessage) {
				t.Errorf("error = %q, want field %q and message %q", err, tt.wantField, tt.wantMessage)
			}
		})
	}
	stored, err := s.GetItem(ctx, added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Title != "Dune: Part Two" || stored.Source != SourceAgent {
		t.Errorf("a refused update changed the row: %+v", stored)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want 1 (the create)", n)
	}
}

func TestDeleteDeletesARecordByType(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added := addItem(t, s, "book", "Dune")

	err := s.Delete(ctx, "item", added.ID, DeleteParams{Actor: SourceAgent})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "confirmed" {
		t.Fatalf("delete with no confirmation: error = %v, want a ValidationError for confirmed", err)
	}

	if err := s.Delete(ctx, "item", added.ID, DeleteParams{Actor: SourceAgent, Confirmed: true}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "item", added.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete: error = %v, want ErrNotFound", err)
	}
}

// A model or a Shortcut can send null to empty a field. Update must not
// accept that as a change that does nothing: it must say how to empty a
// field.
func TestUpdateRejectsANullValue(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added, err := s.AddItem(ctx, dune())
	if err != nil {
		t.Fatal(err)
	}

	for name, fields := range map[string]map[string]any{
		"only a null":           {"rating": nil},
		"a value and a null":    {"title": "New", "rating": nil},
		"null for a text field": {"title": nil},
	} {
		_, err := s.Update(ctx, "item", added.ID, UpdateParams{Fields: fields, Actor: SourceAgent})

		var invalid *ValidationError
		if !errors.As(err, &invalid) {
			t.Fatalf("%s: error = %v, want a ValidationError", name, err)
		}
		if invalid.Field == "" || !strings.Contains(invalid.Message, "clear") {
			t.Errorf("%s: error = %q, want it to name the field and to point to clear", name, err)
		}
	}
	stored, err := s.GetItem(ctx, added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Title != "Dune: Part Two" || stored.Rating == nil {
		t.Errorf("a refused update changed the row: %+v", stored)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want 1 (the create)", n)
	}
}
