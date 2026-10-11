package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// Purge removes one record for good. Only the admin pages call it, so the
// change is Gautam's own. The record must be deleted already: Purge
// returns ErrNotFound for a live record and for an ID that no record has.
//
// In one transaction, Purge removes the row, empties the values of the
// earlier events of the record and appends a purge event that has no
// value. The events stay, so the log shows that the record was there.
func (s *Service) Purge(ctx context.Context, recordType string, id int64) error {
	if err := checkRecordType(recordType); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("purge: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	table, err := purgeRow(ctx, q, recordType, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("purge from %s: %w", table, err)
	}
	if err := q.EmptyEventValues(ctx, store.EmptyEventValuesParams{RecordType: table, RecordID: &id}); err != nil {
		return fmt.Errorf("purge from %s: %w", table, err)
	}
	if err := recordEvent(ctx, q, "purge", table, id, SourceManual, nil, nil); err != nil {
		return fmt.Errorf("purge from %s: %w", table, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("purge from %s: %w", table, err)
	}
	return nil
}

// purgeRow removes the row of one deleted record and returns its table.
func purgeRow(ctx context.Context, q *store.Queries, recordType string, id int64) (table string, err error) {
	switch recordType {
	case "item":
		_, err = q.PurgeCatalogItem(ctx, id)
		return "catalog_items", err
	case "journal":
		_, err = q.PurgeJournalEntry(ctx, id)
		return "journal_entries", err
	case "todo":
		_, err = q.PurgeTodo(ctx, id)
		return "todos", err
	case "habit_log":
		_, err = q.PurgeHabitLog(ctx, id)
		return "habit_logs", err
	}
	return "", errNoCode(recordType)
}
