package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// EntityKinds are the kinds of entity: the things that records are about.
var EntityKinds = []string{"person", "place", "project", "topic"}

const maxEntityNameLength = 100

// Entity is one row of entities: a person, a place, a project or a topic.
type Entity struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
	// Aliases are other names of the entity. A name in a message that
	// matches an alias names this entity.
	Aliases []string `json:"aliases,omitempty"`
	// WikiPath is the page of the entity, such as people/ravi.md. It is
	// empty until a page exists.
	WikiPath   string     `json:"wiki_path,omitempty"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	Source     Source     `json:"source"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func entityFromRow(row store.Entity) Entity {
	e := Entity{
		ID:        row.ID,
		Kind:      row.Kind,
		Name:      row.Name,
		Aliases:   row.Aliases,
		Source:    Source(row.Source),
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
	if row.WikiPath != nil {
		e.WikiPath = *row.WikiPath
	}
	if row.ArchivedAt != nil {
		archivedAt := row.ArchivedAt.UTC()
		e.ArchivedAt = &archivedAt
	}
	return e
}

// checkEntityKind refuses a kind that is not in EntityKinds.
func checkEntityKind(field, kind string) error {
	if !slices.Contains(EntityKinds, kind) {
		return invalid(field, "must be one of "+strings.Join(EntityKinds, ", "))
	}
	return nil
}

// checkEntityName trims a name or an alias and refuses one that is empty,
// too long or secret. field is the name of the input field, for the error.
func checkEntityName(field, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", invalid(field, "must not be empty")
	}
	if utf8.RuneCountInString(name) > maxEntityNameLength {
		return "", invalid(field, fmt.Sprintf("must be at most %d characters", maxEntityNameLength))
	}
	if err := checkText(field, name); err != nil {
		return "", err
	}
	return name, nil
}

// findEntity returns the live entity of this kind that has this name or
// this alias, in any case. found is false when there is none.
func findEntity(ctx context.Context, q *store.Queries, kind, name string) (e store.Entity, found bool, err error) {
	rows, err := q.FindEntitiesByName(ctx, store.FindEntitiesByNameParams{Kind: &kind, Name: name})
	if err != nil {
		return store.Entity{}, false, fmt.Errorf("find the entity %q: %w", name, err)
	}
	if len(rows) == 0 {
		return store.Entity{}, false, nil
	}
	// The service refuses a name or an alias that a different entity of
	// the kind has, so there is one row. The first is the oldest.
	return rows[0], true, nil
}

// createEntity stores a new entity and appends the create event. Call it in
// the transaction of the write that needs the entity.
func createEntity(ctx context.Context, q *store.Queries, kind, name string, aliases []string, wikiPath *string, actor Source) (store.Entity, error) {
	row, err := q.InsertEntity(ctx, store.InsertEntityParams{
		Kind:     kind,
		Name:     name,
		Aliases:  nonNilStrings(aliases),
		WikiPath: wikiPath,
		Source:   string(actor),
	})
	if err != nil {
		return store.Entity{}, fmt.Errorf("add the entity %q: %w", name, err)
	}
	if err := recordEvent(ctx, q, "create", "entities", row.ID, actor, nil, entityFromRow(row)); err != nil {
		return store.Entity{}, fmt.Errorf("add the entity %q: %w", name, err)
	}
	return row, nil
}

// ensureEntity returns the entity of this kind that has this name or alias,
// and makes it when there is none (decision 1 of #41: the agent never asks
// about an entity). created says that this call made it.
func ensureEntity(ctx context.Context, q *store.Queries, kind, name string, actor Source) (e store.Entity, created bool, err error) {
	e, found, err := findEntity(ctx, q, kind, name)
	if err != nil || found {
		return e, false, err
	}
	e, err = createEntity(ctx, q, kind, name, nil, nil, actor)
	return e, err == nil, err
}

// nonNilStrings returns an empty list for nil: the column is NOT NULL.
func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// entityNames returns the names of entities by ID, also of deleted ones.
func entityNames(ctx context.Context, q *store.Queries, ids []int64) (map[int64]string, error) {
	names := map[int64]string{}
	if len(ids) == 0 {
		return names, nil
	}
	rows, err := q.ListEntityNames(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("entity names: %w", err)
	}
	for _, row := range rows {
		names[row.ID] = row.Name
	}
	return names, nil
}
