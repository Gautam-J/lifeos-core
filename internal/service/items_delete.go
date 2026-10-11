package service

import (
	"context"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// DeleteParams is the input of a delete.
type DeleteParams struct {
	// Actor is who deletes the record. It goes into the event.
	Actor Source
	// Confirmed is true when Gautam saw the record and said yes to the
	// delete. An agent must not delete without it.
	Confirmed bool
}

func (p DeleteParams) check() error {
	if err := (Provenance{Source: p.Actor}).validate(); err != nil {
		return err
	}
	if p.Actor == SourceAgent && !p.Confirmed {
		return invalid("confirmed", "must be true: show the record to the user, and delete it only after the user says yes")
	}
	return nil
}

// DeleteItem marks one catalog item as deleted and appends a delete event
// that holds the row, in one transaction. The row stays in the table; a
// read leaves it out.
func (s *Service) DeleteItem(ctx context.Context, id int64, p DeleteParams) error {
	return s.deleteRecord(ctx, "catalog_items", id, p, func(q *store.Queries) (any, error) {
		row, err := q.SoftDeleteCatalogItem(ctx, id)
		if err != nil {
			return nil, err
		}
		return itemFromRow(row)
	})
}
