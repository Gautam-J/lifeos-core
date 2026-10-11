package service

import (
	"context"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

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
