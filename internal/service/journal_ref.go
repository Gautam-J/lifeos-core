package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// checkJournalRef refuses a journal_ref that names no live journal entry,
// and returns the entry. 0 means none: it returns an empty entry. Call it
// in the transaction of the write. The foreign key refuses an unknown ID
// too, but it accepts a deleted entry, and its error does not name the
// field.
func checkJournalRef(ctx context.Context, q *store.Queries, ref int64) (store.JournalEntry, error) {
	if ref == 0 {
		return store.JournalEntry{}, nil
	}
	entry, err := q.GetJournalEntry(ctx, ref)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.JournalEntry{}, invalid("journal_ref", "must be the ID of a journal entry that exists")
	}
	if err != nil {
		return store.JournalEntry{}, fmt.Errorf("check journal_ref: %w", err)
	}
	return entry, nil
}

// journalRefParam is the value of a journal_ref column: nil for 0.
func journalRefParam(ref int64) *int64 {
	if ref == 0 {
		return nil
	}
	return &ref
}

// journalRefValue is the value of a journal_ref field: 0 for NULL.
func journalRefValue(ref *int64) int64 {
	if ref == nil {
		return 0
	}
	return *ref
}
