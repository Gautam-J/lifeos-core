package service

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// setTime puts a time into one row by hand. The database sets created_at
// and occurred_at from its own clock, which is not the clock of the test.
func setTime(t *testing.T, pool *pgxpool.Pool, table, column string, id int64, at time.Time) {
	t.Helper()
	tag, err := pool.Exec(context.Background(), "UPDATE "+table+" SET "+column+" = $1 WHERE id = $2", at, id)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("set %s.%s of row %d: %v (%d rows)", table, column, id, err, tag.RowsAffected())
	}
}

// ist returns a time on a day of October 2026 in IST.
func ist(day, hour, minute int) time.Time {
	return time.Date(2026, 10, day, hour, minute, 0, 0, IST)
}

// The summary page shows two days. A day is a day in IST: testNow is
// 01:30 on 11 October in IST, and 10 October in UTC.
func TestOverviewCountsTodayAndYesterdayInIST(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	addJournal(t, s, "today, first", "")
	addJournal(t, s, "today, second", "")
	addJournal(t, s, "yesterday", "2026-10-10")
	addJournal(t, s, "two days ago", "2026-10-09")

	addHabit(t, s, "gym")
	addHabit(t, s, "read")
	addHabit(t, s, "walk")
	logHabit(t, s, "gym", "")
	inferHabitLog(t, s, "read", "") // waits for review: not done
	logHabit(t, s, "gym", "2026-10-10")
	logHabit(t, s, "walk", "2026-10-10")

	for title, doneAt := range map[string]time.Time{
		"done at the start of today":   ist(11, 0, 0),
		"done at the end of yesterday": ist(10, 23, 59),
		"done at noon yesterday":       ist(10, 12, 0),
		"done two days ago":            ist(9, 12, 0),
	} {
		todo := addTodo(t, s, title)
		setStatus(t, s, todo.ID, "done")
		setTime(t, pool, "todos", "done_at", todo.ID, doneAt)
	}

	for title, createdAt := range map[string]time.Time{
		"Today":        ist(11, 1, 0),
		"Yesterday":    ist(10, 9, 0),
		"Two days ago": ist(9, 9, 0),
	} {
		item := addItem(t, s, "movie", title)
		setTime(t, pool, "catalog_items", "created_at", item.ID, createdAt)
		capture := captureText(t, s, "a note of "+title)
		setTime(t, pool, "sources", "created_at", capture.ID, createdAt)
	}
	gone := addItem(t, s, "movie", "Deleted today")
	setTime(t, pool, "catalog_items", "created_at", gone.ID, ist(11, 1, 0))
	if err := s.DeleteItem(ctx, gone.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Overview(ctx)
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}

	wantToday := DayCounts{Date: "2026-10-11", JournalEntries: 2, HabitsDone: 1, TodosDone: 1, ItemsAdded: 1, Captures: 1}
	if got.Today != wantToday {
		t.Errorf("today = %+v, want %+v", got.Today, wantToday)
	}
	wantYesterday := DayCounts{Date: "2026-10-10", JournalEntries: 1, HabitsDone: 2, TodosDone: 2, ItemsAdded: 1, Captures: 1}
	if got.Yesterday != wantYesterday {
		t.Errorf("yesterday = %+v, want %+v", got.Yesterday, wantYesterday)
	}
	if len(got.Habits.Habits) != 3 || !got.Habits.Habits[0].Done || got.Habits.Habits[1].Done {
		t.Errorf("habits = %+v, want gym done, read and walk not done", got.Habits.Habits)
	}
}

// The page lists the work that waits: the open todos in the order of their
// dates, the entries of the two days and the records that an agent
// inferred.
func TestOverviewListsTheOpenTodosAndTheJournal(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	due := func(title, day string) Todo {
		p := buyDomain()
		p.Title, p.DueOn = title, day
		todo, err := s.AddTodo(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		return todo
	}
	due("due today", "2026-10-11")
	due("late", "2026-10-01")
	due("due tomorrow", "2026-10-12")
	due("no date, old", "")
	due("no date, new", "")
	setStatus(t, s, due("late and done", "2026-10-02").ID, "done")

	addJournal(t, s, "two days ago", "2026-10-09")
	addJournal(t, s, "yesterday", "2026-10-10")
	addJournal(t, s, "today", "")

	got, err := s.Overview(ctx)
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}

	if got.OpenTodos != 5 || got.DueTodos != 2 {
		t.Errorf("open, due = %d, %d, want 5, 2", got.OpenTodos, got.DueTodos)
	}
	var next []string
	for _, todo := range got.Next {
		next = append(next, todo.Title)
	}
	if want := []string{"late", "due today", "due tomorrow", "no date, new", "no date, old"}; !slices.Equal(next, want) {
		t.Errorf("next todos = %q, want %q", next, want)
	}
	if len(got.Journal) != 2 || got.Journal[0].Body != "today" || got.Journal[1].Body != "yesterday" {
		t.Errorf("journal = %+v, want today, then yesterday", got.Journal)
	}
}

// The activity is the events log of the two days, newest first. It shows
// what an agent wrote while Gautam did not look.
func TestOverviewShowsTheActivityOfTheTwoDays(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	old := addItem(t, s, "movie", "Old")
	if _, err := pool.Exec(ctx, "UPDATE events SET occurred_at = $1 WHERE record_id = $2", ist(9, 23, 59), old.ID); err != nil {
		t.Fatal(err)
	}
	todo := addTodo(t, s, "Write the page")
	if _, err := pool.Exec(ctx, "UPDATE events SET occurred_at = $1 WHERE record_type = 'todos'", ist(10, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO events (occurred_at, action, record_type, source, new_value)
		VALUES ($1, 'append', 'wiki', 'agent', '{"path": "projects/lifeos.md", "hash": "abc"}')`, ist(11, 1, 0)); err != nil {
		t.Fatal(err)
	}

	got, err := s.Overview(ctx)
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}

	if len(got.Activity) != 2 {
		t.Fatalf("activity = %+v, want the wiki event, then the todo event", got.Activity)
	}
	wiki, first := got.Activity[0], got.Activity[1]
	if wiki.RecordType != "wiki" || wiki.Action != "append" || wiki.Path != "projects/lifeos.md" || wiki.RecordID != 0 || wiki.Source != SourceAgent {
		t.Errorf("wiki event = %+v", wiki)
	}
	if first.RecordType != "todos" || first.Action != "create" || first.RecordID != todo.ID || first.Path != "" || !first.At.Equal(ist(10, 0, 0)) {
		t.Errorf("todo event = %+v", first)
	}
}
