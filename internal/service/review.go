package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

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

// ReviewQueue returns the records that are not verified and not deleted,
// oldest first. limit 0 means 50; the maximum is 200.
func (s *Service) ReviewQueue(ctx context.Context, limit int) ([]ReviewEntry, error) {
	if limit == 0 {
		limit = defaultListLimit
	}
	if limit < 1 || limit > maxListLimit {
		return nil, invalid("limit", fmt.Sprintf("must be from 1 to %d", maxListLimit))
	}

	rows, err := store.New(s.pool).ListUnverifiedCatalogItems(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("review queue: %w", err)
	}
	entries := make([]ReviewEntry, 0, len(rows))
	for _, row := range rows {
		item, err := itemFromRow(row)
		if err != nil {
			return nil, fmt.Errorf("review queue: %w", err)
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
		if ref.Type != "item" {
			return 0, invalid("records", fmt.Sprintf("%q is not a record type; the only type is item", ref.Type))
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
		current, err := q.GetCatalogItemForUpdate(ctx, ref.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, invalid("records", fmt.Sprintf("item %d does not exist", ref.ID))
		}
		if err != nil {
			return 0, fmt.Errorf("verify: %w", err)
		}
		if current.Verified {
			continue
		}
		old, err := itemFromRow(current)
		if err != nil {
			return 0, fmt.Errorf("verify: %w", err)
		}
		stored, err := q.VerifyCatalogItem(ctx, ref.ID)
		if err != nil {
			return 0, fmt.Errorf("verify: %w", err)
		}
		item, err := itemFromRow(stored)
		if err != nil {
			return 0, fmt.Errorf("verify: %w", err)
		}
		if err := recordEvent(ctx, q, "verify", "catalog_items", ref.ID, actor, old, item); err != nil {
			return 0, fmt.Errorf("verify: %w", err)
		}
		changed++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("verify: %w", err)
	}
	return changed, nil
}
