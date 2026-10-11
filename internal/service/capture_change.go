package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// MarkProcessed records that the source was read: its facts are in the
// records and its ideas are in the wiki. It sets processed_at and returns
// the source. It changes nothing else: the content of a source stays as it
// came. A source that is marked already is not a write.
func (s *Service) MarkProcessed(ctx context.Context, id int64, actor Source) (Capture, error) {
	if err := (Provenance{Source: actor}).validate(); err != nil {
		return Capture{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Capture{}, fmt.Errorf("mark source processed: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	row, err := q.GetSourceForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Capture{}, ErrNotFound
	}
	if err != nil {
		return Capture{}, fmt.Errorf("mark source processed: %w", err)
	}
	old := captureFromRow(row)
	if old.ProcessedAt != nil {
		return old, nil
	}
	row, err = q.MarkSourceProcessed(ctx, id)
	if err != nil {
		return Capture{}, fmt.Errorf("mark source processed: %w", err)
	}
	marked := captureFromRow(row)
	if err := recordEvent(ctx, q, "process", "sources", id, actor, old, marked); err != nil {
		return Capture{}, fmt.Errorf("mark source processed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Capture{}, fmt.Errorf("mark source processed: %w", err)
	}
	return marked, nil
}

// DeleteCapture marks one source as deleted and appends a delete event that
// holds the row, in one transaction. The file of a file capture stays in
// the file store: only the admin pages remove a record for good.
func (s *Service) DeleteCapture(ctx context.Context, id int64, p DeleteParams) error {
	return s.deleteRecord(ctx, "sources", id, p, func(q *store.Queries) (any, error) {
		row, err := q.SoftDeleteSource(ctx, id)
		if err != nil {
			return nil, err
		}
		return captureFromRow(row), nil
	})
}

// summary is the one line that a search shows for a source.
func (c Capture) summary() string {
	s := c.Kind + ": "
	if c.Title != "" && c.Kind != "file" {
		s += c.Title + ": "
	}
	switch c.Kind {
	case "link":
		return s + c.URL
	case "file":
		return s + c.Title + " (" + c.MediaType + ")"
	}
	return s + shorten(c.Content, 80)
}
