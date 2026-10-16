package admin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// onToday says that a click came from the summary page. htmx sends the ID
// of the part of the page that the response replaces.
func onToday(r *http.Request) bool {
	return r.Header.Get("HX-Target") == "today"
}

func (h *handlers) todayPage(w http.ResponseWriter, r *http.Request) {
	view, err := h.svc.Overview(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, todayPage(view))
}

// showToday renders the summary again after a write, with message above
// its lists.
func (h *handlers) showToday(w http.ResponseWriter, r *http.Request, message string) {
	view, err := h.svc.Overview(r.Context())
	if err != nil {
		listFailure(w, r, err, "The page did not load: internal error. Reload the page.")
		return
	}
	render(w, r, todayPart(view, message))
}

// lead says the state of the day in one line.
func lead(view service.Overview) string {
	var parts []string
	if total := len(view.Habits.Habits); total > 0 {
		parts = append(parts, strconv.Itoa(view.Today.HabitsDone)+" of "+count(total, "habit", "habits")+" done.")
	}
	todos := count(view.OpenTodos, "todo", "todos") + " open"
	if view.DueTodos > 0 {
		todos += ", " + strconv.Itoa(view.DueTodos) + " due"
	}
	parts = append(parts, todos+".")
	return strings.Join(parts, " ")
}

// dayName names a day of the summary: Today, Yesterday, or the date.
func dayName(view service.Overview, day string) string {
	switch day {
	case view.Today.Date:
		return "Today"
	case view.Yesterday.Date:
		return "Yesterday"
	}
	return shortDay(day)
}

// dueLine says when an open todo is due, and late says that the day is
// over.
func dueLine(view service.Overview, todo service.Todo) (line string, late bool) {
	switch {
	case todo.DueOn == "":
		return "", false
	case todo.DueOn == view.Today.Date:
		return "Due today", false
	case todo.DueOn < view.Today.Date:
		return "Was due " + shortDay(todo.DueOn), true
	}
	return "Due " + shortDay(todo.DueOn), false
}

// recordNames gives the word for a row of each table in the activity.
var recordNames = map[string]string{
	"catalog_items":   "item",
	"journal_entries": "journal entry",
	"todos":           "todo",
	"habits":          "habit",
	"habit_logs":      "habit log",
	"sources":         "source",
}

// activityRecord names the record of one event, and gives the address of
// its edit page when the record has one and is not deleted.
func activityRecord(a service.Activity) (name, href string) {
	if a.RecordType == "wiki" {
		return "wiki " + a.Path, ""
	}
	word, ok := recordNames[a.RecordType]
	if !ok {
		word = a.RecordType
	}
	name = word + " " + strconv.FormatInt(a.RecordID, 10)
	if a.Action == "delete" || a.Action == "purge" {
		return name, ""
	}
	switch a.RecordType {
	case "catalog_items":
		href = itemPath(a.RecordID) + "/edit"
	case "journal_entries":
		href = journalPath(a.RecordID) + "/edit"
	case "todos":
		href = todoPath(a.RecordID) + "/edit"
	}
	return name, href
}

// activityDay returns the IST day of an event, as the service names a day.
func activityDay(a service.Activity) string {
	return a.At.In(service.IST).Format(time.DateOnly)
}
