package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// UpdateTodoParams is the input of UpdateTodo. A field that is nil keeps
// its stored value.
type UpdateTodoParams struct {
	Title *string `json:"title"`
	// Status is open, done or dropped. The service sets done_at from it.
	Status  *string `json:"status"`
	DueOn   *string `json:"due_on"`
	Project *string `json:"project"`
	// JournalRef is the ID of the journal entry that reports the change,
	// such as the entry that says that the todo is done. nil keeps the
	// stored value.
	JournalRef *int64 `json:"journal_ref"`
	// Clear names the optional fields to empty: due_on, project.
	Clear []string `json:"-"`
	// Actor is who makes the change. It goes into the event; the source
	// of the row stays as it is.
	Actor Source `json:"-"`
}

// clearableTodoFields are the optional fields that Clear can name.
var clearableTodoFields = []string{"due_on", "project"}

// UpdateTodo changes the given fields of one todo and appends an update
// event that holds the old row, in one transaction. It locks the row
// first, so two updates at the same moment do not lose a field. A call
// that changes no value writes nothing and appends no event.
func (s *Service) UpdateTodo(ctx context.Context, id int64, p UpdateTodoParams) (Todo, error) {
	if err := p.check(); err != nil {
		return Todo{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Todo{}, fmt.Errorf("update todo: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	current, err := q.GetTodoForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Todo{}, ErrNotFound
	}
	if err != nil {
		return Todo{}, fmt.Errorf("update todo: %w", err)
	}
	old, err := todoFromStore(ctx, q, current)
	if err != nil {
		return Todo{}, err
	}
	if p.JournalRef != nil {
		if _, err := checkJournalRef(ctx, q, *p.JournalRef); err != nil {
			return Todo{}, err
		}
	}

	// The changed todo goes through the same checks as a new todo.
	next := p.applyTo(old)
	v, err := checkTodo(next.Title, next.Status, next.DueOn, next.Project)
	if err != nil {
		return Todo{}, err
	}
	next.Title = v.title
	// A name that differs only in case from the project that is stored
	// is the same project: it is no change.
	if v.project != nil {
		if e, found, err := findEntity(ctx, q, "project", *v.project); err != nil {
			return Todo{}, err
		} else if found {
			next.Project = e.Name
		}
	}
	if next == old {
		return old, nil
	}
	// done_at follows the status. It keeps its value while the todo
	// stays done.
	doneAt := current.DoneAt
	switch {
	case v.status != "done":
		doneAt = nil
	case old.Status != "done":
		now := s.now().UTC()
		doneAt = &now
	}
	project, err := projectOf(ctx, q, v.project, p.Actor)
	if err != nil {
		return Todo{}, err
	}
	stored, err := q.UpdateTodo(ctx, store.UpdateTodoParams{
		ID:         id,
		Title:      v.title,
		Status:     v.status,
		DueOn:      v.dueOn,
		ProjectID:  project.id,
		DoneAt:     doneAt,
		JournalRef: journalRefParam(next.JournalRef),
	})
	if err != nil {
		return Todo{}, fmt.Errorf("update todo: %w", err)
	}
	todo := todoFromRow(stored)
	todo.Project = project.name
	if err := recordEvent(ctx, q, "update", "todos", id, p.Actor, old, todo); err != nil {
		return Todo{}, fmt.Errorf("update todo: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Todo{}, fmt.Errorf("update todo: %w", err)
	}
	return todo, nil
}

// check refuses input that is wrong for every stored row.
func (p UpdateTodoParams) check() error {
	if err := (Provenance{Source: p.Actor}).validate(); err != nil {
		return err
	}
	if p.Title == nil && p.Status == nil && p.DueOn == nil && p.Project == nil && p.JournalRef == nil && len(p.Clear) == 0 {
		return invalid("fields", "must name at least one field to change, or a field to empty in clear")
	}
	if p.JournalRef != nil && *p.JournalRef <= 0 {
		return invalid("journal_ref", "must be the ID of a journal entry")
	}
	for _, name := range p.Clear {
		if !slices.Contains(clearableTodoFields, name) {
			return invalid("clear", "can hold only due_on, project")
		}
	}
	if p.DueOn != nil {
		if slices.Contains(p.Clear, "due_on") {
			return invalid("due_on", "has a value and is also in clear")
		}
		if *p.DueOn == "" {
			return invalid("due_on", "must be a date as YYYY-MM-DD; to remove the date, put due_on in clear")
		}
	}
	if p.Project != nil {
		if slices.Contains(p.Clear, "project") {
			return invalid("project", "has a value and is also in clear")
		}
		if strings.TrimSpace(*p.Project) == "" {
			return invalid("project", "must not be empty; to remove the project, put project in clear")
		}
	}
	return nil
}

// applyTo returns old with the changes of p. The values are not checked
// yet.
func (p UpdateTodoParams) applyTo(old Todo) Todo {
	next := old
	if p.Title != nil {
		next.Title = *p.Title
	}
	if p.Status != nil {
		next.Status = *p.Status
	}
	if p.DueOn != nil {
		next.DueOn = *p.DueOn
	}
	if p.Project != nil {
		next.Project = strings.TrimSpace(*p.Project)
	}
	if p.JournalRef != nil {
		next.JournalRef = *p.JournalRef
	}
	if slices.Contains(p.Clear, "due_on") {
		next.DueOn = ""
	}
	if slices.Contains(p.Clear, "project") {
		next.Project = ""
	}
	return next
}

// DeleteTodo marks one todo as deleted and appends a delete event that
// holds the row, in one transaction.
func (s *Service) DeleteTodo(ctx context.Context, id int64, p DeleteParams) error {
	return s.deleteRecord(ctx, "todos", id, p, func(q *store.Queries) (any, error) {
		row, err := q.SoftDeleteTodo(ctx, id)
		if err != nil {
			return nil, err
		}
		return todoFromStore(ctx, q, row)
	})
}

// summary is the one line that search and the list of deleted records show for a todo.
func (t Todo) summary() string {
	s := "todo: " + t.Title
	if t.Status != "open" {
		s += ", " + t.Status
	}
	if t.Project != "" {
		s += ", project " + t.Project
	}
	if t.DueOn != "" {
		s += ", due " + t.DueOn
	}
	return s
}
