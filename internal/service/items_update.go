package service

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// UpdateItemParams is the input of UpdateItem. A field that is nil keeps
// its stored value.
type UpdateItemParams struct {
	Kind       *string  `json:"kind"`
	Title      *string  `json:"title"`
	Rating     *float64 `json:"rating"`
	OccurredOn *string  `json:"occurred_on"`
	// Attrs replaces every attr when it is not nil.
	Attrs map[string]any `json:"attrs"`
	// Clear names the optional fields to empty: rating, occurred_on.
	Clear []string `json:"-"`
	// Actor is who makes the change. It goes into the event; the source
	// of the row stays as it is.
	Actor Source `json:"-"`
	// Inferred is true when an agent makes a change that Gautam did not
	// state. The row then waits in the review queue again.
	Inferred bool `json:"-"`
}

// clearableItemFields are the optional fields that Clear can name.
var clearableItemFields = []string{"rating", "occurred_on"}

// UpdateItem changes the given fields of one catalog item and appends an
// update event that holds the old row, in one transaction. It locks the
// row first, so two updates at the same moment do not lose a field.
func (s *Service) UpdateItem(ctx context.Context, id int64, p UpdateItemParams) (Item, error) {
	if err := p.check(); err != nil {
		return Item{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("update item: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	current, err := q.GetCatalogItemForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("update item: %w", err)
	}
	old, err := itemFromRow(current)
	if err != nil {
		return Item{}, fmt.Errorf("update item: %w", err)
	}

	// The changed row goes through the same checks as a new row.
	row, err := p.applyTo(old).toRow(s.now())
	if err != nil {
		return Item{}, err
	}
	stored, err := q.UpdateCatalogItem(ctx, store.UpdateCatalogItemParams{
		ID:         id,
		Kind:       row.Kind,
		Title:      row.Title,
		Rating:     row.Rating,
		OccurredOn: row.OccurredOn,
		Attrs:      row.Attrs,
		Verified:   row.Verified,
	})
	if err != nil {
		return Item{}, fmt.Errorf("update item: %w", err)
	}
	item, err := itemFromRow(stored)
	if err != nil {
		return Item{}, fmt.Errorf("update item: %w", err)
	}
	if err := recordEvent(ctx, q, "update", "catalog_items", id, p.Actor, old, item); err != nil {
		return Item{}, fmt.Errorf("update item: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Item{}, fmt.Errorf("update item: %w", err)
	}
	return item, nil
}

// check refuses input that is wrong for every stored row.
func (p UpdateItemParams) check() error {
	if err := (Provenance{Source: p.Actor}).validate(); err != nil {
		return err
	}
	for _, name := range p.Clear {
		if !slices.Contains(clearableItemFields, name) {
			return invalid("clear", "can hold only rating, occurred_on")
		}
	}
	if p.Rating != nil && slices.Contains(p.Clear, "rating") {
		return invalid("rating", "has a value and is also in clear")
	}
	if p.OccurredOn != nil {
		if slices.Contains(p.Clear, "occurred_on") {
			return invalid("occurred_on", "has a value and is also in clear")
		}
		if *p.OccurredOn == "" {
			return invalid("occurred_on", "must be a date as YYYY-MM-DD; to remove the date, put occurred_on in clear")
		}
	}
	return nil
}

// applyTo returns old with the changes of p, as the input of the checks
// that a new item gets.
func (p UpdateItemParams) applyTo(old Item) AddItemParams {
	next := AddItemParams{
		Kind:       old.Kind,
		Title:      old.Title,
		Rating:     old.Rating,
		OccurredOn: old.OccurredOn,
		Attrs:      old.Attrs,
		Provenance: Provenance{Source: old.Source, SourceRef: old.SourceRef, Verified: old.Verified},
	}
	if p.Kind != nil {
		next.Kind = *p.Kind
	}
	if p.Title != nil {
		next.Title = *p.Title
	}
	if p.Rating != nil {
		next.Rating = p.Rating
	}
	if p.OccurredOn != nil {
		next.OccurredOn = *p.OccurredOn
	}
	if p.Attrs != nil {
		next.Attrs = p.Attrs
	}
	if slices.Contains(p.Clear, "rating") {
		next.Rating = nil
	}
	if slices.Contains(p.Clear, "occurred_on") {
		next.OccurredOn = ""
	}
	if p.Inferred {
		next.Verified = false
	}
	return next
}
