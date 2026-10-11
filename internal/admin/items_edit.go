package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// itemID reads the {id} part of the path. ok is false when it is not a
// number; the handler then answers 404.
func itemID(r *http.Request) (id int64, ok bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil
}

func (h *handlers) editItemPage(w http.ResponseWriter, r *http.Request) {
	id, ok := itemID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	item, err := h.svc.GetItem(r.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, editItemPage(itemForm{
		ID:         item.ID,
		Kind:       item.Kind,
		Title:      item.Title,
		Rating:     formatRating(item.Rating),
		OccurredOn: item.OccurredOn,
	}, ""))
}

// saveItem stores the edit form and goes back to the list. When the
// service refuses a value, the form shows again with the reason. An empty
// rating or date removes the stored value. The attrs stay as they are.
func (h *handlers) saveItem(w http.ResponseWriter, r *http.Request) {
	id, ok := itemID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	form := itemForm{
		ID:         id,
		Kind:       r.PostForm.Get("kind"),
		Title:      r.PostForm.Get("title"),
		Rating:     r.PostForm.Get("rating"),
		OccurredOn: r.PostForm.Get("occurred_on"),
	}
	params, err := form.params()
	if err == nil {
		_, err = h.svc.UpdateItem(r.Context(), id, params)
	}
	var invalid *service.ValidationError
	switch {
	case err == nil:
		http.Redirect(w, r, "/admin/items", http.StatusSeeOther)
	case errors.As(err, &invalid):
		render(w, r, editItemPage(form, invalid.Error()))
	case errors.Is(err, service.ErrNotFound):
		http.NotFound(w, r)
	default:
		internalError(w, err)
	}
}

// params turns the form into the input of UpdateItem. A change in the
// admin pages is one that Gautam typed.
func (f itemForm) params() (service.UpdateItemParams, error) {
	params := service.UpdateItemParams{Kind: &f.Kind, Title: &f.Title, Actor: service.SourceManual}
	if f.Rating == "" {
		params.Clear = append(params.Clear, "rating")
	} else {
		rating, err := strconv.ParseFloat(f.Rating, 64)
		if err != nil {
			return params, &service.ValidationError{Field: "rating", Message: "must be a number"}
		}
		params.Rating = &rating
	}
	if f.OccurredOn == "" {
		params.Clear = append(params.Clear, "occurred_on")
	} else {
		params.OccurredOn = &f.OccurredOn
	}
	return params, nil
}

// deleteItem deletes one item and renders the list again.
func (h *handlers) deleteItem(w http.ResponseWriter, r *http.Request) {
	id, ok := itemID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	message := ""
	err := h.svc.DeleteItem(r.Context(), id, service.DeleteParams{Actor: service.SourceManual})
	switch {
	case err == nil:
	case errors.Is(err, service.ErrNotFound):
		message = fmt.Sprintf("Item %d is not in the list. It was possibly deleted before.", id)
	default:
		listFailure(w, r, err, "The item was not deleted: internal error. Try again.")
		return
	}

	items, err := h.svc.ListItems(r.Context(), service.ListItemsParams{})
	if err != nil {
		listFailure(w, r, err, "The list did not load: internal error. Reload the page.")
		return
	}
	render(w, r, itemsList(items, message))
}
