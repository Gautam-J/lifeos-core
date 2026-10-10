package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// GetItem returns the catalog item with this ID. It returns ErrNotFound when
// there is no such item or the item is deleted.
func (s *Service) GetItem(ctx context.Context, id int64) (Item, error) {
	row, err := store.New(s.pool).GetCatalogItem(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("get item: %w", err)
	}
	return itemFromRow(row)
}

// ListItemsParams is the input of ListItems.
type ListItemsParams struct {
	// Kind is optional: an empty string lists every kind.
	Kind string
	// Limit is optional: 0 means 50. The maximum is 200.
	Limit int
}

// ListItems returns the catalog items that are not deleted, newest first.
func (s *Service) ListItems(ctx context.Context, p ListItemsParams) ([]Item, error) {
	if p.Limit == 0 {
		p.Limit = defaultListLimit
	}
	if p.Limit < 1 || p.Limit > maxListLimit {
		return nil, invalid("limit", fmt.Sprintf("must be from 1 to %d", maxListLimit))
	}
	arg := store.ListCatalogItemsParams{MaxRows: int32(p.Limit)}
	if p.Kind != "" {
		if err := checkKind(p.Kind); err != nil {
			return nil, err
		}
		arg.Kind = &p.Kind
	}

	rows, err := store.New(s.pool).ListCatalogItems(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		item, err := itemFromRow(row)
		if err != nil {
			return nil, fmt.Errorf("list items: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}
