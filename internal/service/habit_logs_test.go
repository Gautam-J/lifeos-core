package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// logHabit logs a habit as a Shortcut or the admin page does: manual. An
// empty day means today in IST.
func logHabit(t *testing.T, s *Service, habit, day string) HabitLog {
	t.Helper()
	log, err := s.LogHabit(context.Background(), LogHabitParams{
		Habit: habit, LoggedOn: day, Provenance: Provenance{Source: SourceManual},
	})
	if err != nil {
		t.Fatalf("LogHabit(%q, %q): %v", habit, day, err)
	}
	return log
}

// inferHabitLog logs a habit as an agent does when it reads "went to the
// gym" in a journal entry.
func inferHabitLog(t *testing.T, s *Service, habit, day string) HabitLog {
	t.Helper()
	log, err := s.LogHabit(context.Background(), LogHabitParams{
		Habit: habit, LoggedOn: day, Provenance: Provenance{Source: SourceAgent},
	})
	if err != nil {
		t.Fatalf("LogHabit(%q, %q): %v", habit, day, err)
	}
	return log
}

func TestLogHabitStoresALogForTodayInIST(t *testing.T) {
	s, pool := newTestService(t) // the clock is 2026-10-10 20:00 UTC = 2026-10-11 01:30 IST
	gym := addHabit(t, s, "gym")

	// Siri hears "Gym". The name in the table is gym.
	got := logHabit(t, s, " Gym ", "")

	if got.ID == 0 || got.HabitID != gym.ID || got.Habit != "gym" || got.LoggedOn != "2026-10-11" {
		t.Errorf("log = %+v, want the habit gym on 2026-10-11", got)
	}
	if got.Source != SourceManual {
		t.Errorf("source = %q", got.Source)
	}
	events := allEvents(t, pool)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (the habit, the log)", len(events))
	}
	if e := events[1]; e.Action != "create" || e.RecordType != "habit_logs" || e.RecordID != got.ID || e.Source != "manual" || e.New["habit"] != "gym" || e.New["logged_on"] != "2026-10-11" {
		t.Errorf("event = %+v, want a create of the log", e)
	}
}

func TestLogHabitAcceptsADayInThePastAndRefusesTomorrow(t *testing.T) {
	s, _ := newTestService(t)
	addHabit(t, s, "gym")

	if got := logHabit(t, s, "gym", "2026-10-09"); got.LoggedOn != "2026-10-09" {
		t.Errorf("logged_on = %q, want 2026-10-09", got.LoggedOn)
	}
	_, err := s.LogHabit(context.Background(), LogHabitParams{Habit: "gym", LoggedOn: "2026-10-12", Provenance: Provenance{Source: SourceManual}})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "logged_on" {
		t.Errorf("log for tomorrow in IST: error = %v, want a ValidationError on logged_on", err)
	}
}

// "Log gym" two times on one day is one visit to the gym. The second call
// must not fail, must not store a second row and must not append an event.
func TestLogHabitASecondTimeChangesNothing(t *testing.T) {
	s, pool := newTestService(t)
	addHabit(t, s, "gym")
	first := logHabit(t, s, "gym", "")

	second := logHabit(t, s, "gym", "2026-10-11")

	if second != first {
		t.Errorf("second log = %+v, want the first log %+v", second, first)
	}
	if n := countRows(t, pool, "habit_logs"); n != 1 {
		t.Errorf("habit_logs has %d rows, want 1", n)
	}
	if n := countRows(t, pool, "events"); n != 2 {
		t.Errorf("events = %d, want 2 (the habit, one log)", n)
	}
	if other := logHabit(t, s, "gym", "2026-10-10"); other.ID == first.ID {
		t.Error("a log for a different day returned the first log")
	}
}

func TestLogHabitRejectsBadInput(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	manual := Provenance{Source: SourceManual}

	_, err := s.LogHabit(ctx, LogHabitParams{Habit: "gym", Provenance: manual})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "habit" || !strings.Contains(invalid.Message, "there is no active habit") {
		t.Errorf("no habit in the table: error = %v, want a ValidationError on habit that says so", err)
	}

	addHabit(t, s, "gym")
	addHabit(t, s, "read")
	retired := addHabit(t, s, "run")
	if _, err := s.UpdateHabit(ctx, retired.ID, UpdateHabitParams{Active: ptr(false), Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		params      LogHabitParams
		wantField   string
		wantMessage string
	}{
		{"empty habit", LogHabitParams{Habit: " ", Provenance: manual}, "habit", "must not be empty"},
		// The error gives the names, so that an agent can ask Gautam which one he means.
		{"habit that is not in the table", LogHabitParams{Habit: "gymm", Provenance: manual}, "habit", `no active habit has the name "gymm"; the active habits are: gym, read`},
		{"habit that is not active", LogHabitParams{Habit: "run", Provenance: manual}, "habit", `no active habit has the name "run"`},
		{"date that is not a date", LogHabitParams{Habit: "gym", LoggedOn: "today", Provenance: manual}, "logged_on", "YYYY-MM-DD"},
		{"unknown source", LogHabitParams{Habit: "gym", Provenance: Provenance{Source: "robot"}}, "source", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.LogHabit(ctx, tt.params)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Fatalf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
			if !strings.Contains(invalid.Message, tt.wantMessage) {
				t.Errorf("message = %q, want it to contain %q", invalid.Message, tt.wantMessage)
			}
		})
	}
	if n := countRows(t, pool, "habit_logs"); n != 0 {
		t.Errorf("habit_logs has %d rows after refused input, want 0", n)
	}
}

