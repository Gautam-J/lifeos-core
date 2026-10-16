package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// TodoStatuses are the states of a todo. A new todo is open.
var TodoStatuses = []string{"open", "done", "dropped"}

const maxProjectLength = 100

// Todo is one row of todos.
type Todo struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	// Status is open, done or dropped.
	Status string `json:"status"`
	// DueOn is the day that the todo is due, as YYYY-MM-DD.
	DueOn   string `json:"due_on,omitempty"`
	Project string `json:"project,omitempty"`
	// DoneAt is the moment that the status became done.
	DoneAt    *time.Time `json:"done_at,omitempty"`
	Source    Source     `json:"source"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// AddTodoParams is the input of AddTodo.
type AddTodoParams struct {
	Title string
	// DueOn is optional: YYYY-MM-DD. A day in the past is valid.
	DueOn string
	// Project is optional: free text, such as lifeos.
	Project string
	Provenance
}

// todoValues are the fields of a todo that a caller can set, after the
// checks.
type todoValues struct {
	title   string
	status  string
	dueOn   *time.Time
	project *string
}

// checkTodo checks the fields that a caller can set. An empty dueOn or
// project means that the todo has none.
func checkTodo(title, status, dueOn, project string) (todoValues, error) {
	v := todoValues{title: strings.TrimSpace(title), status: status}

	if v.title == "" {
		return v, invalid("title", "must not be empty")
	}
	if utf8.RuneCountInString(v.title) > maxTitleLength {
		return v, invalid("title", fmt.Sprintf("must be at most %d characters", maxTitleLength))
	}
	if err := checkText("title", v.title); err != nil {
		return v, err
	}
	if !slices.Contains(TodoStatuses, status) {
		return v, invalid("status", "must be one of "+strings.Join(TodoStatuses, ", "))
	}
	if dueOn != "" {
		day, err := parseDay("due_on", dueOn)
		if err != nil {
			return v, err
		}
		v.dueOn = &day
	}
	if project = strings.TrimSpace(project); project != "" {
		if utf8.RuneCountInString(project) > maxProjectLength {
			return v, invalid("project", fmt.Sprintf("must be at most %d characters", maxProjectLength))
		}
		if err := checkText("project", project); err != nil {
			return v, err
		}
		v.project = &project
	}
	return v, nil
}

// AddTodo validates p, stores one open todo and appends a create event, in
// one transaction.
func (s *Service) AddTodo(ctx context.Context, p AddTodoParams) (Todo, error) {
	v, err := checkTodo(p.Title, "open", p.DueOn, p.Project)
	if err != nil {
		return Todo{}, err
	}
	if err := p.Provenance.validate(); err != nil {
		return Todo{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Todo{}, fmt.Errorf("add todo: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	stored, err := q.InsertTodo(ctx, store.InsertTodoParams{
		Title:   v.title,
		DueOn:   v.dueOn,
		Project: v.project,
		Source:  string(p.Source),
	})
	if err != nil {
		return Todo{}, fmt.Errorf("add todo: %w", err)
	}
	todo := todoFromRow(stored)
	if err := recordEvent(ctx, q, "create", "todos", todo.ID, p.Source, nil, todo); err != nil {
		return Todo{}, fmt.Errorf("add todo: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Todo{}, fmt.Errorf("add todo: %w", err)
	}
	return todo, nil
}

func todoFromRow(row store.Todo) Todo {
	todo := Todo{
		ID:        row.ID,
		Title:     row.Title,
		Status:    row.Status,
		Source:    Source(row.Source),
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
	if row.DueOn != nil {
		todo.DueOn = row.DueOn.Format(time.DateOnly)
	}
	if row.Project != nil {
		todo.Project = *row.Project
	}
	if row.DoneAt != nil {
		doneAt := row.DoneAt.UTC()
		todo.DoneAt = &doneAt
	}
	return todo
}

// GetTodo returns the todo with this ID. It returns ErrNotFound when there
// is no such todo or the todo is deleted.
func (s *Service) GetTodo(ctx context.Context, id int64) (Todo, error) {
	row, err := store.New(s.pool).GetTodo(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Todo{}, ErrNotFound
	}
	if err != nil {
		return Todo{}, fmt.Errorf("get todo: %w", err)
	}
	return todoFromRow(row), nil
}

// ListTodosParams is the input of ListTodos.
type ListTodosParams struct {
	// Status is optional: an empty string lists every status.
	Status string
	// Limit is optional: 0 means 50. The maximum is 200.
	Limit int
}

// ListTodos returns the todos that are not deleted. The open todos come
// first; in each group the newest comes first.
func (s *Service) ListTodos(ctx context.Context, p ListTodosParams) ([]Todo, error) {
	maxRows, err := checkLimit(p.Limit)
	if err != nil {
		return nil, err
	}
	arg := store.ListTodosParams{MaxRows: maxRows}
	if p.Status != "" {
		if !slices.Contains(TodoStatuses, p.Status) {
			return nil, invalid("status", "must be one of "+strings.Join(TodoStatuses, ", "))
		}
		arg.Status = &p.Status
	}

	rows, err := store.New(s.pool).ListTodos(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("list todos: %w", err)
	}
	todos := make([]Todo, 0, len(rows))
	for _, row := range rows {
		todos = append(todos, todoFromRow(row))
	}
	return todos, nil
}
