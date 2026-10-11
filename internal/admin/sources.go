package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// previewLength is the number of characters of a text that the list shows.
const previewLength = 200

// preview returns the start of text as one line.
func preview(text string) string {
	line := []rune(strings.Join(strings.Fields(text), " "))
	if len(line) <= previewLength {
		return string(line)
	}
	return strings.TrimSpace(string(line[:previewLength])) + "…"
}

// fileSize returns a number of bytes in words: "3 MB", "12 kB" or
// "9 bytes".
func fileSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%d MB", n>>20)
	case n >= 1<<10:
		return fmt.Sprintf("%d kB", n>>10)
	}
	return fmt.Sprintf("%d bytes", n)
}

func (h *handlers) sourcesPage(w http.ResponseWriter, r *http.Request) {
	captures, err := h.svc.ListCaptures(r.Context(), 0)
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, sourcesPage(captures))
}

// showSourcesList renders the list again, with message above it.
func (h *handlers) showSourcesList(w http.ResponseWriter, r *http.Request, message string) {
	captures, err := h.svc.ListCaptures(r.Context(), 0)
	if err != nil {
		listFailure(w, r, err, "The list did not load: internal error. Reload the page.")
		return
	}
	render(w, r, sourcesList(captures, message))
}

// addSource stores the text or the link from the form and renders the list
// again. A capture from the admin pages is one that Gautam made, so it is
// manual.
func (h *handlers) addSource(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	message := ""
	_, err := h.svc.Capture(r.Context(), service.CaptureParams{
		Text:   formText(r, "text"),
		URL:    r.PostForm.Get("url"),
		Title:  r.PostForm.Get("title"),
		Source: service.SourceManual,
	})
	var invalid *service.ValidationError
	switch {
	case err == nil:
		// htmx turns this header into an event; the form resets on it.
		w.Header().Set("HX-Trigger", "source-added")
	case errors.As(err, &invalid):
		message = invalid.Error()
	default:
		listFailure(w, r, err, "The source was not stored: internal error. Try again.")
		return
	}
	h.showSourcesList(w, r, message)
}

// deleteSource deletes one source and renders the list again.
func (h *handlers) deleteSource(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	message := ""
	err := h.svc.DeleteCapture(r.Context(), id, service.DeleteParams{Actor: service.SourceManual})
	switch {
	case err == nil:
	case errors.Is(err, service.ErrNotFound):
		message = fmt.Sprintf("Source %d is not in the list. It was possibly deleted before.", id)
	default:
		listFailure(w, r, err, "The source was not deleted: internal error. Try again.")
		return
	}
	h.showSourcesList(w, r, message)
}
