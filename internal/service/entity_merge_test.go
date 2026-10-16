package service

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

func TestMergeEntitiesMovesTheLinksAndTheAliases(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	keep := addEntity(t, s, "person", "Ravi Kumar", "RK")
	drop := addEntity(t, s, "person", "R", "Ravi")
	both := addJournal(t, s, "both", "2026-10-09")
	onlyDrop := addJournal(t, s, "only drop", "2026-10-10")
	todo := addTodo(t, s, "call")
	tag(t, s, "journal", both.ID, EntityRef{ID: keep.ID}, EntityRef{ID: drop.ID})
	tag(t, s, "journal", onlyDrop.ID, EntityRef{ID: drop.ID})
	tag(t, s, "todo", todo.ID, EntityRef{ID: drop.ID})
	before := countRows(t, pool, "events")

	got, err := s.MergeEntities(ctx, MergeEntitiesParams{Keep: keep.ID, Drop: drop.ID, Actor: SourceAgent})

	if err != nil {
		t.Fatalf("MergeEntities: %v", err)
	}
	if got.Kept.ID != keep.ID || got.Kept.Name != "Ravi Kumar" {
		t.Errorf("kept = %+v", got.Kept)
	}
	if want := []string{"RK", "R", "Ravi"}; !slices.Equal(got.Kept.Aliases, want) {
		t.Errorf("aliases = %v, want %v: the name and the aliases of the dropped entity", got.Kept.Aliases, want)
	}
	if got.LinksMoved != 2 || got.LinksDropped != 1 {
		t.Errorf("moved %d, dropped %d, want 2 and 1", got.LinksMoved, got.LinksDropped)
	}
	if _, err := s.GetEntity(ctx, drop.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("dropped entity: error = %v, want ErrNotFound", err)
	}
	detail, err := s.GetEntityDetail(ctx, keep.ID)
	if err != nil || len(detail.Records) != 3 {
		t.Errorf("records of the kept entity = %+v, %v, want 3", detail.Records, err)
	}
	if n := countRows(t, pool, "record_entities"); n != 3 {
		t.Errorf("links = %d, want 3", n)
	}
	// Its old name finds the kept entity now.
	res := tag(t, s, "journal", both.ID, EntityRef{Name: "r"})
	if res.Added[0].Ref != "entity:"+strconvID(keep.ID) {
		t.Errorf("a name of the dropped entity resolved to %+v, want the kept entity", res.Added)
	}

	var kinds []string
	events := allEvents(t, pool)
	for _, e := range events[before:] {
		kinds = append(kinds, e.Action+" "+e.RecordType)
	}
	for _, want := range []string{"update entities", "delete entities", "update record_entities", "delete record_entities"} {
		if !slices.Contains(kinds, want) {
			t.Errorf("events after the merge = %v, want %q in them", kinds, want)
		}
	}
}

func strconvID(id int64) string { return strconv.FormatInt(id, 10) }

// findForTest returns the ID of the live entity that has this name.
func findForTest(t *testing.T, s *Service, kind, name string) (id int64, created bool, err error) {
	t.Helper()
	row, found, err := findEntity(context.Background(), store.New(s.pool), kind, name)
	if err != nil || !found {
		t.Fatalf("no %s %q: %v", kind, name, err)
	}
	return row.ID, false, nil
}

func TestMergeOfProjectsMovesTheTodos(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	one, err := s.AddTodo(ctx, AddTodoParams{Title: "a", Project: "LifeOS", Provenance: Provenance{Source: SourceAgent}})
	if err != nil {
		t.Fatal(err)
	}
	two, err := s.AddTodo(ctx, AddTodoParams{Title: "b", Project: "second brain", Provenance: Provenance{Source: SourceAgent}})
	if err != nil {
		t.Fatal(err)
	}
	keep, _, _ := findForTest(t, s, "project", "LifeOS")
	drop, _, _ := findForTest(t, s, "project", "second brain")

	if _, err := s.MergeEntities(ctx, MergeEntitiesParams{Keep: keep, Drop: drop, Actor: SourceAgent}); err != nil {
		t.Fatalf("MergeEntities: %v", err)
	}

	for _, id := range []int64{one.ID, two.ID} {
		got, err := s.GetTodo(ctx, id)
		if err != nil || got.Project != "LifeOS" {
			t.Errorf("todo %d = %+v, %v, want the project LifeOS", id, got, err)
		}
	}
}

func TestMergeEntitiesRefusesBadInput(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	a := addEntity(t, s, "person", "A")
	b := addEntity(t, s, "person", "B")
	place := addEntity(t, s, "place", "P")
	gone := addEntity(t, s, "person", "Gone")
	if err := s.DeleteEntity(ctx, gone.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}
	before := countRows(t, pool, "events")

	for name, p := range map[string]MergeEntitiesParams{
		"an entity with itself":       {Keep: a.ID, Drop: a.ID, Actor: SourceAgent},
		"two kinds":                   {Keep: a.ID, Drop: place.ID, Actor: SourceAgent},
		"a deleted entity":            {Keep: a.ID, Drop: gone.ID, Actor: SourceAgent},
		"an entity that is not there": {Keep: a.ID, Drop: 9999, Actor: SourceAgent},
		"an unknown actor":            {Keep: a.ID, Drop: b.ID, Actor: "robot"},
	} {
		_, err := s.MergeEntities(ctx, p)
		var invalid *ValidationError
		if !errors.As(err, &invalid) && !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: error = %v, want a ValidationError or ErrNotFound", name, err)
		}
	}
	if n := countRows(t, pool, "events"); n != before {
		t.Errorf("events = %d, want %d: refused input appended an event", n, before)
	}
}
