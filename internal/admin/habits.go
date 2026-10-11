package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// loadHabits reads the three lists that the habits page shows.
func (h *handlers) loadHabits(ctx context.Context) (habitsView, error) {
	var view habitsView
	var err error
	if view.Today, err = h.svc.HabitsToday(ctx); err != nil {
		return view, err
	}
	if view.Habits, err = h.svc.ListHabits(ctx); err != nil {
		return view, err
	}
	view.Logs, err = h.svc.ListHabitLogs(ctx, 0)
	return view, err
}

func (h *handlers) habitsPage(w http.ResponseWriter, r *http.Request) {
	view, err := h.loadHabits(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, habitsPage(view))
}

// showHabits renders the lists again after a write. When err is a refusal
// of the service or a record that is not there, the lists show the reason.
// what names the write for the message of each other failure.
func (h *handlers) showHabits(w http.ResponseWriter, r *http.Request, err error, notFound, what string) {
	message := ""
	var invalid *service.ValidationError
	switch {
	case err == nil:
	case errors.As(err, &invalid):
		message = invalid.Error()
	case errors.Is(err, service.ErrNotFound):
		message = notFound
	default:
		listFailure(w, r, err, what+": internal error. Try again.")
		return
	}
	view, err := h.loadHabits(r.Context())
	if err != nil {
		listFailure(w, r, err, "The list did not load: internal error. Reload the page.")
		return
	}
	render(w, r, habitsList(view, message))
}

// addHabit stores the habit from the form. A habit from the admin pages is
// one that Gautam typed, so it is manual and verified.
func (h *handlers) addHabit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	_, err := h.svc.AddHabit(r.Context(), service.AddHabitParams{
		Name:       r.PostForm.Get("name"),
		Provenance: service.Provenance{Source: service.SourceManual, Verified: true},
	})
	if err == nil {
		// htmx turns this header into an event; the form resets on it.
		w.Header().Set("HX-Trigger", "habit-added")
	}
	h.showHabits(w, r, err, "", "The habit was not stored")
}

// saveHabit stores the name and the active state of one habit. A browser
// sends no value for a check box that is not set.
func (h *handlers) saveHabit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := r.PostForm.Get("name")
	active := r.PostForm.Get("active") == "true"
	_, err := h.svc.UpdateHabit(r.Context(), id, service.UpdateHabitParams{Name: &name, Active: &active, Actor: service.SourceManual})
	h.showHabits(w, r, err, fmt.Sprintf("Habit %d is not in the list.", id), "The habit was not changed")
}

// logHabitToday logs the habit of the form for today. A click in the admin
// pages is Gautam's own statement.
func (h *handlers) logHabitToday(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	_, err := h.svc.LogHabit(r.Context(), service.LogHabitParams{
		Habit:      r.PostForm.Get("habit"),
		Provenance: service.Provenance{Source: service.SourceManual, Verified: true},
	})
	h.showHabits(w, r, err, "", "The habit was not logged")
}

// deleteHabitLog deletes one log.
func (h *handlers) deleteHabitLog(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	err := h.svc.DeleteHabitLog(r.Context(), id, service.DeleteParams{Actor: service.SourceManual})
	h.showHabits(w, r, err, fmt.Sprintf("Log %d is not in the list. It was possibly deleted before.", id), "The log was not deleted")
}
