package admin_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// addStoredHabit stores one habit through the service and returns it.
func addStoredHabit(t *testing.T, pool *pgxpool.Pool, name string) service.Habit {
	t.Helper()
	habit, err := service.New(pool).AddHabit(t.Context(), service.AddHabitParams{
		Name: name, Provenance: service.Provenance{Source: service.SourceManual, Verified: true},
	})
	if err != nil {
		t.Fatalf("add habit %q: %v", name, err)
	}
	return habit
}

// logStored logs a habit for today through the service and returns the log.
// verified false gives a log as an agent infers it.
func logStored(t *testing.T, pool *pgxpool.Pool, habit string, verified bool) service.HabitLog {
	t.Helper()
	source := service.SourceManual
	if !verified {
		source = service.SourceAgent
	}
	log, err := service.New(pool).LogHabit(t.Context(), service.LogHabitParams{
		Habit: habit, Provenance: service.Provenance{Source: source, Verified: verified},
	})
	if err != nil {
		t.Fatalf("log %q: %v", habit, err)
	}
	return log
}

func TestHabitsPageShowsTheFormAndNoHabits(t *testing.T) {
	h, _ := newTestAdmin(t)

	rec := get(h, "/admin/habits")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`hx-post="/admin/habits"`, `name="name"`, "No habits yet.", "No logs yet.", `datetime="` + todayIST() + `"`, `href="/admin/habits"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %q", want)
		}
	}
}

func TestAddHabitStoresTheHabitAndShowsItInTheChecklist(t *testing.T) {
	h, pool := newTestAdmin(t)

	rec := postForm(h, "/admin/habits", url.Values{"name": {" Gym "}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("HX-Trigger"); got != "habit-added" {
		t.Errorf("HX-Trigger = %q, want habit-added so that the form resets", got)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(body), `<div id="list">`) {
		t.Errorf("the response is not the #list fragment:\n%s", body)
	}
	for _, want := range []string{`<span class="row-main">gym</span>`, "not done", `name="habit" value="gym"`, `value="gym"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the list does not contain %q:\n%s", want, body)
		}
	}
	habits, err := service.New(pool).ListHabits(t.Context())
	if err != nil || len(habits) != 1 {
		t.Fatalf("habits = %v, %v, want one habit", habits, err)
	}
	if got := habits[0]; got.Name != "gym" || !got.Active || got.Source != service.SourceManual || !got.Verified {
		t.Errorf("habit = %+v, want gym, active, manual, verified", got)
	}

	for name, wantText := range map[string]string{"gym": "name: a habit has this name already", " ": "name: must not be empty"} {
		rec := postForm(h, "/admin/habits", url.Values{"name": {name}})
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), wantText) {
			t.Errorf("name %q: status = %d, want 200 with %q\n%s", name, rec.Code, wantText, rec.Body)
		}
		if got := rec.Header().Get("HX-Trigger"); got != "" {
			t.Errorf("name %q: HX-Trigger = %q, want none so that the form keeps its input", name, got)
		}
	}
	if n := countEvents(t, pool); n != 1 {
		t.Errorf("events = %d, want 1 (the create)", n)
	}
}

// One click logs a habit for today. The row is Gautam's own.
func TestLogHabitTodayMarksTheHabitDone(t *testing.T) {
	h, pool := newTestAdmin(t)
	addStoredHabit(t, pool, "gym")
	addStoredHabit(t, pool, "read")

	rec := postForm(h, "/admin/habits/log", url.Values{"habit": {"gym"}})

	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(body), `<div id="list">`) {
		t.Fatalf("status = %d, want 200 with the #list fragment\n%s", rec.Code, body)
	}
	// The habit that is done has no Log button. The other habit keeps it.
	if strings.Contains(body, `name="habit" value="gym"`) || !strings.Contains(body, `name="habit" value="read"`) {
		t.Errorf("the checklist must have a Log button for read and not for gym:\n%s", body)
	}
	logs, err := service.New(pool).ListHabitLogs(t.Context(), 0)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs = %v, %v, want one log", logs, err)
	}
	if got := logs[0]; got.Habit != "gym" || got.LoggedOn != todayIST() || got.Source != service.SourceManual || !got.Verified {
		t.Errorf("log = %+v, want gym for today, manual, verified", got)
	}
	if !strings.Contains(body, fmt.Sprintf(`hx-post="/admin/habits/logs/%d/delete"`, logs[0].ID)) {
		t.Errorf("the list of logs has no Delete button for the new log:\n%s", body)
	}

	// A second click, for example from a page that is out of date.
	postForm(h, "/admin/habits/log", url.Values{"habit": {"gym"}})
	if n := countEvents(t, pool); n != 3 {
		t.Errorf("events = %d, want 3 (two habits, one log)", n)
	}
	unknown := postForm(h, "/admin/habits/log", url.Values{"habit": {"run"}})
	if unknown.Code != http.StatusOK || !strings.Contains(unknown.Body.String(), "no active habit has the name") {
		t.Errorf("unknown habit: status = %d, want 200 with the reason\n%s", unknown.Code, unknown.Body)
	}
}

