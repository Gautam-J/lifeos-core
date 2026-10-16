package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

func (h *handlers) todosPage(w http.ResponseWriter, r *http.Request) {
	todos, err := h.svc.ListTodos(r.Context(), service.ListTodosParams{})
	if err != nil {
		internalError(w, err)
		return
	}
	projects, err := h.projectNames(r)
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, todosPage(todos, projects))
}

// showTodosList renders the list again, with message above it.
func (h *handlers) showTodosList(w http.ResponseWriter, r *http.Request, message string) {
	if onToday(r) {
		h.showToday(w, r, message)
		return
	}
	todos, err := h.svc.ListTodos(r.Context(), service.ListTodosParams{})
	if err != nil {
		listFailure(w, r, err, "The list did not load: internal error. Reload the page.")
		return
	}
	render(w, r, todosList(todos, message))
}

// addTodo stores the todo from the form and renders the list again. A todo
// from the admin pages is one that Gautam typed, so it is manual.
func (h *handlers) addTodo(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	message := ""
	_, err := h.svc.AddTodo(r.Context(), service.AddTodoParams{
		Title:      r.PostForm.Get("title"),
		Project:    r.PostForm.Get("project"),
		DueOn:      r.PostForm.Get("due_on"),
		Provenance: service.Provenance{Source: service.SourceManual},
	})
	var invalid *service.ValidationError
	switch {
	case err == nil:
		// htmx turns this header into an event; the form resets on it.
		w.Header().Set("HX-Trigger", "todo-added")
	case errors.As(err, &invalid):
		message = invalid.Error()
	default:
		listFailure(w, r, err, "The todo was not stored: internal error. Try again.")
		return
	}
	h.showTodosList(w, r, message)
}

// markTodoDone sets the status of one todo to done and renders the list
// again.
func (h *handlers) markTodoDone(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	done := "done"
	message := ""
	_, err := h.svc.UpdateTodo(r.Context(), id, service.UpdateTodoParams{Status: &done, Actor: service.SourceManual})
	switch {
	case err == nil:
	case errors.Is(err, service.ErrNotFound):
		message = fmt.Sprintf("Todo %d is not in the list. It was possibly deleted before.", id)
	default:
		listFailure(w, r, err, "The todo was not changed: internal error. Try again.")
		return
	}
	h.showTodosList(w, r, message)
}

func (h *handlers) editTodoPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	todo, err := h.svc.GetTodo(r.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	projects, err := h.projectNames(r)
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, editTodoPage(todoForm{
		ID:      todo.ID,
		Title:   todo.Title,
		Status:  todo.Status,
		Project: todo.Project,
		DueOn:   todo.DueOn,
	}, projects, ""))
}

// saveTodo stores the edit form and goes back to the list. When the
// service refuses a value, the form shows again with the reason. An empty
// project or date removes the stored value.
func (h *handlers) saveTodo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	form := todoForm{
		ID:      id,
		Title:   r.PostForm.Get("title"),
		Status:  r.PostForm.Get("status"),
		Project: r.PostForm.Get("project"),
		DueOn:   r.PostForm.Get("due_on"),
	}
	_, err := h.svc.UpdateTodo(r.Context(), id, form.params())
	var invalid *service.ValidationError
	switch {
	case err == nil:
		http.Redirect(w, r, "/admin/todos", http.StatusSeeOther)
	case errors.As(err, &invalid):
		projects, listErr := h.projectNames(r)
		if listErr != nil {
			internalError(w, listErr)
			return
		}
		render(w, r, editTodoPage(form, projects, invalid.Error()))
	case errors.Is(err, service.ErrNotFound):
		http.NotFound(w, r)
	default:
		internalError(w, err)
	}
}

// params turns the form into the input of UpdateTodo. A change in the
// admin pages is one that Gautam typed.
func (f todoForm) params() service.UpdateTodoParams {
	params := service.UpdateTodoParams{Title: &f.Title, Status: &f.Status, Actor: service.SourceManual}
	if strings.TrimSpace(f.Project) == "" {
		params.Clear = append(params.Clear, "project")
	} else {
		params.Project = &f.Project
	}
	if f.DueOn == "" {
		params.Clear = append(params.Clear, "due_on")
	} else {
		params.DueOn = &f.DueOn
	}
	return params
}

// deleteTodo deletes one todo and renders the list again.
func (h *handlers) deleteTodo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	message := ""
	err := h.svc.DeleteTodo(r.Context(), id, service.DeleteParams{Actor: service.SourceManual})
	switch {
	case err == nil:
	case errors.Is(err, service.ErrNotFound):
		message = fmt.Sprintf("Todo %d is not in the list. It was possibly deleted before.", id)
	default:
		listFailure(w, r, err, "The todo was not deleted: internal error. Try again.")
		return
	}
	h.showTodosList(w, r, message)
}
