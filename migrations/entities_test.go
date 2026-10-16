package migrations_test

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/Gautam-J/lifeos-core/internal/testdb"
	"github.com/Gautam-J/lifeos-core/migrations"
)

func TestEntityTablesExistAndKeepTheProvenanceColumns(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	for _, column := range []string{"source", "created_at", "updated_at", "deleted_at", "aliases", "wiki_path", "archived_at"} {
		var exists bool
		if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'entities' AND column_name = $1)", column).Scan(&exists); err != nil || !exists {
			t.Errorf("entities has no column %s (%v)", column, err)
		}
	}
	var triggers int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM pg_trigger WHERE tgrelid = 'entities'::regclass AND tgname = 'entities_set_updated_at'").Scan(&triggers); err != nil || triggers != 1 {
		t.Errorf("entities has %d updated_at triggers, want 1 (%v)", triggers, err)
	}
	if got, want := columnTables(t, pool, "project_id"), []string{"todos"}; !slices.Equal(got, want) {
		t.Errorf("tables with project_id = %v, want %v", got, want)
	}
	if got := columnTables(t, pool, "project"); len(got) != 0 {
		t.Errorf("tables with project = %v, want none: todos.project is gone", got)
	}
}

func TestEntitiesRefuseABadKindAndADuplicateName(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "INSERT INTO entities (kind, name, source) VALUES ('animal', 'Rex', 'agent')"); err == nil {
		t.Error("an entity of the kind animal was stored")
	}
	if _, err := pool.Exec(ctx, "INSERT INTO entities (kind, name, source) VALUES ('person', 'Ravi', 'agent')"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO entities (kind, name, source) VALUES ('person', 'ravi', 'agent')"); err == nil {
		t.Error("a second person with the same name in a different case was stored")
	}
	if _, err := pool.Exec(ctx, "INSERT INTO entities (kind, name, source) VALUES ('place', 'Ravi', 'agent')"); err != nil {
		t.Errorf("the same name in a different kind: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE entities SET deleted_at = now() WHERE kind = 'person'"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO entities (kind, name, source) VALUES ('person', 'Ravi', 'agent')"); err != nil {
		t.Errorf("the name of a deleted entity: %v", err)
	}
}

func TestALinkHasExactlyOneRecordAndExistsOnce(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	var entity, todo, journal int64
	if err := pool.QueryRow(ctx, "INSERT INTO entities (kind, name, source) VALUES ('person', 'Ravi', 'agent') RETURNING id").Scan(&entity); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO todos (title, source) VALUES ('Call', 'agent') RETURNING id").Scan(&todo); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO journal_entries (body, entry_date, source) VALUES ('met', '2026-10-15', 'agent') RETURNING id").Scan(&journal); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, "INSERT INTO record_entities (entity_id, source) VALUES ($1, 'agent')", entity); err == nil {
		t.Error("a link with no record was stored")
	}
	if _, err := pool.Exec(ctx, "INSERT INTO record_entities (entity_id, todo_id, journal_id, source) VALUES ($1, $2, $3, 'agent')", entity, todo, journal); err == nil {
		t.Error("a link with two records was stored")
	}
	if _, err := pool.Exec(ctx, "INSERT INTO record_entities (entity_id, todo_id, source) VALUES ($1, $2, 'agent')", entity, todo); err != nil {
		t.Fatalf("a link of a todo: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO record_entities (entity_id, todo_id, source) VALUES ($1, $2, 'agent')", entity, todo); err == nil {
		t.Error("the same link was stored two times")
	}
	if _, err := pool.Exec(ctx, "INSERT INTO record_entities (entity_id, journal_id, source) VALUES ($1, $2, 'agent')", entity, journal); err != nil {
		t.Errorf("a link of a journal entry to the same entity: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO record_entities (entity_id, todo_id, source) VALUES ($1, 99999, 'agent')", entity); err == nil {
		t.Error("a link to a todo that does not exist was stored")
	}
}

func TestOnlyAProjectCanBeTheProjectOfATodo(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	var person, project int64
	if err := pool.QueryRow(ctx, "INSERT INTO entities (kind, name, source) VALUES ('person', 'Ravi', 'agent') RETURNING id").Scan(&person); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO entities (kind, name, source) VALUES ('project', 'lifeos', 'agent') RETURNING id").Scan(&project); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, "INSERT INTO todos (title, source, project_id) VALUES ('x', 'agent', $1)", person); err == nil {
		t.Error("a todo with a person as its project was stored")
	}
	if _, err := pool.Exec(ctx, "INSERT INTO todos (title, source, project_id) VALUES ('x', 'agent', $1)", project); err != nil {
		t.Errorf("a todo with a project: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO todos (title, source) VALUES ('y', 'agent')"); err != nil {
		t.Errorf("a todo with no project: %v", err)
	}
}

// The live database has todos with a project as text. Each distinct value
// becomes one project, and each todo keeps its project.
func TestTheMigrationOf41TurnsTheProjectOfATodoIntoAnEntity(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	if err := migrations.DownTo(ctx, db, 7); err != nil {
		t.Fatalf("DownTo 7: %v", err)
	}
	for _, row := range []struct{ title, project string }{{"a", "LifeOS"}, {"b", "lifeos"}, {"c", "garden"}, {"d", ""}} {
		var project any
		if row.project != "" {
			project = row.project
		}
		if _, err := pool.Exec(ctx, "INSERT INTO todos (title, source, project) VALUES ($1, 'agent', $2)", row.title, project); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("Up: %v", err)
	}

	rows, err := pool.Query(ctx, "SELECT name FROM entities WHERE kind = 'project'")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	slices.Sort(names)
	if want := []string{"LifeOS", "garden"}; !slices.Equal(names, want) {
		t.Errorf("projects = %v, want %v (the spelling of the oldest todo)", names, want)
	}
	for title, want := range map[string]string{"a": "LifeOS", "b": "LifeOS", "c": "garden", "d": ""} {
		var got string
		if err := pool.QueryRow(ctx, "SELECT coalesce(e.name, '') FROM todos t LEFT JOIN entities e ON e.id = t.project_id WHERE t.title = $1", title).Scan(&got); err != nil || got != want {
			t.Errorf("project of todo %s = %q, %v, want %q", title, got, err, want)
		}
	}
	var events int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM events WHERE record_type = 'entities' AND action = 'create' AND source = 'import'").Scan(&events); err != nil || events != 2 {
		t.Errorf("create events of the migrated projects = %d, %v, want 2", events, err)
	}

	// Down and Up again keeps the todos and their projects.
	if err := migrations.DownTo(ctx, db, 7); err != nil {
		t.Fatalf("DownTo 7 (second): %v", err)
	}
	var project string
	if err := pool.QueryRow(ctx, "SELECT project FROM todos WHERE title = 'b'").Scan(&project); err != nil || project != "LifeOS" {
		t.Errorf("after Down: project = %q, %v, want LifeOS", project, err)
	}
}
