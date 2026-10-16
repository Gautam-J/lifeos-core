package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// HabitLog is one row of habit_logs: Gautam did the habit on that day.
type HabitLog struct {
	ID      int64 `json:"id"`
	HabitID int64 `json:"habit_id"`
	// Habit is the name of the habit.
	Habit string `json:"habit"`
	// LoggedOn is the IST day, as YYYY-MM-DD.
	LoggedOn  string    `json:"logged_on"`
	Source    Source    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LogHabitParams is the input of LogHabit.
type LogHabitParams struct {
	// Habit is the name of an active habit. Case and extra spaces do not
	// matter.
	Habit string
	// LoggedOn is optional: YYYY-MM-DD, not later than today in IST. An
	// empty value means today in IST.
	LoggedOn string
	Provenance
}

// LogHabit records that Gautam did a habit on a day, and appends a create
// event, in one transaction. A habit has one log for each day: when the
// log exists already, LogHabit returns it and writes nothing.
func (s *Service) LogHabit(ctx context.Context, p LogHabitParams) (HabitLog, error) {
	name := habitName(p.Habit)
	if name == "" {
		return HabitLog{}, invalid("habit", "must not be empty")
	}
	day, err := pastDay("logged_on", p.LoggedOn, s.now())
	if err != nil {
		return HabitLog{}, err
	}
	if err := p.Provenance.validate(); err != nil {
		return HabitLog{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return HabitLog{}, fmt.Errorf("log habit: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	habit, err := q.GetActiveHabitByName(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return HabitLog{}, unknownHabit(ctx, q, name)
	}
	if err != nil {
		return HabitLog{}, fmt.Errorf("log habit: %w", err)
	}

	row, err := q.InsertHabitLog(ctx, store.InsertHabitLogParams{
		HabitID:  habit.ID,
		LoggedOn: day,
		Source:   string(p.Source),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// The habit has a log for this day already.
		existing, err := q.GetHabitLogOfDayForUpdate(ctx, store.GetHabitLogOfDayForUpdateParams{HabitID: habit.ID, LoggedOn: day})
		if err != nil {
			return HabitLog{}, fmt.Errorf("log habit: %w", err)
		}
		return habitLogFromRow(existing, habit.Name), nil
	}
	if err != nil {
		return HabitLog{}, fmt.Errorf("log habit: %w", err)
	}

	log := habitLogFromRow(row, habit.Name)
	if err := recordEvent(ctx, q, "create", "habit_logs", log.ID, p.Source, nil, log); err != nil {
		return HabitLog{}, fmt.Errorf("log habit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return HabitLog{}, fmt.Errorf("log habit: %w", err)
	}
	return log, nil
}

// unknownHabit returns the error for a name that no active habit has. The
// message lists the active habits, so that an agent can ask Gautam which
// one he means.
func unknownHabit(ctx context.Context, q *store.Queries, name string) error {
	habits, err := q.ListHabits(ctx)
	if err != nil {
		return fmt.Errorf("log habit: %w", err)
	}
	var active []string
	for _, habit := range habits {
		if habit.Active {
			active = append(active, habit.Name)
		}
	}
	if len(active) == 0 {
		return invalid("habit", "there is no active habit: add the habit in the admin pages first")
	}
	return invalid("habit", fmt.Sprintf("no active habit has the name %q; the active habits are: %s", name, strings.Join(active, ", ")))
}

func habitLogFromRow(row store.HabitLog, habit string) HabitLog {
	return HabitLog{
		ID:        row.ID,
		HabitID:   row.HabitID,
		Habit:     habit,
		LoggedOn:  row.LoggedOn.Format(time.DateOnly),
		Source:    Source(row.Source),
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
}

// GetHabitLog returns the habit log with this ID. It returns ErrNotFound
// when there is no such log or the log is deleted.
func (s *Service) GetHabitLog(ctx context.Context, id int64) (HabitLog, error) {
	row, err := store.New(s.pool).GetHabitLog(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return HabitLog{}, ErrNotFound
	}
	if err != nil {
		return HabitLog{}, fmt.Errorf("get habit log: %w", err)
	}
	return habitLogFromRow(row.HabitLog, row.Habit), nil
}

// ListHabitLogs returns the habit logs that are not deleted, the newest
// day first. limit 0 means 50; the maximum is 200.
func (s *Service) ListHabitLogs(ctx context.Context, limit int) ([]HabitLog, error) {
	maxRows, err := checkLimit(limit)
	if err != nil {
		return nil, err
	}
	rows, err := store.New(s.pool).ListHabitLogs(ctx, maxRows)
	if err != nil {
		return nil, fmt.Errorf("list habit logs: %w", err)
	}
	logs := make([]HabitLog, 0, len(rows))
	for _, row := range rows {
		logs = append(logs, habitLogFromRow(row.HabitLog, row.Habit))
	}
	return logs, nil
}

// HabitStatus says whether one active habit is done on a day.
type HabitStatus struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// Done is true when the habit has a live log on the day.
	Done bool `json:"done"`
}

// HabitChecklist is the state of every active habit on one day.
type HabitChecklist struct {
	// Date is the IST day, as YYYY-MM-DD.
	Date   string        `json:"date"`
	Habits []HabitStatus `json:"habits"`
}

// HabitsToday returns the active habits, by name, with their state for
// today in IST.
func (s *Service) HabitsToday(ctx context.Context) (HabitChecklist, error) {
	day := today(s.now())
	rows, err := store.New(s.pool).ListHabitsOnDay(ctx, day)
	if err != nil {
		return HabitChecklist{}, fmt.Errorf("habits today: %w", err)
	}
	list := HabitChecklist{Date: day.Format(time.DateOnly), Habits: make([]HabitStatus, 0, len(rows))}
	for _, row := range rows {
		list.Habits = append(list.Habits, HabitStatus{ID: row.ID, Name: row.Name, Done: row.Done})
	}
	return list, nil
}
