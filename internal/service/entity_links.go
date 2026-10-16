package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

const (
	maxTagRefs         = 20
	maxEntityRecords   = 100
	entityRefWhatItIs  = "an entity as {id}, or as {name} (and {kind}, to make it when there is none)"
	bareNameNoMatchMsg = "matches no entity: give its kind (person, place, project or topic) and the entity is made"
)

// recordLink holds the column of record_entities that one record uses. Only
// one field is set.
type recordLink struct {
	journal, todo, item, habitLog, source *int64
}

// EntityRef names an entity in the input of a write: by ID, or by name. A
// name with a kind makes the entity when there is none (decision 1 of #41);
// a name with no kind names an entity that exists.
type EntityRef struct {
	ID   int64  `json:"id,omitempty" jsonschema:"The ID of an entity that exists."`
	Name string `json:"name,omitempty" jsonschema:"The name or an alias of the entity."`
	Kind string `json:"kind,omitempty" jsonschema:"person, place, project or topic. With a kind, a name that matches no entity makes a new one."`
}

// EntityLink is one entity that a write linked to a record, or removed from
// it. The result of each write lists them, so a wrong link is easy to see.
type EntityLink struct {
	Ref  string `json:"ref"`
	Kind string `json:"kind"`
	Name string `json:"name"`
	// NewEntity says that this call made the entity.
	NewEntity bool `json:"new_entity,omitempty"`
	// AlreadyLinked says that the link existed: this call wrote nothing.
	AlreadyLinked bool `json:"already_linked,omitempty"`
}

func entityLinkOf(e store.Entity) EntityLink {
	return EntityLink{Ref: fmt.Sprintf("entity:%d", e.ID), Kind: e.Kind, Name: e.Name}
}

// LinkedRecord is one record that links to an entity.
type LinkedRecord struct {
	Type     string    `json:"type"`
	ID       int64     `json:"id"`
	Ref      string    `json:"ref"`
	Summary  string    `json:"summary"`
	LinkedAt time.Time `json:"linked_at"`
}

// EntityDetail is an entity with the records that link to it, the newest
// link first. get returns it for the type entity.
type EntityDetail struct {
	Entity
	Records []LinkedRecord `json:"records"`
}

// GetEntityDetail returns the entity and the newest of its records
// (maxEntityRecords). A deleted record is left out.
func (s *Service) GetEntityDetail(ctx context.Context, id int64) (EntityDetail, error) {
	entity, err := s.GetEntity(ctx, id)
	if err != nil {
		return EntityDetail{}, err
	}
	links, err := store.New(s.pool).ListLinksOfEntity(ctx, id)
	if err != nil {
		return EntityDetail{}, fmt.Errorf("get entity: %w", err)
	}
	detail := EntityDetail{Entity: entity, Records: []LinkedRecord{}}
	for _, link := range links {
		if len(detail.Records) == maxEntityRecords {
			break
		}
		name, recordID := linkTarget(link)
		rec, err := s.Get(ctx, name, recordID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return EntityDetail{}, fmt.Errorf("get entity: %w", err)
		}
		detail.Records = append(detail.Records, LinkedRecord{
			Type: name, ID: recordID, Ref: refOfRecord(name, rec), Summary: summaryOf(rec), LinkedAt: link.CreatedAt.UTC(),
		})
	}
	return detail, nil
}

// linkTarget returns the record type and the ID that a link names.
func linkTarget(link store.RecordEntity) (recordType string, id int64) {
	switch {
	case link.JournalID != nil:
		return "journal", *link.JournalID
	case link.TodoID != nil:
		return "todo", *link.TodoID
	case link.ItemID != nil:
		return "item", *link.ItemID
	case link.HabitLogID != nil:
		return "habit_log", *link.HabitLogID
	default:
		return "source", *link.SourceID
	}
}

// refOfRecord is the ref of a record, such as journal:57 or movie:3: the
// prefix that a wiki page cites.
func refOfRecord(recordType string, rec any) string {
	t, _ := lookupRecordType(recordType)
	if item, ok := rec.(Item); ok {
		return fmt.Sprintf("%s:%d", item.Kind, item.ID)
	}
	var id int64
	switch r := rec.(type) {
	case JournalEntry:
		id = r.ID
	case Todo:
		id = r.ID
	case HabitLog:
		id = r.ID
	case Capture:
		id = r.ID
	case Entity:
		id = r.ID
	}
	return fmt.Sprintf("%s:%d", t.prefixes[0], id)
}

// summaryOf is the one line that a record shows in a list.
func summaryOf(rec any) string {
	if s, ok := rec.(interface{ summary() string }); ok {
		return s.summary()
	}
	return ""
}

// TagParams is the input of Tag.
type TagParams struct {
	// Type and ID name the record. Every record type but entity can be
	// tagged.
	Type string
	ID   int64
	// Add links the record to these entities.
	Add []EntityRef
	// Remove takes the links of these entities away.
	Remove []EntityRef
	Actor  Source
}

