package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// contextDay is the IST day of testNow.
var contextDay = time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)

func addTodoDue(t *testing.T, s *Service, title, dueOn, project string) Todo {
	t.Helper()
	todo, err := s.AddTodo(context.Background(), AddTodoParams{Title: title, DueOn: dueOn, Project: project, Provenance: Provenance{Source: SourceAgent}})
	if err != nil {
		t.Fatalf("AddTodo(%q): %v", title, err)
	}
	return todo
}

// finishTodo makes the todo done at the moment at.
func finishTodo(t *testing.T, s *Service, id int64, at time.Time) {
	t.Helper()
	keep := s.now
	s.now = func() time.Time { return at }
	defer func() { s.now = keep }()
	if _, err := s.UpdateTodo(context.Background(), id, UpdateTodoParams{Status: ptr("done"), Actor: SourceAgent}); err != nil {
		t.Fatalf("UpdateTodo(%d): %v", id, err)
	}
}

func contextRefs[T interface{ ref() string }](rows []T) []string {
	var refs []string
	for _, row := range rows {
		refs = append(refs, row.ref())
	}
	return refs
}

func (e ContextEntry) ref() string { return e.Ref }
func (e ContextTodo) ref() string  { return e.Ref }

// #37: the sections come in the order of the issue, for the right IST days.
func TestContextReturnsTheSectionsInOrderForAFixedDay(t *testing.T) {
	s, _ := newWikiService(t)
	ctx := context.Background()
	putPage(t, s, "me.md", "---\ntype: me\ntitle: About Gautam\nsummary: S.\ncreated: 2026-10-01\nupdated: 2026-10-01\n---\n\nGautam builds LifeOS.\n")

	older := addJournal(t, s, "two days ago", "2026-10-09")
	yesterday := addJournal(t, s, "  yesterday  \n", "2026-10-10")
	todayEntry := addJournal(t, s, "today", "2026-10-11")

	overdue := addTodoDue(t, s, "pay the bill", "2026-10-08", "home")
	dueToday := addTodoDue(t, s, "call the plumber", "2026-10-11", "")
	tomorrow := addTodoDue(t, s, "book the bike service", "2026-10-12", "bike")
	later := addTodoDue(t, s, "renew the passport", "2026-10-20", "")
	undated := addTodoDue(t, s, "someday", "", "")
	// 23:30 UTC on 9 October is 05:00 IST on 10 October: yesterday.
	doneYesterday := addTodoDue(t, s, "done after midnight in IST", "", "lifeos")
	finishTodo(t, s, doneYesterday.ID, time.Date(2026, 10, 9, 23, 30, 0, 0, time.UTC))
	// 18:00 UTC on 9 October is 23:30 IST on 9 October: two days ago.
	doneOlder := addTodoDue(t, s, "done the day before", "", "")
	finishTodo(t, s, doneOlder.ID, time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC))
	doneToday := addTodoDue(t, s, "done today", "", "")
	finishTodo(t, s, doneToday.ID, testNow)

	got, err := s.Context(ctx, contextDay)
	if err != nil {
		t.Fatalf("Context: %v", err)
	}

	if got.Day != "2026-10-11" || !strings.Contains(got.Me, "Gautam builds LifeOS.") {
		t.Errorf("day, me = %q, %q", got.Day, got.Me)
	}
	wantJournal := []string{fmt.Sprintf("journal:%d", yesterday.ID), fmt.Sprintf("journal:%d", todayEntry.ID)}
	if refs := contextRefs(got.Journal); !equalStrings(refs, wantJournal) {
		t.Errorf("journal = %v, want %v (not %d)", refs, wantJournal, older.ID)
	}
	if got.Journal[0].Body != "  yesterday  \n" || got.Journal[0].Date != "2026-10-10" || got.Journal[0].ID != yesterday.ID {
		t.Errorf("first entry = %+v, want the body unchanged", got.Journal[0])
	}
	if !regexp.MustCompile(`^[0-9]{2}:[0-9]{2}$`).MatchString(got.Journal[0].Time) {
		t.Errorf("time = %q, want HH:MM", got.Journal[0].Time)
	}
	if refs, want := contextRefs(got.Due), []string{todoRef(overdue.ID), todoRef(dueToday.ID)}; !equalStrings(refs, want) {
		t.Errorf("due = %v, want %v: overdue first", refs, want)
	}
	if got.Due[0].Project != "home" || got.Due[0].DueOn != "2026-10-08" {
		t.Errorf("first due = %+v, want the project and the day", got.Due[0])
	}
	if refs, want := contextRefs(got.Tomorrow), []string{todoRef(tomorrow.ID)}; !equalStrings(refs, want) {
		t.Errorf("tomorrow = %v, want %v (not %d or %d)", refs, want, later.ID, undated.ID)
	}
	if refs, want := contextRefs(got.Done), []string{todoRef(doneYesterday.ID), todoRef(doneToday.ID)}; !equalStrings(refs, want) {
		t.Errorf("done = %v, want %v (not %d): done_at is cut by the IST day", refs, want, doneOlder.ID)
	}
	if got.Done[0].DoneOn != "2026-10-10" || got.Done[0].Project != "lifeos" {
		t.Errorf("first done = %+v, want the IST day 2026-10-10 and the project", got.Done[0])
	}

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	last := -1
	for _, key := range []string{`"me"`, `"journal"`, `"due"`, `"tomorrow"`, `"done"`} {
		at := strings.Index(string(raw), key)
		if at <= last {
			t.Errorf("%s is at %d in %s, want it after the section before", key, at, raw)
		}
		last = at
	}
}

