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

// summary is the one line that search and the list of deleted records show for a habit log.
func (l HabitLog) summary() string {
	return "habit: " + l.Habit + " on " + l.LoggedOn
}
