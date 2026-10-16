package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// The numbers of Context (#37).
const (
	// contextBudget is the most characters that a result holds, counted on
	// its JSON text.
	contextBudget = 8000
	// contextBodyRunes is the length at which a journal body is cut.
	contextBodyRunes = 600
)

// contextBodyLimits are the lengths of a journal body, from the one that
// every result starts with to the smallest: the budget cuts the bodies
// step by step before it cuts anything else.
var contextBodyLimits = []int{contextBodyRunes, 300, 150, 60}

// ContextEntry is one journal entry in a SessionContext.
type ContextEntry struct {
	// Ref is the citation of the row, such as journal:57.
	Ref string `json:"ref"`
	ID  int64  `json:"id"`
	// Date is the day that the entry is about; Time is the time in IST at
	// which it was stored.
	Date string `json:"date"`
	Time string `json:"time"`
	// Body is the text as stored. A body that was cut ends with …; get
	// returns the whole text.
	Body string `json:"body"`
}

// ContextTodo is one todo in a SessionContext.
type ContextTodo struct {
	// Ref is the citation of the row, such as todo:12.
	Ref     string `json:"ref"`
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	DueOn   string `json:"due_on,omitempty"`
	Project string `json:"project,omitempty"`
	// DoneOn is the IST day on which a done todo became done.
	DoneOn string `json:"done_on,omitempty"`
}

// SessionContext is what each session starts with. The fields are in the
// order of the issue: a section with no rows is left out.
type SessionContext struct {
	// Me is the body of me.md without the lines that are past their day.
	Me string `json:"me,omitempty"`
	// Note says what is missing and how to make it.
	Note string `json:"note,omitempty"`
	// Day is today in IST, or the day that the caller asked for.
	Day string `json:"day"`
	// Journal holds the entries about the day and the day before, in the
	// order that they were written.
	Journal []ContextEntry `json:"journal,omitempty"`
	// Due holds the open todos due on the day or before it, overdue first.
	Due []ContextTodo `json:"due,omitempty"`
	// Tomorrow holds the open todos due on the next day.
	Tomorrow []ContextTodo `json:"tomorrow,omitempty"`
	// Done holds the todos that became done on the day or the day before.
	Done []ContextTodo `json:"done,omitempty"`
	// Counts is one line: the habits done on the day and the sources that
	// are not processed yet.
	Counts string `json:"counts,omitempty"`
	// Cut says what the budget left out or shortened.
	Cut []string `json:"cut,omitempty"`
}

// contextData is what the queries return, before the budget.
type contextData struct {
	me      SessionContext
	entries []store.JournalEntry
	due     []ContextTodo
	morrow  []ContextTodo
	done    []ContextTodo
	counts  string
}

// Context returns what a session starts with: me.md, the journal of the
// day and the day before, the todos, and a line of counts. day is an IST
// day; the zero time means today. All facts come from SQL. The result is
// at most contextBudget characters when it can be: the budget cuts the
// journal bodies first, then the done todos, and it never cuts an ID,
// me.md or an overdue todo.
//
// The highlights of the journal (#30) and of the todos (#36), and the
// entities of the day (#41), are not here yet: each adds a section when
// its job exists.
func (s *Service) Context(ctx context.Context, day time.Time) (SessionContext, error) {
	if day.IsZero() {
		day = today(s.now())
	}
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)

	data, err := s.loadContext(ctx, day)
	if err != nil {
		return SessionContext{}, err
	}
	return fitContext(day, data), nil
}

func (s *Service) loadContext(ctx context.Context, day time.Time) (contextData, error) {
	q := store.New(s.pool)
	yesterday, tomorrow := day.AddDate(0, 0, -1), day.AddDate(0, 0, 1)
	var data contextData
	var err error

	if data.me, err = s.Me(ctx); err != nil {
		return data, err
	}
	if data.entries, err = q.ListJournalEntriesBetween(ctx, store.ListJournalEntriesBetweenParams{FromDay: yesterday, ToDay: day}); err != nil {
		return data, fmt.Errorf("context: journal: %w", err)
	}
	due, err := q.ListOpenTodosDueBy(ctx, day)
	if err != nil {
		return data, fmt.Errorf("context: due todos: %w", err)
	}
	next, err := q.ListOpenTodosDueOn(ctx, tomorrow)
	if err != nil {
		return data, fmt.Errorf("context: todos of tomorrow: %w", err)
	}
	done, err := q.ListTodosDoneBetween(ctx, store.ListTodosDoneBetweenParams{SpanStart: startOfDay(yesterday), SpanEnd: startOfDay(tomorrow)})
	if err != nil {
		return data, fmt.Errorf("context: done todos: %w", err)
	}
	if data.due, err = contextTodos(ctx, q, due); err != nil {
		return data, fmt.Errorf("context: due todos: %w", err)
	}
	if data.morrow, err = contextTodos(ctx, q, next); err != nil {
		return data, fmt.Errorf("context: todos of tomorrow: %w", err)
	}
	if data.done, err = contextTodos(ctx, q, done); err != nil {
		return data, fmt.Errorf("context: done todos: %w", err)
	}

	habits, err := q.ListHabitsOnDay(ctx, day)
	if err != nil {
		return data, fmt.Errorf("context: habits: %w", err)
	}
	waiting, err := q.CountSourcesNotProcessed(ctx)
	if err != nil {
		return data, fmt.Errorf("context: sources: %w", err)
	}
	data.counts = contextCounts(habits, waiting)
	return data, nil
}

