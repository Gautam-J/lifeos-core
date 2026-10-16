package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// The summary page is one screen, so each of its lists is short.
const (
	overviewTodoRows     = 8
	overviewActivityRows = 40
)

// DayCounts is what one IST day holds.
type DayCounts struct {
	// Date is the IST day, as YYYY-MM-DD.
	Date string
	// JournalEntries is the number of entries about the day.
	JournalEntries int
	// HabitsDone is the number of active habits with a live log on the
	// day.
	HabitsDone int
	// TodosDone is the number of todos that became done on the day.
	TodosDone int
	// ItemsAdded and Captures count the rows that were stored on the day.
	ItemsAdded int
	Captures   int
}

// Activity is one row of the events log.
type Activity struct {
	At     time.Time
	Action string
	// RecordType is the name of a table, or wiki.
	RecordType string
	// RecordID is 0 for a wiki page. Path names the page.
	RecordID int64
	Path     string
	Source   Source
}

// Overview is what the summary page of the admin pages shows: today and
// yesterday in IST, and the open todos.
type Overview struct {
	Today     DayCounts
	Yesterday DayCounts
	// Habits is the checklist of today.
	Habits HabitChecklist
	// OpenTodos is the number of open todos. DueTodos is how many of them
	// are due today or before.
	OpenTodos int
	DueTodos  int
	// Next holds the first open todos in the order of the work: the oldest
	// due date first, then the todos that have no date.
	Next []Todo
	// Journal holds the entries about the two days, newest first.
	Journal []JournalEntry
	// Activity holds the events of the two days, newest first.
	Activity []Activity
}

// Overview returns the state of today and yesterday in IST.
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	day := today(s.now())
	yesterday := day.AddDate(0, 0, -1)
	q := store.New(s.pool)

	var view Overview
	var err error
	if view.Today, err = countDay(ctx, q, day); err != nil {
		return Overview{}, err
	}
	if view.Yesterday, err = countDay(ctx, q, yesterday); err != nil {
		return Overview{}, err
	}
	if view.Habits, err = s.HabitsToday(ctx); err != nil {
		return Overview{}, err
	}

	todos, err := q.CountOpenTodos(ctx, day)
	if err != nil {
		return Overview{}, fmt.Errorf("overview: open todos: %w", err)
	}
	view.OpenTodos, view.DueTodos = int(todos.Open), int(todos.Due)
	next, err := q.ListNextTodos(ctx, overviewTodoRows)
	if err != nil {
		return Overview{}, fmt.Errorf("overview: next todos: %w", err)
	}
	if view.Next, err = todosFromStore(ctx, q, next); err != nil {
		return Overview{}, fmt.Errorf("overview: next todos: %w", err)
	}

	entries, err := q.ListJournalEntriesSince(ctx, store.ListJournalEntriesSinceParams{Day: yesterday, MaxRows: defaultListLimit})
	if err != nil {
		return Overview{}, fmt.Errorf("overview: journal: %w", err)
	}
	view.Journal = make([]JournalEntry, 0, len(entries))
	for _, row := range entries {
		view.Journal = append(view.Journal, journalEntryFromRow(row))
	}

	events, err := q.ListEventsSince(ctx, store.ListEventsSinceParams{Since: startOfDay(yesterday), MaxRows: overviewActivityRows})
	if err != nil {
		return Overview{}, fmt.Errorf("overview: activity: %w", err)
	}
	view.Activity = make([]Activity, 0, len(events))
	for _, row := range events {
		activity := Activity{
			At:         row.OccurredAt,
			Action:     row.Action,
			RecordType: row.RecordType,
			Path:       row.WikiPath,
			Source:     Source(row.Source),
		}
		if row.RecordID != nil {
			activity.RecordID = *row.RecordID
		}
		view.Activity = append(view.Activity, activity)
	}
	return view, nil
}

// startOfDay returns the moment that an IST day starts. day is a date as
// today returns it.
func startOfDay(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, IST)
}

// countDay counts the records of one IST day.
func countDay(ctx context.Context, q *store.Queries, day time.Time) (DayCounts, error) {
	row, err := q.CountDay(ctx, store.CountDayParams{
		Day:      day,
		DayStart: startOfDay(day),
		DayEnd:   startOfDay(day.AddDate(0, 0, 1)),
	})
	if err != nil {
		return DayCounts{}, fmt.Errorf("overview: count %s: %w", day.Format(time.DateOnly), err)
	}
	habits, err := q.ListHabitsOnDay(ctx, day)
	if err != nil {
		return DayCounts{}, fmt.Errorf("overview: habits of %s: %w", day.Format(time.DateOnly), err)
	}
	counts := DayCounts{
		Date:           day.Format(time.DateOnly),
		JournalEntries: int(row.JournalEntries),
		TodosDone:      int(row.TodosDone),
		ItemsAdded:     int(row.ItemsAdded),
		Captures:       int(row.Captures),
	}
	for _, habit := range habits {
		if habit.Done {
			counts.HabitsDone++
		}
	}
	return counts, nil
}
