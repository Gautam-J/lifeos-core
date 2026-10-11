// Package admin is the adapter for the server-rendered admin pages (templ
// and htmx). Handlers read the form, call one service function and render
// the result. They hold no logic.
package admin

import (
	"context"
	"embed"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

//go:embed static
var static embed.FS

// Register adds every /admin route to router.
func Register(router chi.Router, svc *service.Service) {
	h := &handlers{svc: svc}
	router.Get("/admin/items", h.itemsPage)
	router.Post("/admin/items", h.addItem)
	router.Get("/admin/items/{id}/edit", h.editItemPage)
	router.Post("/admin/items/{id}", h.saveItem)
	router.Post("/admin/items/{id}/delete", h.deleteItem)
	router.Get("/admin/journal", h.journalPage)
	router.Post("/admin/journal", h.addJournalEntry)
	router.Get("/admin/journal/{id}/edit", h.editJournalPage)
	router.Post("/admin/journal/{id}", h.saveJournalEntry)
	router.Post("/admin/journal/{id}/delete", h.deleteJournalEntry)
	router.Get("/admin/todos", h.todosPage)
	router.Post("/admin/todos", h.addTodo)
	router.Post("/admin/todos/{id}/done", h.markTodoDone)
	router.Get("/admin/todos/{id}/edit", h.editTodoPage)
	router.Post("/admin/todos/{id}", h.saveTodo)
	router.Post("/admin/todos/{id}/delete", h.deleteTodo)
	router.Get("/admin/habits", h.habitsPage)
	router.Post("/admin/habits", h.addHabit)
	router.Post("/admin/habits/log", h.logHabitToday)
	router.Post("/admin/habits/{id}", h.saveHabit)
	router.Post("/admin/habits/logs/{id}/delete", h.deleteHabitLog)
	router.Get("/admin/sources", h.sourcesPage)
	router.Post("/admin/sources", h.addSource)
	router.Post("/admin/sources/{id}/delete", h.deleteSource)
	router.Get("/admin/review", h.reviewPage)
	router.Post("/admin/review/verify", h.verifyRecord)
	router.Handle("/admin/static/*", http.StripPrefix("/admin/", http.FileServerFS(static)))
}

type handlers struct {
	svc *service.Service
}

func (h *handlers) itemsPage(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListItems(r.Context(), service.ListItemsParams{})
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, itemsPage(items))
}

// addItem stores the item from the form and renders the list again. When
// the service refuses the input, the list shows the reason and the form
// keeps what Gautam typed.
func (h *handlers) addItem(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	message := ""
	params, err := itemFromForm(r)
	if err == nil {
		_, err = h.svc.AddItem(r.Context(), params)
	}
	var invalid *service.ValidationError
	switch {
	case err == nil:
		// htmx turns this header into an event; the form resets on it.
		w.Header().Set("HX-Trigger", "item-added")
	case errors.As(err, &invalid):
		message = invalid.Error()
	default:
		listFailure(w, r, err, "The item was not stored: internal error. Try again.")
		return
	}

	items, err := h.svc.ListItems(r.Context(), service.ListItemsParams{})
	if err != nil {
		listFailure(w, r, err, "The list did not load: internal error. Reload the page.")
		return
	}
	render(w, r, itemsList(items, message))
}

// itemFromForm reads the form fields. A row from the admin pages is one
// that Gautam typed, so it is manual and verified.
func itemFromForm(r *http.Request) (service.AddItemParams, error) {
	params := service.AddItemParams{
		Kind:       r.PostForm.Get("kind"),
		Title:      r.PostForm.Get("title"),
		OccurredOn: r.PostForm.Get("occurred_on"),
		Provenance: service.Provenance{Source: service.SourceManual, Verified: true},
	}
	if raw := r.PostForm.Get("rating"); raw != "" {
		rating, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return params, &service.ValidationError{Field: "rating", Message: "must be a number"}
		}
		params.Rating = &rating
	}
	return params, nil
}

type component interface {
	Render(ctx context.Context, w io.Writer) error
}

func render(w http.ResponseWriter, r *http.Request, c component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Render(r.Context(), w); err != nil {
		slog.Error("admin: render failed", "error", err)
	}
}

// listFailure logs err and shows message in place of the list. The status
// is 200, because htmx does not show the body of a 5xx response.
func listFailure(w http.ResponseWriter, r *http.Request, err error, message string) {
	slog.Error("admin: request failed", "error", err)
	render(w, r, itemsFailure(message))
}

func internalError(w http.ResponseWriter, err error) {
	slog.Error("admin: request failed", "error", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
