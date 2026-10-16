package mcp_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// todayIST is today's date for Gautam, as the service computes it.
func todayIST() string {
	return time.Now().In(service.IST).Format(time.DateOnly)
}

// The journal holds the user's own words. The tool must store them as they
// came.
func TestJournalStoresTheWordsOfTheUser(t *testing.T) {
	session, pool := connect(t)
	const body = "spent the entire day planning   a second brain\n\nno gym :(\n"

	added := structured(t, call(t, session, "journal", map[string]any{"body": body}))

	if added["body"] != body {
		t.Errorf("body = %q, want it unchanged: %q", added["body"], body)
	}
	if added["source"] != "agent" {
		t.Errorf("source = %v, want agent", added["source"])
	}
	if added["entry_date"] != todayIST() {
		t.Errorf("entry_date = %v, want today in IST (%s)", added["entry_date"], todayIST())
	}
	got := structured(t, call(t, session, "get", map[string]any{"type": "journal", "id": added["id"]}))
	if got["body"] != body {
		t.Errorf("get returned %v", got)
	}
	var source string
	if err := pool.QueryRow(context.Background(), "SELECT source FROM events WHERE record_type = 'journal_entries'").Scan(&source); err != nil {
		t.Fatalf("read the event (want exactly one row): %v", err)
	}
	if source != "agent" {
		t.Errorf("event source = %q, want agent", source)
	}
}

func TestJournalToolTellsTheModelNotToRewriteTheEntry(t *testing.T) {
	session, _ := connect(t)

	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range list.Tools {
		if tool.Name == "journal" {
			if !strings.Contains(tool.Description, "own words") {
				t.Errorf("the description of journal does not say that the entry holds the user's own words: %q", tool.Description)
			}
			return
		}
	}
	t.Error("tool journal is not registered")
}

// A change of the day keeps the words of the entry.
func TestUpdateMovesAJournalEntryToADifferentDay(t *testing.T) {
	session, _ := connect(t)
	added := structured(t, call(t, session, "journal", map[string]any{"body": "felt gr8", "entry_date": "2026-01-05"}))

	updated := structured(t, call(t, session, "update", map[string]any{
		"type": "journal", "id": added["id"], "fields": map[string]any{"entry_date": "2026-01-04"},
	}))
	if updated["body"] != "felt gr8" || updated["entry_date"] != "2026-01-04" {
		t.Errorf("update returned %v", updated)
	}
}

func TestDeleteOfAJournalEntryNeedsTheConfirmationOfTheUser(t *testing.T) {
	session, _ := connect(t)
	added := structured(t, call(t, session, "journal", map[string]any{"body": "a day"}))

	refused := call(t, session, "delete", map[string]any{"type": "journal", "id": added["id"]})
	if !refused.IsError || !strings.Contains(text(refused), "confirmed") {
		t.Fatalf("delete with no confirmation: IsError = %v, text = %q", refused.IsError, text(refused))
	}

	deleted := structured(t, call(t, session, "delete", map[string]any{"type": "journal", "id": added["id"], "confirmed": true}))
	if deleted["deleted"] != true {
		t.Errorf("delete returned %v", deleted)
	}
	if result := call(t, session, "get", map[string]any{"type": "journal", "id": added["id"]}); !result.IsError || text(result) != "not found" {
		t.Errorf("get after delete: IsError = %v, text = %q", result.IsError, text(result))
	}
}

func TestJournalReturnsErrorsTheModelCanRead(t *testing.T) {
	session, pool := connect(t)
	tests := []struct {
		name     string
		args     map[string]any
		wantText string
	}{
		{"empty body", map[string]any{"body": " \n"}, "body"},
		{"missing body", map[string]any{"entry_date": "2026-01-05"}, "body"},
		{"date in the future", map[string]any{"body": "x", "entry_date": "2999-01-01"}, "entry_date"},
		{"misspelled argument", map[string]any{"body": "x", "date": "2026-01-05"}, "date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := call(t, session, "journal", tt.args)
			if !result.IsError {
				t.Fatalf("IsError = false, want an error result")
			}
			if !strings.Contains(text(result), tt.wantText) {
				t.Errorf("error text %q does not name %q", text(result), tt.wantText)
			}
		})
	}
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM journal_entries").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("journal_entries has %d rows after rejected calls, want 0", n)
	}

	pool.Close()
	result := call(t, session, "journal", map[string]any{"body": "a day"})
	if !result.IsError || text(result) != "internal error" {
		t.Errorf("database failure: IsError = %v, text = %q; want only \"internal error\"", result.IsError, text(result))
	}
}

// #39: a log that a journal entry reports names the entry.
func TestLogHabitTakesTheEntryThatReportsIt(t *testing.T) {
	session, pool := connect(t)
	addHabit(t, pool, "run")
	entry := structured(t, call(t, session, "journal", map[string]any{"body": "Went for a run."}))
	id := entry["id"].(float64)

	log := structured(t, call(t, session, "log_habit", map[string]any{"habit": "run", "logged_on": entry["entry_date"], "journal_ref": id}))
	if log["journal_ref"] != id {
		t.Errorf("log = %v, want journal_ref %v", log, id)
	}
}
