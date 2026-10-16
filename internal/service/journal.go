package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// JournalEntry is one row of journal_entries.
type JournalEntry struct {
	ID int64 `json:"id"`
	// Body is the entry exactly as Gautam wrote it.
	Body string `json:"body"`
	// EntryDate is the IST day that the entry is about, as YYYY-MM-DD.
	EntryDate string    `json:"entry_date"`
	Source    Source    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AddJournalEntryParams is the input of AddJournalEntry.
type AddJournalEntryParams struct {
	// Body is stored as it is. The service does not trim it.
	Body string
	// EntryDate is optional: YYYY-MM-DD, not later than today in IST. An
	// empty value means today in IST.
	EntryDate string
	Provenance
}

// AddJournalEntry validates p, stores one journal entry and appends a
// create event, in one transaction.
func (s *Service) AddJournalEntry(ctx context.Context, p AddJournalEntryParams) (JournalEntry, error) {
	row, err := p.toRow(s.now())
	if err != nil {
		return JournalEntry{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return JournalEntry{}, fmt.Errorf("add journal entry: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	stored, err := q.InsertJournalEntry(ctx, row)
	if err != nil {
		return JournalEntry{}, fmt.Errorf("add journal entry: %w", err)
	}
	entry := journalEntryFromRow(stored)
	if err := recordEvent(ctx, q, "create", "journal_entries", entry.ID, p.Source, nil, entry); err != nil {
		return JournalEntry{}, fmt.Errorf("add journal entry: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return JournalEntry{}, fmt.Errorf("add journal entry: %w", err)
	}
	return entry, nil
}

// toRow checks every field and returns the row to insert.
func (p AddJournalEntryParams) toRow(now time.Time) (store.InsertJournalEntryParams, error) {
	var row store.InsertJournalEntryParams

	if strings.TrimSpace(p.Body) == "" {
		return row, invalid("body", "must not be empty")
	}
	if err := checkText("body", p.Body); err != nil {
		return row, err
	}
	day, err := pastDay("entry_date", p.EntryDate, now)
	if err != nil {
		return row, err
	}
	if err := p.Provenance.validate(); err != nil {
		return row, err
	}
	return store.InsertJournalEntryParams{
		Body:      p.Body,
		EntryDate: day,
		Source:    string(p.Source),
	}, nil
}

func journalEntryFromRow(row store.JournalEntry) JournalEntry {
	return JournalEntry{
		ID:        row.ID,
		Body:      row.Body,
		EntryDate: row.EntryDate.Format(time.DateOnly),
		Source:    Source(row.Source),
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
}

// GetJournalEntry returns the journal entry with this ID. It returns
// ErrNotFound when there is no such entry or the entry is deleted.
func (s *Service) GetJournalEntry(ctx context.Context, id int64) (JournalEntry, error) {
	row, err := store.New(s.pool).GetJournalEntry(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return JournalEntry{}, ErrNotFound
	}
	if err != nil {
		return JournalEntry{}, fmt.Errorf("get journal entry: %w", err)
	}
	return journalEntryFromRow(row), nil
}

// ListJournalEntries returns the journal entries that are not deleted, the
// newest day first. limit 0 means 50; the maximum is 200.
func (s *Service) ListJournalEntries(ctx context.Context, limit int) ([]JournalEntry, error) {
	maxRows, err := checkLimit(limit)
	if err != nil {
		return nil, err
	}
	rows, err := store.New(s.pool).ListJournalEntries(ctx, maxRows)
	if err != nil {
		return nil, fmt.Errorf("list journal entries: %w", err)
	}
	entries := make([]JournalEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, journalEntryFromRow(row))
	}
	return entries, nil
}
