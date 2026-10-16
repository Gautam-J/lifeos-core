package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// entitiesView is what the entities page shows.
type entitiesView struct {
	Entities []service.Entity
	// Kind is the kind that the list shows, or "" for every kind.
	Kind string
	// Archived adds the archived entities to the list.
	Archived bool
}

func (h *handlers) loadEntities(r *http.Request) (entitiesView, error) {
	view := entitiesView{Kind: r.URL.Query().Get("kind"), Archived: r.URL.Query().Get("archived") == "1"}
	var err error
	view.Entities, err = h.svc.ListEntities(r.Context(), service.ListEntitiesParams{Kind: view.Kind, IncludeArchived: view.Archived, Limit: 200})
	return view, err
}

func (h *handlers) entitiesPage(w http.ResponseWriter, r *http.Request) {
	view, err := h.loadEntities(r)
	var invalid *service.ValidationError
	if errors.As(err, &invalid) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, entitiesPage(view))
}

// aliasList turns the text of the aliases field into a list. A comma
// separates two aliases.
func aliasList(text string) []string {
	var aliases []string
	for _, alias := range strings.Split(text, ",") {
		if alias = strings.TrimSpace(alias); alias != "" {
			aliases = append(aliases, alias)
		}
	}
	return aliases
}

// addEntity stores the entity from the form and renders the list again. An
// entity from the admin pages is one that Gautam typed, so it is manual.
func (h *handlers) addEntity(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	message := ""
	_, err := h.svc.AddEntity(r.Context(), service.AddEntityParams{
		Kind:       r.PostForm.Get("kind"),
		Name:       r.PostForm.Get("name"),
		Aliases:    aliasList(r.PostForm.Get("aliases")),
		WikiPath:   r.PostForm.Get("wiki_path"),
		Provenance: service.Provenance{Source: service.SourceManual},
	})
	var invalid *service.ValidationError
	switch {
	case err == nil:
		// htmx turns this header into an event; the form resets on it.
		w.Header().Set("HX-Trigger", "entity-added")
	case errors.As(err, &invalid):
		message = invalid.Error()
	default:
		listFailure(w, r, err, "The entity was not stored: internal error. Try again.")
		return
	}
	view, err := h.loadEntities(r)
	if err != nil {
		listFailure(w, r, err, "The list did not load: internal error. Reload the page.")
		return
	}
	render(w, r, entitiesList(view, message))
}

// entityView is what the page of one entity shows.
type entityView struct {
	Detail service.EntityDetail
	// Form holds the values of the edit form: the stored values, or the
	// values that the service refused.
	Form entityForm
	// Others are the entities of the same kind that can merge into this one.
	Others  []service.Entity
	Message string
}

// entityForm is the edit form of an entity.
type entityForm struct {
	Name     string
	Aliases  string
	WikiPath string
	Archived bool
}

func entityFormOf(e service.Entity) entityForm {
	return entityForm{Name: e.Name, Aliases: strings.Join(e.Aliases, ", "), WikiPath: e.WikiPath, Archived: e.ArchivedAt != nil}
}

func entityPath(id int64) string { return "/admin/entities/" + strconv.FormatInt(id, 10) }

// showEntity renders the page of one entity. form is the edit form; nil
// shows the stored values. message is the reason of a refusal.
func (h *handlers) showEntity(w http.ResponseWriter, r *http.Request, id int64, form *entityForm, message string) {
	detail, err := h.svc.GetEntityDetail(r.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	view := entityView{Detail: detail, Form: entityFormOf(detail.Entity), Message: message}
	if form != nil {
		view.Form = *form
	}
	others, err := h.svc.ListEntities(r.Context(), service.ListEntitiesParams{Kind: detail.Kind, IncludeArchived: true, Limit: 200})
	if err != nil {
		internalError(w, err)
		return
	}
	for _, other := range others {
		if other.ID != id {
			view.Others = append(view.Others, other)
		}
	}
	render(w, r, entityPage(view))
}

func (h *handlers) entityPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.showEntity(w, r, id, nil, "")
}

