package mcp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// addHabit stores one habit through the service. No tool adds a habit:
// Gautam adds a habit in the admin pages.
func addHabit(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	_, err := service.New(pool).AddHabit(context.Background(), service.AddHabitParams{
		Name: name, Provenance: service.Provenance{Source: service.SourceManual, Verified: true},
	})
	if err != nil {
		t.Fatalf("add habit %q: %v", name, err)
	}
}

func TestLogHabitStoresALogThatTheUserDictated(t *testing.T) {
	session, pool := connect(t)
	addHabit(t, pool, "gym")

	logged := structured(t, call(t, session, "log_habit", map[string]any{"habit": "Gym", "dictated": true}))

	if logged["habit"] != "gym" || logged["logged_on"] != todayIST() {
		t.Errorf("log = %v, want gym for today in IST (%s)", logged, todayIST())
	}
	if logged["source"] != "agent" || logged["verified"] != true {
		t.Errorf("source, verified = %v, %v, want agent, true", logged["source"], logged["verified"])
	}
	got := structured(t, call(t, session, "get", map[string]any{"type": "habit_log", "id": logged["id"]}))
	if got["habit"] != "gym" {
		t.Errorf("get returned %v", got)
	}

	// The same habit and day again: the same log, and no error.
	again := structured(t, call(t, session, "log_habit", map[string]any{"habit": "gym", "dictated": true}))
	if again["id"] != logged["id"] {
		t.Errorf("second call returned the log %v, want %v", again["id"], logged["id"])
	}
}

// A log that the agent infers, for example from "went to the gym" in a
// journal entry, waits for review. The user approves it or deletes it.
func TestLogHabitIsAProposalUnlessDictated(t *testing.T) {
	session, pool := connect(t)
	addHabit(t, pool, "gym")

	logged := structured(t, call(t, session, "log_habit", map[string]any{"habit": "gym", "logged_on": "2026-01-05"}))

	if logged["verified"] != false {
		t.Fatalf("verified = %v, want false when dictated is absent", logged["verified"])
	}
	queue := structured(t, call(t, session, "review_queue", map[string]any{}))
	entries := queue["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("queue = %v, want the log", queue)
	}
	if entry := entries[0].(map[string]any); entry["type"] != "habit_log" || entry["id"] != logged["id"] || entry["summary"] != "habit: gym on 2026-01-05" {
		t.Errorf("entry = %v", entry)
	}

	// A log has no field to correct. The error tells the model what to do.
	update := call(t, session, "update", map[string]any{"type": "habit_log", "id": logged["id"], "fields": map[string]any{"logged_on": "2026-01-04"}})
	if !update.IsError || !strings.Contains(text(update), "delete the log") {
		t.Errorf("update: IsError = %v, text = %q; want an error that says to delete the log", update.IsError, text(update))
	}
	refused := call(t, session, "delete", map[string]any{"type": "habit_log", "id": logged["id"]})
	if !refused.IsError || !strings.Contains(text(refused), "confirmed") {
		t.Errorf("delete with no confirmation: IsError = %v, text = %q", refused.IsError, text(refused))
	}
	deleted := structured(t, call(t, session, "delete", map[string]any{"type": "habit_log", "id": logged["id"], "confirmed": true}))
	if deleted["deleted"] != true {
		t.Errorf("delete returned %v", deleted)
	}
}

func TestLogHabitReturnsErrorsTheModelCanRead(t *testing.T) {
	session, pool := connect(t)
	addHabit(t, pool, "gym")
	addHabit(t, pool, "read")
	tests := []struct {
		name     string
		args     map[string]any
		wantText string
	}{
		// The names let the model ask the user which habit they mean.
		{"habit that is not in the table", map[string]any{"habit": "workout"}, "the active habits are: gym, read"},
		{"missing habit", map[string]any{"logged_on": "2026-01-05"}, "habit"},
		{"date in the future", map[string]any{"habit": "gym", "logged_on": "2999-01-01"}, "logged_on"},
		{"misspelled argument", map[string]any{"habit": "gym", "date": "2026-01-05"}, "date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := call(t, session, "log_habit", tt.args)
			if !result.IsError {
				t.Fatalf("IsError = false, want an error result")
			}
			if !strings.Contains(text(result), tt.wantText) {
				t.Errorf("error text %q does not name %q", text(result), tt.wantText)
			}
		})
	}
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM habit_logs").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("habit_logs has %d rows after rejected calls, want 0", n)
	}

	pool.Close()
	result := call(t, session, "log_habit", map[string]any{"habit": "gym"})
	if !result.IsError || text(result) != "internal error" {
		t.Errorf("database failure: IsError = %v, text = %q; want only \"internal error\"", result.IsError, text(result))
	}
}

// Each tool of this plan must be in the list that a client reads, with a
// description.
func TestServerListsTheToolsForDailyRecords(t *testing.T) {
	session, _ := connect(t)

	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	described := map[string]bool{}
	for _, tool := range list.Tools {
		described[tool.Name] = tool.Description != ""
	}
	for _, name := range []string{"journal", "todo", "log_habit"} {
		if !described[name] {
			t.Errorf("tool %s is not registered with a description", name)
		}
	}
}