// A log that an agent inferred does not count as done. A click on Log is
// Gautam's own statement, so it approves that log.
func TestLogHabitTodayApprovesALogThatWaitsForReview(t *testing.T) {
	h, pool := newTestAdmin(t)
	addStoredHabit(t, pool, "gym")
	inferred := logStored(t, pool, "gym", false)
	if page := get(h, "/admin/habits").Body.String(); !strings.Contains(page, `name="habit" value="gym"`) {
		t.Fatalf("the checklist shows gym as done although its log waits for review:\n%s", page)
	}

	postForm(h, "/admin/habits/log", url.Values{"habit": {"gym"}})

	got, err := service.New(pool).GetHabitLog(t.Context(), inferred.ID)
	if err != nil || !got.Verified {
		t.Errorf("log = %+v, %v, want it verified", got, err)
	}
	if page := get(h, "/admin/habits").Body.String(); strings.Contains(page, `name="habit" value="gym"`) {
		t.Errorf("the checklist still has a Log button for gym:\n%s", page)
	}
}

func TestSaveHabitRenamesAndRetiresTheHabit(t *testing.T) {
	h, pool := newTestAdmin(t)
	habit := addStoredHabit(t, pool, "gymm")
	addStoredHabit(t, pool, "read")
	path := fmt.Sprintf("/admin/habits/%d", habit.ID)
	if page := get(h, "/admin/habits").Body.String(); !strings.Contains(page, `hx-post="`+path+`"`) || !strings.Contains(page, `name="active" value="true" checked`) {
		t.Fatalf("the page has no form to change the habit:\n%s", page)
	}

	// A browser sends no value for a check box that is not set.
	rec := postForm(h, path, url.Values{"name": {"Gym"}})

	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(body), `<div id="list">`) {
		t.Fatalf("status = %d, want 200 with the #list fragment\n%s", rec.Code, body)
	}
	habits, err := service.New(pool).ListHabits(t.Context())
	if err != nil || len(habits) != 2 || habits[0].Name != "gym" || habits[0].Active {
		t.Fatalf("habits = %+v, %v, want gym, not active", habits, err)
	}
	// A habit that is not active is not in the checklist of today.
	if strings.Contains(body, `name="habit" value="gym"`) {
		t.Errorf("the checklist still shows the habit that is not active:\n%s", body)
	}

	refused := map[string]string{
		"read": "name: a habit has this name already",
		" ":    "name: must not be empty",
	}
	for name, wantText := range refused {
		rec := postForm(h, path, url.Values{"name": {name}, "active": {"true"}})
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), wantText) {
			t.Errorf("name %q: status = %d, want 200 with %q\n%s", name, rec.Code, wantText, rec.Body)
		}
	}
	gone := postForm(h, "/admin/habits/9999", url.Values{"name": {"run"}, "active": {"true"}})
	if gone.Code != http.StatusOK || !strings.Contains(gone.Body.String(), "Habit 9999 is not in the list") {
		t.Errorf("unknown ID: status = %d, want 200 with a message\n%s", gone.Code, gone.Body)
	}
	if rec := postForm(h, "/admin/habits/abc", url.Values{"name": {"run"}}); rec.Code != http.StatusNotFound {
		t.Errorf("ID that is not a number: status = %d, want 404", rec.Code)
	}
	if n := countEvents(t, pool); n != 3 {
		t.Errorf("events = %d, want 3 (two creates, one update)", n)
	}
}

