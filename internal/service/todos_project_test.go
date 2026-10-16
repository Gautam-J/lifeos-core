package service

import (
	"context"
	"testing"
)

// #41: the project of a todo is a project entity. Two todos that name the
// same project share one entity, also when the case differs.
func TestTodosShareOneProjectEntity(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	first, err := s.AddTodo(ctx, AddTodoParams{Title: "a", Project: "LifeOS", Provenance: Provenance{Source: SourceAgent}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.AddTodo(ctx, AddTodoParams{Title: "b", Project: "lifeos", Provenance: Provenance{Source: SourceAgent}})
	if err != nil {
		t.Fatal(err)
	}

	if first.Project != "LifeOS" || second.Project != "LifeOS" {
		t.Errorf("projects = %q, %q, want LifeOS twice: the stored spelling wins", first.Project, second.Project)
	}
	if n := countRows(t, pool, "entities"); n != 1 {
		t.Errorf("entities = %d, want 1", n)
	}
	got, err := s.GetTodo(ctx, second.ID)
	if err != nil || got.Project != "LifeOS" {
		t.Errorf("GetTodo = %+v, %v, want the project name from the entity", got, err)
	}
}

func TestAProjectNameThatDiffersOnlyInCaseIsNoChange(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added := addTodo(t, s, "a") // project lifeos
	before := countRows(t, pool, "events")

	got, err := s.UpdateTodo(ctx, added.ID, UpdateTodoParams{Project: ptr("LIFEOS"), Actor: SourceAgent})

	if err != nil || got != added {
		t.Errorf("UpdateTodo = %+v, %v, want the stored todo", got, err)
	}
	if n := countRows(t, pool, "events"); n != before {
		t.Errorf("events = %d, want %d: no value changed", n, before)
	}
}

func TestChangeAndClearTheProjectOfATodo(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added := addTodo(t, s, "a")

	moved, err := s.UpdateTodo(ctx, added.ID, UpdateTodoParams{Project: ptr("garden"), Actor: SourceManual})
	if err != nil || moved.Project != "garden" {
		t.Fatalf("UpdateTodo = %+v, %v, want the project garden", moved, err)
	}
	if n := countRows(t, pool, "entities"); n != 2 {
		t.Errorf("entities = %d, want 2 (lifeos, garden)", n)
	}

	cleared, err := s.UpdateTodo(ctx, added.ID, UpdateTodoParams{Clear: []string{"project"}, Actor: SourceManual})
	if err != nil || cleared.Project != "" {
		t.Fatalf("UpdateTodo clear = %+v, %v, want no project", cleared, err)
	}
	if n := countRows(t, pool, "entities"); n != 2 {
		t.Errorf("entities = %d, want 2: a project stays when no todo names it", n)
	}
}

func TestAProjectAliasNamesTheProject(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "INSERT INTO entities (kind, name, aliases, source) VALUES ('project', 'LifeOS', ARRAY['second brain'], 'manual')"); err != nil {
		t.Fatal(err)
	}

	got, err := s.AddTodo(ctx, AddTodoParams{Title: "a", Project: "Second Brain", Provenance: Provenance{Source: SourceAgent}})

	if err != nil || got.Project != "LifeOS" {
		t.Fatalf("AddTodo = %+v, %v, want the project LifeOS", got, err)
	}
	if n := countRows(t, pool, "entities"); n != 1 {
		t.Errorf("entities = %d, want 1: an alias makes no new project", n)
	}
}
