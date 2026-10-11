package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// addHabit stores one habit through the service. The API has no route for
// that: Gautam adds a habit in the admin pages.
func addHabit(t *testing.T, pool *pgxpool.Pool, name string) service.Habit {
	t.Helper()
	habit, err := service.New(pool).AddHabit(t.Context(), service.AddHabitParams{
		Name: name, Provenance: service.Provenance{Source: service.SourceManual, Verified: true},
	})
	if err != nil {
		t.Fatalf("add habit %q: %v", name, err)
	}
	return habit
}

// The Siri Shortcut posts {"habit": "gym"} and nothing else.
func TestLogHabitThenReadIt(t *testing.T) {
	h, pool := newTestAPI(t)
	gym := addHabit(t, pool, "gym")

	status, logged, raw := do(t, h, "POST", "/api/habits/log", `{"habit": "gym"}`)

	if status != http.StatusOK {
		t.Fatalf("POST status = %d, want 200\n%s", status, raw)
	}
	if logged["habit"] != "gym" || logged["habit_id"] != float64(gym.ID) || logged["logged_on"] != todayIST() {
		t.Errorf("log = %s, want gym for today in IST (%s)", raw, todayIST())
	}
	if logged["source"] != "manual" || logged["verified"] != true {
		t.Errorf("source, verified = %v, %v, want manual, true", logged["source"], logged["verified"])
	}

	// A second request for the same day returns the same log.
	status, again, raw := do(t, h, "POST", "/api/habits/log", `{"habit": "Gym", "logged_on": "`+todayIST()+`"}`)
	if status != http.StatusOK || again["id"] != logged["id"] {
		t.Errorf("second POST: status = %d, log = %s; want 200 and the log %v", status, raw, logged["id"])
	}
	if n := countEvents(t, pool); n != 2 {
		t.Errorf("events = %d, want 2 (the habit, one log)", n)
	}

	path := fmt.Sprintf("/api/habits/logs/%d", int64(logged["id"].(float64)))
	status, got, raw := do(t, h, "GET", path, "")
	if status != http.StatusOK || got["habit"] != "gym" || got["logged_on"] != todayIST() {
		t.Errorf("GET: status = %d, log = %s", status, raw)
	}
}

func TestLogHabitRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantInBody string
	}{
		{"habit that is not in the table", `{"habit":"gymm"}`, 422, "the active habits are: gym"},
		{"empty habit", `{"habit":" "}`, 422, "body.habit"},
		{"missing habit", `{"logged_on":"2026-01-05"}`, 422, "habit"},
		{"date in the future", `{"habit":"gym","logged_on":"2999-01-01"}`, 422, "body.logged_on"},
		{"field that a caller cannot set", `{"habit":"gym","verified":false}`, 422, "verified"},
		{"request that is not JSON", `habit=gym`, 400, ""},
	}
	h, pool := newTestAPI(t)
	addHabit(t, pool, "gym")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, raw := do(t, h, "POST", "/api/habits/log", tt.body)

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d\n%s", status, tt.wantStatus, raw)
			}
			if !strings.Contains(raw, tt.wantInBody) {
				t.Errorf("body does not name %q:\n%s", tt.wantInBody, raw)
			}
		})
	}
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM habit_logs").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("habit_logs has %d rows after rejected requests, want 0", n)
	}
}

// The Kindle reads this route every 30 minutes and shows the checklist.
func TestHabitsToday(t *testing.T) {
	h, pool := newTestAPI(t)

	status, _, raw := do(t, h, "GET", "/api/habits/today", "")
	if status != http.StatusOK || !strings.Contains(raw, `"habits":[]`) {
		t.Fatalf("no habit: status = %d, body = %s; want 200 and an empty array", status, raw)
	}

	addHabit(t, pool, "read")
	addHabit(t, pool, "gym")
	if status, _, raw := do(t, h, "POST", "/api/habits/log", `{"habit":"gym"}`); status != http.StatusOK {
		t.Fatalf("POST: status = %d\n%s", status, raw)
	}

	status, got, raw := do(t, h, "GET", "/api/habits/today", "")
	if status != http.StatusOK || got["date"] != todayIST() {
		t.Fatalf("status = %d, body = %s; want 200 and the date %s", status, raw, todayIST())
	}
	habits := got["habits"].([]any)
	if len(habits) != 2 {
		t.Fatalf("habits = %s, want gym and read", raw)
	}
	gym, read := habits[0].(map[string]any), habits[1].(map[string]any)
	if gym["name"] != "gym" || gym["done"] != true || read["name"] != "read" || read["done"] != false {
		t.Errorf("habits = %s, want gym done and read not done", raw)
	}
}

func TestDeleteHabitLog(t *testing.T) {
	h, pool := newTestAPI(t)
	addHabit(t, pool, "gym")
	_, logged, _ := do(t, h, "POST", "/api/habits/log", `{"habit":"gym"}`)
	path := fmt.Sprintf("/api/habits/logs/%d", int64(logged["id"].(float64)))

	status, _, raw := do(t, h, "DELETE", path, "")

	if status != http.StatusNoContent || raw != "" {
		t.Fatalf("DELETE: status = %d, body = %q; want 204 and no body", status, raw)
	}
	if status, _, _ := do(t, h, "GET", path, ""); status != http.StatusNotFound {
		t.Errorf("GET after DELETE: status = %d, want 404", status)
	}
	if _, _, raw := do(t, h, "GET", "/api/habits/today", ""); !strings.Contains(raw, `"done":false`) {
		t.Errorf("the checklist still shows the habit as done:\n%s", raw)
	}
	for _, target := range []string{path, "/api/habits/logs/9999"} {
		if status, _, raw := do(t, h, "DELETE", target, ""); status != http.StatusNotFound {
			t.Errorf("DELETE %s: status = %d, want 404\n%s", target, status, raw)
		}
	}
	if status, _, raw := do(t, h, "GET", "/api/habits/logs/abc", ""); status != http.StatusUnprocessableEntity {
		t.Errorf("ID that is not a number: status = %d, want 422\n%s", status, raw)
	}
}

func TestDatabaseFailureOnAHabitRouteGivesAPlain500(t *testing.T) {
	h, pool := newTestAPI(t)
	pool.Close()

	requests := []struct{ method, path, body string }{
		{"POST", "/api/habits/log", `{"habit":"gym"}`},
		{"GET", "/api/habits/today", ""},
		{"GET", "/api/habits/logs/1", ""},
		{"DELETE", "/api/habits/logs/1", ""},
	}
	for _, r := range requests {
		status, body, raw := do(t, h, r.method, r.path, r.body)
		if status != http.StatusInternalServerError {
			t.Errorf("%s %s: status = %d, want 500\n%s", r.method, r.path, status, raw)
		}
		if body["detail"] != "internal error" || body["errors"] != nil {
			t.Errorf("%s %s: the response is not the plain error:\n%s", r.method, r.path, raw)
		}
	}
}