func TestDeleteHabitLogRemovesTheLogAndTheHabitIsNotDone(t *testing.T) {
	h, pool := newTestAdmin(t)
	addStoredHabit(t, pool, "gym")
	log := logStored(t, pool, "gym", true)
	path := fmt.Sprintf("/admin/habits/logs/%d/delete", log.ID)
	if page := get(h, "/admin/habits").Body.String(); !strings.Contains(page, `hx-confirm="Delete the log of gym on `+todayIST()+`?"`) {
		t.Errorf("the Delete button does not ask for a confirmation:\n%s", page)
	}

	rec := postForm(h, path, nil)

	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(body), `<div id="list">`) {
		t.Fatalf("status = %d, want 200 with the #list fragment\n%s", rec.Code, body)
	}
	if !strings.Contains(body, "No logs yet.") || !strings.Contains(body, `name="habit" value="gym"`) {
		t.Errorf("the list must show no log, and a Log button for gym:\n%s", body)
	}

	again := postForm(h, path, nil)
	if again.Code != http.StatusOK || !strings.Contains(again.Body.String(), fmt.Sprintf("Log %d is not in the list", log.ID)) {
		t.Errorf("second delete: status = %d, want 200 with a message\n%s", again.Code, again.Body)
	}
	if n := countEvents(t, pool); n != 3 {
		t.Errorf("events = %d, want 3 (the habit, the log, one delete)", n)
	}
}

// A name goes into the text of the page and into the value of a field.
func TestHabitsPageEscapesTheName(t *testing.T) {
	h, pool := newTestAdmin(t)
	addStoredHabit(t, pool, `a"<b>x`)

	body := get(h, "/admin/habits").Body.String()

	if strings.Contains(body, "<b>x") {
		t.Error("the page contains the name as raw HTML")
	}
	for _, want := range []string{`value="a&#34;&lt;b&gt;x"`, `<span class="row-main">a&#34;&lt;b&gt;x</span>`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %s", want)
		}
	}
}

func TestDatabaseFailureOnTheHabitPages(t *testing.T) {
	h, pool := newTestAdmin(t)
	pool.Close()

	if rec := get(h, "/admin/habits"); rec.Code != http.StatusInternalServerError || strings.TrimSpace(rec.Body.String()) != "internal error" {
		t.Errorf("page: status = %d, body = %q; want 500 and only \"internal error\"", rec.Code, rec.Body)
	}
	clicks := map[string]url.Values{
		"/admin/habits":               {"name": {"gym"}},
		"/admin/habits/log":           {"habit": {"gym"}},
		"/admin/habits/1":             {"name": {"gym"}},
		"/admin/habits/logs/1/delete": nil,
	}
	for path, form := range clicks {
		rec := postForm(h, path, form)
		if rec.Code != http.StatusOK {
			t.Errorf("POST %s: status = %d, want 200: htmx does not show a 5xx response", path, rec.Code)
		}
		page := strings.TrimSpace(rec.Body.String())
		if !strings.HasPrefix(page, `<div id="list"><p class="error" role="alert">`) || !strings.Contains(page, "internal error") {
			t.Errorf("POST %s: the response is not the list with the error:\n%s", path, page)
		}
	}
}

// A habit log has no edit page. The review queue links to the habits page,
// where Gautam can delete a log that is wrong.
func TestReviewPageLinksAHabitLogToTheHabitsPage(t *testing.T) {
	h, pool := newTestAdmin(t)
	addStoredHabit(t, pool, "gym")
	log := logStored(t, pool, "gym", false)

	body := get(h, "/admin/review").Body.String()

	for _, want := range []string{
		"habit: gym on " + todayIST(),
		`name="type" value="habit_log"`,
		`<a href="/admin/habits">Habits</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %s", want)
		}
	}
	if strings.Contains(body, fmt.Sprintf(`/%d/edit"`, log.ID)) {
		t.Error("the habit log has an Edit link, and no edit page exists for it")
	}

	rec := postForm(h, "/admin/review/verify", url.Values{"type": {"habit_log"}, "id": {fmt.Sprint(log.ID)}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "No record waits for review.") {
		t.Errorf("after the approval the queue must be empty:\n%s", rec.Body)
	}
}
