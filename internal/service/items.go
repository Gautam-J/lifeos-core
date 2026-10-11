package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// ItemKinds are the lists that catalog_items holds.
var ItemKinds = []string{"movie", "book", "place", "game"}

const maxTitleLength = 300

// Item is one row of catalog_items.
type Item struct {
	ID    int64  `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	// Rating is 0 to 5 in steps of 0.5.
	Rating *float64 `json:"rating,omitempty"`
	// OccurredOn is the day watched, read, visited or played, as YYYY-MM-DD.
	OccurredOn string         `json:"occurred_on,omitempty"`
	Attrs      map[string]any `json:"attrs"`
	Source     Source         `json:"source"`
	SourceRef  *int64         `json:"source_ref,omitempty"`
	Verified   bool           `json:"verified"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// AddItemParams is the input of AddItem.
type AddItemParams struct {
	Kind  string
	Title string
	// Rating is optional: 0 to 5 in steps of 0.5.
	Rating *float64
	// OccurredOn is optional: YYYY-MM-DD, not later than today in IST.
	OccurredOn string
	// Attrs is optional: more fields for this kind, such as author or year.
	Attrs map[string]any
	Provenance
}

// AddItem validates p, stores one catalog item and appends a create event,
// in one transaction.
func (s *Service) AddItem(ctx context.Context, p AddItemParams) (Item, error) {
	row, err := p.toRow(s.now())
	if err != nil {
		return Item{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("add item: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	stored, err := q.InsertCatalogItem(ctx, row)
	if err != nil {
		return Item{}, fmt.Errorf("add item: %w", err)
	}
	item, err := itemFromRow(stored)
	if err != nil {
		return Item{}, fmt.Errorf("add item: %w", err)
	}
	if err := recordEvent(ctx, q, "create", "catalog_items", item.ID, p.Source, nil, item); err != nil {
		return Item{}, fmt.Errorf("add item: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Item{}, fmt.Errorf("add item: %w", err)
	}
	return item, nil
}

// toRow checks every field and returns the row to insert.
func (p AddItemParams) toRow(now time.Time) (store.InsertCatalogItemParams, error) {
	var row store.InsertCatalogItemParams

	if err := checkKind(p.Kind); err != nil {
		return row, err
	}
	title := strings.TrimSpace(p.Title)
	if title == "" {
		return row, invalid("title", "must not be empty")
	}
	if utf8.RuneCountInString(title) > maxTitleLength {
		return row, invalid("title", fmt.Sprintf("must be at most %d characters", maxTitleLength))
	}
	if err := checkText("title", title); err != nil {
		return row, err
	}
	if p.Rating != nil {
		r := *p.Rating
		if math.IsNaN(r) || r < 0 || r > 5 || math.Mod(r*2, 1) != 0 {
			return row, invalid("rating", "must be from 0 to 5 in steps of 0.5")
		}
	}
	var occurredOn *time.Time
	if p.OccurredOn != "" {
		day, err := time.Parse(time.DateOnly, p.OccurredOn)
		if err != nil {
			return row, invalid("occurred_on", "must be a date as YYYY-MM-DD")
		}
		if day.Format(time.DateOnly) > now.In(IST).Format(time.DateOnly) {
			return row, invalid("occurred_on", "must not be in the future")
		}
		occurredOn = &day
	}
	if err := p.Provenance.validate(); err != nil {
		return row, err
	}
	attrs := p.Attrs
	if attrs == nil {
		attrs = map[string]any{}
	}
	if err := checkAttr(attrs); err != nil {
		return row, err
	}
	attrsJSON, err := json.Marshal(attrs)
	if err != nil {
		return row, invalid("attrs", "must be a JSON object")
	}

	return store.InsertCatalogItemParams{
		Kind:       p.Kind,
		Title:      title,
		Rating:     p.Rating,
		OccurredOn: occurredOn,
		Attrs:      attrsJSON,
		Source:     string(p.Source),
		SourceRef:  p.SourceRef,
		Verified:   p.Verified,
	}, nil
}

// checkAttr checks each name, each text and each number in an attr value,
// at any depth. A card number can come as a JSON number.
func checkAttr(value any) error {
	switch v := value.(type) {
	case string:
		return checkText("attrs", v)
	case float64:
		return checkText("attrs", strconv.FormatFloat(v, 'f', -1, 64))
	case []any:
		for _, inner := range v {
			if err := checkAttr(inner); err != nil {
				return err
			}
		}
	case map[string]any:
		for name, inner := range v {
			if err := checkText("attrs", name); err != nil {
				return err
			}
			if err := checkAttr(inner); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkKind(kind string) error {
	for _, k := range ItemKinds {
		if kind == k {
			return nil
		}
	}
	return invalid("kind", "must be one of "+strings.Join(ItemKinds, ", "))
}

func itemFromRow(row store.CatalogItem) (Item, error) {
	item := Item{
		ID:        row.ID,
		Kind:      row.Kind,
		Title:     row.Title,
		Rating:    row.Rating,
		Source:    Source(row.Source),
		SourceRef: row.SourceRef,
		Verified:  row.Verified,
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
	if row.OccurredOn != nil {
		item.OccurredOn = row.OccurredOn.Format(time.DateOnly)
	}
	if err := json.Unmarshal(row.Attrs, &item.Attrs); err != nil {
		return Item{}, fmt.Errorf("attrs of catalog item %d: %w", row.ID, err)
	}
	return item, nil
}