func contextTodos(ctx context.Context, q *store.Queries, rows []store.Todo) ([]ContextTodo, error) {
	var todos []ContextTodo
	stored, err := todosFromStore(ctx, q, rows)
	if err != nil {
		return nil, err
	}
	for _, todo := range stored {
		item := ContextTodo{Ref: fmt.Sprintf("todo:%d", todo.ID), ID: todo.ID, Title: todo.Title, DueOn: todo.DueOn, Project: todo.Project}
		if todo.DoneAt != nil {
			item.DoneOn = todo.DoneAt.In(IST).Format(time.DateOnly)
		}
		todos = append(todos, item)
	}
	return todos, nil
}

// contextCounts is the line of counts, or "" when it has nothing to say.
func contextCounts(habits []store.ListHabitsOnDayRow, waiting int64) string {
	var parts []string
	if len(habits) > 0 {
		done := 0
		for _, h := range habits {
			if h.Done {
				done++
			}
		}
		parts = append(parts, fmt.Sprintf("Habits done today: %d of %d.", done, len(habits)))
	}
	if waiting > 0 {
		noun := "sources are"
		if waiting == 1 {
			noun = "source is"
		}
		parts = append(parts, fmt.Sprintf("%d %s not processed yet.", waiting, noun))
	}
	return strings.Join(parts, " ")
}

// fitContext builds the result and, when it is over the budget, shortens
// the journal bodies step by step, then drops the oldest done todos.
func fitContext(day time.Time, data contextData) SessionContext {
	var built SessionContext
	for _, limit := range contextBodyLimits {
		built = buildContext(day, data, limit, len(data.done))
		if contextSize(built) <= contextBudget {
			return built
		}
	}
	last := contextBodyLimits[len(contextBodyLimits)-1]
	for keep := len(data.done) - 1; keep >= 0; keep-- {
		built = buildContext(day, data, last, keep)
		if contextSize(built) <= contextBudget {
			return built
		}
	}
	return built
}

func contextSize(c SessionContext) int {
	raw, err := json.Marshal(c)
	if err != nil {
		return 0
	}
	return utf8.RuneCount(raw)
}

// buildContext makes the result with every journal body cut to bodyRunes
// characters and only the newest keepDone done todos.
func buildContext(day time.Time, data contextData, bodyRunes, keepDone int) SessionContext {
	c := SessionContext{
		Me: data.me.Me, Note: data.me.Note, Day: day.Format(time.DateOnly),
		Due: data.due, Tomorrow: data.morrow, Counts: data.counts,
	}
	cutBodies := 0
	for _, e := range data.entries {
		body := e.Body
		if utf8.RuneCountInString(body) > bodyRunes {
			body = string([]rune(body)[:bodyRunes]) + "…"
			cutBodies++
		}
		c.Journal = append(c.Journal, ContextEntry{
			Ref: fmt.Sprintf("journal:%d", e.ID), ID: e.ID,
			Date: e.EntryDate.Format(time.DateOnly), Time: e.CreatedAt.In(IST).Format("15:04"), Body: body,
		})
	}
	if cutBodies > 0 {
		c.Cut = append(c.Cut, fmt.Sprintf("%d of %d journal bodies are cut to %d characters and end with …. Call get with record_type journal and the ID for the whole text.", cutBodies, len(data.entries), bodyRunes))
	}
	if left := len(data.done) - keepDone; left > 0 {
		c.Cut = append(c.Cut, fmt.Sprintf("%d older done todos are left out. Call query on the table todos to list them.", left))
	}
	c.Done = data.done[len(data.done)-keepDone:]
	return c
}