// TagResult lists what Tag did.
type TagResult struct {
	Added   []EntityLink `json:"added,omitempty"`
	Removed []EntityLink `json:"removed,omitempty"`
}

// Tag links a record to entities and takes links away, in one transaction.
// An entity that a name with a kind names, and that does not exist, is
// made. The same link a second time is not a write: it appends no event.
func (s *Service) Tag(ctx context.Context, p TagParams) (TagResult, error) {
	if err := (Provenance{Source: p.Actor}).validate(); err != nil {
		return TagResult{}, err
	}
	t, err := lookupRecordType(p.Type)
	if err != nil {
		return TagResult{}, err
	}
	if t.link == nil {
		return TagResult{}, invalid("type", "a "+t.name+" cannot be tagged")
	}
	if len(p.Add)+len(p.Remove) == 0 {
		return TagResult{}, invalid("add", "must name at least one entity to add, or one in remove")
	}
	if len(p.Add) > maxTagRefs || len(p.Remove) > maxTagRefs {
		return TagResult{}, invalid("add", fmt.Sprintf("must have at most %d entities", maxTagRefs))
	}
	if _, err := t.get(s, ctx, p.ID); err != nil {
		return TagResult{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return TagResult{}, fmt.Errorf("tag: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	var result TagResult
	if result.Removed, err = unlinkEntities(ctx, q, t, p.ID, p.Remove, p.Actor); err != nil {
		return TagResult{}, err
	}
	if result.Added, err = linkEntities(ctx, q, t, p.ID, p.Add, p.Actor); err != nil {
		return TagResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TagResult{}, fmt.Errorf("tag: %w", err)
	}
	return result, nil
}

// resolveRefs turns each reference into an entity. A reference with an ID or
// with a kind is exact. A name with no kind may match entities of different
// kinds: the entity that most records link to together with the exact
// entities of this call wins, and the oldest entity breaks a tie.
func resolveRefs(ctx context.Context, q *store.Queries, refs []EntityRef, create bool, actor Source) (entities []store.Entity, created []bool, err error) {
	entities = make([]store.Entity, len(refs))
	created = make([]bool, len(refs))
	var bare []int
	for i, ref := range refs {
		switch {
		case ref.ID != 0:
			row, err := q.GetEntity(ctx, ref.ID)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil, invalid("entities", fmt.Sprintf("entity %d does not exist", ref.ID))
			}
			if err != nil {
				return nil, nil, fmt.Errorf("find the entity %d: %w", ref.ID, err)
			}
			entities[i] = row
		case ref.Name == "":
			return nil, nil, invalid("entities", "needs "+entityRefWhatItIs)
		case ref.Kind != "":
			if err := checkEntityKind("entities", ref.Kind); err != nil {
				return nil, nil, err
			}
			name, err := checkEntityName("entities", ref.Name)
			if err != nil {
				return nil, nil, err
			}
			if create {
				row, isNew, err := ensureEntity(ctx, q, ref.Kind, name, actor)
				if err != nil {
					return nil, nil, err
				}
				entities[i], created[i] = row, isNew
				break
			}
			row, found, err := findEntity(ctx, q, ref.Kind, name)
			if err != nil {
				return nil, nil, err
			}
			if !found {
				return nil, nil, invalid("entities", fmt.Sprintf("%s %q matches no entity", ref.Kind, name))
			}
			entities[i] = row
		default:
			bare = append(bare, i)
		}
	}
	var exact []int64
	for i, e := range entities {
		if e.ID != 0 {
			exact = append(exact, e.ID)
		}
		_ = i
	}
	for _, i := range bare {
		name, err := checkEntityName("entities", refs[i].Name)
		if err != nil {
			return nil, nil, err
		}
		rows, err := q.FindEntitiesByName(ctx, store.FindEntitiesByNameParams{Name: name})
		if err != nil {
			return nil, nil, fmt.Errorf("find the entity %q: %w", name, err)
		}
		switch len(rows) {
		case 0:
			return nil, nil, invalid("entities", fmt.Sprintf("%q %s", name, bareNameNoMatchMsg))
		case 1:
			entities[i] = rows[0]
		default:
			if entities[i], err = mostLinked(ctx, q, rows, exact); err != nil {
				return nil, nil, err
			}
		}
		exact = append(exact, entities[i].ID)
	}
	return entities, created, nil
}

// mostLinked picks the candidate that most records link to together with
// the other entities. The first candidate (the oldest) wins a tie.
func mostLinked(ctx context.Context, q *store.Queries, candidates []store.Entity, others []int64) (store.Entity, error) {
	best, bestScore := candidates[0], int64(-1)
	for _, c := range candidates {
		var score int64
		if len(others) > 0 {
			var err error
			score, err = q.CountCoLinks(ctx, store.CountCoLinksParams{EntityID: c.ID, Others: others})
			if err != nil {
				return store.Entity{}, fmt.Errorf("choose between entities: %w", err)
			}
		}
		if score > bestScore {
			best, bestScore = c, score
		}
	}
	return best, nil
}

func (l recordLink) insertParams(entityID int64, actor Source) store.InsertRecordEntityParams {
	return store.InsertRecordEntityParams{EntityID: entityID, JournalID: l.journal, TodoID: l.todo, ItemID: l.item, HabitLogID: l.habitLog, SourceID: l.source, Source: string(actor)}
}

// linkEvent is the value of the event of a link.
func linkEvent(entityID int64, t *recordType, recordID int64) map[string]any {
	return map[string]any{"entity_id": entityID, "record_type": t.name, "record_id": recordID}
}

// linkEntities links one record to the entities that refs name. Call it in
// a transaction.
func linkEntities(ctx context.Context, q *store.Queries, t *recordType, recordID int64, refs []EntityRef, actor Source) ([]EntityLink, error) {
	entities, created, err := resolveRefs(ctx, q, refs, true, actor)
	if err != nil {
		return nil, err
	}
	var links []EntityLink
	for i, e := range entities {
		link := entityLinkOf(e)
		link.NewEntity = created[i]
		if slices.ContainsFunc(links, func(l EntityLink) bool { return l.Ref == link.Ref }) {
			continue
		}
		row, err := q.InsertRecordEntity(ctx, t.link(recordID).insertParams(e.ID, actor))
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			link.AlreadyLinked = true
		case err != nil:
			return nil, fmt.Errorf("link the entity %d: %w", e.ID, err)
		default:
			if err := recordEvent(ctx, q, "create", "record_entities", row.ID, actor, nil, linkEvent(e.ID, t, recordID)); err != nil {
				return nil, fmt.Errorf("link the entity %d: %w", e.ID, err)
			}
		}
		links = append(links, link)
	}
	return links, nil
}

// unlinkEntities takes the links of the entities that refs name away from one
// record. A link that is not there is not a write. Call it in a transaction.
func unlinkEntities(ctx context.Context, q *store.Queries, t *recordType, recordID int64, refs []EntityRef, actor Source) ([]EntityLink, error) {
	entities, _, err := resolveRefs(ctx, q, refs, false, actor)
	if err != nil {
		return nil, err
	}
	var links []EntityLink
	for _, e := range entities {
		l := t.link(recordID)
		row, err := q.DeleteRecordEntity(ctx, store.DeleteRecordEntityParams{EntityID: e.ID, JournalID: l.journal, TodoID: l.todo, ItemID: l.item, HabitLogID: l.habitLog, SourceID: l.source})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("remove the link of the entity %d: %w", e.ID, err)
		}
		if err := recordEvent(ctx, q, "delete", "record_entities", row.ID, actor, linkEvent(e.ID, t, recordID), nil); err != nil {
			return nil, fmt.Errorf("remove the link of the entity %d: %w", e.ID, err)
		}
		links = append(links, entityLinkOf(e))
	}
	return links, nil
}

