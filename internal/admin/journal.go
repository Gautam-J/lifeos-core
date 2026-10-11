package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// formText reads a text field that has more than one line. A browser sends
// each line break of a textarea as CR LF; the text that Gautam typed has
// LF. Without this, a click on Save with no change is a change.
func formText(r *http.Request, name string) string {
	return strings.ReplaceAll(r.PostForm.Get(name), "\r\n", "\n")
}

func (h *handlers) journalPage(w http.ResponseWriter, r *http.Request) {
	entries, err := h.svc.ListJournalEntries(r.Context(), 0)
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, journalPage(entries))
}

// showJournalList renders the list again, with message above it.
func (h *handlers) showJournalList(w http.ResponseWriter, r *http.Request, message string) {
	if onToday(r) {
		h.showToday(w, r, message)
		return
	}
	entries, err := h.svc.ListJournalEntries(r.Context(), 0)
	if err != nil {
		listFailure(w, r, err, "The list did not load: internal error. Reload the page.")
		return
	}
	render(w, r, journalList(entries, message))
}

// addJournalEntry stores the entry from the form and renders the list
// again. An entry from the admin pages is one that Gautam typed, so it is
// manual and verified.
func (h *handlers) addJournalEntry(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	message := ""
	_, err := h.svc.AddJournalEntry(r.Context(), service.AddJournalEntryParams{
		Body:       formText(r, "body"),
		EntryDate:  r.PostForm.Get("entry_date"),
		Provenance: service.Provenance{Source: service.SourceManual, Verified: true},
	})
	var invalid *service.ValidationError
	switch {
	case err == nil:
		// htmx turns this header into an event; the form resets on it.
		w.Header().Set("HX-Trigger", "entry-added")
	case errors.As(err, &invalid):
		message = invalid.Error()
	default:
		listFailure(w, r, err, "The entry was not stored: internal error. Try again.")
		return
	}
	h.showJournalList(w, r, message)
}

func (h *handlers) editJournalPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	entry, err := h.svc.GetJournalEntry(r.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, editJournalPage(journalForm{ID: entry.ID, Body: entry.Body, EntryDate: entry.EntryDate}, ""))
}

// saveJournalEntry stores the edit form and goes back to the list. When
// the service refuses a value, the form shows again with the reason.
func (h *handlers) saveJournalEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	form := journalForm{ID: id, Body: formText(r, "body"), EntryDate: r.PostForm.Get("entry_date")}
	_, err := h.svc.UpdateJournalEntry(r.Context(), id, service.UpdateJournalEntryParams{
		Body:      &form.Body,
		EntryDate: &form.EntryDate,
		Actor:     service.SourceManual,
	})
	var invalid *service.ValidationError
	switch {
	case err == nil:
		http.Redirect(w, r, "/admin/journal", http.StatusSeeOther)
	case errors.As(err, &invalid):
		render(w, r, editJournalPage(form, invalid.Error()))
	case errors.Is(err, service.ErrNotFound):
		http.NotFound(w, r)
	default:
		internalError(w, err)
	}
}

// deleteJournalEntry deletes one entry and renders the list again.
func (h *handlers) deleteJournalEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	message := ""
	err := h.svc.DeleteJournalEntry(r.Context(), id, service.DeleteParams{Actor: service.SourceManual})
	switch {
	case err == nil:
	case errors.Is(err, service.ErrNotFound):
		message = fmt.Sprintf("Entry %d is not in the list. It was possibly deleted before.", id)
	default:
		listFailure(w, r, err, "The entry was not deleted: internal error. Try again.")
		return
	}
	h.showJournalList(w, r, message)
}
