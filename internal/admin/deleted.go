package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// deletedLimit is the number of deleted records that the page shows.
const deletedLimit = 200

func (h *handlers) deletedPage(w http.ResponseWriter, r *http.Request) {
	records, err := h.svc.ListDeleted(r.Context(), deletedLimit)
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, deletedPage(records))
}

// purgeRecord removes the one record of the form for good and renders the
// list again. Only this handler calls Purge: permanent removal happens
// only in the admin pages.
func (h *handlers) purgeRecord(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	id, err := strconv.ParseInt(r.PostForm.Get("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	recordType := r.PostForm.Get("type")
	message := ""
	err = h.svc.Purge(r.Context(), recordType, id)
	var invalid *service.ValidationError
	switch {
	case err == nil:
	case errors.Is(err, service.ErrNotFound):
		message = fmt.Sprintf("%s %d is not in the list. It was possibly removed before.", recordType, id)
	case errors.As(err, &invalid):
		message = invalid.Error()
	default:
		listFailure(w, r, err, "The record was not removed: internal error. Try again.")
		return
	}

	records, err := h.svc.ListDeleted(r.Context(), deletedLimit)
	if err != nil {
		listFailure(w, r, err, "The list did not load: internal error. Reload the page.")
		return
	}
	render(w, r, deletedList(records, message))
}
