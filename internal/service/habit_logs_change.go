package service

import (
	"context"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// DeleteHabitLog marks one habit log as deleted and appends a delete event
// that holds the row, in one transaction. After that, the habit can get a
// new log for the same day.
func (s *Service) DeleteHabitLog(ctx context.Context, id int64, p DeleteParams) error {
	return s.deleteRecord(ctx, "habit_logs", id, p, func(q *store.Queries) (any, error) {
		// The first query locks the row and gives the name of the habit.
		current, err := q.GetHabitLogForUpdate(ctx, id)
		if err != nil {
			return nil, err
		}
		row, err := q.SoftDeleteHabitLog(ctx, id)
		if err != nil {
			return nil, err
		}
		return habitLogFromRow(row, current.Habit), nil
	})
}

// summary is the one line that the review queue shows for a habit log.
func (l HabitLog) summary() string {
	return "habit: " + l.Habit + " on " + l.LoggedOn
}

// unverifiedHabitLogs returns the habit logs that wait for review, oldest
// first.
func unverifiedHabitLogs(ctx context.Context, q *store.Queries, limit int32) ([]ReviewEntry, error) {
	rows, err := q.ListUnverifiedHabitLogs(ctx, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]ReviewEntry, 0, len(rows))
	for _, row := range rows {
		log := habitLogFromRow(row.HabitLog, row.Habit)
		entries = append(entries, ReviewEntry{
			Type:      "habit_log",
			ID:        log.ID,
			Summary:   log.summary(),
			Source:    log.Source,
			CreatedAt: log.CreatedAt,
			UpdatedAt: log.UpdatedAt,
		})
	}
	return entries, nil
}

func verifyHabitLog(ctx context.Context, q *store.Queries, id int64) (before, after any, err error) {
	current, err := q.GetHabitLogForUpdate(ctx, id)
	if err != nil || current.HabitLog.Verified {
		return nil, nil, err
	}
	stored, err := q.VerifyHabitLog(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return habitLogFromRow(current.HabitLog, current.Habit), habitLogFromRow(stored, current.Habit), nil
}
