package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func contextEntityNames(c SessionContext) []string {
	var names []string
	for _, e := range c.Entities {
		names = append(names, e.Name)
	}
	return names
}

// #41: the entities that the records of the two days name, with the ref
// and the aliases, so that the agent connects "met R" to the right entity
// with no search.
func TestContextListsTheEntitiesOfTheTwoDays(t *testing.T) {
	s, _ := newWikiService(t)
	ctx := context.Background()
	ravi := addEntity(t, s, "person", "Ravi Kumar", "R")
	addEntity(t, s, "person", "Old Friend") // linked to nothing
	older := addEntity(t, s, "person", "Last Week")

	today := addJournal(t, s, "met R", "2026-10-11")
	yesterday := addJournal(t, s, "ran", "2026-10-10")
	lastWeek := addJournal(t, s, "old", "2026-10-02")
	tag(t, s, "journal", today.ID, EntityRef{ID: ravi.ID}, EntityRef{Kind: "place", Name: "Lonavala"})
	tag(t, s, "journal", yesterday.ID, EntityRef{Kind: "topic", Name: "running"})
	tag(t, s, "journal", lastWeek.ID, EntityRef{ID: older.ID})
	due := addTodoDue(t, s, "call", "2026-10-11", "")
	tag(t, s, "todo", due.ID, EntityRef{Kind: "person", Name: "Sam"})

	got, err := s.Context(ctx, contextDay)

	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	want := []string{"Ravi Kumar", "Sam", "Lonavala", "running"} // by kind, then name
	if names := contextEntityNames(got); strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("entities = %v, want %v", names, want)
	}
	first := got.Entities[0]
	if first.Ref != fmt.Sprintf("entity:%d", ravi.ID) || first.ID != ravi.ID || first.Kind != "person" || len(first.Aliases) != 1 || first.Aliases[0] != "R" {
		t.Errorf("first entity = %+v", first)
	}
	// The section comes after done and before counts: the JSON order.
	raw, _ := json.Marshal(got)
	if i, j := strings.Index(string(raw), `"entities"`), strings.Index(string(raw), `"day"`); i < j {
		t.Errorf("entities come before day in %s", raw)
	}
}

func TestContextLeavesOutAnArchivedAndADeletedEntity(t *testing.T) {
	s, _ := newWikiService(t)
	ctx := context.Background()
	entry := addJournal(t, s, "met", "2026-10-11")
	archived := addEntity(t, s, "person", "Archived")
	gone := addEntity(t, s, "person", "Gone")
	tag(t, s, "journal", entry.ID, EntityRef{ID: archived.ID}, EntityRef{ID: gone.ID}, EntityRef{Kind: "person", Name: "Kept"})
	if _, err := s.UpdateEntity(ctx, archived.ID, UpdateEntityParams{Archived: ptr(true), Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteEntity(ctx, gone.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Context(ctx, contextDay)

	if err != nil || strings.Join(contextEntityNames(got), ",") != "Kept" {
		t.Errorf("entities = %v, %v, want Kept only", contextEntityNames(got), err)
	}
}

// The todo of the result names its project entity with the ref.
func TestContextTodoNamesItsProjectEntity(t *testing.T) {
	s, _ := newWikiService(t)
	todo := addTodoDue(t, s, "pay", "2026-10-11", "home")

	got, err := s.Context(context.Background(), contextDay)

	if err != nil || len(got.Due) != 1 {
		t.Fatalf("Context: %+v, %v", got.Due, err)
	}
	var id int64
	if err := s.pool.QueryRow(context.Background(), "SELECT project_id FROM todos WHERE id = $1", todo.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if due := got.Due[0]; due.Project != "home" || due.ProjectRef != fmt.Sprintf("entity:%d", id) {
		t.Errorf("due todo = %+v, want the project home with its ref", due)
	}
}

// The budget cuts the entities after the done todos, and it keeps the refs
// of the entities that stay.
func TestContextBudgetCutsEntitiesAfterTheDoneTodos(t *testing.T) {
	s, _ := newWikiService(t)
	entry := addJournal(t, s, "a short day", "2026-10-11")
	for i := 0; i < 400; i++ {
		tag(t, s, "journal", entry.ID, EntityRef{Kind: "topic", Name: fmt.Sprintf("topic %03d %s", i, strings.Repeat("y", 40))})
	}

	got, err := s.Context(context.Background(), contextDay)

	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	raw, _ := json.Marshal(got)
	if n := utf8.RuneCount(raw); n > contextBudget {
		t.Errorf("the result has %d characters, want at most %d", n, contextBudget)
	}
	if len(got.Entities) == 0 || len(got.Entities) >= 400 {
		t.Fatalf("entities = %d, want some but not all 400", len(got.Entities))
	}
	if got.Entities[0].Ref == "" || !strings.Contains(strings.Join(got.Cut, " "), "entities") {
		t.Errorf("first = %+v, cut = %v, want a ref and a cut line about entities", got.Entities[0], got.Cut)
	}
}