// removeLinksOf deletes every link of a record that Purge removes, and
// appends a delete event for each.
func removeLinksOf(ctx context.Context, q *store.Queries, t *recordType, recordID int64) error {
	l := t.link(recordID)
	rows, err := q.DeleteLinksOfRecord(ctx, store.DeleteLinksOfRecordParams{JournalID: l.journal, TodoID: l.todo, ItemID: l.item, HabitLogID: l.habitLog, SourceID: l.source})
	if err != nil {
		return fmt.Errorf("remove the links of %s %d: %w", t.name, recordID, err)
	}
	for _, row := range rows {
		if err := recordEvent(ctx, q, "delete", "record_entities", row.ID, SourceManual, linkEvent(row.EntityID, t, recordID), nil); err != nil {
			return fmt.Errorf("remove the links of %s %d: %w", t.name, recordID, err)
		}
	}
	return nil
}

// EntitiesOf returns the live entities that one record links to.
func (s *Service) EntitiesOf(ctx context.Context, recordType string, id int64) ([]Entity, error) {
	t, err := lookupRecordType(recordType)
	if err != nil {
		return nil, err
	}
	if t.link == nil {
		return nil, invalid("type", "a "+t.name+" has no entities")
	}
	return entitiesOfRecord(ctx, store.New(s.pool), t, id)
}

func entitiesOfRecord(ctx context.Context, q *store.Queries, t *recordType, id int64) ([]Entity, error) {
	l := t.link(id)
	rows, err := q.ListEntitiesOfRecord(ctx, store.ListEntitiesOfRecordParams{JournalID: l.journal, TodoID: l.todo, ItemID: l.item, HabitLogID: l.habitLog, SourceID: l.source})
	if err != nil {
		return nil, fmt.Errorf("entities of %s %d: %w", t.name, id, err)
	}
	entities := make([]Entity, len(rows))
	for i, row := range rows {
		entities[i] = entityFromRow(row.Entity)
	}
	return entities, nil
}
