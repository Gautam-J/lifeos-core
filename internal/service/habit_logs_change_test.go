package service

import (
	"context"
	"errors"
	"testing"
)

func TestDeleteHabitLog(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	addHabit(t, s, "gym")
	added := logHabit(t, s, "gym", "")

	err := s.DeleteHabitLog(ctx, added.ID, DeleteParams{Actor: SourceAgent})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "confirmed" {
		t.Fatalf("delete by an agent with no confirmation: error = %v, want a ValidationError on confirmed", err)
	}

	if err := s.DeleteHabitLog(ctx, added.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("DeleteHabitLog: %v", err)
	}
	if _, err := s.GetHabitLog(ctx, added.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get after delete: error = %v, want ErrNotFound", err)
	}
	if logs, _ := s.ListHabitLogs(ctx, 0); len(logs) != 0 {
		t.Errorf("list after delete = %v, want it empty", logs)
	}
	if today, _ := s.HabitsToday(ctx); len(today.Habits) != 1 || today.Habits[0].Done {
		t.Errorf("checklist after delete = %+v, want gym not done", today)
	}
	if n := countRows(t, pool, "habit_logs"); n != 1 {
		t.Errorf("habit_logs has %d rows, want 1: a delete keeps the row", n)
	}
	events := allEvents(t, pool)
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3 (the habit, create, delete)", len(events))
	}
	if e := events[2]; e.Action != "delete" || e.RecordType != "habit_logs" || e.RecordID != added.ID || e.Source != "manual" || e.Old["habit"] != "gym" || e.New != nil {
		t.Errorf("event = %+v, want a delete with the old log", e)
	}

	if err := s.DeleteHabitLog(ctx, added.ID, DeleteParams{Actor: SourceManual}); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: error = %v, want ErrNotFound", err)
	}
}

// "I did not go to the gym" deletes the log. A log for the same day after
// that is a new log, not an error.
func TestLogHabitAfterADeleteStoresANewLog(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	addHabit(t, s, "gym")
	first := logHabit(t, s, "gym", "")
	if err := s.DeleteHabitLog(ctx, first.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	second := logHabit(t, s, "gym", "")

	if second.ID == first.ID {
		t.Errorf("the new log has the ID of the deleted log: %d", first.ID)
	}
	if n := countRows(t, pool, "habit_logs"); n != 2 {
		t.Errorf("habit_logs has %d rows, want 2 (one deleted, one live)", n)
	}
	if today, _ := s.HabitsToday(ctx); len(today.Habits) != 1 || !today.Habits[0].Done {
		t.Errorf("checklist = %+v, want gym done", today)
	}
}

func TestRecordFunctionsWorkOnAHabitLog(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	addHabit(t, s, "gym")
	added := logHabit(t, s, "gym", "")

	got, err := s.Get(ctx, "habit_log", added.ID)
	if log, ok := got.(HabitLog); err != nil || !ok || log != added {
		t.Fatalf("Get = %#v, %v, want the log", got, err)
	}

	// A log has no field to correct: the habit and the day are the log.
	_, err = s.Update(ctx, "habit_log", added.ID, UpdateParams{Fields: map[string]any{"logged_on": "2026-10-10"}, Actor: SourceManual})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "type" {
		t.Errorf("Update: error = %v, want a ValidationError on type", err)
	}
	if n := countRows(t, pool, "events"); n != 2 {
		t.Errorf("events = %d after the refused update, want 2", n)
	}

	if err := s.Delete(ctx, "habit_log", added.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "habit_log", added.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: error = %v, want ErrNotFound", err)
	}
}

func TestReviewQueueAndVerifyHandleAnInferredHabitLog(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	addHabit(t, s, "gym")
	item := addUnverified(t, s, "Dune")
	inferred := inferHabitLog(t, s, "gym", "2026-10-10")
	logHabit(t, s, "gym", "") // verified

	queue, err := s.ReviewQueue(ctx, 0)
	if err != nil {
		t.Fatalf("ReviewQueue: %v", err)
	}
	if len(queue) != 2 || queue[0].ID != item.ID {
		t.Fatalf("queue = %+v, want the item, then the log", queue)
	}
	if e := queue[1]; e.Type != "habit_log" || e.ID != inferred.ID || e.Summary != "habit: gym on 2026-10-10" || e.Source != SourceAgent {
		t.Errorf("entry = %+v", e)
	}

	n, err := s.Verify(ctx, []RecordRef{{"habit_log", inferred.ID}}, SourceManual)
	if err != nil || n != 1 {
		t.Fatalf("Verify = %d, %v, want 1", n, err)
	}
	if got, _ := s.GetHabitLog(ctx, inferred.ID); !got.Verified {
		t.Error("the log is not verified")
	}
	events := allEvents(t, pool)
	if e := events[len(events)-1]; e.Action != "verify" || e.RecordType != "habit_logs" || e.RecordID != inferred.ID || e.Old["verified"] != false || e.New["habit"] != "gym" {
		t.Errorf("event = %+v, want a verify of the log", e)
	}
}