// finishEntityWrite answers a write on the page of one entity: a redirect
// back to the page, the reason of a refusal on the page, or 404.
func (h *handlers) finishEntityWrite(w http.ResponseWriter, r *http.Request, id int64, err error, form *entityForm) {
	var invalid *service.ValidationError
	switch {
	case err == nil:
		http.Redirect(w, r, entityPath(id), http.StatusSeeOther)
	case errors.As(err, &invalid):
		h.showEntity(w, r, id, form, invalid.Error())
	case errors.Is(err, service.ErrNotFound):
		http.NotFound(w, r)
	default:
		internalError(w, err)
	}
}

// saveEntity stores the edit form. An empty page removes the stored page,
// and a check box that is not set brings the entity back from the archive.
func (h *handlers) saveEntity(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	form := entityForm{
		Name:     r.PostForm.Get("name"),
		Aliases:  r.PostForm.Get("aliases"),
		WikiPath: strings.TrimSpace(r.PostForm.Get("wiki_path")),
		Archived: r.PostForm.Get("archived") == "true",
	}
	aliases := aliasList(form.Aliases)
	if aliases == nil {
		aliases = []string{}
	}
	params := service.UpdateEntityParams{Name: &form.Name, Aliases: &aliases, Archived: &form.Archived, Actor: service.SourceManual}
	if form.WikiPath == "" {
		params.Clear = []string{"wiki_path"}
	} else {
		params.WikiPath = &form.WikiPath
	}
	_, err := h.svc.UpdateEntity(r.Context(), id, params)
	h.finishEntityWrite(w, r, id, err, &form)
}

// mergeEntity moves the entity of the form into this one.
func (h *handlers) mergeEntity(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	drop, err := strconv.ParseInt(r.PostForm.Get("drop"), 10, 64)
	if err != nil {
		h.showEntity(w, r, id, nil, "Choose the entity to merge into this one.")
		return
	}
	_, err = h.svc.MergeEntities(r.Context(), service.MergeEntitiesParams{Keep: id, Drop: drop, Actor: service.SourceManual})
	h.finishEntityWrite(w, r, id, err, nil)
}

// unlinkEntity takes the link of one record away.
func (h *handlers) unlinkEntity(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	recordID, err := strconv.ParseInt(r.PostForm.Get("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_, err = h.svc.Tag(r.Context(), service.TagParams{
		Type: r.PostForm.Get("type"), ID: recordID, Remove: []service.EntityRef{{ID: id}}, Actor: service.SourceManual,
	})
	h.finishEntityWrite(w, r, id, err, nil)
}

// deleteEntity deletes one entity. The page of the deleted records can
// bring nothing back, but it holds the entity until Gautam removes it.
func (h *handlers) deleteEntity(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	err := h.svc.DeleteEntity(r.Context(), id, service.DeleteParams{Actor: service.SourceManual})
	switch {
	case err == nil:
		http.Redirect(w, r, "/admin/entities", http.StatusSeeOther)
	case errors.Is(err, service.ErrNotFound):
		http.NotFound(w, r)
	default:
		internalError(w, err)
	}
}

// recordHref is the address of the page that shows or edits a record, or ""
// when the record has none.
func recordHref(recordType string, id int64) string {
	switch recordType {
	case "item":
		return itemPath(id) + "/edit"
	case "journal":
		return journalPath(id) + "/edit"
	case "todo":
		return todoPath(id) + "/edit"
	case "source":
		return fmt.Sprintf("/admin/sources/%d", id)
	}
	return ""
}

// projectNames returns the names of the live projects, for the list that
// the project field offers.
func (h *handlers) projectNames(r *http.Request) ([]string, error) {
	projects, err := h.svc.ListEntities(r.Context(), service.ListEntitiesParams{Kind: "project", IncludeArchived: true, Limit: 200})
	if err != nil {
		return nil, err
	}
	names := make([]string, len(projects))
	for i, p := range projects {
		names[i] = p.Name
	}
	return names, nil
}
