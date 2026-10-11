package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// deleteRecord marks one row of table as deleted and appends a delete
// event that holds the row, in one transaction. remove sets deleted_at and
// returns the row as the record of its type. The row stays in the table; a
// read leaves it out.
func (s *Service) deleteRecord(ctx context.Context, table string, id int64, p DeleteParams, remove func(q *store.Queries) (any, error)) error {
	if err := p.check(); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("delete from %s: %w", table, err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	old, err := remove(q)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete from %s: %w", table, err)
	}
	if err := recordEvent(ctx, q, "delete", table, id, p.Actor, old, nil); err != nil {
		return fmt.Errorf("delete from %s: %w", table, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("delete from %s: %w", table, err)
	}
	return nil
}
