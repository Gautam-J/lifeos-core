package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

const maxHabitNameLength = 50

// Habit is one row of habits: the definition of a habit.
type Habit struct {
	ID int64 `json:"id"`
	// Name is in lower case. No two habits have the same name.
	Name string `json:"name"`
	// Active is false for a habit that Gautam does not follow now. Its
	// logs stay.
	Active    bool      `json:"active"`
	Source    Source    `json:"source"`
	SourceRef *int64    `json:"source_ref,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AddHabitParams is the input of AddHabit.
type AddHabitParams struct {
	Name string
	Provenance
}

// habitName returns name as the table holds it: in lower case, with one
// space between words. "Gym", "gym " and "GYM" are one habit.
func habitName(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), " ")
}

func checkHabitName(field, name string) error {
	if name == "" {
		return invalid(field, "must not be empty")
	}
	if utf8.RuneCountInString(name) > maxHabitNameLength {
		return invalid(field, fmt.Sprintf("must be at most %d characters", maxHabitNameLength))
	}
	return checkText(field, name)
}

// habitNameTaken reports whether err comes from the unique index on the
// habit names. The index is what stops two requests at the same moment.
func habitNameTaken(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

const habitNameTakenMessage = "a habit has this name already; if that habit is not active, make it active again"

// AddHabit validates p, stores one active habit and appends a create
// event, in one transaction.
func (s *Service) AddHabit(ctx context.Context, p AddHabitParams) (Habit, error) {
	name := habitName(p.Name)
	if err := checkHabitName("name", name); err != nil {
		return Habit{}, err
	}
	if err := p.Provenance.validate(); err != nil {
		return Habit{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Habit{}, fmt.Errorf("add habit: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)
	if err := p.checkSourceRef(ctx, q); err != nil {
		return Habit{}, err
	}

	stored, err := q.InsertHabit(ctx, store.InsertHabitParams{
		Name:      name,
		Source:    string(p.Source),
		SourceRef: p.SourceRef,
	})
	if habitNameTaken(err) {
		return Habit{}, invalid("name", habitNameTakenMessage)
	}
	if err != nil {
		return Habit{}, fmt.Errorf("add habit: %w", err)
	}
	habit := habitFromRow(stored)
	if err := recordEvent(ctx, q, "create", "habits", habit.ID, p.Source, nil, habit); err != nil {
		return Habit{}, fmt.Errorf("add habit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Habit{}, fmt.Errorf("add habit: %w", err)
	}
	return habit, nil
}

func habitFromRow(row store.Habit) Habit {
	return Habit{
		ID:        row.ID,
		Name:      row.Name,
		Active:    row.Active,
		Source:    Source(row.Source),
		SourceRef: row.SourceRef,
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
}

// ListHabits returns every habit, active or not, by name.
func (s *Service) ListHabits(ctx context.Context) ([]Habit, error) {
	rows, err := store.New(s.pool).ListHabits(ctx)
	if err != nil {
		return nil, fmt.Errorf("list habits: %w", err)
	}
	habits := make([]Habit, 0, len(rows))
	for _, row := range rows {
		habits = append(habits, habitFromRow(row))
	}
	return habits, nil
}

// UpdateHabitParams is the input of UpdateHabit. A field that is nil keeps
// its stored value.
type UpdateHabitParams struct {
	Name   *string
	Active *bool
	// Actor is who makes the change. It goes into the event.
	Actor Source
}

// UpdateHabit renames one habit, or makes it active or not active, and
// appends an update event that holds the old row, in one transaction. A
// call that changes no value writes nothing and appends no event.
func (s *Service) UpdateHabit(ctx context.Context, id int64, p UpdateHabitParams) (Habit, error) {
	if err := (Provenance{Source: p.Actor}).validate(); err != nil {
		return Habit{}, err
	}
	if p.Name == nil && p.Active == nil {
		return Habit{}, invalid("fields", "must name at least one field to change")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Habit{}, fmt.Errorf("update habit: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	current, err := q.GetHabitForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Habit{}, ErrNotFound
	}
	if err != nil {
		return Habit{}, fmt.Errorf("update habit: %w", err)
	}
	old := habitFromRow(current)

	name, active := old.Name, old.Active
	if p.Name != nil {
		name = habitName(*p.Name)
		if err := checkHabitName("name", name); err != nil {
			return Habit{}, err
		}
	}
	if p.Active != nil {
		active = *p.Active
	}
	if name == old.Name && active == old.Active {
		return old, nil
	}
	stored, err := q.UpdateHabit(ctx, store.UpdateHabitParams{ID: id, Name: name, Active: active})
	if habitNameTaken(err) {
		return Habit{}, invalid("name", habitNameTakenMessage)
	}
	if err != nil {
		return Habit{}, fmt.Errorf("update habit: %w", err)
	}
	habit := habitFromRow(stored)
	if err := recordEvent(ctx, q, "update", "habits", id, p.Actor, old, habit); err != nil {
		return Habit{}, fmt.Errorf("update habit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Habit{}, fmt.Errorf("update habit: %w", err)
	}
	return habit, nil
}
