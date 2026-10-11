package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// addHabit stores one habit as the admin page does.
func addHabit(t *testing.T, s *Service, name string) Habit {
	t.Helper()
	habit, err := s.AddHabit(context.Background(), AddHabitParams{Name: name, Provenance: Provenance{Source: SourceManual, Verified: true}})
	if err != nil {
		t.Fatalf("AddHabit(%q): %v", name, err)
	}
	return habit
}

func habitNames(habits []Habit) []string {
	out := make([]string, len(habits))
	for i, h := range habits {
		out[i] = h.Name
	}
	return out
}

// A habit is logged by its name, from a form, from Siri and from an agent.
// One spelling in the table makes "Gym", "gym " and "GYM" the same habit.
func TestAddHabitStoresTheNameInLowerCase(t *testing.T) {
	s, pool := newTestService(t)

	got, err := s.AddHabit(context.Background(), AddHabitParams{Name: "  Morning   Walk ", Provenance: Provenance{Source: SourceManual, Verified: true}})
	if err != nil {
		t.Fatalf("AddHabit: %v", err)
	}

	if got.ID == 0 || got.Name != "morning walk" || !got.Active {
		t.Errorf("habit = %+v, want the name morning walk, active", got)
	}
	if got.Source != SourceManual || !got.Verified {
		t.Errorf("source, verified = %q, %v", got.Source, got.Verified)
	}
	events := allEvents(t, pool)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if e := events[0]; e.Action != "create" || e.RecordType != "habits" || e.RecordID != got.ID || e.Source != "manual" || e.New["name"] != "morning walk" {
		t.Errorf("event = %+v, want a create of the habit", e)
	}
}

func TestAddHabitRejectsBadInput(t *testing.T) {
	s, pool := newTestService(t)
	addHabit(t, s, "gym")
	tests := []struct {
		name      string
		habit     string
		source    Source
		wantField string
	}{
		{"empty name", "", SourceManual, "name"},
		{"name of spaces", " \t", SourceManual, "name"},
		{"name too long", strings.Repeat("a", 51), SourceManual, "name"},
		{"name with a NUL character", "a\x00b", SourceManual, "name"},
		{"name that exists", "gym", SourceManual, "name"},
		{"name that exists in a different case", " GYM", SourceManual, "name"},
		{"unknown source", "read", "robot", "source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.AddHabit(context.Background(), AddHabitParams{Name: tt.habit, Provenance: Provenance{Source: tt.source, Verified: true}})
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
	if n := countRows(t, pool, "habits"); n != 1 {
		t.Errorf("habits has %d rows after refused input, want 1", n)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d after refused input, want 1", n)
	}
}

func TestListHabitsReturnsEveryHabitByName(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	empty, err := s.ListHabits(ctx)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("ListHabits with no habit = %#v, %v, want an empty non-nil slice", empty, err)
	}

	addHabit(t, s, "read")
	gym := addHabit(t, s, "gym")
	addHabit(t, s, "meditate")
	if _, err := s.UpdateHabit(ctx, gym.ID, UpdateHabitParams{Active: ptr(false), Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	habits, err := s.ListHabits(ctx)
	if err != nil {
		t.Fatalf("ListHabits: %v", err)
	}
	if want := []string{"gym", "meditate", "read"}; !equal(habitNames(habits), want) {
		t.Errorf("names = %v, want %v", habitNames(habits), want)
	}
	if habits[0].Active || !habits[1].Active {
		t.Errorf("active = %v, %v, want false for gym and true for meditate", habits[0].Active, habits[1].Active)
	}
}

func TestUpdateHabitRenamesAndRetiresAHabit(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added := addHabit(t, s, "gymm")

	renamed, err := s.UpdateHabit(ctx, added.ID, UpdateHabitParams{Name: ptr(" Gym "), Actor: SourceManual})
	if err != nil || renamed.Name != "gym" || !renamed.Active {
		t.Fatalf("rename = %+v, %v, want the name gym, active", renamed, err)
	}
	retired, err := s.UpdateHabit(ctx, added.ID, UpdateHabitParams{Active: ptr(false), Actor: SourceManual})
	if err != nil || retired.Active || retired.Name != "gym" {
		t.Fatalf("retire = %+v, %v, want the name gym, not active", retired, err)
	}

	events := allEvents(t, pool)
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3 (create, two updates)", len(events))
	}
	if e := events[1]; e.Action != "update" || e.RecordType != "habits" || e.Source != "manual" || e.Old["name"] != "gymm" || e.New["name"] != "gym" {
		t.Errorf("event = %+v, want an update with the old and the new name", e)
	}

	// The same values again are not a write.
	if _, err := s.UpdateHabit(ctx, added.ID, UpdateHabitParams{Name: ptr("GYM"), Active: ptr(false), Actor: SourceManual}); err != nil {
		t.Errorf("update with no change: %v", err)
	}
	if n := countRows(t, pool, "events"); n != 3 {
		t.Errorf("events = %d after an update with no change, want 3", n)
	}
}

func TestUpdateHabitRejectsBadInput(t *testing.T) {
	s, pool := newTestService(t)
	gym := addHabit(t, s, "gym")
	addHabit(t, s, "read")
	tests := []struct {
		name      string
		params    UpdateHabitParams
		wantField string
	}{
		{"no field", UpdateHabitParams{Actor: SourceManual}, "fields"},
		{"empty name", UpdateHabitParams{Name: ptr(" "), Actor: SourceManual}, "name"},
		{"name of a different habit", UpdateHabitParams{Name: ptr("Read"), Actor: SourceManual}, "name"},
		{"unknown actor", UpdateHabitParams{Name: ptr("run"), Actor: "robot"}, "source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.UpdateHabit(context.Background(), gym.ID, tt.params)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
	if _, err := s.UpdateHabit(context.Background(), 9999, UpdateHabitParams{Name: ptr("run"), Actor: SourceManual}); !errors.Is(err, ErrNotFound) {
		t.Errorf("absent habit: error = %v, want ErrNotFound", err)
	}
	if n := countRows(t, pool, "events"); n != 2 {
		t.Errorf("events = %d, want 2 (the two creates)", n)
	}
}