func todoRef(id int64) string { return fmt.Sprintf("todo:%d", id) }

func equalStrings(a, b []string) bool { return strings.Join(a, ",") == strings.Join(b, ",") }

// A body over 600 characters is cut with … and keeps its ID.
func TestContextCutsALongBodyAtSixHundredCharacters(t *testing.T) {
	s, _ := newWikiService(t)
	long := addJournal(t, s, strings.Repeat("é", 700), "2026-10-11")
	short := addJournal(t, s, strings.Repeat("a", 600), "2026-10-11")

	got, err := s.Context(context.Background(), contextDay)
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if len(got.Journal) != 2 || got.Journal[0].ID != long.ID || got.Journal[1].ID != short.ID {
		t.Fatalf("journal = %+v", got.Journal)
	}
	if body := got.Journal[0].Body; body != strings.Repeat("é", 600)+"…" {
		t.Errorf("long body has %d characters, want 600 and …", utf8.RuneCountInString(body))
	}
	if got.Journal[1].Body != strings.Repeat("a", 600) {
		t.Errorf("a body of 600 characters was changed")
	}
	if len(got.Cut) != 1 || !strings.Contains(got.Cut[0], "get") {
		t.Errorf("cut = %v, want one line that says how to get the rest", got.Cut)
	}
}

// The budget: a long day loses journal bodies first. Every ID, me.md and
// the overdue todos stay, and the result says what it cut.
func TestContextBudgetCutsJournalBodiesFirst(t *testing.T) {
	s, _ := newWikiService(t)
	putPage(t, s, "me.md", "---\ntype: me\ntitle: About Gautam\nsummary: S.\ncreated: 2026-10-01\nupdated: 2026-10-01\n---\n\nGautam builds LifeOS.\n")
	var ids []int64
	for i := 0; i < 30; i++ {
		ids = append(ids, addJournal(t, s, fmt.Sprintf("entry %d: %s", i, strings.Repeat("word ", 100)), "2026-10-11").ID)
	}
	overdue := addTodoDue(t, s, "pay the bill", "2026-10-01", "")
	done := addTodoDue(t, s, "done today", "", "")
	finishTodo(t, s, done.ID, testNow)

	got, err := s.Context(context.Background(), contextDay)
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	raw, _ := json.Marshal(got)
	if n := utf8.RuneCount(raw); n > contextBudget {
		t.Errorf("the result has %d characters, want at most %d", n, contextBudget)
	}
	if len(got.Journal) != len(ids) {
		t.Fatalf("journal has %d entries, want all %d", len(got.Journal), len(ids))
	}
	for i, e := range got.Journal {
		if e.ID != ids[i] || e.Ref == "" {
			t.Errorf("entry %d = %+v, want ID %d", i, e, ids[i])
		}
	}
	if !strings.Contains(got.Me, "Gautam builds LifeOS.") || len(got.Due) != 1 || got.Due[0].ID != overdue.ID {
		t.Errorf("me, due = %q, %+v, want both whole", got.Me, got.Due)
	}
	if len(got.Done) != 1 {
		t.Errorf("done = %+v, want it kept: the journal bodies go first", got.Done)
	}
	if len(got.Cut) == 0 || !strings.Contains(strings.Join(got.Cut, " "), "journal") {
		t.Errorf("cut = %v, want a line about the journal bodies", got.Cut)
	}
}

