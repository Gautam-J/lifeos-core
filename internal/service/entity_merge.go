package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// MergeEntitiesParams is the input of MergeEntities.
type MergeEntitiesParams struct {
	// Keep is the entity that stays. Drop is the entity that goes into it.
	Keep, Drop int64
	Actor      Source
}

// MergeResult says what MergeEntities did.
type MergeResult struct {
	Kept         Entity `json:"kept"`
	Dropped      Entity `json:"dropped"`
	LinksMoved   int    `json:"links_moved"`
	LinksDropped int    `json:"links_dropped"`
	TodosMoved   int    `json:"todos_moved"`
}

// MergeEntities moves each link and each todo project from Drop to Keep,
// adds the name and the aliases of Drop to the aliases of Keep, and
// soft-deletes Drop, in one transaction. Each change appends an event, so
// a wrong merge can be undone by hand. A link of Drop to a record that Keep
// links to already is removed. Both entities must be live and of one kind.
func (s *Service) MergeEntities(ctx context.Context, p MergeEntitiesParams) (MergeResult, error) {
	if err := (Provenance{Source: p.Actor}).validate(); err != nil {
		return MergeResult{}, err
	}
	if p.Keep == p.Drop {
		return MergeResult{}, invalid("drop", "must be a different entity from keep")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MergeResult{}, fmt.Errorf("merge entities: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	// Lock the lower ID first, so that two merges at the same moment
	// cannot wait for each other.
	var keep, drop store.Entity
	for _, id := range sortedIDs(p.Keep, p.Drop) {
		row, err := q.GetEntityForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return MergeResult{}, ErrNotFound
		}
		if err != nil {
			return MergeResult{}, fmt.Errorf("merge entities: %w", err)
		}
		if id == p.Keep {
			keep = row
		} else {
			drop = row
		}
	}
	if keep.Kind != drop.Kind {
		return MergeResult{}, invalid("drop", fmt.Sprintf("is a %s and keep is a %s: only two entities of one kind merge", drop.Kind, keep.Kind))
	}

	var result MergeResult
	// The records first, while Drop is live.
	duplicates, err := q.DeleteLinksAlreadyOnEntity(ctx, store.DeleteLinksAlreadyOnEntityParams{KeepID: keep.ID, DropID: drop.ID})
	if err != nil {
		return MergeResult{}, fmt.Errorf("merge entities: %w", err)
	}
	for _, link := range duplicates {
		recordType, recordID := linkTarget(store.RecordEntity(link))
		t, _ := lookupRecordType(recordType)
		if err := recordEvent(ctx, q, "delete", "record_entities", link.ID, p.Actor, linkEvent(drop.ID, t, recordID), nil); err != nil {
			return MergeResult{}, fmt.Errorf("merge entities: %w", err)
		}
	}
	result.LinksDropped = len(duplicates)
	moved, err := q.MoveLinks(ctx, store.MoveLinksParams{KeepID: keep.ID, DropID: drop.ID})
	if err != nil {
		return MergeResult{}, fmt.Errorf("merge entities: %w", err)
	}
	for _, link := range moved {
		recordType, recordID := linkTarget(link)
		t, _ := lookupRecordType(recordType)
		if err := recordEvent(ctx, q, "update", "record_entities", link.ID, p.Actor, linkEvent(drop.ID, t, recordID), linkEvent(keep.ID, t, recordID)); err != nil {
			return MergeResult{}, fmt.Errorf("merge entities: %w", err)
		}
	}
	result.LinksMoved = len(moved)

	if keep.Kind == "project" {
		todos, err := q.ListTodosOfProject(ctx, &drop.ID)
		if err != nil {
			return MergeResult{}, fmt.Errorf("merge entities: %w", err)
		}
		if err := q.MoveTodosOfProject(ctx, store.MoveTodosOfProjectParams{KeepID: &keep.ID, DropID: &drop.ID}); err != nil {
			return MergeResult{}, fmt.Errorf("merge entities: %w", err)
		}
		for _, row := range todos {
			old := todoFromRow(row)
			old.Project = drop.Name
			next := old
			next.Project = keep.Name
			if err := recordEvent(ctx, q, "update", "todos", row.ID, p.Actor, old, next); err != nil {
				return MergeResult{}, fmt.Errorf("merge entities: %w", err)
			}
		}
		result.TodosMoved = len(todos)
	}

	// The name and the aliases of Drop name Keep from now on.
	aliases, err := cleanAliases(keep.Name, append(append(append([]string{}, keep.Aliases...), drop.Name), drop.Aliases...))
	if err != nil {
		return MergeResult{}, err
	}
	stored, err := q.UpdateEntity(ctx, store.UpdateEntityParams{ID: keep.ID, Name: keep.Name, Aliases: aliases, WikiPath: keep.WikiPath, ArchivedAt: keep.ArchivedAt})
	if err != nil {
		return MergeResult{}, fmt.Errorf("merge entities: %w", err)
	}
	if err := recordEvent(ctx, q, "update", "entities", keep.ID, p.Actor, entityFromRow(keep), entityFromRow(stored)); err != nil {
		return MergeResult{}, fmt.Errorf("merge entities: %w", err)
	}
	gone, err := q.SoftDeleteEntity(ctx, drop.ID)
	if err != nil {
		return MergeResult{}, fmt.Errorf("merge entities: %w", err)
	}
	if err := recordEvent(ctx, q, "delete", "entities", drop.ID, p.Actor, entityFromRow(drop), nil); err != nil {
		return MergeResult{}, fmt.Errorf("merge entities: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return MergeResult{}, fmt.Errorf("merge entities: %w", err)
	}
	result.Kept, result.Dropped = entityFromRow(stored), entityFromRow(gone)
	return result, nil
}

func sortedIDs(a, b int64) []int64 {
	if a < b {
		return []int64{a, b}
	}
	return []int64{b, a}
}
