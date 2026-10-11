package admin

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

func (h *handlers) reviewPage(w http.ResponseWriter, r *http.Request) {
	entries, err := h.svc.ReviewQueue(r.Context(), 0)
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, reviewPage(entries))
}

// verifyRecord approves the one record of the form and renders the queue
// again. An approval in the admin pages is Gautam's own.
func (h *handlers) verifyRecord(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	id, err := strconv.ParseInt(r.PostForm.Get("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	message := ""
	_, err = h.svc.Verify(r.Context(), []service.RecordRef{{Type: r.PostForm.Get("type"), ID: id}}, service.SourceManual)
	var invalid *service.ValidationError
	switch {
	case err == nil:
	case errors.As(err, &invalid):
		message = invalid.Error()
	default:
		listFailure(w, r, err, "The record was not approved: internal error. Try again.")
		return
	}
	if onToday(r) {
		h.showToday(w, r, message)
		return
	}

	entries, err := h.svc.ReviewQueue(r.Context(), 0)
	if err != nil {
		listFailure(w, r, err, "The list did not load: internal error. Reload the page.")
		return
	}
	render(w, r, reviewList(entries, message))
}