// When the bodies are at their smallest, the done todos go next.
func TestContextBudgetCutsDoneTodosSecond(t *testing.T) {
	s, _ := newWikiService(t)
	entry := addJournal(t, s, "a short day", "2026-10-11")
	overdue := addTodoDue(t, s, "pay the bill", "2026-10-01", "")
	for i := 0; i < 120; i++ {
		done := addTodoDue(t, s, fmt.Sprintf("done task %d %s", i, strings.Repeat("x", 100)), "", "")
		finishTodo(t, s, done.ID, testNow)
	}

	got, err := s.Context(context.Background(), contextDay)
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	raw, _ := json.Marshal(got)
	if n := utf8.RuneCount(raw); n > contextBudget {
		t.Errorf("the result has %d characters, want at most %d", n, contextBudget)
	}
	if len(got.Journal) != 1 || got.Journal[0].ID != entry.ID || got.Journal[0].Body != "a short day" {
		t.Errorf("journal = %+v, want the entry whole", got.Journal)
	}
	if len(got.Due) != 1 || got.Due[0].ID != overdue.ID {
		t.Errorf("due = %+v, want the overdue todo", got.Due)
	}
	if len(got.Done) == 0 || len(got.Done) >= 120 {
		t.Errorf("done has %d todos, want some but not all 120", len(got.Done))
	}
	if !strings.Contains(strings.Join(got.Cut, " "), "done") {
		t.Errorf("cut = %v, want a line about the done todos", got.Cut)
	}
}

// A new database with no me.md, no highlight and no row still gives a
// result, and says what is missing.
func TestContextWorksWithNothingInIt(t *testing.T) {
	s, _ := newWikiService(t)

	got, err := s.Context(context.Background(), contextDay)
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if got.Day != "2026-10-11" || !strings.Contains(got.Note, "me.md") {
		t.Errorf("day, note = %q, %q, want the day and a note about me.md", got.Day, got.Note)
	}
	raw, _ := json.Marshal(got)
	for _, key := range []string{`"journal"`, `"due"`, `"tomorrow"`, `"done"`, `"counts"`, `"cut"`, `"highlights"`} {
		if strings.Contains(string(raw), key) {
			t.Errorf("the result has %s with no row: %s", key, raw)
		}
	}
}

// A zero day means today in IST.
func TestContextWithNoDayIsToday(t *testing.T) {
	s, _ := newWikiService(t)
	addJournal(t, s, "today", "2026-10-11")

	got, err := s.Context(context.Background(), time.Time{})
	if err != nil || got.Day != "2026-10-11" || len(got.Journal) != 1 {
		t.Errorf("Context = %+v, %v, want today 2026-10-11 with one entry", got, err)
	}
}

// The line of counts: the habits done today and the sources that wait.
func TestContextCountsHabitsAndSourcesThatWait(t *testing.T) {
	s, _ := newWikiService(t)
	ctx := context.Background()
	for _, name := range []string{"gym", "read"} {
		if _, err := s.AddHabit(ctx, AddHabitParams{Name: name, Provenance: Provenance{Source: SourceManual}}); err != nil {
			t.Fatalf("AddHabit: %v", err)
		}
	}
	if _, err := s.LogHabit(ctx, LogHabitParams{Habit: "gym", Provenance: Provenance{Source: SourceAgent}}); err != nil {
		t.Fatalf("LogHabit: %v", err)
	}
	kept, err := s.Capture(ctx, CaptureParams{Text: "read later", Source: SourceAgent})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	done, err := s.Capture(ctx, CaptureParams{Text: "already read", Source: SourceAgent})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if _, err := s.MarkProcessed(ctx, done.ID, SourceAgent); err != nil {
		t.Fatalf("MarkProcessed: %v", err)
	}

	got, err := s.Context(ctx, contextDay)
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if !strings.Contains(got.Counts, "1 of 2") || !strings.Contains(got.Counts, "1 source") {
		t.Errorf("counts = %q, want 1 of 2 habits and 1 source (id %d)", got.Counts, kept.ID)
	}
}
