package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

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

// summary is the one line that search and the list of deleted records show
// for an entity.
func (e Entity) summary() string {
	s := e.Kind + ": " + e.Name
	if len(e.Aliases) > 0 {
		s += " (also " + strings.Join(e.Aliases, ", ") + ")"
	}
	if e.ArchivedAt != nil {
		s += ", archived"
	}
	return s
}

// AddEntityParams is the input of AddEntity.
type AddEntityParams struct {
	Kind string
	Name string
	// Aliases are other names of the entity. Optional.
	Aliases []string
	// WikiPath is the page of the entity, such as people/ravi-kumar.md.
	// Optional.
	WikiPath string
	Provenance
}

// entityWikiFolders gives the folder of the page of each kind. A place has
// no folder, so it has no page.
var entityWikiFolders = map[string]string{"person": "person", "project": "project", "topic": "topic"}

// checkEntityWikiPath refuses a path that is not the path of a page of this
// kind. "" means none.
func checkEntityWikiPath(kind, path string) (*string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	pageType, err := wikiPageType(path)
	if err != nil {
		return nil, invalid("wiki_path", wikiPathRule)
	}
	if entityWikiFolders[kind] != pageType {
		return nil, invalid("wiki_path", "a "+kind+" has no page in this folder")
	}
	return &path, nil
}

// cleanAliases trims the aliases and removes the repeats and the name. It
// refuses an alias that is empty, too long or secret.
func cleanAliases(name string, aliases []string) ([]string, error) {
	clean := []string{}
	seen := map[string]bool{strings.ToLower(name): true}
	for _, alias := range aliases {
		alias, err := checkEntityName("aliases", alias)
		if err != nil {
			return nil, err
		}
		if !seen[strings.ToLower(alias)] {
			seen[strings.ToLower(alias)] = true
			clean = append(clean, alias)
		}
	}
	return clean, nil
}

// checkNamesFree refuses a name or an alias that a different live entity of
// the kind has already. selfID is the entity that is being changed, or 0.
func checkNamesFree(ctx context.Context, q *store.Queries, kind string, selfID int64, name string, aliases []string) error {
	check := func(field, value string) error {
		rows, err := q.FindEntitiesByName(ctx, store.FindEntitiesByNameParams{Kind: &kind, Name: value})
		if err != nil {
			return fmt.Errorf("check the name %q: %w", value, err)
		}
		for _, row := range rows {
			if row.ID != selfID {
				return invalid(field, fmt.Sprintf("%q is the name or an alias of the %s %q (entity:%d) already; use that entity, and to add an alias to it, update it with add_aliases", value, kind, row.Name, row.ID))
			}
		}
		return nil
	}
	if err := check("name", name); err != nil {
		return err
	}
	for _, alias := range aliases {
		if err := check("aliases", alias); err != nil {
			return err
		}
	}
	return nil
}

