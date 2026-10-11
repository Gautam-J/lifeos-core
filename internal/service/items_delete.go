package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// DeleteParams is the input of a delete.
type DeleteParams struct {
	// Actor is who deletes the record. It goes into the event.
	Actor Source
	// Confirmed is true when Gautam saw the record and said yes to the
	// delete. An agent must not delete without it.
	Confirmed bool
}

func (p DeleteParams) check() error {
	if err := (Provenance{Source: p.Actor}).validate(); err != nil {
		return err
	}
	if p.Actor == SourceAgent && !p.Confirmed {
		return invalid("confirmed", "must be true: show the record to the user, and delete it only after the user says yes")
	}
	return nil
}

// DeleteItem marks one catalog item as deleted and appends a delete event
// that holds the row, in one transaction. The row stays in the table; a
// read leaves it out.
func (s *Service) DeleteItem(ctx context.Context, id int64, p DeleteParams) error {
	if err := p.check(); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	row, err := q.SoftDeleteCatalogItem(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	old, err := itemFromRow(row)
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	if err := recordEvent(ctx, q, "delete", "catalog_items", id, p.Actor, old, nil); err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	return nil
}
