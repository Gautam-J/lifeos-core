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
	for _, deleted := range []func(context.Context, *store.Queries, int32) ([]DeletedRecord, error){
		deletedItems,
		deletedJournalEntries,
		deletedTodos,
		deletedHabitLogs,
		deletedSources,
	} {
		more, err := deleted(ctx, q, maxRows)
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

func deletedRecord(recordType string, id int64, summary string, deletedAt *time.Time) DeletedRecord {
	return DeletedRecord{Type: recordType, ID: id, Summary: summary, DeletedAt: *deletedAt}
}

func deletedItems(ctx context.Context, q *store.Queries, limit int32) ([]DeletedRecord, error) {
	rows, err := q.ListDeletedCatalogItems(ctx, limit)
	if err != nil {
		return nil, err
	}
	records := make([]DeletedRecord, 0, len(rows))
	for _, row := range rows {
		item, err := itemFromRow(row)
		if err != nil {
			return nil, err
		}
		records = append(records, deletedRecord("item", row.ID, item.summary(), row.DeletedAt))
	}
	return records, nil
}

func deletedJournalEntries(ctx context.Context, q *store.Queries, limit int32) ([]DeletedRecord, error) {
	rows, err := q.ListDeletedJournalEntries(ctx, limit)
	if err != nil {
		return nil, err
	}
	records := make([]DeletedRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, deletedRecord("journal", row.ID, journalEntryFromRow(row).summary(), row.DeletedAt))
	}
	return records, nil
}

func deletedTodos(ctx context.Context, q *store.Queries, limit int32) ([]DeletedRecord, error) {
	rows, err := q.ListDeletedTodos(ctx, limit)
	if err != nil {
		return nil, err
	}
	records := make([]DeletedRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, deletedRecord("todo", row.ID, todoFromRow(row).summary(), row.DeletedAt))
	}
	return records, nil
}

func deletedHabitLogs(ctx context.Context, q *store.Queries, limit int32) ([]DeletedRecord, error) {
	rows, err := q.ListDeletedHabitLogs(ctx, limit)
	if err != nil {
		return nil, err
	}
	records := make([]DeletedRecord, 0, len(rows))
	for _, row := range rows {
		log := habitLogFromRow(row.HabitLog, row.Habit)
		records = append(records, deletedRecord("habit_log", log.ID, log.summary(), row.HabitLog.DeletedAt))
	}
	return records, nil
}

func deletedSources(ctx context.Context, q *store.Queries, limit int32) ([]DeletedRecord, error) {
	rows, err := q.ListDeletedSources(ctx, limit)
	if err != nil {
		return nil, err
	}
	records := make([]DeletedRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, deletedRecord("source", row.ID, captureFromRow(row).summary(), row.DeletedAt))
	}
	return records, nil
}