// AddEntity stores a new entity and appends a create event, in one
// transaction. It refuses a name or an alias that a different entity of the
// kind has.
func (s *Service) AddEntity(ctx context.Context, p AddEntityParams) (Entity, error) {
	if err := checkEntityKind("kind", p.Kind); err != nil {
		return Entity{}, err
	}
	name, err := checkEntityName("name", p.Name)
	if err != nil {
		return Entity{}, err
	}
	aliases, err := cleanAliases(name, p.Aliases)
	if err != nil {
		return Entity{}, err
	}
	wikiPath, err := checkEntityWikiPath(p.Kind, p.WikiPath)
	if err != nil {
		return Entity{}, err
	}
	if err := p.Provenance.validate(); err != nil {
		return Entity{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Entity{}, fmt.Errorf("add entity: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)
	if err := checkNamesFree(ctx, q, p.Kind, 0, name, aliases); err != nil {
		return Entity{}, err
	}
	row, err := createEntity(ctx, q, p.Kind, name, aliases, wikiPath, p.Source)
	if err != nil {
		return Entity{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Entity{}, fmt.Errorf("add entity: %w", err)
	}
	return entityFromRow(row), nil
}

// GetEntity returns the entity with this ID. It returns ErrNotFound when
// there is none or the entity is deleted.
func (s *Service) GetEntity(ctx context.Context, id int64) (Entity, error) {
	row, err := store.New(s.pool).GetEntity(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Entity{}, ErrNotFound
	}
	if err != nil {
		return Entity{}, fmt.Errorf("get entity: %w", err)
	}
	return entityFromRow(row), nil
}

// ListEntitiesParams is the input of ListEntities.
type ListEntitiesParams struct {
	// Kind is optional: an empty string lists every kind.
	Kind string
	// IncludeArchived adds the archived entities.
	IncludeArchived bool
	// Limit is optional: 0 means 50. The maximum is 200.
	Limit int
}

// ListEntities returns the live entities, by kind and name.
func (s *Service) ListEntities(ctx context.Context, p ListEntitiesParams) ([]Entity, error) {
	maxRows, err := checkLimit(p.Limit)
	if err != nil {
		return nil, err
	}
	arg := store.ListEntitiesParams{IncludeArchived: p.IncludeArchived, MaxRows: maxRows}
	if p.Kind != "" {
		if err := checkEntityKind("kind", p.Kind); err != nil {
			return nil, err
		}
		arg.Kind = &p.Kind
	}
	rows, err := store.New(s.pool).ListEntities(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("list entities: %w", err)
	}
	entities := make([]Entity, len(rows))
	for i, row := range rows {
		entities[i] = entityFromRow(row)
	}
	return entities, nil
}

// UpdateEntityParams is the input of UpdateEntity. A field that is nil keeps
// its stored value. The kind never changes: a person cannot become a
// project.
type UpdateEntityParams struct {
	Name *string `json:"name"`
	// Aliases replaces the list of aliases.
	Aliases *[]string `json:"aliases"`
	// AddAliases adds to the list of aliases.
	AddAliases []string `json:"add_aliases"`
	WikiPath   *string  `json:"wiki_path"`
	// Archived true archives the entity, false brings it back.
	Archived *bool `json:"archived"`
	// Clear names the optional fields to empty: wiki_path.
	Clear []string `json:"-"`
	// Actor is who makes the change.
	Actor Source `json:"-"`
}

// UpdateEntity changes the given fields of one entity and appends an update
// event that holds the old row, in one transaction. A call that changes no
// value writes nothing and appends no event.
func (s *Service) UpdateEntity(ctx context.Context, id int64, p UpdateEntityParams) (Entity, error) {
	if err := (Provenance{Source: p.Actor}).validate(); err != nil {
		return Entity{}, err
	}
	if p.Name == nil && p.Aliases == nil && len(p.AddAliases) == 0 && p.WikiPath == nil && p.Archived == nil && len(p.Clear) == 0 {
		return Entity{}, invalid("fields", "must name at least one field to change, or a field to empty in clear")
	}
	for _, name := range p.Clear {
		if name != "wiki_path" {
			return Entity{}, invalid("clear", "can hold only wiki_path")
		}
	}
	clearPath := slices.Contains(p.Clear, "wiki_path")
	if p.WikiPath != nil && clearPath {
		return Entity{}, invalid("wiki_path", "has a value and is also in clear")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Entity{}, fmt.Errorf("update entity: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)
	current, err := q.GetEntityForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Entity{}, ErrNotFound
	}
	if err != nil {
		return Entity{}, fmt.Errorf("update entity: %w", err)
	}
	old := entityFromRow(current)

	name := old.Name
	if p.Name != nil {
		if name, err = checkEntityName("name", *p.Name); err != nil {
			return Entity{}, err
		}
	}
	aliases := old.Aliases
	if p.Aliases != nil {
		aliases = *p.Aliases
	}
	aliases, err = cleanAliases(name, append(slices.Clone(aliases), p.AddAliases...))
	if err != nil {
		return Entity{}, err
	}
	var wikiPath *string
	switch {
	case p.WikiPath != nil:
		if wikiPath, err = checkEntityWikiPath(old.Kind, *p.WikiPath); err != nil {
			return Entity{}, err
		}
		if wikiPath == nil {
			return Entity{}, invalid("wiki_path", "must not be empty; to remove the page, put wiki_path in clear")
		}
	case !clearPath:
		wikiPath = current.WikiPath
	}
	archivedAt := current.ArchivedAt
	if p.Archived != nil {
		switch {
		case *p.Archived && archivedAt == nil:
			now := s.now().UTC()
			archivedAt = &now
		case !*p.Archived:
			archivedAt = nil
		}
	}

	same := name == old.Name && slices.Equal(aliases, old.Aliases) &&
		(wikiPath == nil) == (current.WikiPath == nil) && (wikiPath == nil || *wikiPath == *current.WikiPath) &&
		(archivedAt == nil) == (current.ArchivedAt == nil)
	if same {
		return old, nil
	}
	if err := checkNamesFree(ctx, q, old.Kind, id, name, aliases); err != nil {
		return Entity{}, err
	}
	stored, err := q.UpdateEntity(ctx, store.UpdateEntityParams{ID: id, Name: name, Aliases: aliases, WikiPath: wikiPath, ArchivedAt: archivedAt})
	if err != nil {
		return Entity{}, fmt.Errorf("update entity: %w", err)
	}
	entity := entityFromRow(stored)
	if err := recordEvent(ctx, q, "update", "entities", id, p.Actor, old, entity); err != nil {
		return Entity{}, fmt.Errorf("update entity: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Entity{}, fmt.Errorf("update entity: %w", err)
	}
	return entity, nil
}

// DeleteEntity marks one entity as deleted and appends a delete event that
// holds the row, in one transaction. The links stay; a read leaves out the
// links of a deleted entity.
func (s *Service) DeleteEntity(ctx context.Context, id int64, p DeleteParams) error {
	return s.deleteRecord(ctx, "entities", id, p, func(q *store.Queries) (any, error) {
		row, err := q.SoftDeleteEntity(ctx, id)
		if err != nil {
			return nil, err
		}
		return entityFromRow(row), nil
	})
}