// A Shortcut that Siri runs two times, or a double tap, sends two requests
// at the same moment. One row must be stored, and no request must fail.
func TestLogHabitAtTheSameMomentStoresOneLog(t *testing.T) {
	s, pool := newTestService(t)
	addHabit(t, s, "gym")

	const callers = 8
	ids := make([]int64, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log, err := s.LogHabit(context.Background(), LogHabitParams{Habit: "gym", Provenance: Provenance{Source: SourceManual}})
			ids[i], errs[i] = log.ID, err
		}()
	}
	wg.Wait()

	for i := range callers {
		if errs[i] != nil {
			t.Errorf("caller %d: %v", i, errs[i])
		}
		if ids[i] != ids[0] {
			t.Errorf("caller %d got the log %d, caller 0 got %d", i, ids[i], ids[0])
		}
	}
	if n := countRows(t, pool, "habit_logs"); n != 1 {
		t.Errorf("habit_logs has %d rows, want 1", n)
	}
	if n := countRows(t, pool, "events"); n != 2 {
		t.Errorf("events = %d, want 2 (the habit, one log)", n)
	}
}

func TestGetHabitLog(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	addHabit(t, s, "gym")
	added := logHabit(t, s, "gym", "")

	got, err := s.GetHabitLog(ctx, added.ID)
	if err != nil || got != added {
		t.Errorf("GetHabitLog = %+v, %v, want %+v", got, err, added)
	}
	if _, err := s.GetHabitLog(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Errorf("absent log: error = %v, want ErrNotFound", err)
	}
}

func TestListHabitLogsReturnsTheNewestDayFirst(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	empty, err := s.ListHabitLogs(ctx, 0)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("ListHabitLogs with no log = %#v, %v, want an empty non-nil slice", empty, err)
	}

	addHabit(t, s, "gym")
	addHabit(t, s, "read")
	logHabit(t, s, "gym", "2026-10-09")
	logHabit(t, s, "gym", "2026-10-11")
	logHabit(t, s, "read", "2026-10-10")

	logs, err := s.ListHabitLogs(ctx, 2)
	if err != nil {
		t.Fatalf("ListHabitLogs: %v", err)
	}
	if len(logs) != 2 || logs[0].Habit != "gym" || logs[0].LoggedOn != "2026-10-11" || logs[1].Habit != "read" || logs[1].LoggedOn != "2026-10-10" {
		t.Errorf("logs = %+v, want gym on 2026-10-11, then read on 2026-10-10", logs)
	}
	if _, err := s.ListHabitLogs(ctx, 201); err == nil {
		t.Error("ListHabitLogs accepted a limit of 201")
	}
}

// The checklist is what the Kindle and the admin page show. A live log
// makes the habit done, also a log that an agent wrote.
func TestHabitsTodayShowsTheActiveHabitsAndTheirLogs(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	gym := addHabit(t, s, "gym")
	addHabit(t, s, "read")
	addHabit(t, s, "walk")
	retired := addHabit(t, s, "run")
	logHabit(t, s, "gym", "")
	inferHabitLog(t, s, "read", "")
	logHabit(t, s, "walk", "2026-10-10") // yesterday in IST
	logHabit(t, s, "run", "")
	if _, err := s.UpdateHabit(ctx, retired.ID, UpdateHabitParams{Active: ptr(false), Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	got, err := s.HabitsToday(ctx)
	if err != nil {
		t.Fatalf("HabitsToday: %v", err)
	}

	if got.Date != "2026-10-11" {
		t.Errorf("date = %q, want 2026-10-11 (today in IST)", got.Date)
	}
	want := []struct {
		name string
		done bool
	}{{"gym", true}, {"read", true}, {"walk", false}}
	if len(got.Habits) != len(want) {
		t.Fatalf("habits = %+v, want gym, read, walk", got.Habits)
	}
	for i, w := range want {
		if h := got.Habits[i]; h.Name != w.name || h.Done != w.done {
			t.Errorf("habit %d = %+v, want %s with done = %v", i, h, w.name, w.done)
		}
	}
	if got.Habits[0].ID != gym.ID {
		t.Errorf("ID of gym = %d, want %d", got.Habits[0].ID, gym.ID)
	}
}

func TestHabitsTodayReturnsAnEmptyListNotNil(t *testing.T) {
	s, _ := newTestService(t)

	got, err := s.HabitsToday(context.Background())
	if err != nil {
		t.Fatalf("HabitsToday: %v", err)
	}
	if got.Habits == nil || len(got.Habits) != 0 {
		t.Errorf("habits = %#v, want an empty non-nil slice so JSON shows []", got.Habits)
	}
}

// #40 and rule 11: a live log makes the habit done for its day, whoever
// wrote it. A second log for the same day stores nothing.
func TestALogOfTheAgentMakesTheHabitDone(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	addHabit(t, s, "run")

	first, err := s.LogHabit(ctx, LogHabitParams{Habit: "run", Provenance: Provenance{Source: SourceAgent}})
	if err != nil {
		t.Fatalf("LogHabit: %v", err)
	}
	second, err := s.LogHabit(ctx, LogHabitParams{Habit: "run", Provenance: Provenance{Source: SourceManual}})
	if err != nil || second != first {
		t.Errorf("second log = %+v, %v; want the stored log %+v", second, err, first)
	}
	var events int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM events WHERE record_type = 'habit_logs'").Scan(&events); err != nil || events != 1 {
		t.Errorf("habit_logs events = %d, %v; want 1", events, err)
	}
	checklist, err := s.HabitsToday(ctx)
	if err != nil {
		t.Fatalf("HabitsToday: %v", err)
	}
	for _, h := range checklist.Habits {
		if h.Name == "run" && !h.Done {
			t.Error("run is not done after a log of the agent")
		}
	}
}
