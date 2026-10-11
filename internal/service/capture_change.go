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
