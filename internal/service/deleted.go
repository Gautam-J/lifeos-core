package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// DeletedRecord is one record that is deleted and not yet removed for
// good.
type DeletedRecord struct {
	Type string
	ID   int64
	// Summary is one line that says what the record holds.
	Summary   string
	DeletedAt time.Time
}

// ListDeleted returns the deleted records of every type, the last deleted
// first. limit 0 means 50; the maximum is 200. Purge removes one of them
// for good.
func (s *Service) ListDeleted(ctx context.Context, limit int) ([]DeletedRecord, error) {
	maxRows, err := checkLimit(limit)
	if err != nil {
		return nil, err
	}

	q := store.New(s.pool)
	records := []DeletedRecord{}
	for _, t := range recordTypes {
		more, err := t.listDeleted(ctx, q, maxRows)
		if err != nil {
			return nil, fmt.Errorf("list deleted: %w", err)
		}
		records = append(records, more...)
	}
	// Each type gave its last deleted records. Put them in one order.
	sort.SliceStable(records, func(i, j int) bool { return records[i].DeletedAt.After(records[j].DeletedAt) })
	if len(records) > int(maxRows) {
		records = records[:maxRows]
	}
	return records, nil
}
