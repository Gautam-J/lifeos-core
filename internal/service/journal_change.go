package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// UpdateJournalEntryParams is the input of UpdateJournalEntry. A field
// that is nil keeps its stored value.
type UpdateJournalEntryParams struct {
	// Body is stored as it is, the same as the body of a new entry.
	Body      *string `json:"body"`
	EntryDate *string `json:"entry_date"`
	// Actor is who makes the change. It goes into the event; the source
	// of the row stays as it is.
	Actor Source `json:"-"`
	// Inferred is true when an agent makes a change that Gautam did not
	// state. The service refuses such a change to a verified row.
	Inferred bool `json:"-"`
}

// UpdateJournalEntry changes the given fields of one journal entry and
// appends an update event that holds the old row, in one transaction. A
// call that changes no value writes nothing and appends no event.
func (s *Service) UpdateJournalEntry(ctx context.Context, id int64, p UpdateJournalEntryParams) (JournalEntry, error) {
	if err := (Provenance{Source: p.Actor}).validate(); err != nil {
		return JournalEntry{}, err
	}
	if p.Body == nil && p.EntryDate == nil {
		return JournalEntry{}, invalid("fields", "must name at least one field to change")
	}
	if p.EntryDate != nil && *p.EntryDate == "" {
		return JournalEntry{}, invalid("entry_date", "must be a date as YYYY-MM-DD")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return JournalEntry{}, fmt.Errorf("update journal entry: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	current, err := q.GetJournalEntryForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return JournalEntry{}, ErrNotFound
	}
	if err != nil {
		return JournalEntry{}, fmt.Errorf("update journal entry: %w", err)
	}
	old := journalEntryFromRow(current)

	// The changed entry goes through the same checks as a new entry.
	next := AddJournalEntryParams{
		Body:       old.Body,
		EntryDate:  old.EntryDate,
		Provenance: Provenance{Source: old.Source, SourceRef: old.SourceRef, Verified: old.Verified},
	}
	if p.Body != nil {
		next.Body = *p.Body
	}
	if p.EntryDate != nil {
		next.EntryDate = *p.EntryDate
	}
	row, err := next.toRow(s.now())
	if err != nil {
		return JournalEntry{}, err
	}
	if next.Body == old.Body && next.EntryDate == old.EntryDate {
		return old, nil
	}
	// The words are Gautam's own. An agent must not reword them.
	if p.Inferred && old.Verified {
		return JournalEntry{}, invalid("dictated", "must be true to change a verified record: ask the user to confirm the change, then send it again")
	}
	stored, err := q.UpdateJournalEntry(ctx, store.UpdateJournalEntryParams{ID: id, Body: row.Body, EntryDate: row.EntryDate})
	if err != nil {
		return JournalEntry{}, fmt.Errorf("update journal entry: %w", err)
	}
	entry := journalEntryFromRow(stored)
	if err := recordEvent(ctx, q, "update", "journal_entries", id, p.Actor, old, entry); err != nil {
		return JournalEntry{}, fmt.Errorf("update journal entry: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return JournalEntry{}, fmt.Errorf("update journal entry: %w", err)
	}
	return entry, nil
}

// DeleteJournalEntry marks one journal entry as deleted and appends a
// delete event that holds the row, in one transaction.
func (s *Service) DeleteJournalEntry(ctx context.Context, id int64, p DeleteParams) error {
	return s.deleteRecord(ctx, "journal_entries", id, p, func(q *store.Queries) (any, error) {
		row, err := q.SoftDeleteJournalEntry(ctx, id)
		if err != nil {
			return nil, err
		}
		return journalEntryFromRow(row), nil
	})
}

// summary is the one line that the review queue shows for a journal entry.
func (e JournalEntry) summary() string {
	return "journal " + e.EntryDate + ": " + shorten(e.Body, 80)
}

// unverifiedJournalEntries returns the journal entries that wait for
// review, oldest first.
func unverifiedJournalEntries(ctx context.Context, q *store.Queries, limit int32) ([]ReviewEntry, error) {
	rows, err := q.ListUnverifiedJournalEntries(ctx, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]ReviewEntry, 0, len(rows))
	for _, row := range rows {
		entry := journalEntryFromRow(row)
		entries = append(entries, ReviewEntry{
			Type:      "journal",
			ID:        entry.ID,
			Summary:   entry.summary(),
			Source:    entry.Source,
			CreatedAt: entry.CreatedAt,
			UpdatedAt: entry.UpdatedAt,
		})
	}
	return entries, nil
}

func verifyJournalEntry(ctx context.Context, q *store.Queries, id int64) (before, after any, err error) {
	current, err := q.GetJournalEntryForUpdate(ctx, id)
	if err != nil || current.Verified {
		return nil, nil, err
	}
	stored, err := q.VerifyJournalEntry(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return journalEntryFromRow(current), journalEntryFromRow(stored), nil
}
