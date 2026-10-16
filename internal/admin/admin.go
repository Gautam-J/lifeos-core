// Package admin is the adapter for the server-rendered admin pages (templ
// and htmx). Handlers read the form, call one service function and render
// the result. They hold no logic.
package admin

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

//go:embed static
var static embed.FS

// Register adds every /admin route to router.
func Register(router chi.Router, svc *service.Service) {
	h := &handlers{svc: svc}
	// The address that Gautam types has no path.
	for _, path := range []string{"/", "/admin/"} {
		router.Get(path, home)
	}
	router.Get("/admin", h.todayPage)
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
	router.Get("/admin/sources/{id}", h.sourcePage)
	router.Post("/admin/sources/{id}/delete", h.deleteSource)
	router.Get("/admin/deleted", h.deletedPage)
	router.Post("/admin/deleted/purge", h.purgeRecord)
	router.Handle("/admin/static/*", staticFiles())
}

// assetVersions holds the start of the hash of each static file, by the
// path of the file below static/.
var assetVersions = func() map[string]string {
	versions := map[string]string{}
	err := fs.WalkDir(static, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := static.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(content)
		versions[strings.TrimPrefix(path, "static/")] = hex.EncodeToString(sum[:])[:12]
		return nil
	})
	if err != nil {
		panic("admin: read the static files: " + err.Error())
	}
	return versions
}()

// asset returns the address of a static file. The address changes with
// the content of the file, so that a browser can keep the file for good.
func asset(name string) string {
	return "/admin/static/" + name + "?v=" + assetVersions[name]
}

// staticFiles serves the folder static. Cloudflare and the browser keep a
// static file for hours when the server says nothing, and a deploy would
// show the old styles. So a file stays only when its address names its
// content. A font has no version: a new font gets a new file name.
func staticFiles() http.Handler {
	files := http.StripPrefix("/admin/", http.FileServerFS(static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/admin/static/")
		version, known := assetVersions[name]
		switch {
		case known && (r.URL.Query().Get("v") == version || strings.HasPrefix(name, "fonts/")):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		default:
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

type handlers struct {
	svc *service.Service
}

// home sends the browser to the summary page.
func home(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
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
// that Gautam typed, so it is manual.
func itemFromForm(r *http.Request) (service.AddItemParams, error) {
	params := service.AddItemParams{
		Kind:       r.PostForm.Get("kind"),
		Title:      r.PostForm.Get("title"),
		OccurredOn: r.PostForm.Get("occurred_on"),
		Provenance: service.Provenance{Source: service.SourceManual},
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

// listFailure logs err and shows message in place of the list, or in
// place of the summary when the click came from there. The status is 200,
// because htmx does not show the body of a 5xx response.
func listFailure(w http.ResponseWriter, r *http.Request, err error, message string) {
	slog.Error("admin: request failed", "error", err)
	if onToday(r) {
		render(w, r, todayFailure(message))
		return
	}
	render(w, r, itemsFailure(message))
}

func internalError(w http.ResponseWriter, err error) {
	slog.Error("admin: request failed", "error", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
