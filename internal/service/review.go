package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// RecordRef names one record by its type and its ID.
type RecordRef struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
}

// ReviewEntry is one record that waits for Gautam's approval.
type ReviewEntry struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
	// Summary is one line that says what the record holds.
	Summary   string    `json:"summary"`
	Source    Source    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const maxVerifyRecords = 200

// ReviewQueue returns the records of every type that are not verified and
// not deleted, oldest first. limit 0 means 50; the maximum is 200.
func (s *Service) ReviewQueue(ctx context.Context, limit int) ([]ReviewEntry, error) {
	maxRows, err := checkLimit(limit)
	if err != nil {
		return nil, err
	}

	q := store.New(s.pool)
	entries := []ReviewEntry{}
	for _, unverified := range []func(context.Context, *store.Queries, int32) ([]ReviewEntry, error){
		unverifiedItems,
		unverifiedJournalEntries,
		unverifiedTodos,
		unverifiedHabitLogs,
	} {
		more, err := unverified(ctx, q, maxRows)
		if err != nil {
			return nil, fmt.Errorf("review queue: %w", err)
		}
		entries = append(entries, more...)
	}
	// Each type gave its oldest records. Put them in one order.
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].CreatedAt.Before(entries[j].CreatedAt) })
	if len(entries) > int(maxRows) {
		entries = entries[:maxRows]
	}
	return entries, nil
}

// unverifiedItems returns the items that wait for review, oldest first.
func unverifiedItems(ctx context.Context, q *store.Queries, limit int32) ([]ReviewEntry, error) {
	rows, err := q.ListUnverifiedCatalogItems(ctx, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]ReviewEntry, 0, len(rows))
	for _, row := range rows {
		item, err := itemFromRow(row)
		if err != nil {
			return nil, err
		}
		entries = append(entries, ReviewEntry{
			Type:      "item",
			ID:        item.ID,
			Summary:   item.summary(),
			Source:    item.Source,
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
		})
	}
	return entries, nil
}

// summary is the one line that the review queue shows for an item.
func (i Item) summary() string {
	s := i.Kind + ": " + i.Title
	if i.Rating != nil {
		s += ", rating " + strconv.FormatFloat(*i.Rating, 'f', -1, 64)
	}
	if i.OccurredOn != "" {
		s += ", on " + i.OccurredOn
	}
	return s
}

// shorten returns text as one line of at most max characters.
func shorten(text string, max int) string {
	line := strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(line) <= max {
		return line
	}
	return strings.TrimSpace(string([]rune(line)[:max])) + "…"
}

// Verify marks the records as verified and appends one verify event for
// each record that it changes, in one transaction. A record that is
// verified already is left as it is. When one record does not exist, Verify
// changes nothing. It returns the number of records that it changed.
func (s *Service) Verify(ctx context.Context, refs []RecordRef, actor Source) (int, error) {
	if err := (Provenance{Source: actor}).validate(); err != nil {
		return 0, err
	}
	if len(refs) == 0 {
		return 0, invalid("records", "must name at least one record")
	}
	if len(refs) > maxVerifyRecords {
		return 0, invalid("records", fmt.Sprintf("must name at most %d records", maxVerifyRecords))
	}
	for _, ref := range refs {
		if !slices.Contains(RecordTypes, ref.Type) {
			return 0, invalid("records", fmt.Sprintf("%q is not a record type; the types are: %s", ref.Type, strings.Join(RecordTypes, ", ")))
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("verify: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	changed := 0
	for _, ref := range refs {
		table, before, after, err := verifyRecord(ctx, q, ref)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, invalid("records", fmt.Sprintf("%s %d does not exist", ref.Type, ref.ID))
		}
		if err != nil {
			return 0, fmt.Errorf("verify: %w", err)
		}
		if before == nil {
			continue
		}
		if err := recordEvent(ctx, q, "verify", table, ref.ID, actor, before, after); err != nil {
			return 0, fmt.Errorf("verify: %w", err)
		}
		changed++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("verify: %w", err)
	}
	return changed, nil
}

// verifyRecord locks one record and sets verified when it is not set. It
// returns the table of the record, and the record before and after the
// change. before is nil when the record is verified already.
func verifyRecord(ctx context.Context, q *store.Queries, ref RecordRef) (table string, before, after any, err error) {
	switch ref.Type {
	case "item":
		before, after, err = verifyItem(ctx, q, ref.ID)
		return "catalog_items", before, after, err
	case "journal":
		before, after, err = verifyJournalEntry(ctx, q, ref.ID)
		return "journal_entries", before, after, err
	case "todo":
		before, after, err = verifyTodo(ctx, q, ref.ID)
		return "todos", before, after, err
	case "habit_log":
		before, after, err = verifyHabitLog(ctx, q, ref.ID)
		return "habit_logs", before, after, err
	}
	return "", nil, nil, errNoCode(ref.Type)
}

func verifyItem(ctx context.Context, q *store.Queries, id int64) (before, after any, err error) {
	current, err := q.GetCatalogItemForUpdate(ctx, id)
	if err != nil || current.Verified {
		return nil, nil, err
	}
	stored, err := q.VerifyCatalogItem(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	old, err := itemFromRow(current)
	if err != nil {
		return nil, nil, err
	}
	item, err := itemFromRow(stored)
	if err != nil {
		return nil, nil, err
	}
	return old, item, nil
}
