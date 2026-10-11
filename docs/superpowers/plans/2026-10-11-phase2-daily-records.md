# Phase 2 Daily Records (Journal, Todos, Habits) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Gautam can write a journal entry, add a todo and mark it done, and log a habit, through the service, the REST API, the MCP server and the admin pages.

**Architecture:** One migration adds four tables: `journal_entries`, `todos`, `habits` and `habit_logs`. `internal/service` gets the functions for each table, with the same rules as list items: validation, provenance, one event for each write, soft delete. Three new record types (`journal`, `todo`, `habit_log`) join `Get`, `Update`, `Delete`, `Verify` and `ReviewQueue`. The REST API, the MCP tools and the admin pages stay thin adapters: each reads its input, calls the service and formats the result.

**Tech Stack:** No new dependency. Go 1.26.0, chi v5.3.2, Huma v2.39.1, pgx v5.11.0, sqlc v1.31.1, goose v3.28.0, MCP Go SDK v1.8.0, templ v0.3.1070, htmx 2.0.11, Postgres 18 in Docker.

**Spec:** `docs/design.md` (section "Data model": the rows `journal_entries`, `habits`, `habit_logs` and `todos`, and the provenance fields; section "Access layer": the rows "Write a journal entry", "Log a habit" and "Add or update a todo"; workflows S1, S2, S4, S18 and S25; edge cases E3 and E4) and `AGENTS.md`. Read both before you start a task.

**Scope:** This is plan 3 of Phase 2. Plans 1 and 2 are complete and confirmed. This plan stops at a confirmation gate (Task 14). The section "After this plan" lists the Phase 2 work that later plans cover. Do not start that work in this plan.

**Tested:** Every code block in this plan comes from a scratch copy of the repository. In that copy each test failed first, then passed after its code was added, in the task order below. Then the steps of this file were done one more time on a fresh clone, by a script that reads this file.

## Changes after the review

The work is complete. One change was made after the final review, so the code in the repository differs from the code blocks below in this point.

- **The description of the tool `log_habit` changed.** It said that a second call for the same day changes nothing. That is false when the user states a log that the agent proposed before: the second call approves the log. The description now tells the model to call again with `dictated` set to true. The test is `TestLogHabitAgainAsDictatedApprovesAProposedLog`. Commit: `fix(mcp): tell the model that a stated habit log approves a proposed one`.
- **The order of two steps changed.** The final review ran before Step 7 of Task 14, because a migration that is applied cannot be edited.

## Decisions in this plan

The spec names the four tables and does not give their columns or their rules. Gautam approved the columns. The plan decides the other points as follows. Gautam can change a decision before the work starts.

**Columns.** Each table also has the provenance columns, `deleted_at` and the `set_updated_at()` trigger.

| Table | Columns |
| --- | --- |
| `journal_entries` | `body` (text, stored as written), `entry_date` (the IST day that the entry is about; the default is today) |
| `todos` | `title`, `status` (`open`, `done`, `dropped`), `due_on` (optional date), `project` (optional text), `done_at` |
| `habits` | `name` (unique, in lower case), `active` |
| `habit_logs` | `habit_id`, `logged_on` (the IST day); one live log for each habit and day |

**Journal**

- **The body is stored exactly as it comes.** The service does not trim it or change it. It refuses only a body that is empty, or that has a NUL character or bytes that are not UTF-8.
- **The `journal` tool always writes a verified row.** The words are Gautam's own (S1). The tool has no `dictated` argument, because no agent infers a journal entry.
- **An agent cannot reword an entry.** The rule from plan 2 does this with no new code: an inferred change to a verified record is refused (E4).
- **The admin form changes CR LF to LF.** A browser sends each line break of a text area as two characters. Without this, a click on Save with no change is a change.

**Todos**

- **The tool `todo` adds. The tool `update` changes.** The spec says "Add or update a todo" for `todo`. Two tools that change a todo give a model two ways to do one thing. To mark a todo done, an agent calls `update` with the type `todo` and `{"status": "done"}`.
- **`done_at` follows the status.** The service sets it when the status becomes `done` and empties it when the status becomes a different value. No caller sets it.
- **A due date can be in the past.** That is a todo that is late.
- **`project` is free text for now.** The backlog has the `projects` table and the `description` field.
- **A list shows the open todos first.** After that, the newest comes first.

**Habits**

- **A habit (the definition) is not a record type.** Gautam adds, renames and retires a habit in the admin pages only. No REST route and no MCP tool does that. A habit is not deleted: Gautam sets it to not active, and its logs stay.
- **A log needs a habit that exists.** `{"habit": "gymm"}` is refused, and the error lists the active habits. A spelling mistake does not make a new habit.
- **A name is in lower case.** "Gym", "gym " and "GYM" are one habit.
- **One log for each habit and day.** A second log for the same day is not a write: the service returns the stored log, with no error and no event.
- **Gautam's log approves an inferred log.** When an agent proposed a log and Gautam then logs the same habit and day, the stored log becomes verified. An inferred log does not change a verified log.
- **"Done" means a verified log.** `GET /api/habits/today` and the admin page count a log only when it is verified, as reports do.
- **A habit log has no field to change.** The habit and the day are the log. `update` refuses the type `habit_log` and tells the caller to delete the log and to log again.
- **`POST /api/habits/log` answers 200,** not 201, because a second request for the same day creates nothing.

**All types**

- **REST lists.** Only todos get a list route (`GET /api/todos`), because the spec names `/api/todos`. The journal and the habit logs have a list in the admin pages.
- **The review queue is in the order of creation,** across the types.
- **Three small changes to code from plans 1 and 2.** `DeleteItem` and the `PATCH` handler of items move their shared steps into one helper each, and `itemID` in the admin package gets the name `pathID`. Each change has its own commit, and the tests of plan 2 must pass before and after it.
- **The real database gets the migration in Task 14.** It only adds tables. It does not change `events` or `catalog_items`.
- **Not in this plan.** The rejection of secrets in free text (plan 4 adds it for the journal body and the titles), search, permanent delete.

## Global Constraints

- **One logic layer.** All validation, provenance and event code is in `internal/service`. An adapter (`internal/api`, `internal/mcp`, `internal/admin`, `cmd/lifeos`) parses input, calls the service and formats the result. No adapter calls a different adapter.
- **SQL only through sqlc.** Queries are in `queries/*.sql`. Only `internal/service` imports `internal/store`. No SQL strings in other non-test Go code.
- **One event for each write.** The service appends one `events` row in the same transaction as the write. An update event and a verify event hold the old row and the new row. A delete event holds the old row. A call that changes no value is not a write.
- **Provenance.** The REST API and the admin pages write `manual`, verified rows. An MCP tool writes `agent` rows, verified only when the tool input says that the user dictated the row. The one exception is the `journal` tool: see "Decisions in this plan".
- **Verified rule.** An update keeps `verified`. The service refuses an inferred change to a verified row. Only `Verify` sets `verified` to `true`; the one more case in this plan is a verified habit log over an inferred one.
- **Soft delete.** A delete sets `deleted_at`. Every read, update, delete and verify leaves out rows where `deleted_at IS NOT NULL`. An agent needs `confirmed` to delete.
- **Time.** Store `timestamptz` in UTC. Show times in IST. "Today" for a date-only field is the IST date.
- **Loopback only.** Every listener is on `127.0.0.1`. Do not add a Cloudflare Tunnel hostname in this plan.
- **Migrations.** `migrations/00001_init.sql` is applied on the Pi. Do not edit it. This plan adds `migrations/00002_daily_records.sql`. Do not edit that file after Task 14 applies it.
- **Generated code is not in git.** `internal/store/` (sqlc) and `*_templ.go` (templ) are in `.gitignore`. Never `git add` them. `make build` and `make test` generate them first.
- **Tests use real Postgres.** Run tests with `make test`, not `go test`. Each test creates its own database and drops it. The Postgres container from `deploy/docker-compose.yml` must be running.
- **Where to work.** Work in `/home/gautam/lifeos-core` on `main`. Do not make a branch or a worktree: `deploy/.env` is not in git and exists only in this checkout. Do not push.
- **Commits.** Conventional commit format, one concern for each commit, with the exact file lists in the steps.
- **Out of scope for this plan.** `sources` and `capture`, search, the read-only `query` user, the rejection of secrets in free text, money tables, CSV importers, API tokens, the admin password, CSRF protection, a systemd unit, permanent delete, the entries in `docs/backlog.md`, and the minor review findings from plans 1 and 2.

## Review Focus

These five conditions are implied by the spec, and no scenario in the spec tests them. Each has a test in the task that owns the code.

1. **"Log gym" two times on one day.** Siri runs the Shortcut two times, or two requests come at the same moment. One log must be stored, and no request must fail. Tests: `TestLogHabitASecondTimeChangesNothing` and `TestLogHabitAtTheSameMomentStoresOneLog` (Task 11), `TestLogHabitThenReadIt` (Task 12).
2. **Gautam logs a habit that an agent proposed before.** His own log must count. If the service drops it, the checklist shows "not done" after he logged the habit. Tests: `TestLogHabitByGautamApprovesALogThatAnAgentInferred` (Task 11), `TestLogHabitTodayApprovesALogThatWaitsForReview` (Task 13).
3. **A journal entry goes through the admin form.** The browser sends CR LF for each line break, and the edit form must give back each character. A click on Save with no change must write nothing. Tests: `TestAddJournalEntryStoresTheTextOfTheForm`, `TestJournalPagesShowTheEntryAsText` and `TestSaveJournalEntryWithNoChangeWritesNoEvent` (Task 5).
4. **A habit name that does not exist.** "gymm", or a habit that is not active, must be refused, and the error must list the habits so that an agent can ask. Tests: `TestLogHabitRejectsBadInput` (Task 11), `TestLogHabitRejectsBadRequests` and `TestLogHabitReturnsErrorsTheModelCanRead` (Task 12).
5. **Two records of different types have the same ID.** Each table starts its IDs at 1. An approval or an Edit link in the review queue must go to the record of the correct type. Tests: `TestEveryRecordTypeHasEveryEntryPoint` and `TestVerifyApprovesAJournalEntry` (Task 3), `TestReviewPageLinksAJournalEntryToItsEditPage` (Task 5), `TestReviewPageLinksATodoToItsEditPage` (Task 9).

Three more conditions have tests although they did not make the list of five: the IST date between 00:00 and 05:30 (`TestAddJournalEntryUsesTheISTDateAsToday`, Task 2; `TestLogHabitStoresALogForTodayInIST`, Task 11), an agent that closes a todo that Gautam stated (`TestUpdateTodoRefusesAnInferredChangeToAVerifiedTodo`, Task 7), and a new log after a deleted log (`TestLogHabitAfterADeleteStoresANewLog`, Task 11).

## File Structure

| File | Task | Responsibility |
| --- | --- | --- |
| `migrations/00002_daily_records.sql` | 1 | The four tables, their indexes and triggers |
| `sqlc.yaml` | 2 | Maps a `date` column that is not null to `time.Time` |
| `queries/journal_entries.sql` | 2, 3 | The journal queries |
| `internal/service/fields.go` | 2 | Shared checks: text, list limit, day |
| `internal/service/journal.go` | 2 | `JournalEntry`, `AddJournalEntry`, `GetJournalEntry`, `ListJournalEntries` |
| `internal/service/delete.go` | 3 | `deleteRecord`: the shared steps of a soft delete |
| `internal/service/journal_change.go` | 3 | `UpdateJournalEntry`, `DeleteJournalEntry`, and the review parts of a journal entry |
| `internal/service/records.go`, `review.go` | 3, 7, 11 | One case for each record type |
| `internal/api/api.go` | 4, 8, 12 | `patch`: the shared steps of a `PATCH`; `idInput` |
| `internal/api/journal.go`, `internal/mcp/journal.go` | 4 | The journal routes and the tool `journal` |
| `internal/admin/journal.templ`, `journal.go` | 5 | The journal page |
| `queries/todos.sql` | 6, 7 | The todo queries |
| `internal/service/todos.go` | 6 | `Todo`, `AddTodo`, `GetTodo`, `ListTodos` |
| `internal/service/todos_change.go` | 7 | `UpdateTodo`, `DeleteTodo`, and the review parts of a todo |
| `internal/api/todos.go`, `internal/mcp/todos.go` | 8 | The todo routes and the tool `todo` |
| `internal/admin/todos.templ`, `todos.go` | 9 | The todos page |
| `queries/habits.sql`, `internal/service/habits.go` | 10 | `Habit`, `AddHabit`, `ListHabits`, `UpdateHabit` |
| `queries/habit_logs.sql` | 11 | The habit log queries |
| `internal/service/habit_logs.go` | 11 | `HabitLog`, `LogHabit`, `GetHabitLog`, `ListHabitLogs`, `HabitsToday` |
| `internal/service/habit_logs_change.go` | 11 | `DeleteHabitLog`, and the review parts of a habit log |
| `internal/api/habits.go`, `internal/mcp/habits.go` | 12 | The habit routes and the tool `log_habit` |
| `internal/admin/habits.templ`, `habits.go` | 13 | The habits page |
| `internal/admin/review.templ` | 5, 9, 13 | One link for each record type |
| `cmd/lifeos/daily_test.go` | 14 | A day of records through every adapter over real HTTP |

## How to read the steps

A step that changes a file uses one of these forms. Do each form exactly.

- **Create `path`:** make the new file with the content of the code block.
- **Replace the whole content of `path` with this:** the code block is the new content of a file that exists.
- **Add this at the end of `path`:** put one empty line after the last line of the file, then the code block.
- **In `path`, replace this: ... With this: ...** The first code block is in the file one time. Put the second code block in its place.
- **In `path`, replace each occurrence of this: ...** The first code block is in the file more than one time. Change each one.

Go code uses tabs. Copy the code blocks with their tabs.

---


### Task 1: Migration: the four tables

**Files:**
- Create: `migrations/00002_daily_records.sql`
- Modify: `migrations/migrations_test.go` (replace the whole file)

**Interfaces:**
- Consumes (plan 1): `migrations.Up(ctx, db) ([]string, error)`, which applies every `migrations/*.sql` file in order; `testdb.New(t) *pgxpool.Pool`, which gives a new database with every migration applied; the function `set_updated_at()` and the provenance columns of `catalog_items` in `migrations/00001_init.sql`.
- Produces: the tables `journal_entries`, `todos`, `habits` and `habit_logs`. Each has `id`, `source`, `source_ref`, `verified`, `created_at`, `updated_at`, `deleted_at` and a trigger `<table>_set_updated_at`. The unique indexes `habits_name_idx` on `habits (name)` and `habit_logs_habit_day_idx` on `habit_logs (habit_id, logged_on)` count only rows where `deleted_at IS NULL`.

- [ ] **Step 1: Check the prerequisites**

Run:
```bash
cd /home/gautam/lifeos-core
git status -sb
docker compose -f deploy/docker-compose.yml ps --format '{{.Name}}: {{.Status}}'
make test
```
Expected: the run passes. The output contains `## main`, `lifeos-postgres-1: Up` and `ok  	github.com/Gautam-J/lifeos-core/internal/service`.

No tracked file is changed, except that this plan file and `docs/backlog.md` can be new or changed. The container is `(healthy)`. `make test` prints `ok` for seven packages. If `docker` reports `permission denied`, put `sudo` before `docker`.

- [ ] **Step 2: Write the failing tests**

The new tests check the tables through SQL, because no Go code uses the tables yet.

Replace the whole content of `migrations/migrations_test.go` with this:

```go
package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/Gautam-J/lifeos-core/internal/testdb"
	"github.com/Gautam-J/lifeos-core/migrations"
)

func TestUpCreatesTheTables(t *testing.T) {
	pool := testdb.New(t) // testdb.New already ran migrations.Up once
	ctx := context.Background()

	for _, table := range []string{"events", "catalog_items", "journal_entries", "todos", "habits", "habit_logs"} {
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = $1)",
			table).Scan(&exists)
		if err != nil {
			t.Fatalf("query %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %s does not exist after Up", table)
		}
	}
}

// Every record table has the provenance columns, deleted_at and the
// trigger that keeps updated_at current.
func TestRecordTablesHaveTheProvenanceColumns(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	for _, table := range []string{"catalog_items", "journal_entries", "todos", "habits", "habit_logs"} {
		for _, column := range []string{"source", "source_ref", "verified", "created_at", "updated_at", "deleted_at"} {
			var exists bool
			err := pool.QueryRow(ctx,
				"SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2)",
				table, column).Scan(&exists)
			if err != nil {
				t.Fatalf("query %s.%s: %v", table, column, err)
			}
			if !exists {
				t.Errorf("table %s has no column %s", table, column)
			}
		}
		var triggers int
		err := pool.QueryRow(ctx,
			"SELECT count(*) FROM pg_trigger WHERE tgrelid = $1::regclass AND tgname = $2",
			table, table+"_set_updated_at").Scan(&triggers)
		if err != nil {
			t.Fatalf("query the trigger of %s: %v", table, err)
		}
		if triggers != 1 {
			t.Errorf("table %s has no trigger %s_set_updated_at", table, table)
		}
	}
}

// The database itself keeps one live log for each habit and day, and one
// live habit for each name. A deleted row does not count.
func TestHabitTablesRefuseADuplicate(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	const addHabit = "INSERT INTO habits (name, source) VALUES ('gym', 'manual') RETURNING id"
	const addLog = "INSERT INTO habit_logs (habit_id, logged_on, source) VALUES ($1, '2026-10-11', 'manual')"

	var habit int64
	if err := pool.QueryRow(ctx, addHabit).Scan(&habit); err != nil {
		t.Fatalf("first habit: %v", err)
	}
	if _, err := pool.Exec(ctx, addHabit); err == nil {
		t.Error("a second live habit with the same name was stored")
	}
	if _, err := pool.Exec(ctx, addLog, habit); err != nil {
		t.Fatalf("first log: %v", err)
	}
	if _, err := pool.Exec(ctx, addLog, habit); err == nil {
		t.Error("a second live log for the same habit and day was stored")
	}

	if _, err := pool.Exec(ctx, "UPDATE habit_logs SET deleted_at = now() WHERE habit_id = $1", habit); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, addLog, habit); err != nil {
		t.Errorf("a new log after a deleted log was refused: %v", err)
	}
}

func TestTodosRefuseAnUnknownStatus(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	const add = "INSERT INTO todos (title, status, source) VALUES ('Buy a domain', $1, 'manual')"

	for _, status := range []string{"open", "done", "dropped"} {
		if _, err := pool.Exec(ctx, add, status); err != nil {
			t.Errorf("status %s: %v", status, err)
		}
	}
	if _, err := pool.Exec(ctx, add, "later"); err == nil {
		t.Error("a todo with the status later was stored")
	}
}

func TestUpAppliesNothingTheSecondTime(t *testing.T) {
	pool := testdb.New(t)
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	applied, err := migrations.Up(context.Background(), db)
	if err != nil {
		t.Fatalf("second Up: %v", err)
	}
	if len(applied) != 0 {
		t.Errorf("second Up applied %v, want nothing", applied)
	}
}
```

- [ ] **Step 3: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./migrations
```
Expected: the run fails. The output contains `table journal_entries does not exist after Up` and `FAIL	github.com/Gautam-J/lifeos-core/migrations`.

- [ ] **Step 4: Write the migration**

Create `migrations/00002_daily_records.sql`:

```sql
-- +goose Up

-- Journal text exactly as Gautam wrote it.
CREATE TABLE journal_entries (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    body        text NOT NULL,       -- stored as written; the service never rewords it
    entry_date  date NOT NULL,       -- the IST day that the entry is about

    source      text NOT NULL CHECK (source IN ('manual', 'import', 'connector', 'agent')),
    source_ref  bigint,
    verified    boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE TRIGGER journal_entries_set_updated_at
    BEFORE UPDATE ON journal_entries
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Tasks.
CREATE TABLE todos (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title       text NOT NULL,
    status      text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'done', 'dropped')),
    due_on      date,
    project     text,
    done_at     timestamptz,         -- set when the status becomes done

    source      text NOT NULL CHECK (source IN ('manual', 'import', 'connector', 'agent')),
    source_ref  bigint,
    verified    boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE TRIGGER todos_set_updated_at
    BEFORE UPDATE ON todos
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Habit definitions.
CREATE TABLE habits (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        text NOT NULL,       -- in lower case
    active      boolean NOT NULL DEFAULT true,

    source      text NOT NULL CHECK (source IN ('manual', 'import', 'connector', 'agent')),
    source_ref  bigint,
    verified    boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE UNIQUE INDEX habits_name_idx ON habits (name) WHERE deleted_at IS NULL;

CREATE TRIGGER habits_set_updated_at
    BEFORE UPDATE ON habits
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Daily check-ins: one live log for each habit and day.
CREATE TABLE habit_logs (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    habit_id    bigint NOT NULL REFERENCES habits (id),
    logged_on   date NOT NULL,       -- the IST day

    source      text NOT NULL CHECK (source IN ('manual', 'import', 'connector', 'agent')),
    source_ref  bigint,
    verified    boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE UNIQUE INDEX habit_logs_habit_day_idx ON habit_logs (habit_id, logged_on) WHERE deleted_at IS NULL;

CREATE TRIGGER habit_logs_set_updated_at
    BEFORE UPDATE ON habit_logs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE habit_logs;
DROP TABLE habits;
DROP TABLE todos;
DROP TABLE journal_entries;
```

- [ ] **Step 5: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./migrations
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/migrations`.

- [ ] **Step 6: Commit**

Run:
```bash
git add migrations/00002_daily_records.sql migrations/migrations_test.go
git commit -m "feat(db): add the journal, todo and habit tables"
```

---

### Task 2: Service: write and read a journal entry

**Files:**
- Create: `queries/journal_entries.sql`, `internal/service/fields.go`, `internal/service/journal.go`
- Modify: `sqlc.yaml` (one more override)
- Test: `internal/service/journal_test.go`

**Interfaces:**
- Consumes (plans 1 and 2, package `service`): `Service` with the fields `pool` and `now`, `IST`, `Provenance` and its method `validate() error`, `Source` and its four values, `invalid(field, message string) error`, `ErrNotFound`, `ValidationError`, `recordEvent(ctx, q, action, recordType string, recordID int64, source Source, oldValue, newValue any) error`, the constants `defaultListLimit` (50) and `maxListLimit` (200). Test helpers in package `service`: `newTestService(t) (*Service, *pgxpool.Pool)`, `ptr`, `countRows(t, pool, table) int`, `equal(a, b []string) bool`, `type event` and `allEvents(t, pool) []event`. The test clock `testNow` is 2026-10-10 20:00 UTC, which is 2026-10-11 in IST.
- Produces (package `service`):
  - `type JournalEntry struct{ ID int64; Body, EntryDate string; Source Source; SourceRef *int64; Verified bool; CreatedAt, UpdatedAt time.Time }` with the JSON names `id`, `body`, `entry_date`, `source`, `source_ref`, `verified`, `created_at`, `updated_at`
  - `type AddJournalEntryParams struct{ Body, EntryDate string; Provenance }` and its method `toRow(now time.Time) (store.InsertJournalEntryParams, error)`
  - `(*Service).AddJournalEntry(ctx, AddJournalEntryParams) (JournalEntry, error)`, `(*Service).GetJournalEntry(ctx, id int64) (JournalEntry, error)`, `(*Service).ListJournalEntries(ctx, limit int) ([]JournalEntry, error)`
  - `journalEntryFromRow(store.JournalEntry) JournalEntry`
  - In `fields.go`: `checkText(field, value string) error`, `checkLimit(limit int) (int32, error)`, `today(now time.Time) time.Time`, `parseDay(field, value string) (time.Time, error)`, `pastDay(field, value string, now time.Time) (time.Time, error)`
  - Test helpers: `addJournal(t, s, body, entryDate string) JournalEntry` (a verified entry), `bodies([]JournalEntry) []string`
  - sqlc: `InsertJournalEntry`, `GetJournalEntry`, `ListJournalEntries`; the type `store.JournalEntry`, where `EntryDate` is a `time.Time`

- [ ] **Step 1: Write the failing tests**

Create `internal/service/journal_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

// addJournal stores one journal entry as the MCP tool does: the words are
// Gautam's own, so the row is verified.
func addJournal(t *testing.T, s *Service, body, entryDate string) JournalEntry {
	t.Helper()
	entry, err := s.AddJournalEntry(context.Background(), AddJournalEntryParams{
		Body:       body,
		EntryDate:  entryDate,
		Provenance: Provenance{Source: SourceAgent, Verified: true},
	})
	if err != nil {
		t.Fatalf("AddJournalEntry(%q): %v", body, err)
	}
	return entry
}

// The journal holds Gautam's words. The service must not trim, correct or
// shorten them.
func TestAddJournalEntryStoresTheBodyExactlyAsWritten(t *testing.T) {
	s, pool := newTestService(t)
	const body = "  spent the day on LifeOS\n\n- planned the schema\n- no gym :(  \n"

	got, err := s.AddJournalEntry(context.Background(), AddJournalEntryParams{
		Body:       body,
		Provenance: Provenance{Source: SourceAgent, Verified: true},
	})
	if err != nil {
		t.Fatalf("AddJournalEntry: %v", err)
	}

	if got.ID == 0 {
		t.Error("AddJournalEntry returned ID 0")
	}
	if got.Body != body {
		t.Errorf("body = %q, want it unchanged: %q", got.Body, body)
	}
	var stored string
	if err := pool.QueryRow(context.Background(), "SELECT body FROM journal_entries").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != body {
		t.Errorf("stored body = %q, want it unchanged: %q", stored, body)
	}
	if got.Source != SourceAgent || !got.Verified {
		t.Errorf("source, verified = %q, %v", got.Source, got.Verified)
	}
	if got.CreatedAt.IsZero() || got.CreatedAt.Location() != time.UTC {
		t.Errorf("created_at = %v, want a time in UTC", got.CreatedAt)
	}
}

// Between 00:00 and 05:30 IST the UTC date is one day behind. An entry
// with no date is about today in IST.
func TestAddJournalEntryUsesTheISTDateAsToday(t *testing.T) {
	s, _ := newTestService(t) // the clock is 2026-10-10 20:00 UTC = 2026-10-11 01:30 IST
	ctx := context.Background()

	if got := addJournal(t, s, "no date", ""); got.EntryDate != "2026-10-11" {
		t.Errorf("entry_date = %q, want 2026-10-11 (today in IST)", got.EntryDate)
	}
	if got := addJournal(t, s, "about last week", "2026-10-03"); got.EntryDate != "2026-10-03" {
		t.Errorf("entry_date = %q, want 2026-10-03", got.EntryDate)
	}

	_, err := s.AddJournalEntry(ctx, AddJournalEntryParams{
		Body: "about tomorrow", EntryDate: "2026-10-12", Provenance: Provenance{Source: SourceManual, Verified: true},
	})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "entry_date" {
		t.Errorf("entry for tomorrow in IST: error = %v, want a ValidationError on entry_date", err)
	}
}

func TestAddJournalEntryAppendsACreateEvent(t *testing.T) {
	s, pool := newTestService(t)

	entry := addJournal(t, s, "planned the schema", "")

	events := allEvents(t, pool)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Action != "create" || e.RecordType != "journal_entries" || e.RecordID != entry.ID || e.Source != "agent" {
		t.Errorf("event = %s %s %d by %s, want create journal_entries %d by agent", e.Action, e.RecordType, e.RecordID, e.Source, entry.ID)
	}
	if e.Old != nil {
		t.Errorf("old value = %v, want none", e.Old)
	}
	if e.New["body"] != "planned the schema" || e.New["entry_date"] != "2026-10-11" {
		t.Errorf("new value = %v, want the stored entry", e.New)
	}
}

func TestAddJournalEntryRejectsBadInput(t *testing.T) {
	valid := AddJournalEntryParams{Body: "a day", Provenance: Provenance{Source: SourceManual, Verified: true}}
	tests := []struct {
		name      string
		change    func(*AddJournalEntryParams)
		wantField string
	}{
		{"empty body", func(p *AddJournalEntryParams) { p.Body = "" }, "body"},
		{"body of spaces", func(p *AddJournalEntryParams) { p.Body = " \t\n" }, "body"},
		{"body with a NUL character", func(p *AddJournalEntryParams) { p.Body = "a\x00b" }, "body"},
		{"body that is not UTF-8", func(p *AddJournalEntryParams) { p.Body = "a\xffb" }, "body"},
		{"date that is not a date", func(p *AddJournalEntryParams) { p.EntryDate = "yesterday" }, "entry_date"},
		{"date in the wrong order", func(p *AddJournalEntryParams) { p.EntryDate = "11-10-2026" }, "entry_date"},
		{"day that does not exist", func(p *AddJournalEntryParams) { p.EntryDate = "2026-02-30" }, "entry_date"},
		{"unknown source", func(p *AddJournalEntryParams) { p.Source = "robot" }, "source"},
	}
	s, pool := newTestService(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := valid
			tt.change(&p)
			_, err := s.AddJournalEntry(context.Background(), p)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
	if n := countRows(t, pool, "journal_entries"); n != 0 {
		t.Errorf("journal_entries has %d rows after refused input, want 0", n)
	}
	if n := countRows(t, pool, "events"); n != 0 {
		t.Errorf("events has %d rows after refused input, want 0", n)
	}
}

func TestAddJournalEntryStoresNothingWhenTheEventFails(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "DROP TABLE events"); err != nil {
		t.Fatal(err)
	}

	_, err := s.AddJournalEntry(ctx, AddJournalEntryParams{Body: "a day", Provenance: Provenance{Source: SourceManual, Verified: true}})

	if err == nil {
		t.Fatal("AddJournalEntry returned no error although the event insert must fail")
	}
	var invalid *ValidationError
	if errors.As(err, &invalid) {
		t.Errorf("a database failure came back as a ValidationError: %v", err)
	}
	if n := countRows(t, pool, "journal_entries"); n != 0 {
		t.Errorf("journal_entries has %d rows, want 0: the entry must roll back with its event", n)
	}
}

func TestGetJournalEntry(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added := addJournal(t, s, "a day", "2026-10-09")

	got, err := s.GetJournalEntry(ctx, added.ID)
	if err != nil {
		t.Fatalf("GetJournalEntry: %v", err)
	}
	if got != added {
		t.Errorf("GetJournalEntry = %+v, want %+v", got, added)
	}

	if _, err := s.GetJournalEntry(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Errorf("absent entry: error = %v, want ErrNotFound", err)
	}
}

func bodies(entries []JournalEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Body
	}
	return out
}

// The list is by the day that an entry is about, not by the day it was
// typed: an entry written late for last week must not show first.
func TestListJournalEntriesReturnsTheNewestDayFirst(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	addJournal(t, s, "friday", "2026-10-09")
	addJournal(t, s, "sunday morning", "2026-10-11")
	addJournal(t, s, "late entry for last week", "2026-10-03")
	addJournal(t, s, "sunday evening", "2026-10-11")

	all, err := s.ListJournalEntries(ctx, 0)
	if err != nil {
		t.Fatalf("ListJournalEntries: %v", err)
	}
	if want := []string{"sunday evening", "sunday morning", "friday", "late entry for last week"}; !equal(bodies(all), want) {
		t.Errorf("bodies = %v, want %v", bodies(all), want)
	}

	two, err := s.ListJournalEntries(ctx, 2)
	if err != nil {
		t.Fatalf("ListJournalEntries with a limit: %v", err)
	}
	if want := []string{"sunday evening", "sunday morning"}; !equal(bodies(two), want) {
		t.Errorf("bodies with limit 2 = %v, want %v", bodies(two), want)
	}
}

func TestListJournalEntriesReturnsAnEmptyListNotNil(t *testing.T) {
	s, _ := newTestService(t)

	entries, err := s.ListJournalEntries(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListJournalEntries: %v", err)
	}
	if entries == nil || len(entries) != 0 {
		t.Errorf("entries = %#v, want an empty non-nil slice so JSON shows []", entries)
	}
}

func TestListJournalEntriesRejectsABadLimit(t *testing.T) {
	s, _ := newTestService(t)

	for _, limit := range []int{-1, 201} {
		_, err := s.ListJournalEntries(context.Background(), limit)
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != "limit" {
			t.Errorf("limit %d: error = %v, want a ValidationError on limit", limit, err)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run Journal"
```
Expected: the run fails. The output contains `undefined: JournalEntry`.

- [ ] **Step 3: Tell sqlc how to map a date that is not null**

`entry_date` is the first `date` column that is `NOT NULL`. Without an override, sqlc gives it the type `pgtype.Date`.

In `sqlc.yaml`, replace this:

```yaml
          - db_type: date
            nullable: true
            go_type: { import: time, type: Time, pointer: true }
```

With this:

```yaml
          - db_type: date
            go_type: { import: time, type: Time }
          - db_type: date
            nullable: true
            go_type: { import: time, type: Time, pointer: true }
```

- [ ] **Step 4: Write the queries**

Create `queries/journal_entries.sql`:

```sql
-- name: InsertJournalEntry :one
INSERT INTO journal_entries (body, entry_date, source, source_ref, verified)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetJournalEntry :one
SELECT * FROM journal_entries
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListJournalEntries :many
SELECT * FROM journal_entries
WHERE deleted_at IS NULL
ORDER BY entry_date DESC, id DESC
LIMIT $1;
```

- [ ] **Step 5: Write the shared checks**

Later tasks use these functions for todos and habit logs too.

Create `internal/service/fields.go`:

```go
package service

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// checkText refuses text that Postgres cannot store.
func checkText(field, value string) error {
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return invalid(field, "must be valid text without a NUL character")
	}
	return nil
}

// checkLimit returns the number of rows for a list. 0 means 50; the
// maximum is 200.
func checkLimit(limit int) (int32, error) {
	if limit == 0 {
		limit = defaultListLimit
	}
	if limit < 1 || limit > maxListLimit {
		return 0, invalid("limit", fmt.Sprintf("must be from 1 to %d", maxListLimit))
	}
	return int32(limit), nil
}

// today returns the IST date of now. The time is 00:00 UTC, as a date
// column reads.
func today(now time.Time) time.Time {
	y, m, d := now.In(IST).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// parseDay reads a day as YYYY-MM-DD.
func parseDay(field, value string) (time.Time, error) {
	day, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return time.Time{}, invalid(field, "must be a date as YYYY-MM-DD")
	}
	return day, nil
}

// pastDay reads a day as YYYY-MM-DD that is not later than today in IST.
// An empty value means today.
func pastDay(field, value string, now time.Time) (time.Time, error) {
	if value == "" {
		return today(now), nil
	}
	day, err := parseDay(field, value)
	if err != nil {
		return day, err
	}
	if day.After(today(now)) {
		return day, invalid(field, "must not be in the future")
	}
	return day, nil
}
```

- [ ] **Step 6: Write the journal functions**

`toRow` does not trim the body. The check for an empty body uses a trimmed copy.

Create `internal/service/journal.go`:

```go
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// JournalEntry is one row of journal_entries.
type JournalEntry struct {
	ID int64 `json:"id"`
	// Body is the entry exactly as Gautam wrote it.
	Body string `json:"body"`
	// EntryDate is the IST day that the entry is about, as YYYY-MM-DD.
	EntryDate string    `json:"entry_date"`
	Source    Source    `json:"source"`
	SourceRef *int64    `json:"source_ref,omitempty"`
	Verified  bool      `json:"verified"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AddJournalEntryParams is the input of AddJournalEntry.
type AddJournalEntryParams struct {
	// Body is stored as it is. The service does not trim it.
	Body string
	// EntryDate is optional: YYYY-MM-DD, not later than today in IST. An
	// empty value means today in IST.
	EntryDate string
	Provenance
}

// AddJournalEntry validates p, stores one journal entry and appends a
// create event, in one transaction.
func (s *Service) AddJournalEntry(ctx context.Context, p AddJournalEntryParams) (JournalEntry, error) {
	row, err := p.toRow(s.now())
	if err != nil {
		return JournalEntry{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return JournalEntry{}, fmt.Errorf("add journal entry: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	stored, err := q.InsertJournalEntry(ctx, row)
	if err != nil {
		return JournalEntry{}, fmt.Errorf("add journal entry: %w", err)
	}
	entry := journalEntryFromRow(stored)
	if err := recordEvent(ctx, q, "create", "journal_entries", entry.ID, p.Source, nil, entry); err != nil {
		return JournalEntry{}, fmt.Errorf("add journal entry: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return JournalEntry{}, fmt.Errorf("add journal entry: %w", err)
	}
	return entry, nil
}

// toRow checks every field and returns the row to insert.
func (p AddJournalEntryParams) toRow(now time.Time) (store.InsertJournalEntryParams, error) {
	var row store.InsertJournalEntryParams

	if strings.TrimSpace(p.Body) == "" {
		return row, invalid("body", "must not be empty")
	}
	if err := checkText("body", p.Body); err != nil {
		return row, err
	}
	day, err := pastDay("entry_date", p.EntryDate, now)
	if err != nil {
		return row, err
	}
	if err := p.Provenance.validate(); err != nil {
		return row, err
	}
	return store.InsertJournalEntryParams{
		Body:      p.Body,
		EntryDate: day,
		Source:    string(p.Source),
		SourceRef: p.SourceRef,
		Verified:  p.Verified,
	}, nil
}

func journalEntryFromRow(row store.JournalEntry) JournalEntry {
	return JournalEntry{
		ID:        row.ID,
		Body:      row.Body,
		EntryDate: row.EntryDate.Format(time.DateOnly),
		Source:    Source(row.Source),
		SourceRef: row.SourceRef,
		Verified:  row.Verified,
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
}

// GetJournalEntry returns the journal entry with this ID. It returns
// ErrNotFound when there is no such entry or the entry is deleted.
func (s *Service) GetJournalEntry(ctx context.Context, id int64) (JournalEntry, error) {
	row, err := store.New(s.pool).GetJournalEntry(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return JournalEntry{}, ErrNotFound
	}
	if err != nil {
		return JournalEntry{}, fmt.Errorf("get journal entry: %w", err)
	}
	return journalEntryFromRow(row), nil
}

// ListJournalEntries returns the journal entries that are not deleted, the
// newest day first. limit 0 means 50; the maximum is 200.
func (s *Service) ListJournalEntries(ctx context.Context, limit int) ([]JournalEntry, error) {
	maxRows, err := checkLimit(limit)
	if err != nil {
		return nil, err
	}
	rows, err := store.New(s.pool).ListJournalEntries(ctx, maxRows)
	if err != nil {
		return nil, fmt.Errorf("list journal entries: %w", err)
	}
	entries := make([]JournalEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, journalEntryFromRow(row))
	}
	return entries, nil
}
```

- [ ] **Step 7: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./internal/service ARGS="-run Journal"
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/service`.

- [ ] **Step 8: Commit**

Run:
```bash
git add sqlc.yaml queries/journal_entries.sql internal/service/fields.go internal/service/journal.go internal/service/journal_test.go
git commit -m "feat(service): add, read and list journal entries"
```

---

### Task 3: Service: correct and delete a journal entry; the record functions for more than one type

**Files:**
- Create: `internal/service/delete.go`, `internal/service/journal_change.go`
- Modify: `internal/service/items_delete.go` (replace the whole file), `internal/service/records.go` (replace the whole file), `internal/service/review.go` (replace the whole file), `queries/journal_entries.sql` (five more queries), `queries/catalog_items.sql` (one line), `internal/mcp/items_test.go` (one line), `internal/mcp/records_test.go` (two lines)
- Test: `internal/service/journal_change_test.go`, `internal/service/journal_records_test.go`

**Interfaces:**
- Consumes (Task 2): `JournalEntry`, `AddJournalEntryParams` and `toRow`, `journalEntryFromRow`, `checkLimit`, `addJournal`. Consumes (plan 2): `DeleteParams{Actor Source; Confirmed bool}` and its method `check() error`, `DeleteItem`, `UpdateItem`, `UpdateItemParams`, `GetItem`, `itemFromRow`, `Item.summary()`, `RecordRef`, `ReviewEntry`, `UpdateParams{Fields map[string]any; Clear []string; Actor Source; Inferred bool}`, `decodeFields(fields, target, recordType) error`, `checkRecordType`, `maxVerifyRecords`, the test helpers `addItem` and `addUnverified`.
- Produces (package `service`):
  - `(*Service).deleteRecord(ctx, table string, id int64, p DeleteParams, remove func(q *store.Queries) (any, error)) error`. `remove` sets `deleted_at` and returns the row as the record of its type. It returns `pgx.ErrNoRows` when the row is not there.
  - `type UpdateJournalEntryParams struct{ Body, EntryDate *string; Actor Source; Inferred bool }`. The first two fields have the JSON tags `body` and `entry_date`. The last two have the tag `json:"-"`.
  - `(*Service).UpdateJournalEntry(ctx, id int64, p UpdateJournalEntryParams) (JournalEntry, error)`, `(*Service).DeleteJournalEntry(ctx, id int64, p DeleteParams) error`
  - `RecordTypes` is `{"item", "journal"}`. `Get`, `Update` and `Delete` have one `case` for each type. `errNoCode(recordType string) error` and `asRecord[T any](record T, err error) (any, error)`.
  - `ReviewQueue` calls one function for each type. Each has the signature `func(ctx context.Context, q *store.Queries, limit int32) ([]ReviewEntry, error)`: `unverifiedItems`, `unverifiedJournalEntries`.
  - `verifyRecord(ctx, q, ref RecordRef) (table string, before, after any, err error)` has one `case` for each type. Each case calls a function with the signature `func(ctx context.Context, q *store.Queries, id int64) (before, after any, err error)`: `verifyItem`, `verifyJournalEntry`. `before` is nil when the record is verified already.
  - `shorten(text string, max int) string`, `JournalEntry.summary() string`
  - Test helper: `addInferredJournal(t, s, body string) JournalEntry` (an entry that is not verified)
  - sqlc: `GetJournalEntryForUpdate`, `UpdateJournalEntry`, `SoftDeleteJournalEntry`, `ListUnverifiedJournalEntries`, `VerifyJournalEntry`

- [ ] **Step 1: Run the delete tests of plan 2 before the change**

Three more types need the steps of `DeleteItem`. This task moves those steps into one function first. The tests of plan 2 guard the move.

Run:
```bash
make test PKG=./internal/service ARGS="-run Delete"
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/service`.

- [ ] **Step 2: Move the steps of a delete into `deleteRecord`**

Create `internal/service/delete.go`:

```go
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// deleteRecord marks one row of table as deleted and appends a delete
// event that holds the row, in one transaction. remove sets deleted_at and
// returns the row as the record of its type. The row stays in the table; a
// read leaves it out.
func (s *Service) deleteRecord(ctx context.Context, table string, id int64, p DeleteParams, remove func(q *store.Queries) (any, error)) error {
	if err := p.check(); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("delete from %s: %w", table, err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	old, err := remove(q)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete from %s: %w", table, err)
	}
	if err := recordEvent(ctx, q, "delete", table, id, p.Actor, old, nil); err != nil {
		return fmt.Errorf("delete from %s: %w", table, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("delete from %s: %w", table, err)
	}
	return nil
}
```

Replace the whole content of `internal/service/items_delete.go` with this:

```go
package service

import (
	"context"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// DeleteParams is the input of a delete.
type DeleteParams struct {
	// Actor is who deletes the record. It goes into the event.
	Actor Source
	// Confirmed is true when Gautam saw the record and said yes to the
	// delete. An agent must not delete without it.
	Confirmed bool
}

func (p DeleteParams) check() error {
	if err := (Provenance{Source: p.Actor}).validate(); err != nil {
		return err
	}
	if p.Actor == SourceAgent && !p.Confirmed {
		return invalid("confirmed", "must be true: show the record to the user, and delete it only after the user says yes")
	}
	return nil
}

// DeleteItem marks one catalog item as deleted and appends a delete event
// that holds the row, in one transaction. The row stays in the table; a
// read leaves it out.
func (s *Service) DeleteItem(ctx context.Context, id int64, p DeleteParams) error {
	return s.deleteRecord(ctx, "catalog_items", id, p, func(q *store.Queries) (any, error) {
		row, err := q.SoftDeleteCatalogItem(ctx, id)
		if err != nil {
			return nil, err
		}
		return itemFromRow(row)
	})
}
```

- [ ] **Step 3: Run the delete tests again**

Run:
```bash
make test PKG=./internal/service ARGS="-run Delete"
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/service`.

- [ ] **Step 4: Commit**

Run:
```bash
git add internal/service/delete.go internal/service/items_delete.go
git commit -m "refactor(service): share the delete steps between the record types"
```

- [ ] **Step 5: Write the failing tests for update and delete**

Create `internal/service/journal_change_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"
)

func TestUpdateJournalEntryChangesOnlyTheGivenFields(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added := addJournal(t, s, "a day", "2026-10-09")

	got, err := s.UpdateJournalEntry(ctx, added.ID, UpdateJournalEntryParams{EntryDate: ptr("2026-10-08"), Actor: SourceManual})
	if err != nil {
		t.Fatalf("UpdateJournalEntry: %v", err)
	}
	if got.EntryDate != "2026-10-08" || got.Body != "a day" {
		t.Errorf("after a change of the date: %+v", got)
	}

	// The new body is stored as written, the same as a new entry.
	const body = "  a long day\n\nwith two paragraphs \n"
	got, err = s.UpdateJournalEntry(ctx, added.ID, UpdateJournalEntryParams{Body: ptr(body), Actor: SourceManual})
	if err != nil {
		t.Fatalf("UpdateJournalEntry: %v", err)
	}
	if got.Body != body || got.EntryDate != "2026-10-08" {
		t.Errorf("after a change of the body: %+v", got)
	}
	if got.Source != SourceAgent || !got.Verified {
		t.Errorf("source, verified = %q, %v, want them unchanged", got.Source, got.Verified)
	}
	stored, err := s.GetJournalEntry(ctx, added.ID)
	if err != nil || stored != got {
		t.Errorf("stored = %+v, %v, want %+v", stored, err, got)
	}
}

func TestUpdateJournalEntryAppendsAnUpdateEventWithTheOldValue(t *testing.T) {
	s, pool := newTestService(t)
	added := addJournal(t, s, "a day", "2026-10-09")

	if _, err := s.UpdateJournalEntry(context.Background(), added.ID, UpdateJournalEntryParams{Body: ptr("a good day"), Actor: SourceManual}); err != nil {
		t.Fatalf("UpdateJournalEntry: %v", err)
	}

	events := allEvents(t, pool)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (create, update)", len(events))
	}
	e := events[1]
	if e.Action != "update" || e.RecordType != "journal_entries" || e.RecordID != added.ID || e.Source != "manual" {
		t.Errorf("event = %s %s %d by %s, want update journal_entries %d by manual", e.Action, e.RecordType, e.RecordID, e.Source, added.ID)
	}
	if e.Old["body"] != "a day" || e.New["body"] != "a good day" {
		t.Errorf("old, new body = %v, %v", e.Old["body"], e.New["body"])
	}
}

func TestUpdateJournalEntryRejectsBadInput(t *testing.T) {
	s, pool := newTestService(t)
	added := addJournal(t, s, "a day", "2026-10-09")
	tests := []struct {
		name      string
		params    UpdateJournalEntryParams
		wantField string
	}{
		{"no field", UpdateJournalEntryParams{Actor: SourceManual}, "fields"},
		{"empty body", UpdateJournalEntryParams{Body: ptr(" \n"), Actor: SourceManual}, "body"},
		{"body with a NUL character", UpdateJournalEntryParams{Body: ptr("a\x00b"), Actor: SourceManual}, "body"},
		{"empty date", UpdateJournalEntryParams{EntryDate: ptr(""), Actor: SourceManual}, "entry_date"},
		{"date in the future", UpdateJournalEntryParams{EntryDate: ptr("2026-10-12"), Actor: SourceManual}, "entry_date"},
		{"unknown actor", UpdateJournalEntryParams{Body: ptr("x"), Actor: "robot"}, "source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.UpdateJournalEntry(context.Background(), added.ID, tt.params)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want 1 (the create): refused input appended an event", n)
	}
}

func TestUpdateJournalEntryReturnsNotFound(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	deleted := addJournal(t, s, "a day", "")
	if err := s.DeleteJournalEntry(ctx, deleted.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	for name, id := range map[string]int64{"deleted": deleted.ID, "absent": 9999} {
		_, err := s.UpdateJournalEntry(ctx, id, UpdateJournalEntryParams{Body: ptr("x"), Actor: SourceManual})
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", name, err)
		}
	}
}

// The words in the journal are Gautam's own. An agent that rewords an
// entry on its own must be refused; a change that Gautam states goes in.
func TestUpdateJournalEntryRefusesAnInferredChange(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added := addJournal(t, s, "went to the gym, felt gr8", "")

	_, err := s.UpdateJournalEntry(ctx, added.ID, UpdateJournalEntryParams{
		Body: ptr("Went to the gym and felt great."), Actor: SourceAgent, Inferred: true,
	})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "dictated" {
		t.Fatalf("error = %v, want a ValidationError on dictated", err)
	}
	if got, _ := s.GetJournalEntry(ctx, added.ID); got.Body != "went to the gym, felt gr8" {
		t.Errorf("body = %q, want it unchanged", got.Body)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want 1 (the create)", n)
	}

	got, err := s.UpdateJournalEntry(ctx, added.ID, UpdateJournalEntryParams{Body: ptr("went to the gym, felt great"), Actor: SourceAgent})
	if err != nil || got.Body != "went to the gym, felt great" || !got.Verified {
		t.Errorf("dictated change = %+v, %v", got, err)
	}
}

func TestUpdateJournalEntryWritesNothingWhenNoValueChanges(t *testing.T) {
	s, pool := newTestService(t)
	added := addJournal(t, s, "a day", "2026-10-09")

	got, err := s.UpdateJournalEntry(context.Background(), added.ID, UpdateJournalEntryParams{
		Body: ptr("a day"), EntryDate: ptr("2026-10-09"), Actor: SourceAgent, Inferred: true,
	})

	if err != nil || got != added {
		t.Errorf("UpdateJournalEntry = %+v, %v, want the stored entry and no error", got, err)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want 1 (the create)", n)
	}
}

func TestDeleteJournalEntry(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added := addJournal(t, s, "a day", "")

	err := s.DeleteJournalEntry(ctx, added.ID, DeleteParams{Actor: SourceAgent})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "confirmed" {
		t.Fatalf("delete by an agent with no confirmation: error = %v, want a ValidationError on confirmed", err)
	}

	if err := s.DeleteJournalEntry(ctx, added.ID, DeleteParams{Actor: SourceAgent, Confirmed: true}); err != nil {
		t.Fatalf("DeleteJournalEntry: %v", err)
	}
	if _, err := s.GetJournalEntry(ctx, added.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get after delete: error = %v, want ErrNotFound", err)
	}
	if entries, _ := s.ListJournalEntries(ctx, 0); len(entries) != 0 {
		t.Errorf("list after delete = %v, want it empty", entries)
	}
	if n := countRows(t, pool, "journal_entries"); n != 1 {
		t.Errorf("journal_entries has %d rows, want 1: a delete keeps the row", n)
	}
	events := allEvents(t, pool)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (create, delete)", len(events))
	}
	e := events[1]
	if e.Action != "delete" || e.RecordType != "journal_entries" || e.RecordID != added.ID || e.Source != "agent" {
		t.Errorf("event = %s %s %d by %s", e.Action, e.RecordType, e.RecordID, e.Source)
	}
	if e.Old["body"] != "a day" || e.New != nil {
		t.Errorf("old, new = %v, %v, want the entry and none", e.Old, e.New)
	}

	if err := s.DeleteJournalEntry(ctx, added.ID, DeleteParams{Actor: SourceManual}); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: error = %v, want ErrNotFound", err)
	}
	if n := countRows(t, pool, "events"); n != 2 {
		t.Errorf("events = %d after a second delete, want 2", n)
	}
}
```

- [ ] **Step 6: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run Journal"
```
Expected: the run fails. The output contains `s.UpdateJournalEntry undefined`.

- [ ] **Step 7: Write the queries**

Add this at the end of `queries/journal_entries.sql`:

```sql
-- name: GetJournalEntryForUpdate :one
SELECT * FROM journal_entries
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- name: UpdateJournalEntry :one
UPDATE journal_entries
SET body = $2, entry_date = $3
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteJournalEntry :one
UPDATE journal_entries
SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;
```

- [ ] **Step 8: Write the update and the delete**

The changed entry goes through `toRow`, so an update has the same checks as a new entry. The check for "no change" comes before the check for an inferred change, as in `UpdateItem`.

Create `internal/service/journal_change.go`:

```go
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// UpdateJournalEntryParams is the input of UpdateJournalEntry. A field
// that is nil keeps its stored value.
type UpdateJournalEntryParams struct {
	// Body is stored as it is, the same as the body of a new entry.
	Body      *string `json:"body"`
	EntryDate *string `json:"entry_date"`
	// Actor is who makes the change. It goes into the event; the source
	// of the row stays as it is.
	Actor Source `json:"-"`
	// Inferred is true when an agent makes a change that Gautam did not
	// state. The service refuses such a change to a verified row.
	Inferred bool `json:"-"`
}

// UpdateJournalEntry changes the given fields of one journal entry and
// appends an update event that holds the old row, in one transaction. A
// call that changes no value writes nothing and appends no event.
func (s *Service) UpdateJournalEntry(ctx context.Context, id int64, p UpdateJournalEntryParams) (JournalEntry, error) {
	if err := (Provenance{Source: p.Actor}).validate(); err != nil {
		return JournalEntry{}, err
	}
	if p.Body == nil && p.EntryDate == nil {
		return JournalEntry{}, invalid("fields", "must name at least one field to change")
	}
	if p.EntryDate != nil && *p.EntryDate == "" {
		return JournalEntry{}, invalid("entry_date", "must be a date as YYYY-MM-DD")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return JournalEntry{}, fmt.Errorf("update journal entry: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	current, err := q.GetJournalEntryForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return JournalEntry{}, ErrNotFound
	}
	if err != nil {
		return JournalEntry{}, fmt.Errorf("update journal entry: %w", err)
	}
	old := journalEntryFromRow(current)

	// The changed entry goes through the same checks as a new entry.
	next := AddJournalEntryParams{
		Body:       old.Body,
		EntryDate:  old.EntryDate,
		Provenance: Provenance{Source: old.Source, SourceRef: old.SourceRef, Verified: old.Verified},
	}
	if p.Body != nil {
		next.Body = *p.Body
	}
	if p.EntryDate != nil {
		next.EntryDate = *p.EntryDate
	}
	row, err := next.toRow(s.now())
	if err != nil {
		return JournalEntry{}, err
	}
	if next.Body == old.Body && next.EntryDate == old.EntryDate {
		return old, nil
	}
	// The words are Gautam's own. An agent must not reword them.
	if p.Inferred && old.Verified {
		return JournalEntry{}, invalid("dictated", "must be true to change a verified record: ask the user to confirm the change, then send it again")
	}
	stored, err := q.UpdateJournalEntry(ctx, store.UpdateJournalEntryParams{ID: id, Body: row.Body, EntryDate: row.EntryDate})
	if err != nil {
		return JournalEntry{}, fmt.Errorf("update journal entry: %w", err)
	}
	entry := journalEntryFromRow(stored)
	if err := recordEvent(ctx, q, "update", "journal_entries", id, p.Actor, old, entry); err != nil {
		return JournalEntry{}, fmt.Errorf("update journal entry: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return JournalEntry{}, fmt.Errorf("update journal entry: %w", err)
	}
	return entry, nil
}

// DeleteJournalEntry marks one journal entry as deleted and appends a
// delete event that holds the row, in one transaction.
func (s *Service) DeleteJournalEntry(ctx context.Context, id int64, p DeleteParams) error {
	return s.deleteRecord(ctx, "journal_entries", id, p, func(q *store.Queries) (any, error) {
		row, err := q.SoftDeleteJournalEntry(ctx, id)
		if err != nil {
			return nil, err
		}
		return journalEntryFromRow(row), nil
	})
}
```

- [ ] **Step 9: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./internal/service ARGS="-run Journal"
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/service`.

- [ ] **Step 10: Commit**

Run:
```bash
git add queries/journal_entries.sql internal/service/journal_change.go internal/service/journal_change_test.go
git commit -m "feat(service): correct and delete a journal entry"
```

- [ ] **Step 11: Write the failing tests for the record functions and the review**

No adapter writes a journal entry that is not verified. The service accepts such a row, so the review queue and `Verify` must handle it.

Create `internal/service/journal_records_test.go`:

```go
package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRecordFunctionsWorkOnAJournalEntry(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added := addJournal(t, s, "a day", "2026-10-09")

	got, err := s.Get(ctx, "journal", added.ID)
	if entry, ok := got.(JournalEntry); err != nil || !ok || entry != added {
		t.Fatalf("Get = %#v, %v, want the entry", got, err)
	}

	got, err = s.Update(ctx, "journal", added.ID, UpdateParams{Fields: map[string]any{"body": "a good day"}, Actor: SourceManual})
	if entry, ok := got.(JournalEntry); err != nil || !ok || entry.Body != "a good day" || entry.EntryDate != "2026-10-09" {
		t.Fatalf("Update = %#v, %v, want the changed entry", got, err)
	}

	refused := map[string]UpdateParams{
		"verified":   {Fields: map[string]any{"verified": false}, Actor: SourceManual},
		"id":         {Fields: map[string]any{"id": 7}, Actor: SourceManual},
		"title":      {Fields: map[string]any{"title": "a field of an item"}, Actor: SourceManual},
		"body":       {Fields: map[string]any{"body": 5}, Actor: SourceManual},
		"entry_date": {Fields: map[string]any{"entry_date": nil}, Actor: SourceManual},
		"clear":      {Fields: map[string]any{"body": "x"}, Clear: []string{"entry_date"}, Actor: SourceManual},
	}
	for field, p := range refused {
		_, err := s.Update(ctx, "journal", added.ID, p)
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != field {
			t.Errorf("%s: error = %v, want a ValidationError on %s", field, err, field)
		}
	}

	if err := s.Delete(ctx, "journal", added.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "journal", added.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: error = %v, want ErrNotFound", err)
	}
}

// A type in RecordTypes with no code in one entry point is a bug that only
// shows when an agent uses that type. Each entry point must answer "not
// found" for an ID that does not exist, for every type.
func TestEveryRecordTypeHasEveryEntryPoint(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	for _, recordType := range RecordTypes {
		if _, err := s.Get(ctx, recordType, 9999); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get %s: error = %v, want ErrNotFound", recordType, err)
		}
		if err := s.Delete(ctx, recordType, 9999, DeleteParams{Actor: SourceManual}); !errors.Is(err, ErrNotFound) {
			t.Errorf("Delete %s: error = %v, want ErrNotFound", recordType, err)
		}
		_, err := s.Verify(ctx, []RecordRef{{recordType, 9999}}, SourceManual)
		var invalid *ValidationError
		if !errors.As(err, &invalid) || !strings.Contains(invalid.Message, recordType+" 9999 does not exist") {
			t.Errorf("Verify %s: error = %v, want it to say that the record does not exist", recordType, err)
		}
		// A field that no type has: each type must refuse it by name.
		_, err = s.Update(ctx, recordType, 9999, UpdateParams{Fields: map[string]any{"verified": true}, Actor: SourceManual})
		if !errors.As(err, &invalid) {
			t.Errorf("Update %s: error = %v, want a ValidationError", recordType, err)
		}
	}
}

// addInferredJournal stores a journal entry that is not verified. No
// adapter writes such a row; the queue and Verify must still handle it.
func addInferredJournal(t *testing.T, s *Service, body string) JournalEntry {
	t.Helper()
	entry, err := s.AddJournalEntry(context.Background(), AddJournalEntryParams{Body: body, Provenance: Provenance{Source: SourceAgent}})
	if err != nil {
		t.Fatalf("AddJournalEntry(%q): %v", body, err)
	}
	return entry
}

func TestReviewQueueListsEveryTypeOldestFirst(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	first := addInferredJournal(t, s, "went to the gym.\n\n"+strings.Repeat("and then more words ", 10))
	second := addUnverified(t, s, "Dune")
	third := addInferredJournal(t, s, "short")
	addJournal(t, s, "verified", "")

	got, err := s.ReviewQueue(ctx, 0)
	if err != nil {
		t.Fatalf("ReviewQueue: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("queue = %+v, want 3 entries", got)
	}
	want := []RecordRef{{"journal", first.ID}, {"item", second.ID}, {"journal", third.ID}}
	for i, w := range want {
		if got[i].Type != w.Type || got[i].ID != w.ID {
			t.Errorf("entry %d = %s %d, want %s %d", i, got[i].Type, got[i].ID, w.Type, w.ID)
		}
	}
	const wantSummary = "journal 2026-10-11: went to the gym. and then more words and then more words and then more words and…"
	if got[0].Summary != wantSummary {
		t.Errorf("summary = %q, want %q", got[0].Summary, wantSummary)
	}
	if got[2].Summary != "journal 2026-10-11: short" {
		t.Errorf("summary = %q", got[2].Summary)
	}

	limited, err := s.ReviewQueue(ctx, 2)
	if err != nil || len(limited) != 2 || limited[1].Type != "item" {
		t.Errorf("queue with limit 2 = %+v, %v, want the two oldest", limited, err)
	}
}

func TestVerifyApprovesAJournalEntry(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	entry := addInferredJournal(t, s, "a day")
	item := addUnverified(t, s, "Dune")

	n, err := s.Verify(ctx, []RecordRef{{"journal", entry.ID}, {"item", item.ID}}, SourceManual)
	if err != nil || n != 2 {
		t.Fatalf("Verify = %d, %v, want 2", n, err)
	}
	if got, _ := s.GetJournalEntry(ctx, entry.ID); !got.Verified {
		t.Error("the journal entry is not verified")
	}
	events := allEvents(t, pool)
	e := events[len(events)-2]
	if e.Action != "verify" || e.RecordType != "journal_entries" || e.RecordID != entry.ID || e.Old["verified"] != false || e.New["verified"] != true {
		t.Errorf("event = %+v, want a verify of the journal entry", e)
	}

	// A second approval changes nothing and appends no event.
	before := countRows(t, pool, "events")
	if n, err := s.Verify(ctx, []RecordRef{{"journal", entry.ID}}, SourceManual); err != nil || n != 0 {
		t.Errorf("second Verify = %d, %v, want 0", n, err)
	}
	if after := countRows(t, pool, "events"); after != before {
		t.Errorf("events = %d, want %d", after, before)
	}
}
```

- [ ] **Step 12: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run 'Journal|EveryRecordType'"
```
Expected: the run fails. The output contains `--- FAIL: TestRecordFunctionsWorkOnAJournalEntry` and `--- FAIL: TestVerifyApprovesAJournalEntry`.

`TestEveryRecordTypeHasEveryEntryPoint` passes now, because `item` is the only type in `RecordTypes`. It is a guard for the later tasks: Task 7 shows it fail when a type is in `RecordTypes` and a function has no `case` for it.

- [ ] **Step 13: Write the queries**

Add this at the end of `queries/journal_entries.sql`:

```sql
-- name: ListUnverifiedJournalEntries :many
SELECT * FROM journal_entries
WHERE deleted_at IS NULL AND NOT verified
ORDER BY created_at, id
LIMIT $1;

-- name: VerifyJournalEntry :one
UPDATE journal_entries
SET verified = true
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;
```

The review queue puts the records of all types in one order, by `created_at`. The query for items must use the same order.

In `queries/catalog_items.sql`, replace this:

```sql
WHERE deleted_at IS NULL AND NOT verified
ORDER BY id
LIMIT $1;
```

With this:

```sql
WHERE deleted_at IS NULL AND NOT verified
ORDER BY created_at, id
LIMIT $1;
```

- [ ] **Step 14: Give the record functions one case for each type**

`TestEveryRecordTypeHasEveryEntryPoint` guards the rule of this file: a type that is in `RecordTypes` has a `case` in each function.

Replace the whole content of `internal/service/records.go` with this:

```go
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// RecordTypes are the record types that Get, Update and Delete accept. A
// caller that knows the type when it is compiled, such as a REST route,
// calls the function of the type (GetItem, UpdateItem, DeleteItem). An MCP
// tool gets the type as input and calls the functions in this file.
var RecordTypes = []string{"item", "journal"}

func checkRecordType(recordType string) error {
	if !slices.Contains(RecordTypes, recordType) {
		return invalid("type", "must be one of: "+strings.Join(RecordTypes, ", "))
	}
	return nil
}

// errNoCode means that a type is in RecordTypes and one of the functions
// that take a type has no case for it.
func errNoCode(recordType string) error {
	return fmt.Errorf("no code for the record type %q", recordType)
}

// asRecord returns nil, not an empty struct, when there is an error.
func asRecord[T any](record T, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return record, nil
}

// Get returns one record. The value is the struct of the type: an Item for
// the type item, a JournalEntry for the type journal.
func (s *Service) Get(ctx context.Context, recordType string, id int64) (any, error) {
	if err := checkRecordType(recordType); err != nil {
		return nil, err
	}
	switch recordType {
	case "item":
		return asRecord(s.GetItem(ctx, id))
	case "journal":
		return asRecord(s.GetJournalEntry(ctx, id))
	}
	return nil, errNoCode(recordType)
}

// UpdateParams is the input of Update.
type UpdateParams struct {
	// Fields holds the fields to change, by their JSON names, with the
	// new values. A field that is not in Fields keeps its value.
	Fields map[string]any
	// Clear names the optional fields to empty.
	Clear []string
	// Actor is who makes the change.
	Actor Source
	// Inferred is true when an agent makes a change that Gautam did not
	// state. Update refuses such a change to a verified record.
	Inferred bool
}

// Update changes one record and returns it. It refuses a field that the
// type does not let a caller change, such as verified or source.
func (s *Service) Update(ctx context.Context, recordType string, id int64, p UpdateParams) (any, error) {
	if err := checkRecordType(recordType); err != nil {
		return nil, err
	}
	switch recordType {
	case "item":
		var params UpdateItemParams
		if err := decodeFields(p.Fields, &params, recordType); err != nil {
			return nil, err
		}
		params.Clear, params.Actor, params.Inferred = p.Clear, p.Actor, p.Inferred
		return asRecord(s.UpdateItem(ctx, id, params))
	case "journal":
		var params UpdateJournalEntryParams
		if err := decodeFields(p.Fields, &params, recordType); err != nil {
			return nil, err
		}
		if len(p.Clear) > 0 {
			return nil, invalid("clear", "must be empty: a journal entry has no optional field")
		}
		params.Actor, params.Inferred = p.Actor, p.Inferred
		return asRecord(s.UpdateJournalEntry(ctx, id, params))
	}
	return nil, errNoCode(recordType)
}

// Delete marks one record as deleted.
func (s *Service) Delete(ctx context.Context, recordType string, id int64, p DeleteParams) error {
	if err := checkRecordType(recordType); err != nil {
		return err
	}
	switch recordType {
	case "item":
		return s.DeleteItem(ctx, id, p)
	case "journal":
		return s.DeleteJournalEntry(ctx, id, p)
	}
	return errNoCode(recordType)
}

// decodeFields copies fields into target. target is a pointer to a struct
// that has a json tag for each field that a caller can change. A name that
// target does not have, a value of the wrong type, or a null gives a
// ValidationError that names the field. A null is refused because a caller
// can mean "empty this field" by it; that is what Clear is for.
func decodeFields(fields map[string]any, target any, recordType string) error {
	for name, value := range fields {
		if value == nil {
			return invalid(name, "must not be null; to empty an optional field, put its name in clear")
		}
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return invalid("fields", "must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(target)
	if err == nil {
		return nil
	}
	var wrongType *json.UnmarshalTypeError
	if errors.As(err, &wrongType) {
		return invalid(wrongType.Field, "must be "+jsonTypeName(wrongType.Type))
	}
	if name, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return invalid(strings.Trim(name, `"`), "is not a field of "+recordType+" that you can change")
	}
	return invalid("fields", "must be a JSON object")
}

func jsonTypeName(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "text"
	case reflect.Float64, reflect.Int64:
		return "a number"
	case reflect.Bool:
		return "true or false"
	case reflect.Map:
		return "an object"
	}
	return "a different type"
}
```

- [ ] **Step 15: Give the review queue and `Verify` one function for each type**

Replace the whole content of `internal/service/review.go` with this:

```go
package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// RecordRef names one record by its type and its ID.
type RecordRef struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
}

// ReviewEntry is one record that waits for Gautam's approval.
type ReviewEntry struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
	// Summary is one line that says what the record holds.
	Summary   string    `json:"summary"`
	Source    Source    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const maxVerifyRecords = 200

// ReviewQueue returns the records of every type that are not verified and
// not deleted, oldest first. limit 0 means 50; the maximum is 200.
func (s *Service) ReviewQueue(ctx context.Context, limit int) ([]ReviewEntry, error) {
	maxRows, err := checkLimit(limit)
	if err != nil {
		return nil, err
	}

	q := store.New(s.pool)
	entries := []ReviewEntry{}
	for _, unverified := range []func(context.Context, *store.Queries, int32) ([]ReviewEntry, error){
		unverifiedItems,
		unverifiedJournalEntries,
	} {
		more, err := unverified(ctx, q, maxRows)
		if err != nil {
			return nil, fmt.Errorf("review queue: %w", err)
		}
		entries = append(entries, more...)
	}
	// Each type gave its oldest records. Put them in one order.
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].CreatedAt.Before(entries[j].CreatedAt) })
	if len(entries) > int(maxRows) {
		entries = entries[:maxRows]
	}
	return entries, nil
}

// unverifiedItems returns the items that wait for review, oldest first.
func unverifiedItems(ctx context.Context, q *store.Queries, limit int32) ([]ReviewEntry, error) {
	rows, err := q.ListUnverifiedCatalogItems(ctx, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]ReviewEntry, 0, len(rows))
	for _, row := range rows {
		item, err := itemFromRow(row)
		if err != nil {
			return nil, err
		}
		entries = append(entries, ReviewEntry{
			Type:      "item",
			ID:        item.ID,
			Summary:   item.summary(),
			Source:    item.Source,
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
		})
	}
	return entries, nil
}

// summary is the one line that the review queue shows for an item.
func (i Item) summary() string {
	s := i.Kind + ": " + i.Title
	if i.Rating != nil {
		s += ", rating " + strconv.FormatFloat(*i.Rating, 'f', -1, 64)
	}
	if i.OccurredOn != "" {
		s += ", on " + i.OccurredOn
	}
	return s
}

// shorten returns text as one line of at most max characters.
func shorten(text string, max int) string {
	line := strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(line) <= max {
		return line
	}
	return strings.TrimSpace(string([]rune(line)[:max])) + "…"
}

// Verify marks the records as verified and appends one verify event for
// each record that it changes, in one transaction. A record that is
// verified already is left as it is. When one record does not exist, Verify
// changes nothing. It returns the number of records that it changed.
func (s *Service) Verify(ctx context.Context, refs []RecordRef, actor Source) (int, error) {
	if err := (Provenance{Source: actor}).validate(); err != nil {
		return 0, err
	}
	if len(refs) == 0 {
		return 0, invalid("records", "must name at least one record")
	}
	if len(refs) > maxVerifyRecords {
		return 0, invalid("records", fmt.Sprintf("must name at most %d records", maxVerifyRecords))
	}
	for _, ref := range refs {
		if !slices.Contains(RecordTypes, ref.Type) {
			return 0, invalid("records", fmt.Sprintf("%q is not a record type; the types are: %s", ref.Type, strings.Join(RecordTypes, ", ")))
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("verify: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	changed := 0
	for _, ref := range refs {
		table, before, after, err := verifyRecord(ctx, q, ref)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, invalid("records", fmt.Sprintf("%s %d does not exist", ref.Type, ref.ID))
		}
		if err != nil {
			return 0, fmt.Errorf("verify: %w", err)
		}
		if before == nil {
			continue
		}
		if err := recordEvent(ctx, q, "verify", table, ref.ID, actor, before, after); err != nil {
			return 0, fmt.Errorf("verify: %w", err)
		}
		changed++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("verify: %w", err)
	}
	return changed, nil
}

// verifyRecord locks one record and sets verified when it is not set. It
// returns the table of the record, and the record before and after the
// change. before is nil when the record is verified already.
func verifyRecord(ctx context.Context, q *store.Queries, ref RecordRef) (table string, before, after any, err error) {
	switch ref.Type {
	case "item":
		before, after, err = verifyItem(ctx, q, ref.ID)
		return "catalog_items", before, after, err
	case "journal":
		before, after, err = verifyJournalEntry(ctx, q, ref.ID)
		return "journal_entries", before, after, err
	}
	return "", nil, nil, errNoCode(ref.Type)
}

func verifyItem(ctx context.Context, q *store.Queries, id int64) (before, after any, err error) {
	current, err := q.GetCatalogItemForUpdate(ctx, id)
	if err != nil || current.Verified {
		return nil, nil, err
	}
	stored, err := q.VerifyCatalogItem(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	old, err := itemFromRow(current)
	if err != nil {
		return nil, nil, err
	}
	item, err := itemFromRow(stored)
	if err != nil {
		return nil, nil, err
	}
	return old, item, nil
}
```

Add this at the end of `internal/service/journal_change.go`:

```go
// summary is the one line that the review queue shows for a journal entry.
func (e JournalEntry) summary() string {
	return "journal " + e.EntryDate + ": " + shorten(e.Body, 80)
}

// unverifiedJournalEntries returns the journal entries that wait for
// review, oldest first.
func unverifiedJournalEntries(ctx context.Context, q *store.Queries, limit int32) ([]ReviewEntry, error) {
	rows, err := q.ListUnverifiedJournalEntries(ctx, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]ReviewEntry, 0, len(rows))
	for _, row := range rows {
		entry := journalEntryFromRow(row)
		entries = append(entries, ReviewEntry{
			Type:      "journal",
			ID:        entry.ID,
			Summary:   entry.summary(),
			Source:    entry.Source,
			CreatedAt: entry.CreatedAt,
			UpdatedAt: entry.UpdatedAt,
		})
	}
	return entries, nil
}

func verifyJournalEntry(ctx context.Context, q *store.Queries, id int64) (before, after any, err error) {
	current, err := q.GetJournalEntryForUpdate(ctx, id)
	if err != nil || current.Verified {
		return nil, nil, err
	}
	stored, err := q.VerifyJournalEntry(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return journalEntryFromRow(current), journalEntryFromRow(stored), nil
}
```

- [ ] **Step 16: Run the service tests to verify that they pass**

Run:
```bash
make test PKG=./internal/service
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/service`.

- [ ] **Step 17: Run all tests and see three MCP test cases fail**

Three test cases of plan 2 use `journal` as a type that does not exist. It exists now.

Run:
```bash
make test
```
Expected: the run fails. The output contains `--- FAIL: TestToolsReturnErrorsTheModelCanRead` and `--- FAIL: TestRecordToolsReturnErrorsTheModelCanRead`.

- [ ] **Step 18: Use a type that does not exist in the three test cases**

In `internal/mcp/items_test.go`, replace this:

```go
		{"unknown type", "get", map[string]any{"type": "journal", "id": 1}, "type"},
```

With this:

```go
		{"unknown type", "get", map[string]any{"type": "song", "id": 1}, "type"},
```

In `internal/mcp/records_test.go`, replace this:

```go
		{"update of an unknown type", "update", map[string]any{"type": "journal", "id": id, "fields": map[string]any{"title": "New"}}, "type"},
```

With this:

```go
		{"update of an unknown type", "update", map[string]any{"type": "song", "id": id, "fields": map[string]any{"title": "New"}}, "type"},
```

In `internal/mcp/records_test.go`, replace this:

```go
		{"delete of an unknown type", "delete", map[string]any{"type": "journal", "id": id, "confirmed": true}, "type"},
```

With this:

```go
		{"delete of an unknown type", "delete", map[string]any{"type": "song", "id": id, "confirmed": true}, "type"},
```

- [ ] **Step 19: Run all tests to verify that they pass**

Run:
```bash
make test
```
Expected: the run passes. The output contains `ok  	github.com/Gautam-J/lifeos-core/internal/mcp`.

The output has `ok` for seven packages.

- [ ] **Step 20: Commit**

Run:
```bash
git add queries/journal_entries.sql queries/catalog_items.sql internal/service/records.go internal/service/review.go internal/service/journal_change.go internal/service/journal_records_test.go internal/mcp/items_test.go internal/mcp/records_test.go
git commit -m "feat(service): add journal entries to the record functions and the review"
```

---


### Task 4: REST API and MCP tool for the journal

**Files:**
- Create: `internal/api/journal.go`, `internal/mcp/journal.go`
- Modify: `internal/api/api.go` (the helper `patch`, the type `idInput`, one more `register` call), `internal/api/items_change.go` (replace the whole file), `internal/mcp/server.go` (one line), `internal/mcp/records.go` (the argument descriptions)
- Test: `internal/api/journal_test.go`, `internal/mcp/journal_test.go`

**Interfaces:**
- Consumes (Tasks 2 and 3): `service.AddJournalEntry`, `GetJournalEntry`, `DeleteJournalEntry`, `service.Update` with the type `journal`, `service.JournalEntry`. Consumes (plans 1 and 2): `toHTTPError(where string, err error) error`, `toToolError(err error) error`, `itemOutput`, `updateItemInput`, `getItemInput`; the test helpers `newTestAPI(t)`, `do(t, h, method, path, body) (int, map[string]any, string)`, `countEvents(t, pool) int` in package `api_test`, and `connect(t)`, `call(t, session, tool, args)`, `text(result)`, `structured(t, result)` in package `mcp_test`.
- Produces:
  - Package `api`: `patch[T any](ctx, svc, recordType string, id int64, raw []byte, clear []string) (T, error)`, which sends the body of a `PATCH` to `service.Update` and returns the record as `T`; `type idInput struct{ ID int64 }` with the tag `path:"id"`; the routes `POST /api/journal` (201), `GET /api/journal/{id}`, `PATCH /api/journal/{id}`, `DELETE /api/journal/{id}` (204)
  - Package `mcp`: the tool `journal` with the arguments `body` and `entry_date`
  - Test helpers: `todayIST() string` and `addJournal(t, h, body string) string` in package `api_test`; `todayIST() string` in package `mcp_test`

- [ ] **Step 1: Run the PATCH tests of plan 2 before the change**

The `PATCH` handler of items reads the raw body, so that the service can refuse a null. Two more types need the same steps. This task moves them into one function first.

Run:
```bash
make test PKG=./internal/api ARGS="-run Patch"
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/api`.

- [ ] **Step 2: Move the steps of a PATCH into `patch`**

In `internal/api/api.go`, replace this:

```go
import (
	"errors"
	"log/slog"
```

With this:

```go
import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
```

Add this at the end of `internal/api/api.go`:

```go
// patch sends the body of a PATCH request to service.Update and returns
// the record after the change. raw is the body as it came: only a map of
// fields can tell a null from a field that is absent, and the service
// refuses a null. T is the struct of the record type.
func patch[T any](ctx context.Context, svc *service.Service, recordType string, id int64, raw []byte, clear []string) (T, error) {
	var zero T
	// Huma checked the names and the types of the body already.
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return zero, huma.Error400BadRequest("the body is not a JSON object")
	}
	delete(fields, "clear")
	// A request to the REST API comes from Gautam's own devices, so the
	// change is his and the verified flag stays as it is.
	record, err := svc.Update(ctx, recordType, id, service.UpdateParams{
		Fields: fields,
		Clear:  clear,
		Actor:  service.SourceManual,
	})
	if err != nil {
		return zero, toHTTPError("body", err)
	}
	return record.(T), nil
}
```

Replace the whole content of `internal/api/items_change.go` with this:

```go
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type updateItemInput struct {
	ID   int64 `path:"id"`
	Body struct {
		Kind       *string        `json:"kind,omitempty" doc:"One of movie, book, place, game"`
		Title      *string        `json:"title,omitempty"`
		Rating     *float64       `json:"rating,omitempty" doc:"0 to 5 in steps of 0.5"`
		OccurredOn *string        `json:"occurred_on,omitempty" doc:"Day watched, read, visited or played, as YYYY-MM-DD"`
		Attrs      map[string]any `json:"attrs,omitempty" doc:"Replaces every attr of the item"`
		Clear      []string       `json:"clear,omitempty" doc:"Optional fields to empty: rating, occurred_on"`
	}
	// RawBody is the body as it came. Body cannot tell a null from a
	// field that is absent, and the service refuses a null.
	RawBody []byte
}

func registerItemChanges(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "update-item",
		Method:      http.MethodPatch,
		Path:        "/api/items/{id}",
		Summary:     "Correct a list item",
		Description: "Changes the fields in the body. A field that is not in the body keeps its value.",
	}, func(ctx context.Context, in *updateItemInput) (*itemOutput, error) {
		item, err := patch[service.Item](ctx, svc, "item", in.ID, in.RawBody, in.Body.Clear)
		if err != nil {
			return nil, err
		}
		return &itemOutput{Body: item}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-item",
		Method:        http.MethodDelete,
		Path:          "/api/items/{id}",
		Summary:       "Delete a list item",
		Description:   "Marks the item as deleted. The row stays in the database.",
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *getItemInput) (*struct{}, error) {
		if err := svc.DeleteItem(ctx, in.ID, service.DeleteParams{Actor: service.SourceManual}); err != nil {
			return nil, toHTTPError("path", err)
		}
		return nil, nil
	})
}
```

- [ ] **Step 3: Run the API tests again**

Run:
```bash
make test PKG=./internal/api
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/api`.

- [ ] **Step 4: Commit**

Run:
```bash
git add internal/api/api.go internal/api/items_change.go
git commit -m "refactor(api): share the PATCH steps between the record types"
```

- [ ] **Step 5: Write the failing tests for the routes**

The tests of the adapters use the real clock. `todayIST` gives the date that the service uses for an entry with no date.

Create `internal/api/journal_test.go`:

```go
package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// todayIST is today's date for Gautam, as the service computes it.
func todayIST() string {
	return time.Now().In(service.IST).Format(time.DateOnly)
}

// addJournal stores one journal entry through the API and returns its path.
func addJournal(t *testing.T, h http.Handler, body string) string {
	t.Helper()
	status, added, raw := do(t, h, "POST", "/api/journal", body)
	if status != http.StatusCreated {
		t.Fatalf("POST /api/journal: status = %d, want 201\n%s", status, raw)
	}
	return fmt.Sprintf("/api/journal/%d", int64(added["id"].(float64)))
}

func TestAddJournalEntryThenReadIt(t *testing.T) {
	h, _ := newTestAPI(t)

	status, added, raw := do(t, h, "POST", "/api/journal", `{"body":"  planned the schema\n\nno gym :(  \n"}`)

	if status != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201\n%s", status, raw)
	}
	if added["body"] != "  planned the schema\n\nno gym :(  \n" {
		t.Errorf("body = %q, want it exactly as it was sent", added["body"])
	}
	if added["entry_date"] != todayIST() {
		t.Errorf("entry_date = %v, want today in IST (%s)", added["entry_date"], todayIST())
	}
	if added["source"] != "manual" || added["verified"] != true {
		t.Errorf("source, verified = %v, %v, want manual, true", added["source"], added["verified"])
	}

	path := fmt.Sprintf("/api/journal/%d", int64(added["id"].(float64)))
	status, got, raw := do(t, h, "GET", path, "")
	if status != http.StatusOK || got["body"] != added["body"] || got["entry_date"] != added["entry_date"] {
		t.Errorf("GET: status = %d, entry = %s", status, raw)
	}
}

func TestAddJournalEntryRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantInBody string
	}{
		{"body of spaces", `{"body":"  "}`, 422, "body.body"},
		{"missing body", `{"entry_date":"2026-01-01"}`, 422, "body"},
		{"date in the future", `{"body":"x","entry_date":"2999-01-01"}`, 422, "body.entry_date"},
		{"date that is not a date", `{"body":"x","entry_date":"yesterday"}`, 422, "body.entry_date"},
		{"field that a caller cannot set", `{"body":"x","verified":false}`, 422, "verified"},
		{"body of the wrong type", `{"body":5}`, 422, "body"},
		{"request that is not JSON", `body=x`, 400, ""},
	}
	h, pool := newTestAPI(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, raw := do(t, h, "POST", "/api/journal", tt.body)

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d\n%s", status, tt.wantStatus, raw)
			}
			if !strings.Contains(raw, tt.wantInBody) {
				t.Errorf("body does not name %q:\n%s", tt.wantInBody, raw)
			}
		})
	}
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM journal_entries").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("journal_entries has %d rows after rejected requests, want 0", n)
	}
}

func TestPatchJournalEntry(t *testing.T) {
	h, pool := newTestAPI(t)
	path := addJournal(t, h, `{"body":"a day","entry_date":"2026-01-05"}`)

	status, patched, raw := do(t, h, "PATCH", path, `{"entry_date":"2026-01-04"}`)
	if status != http.StatusOK || patched["entry_date"] != "2026-01-04" || patched["body"] != "a day" {
		t.Fatalf("PATCH of the date: status = %d\n%s", status, raw)
	}

	refused := []struct{ name, body, wantInBody string }{
		{"null", `{"body":null}`, "body.body"},
		{"empty object", `{}`, "body.fields"},
		{"empty body text", `{"body":" "}`, "body.body"},
		{"field that a caller cannot change", `{"verified":false}`, "verified"},
		{"clear", `{"body":"x","clear":["entry_date"]}`, "clear"},
	}
	for _, tt := range refused {
		status, _, raw := do(t, h, "PATCH", path, tt.body)
		if status != http.StatusUnprocessableEntity || !strings.Contains(raw, tt.wantInBody) {
			t.Errorf("%s: status = %d, want 422 that names %q\n%s", tt.name, status, tt.wantInBody, raw)
		}
	}
	if status, _, raw := do(t, h, "PATCH", "/api/journal/9999", `{"body":"x"}`); status != http.StatusNotFound {
		t.Errorf("unknown ID: status = %d, want 404\n%s", status, raw)
	}

	_, got, raw := do(t, h, "GET", path, "")
	if got["body"] != "a day" || got["entry_date"] != "2026-01-04" || got["verified"] != true {
		t.Errorf("entry after the refused requests = %s", raw)
	}
	if n := countEvents(t, pool); n != 2 {
		t.Errorf("events = %d, want 2 (create, one update)", n)
	}
}

func TestDeleteJournalEntry(t *testing.T) {
	h, pool := newTestAPI(t)
	path := addJournal(t, h, `{"body":"a day"}`)

	status, _, raw := do(t, h, "DELETE", path, "")

	if status != http.StatusNoContent || raw != "" {
		t.Fatalf("DELETE: status = %d, body = %q; want 204 and no body", status, raw)
	}
	if status, _, _ := do(t, h, "GET", path, ""); status != http.StatusNotFound {
		t.Errorf("GET after DELETE: status = %d, want 404", status)
	}
	for _, target := range []string{path, "/api/journal/9999"} {
		if status, _, raw := do(t, h, "DELETE", target, ""); status != http.StatusNotFound {
			t.Errorf("DELETE %s: status = %d, want 404\n%s", target, status, raw)
		}
	}
	if status, _, raw := do(t, h, "GET", "/api/journal/abc", ""); status != http.StatusUnprocessableEntity {
		t.Errorf("ID that is not a number: status = %d, want 422\n%s", status, raw)
	}
	if n := countEvents(t, pool); n != 2 {
		t.Errorf("events = %d, want 2 (create, one delete)", n)
	}
}

func TestDatabaseFailureOnAJournalRouteGivesAPlain500(t *testing.T) {
	h, pool := newTestAPI(t)
	pool.Close()

	requests := []struct{ method, path, body string }{
		{"POST", "/api/journal", `{"body":"a day"}`},
		{"GET", "/api/journal/1", ""},
		{"PATCH", "/api/journal/1", `{"body":"x"}`},
		{"DELETE", "/api/journal/1", ""},
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
```

- [ ] **Step 6: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/api ARGS="-run Journal"
```
Expected: the run fails. The output contains `POST /api/journal: body is not a JSON object`.

The router answers `404 page not found`, which is not JSON.

- [ ] **Step 7: Write the routes**

Create `internal/api/journal.go`:

```go
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type addJournalInput struct {
	Body struct {
		Body      string `json:"body" doc:"The entry. It is stored exactly as it is written."`
		EntryDate string `json:"entry_date,omitempty" doc:"The day that the entry is about, as YYYY-MM-DD. The default is today in India Standard Time."`
	}
}

type journalOutput struct {
	Body service.JournalEntry
}

type updateJournalInput struct {
	ID   int64 `path:"id"`
	Body struct {
		Body      *string `json:"body,omitempty" doc:"The entry. It is stored exactly as it is written."`
		EntryDate *string `json:"entry_date,omitempty" doc:"The day that the entry is about, as YYYY-MM-DD"`
	}
	// RawBody is the body as it came. Body cannot tell a null from a
	// field that is absent, and the service refuses a null.
	RawBody []byte
}

func registerJournal(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{
		OperationID:   "add-journal-entry",
		Method:        http.MethodPost,
		Path:          "/api/journal",
		Summary:       "Write a journal entry",
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *addJournalInput) (*journalOutput, error) {
		// A request to the REST API comes from Gautam's own devices, so
		// the row is manual and verified.
		entry, err := svc.AddJournalEntry(ctx, service.AddJournalEntryParams{
			Body:       in.Body.Body,
			EntryDate:  in.Body.EntryDate,
			Provenance: service.Provenance{Source: service.SourceManual, Verified: true},
		})
		if err != nil {
			return nil, toHTTPError("body", err)
		}
		return &journalOutput{Body: entry}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-journal-entry",
		Method:      http.MethodGet,
		Path:        "/api/journal/{id}",
		Summary:     "Read one journal entry",
	}, func(ctx context.Context, in *idInput) (*journalOutput, error) {
		entry, err := svc.GetJournalEntry(ctx, in.ID)
		if err != nil {
			return nil, toHTTPError("path", err)
		}
		return &journalOutput{Body: entry}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-journal-entry",
		Method:      http.MethodPatch,
		Path:        "/api/journal/{id}",
		Summary:     "Correct a journal entry",
		Description: "Changes the fields in the body. A field that is not in the body keeps its value.",
	}, func(ctx context.Context, in *updateJournalInput) (*journalOutput, error) {
		entry, err := patch[service.JournalEntry](ctx, svc, "journal", in.ID, in.RawBody, nil)
		if err != nil {
			return nil, err
		}
		return &journalOutput{Body: entry}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-journal-entry",
		Method:        http.MethodDelete,
		Path:          "/api/journal/{id}",
		Summary:       "Delete a journal entry",
		Description:   "Marks the entry as deleted. The row stays in the database.",
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *idInput) (*struct{}, error) {
		if err := svc.DeleteJournalEntry(ctx, in.ID, service.DeleteParams{Actor: service.SourceManual}); err != nil {
			return nil, toHTTPError("path", err)
		}
		return nil, nil
	})
}
```

In `internal/api/api.go`, replace this:

```go
	registerReview(api, svc)
```

With this:

```go
	registerReview(api, svc)
	registerJournal(api, svc)
```

Add this at the end of `internal/api/api.go`:

```go
// idInput is the input of a route that has only the record ID in its path.
type idInput struct {
	ID int64 `path:"id"`
}
```

- [ ] **Step 8: Run the API tests to verify that they pass**

Run:
```bash
make test PKG=./internal/api
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/api`.

- [ ] **Step 9: Commit**

Run:
```bash
git add internal/api/api.go internal/api/journal.go internal/api/journal_test.go
git commit -m "feat(api): add the journal routes"
```

- [ ] **Step 10: Write the failing tests for the tool**

Create `internal/mcp/journal_test.go`:

```go
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
// came, and the row is verified, because no agent infers a journal entry.
func TestJournalStoresTheWordsOfTheUser(t *testing.T) {
	session, pool := connect(t)
	const body = "spent the entire day planning   a second brain\n\nno gym :(\n"

	added := structured(t, call(t, session, "journal", map[string]any{"body": body}))

	if added["body"] != body {
		t.Errorf("body = %q, want it unchanged: %q", added["body"], body)
	}
	if added["source"] != "agent" || added["verified"] != true {
		t.Errorf("source, verified = %v, %v, want agent, true", added["source"], added["verified"])
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

// An agent that "cleans up" an entry on its own must be refused. A change
// that the user states goes in.
func TestUpdateCannotRewordAJournalEntryUnlessTheUserSaysSo(t *testing.T) {
	session, _ := connect(t)
	added := structured(t, call(t, session, "journal", map[string]any{"body": "felt gr8", "entry_date": "2026-01-05"}))

	refused := call(t, session, "update", map[string]any{
		"type": "journal", "id": added["id"], "fields": map[string]any{"body": "Felt great."},
	})
	if !refused.IsError || !strings.Contains(text(refused), "dictated") {
		t.Fatalf("update with no dictated: IsError = %v, text = %q; want an error that names dictated", refused.IsError, text(refused))
	}

	updated := structured(t, call(t, session, "update", map[string]any{
		"type": "journal", "id": added["id"], "fields": map[string]any{"entry_date": "2026-01-04"}, "dictated": true,
	}))
	if updated["body"] != "felt gr8" || updated["entry_date"] != "2026-01-04" || updated["verified"] != true {
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
		// The tool has no dictated argument: a journal entry is never inferred.
		{"dictated", map[string]any{"body": "x", "dictated": false}, "dictated"},
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
```

- [ ] **Step 11: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/mcp ARGS="-run Journal"
```
Expected: the run fails. The output contains `--- FAIL: TestJournalStoresTheWordsOfTheUser` and `tool journal is not registered`.

- [ ] **Step 12: Write the tool**

The description tells the model that an entry holds the user's own words. The tool has no `dictated` argument.

Create `internal/mcp/journal.go`:

```go
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type journalInput struct {
	Body      string `json:"body" jsonschema:"The entry, word for word as the user wrote it. Do not correct, shorten, summarize or translate it."`
	EntryDate string `json:"entry_date,omitempty" jsonschema:"The day that the entry is about, as YYYY-MM-DD in India Standard Time. Leave it out for today."`
}

func registerJournal(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "journal",
		Description: "Store a journal entry. Use it only for text that the user gives you for their journal. An entry holds the user's own words: never write an entry yourself, and never put your analysis in it. Returns the stored entry with its ID.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in journalInput) (*sdk.CallToolResult, service.JournalEntry, error) {
		// The words are the user's own, so the row is verified.
		entry, err := svc.AddJournalEntry(ctx, service.AddJournalEntryParams{
			Body:       in.Body,
			EntryDate:  in.EntryDate,
			Provenance: service.Provenance{Source: service.SourceAgent, Verified: true},
		})
		if err != nil {
			return nil, service.JournalEntry{}, toToolError(err)
		}
		return nil, entry, nil
	})
}
```

In `internal/mcp/server.go`, replace this:

```go
	registerRecords(server, svc)
```

With this:

```go
	registerRecords(server, svc)
	registerJournal(server, svc)
```

- [ ] **Step 13: Tell the model about the new type in `get`, `update` and `delete`**

In `internal/mcp/records.go`, replace each occurrence of this (three places):

```go
Record type. The only type is: item
```

With this:

```go
Record type. One of: item, journal
```

In `internal/mcp/records.go`, replace this:

```go
For an item: kind, title, rating, occurred_on, attrs. Leave out a field
```

With this:

```go
For an item: kind, title, rating, occurred_on, attrs. For a journal: body, entry_date. Leave out a field
```

In `internal/mcp/records.go`, replace this:

```go
For an item: rating, occurred_on"`
```

With this:

```go
For an item: rating, occurred_on. A journal has none."`
```

- [ ] **Step 14: Run the MCP tests to verify that they pass**

Run:
```bash
make test PKG=./internal/mcp
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/mcp`.

- [ ] **Step 15: Commit**

Run:
```bash
git add internal/mcp/journal.go internal/mcp/journal_test.go internal/mcp/server.go internal/mcp/records.go
git commit -m "feat(mcp): add the journal tool"
```

---

### Task 5: Admin page for the journal

**Files:**
- Create: `internal/admin/journal.templ`, `internal/admin/journal.go`
- Modify: `internal/admin/items_edit.go` (a new name for one function), `internal/admin/admin.go` (five routes), `internal/admin/items.templ` (one link, one style rule), `internal/admin/review.templ` (the Edit link)
- Test: `internal/admin/journal_test.go`

**Interfaces:**
- Consumes (Tasks 2 and 3): `service.AddJournalEntry`, `GetJournalEntry`, `ListJournalEntries`, `UpdateJournalEntry`, `DeleteJournalEntry`, `service.JournalEntry`, `service.UpdateJournalEntryParams`. Consumes (plans 1 and 2, package `admin`): `handlers` with the field `svc`, `render(w, r, c)`, `listFailure(w, r, err, message)`, `internalError(w, err)`, the templ components `page(title)` and `reviewList`, `formatVerified(bool) string`, `itemPath(id int64) string`, `itemID(r) (int64, bool)`; the test helpers `newTestAdmin(t)`, `get(h, path)`, `postForm(h, path, form)`, `addStored(t, pool, title, verified) service.Item` in package `admin_test`.
- Produces (package `admin`):
  - `pathID(r *http.Request) (id int64, ok bool)`: the new name of `itemID`
  - `formText(r *http.Request, name string) string`: a form field with CR LF changed to LF
  - The routes `GET /admin/journal`, `POST /admin/journal`, `GET /admin/journal/{id}/edit`, `POST /admin/journal/{id}`, `POST /admin/journal/{id}/delete`
  - `journalPath(id int64) string`; the style rule `td.text`, which keeps spaces and line breaks
  - The review page has a `switch` on the record type for its link. Tasks 9 and 13 add one `case` each.
  - Test helpers in package `admin_test`: `todayIST() string`, `addEntry(t, pool, body, entryDate string) service.JournalEntry`, `countEvents(t, pool) int`

- [ ] **Step 1: Give `itemID` a name that fits every type**

The function reads the `{id}` part of a path. The pages of this plan use it too.

In `internal/admin/items_edit.go`, replace each occurrence of this (five places):

```go
itemID
```

With this:

```go
pathID
```

- [ ] **Step 2: Run the admin tests, then commit**

Run:
```bash
make test PKG=./internal/admin
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/admin`.

Run:
```bash
git add internal/admin/items_edit.go
git commit -m "refactor(admin): rename itemID to pathID"
```

- [ ] **Step 3: Write the failing tests**

Create `internal/admin/journal_test.go`:

```go
package admin_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// todayIST is today's date for Gautam, as the service computes it.
func todayIST() string {
	return time.Now().In(service.IST).Format(time.DateOnly)
}

// addEntry stores one journal entry through the service and returns it.
func addEntry(t *testing.T, pool *pgxpool.Pool, body, entryDate string) service.JournalEntry {
	t.Helper()
	entry, err := service.New(pool).AddJournalEntry(t.Context(), service.AddJournalEntryParams{
		Body:       body,
		EntryDate:  entryDate,
		Provenance: service.Provenance{Source: service.SourceAgent, Verified: true},
	})
	if err != nil {
		t.Fatalf("add %q: %v", body, err)
	}
	return entry
}

func countEvents(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM events").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestJournalPageShowsTheFormAndAnEmptyList(t *testing.T) {
	h, _ := newTestAdmin(t)

	rec := get(h, "/admin/journal")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`hx-post="/admin/journal"`, `<textarea name="body"`, `name="entry_date"`, "No entries yet.", `href="/admin/journal"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %q", want)
		}
	}
}

// A browser sends each line break of a textarea as CR LF. The stored
// entry must have the line breaks that Gautam typed, not two characters
// for each. All other characters stay as they are.
func TestAddJournalEntryStoresTheTextOfTheForm(t *testing.T) {
	h, pool := newTestAdmin(t)

	rec := postForm(h, "/admin/journal", url.Values{"body": {"  planned the schema\r\n\r\nno gym :(  \r\n"}, "entry_date": {""}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("HX-Trigger"); got != "entry-added" {
		t.Errorf("HX-Trigger = %q, want entry-added so that the form resets", got)
	}
	page := rec.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(page), `<div id="list">`) {
		t.Errorf("the response is not the #list fragment:\n%s", page)
	}
	for _, want := range []string{"planned the schema", todayIST(), "manual", "yes"} {
		if !strings.Contains(page, want) {
			t.Errorf("the list does not contain %q", want)
		}
	}

	var body, source string
	var verified bool
	if err := pool.QueryRow(t.Context(), "SELECT body, source, verified FROM journal_entries").Scan(&body, &source, &verified); err != nil {
		t.Fatalf("read the row (want exactly one): %v", err)
	}
	if body != "  planned the schema\n\nno gym :(  \n" {
		t.Errorf("stored body = %q", body)
	}
	if source != "manual" || !verified {
		t.Errorf("source, verified = %q, %v, want manual, true", source, verified)
	}
}

func TestAddJournalEntryShowsWhyTheServiceRefusedTheInput(t *testing.T) {
	tests := []struct {
		name     string
		form     url.Values
		wantText string
	}{
		{"no text", url.Values{"body": {" \r\n"}}, "body: must not be empty"},
		{"date in the future", url.Values{"body": {"a day"}, "entry_date": {"2999-01-01"}}, "entry_date: must not be in the future"},
	}
	h, pool := newTestAdmin(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postForm(h, "/admin/journal", tt.form)

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200: htmx does not show a 4xx response", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), tt.wantText) {
				t.Errorf("the response does not show %q:\n%s", tt.wantText, rec.Body)
			}
			if got := rec.Header().Get("HX-Trigger"); got != "" {
				t.Errorf("HX-Trigger = %q, want none so that the form keeps its input", got)
			}
		})
	}
	if n := countEvents(t, pool); n != 0 {
		t.Errorf("events = %d after refused input, want 0", n)
	}
}

// The entry is text. The page must show it as text, and the edit form must
// give back each character, or a click on Save changes the entry.
func TestJournalPagesShowTheEntryAsText(t *testing.T) {
	h, pool := newTestAdmin(t)
	entry := addEntry(t, pool, "  a <b>bold</b> day\n\n</textarea><script>alert(1)</script> \n", "2026-01-05")

	list := get(h, "/admin/journal").Body.String()
	edit := get(h, fmt.Sprintf("/admin/journal/%d/edit", entry.ID))

	if edit.Code != http.StatusOK {
		t.Fatalf("edit page: status = %d, want 200", edit.Code)
	}
	const escaped = "  a &lt;b&gt;bold&lt;/b&gt; day\n\n&lt;/textarea&gt;&lt;script&gt;alert(1)&lt;/script&gt; \n"
	if !strings.Contains(list, `<td class="text">`+escaped+`</td>`) {
		t.Errorf("the list does not show the entry as text with its spaces and line breaks:\n%s", list)
	}
	for _, want := range []string{
		fmt.Sprintf(`action="/admin/journal/%d"`, entry.ID),
		// The line break after the start tag is the one that a browser drops.
		`rows="12">` + "\n" + escaped + `</textarea>`,
		`value="2026-01-05"`,
		fmt.Sprintf(`href="/admin/journal/%d/edit"`, entry.ID),
	} {
		if !strings.Contains(list+edit.Body.String(), want) {
			t.Errorf("the pages do not contain %q", want)
		}
	}
	if strings.Contains(list+edit.Body.String(), "<script>alert(") {
		t.Error("a page contains the entry as raw HTML")
	}
}

func TestSaveJournalEntryChangesTheEntryAndGoesBackToTheList(t *testing.T) {
	h, pool := newTestAdmin(t)
	entry := addEntry(t, pool, "a day", "2026-01-05")

	rec := postForm(h, fmt.Sprintf("/admin/journal/%d", entry.ID), url.Values{"body": {"a good day\r\nwith two lines"}, "entry_date": {"2026-01-04"}})

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/journal" {
		t.Fatalf("status = %d, Location = %q; want 303 to /admin/journal\n%s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	got, err := service.New(pool).GetJournalEntry(t.Context(), entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "a good day\nwith two lines" || got.EntryDate != "2026-01-04" {
		t.Errorf("entry = %+v", got)
	}
	if got.Source != service.SourceAgent || !got.Verified {
		t.Errorf("source, verified = %q, %v; want no change", got.Source, got.Verified)
	}
}

// The browser sends CR LF for the line breaks of the stored text. A click
// on Save with no change must not count as a change.
func TestSaveJournalEntryWithNoChangeWritesNoEvent(t *testing.T) {
	h, pool := newTestAdmin(t)
	entry := addEntry(t, pool, "line one\nline two\n", "2026-01-05")

	rec := postForm(h, fmt.Sprintf("/admin/journal/%d", entry.ID), url.Values{"body": {"line one\r\nline two\r\n"}, "entry_date": {"2026-01-05"}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303\n%s", rec.Code, rec.Body)
	}
	if n := countEvents(t, pool); n != 1 {
		t.Errorf("events = %d, want 1 (the create)", n)
	}
}

func TestSaveJournalEntryShowsWhyTheServiceRefusedTheInput(t *testing.T) {
	h, pool := newTestAdmin(t)
	entry := addEntry(t, pool, "a day", "2026-01-05")

	rec := postForm(h, fmt.Sprintf("/admin/journal/%d", entry.ID), url.Values{"body": {"typed text"}, "entry_date": {"2999-01-01"}})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 with the form", rec.Code)
	}
	page := rec.Body.String()
	if !strings.Contains(page, "entry_date: must not be in the future") {
		t.Errorf("the page does not show the reason:\n%s", page)
	}
	if !strings.Contains(page, ">\ntyped text</textarea>") || !strings.Contains(page, `value="2999-01-01"`) {
		t.Error("the form lost the typed input")
	}
	if got, _ := service.New(pool).GetJournalEntry(t.Context(), entry.ID); got.Body != "a day" {
		t.Errorf("refused input changed the entry: %+v", got)
	}
}

func TestJournalPagesReturn404ForAnEntryThatIsNotThere(t *testing.T) {
	h, _ := newTestAdmin(t)

	for _, path := range []string{"/admin/journal/9999/edit", "/admin/journal/abc/edit"} {
		if rec := get(h, path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, rec.Code)
		}
	}
	for _, path := range []string{"/admin/journal/9999", "/admin/journal/abc"} {
		if rec := postForm(h, path, url.Values{"body": {"x"}, "entry_date": {"2026-01-05"}}); rec.Code != http.StatusNotFound {
			t.Errorf("POST %s: status = %d, want 404", path, rec.Code)
		}
	}
}

func TestDeleteJournalEntryRemovesTheEntryFromTheList(t *testing.T) {
	h, pool := newTestAdmin(t)
	addEntry(t, pool, "kept entry", "2026-01-05")
	gone := addEntry(t, pool, "gone entry", "2026-01-06")
	path := fmt.Sprintf("/admin/journal/%d/delete", gone.ID)

	rec := postForm(h, path, nil)

	page := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(page), `<div id="list">`) {
		t.Fatalf("status = %d, want 200 with the #list fragment\n%s", rec.Code, page)
	}
	if strings.Contains(page, "gone entry") || !strings.Contains(page, "kept entry") {
		t.Errorf("the list must show the kept entry and not the deleted one:\n%s", page)
	}
	if _, err := service.New(pool).GetJournalEntry(t.Context(), gone.ID); err != service.ErrNotFound {
		t.Errorf("get after delete: error = %v, want ErrNotFound", err)
	}

	// A second click, for example from a page that was open in two tabs.
	again := postForm(h, path, nil)
	if again.Code != http.StatusOK || !strings.Contains(again.Body.String(), fmt.Sprintf("Entry %d is not in the list", gone.ID)) {
		t.Errorf("second delete: status = %d, want 200 with a message\n%s", again.Code, again.Body)
	}
	if n := countEvents(t, pool); n != 3 {
		t.Errorf("events = %d, want 3 (two creates, one delete)", n)
	}
}

func TestDatabaseFailureOnTheJournalPages(t *testing.T) {
	h, pool := newTestAdmin(t)
	pool.Close()

	if rec := get(h, "/admin/journal"); rec.Code != http.StatusInternalServerError || strings.TrimSpace(rec.Body.String()) != "internal error" {
		t.Errorf("page: status = %d, body = %q; want 500 and only \"internal error\"", rec.Code, rec.Body)
	}
	clicks := map[string]url.Values{
		"/admin/journal":          {"body": {"a day"}},
		"/admin/journal/1/delete": nil,
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

// The review queue has records of more than one type. The Edit link of a
// journal entry must go to the journal, not to the item with the same ID.
func TestReviewPageLinksAJournalEntryToItsEditPage(t *testing.T) {
	h, pool := newTestAdmin(t)
	item := addStored(t, pool, "Dune", false)
	entry, err := service.New(pool).AddJournalEntry(t.Context(), service.AddJournalEntryParams{
		Body: "an entry that is not verified", Provenance: service.Provenance{Source: service.SourceAgent},
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.ID != item.ID {
		t.Fatalf("the test needs one ID for the two records; got %d and %d", entry.ID, item.ID)
	}

	body := get(h, "/admin/review").Body.String()

	for _, want := range []string{
		fmt.Sprintf(`href="/admin/items/%d/edit"`, item.ID),
		fmt.Sprintf(`href="/admin/journal/%d/edit"`, entry.ID),
		`name="type" value="journal"`,
		"journal " + todayIST() + ": an entry that is not verified",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %s", want)
		}
	}

	rec := postForm(h, "/admin/review/verify", url.Values{"type": {"journal"}, "id": {fmt.Sprint(entry.ID)}})
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "an entry that is not verified") || !strings.Contains(rec.Body.String(), "Dune") {
		t.Errorf("after the approval the queue must show the item and not the entry:\n%s", rec.Body)
	}
}
```

- [ ] **Step 4: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/admin ARGS="-run Journal"
```
Expected: the run fails. The output contains `--- FAIL: TestJournalPageShowsTheFormAndAnEmptyList` and `--- FAIL: TestReviewPageLinksAJournalEntryToItsEditPage`.

- [ ] **Step 5: Write the templates**

The table cell and the text area hold the entry with no space or line break added by the template. The text area of the edit form starts with one line break, because a browser drops one line break after the start tag.

Create `internal/admin/journal.templ`:

```templ
package admin

import (
	"strconv"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

templ journalPage(entries []service.JournalEntry) {
	@page("Journal") {
		<h1>Journal</h1>
		<form hx-post="/admin/journal" hx-target="#list" hx-swap="outerHTML" hx-on:entry-added="this.reset()">
			<label>
				Entry
				<textarea name="body" required cols="60" rows="6"></textarea>
			</label>
			<label>
				Day (empty means today)
				<input type="date" name="entry_date"/>
			</label>
			<button type="submit">Add</button>
		</form>
		@journalList(entries, "")
	}
}

// journalList is the part of the page that a form post replaces.
templ journalList(entries []service.JournalEntry, errorMessage string) {
	<div id="list">
		if errorMessage != "" {
			<p class="error" role="alert">{ errorMessage }</p>
		}
		if len(entries) == 0 {
			<p class="muted">No entries yet.</p>
		} else {
			<table>
				<thead>
					<tr>
						<th>ID</th>
						<th>Day</th>
						<th>Entry</th>
						<th>Source</th>
						<th>Verified</th>
						<th>Written (IST)</th>
						<th></th>
					</tr>
				</thead>
				<tbody>
					for _, entry := range entries {
						<tr>
							<td>{ strconv.FormatInt(entry.ID, 10) }</td>
							<td>{ entry.EntryDate }</td>
							<td class="text">{ entry.Body }</td>
							<td>{ string(entry.Source) }</td>
							<td>{ formatVerified(entry.Verified) }</td>
							<td>{ entry.CreatedAt.In(service.IST).Format("2 Jan 2006 15:04") }</td>
							<td>
								<a href={ templ.URL(journalPath(entry.ID) + "/edit") }>Edit</a>
								<button
									hx-post={ journalPath(entry.ID) + "/delete" }
									hx-target="#list"
									hx-swap="outerHTML"
									hx-confirm={ "Delete the entry of " + entry.EntryDate + "?" }
								>Delete</button>
							</td>
						</tr>
					}
				</tbody>
			</table>
		}
	</div>
}

// journalForm holds the values of the edit form as text. When the service
// refuses a value, the form shows again what Gautam typed.
type journalForm struct {
	ID        int64
	Body      string
	EntryDate string
}

// The text area starts with a line break, because a browser drops one line
// break after the start tag. Without it, an entry that starts with a line
// break loses that line break on Save.
templ editJournalPage(form journalForm, errorMessage string) {
	@page("Edit journal entry") {
		<h1>Edit journal entry { strconv.FormatInt(form.ID, 10) }</h1>
		if errorMessage != "" {
			<p class="error" role="alert">{ errorMessage }</p>
		}
		<form method="post" action={ templ.URL(journalPath(form.ID)) }>
			<label>
				Entry
				<textarea name="body" required cols="60" rows="12">{ "\n" + form.Body }</textarea>
			</label>
			<label>
				Day
				<input type="date" name="entry_date" required value={ form.EntryDate }/>
			</label>
			<button type="submit">Save</button>
			<a href="/admin/journal">Cancel</a>
		</form>
	}
}

func journalPath(id int64) string {
	return "/admin/journal/" + strconv.FormatInt(id, 10)
}
```

- [ ] **Step 6: Write the handlers**

Create `internal/admin/journal.go`:

```go
package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// formText reads a text field that has more than one line. A browser sends
// each line break of a textarea as CR LF; the text that Gautam typed has
// LF. Without this, a click on Save with no change is a change.
func formText(r *http.Request, name string) string {
	return strings.ReplaceAll(r.PostForm.Get(name), "\r\n", "\n")
}

func (h *handlers) journalPage(w http.ResponseWriter, r *http.Request) {
	entries, err := h.svc.ListJournalEntries(r.Context(), 0)
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, journalPage(entries))
}

// showJournalList renders the list again, with message above it.
func (h *handlers) showJournalList(w http.ResponseWriter, r *http.Request, message string) {
	entries, err := h.svc.ListJournalEntries(r.Context(), 0)
	if err != nil {
		listFailure(w, r, err, "The list did not load: internal error. Reload the page.")
		return
	}
	render(w, r, journalList(entries, message))
}

// addJournalEntry stores the entry from the form and renders the list
// again. An entry from the admin pages is one that Gautam typed, so it is
// manual and verified.
func (h *handlers) addJournalEntry(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	message := ""
	_, err := h.svc.AddJournalEntry(r.Context(), service.AddJournalEntryParams{
		Body:       formText(r, "body"),
		EntryDate:  r.PostForm.Get("entry_date"),
		Provenance: service.Provenance{Source: service.SourceManual, Verified: true},
	})
	var invalid *service.ValidationError
	switch {
	case err == nil:
		// htmx turns this header into an event; the form resets on it.
		w.Header().Set("HX-Trigger", "entry-added")
	case errors.As(err, &invalid):
		message = invalid.Error()
	default:
		listFailure(w, r, err, "The entry was not stored: internal error. Try again.")
		return
	}
	h.showJournalList(w, r, message)
}

func (h *handlers) editJournalPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	entry, err := h.svc.GetJournalEntry(r.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, editJournalPage(journalForm{ID: entry.ID, Body: entry.Body, EntryDate: entry.EntryDate}, ""))
}

// saveJournalEntry stores the edit form and goes back to the list. When
// the service refuses a value, the form shows again with the reason.
func (h *handlers) saveJournalEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	form := journalForm{ID: id, Body: formText(r, "body"), EntryDate: r.PostForm.Get("entry_date")}
	_, err := h.svc.UpdateJournalEntry(r.Context(), id, service.UpdateJournalEntryParams{
		Body:      &form.Body,
		EntryDate: &form.EntryDate,
		Actor:     service.SourceManual,
	})
	var invalid *service.ValidationError
	switch {
	case err == nil:
		http.Redirect(w, r, "/admin/journal", http.StatusSeeOther)
	case errors.As(err, &invalid):
		render(w, r, editJournalPage(form, invalid.Error()))
	case errors.Is(err, service.ErrNotFound):
		http.NotFound(w, r)
	default:
		internalError(w, err)
	}
}

// deleteJournalEntry deletes one entry and renders the list again.
func (h *handlers) deleteJournalEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	message := ""
	err := h.svc.DeleteJournalEntry(r.Context(), id, service.DeleteParams{Actor: service.SourceManual})
	switch {
	case err == nil:
	case errors.Is(err, service.ErrNotFound):
		message = fmt.Sprintf("Entry %d is not in the list. It was possibly deleted before.", id)
	default:
		listFailure(w, r, err, "The entry was not deleted: internal error. Try again.")
		return
	}
	h.showJournalList(w, r, message)
}
```

- [ ] **Step 7: Add the routes, the link and the style rule**

In `internal/admin/admin.go`, replace this:

```go
	router.Get("/admin/review", h.reviewPage)
```

With this:

```go
	router.Get("/admin/journal", h.journalPage)
	router.Post("/admin/journal", h.addJournalEntry)
	router.Get("/admin/journal/{id}/edit", h.editJournalPage)
	router.Post("/admin/journal/{id}", h.saveJournalEntry)
	router.Post("/admin/journal/{id}/delete", h.deleteJournalEntry)
	router.Get("/admin/review", h.reviewPage)
```

In `internal/admin/items.templ`, replace this:

```templ
				<a href="/admin/items">Items</a>
```

With this:

```templ
				<a href="/admin/items">Items</a>
				<a href="/admin/journal">Journal</a>
```

In `internal/admin/items.templ`, replace this:

```templ
				.error { color: #b00020; }
```

With this:

```templ
				td.text { white-space: pre-wrap; }
				.error { color: #b00020; }
```

- [ ] **Step 8: Make the Edit link of the review page go to the page of the type**

The review queue has records of more than one type now. The link went to the item page for every record.

In `internal/admin/review.templ`, replace this:

```templ
								<a href={ templ.URL(itemPath(entry.ID) + "/edit") }>Edit</a>
```

With this:

```templ
								switch entry.Type {
									case "item":
										<a href={ templ.URL(itemPath(entry.ID) + "/edit") }>Edit</a>
									case "journal":
										<a href={ templ.URL(journalPath(entry.ID) + "/edit") }>Edit</a>
								}
```

- [ ] **Step 9: Run the admin tests to verify that they pass**

Run:
```bash
make test PKG=./internal/admin
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/admin`.

- [ ] **Step 10: Commit**

Run:
```bash
git add internal/admin/journal.templ internal/admin/journal.go internal/admin/journal_test.go internal/admin/admin.go internal/admin/items.templ internal/admin/review.templ
git commit -m "feat(admin): add the journal page"
```

---


### Task 6: Service: add and read a todo

**Files:**
- Create: `queries/todos.sql`, `internal/service/todos.go`
- Test: `internal/service/todos_test.go`

**Interfaces:**
- Consumes (Task 2): `checkText`, `checkLimit`, `parseDay`. Consumes (plans 1 and 2): `maxTitleLength` (300), `Provenance`, `invalid`, `recordEvent`, `ErrNotFound`, the test helpers `newTestService`, `countRows`, `equal`, `allEvents`.
- Produces (package `service`):
  - `var TodoStatuses = []string{"open", "done", "dropped"}`
  - `type Todo struct{ ID int64; Title, Status, DueOn, Project string; DoneAt *time.Time; Source Source; SourceRef *int64; Verified bool; CreatedAt, UpdatedAt time.Time }` with the JSON names `id`, `title`, `status`, `due_on`, `project`, `done_at`, `source`, `source_ref`, `verified`, `created_at`, `updated_at`. `due_on`, `project` and `done_at` are left out of the JSON when they are empty.
  - `type AddTodoParams struct{ Title, DueOn, Project string; Provenance }`
  - `(*Service).AddTodo(ctx, AddTodoParams) (Todo, error)`, `(*Service).GetTodo(ctx, id int64) (Todo, error)`, `type ListTodosParams struct{ Status string; Limit int }`, `(*Service).ListTodos(ctx, ListTodosParams) ([]Todo, error)`
  - `type todoValues struct{ title, status string; dueOn *time.Time; project *string }`, `checkTodo(title, status, dueOn, project string) (todoValues, error)`, `todoFromRow(store.Todo) Todo`
  - Test helpers: `buyDomain() AddTodoParams` (a verified todo with a date and a project), `addTodo(t, s, title string) Todo`, `todoTitles([]Todo) []string`, `setStatus(t, s, id int64, status string)`
  - sqlc: `InsertTodo`, `GetTodo`, `ListTodos`; the type `store.Todo`

- [ ] **Step 1: Write the failing tests**

Create `internal/service/todos_test.go`:

```go
package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// buyDomain is a valid input. Tests copy it and change one field.
func buyDomain() AddTodoParams {
	return AddTodoParams{
		Title:      "Buy a Cloudflare domain",
		DueOn:      "2026-10-20",
		Project:    "lifeos",
		Provenance: Provenance{Source: SourceAgent, Verified: true},
	}
}

func addTodo(t *testing.T, s *Service, title string) Todo {
	t.Helper()
	p := buyDomain()
	p.Title = title
	todo, err := s.AddTodo(context.Background(), p)
	if err != nil {
		t.Fatalf("AddTodo(%q): %v", title, err)
	}
	return todo
}

func TestAddTodoStoresAnOpenTodo(t *testing.T) {
	s, pool := newTestService(t)

	got, err := s.AddTodo(context.Background(), buyDomain())
	if err != nil {
		t.Fatalf("AddTodo: %v", err)
	}

	if got.ID == 0 {
		t.Error("AddTodo returned ID 0")
	}
	if got.Title != "Buy a Cloudflare domain" || got.Status != "open" || got.DueOn != "2026-10-20" || got.Project != "lifeos" {
		t.Errorf("todo = %+v", got)
	}
	if got.DoneAt != nil {
		t.Errorf("done_at = %v, want none for an open todo", got.DoneAt)
	}
	if got.Source != SourceAgent || !got.Verified {
		t.Errorf("source, verified = %q, %v", got.Source, got.Verified)
	}
	events := allEvents(t, pool)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Action != "create" || e.RecordType != "todos" || e.RecordID != got.ID || e.Source != "agent" || e.Old != nil {
		t.Errorf("event = %+v, want a create of the todo by agent", e)
	}
	if e.New["title"] != "Buy a Cloudflare domain" || e.New["status"] != "open" {
		t.Errorf("new value = %v", e.New)
	}
}

// "todo - buy cloudflare domain" has no date and can have no project. A
// date in the past is valid too: it is a todo that is late.
func TestAddTodoAcceptsTheMinimumInputAndAnyDay(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	got, err := s.AddTodo(ctx, AddTodoParams{Title: "  set up the tunnel  ", Provenance: Provenance{Source: SourceAgent}})
	if err != nil {
		t.Fatalf("AddTodo: %v", err)
	}
	if got.Title != "set up the tunnel" || got.DueOn != "" || got.Project != "" || got.Verified {
		t.Errorf("todo = %+v, want the trimmed title, no date, no project, not verified", got)
	}

	for _, day := range []string{"2026-01-01", "2027-06-30"} {
		p := buyDomain()
		p.DueOn = day
		if got, err := s.AddTodo(ctx, p); err != nil || got.DueOn != day {
			t.Errorf("due_on %s: todo = %+v, error = %v", day, got, err)
		}
	}
}

func TestAddTodoRejectsBadInput(t *testing.T) {
	tests := []struct {
		name      string
		change    func(*AddTodoParams)
		wantField string
	}{
		{"empty title", func(p *AddTodoParams) { p.Title = "" }, "title"},
		{"title of spaces", func(p *AddTodoParams) { p.Title = " \t\n" }, "title"},
		{"title too long", func(p *AddTodoParams) { p.Title = strings.Repeat("a", 301) }, "title"},
		{"title with a NUL character", func(p *AddTodoParams) { p.Title = "a\x00b" }, "title"},
		{"date that is not a date", func(p *AddTodoParams) { p.DueOn = "next week" }, "due_on"},
		{"day that does not exist", func(p *AddTodoParams) { p.DueOn = "2026-02-30" }, "due_on"},
		{"project too long", func(p *AddTodoParams) { p.Project = strings.Repeat("a", 101) }, "project"},
		{"project with a NUL character", func(p *AddTodoParams) { p.Project = "a\x00b" }, "project"},
		{"unknown source", func(p *AddTodoParams) { p.Source = "robot" }, "source"},
	}
	s, pool := newTestService(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := buyDomain()
			tt.change(&p)
			_, err := s.AddTodo(context.Background(), p)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
	if n := countRows(t, pool, "todos"); n != 0 {
		t.Errorf("todos has %d rows after refused input, want 0", n)
	}
}

func TestAddTodoStoresNothingWhenTheEventFails(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "DROP TABLE events"); err != nil {
		t.Fatal(err)
	}

	_, err := s.AddTodo(ctx, buyDomain())

	var invalid *ValidationError
	if err == nil || errors.As(err, &invalid) {
		t.Fatalf("error = %v, want a database failure", err)
	}
	if n := countRows(t, pool, "todos"); n != 0 {
		t.Errorf("todos has %d rows, want 0: the todo must roll back with its event", n)
	}
}

func TestGetTodo(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added := addTodo(t, s, "Buy a domain")

	got, err := s.GetTodo(ctx, added.ID)
	if err != nil || got != added {
		t.Errorf("GetTodo = %+v, %v, want %+v", got, err, added)
	}
	if _, err := s.GetTodo(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Errorf("absent todo: error = %v, want ErrNotFound", err)
	}
}

func todoTitles(todos []Todo) []string {
	out := make([]string, len(todos))
	for i, todo := range todos {
		out[i] = todo.Title
	}
	return out
}

// setStatus changes the status of a todo with SQL, because this file is
// about the reads. UpdateTodo has its own tests.
func setStatus(t *testing.T, s *Service, id int64, status string) {
	t.Helper()
	if _, err := s.pool.Exec(context.Background(), "UPDATE todos SET status = $2 WHERE id = $1", id, status); err != nil {
		t.Fatal(err)
	}
}

// The open todos are what Gautam looks for. They come first, so that the
// first page of a long list does not hide them behind todos that are done.
func TestListTodosReturnsTheOpenTodosFirst(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	addTodo(t, s, "first open")
	done := addTodo(t, s, "done")
	dropped := addTodo(t, s, "dropped")
	addTodo(t, s, "second open")
	setStatus(t, s, done.ID, "done")
	setStatus(t, s, dropped.ID, "dropped")

	all, err := s.ListTodos(ctx, ListTodosParams{})
	if err != nil {
		t.Fatalf("ListTodos: %v", err)
	}
	if want := []string{"second open", "first open", "dropped", "done"}; !equal(todoTitles(all), want) {
		t.Errorf("titles = %v, want %v", todoTitles(all), want)
	}

	open, err := s.ListTodos(ctx, ListTodosParams{Status: "open", Limit: 1})
	if err != nil {
		t.Fatalf("ListTodos: %v", err)
	}
	if want := []string{"second open"}; !equal(todoTitles(open), want) {
		t.Errorf("open titles with limit 1 = %v, want %v", todoTitles(open), want)
	}

	finished, err := s.ListTodos(ctx, ListTodosParams{Status: "done"})
	if err != nil || !equal(todoTitles(finished), []string{"done"}) {
		t.Errorf("done titles = %v, %v, want [done]", todoTitles(finished), err)
	}
}

func TestListTodosReturnsAnEmptyListNotNil(t *testing.T) {
	s, _ := newTestService(t)

	todos, err := s.ListTodos(context.Background(), ListTodosParams{})
	if err != nil {
		t.Fatalf("ListTodos: %v", err)
	}
	if todos == nil || len(todos) != 0 {
		t.Errorf("todos = %#v, want an empty non-nil slice so JSON shows []", todos)
	}
}

func TestListTodosRejectsBadInput(t *testing.T) {
	s, _ := newTestService(t)
	tests := []struct {
		name      string
		params    ListTodosParams
		wantField string
	}{
		{"unknown status", ListTodosParams{Status: "later"}, "status"},
		{"limit above the maximum", ListTodosParams{Limit: 201}, "limit"},
		{"negative limit", ListTodosParams{Limit: -1}, "limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.ListTodos(context.Background(), tt.params)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run Todo"
```
Expected: the run fails. The output contains `undefined: AddTodoParams`.

- [ ] **Step 3: Write the queries**

`ListTodos` puts the open todos first: `(status = 'open') DESC` sorts `true` before `false`.

Create `queries/todos.sql`:

```sql
-- name: InsertTodo :one
INSERT INTO todos (title, due_on, project, source, source_ref, verified)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetTodo :one
SELECT * FROM todos
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListTodos :many
SELECT * FROM todos
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY (status = 'open') DESC, id DESC
LIMIT sqlc.arg('max_rows');
```

- [ ] **Step 4: Write the todo functions**

`checkTodo` holds the checks that a new todo and a changed todo share. Task 7 calls it from `UpdateTodo`.

Create `internal/service/todos.go`:

```go
package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// TodoStatuses are the states of a todo. A new todo is open.
var TodoStatuses = []string{"open", "done", "dropped"}

const maxProjectLength = 100

// Todo is one row of todos.
type Todo struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	// Status is open, done or dropped.
	Status string `json:"status"`
	// DueOn is the day that the todo is due, as YYYY-MM-DD.
	DueOn   string `json:"due_on,omitempty"`
	Project string `json:"project,omitempty"`
	// DoneAt is the moment that the status became done.
	DoneAt    *time.Time `json:"done_at,omitempty"`
	Source    Source     `json:"source"`
	SourceRef *int64     `json:"source_ref,omitempty"`
	Verified  bool       `json:"verified"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// AddTodoParams is the input of AddTodo.
type AddTodoParams struct {
	Title string
	// DueOn is optional: YYYY-MM-DD. A day in the past is valid.
	DueOn string
	// Project is optional: free text, such as lifeos.
	Project string
	Provenance
}

// todoValues are the fields of a todo that a caller can set, after the
// checks.
type todoValues struct {
	title   string
	status  string
	dueOn   *time.Time
	project *string
}

// checkTodo checks the fields that a caller can set. An empty dueOn or
// project means that the todo has none.
func checkTodo(title, status, dueOn, project string) (todoValues, error) {
	v := todoValues{title: strings.TrimSpace(title), status: status}

	if v.title == "" {
		return v, invalid("title", "must not be empty")
	}
	if utf8.RuneCountInString(v.title) > maxTitleLength {
		return v, invalid("title", fmt.Sprintf("must be at most %d characters", maxTitleLength))
	}
	if err := checkText("title", v.title); err != nil {
		return v, err
	}
	if !slices.Contains(TodoStatuses, status) {
		return v, invalid("status", "must be one of "+strings.Join(TodoStatuses, ", "))
	}
	if dueOn != "" {
		day, err := parseDay("due_on", dueOn)
		if err != nil {
			return v, err
		}
		v.dueOn = &day
	}
	if project = strings.TrimSpace(project); project != "" {
		if utf8.RuneCountInString(project) > maxProjectLength {
			return v, invalid("project", fmt.Sprintf("must be at most %d characters", maxProjectLength))
		}
		if err := checkText("project", project); err != nil {
			return v, err
		}
		v.project = &project
	}
	return v, nil
}

// AddTodo validates p, stores one open todo and appends a create event, in
// one transaction.
func (s *Service) AddTodo(ctx context.Context, p AddTodoParams) (Todo, error) {
	v, err := checkTodo(p.Title, "open", p.DueOn, p.Project)
	if err != nil {
		return Todo{}, err
	}
	if err := p.Provenance.validate(); err != nil {
		return Todo{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Todo{}, fmt.Errorf("add todo: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	stored, err := q.InsertTodo(ctx, store.InsertTodoParams{
		Title:     v.title,
		DueOn:     v.dueOn,
		Project:   v.project,
		Source:    string(p.Source),
		SourceRef: p.SourceRef,
		Verified:  p.Verified,
	})
	if err != nil {
		return Todo{}, fmt.Errorf("add todo: %w", err)
	}
	todo := todoFromRow(stored)
	if err := recordEvent(ctx, q, "create", "todos", todo.ID, p.Source, nil, todo); err != nil {
		return Todo{}, fmt.Errorf("add todo: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Todo{}, fmt.Errorf("add todo: %w", err)
	}
	return todo, nil
}

func todoFromRow(row store.Todo) Todo {
	todo := Todo{
		ID:        row.ID,
		Title:     row.Title,
		Status:    row.Status,
		Source:    Source(row.Source),
		SourceRef: row.SourceRef,
		Verified:  row.Verified,
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
	if row.DueOn != nil {
		todo.DueOn = row.DueOn.Format(time.DateOnly)
	}
	if row.Project != nil {
		todo.Project = *row.Project
	}
	if row.DoneAt != nil {
		doneAt := row.DoneAt.UTC()
		todo.DoneAt = &doneAt
	}
	return todo
}

// GetTodo returns the todo with this ID. It returns ErrNotFound when there
// is no such todo or the todo is deleted.
func (s *Service) GetTodo(ctx context.Context, id int64) (Todo, error) {
	row, err := store.New(s.pool).GetTodo(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Todo{}, ErrNotFound
	}
	if err != nil {
		return Todo{}, fmt.Errorf("get todo: %w", err)
	}
	return todoFromRow(row), nil
}

// ListTodosParams is the input of ListTodos.
type ListTodosParams struct {
	// Status is optional: an empty string lists every status.
	Status string
	// Limit is optional: 0 means 50. The maximum is 200.
	Limit int
}

// ListTodos returns the todos that are not deleted. The open todos come
// first; in each group the newest comes first.
func (s *Service) ListTodos(ctx context.Context, p ListTodosParams) ([]Todo, error) {
	maxRows, err := checkLimit(p.Limit)
	if err != nil {
		return nil, err
	}
	arg := store.ListTodosParams{MaxRows: maxRows}
	if p.Status != "" {
		if !slices.Contains(TodoStatuses, p.Status) {
			return nil, invalid("status", "must be one of "+strings.Join(TodoStatuses, ", "))
		}
		arg.Status = &p.Status
	}

	rows, err := store.New(s.pool).ListTodos(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("list todos: %w", err)
	}
	todos := make([]Todo, 0, len(rows))
	for _, row := range rows {
		todos = append(todos, todoFromRow(row))
	}
	return todos, nil
}
```

- [ ] **Step 5: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./internal/service ARGS="-run Todo"
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/service`.

- [ ] **Step 6: Commit**

Run:
```bash
git add queries/todos.sql internal/service/todos.go internal/service/todos_test.go
git commit -m "feat(service): add, read and list todos"
```

---

### Task 7: Service: change and delete a todo; todos in the record functions and the review

**Files:**
- Create: `internal/service/todos_change.go`
- Modify: `queries/todos.sql` (five more queries), `internal/service/records.go` (one case in each function), `internal/service/review.go` (two lines and one case)
- Test: `internal/service/todos_change_test.go`, `internal/service/todos_records_test.go`

**Interfaces:**
- Consumes (Task 6): `Todo`, `checkTodo`, `todoValues`, `todoFromRow`, `AddTodoParams`, `buyDomain`, `addTodo`. Consumes (Task 3): `deleteRecord`, `asRecord`, `decodeFields`, `RecordTypes`, the list of functions in `ReviewQueue`, `verifyRecord`, `addUnverified`.
- Produces (package `service`):
  - `type UpdateTodoParams struct{ Title, Status, DueOn, Project *string; Clear []string; Actor Source; Inferred bool }`. The first four fields have the JSON tags `title`, `status`, `due_on`, `project`. The last three have the tag `json:"-"`. `Clear` can hold `due_on` and `project`.
  - `(*Service).UpdateTodo(ctx, id int64, p UpdateTodoParams) (Todo, error)`, `(*Service).DeleteTodo(ctx, id int64, p DeleteParams) error`
  - `RecordTypes` is `{"item", "journal", "todo"}`; `Todo.summary() string`, `unverifiedTodos`, `verifyTodo`
  - Test helper: `addProposedTodo(t, s, p AddTodoParams) Todo` (a todo that is not verified)
  - sqlc: `GetTodoForUpdate`, `UpdateTodo`, `SoftDeleteTodo`, `ListUnverifiedTodos`, `VerifyTodo`

- [ ] **Step 1: Write the failing tests for update and delete**

Create `internal/service/todos_change_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUpdateTodoChangesOnlyTheGivenFields(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added := addTodo(t, s, "Buy a domain")

	got, err := s.UpdateTodo(ctx, added.ID, UpdateTodoParams{DueOn: ptr("2026-11-01"), Actor: SourceManual})
	if err != nil {
		t.Fatalf("UpdateTodo: %v", err)
	}
	if got.DueOn != "2026-11-01" || got.Title != "Buy a domain" || got.Project != "lifeos" || got.Status != "open" {
		t.Errorf("after a change of the date: %+v", got)
	}

	got, err = s.UpdateTodo(ctx, added.ID, UpdateTodoParams{Title: ptr("  Buy two domains "), Project: ptr("home"), Actor: SourceManual})
	if err != nil {
		t.Fatalf("UpdateTodo: %v", err)
	}
	if got.Title != "Buy two domains" || got.Project != "home" || got.DueOn != "2026-11-01" {
		t.Errorf("after a change of the title and the project: %+v", got)
	}
	if got.Source != SourceAgent || !got.Verified {
		t.Errorf("source, verified = %q, %v, want them unchanged", got.Source, got.Verified)
	}
	if stored, err := s.GetTodo(ctx, added.ID); err != nil || stored != got {
		t.Errorf("stored = %+v, %v, want %+v", stored, err, got)
	}
}

// done_at is not a field that a caller sets. It follows the status.
func TestUpdateTodoSetsDoneAtWhenTheStatusBecomesDone(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added := addTodo(t, s, "Buy a domain")
	update := func(p UpdateTodoParams) Todo {
		t.Helper()
		p.Actor = SourceManual
		todo, err := s.UpdateTodo(ctx, added.ID, p)
		if err != nil {
			t.Fatalf("UpdateTodo: %v", err)
		}
		return todo
	}

	done := update(UpdateTodoParams{Status: ptr("done")})
	if done.Status != "done" || done.DoneAt == nil || !done.DoneAt.Equal(testNow) {
		t.Fatalf("status, done_at = %q, %v, want done at %v", done.Status, done.DoneAt, testNow)
	}

	// A change of a different field of a done todo keeps the moment.
	s.now = func() time.Time { return testNow.Add(time.Hour) }
	renamed := update(UpdateTodoParams{Title: ptr("Buy the domain")})
	if renamed.DoneAt == nil || !renamed.DoneAt.Equal(testNow) {
		t.Errorf("done_at = %v after a change of the title, want %v", renamed.DoneAt, testNow)
	}

	if reopened := update(UpdateTodoParams{Status: ptr("open")}); reopened.DoneAt != nil {
		t.Errorf("done_at = %v after the todo is open again, want none", reopened.DoneAt)
	}
	if dropped := update(UpdateTodoParams{Status: ptr("dropped")}); dropped.Status != "dropped" || dropped.DoneAt != nil {
		t.Errorf("status, done_at = %q, %v, want dropped and none", dropped.Status, dropped.DoneAt)
	}
}

func TestUpdateTodoClearsAnOptionalField(t *testing.T) {
	s, _ := newTestService(t)
	added := addTodo(t, s, "Buy a domain")

	got, err := s.UpdateTodo(context.Background(), added.ID, UpdateTodoParams{Clear: []string{"due_on", "project"}, Actor: SourceManual})

	if err != nil {
		t.Fatalf("UpdateTodo: %v", err)
	}
	if got.DueOn != "" || got.Project != "" || got.Title != "Buy a domain" {
		t.Errorf("todo = %+v, want no date and no project", got)
	}
}

func TestUpdateTodoAppendsAnUpdateEventWithTheOldValue(t *testing.T) {
	s, pool := newTestService(t)
	added := addTodo(t, s, "Buy a domain")

	if _, err := s.UpdateTodo(context.Background(), added.ID, UpdateTodoParams{Status: ptr("done"), Actor: SourceManual}); err != nil {
		t.Fatalf("UpdateTodo: %v", err)
	}

	events := allEvents(t, pool)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (create, update)", len(events))
	}
	e := events[1]
	if e.Action != "update" || e.RecordType != "todos" || e.RecordID != added.ID || e.Source != "manual" {
		t.Errorf("event = %s %s %d by %s", e.Action, e.RecordType, e.RecordID, e.Source)
	}
	if e.Old["status"] != "open" || e.New["status"] != "done" || e.New["done_at"] == nil {
		t.Errorf("old, new = %v, %v", e.Old, e.New)
	}
}

func TestUpdateTodoRejectsBadInput(t *testing.T) {
	s, pool := newTestService(t)
	added := addTodo(t, s, "Buy a domain")
	tests := []struct {
		name      string
		params    UpdateTodoParams
		wantField string
	}{
		{"no field", UpdateTodoParams{}, "fields"},
		{"empty title", UpdateTodoParams{Title: ptr(" ")}, "title"},
		{"unknown status", UpdateTodoParams{Status: ptr("later")}, "status"},
		{"date that is not a date", UpdateTodoParams{DueOn: ptr("soon")}, "due_on"},
		{"empty date", UpdateTodoParams{DueOn: ptr("")}, "due_on"},
		{"empty project", UpdateTodoParams{Project: ptr(" ")}, "project"},
		{"clear of a required field", UpdateTodoParams{Clear: []string{"title"}}, "clear"},
		{"clear of done_at", UpdateTodoParams{Clear: []string{"done_at"}}, "clear"},
		{"date with a value and in clear", UpdateTodoParams{DueOn: ptr("2026-11-01"), Clear: []string{"due_on"}}, "due_on"},
		{"project with a value and in clear", UpdateTodoParams{Project: ptr("home"), Clear: []string{"project"}}, "project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.params.Actor = SourceManual
			_, err := s.UpdateTodo(context.Background(), added.ID, tt.params)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
	_, err := s.UpdateTodo(context.Background(), added.ID, UpdateTodoParams{Title: ptr("x"), Actor: "robot"})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "source" {
		t.Errorf("unknown actor: error = %v, want a ValidationError on source", err)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want 1 (the create): refused input appended an event", n)
	}
}

func TestUpdateTodoReturnsNotFound(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	deleted := addTodo(t, s, "Deleted")
	if err := s.DeleteTodo(ctx, deleted.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	for name, id := range map[string]int64{"deleted": deleted.ID, "absent": 9999} {
		_, err := s.UpdateTodo(ctx, id, UpdateTodoParams{Title: ptr("x"), Actor: SourceManual})
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", name, err)
		}
	}
}

// An agent that reads "done with the domain" in a captured page must not
// close a todo that Gautam stated. It can change a todo that it proposed.
func TestUpdateTodoRefusesAnInferredChangeToAVerifiedTodo(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	verified := addTodo(t, s, "Stated by Gautam")
	p := buyDomain()
	p.Verified = false
	proposed, err := s.AddTodo(ctx, p)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.UpdateTodo(ctx, verified.ID, UpdateTodoParams{Status: ptr("done"), Actor: SourceAgent, Inferred: true})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "dictated" {
		t.Fatalf("error = %v, want a ValidationError on dictated", err)
	}
	if got, _ := s.GetTodo(ctx, verified.ID); got.Status != "open" {
		t.Errorf("status = %q, want open", got.Status)
	}
	if n := countRows(t, pool, "events"); n != 2 {
		t.Errorf("events = %d, want 2 (the two creates)", n)
	}

	got, err := s.UpdateTodo(ctx, proposed.ID, UpdateTodoParams{DueOn: ptr("2026-11-01"), Actor: SourceAgent, Inferred: true})
	if err != nil || got.DueOn != "2026-11-01" || got.Verified {
		t.Errorf("inferred change to a proposed todo = %+v, %v; want the change, and the todo not verified", got, err)
	}
}

func TestUpdateTodoWritesNothingWhenNoValueChanges(t *testing.T) {
	s, pool := newTestService(t)
	added := addTodo(t, s, "Buy a domain")

	got, err := s.UpdateTodo(context.Background(), added.ID, UpdateTodoParams{
		Title: ptr("Buy a domain "), Status: ptr("open"), DueOn: ptr("2026-10-20"), Project: ptr("lifeos"),
		Actor: SourceAgent, Inferred: true,
	})

	if err != nil || got != added {
		t.Errorf("UpdateTodo = %+v, %v, want the stored todo and no error", got, err)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want 1 (the create)", n)
	}
}

func TestDeleteTodo(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added := addTodo(t, s, "Buy a domain")

	err := s.DeleteTodo(ctx, added.ID, DeleteParams{Actor: SourceAgent})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "confirmed" {
		t.Fatalf("delete by an agent with no confirmation: error = %v, want a ValidationError on confirmed", err)
	}

	if err := s.DeleteTodo(ctx, added.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("DeleteTodo: %v", err)
	}
	if _, err := s.GetTodo(ctx, added.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get after delete: error = %v, want ErrNotFound", err)
	}
	if todos, _ := s.ListTodos(ctx, ListTodosParams{}); len(todos) != 0 {
		t.Errorf("list after delete = %v, want it empty", todos)
	}
	if n := countRows(t, pool, "todos"); n != 1 {
		t.Errorf("todos has %d rows, want 1: a delete keeps the row", n)
	}
	events := allEvents(t, pool)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (create, delete)", len(events))
	}
	if e := events[1]; e.Action != "delete" || e.RecordType != "todos" || e.RecordID != added.ID || e.Old["title"] != "Buy a domain" || e.New != nil {
		t.Errorf("event = %+v, want a delete with the old todo", e)
	}

	if err := s.DeleteTodo(ctx, added.ID, DeleteParams{Actor: SourceManual}); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: error = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run Todo"
```
Expected: the run fails. The output contains `s.UpdateTodo undefined`.

- [ ] **Step 3: Write the queries**

Add this at the end of `queries/todos.sql`:

```sql
-- name: GetTodoForUpdate :one
SELECT * FROM todos
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- name: UpdateTodo :one
UPDATE todos
SET title = $2, status = $3, due_on = $4, project = $5, done_at = $6
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteTodo :one
UPDATE todos
SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;
```

- [ ] **Step 4: Write the update and the delete**

`done_at` comes from the clock of the service, so a test can set it. It keeps its value while the todo stays done.

Create `internal/service/todos_change.go`:

```go
package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// UpdateTodoParams is the input of UpdateTodo. A field that is nil keeps
// its stored value.
type UpdateTodoParams struct {
	Title *string `json:"title"`
	// Status is open, done or dropped. The service sets done_at from it.
	Status  *string `json:"status"`
	DueOn   *string `json:"due_on"`
	Project *string `json:"project"`
	// Clear names the optional fields to empty: due_on, project.
	Clear []string `json:"-"`
	// Actor is who makes the change. It goes into the event; the source
	// of the row stays as it is.
	Actor Source `json:"-"`
	// Inferred is true when an agent makes a change that Gautam did not
	// state. The service refuses such a change to a verified row.
	Inferred bool `json:"-"`
}

// clearableTodoFields are the optional fields that Clear can name.
var clearableTodoFields = []string{"due_on", "project"}

// UpdateTodo changes the given fields of one todo and appends an update
// event that holds the old row, in one transaction. It locks the row
// first, so two updates at the same moment do not lose a field. A call
// that changes no value writes nothing and appends no event.
func (s *Service) UpdateTodo(ctx context.Context, id int64, p UpdateTodoParams) (Todo, error) {
	if err := p.check(); err != nil {
		return Todo{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Todo{}, fmt.Errorf("update todo: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	current, err := q.GetTodoForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Todo{}, ErrNotFound
	}
	if err != nil {
		return Todo{}, fmt.Errorf("update todo: %w", err)
	}
	old := todoFromRow(current)

	// The changed todo goes through the same checks as a new todo.
	next := p.applyTo(old)
	v, err := checkTodo(next.Title, next.Status, next.DueOn, next.Project)
	if err != nil {
		return Todo{}, err
	}
	next.Title = v.title
	if next == old {
		return old, nil
	}
	// A guess must not replace a value that Gautam approved.
	if p.Inferred && old.Verified {
		return Todo{}, invalid("dictated", "must be true to change a verified record: ask the user to confirm the change, then send it again")
	}
	// done_at follows the status. It keeps its value while the todo
	// stays done.
	doneAt := current.DoneAt
	switch {
	case v.status != "done":
		doneAt = nil
	case old.Status != "done":
		now := s.now().UTC()
		doneAt = &now
	}
	stored, err := q.UpdateTodo(ctx, store.UpdateTodoParams{
		ID:      id,
		Title:   v.title,
		Status:  v.status,
		DueOn:   v.dueOn,
		Project: v.project,
		DoneAt:  doneAt,
	})
	if err != nil {
		return Todo{}, fmt.Errorf("update todo: %w", err)
	}
	todo := todoFromRow(stored)
	if err := recordEvent(ctx, q, "update", "todos", id, p.Actor, old, todo); err != nil {
		return Todo{}, fmt.Errorf("update todo: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Todo{}, fmt.Errorf("update todo: %w", err)
	}
	return todo, nil
}

// check refuses input that is wrong for every stored row.
func (p UpdateTodoParams) check() error {
	if err := (Provenance{Source: p.Actor}).validate(); err != nil {
		return err
	}
	if p.Title == nil && p.Status == nil && p.DueOn == nil && p.Project == nil && len(p.Clear) == 0 {
		return invalid("fields", "must name at least one field to change, or a field to empty in clear")
	}
	for _, name := range p.Clear {
		if !slices.Contains(clearableTodoFields, name) {
			return invalid("clear", "can hold only due_on, project")
		}
	}
	if p.DueOn != nil {
		if slices.Contains(p.Clear, "due_on") {
			return invalid("due_on", "has a value and is also in clear")
		}
		if *p.DueOn == "" {
			return invalid("due_on", "must be a date as YYYY-MM-DD; to remove the date, put due_on in clear")
		}
	}
	if p.Project != nil {
		if slices.Contains(p.Clear, "project") {
			return invalid("project", "has a value and is also in clear")
		}
		if strings.TrimSpace(*p.Project) == "" {
			return invalid("project", "must not be empty; to remove the project, put project in clear")
		}
	}
	return nil
}

// applyTo returns old with the changes of p. The values are not checked
// yet.
func (p UpdateTodoParams) applyTo(old Todo) Todo {
	next := old
	if p.Title != nil {
		next.Title = *p.Title
	}
	if p.Status != nil {
		next.Status = *p.Status
	}
	if p.DueOn != nil {
		next.DueOn = *p.DueOn
	}
	if p.Project != nil {
		next.Project = strings.TrimSpace(*p.Project)
	}
	if slices.Contains(p.Clear, "due_on") {
		next.DueOn = ""
	}
	if slices.Contains(p.Clear, "project") {
		next.Project = ""
	}
	return next
}

// DeleteTodo marks one todo as deleted and appends a delete event that
// holds the row, in one transaction.
func (s *Service) DeleteTodo(ctx context.Context, id int64, p DeleteParams) error {
	return s.deleteRecord(ctx, "todos", id, p, func(q *store.Queries) (any, error) {
		row, err := q.SoftDeleteTodo(ctx, id)
		if err != nil {
			return nil, err
		}
		return todoFromRow(row), nil
	})
}
```

- [ ] **Step 5: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./internal/service ARGS="-run Todo"
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/service`.

- [ ] **Step 6: Commit**

Run:
```bash
git add queries/todos.sql internal/service/todos_change.go internal/service/todos_change_test.go
git commit -m "feat(service): correct and delete a todo"
```

- [ ] **Step 7: Write the failing tests for the record functions and the review**

Create `internal/service/todos_records_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"
)

func TestRecordFunctionsWorkOnATodo(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added := addTodo(t, s, "Buy a domain")

	got, err := s.Get(ctx, "todo", added.ID)
	if todo, ok := got.(Todo); err != nil || !ok || todo != added {
		t.Fatalf("Get = %#v, %v, want the todo", got, err)
	}

	got, err = s.Update(ctx, "todo", added.ID, UpdateParams{
		Fields: map[string]any{"status": "done"}, Clear: []string{"due_on"}, Actor: SourceManual,
	})
	if todo, ok := got.(Todo); err != nil || !ok || todo.Status != "done" || todo.DoneAt == nil || todo.DueOn != "" || todo.Project != "lifeos" {
		t.Fatalf("Update = %#v, %v, want the todo done, with no date", got, err)
	}

	// done_at follows the status. A caller must not set it.
	refused := map[string]map[string]any{
		"done_at":  {"done_at": "2026-10-11T00:00:00Z"},
		"verified": {"verified": true},
		"source":   {"source": "manual"},
		"status":   {"status": 1},
		"project":  {"project": nil},
	}
	for field, fields := range refused {
		_, err := s.Update(ctx, "todo", added.ID, UpdateParams{Fields: fields, Actor: SourceManual})
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != field {
			t.Errorf("%s: error = %v, want a ValidationError on %s", field, err, field)
		}
	}

	if err := s.Delete(ctx, "todo", added.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "todo", added.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: error = %v, want ErrNotFound", err)
	}
}

// addProposedTodo stores a todo as an agent proposes it from a source: not
// verified.
func addProposedTodo(t *testing.T, s *Service, p AddTodoParams) Todo {
	t.Helper()
	p.Provenance = Provenance{Source: SourceAgent}
	todo, err := s.AddTodo(context.Background(), p)
	if err != nil {
		t.Fatalf("AddTodo(%q): %v", p.Title, err)
	}
	return todo
}

func TestReviewQueueListsAProposedTodo(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	item := addUnverified(t, s, "Dune")
	full := addProposedTodo(t, s, AddTodoParams{Title: "Renew the insurance", DueOn: "2026-12-01", Project: "home"})
	bare := addProposedTodo(t, s, AddTodoParams{Title: "Call the bank"})
	addTodo(t, s, "Stated by Gautam") // verified

	got, err := s.ReviewQueue(ctx, 0)
	if err != nil {
		t.Fatalf("ReviewQueue: %v", err)
	}
	if len(got) != 3 || got[0].ID != item.ID || got[0].Type != "item" {
		t.Fatalf("queue = %+v, want the item, then the two todos", got)
	}
	if got[1].Type != "todo" || got[1].ID != full.ID || got[1].Summary != "todo: Renew the insurance, project home, due 2026-12-01" {
		t.Errorf("entry 1 = %+v", got[1])
	}
	if got[2].Type != "todo" || got[2].ID != bare.ID || got[2].Summary != "todo: Call the bank" || got[2].Source != SourceAgent {
		t.Errorf("entry 2 = %+v", got[2])
	}
}

func TestVerifyApprovesATodo(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	todo := addProposedTodo(t, s, AddTodoParams{Title: "Call the bank"})

	n, err := s.Verify(ctx, []RecordRef{{"todo", todo.ID}}, SourceManual)
	if err != nil || n != 1 {
		t.Fatalf("Verify = %d, %v, want 1", n, err)
	}
	if got, _ := s.GetTodo(ctx, todo.ID); !got.Verified {
		t.Error("the todo is not verified")
	}
	events := allEvents(t, pool)
	e := events[len(events)-1]
	if e.Action != "verify" || e.RecordType != "todos" || e.RecordID != todo.ID || e.Old["verified"] != false || e.New["verified"] != true {
		t.Errorf("event = %+v, want a verify of the todo", e)
	}
	if queue, _ := s.ReviewQueue(ctx, 0); len(queue) != 0 {
		t.Errorf("queue = %+v, want it empty", queue)
	}
}
```

- [ ] **Step 8: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run Todo"
```
Expected: the run fails. The output contains `--- FAIL: TestRecordFunctionsWorkOnATodo`, `--- FAIL: TestReviewQueueListsAProposedTodo` and `--- FAIL: TestVerifyApprovesATodo`.

- [ ] **Step 9: Write the queries**

Add this at the end of `queries/todos.sql`:

```sql
-- name: ListUnverifiedTodos :many
SELECT * FROM todos
WHERE deleted_at IS NULL AND NOT verified
ORDER BY created_at, id
LIMIT $1;

-- name: VerifyTodo :one
UPDATE todos
SET verified = true
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;
```

- [ ] **Step 10: Add the type `todo` to `RecordTypes` and see the guard test fail**

In `internal/service/records.go`, replace this:

```go
var RecordTypes = []string{"item", "journal"}
```

With this:

```go
var RecordTypes = []string{"item", "journal", "todo"}
```

The type is in the list now, and no function has a `case` for it. The guard test of Task 3 must find that.

Run:
```bash
make test PKG=./internal/service ARGS="-run EveryRecordType"
```
Expected: the run fails. The output contains `--- FAIL: TestEveryRecordTypeHasEveryEntryPoint` and `no code for the record type`.

- [ ] **Step 11: Give each record function a case for `todo`**

In `internal/service/records.go`, replace this:

```go
// the type item, a JournalEntry for the type journal.
```

With this:

```go
// the type item, a JournalEntry for the type journal, a Todo for the type
// todo.
```

In `internal/service/records.go`, replace this:

```go
	case "journal":
		return asRecord(s.GetJournalEntry(ctx, id))
```

With this:

```go
	case "journal":
		return asRecord(s.GetJournalEntry(ctx, id))
	case "todo":
		return asRecord(s.GetTodo(ctx, id))
```

In `internal/service/records.go`, replace this:

```go
		params.Actor, params.Inferred = p.Actor, p.Inferred
		return asRecord(s.UpdateJournalEntry(ctx, id, params))
```

With this:

```go
		params.Actor, params.Inferred = p.Actor, p.Inferred
		return asRecord(s.UpdateJournalEntry(ctx, id, params))
	case "todo":
		var params UpdateTodoParams
		if err := decodeFields(p.Fields, &params, recordType); err != nil {
			return nil, err
		}
		params.Clear, params.Actor, params.Inferred = p.Clear, p.Actor, p.Inferred
		return asRecord(s.UpdateTodo(ctx, id, params))
```

In `internal/service/records.go`, replace this:

```go
	case "journal":
		return s.DeleteJournalEntry(ctx, id, p)
```

With this:

```go
	case "journal":
		return s.DeleteJournalEntry(ctx, id, p)
	case "todo":
		return s.DeleteTodo(ctx, id, p)
```

- [ ] **Step 12: Add the type `todo` to the review queue and to `Verify`**

In `internal/service/review.go`, replace this:

```go
		unverifiedJournalEntries,
```

With this:

```go
		unverifiedJournalEntries,
		unverifiedTodos,
```

In `internal/service/review.go`, replace this:

```go
		return "journal_entries", before, after, err
```

With this:

```go
		return "journal_entries", before, after, err
	case "todo":
		before, after, err = verifyTodo(ctx, q, ref.ID)
		return "todos", before, after, err
```

Add this at the end of `internal/service/todos_change.go`:

```go
// summary is the one line that the review queue shows for a todo.
func (t Todo) summary() string {
	s := "todo: " + t.Title
	if t.Project != "" {
		s += ", project " + t.Project
	}
	if t.DueOn != "" {
		s += ", due " + t.DueOn
	}
	return s
}

// unverifiedTodos returns the todos that wait for review, oldest first.
func unverifiedTodos(ctx context.Context, q *store.Queries, limit int32) ([]ReviewEntry, error) {
	rows, err := q.ListUnverifiedTodos(ctx, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]ReviewEntry, 0, len(rows))
	for _, row := range rows {
		todo := todoFromRow(row)
		entries = append(entries, ReviewEntry{
			Type:      "todo",
			ID:        todo.ID,
			Summary:   todo.summary(),
			Source:    todo.Source,
			CreatedAt: todo.CreatedAt,
			UpdatedAt: todo.UpdatedAt,
		})
	}
	return entries, nil
}

func verifyTodo(ctx context.Context, q *store.Queries, id int64) (before, after any, err error) {
	current, err := q.GetTodoForUpdate(ctx, id)
	if err != nil || current.Verified {
		return nil, nil, err
	}
	stored, err := q.VerifyTodo(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return todoFromRow(current), todoFromRow(stored), nil
}
```

- [ ] **Step 13: Run the service tests to verify that they pass**

Run:
```bash
make test PKG=./internal/service
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/service`.

- [ ] **Step 14: Commit**

Run:
```bash
git add queries/todos.sql internal/service/records.go internal/service/review.go internal/service/todos_change.go internal/service/todos_records_test.go
git commit -m "feat(service): add todos to the record functions and the review"
```

---

### Task 8: REST API and MCP tool for todos

**Files:**
- Create: `internal/api/todos.go`, `internal/mcp/todos.go`
- Modify: `internal/api/api.go` (one line), `internal/mcp/server.go` (one line), `internal/mcp/records.go` (the argument descriptions)
- Test: `internal/api/todos_test.go`, `internal/mcp/todos_test.go`

**Interfaces:**
- Consumes (Tasks 6 and 7): `service.AddTodo`, `GetTodo`, `ListTodos`, `DeleteTodo`, `service.Update` with the type `todo`, `service.Todo`, `service.ListTodosParams`. Consumes (Task 4): `patch`, `idInput`, `toHTTPError`, `toToolError`, the test helpers of the two packages.
- Produces:
  - Package `api`: the routes `POST /api/todos` (201), `GET /api/todos` with the query parameters `status` and `limit`, `GET /api/todos/{id}`, `PATCH /api/todos/{id}`, `DELETE /api/todos/{id}` (204). The list is in the body as `{"todos": [...]}`.
  - Package `mcp`: the tool `todo` with the arguments `title`, `due_on`, `project` and `dictated`
  - Test helper in package `api_test`: `addTodo(t, h, body string) string`, the constant `domainJSON`

- [ ] **Step 1: Write the failing tests for the routes**

Create `internal/api/todos_test.go`:

```go
package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const domainJSON = `{"title":"Buy a Cloudflare domain","due_on":"2026-10-20","project":"lifeos"}`

// addTodo stores one todo through the API and returns its path.
func addTodo(t *testing.T, h http.Handler, body string) string {
	t.Helper()
	status, added, raw := do(t, h, "POST", "/api/todos", body)
	if status != http.StatusCreated {
		t.Fatalf("POST /api/todos: status = %d, want 201\n%s", status, raw)
	}
	return fmt.Sprintf("/api/todos/%d", int64(added["id"].(float64)))
}

func TestAddTodoThenReadIt(t *testing.T) {
	h, _ := newTestAPI(t)

	status, added, raw := do(t, h, "POST", "/api/todos", domainJSON)

	if status != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201\n%s", status, raw)
	}
	if added["title"] != "Buy a Cloudflare domain" || added["status"] != "open" || added["due_on"] != "2026-10-20" || added["project"] != "lifeos" {
		t.Errorf("todo = %s", raw)
	}
	if added["source"] != "manual" || added["verified"] != true {
		t.Errorf("source, verified = %v, %v, want manual, true", added["source"], added["verified"])
	}

	path := fmt.Sprintf("/api/todos/%d", int64(added["id"].(float64)))
	status, got, raw := do(t, h, "GET", path, "")
	if status != http.StatusOK || got["title"] != "Buy a Cloudflare domain" {
		t.Errorf("GET: status = %d, todo = %s", status, raw)
	}
}

func TestAddTodoRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantInBody string
	}{
		{"title of spaces", `{"title":"  "}`, 422, "body.title"},
		{"missing title", `{"project":"lifeos"}`, 422, "title"},
		{"date that is not a date", `{"title":"x","due_on":"soon"}`, 422, "body.due_on"},
		// A new todo is open. A caller cannot set the status or done_at.
		{"status", `{"title":"x","status":"done"}`, 422, "status"},
		{"done_at", `{"title":"x","done_at":"2026-10-11T00:00:00Z"}`, 422, "done_at"},
		{"request that is not JSON", `title=x`, 400, ""},
	}
	h, pool := newTestAPI(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, raw := do(t, h, "POST", "/api/todos", tt.body)

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d\n%s", status, tt.wantStatus, raw)
			}
			if !strings.Contains(raw, tt.wantInBody) {
				t.Errorf("body does not name %q:\n%s", tt.wantInBody, raw)
			}
		})
	}
	if n := countEvents(t, pool); n != 0 {
		t.Errorf("events = %d after rejected requests, want 0", n)
	}
}

func TestListTodos(t *testing.T) {
	h, _ := newTestAPI(t)

	status, _, raw := do(t, h, "GET", "/api/todos", "")
	if status != http.StatusOK || !strings.Contains(raw, `"todos":[]`) {
		t.Fatalf("empty list: status = %d, body = %s; want 200 and an empty array", status, raw)
	}

	done := addTodo(t, h, `{"title":"done"}`)
	addTodo(t, h, `{"title":"open"}`)
	if status, _, raw := do(t, h, "PATCH", done, `{"status":"done"}`); status != http.StatusOK {
		t.Fatalf("PATCH: status = %d\n%s", status, raw)
	}

	titles := func(query string) string {
		t.Helper()
		status, list, raw := do(t, h, "GET", "/api/todos"+query, "")
		if status != http.StatusOK {
			t.Fatalf("GET /api/todos%s: status = %d\n%s", query, status, raw)
		}
		var out []string
		for _, todo := range list["todos"].([]any) {
			out = append(out, todo.(map[string]any)["title"].(string))
		}
		return strings.Join(out, ",")
	}
	if got := titles(""); got != "open,done" {
		t.Errorf("all = %q, want open,done (the open todos first)", got)
	}
	if got := titles("?status=done"); got != "done" {
		t.Errorf("status=done = %q, want done", got)
	}
	if got := titles("?status=open&limit=1"); got != "open" {
		t.Errorf("status=open = %q, want open", got)
	}
	for _, query := range []string{"?status=later", "?limit=500"} {
		if status, _, raw := do(t, h, "GET", "/api/todos"+query, ""); status != http.StatusUnprocessableEntity {
			t.Errorf("GET /api/todos%s: status = %d, want 422\n%s", query, status, raw)
		}
	}
}

// "Done" is one field. The title, the project and the date must stay.
func TestPatchTodoMarksItDoneAndClearsAField(t *testing.T) {
	h, _ := newTestAPI(t)
	path := addTodo(t, h, domainJSON)

	status, patched, raw := do(t, h, "PATCH", path, `{"status":"done"}`)
	if status != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200\n%s", status, raw)
	}
	if patched["status"] != "done" || patched["done_at"] == nil || patched["title"] != "Buy a Cloudflare domain" || patched["project"] != "lifeos" || patched["due_on"] != "2026-10-20" {
		t.Errorf("todo after PATCH = %s", raw)
	}

	status, patched, raw = do(t, h, "PATCH", path, `{"status":"open","clear":["due_on","project"]}`)
	if status != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200\n%s", status, raw)
	}
	for _, field := range []string{"done_at", "due_on", "project"} {
		if _, has := patched[field]; has {
			t.Errorf("the todo still has %s:\n%s", field, raw)
		}
	}
}

func TestPatchTodoRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		body       string
		wantStatus int
		wantInBody string
	}{
		{"unknown status", "", `{"status":"later"}`, 422, "body.status"},
		{"empty title", "", `{"title":" "}`, 422, "body.title"},
		{"clear of a required field", "", `{"clear":["title"]}`, 422, "body.clear"},
		{"done_at", "", `{"done_at":"2026-10-11T00:00:00Z"}`, 422, "done_at"},
		{"field that a caller cannot change", "", `{"verified":false}`, 422, "verified"},
		{"null", "", `{"title":"New","due_on":null}`, 422, "due_on"},
		{"empty object", "", `{}`, 422, "body.fields"},
		{"body that is not JSON", "", `status=done`, 400, ""},
		{"unknown ID", "/api/todos/9999", `{"status":"done"}`, 404, ""},
		{"ID that is not a number", "/api/todos/abc", `{"status":"done"}`, 422, ""},
	}
	h, pool := newTestAPI(t)
	path := addTodo(t, h, domainJSON)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := path
			if tt.path != "" {
				target = tt.path
			}
			status, _, raw := do(t, h, "PATCH", target, tt.body)

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d\n%s", status, tt.wantStatus, raw)
			}
			if !strings.Contains(raw, tt.wantInBody) {
				t.Errorf("body does not name %q:\n%s", tt.wantInBody, raw)
			}
		})
	}
	_, got, raw := do(t, h, "GET", path, "")
	if got["status"] != "open" || got["title"] != "Buy a Cloudflare domain" || got["due_on"] != "2026-10-20" {
		t.Errorf("a refused PATCH changed the todo: %s", raw)
	}
	if n := countEvents(t, pool); n != 1 {
		t.Errorf("events = %d after refused requests, want 1 (the create)", n)
	}
}

func TestDeleteTodo(t *testing.T) {
	h, pool := newTestAPI(t)
	path := addTodo(t, h, domainJSON)

	status, _, raw := do(t, h, "DELETE", path, "")

	if status != http.StatusNoContent || raw != "" {
		t.Fatalf("DELETE: status = %d, body = %q; want 204 and no body", status, raw)
	}
	if status, _, _ := do(t, h, "GET", path, ""); status != http.StatusNotFound {
		t.Errorf("GET after DELETE: status = %d, want 404", status)
	}
	if _, _, raw := do(t, h, "GET", "/api/todos", ""); !strings.Contains(raw, `"todos":[]`) {
		t.Errorf("the list still shows the todo:\n%s", raw)
	}
	for _, target := range []string{path, "/api/todos/9999"} {
		if status, _, raw := do(t, h, "DELETE", target, ""); status != http.StatusNotFound {
			t.Errorf("DELETE %s: status = %d, want 404\n%s", target, status, raw)
		}
	}
	if n := countEvents(t, pool); n != 2 {
		t.Errorf("events = %d, want 2 (create, one delete)", n)
	}
}

func TestDatabaseFailureOnATodoRouteGivesAPlain500(t *testing.T) {
	h, pool := newTestAPI(t)
	pool.Close()

	requests := []struct{ method, path, body string }{
		{"POST", "/api/todos", `{"title":"x"}`},
		{"GET", "/api/todos", ""},
		{"GET", "/api/todos/1", ""},
		{"PATCH", "/api/todos/1", `{"status":"done"}`},
		{"DELETE", "/api/todos/1", ""},
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
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/api ARGS="-run Todo"
```
Expected: the run fails. The output contains `POST /api/todos: body is not a JSON object`.

- [ ] **Step 3: Write the routes**

Create `internal/api/todos.go`:

```go
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type addTodoInput struct {
	Body struct {
		Title   string `json:"title"`
		DueOn   string `json:"due_on,omitempty" doc:"The day that the todo is due, as YYYY-MM-DD"`
		Project string `json:"project,omitempty" doc:"The project of the todo, such as lifeos"`
	}
}

type todoOutput struct {
	Body service.Todo
}

type listTodosInput struct {
	Status string `query:"status" doc:"One of open, done, dropped. Empty lists every status."`
	Limit  int    `query:"limit" doc:"1 to 200. The default is 50."`
}

type listTodosOutput struct {
	Body struct {
		Todos []service.Todo `json:"todos"`
	}
}

type updateTodoInput struct {
	ID   int64 `path:"id"`
	Body struct {
		Title   *string  `json:"title,omitempty"`
		Status  *string  `json:"status,omitempty" doc:"One of open, done, dropped"`
		DueOn   *string  `json:"due_on,omitempty" doc:"The day that the todo is due, as YYYY-MM-DD"`
		Project *string  `json:"project,omitempty"`
		Clear   []string `json:"clear,omitempty" doc:"Optional fields to empty: due_on, project"`
	}
	// RawBody is the body as it came. Body cannot tell a null from a
	// field that is absent, and the service refuses a null.
	RawBody []byte
}

func registerTodos(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{
		OperationID:   "add-todo",
		Method:        http.MethodPost,
		Path:          "/api/todos",
		Summary:       "Add a todo",
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *addTodoInput) (*todoOutput, error) {
		// A request to the REST API comes from Gautam's own devices, so
		// the row is manual and verified.
		todo, err := svc.AddTodo(ctx, service.AddTodoParams{
			Title:      in.Body.Title,
			DueOn:      in.Body.DueOn,
			Project:    in.Body.Project,
			Provenance: service.Provenance{Source: service.SourceManual, Verified: true},
		})
		if err != nil {
			return nil, toHTTPError("body", err)
		}
		return &todoOutput{Body: todo}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-todos",
		Method:      http.MethodGet,
		Path:        "/api/todos",
		Summary:     "List todos: the open todos first, then the newest first",
	}, func(ctx context.Context, in *listTodosInput) (*listTodosOutput, error) {
		todos, err := svc.ListTodos(ctx, service.ListTodosParams{Status: in.Status, Limit: in.Limit})
		if err != nil {
			return nil, toHTTPError("query", err)
		}
		out := &listTodosOutput{}
		out.Body.Todos = todos
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-todo",
		Method:      http.MethodGet,
		Path:        "/api/todos/{id}",
		Summary:     "Read one todo",
	}, func(ctx context.Context, in *idInput) (*todoOutput, error) {
		todo, err := svc.GetTodo(ctx, in.ID)
		if err != nil {
			return nil, toHTTPError("path", err)
		}
		return &todoOutput{Body: todo}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-todo",
		Method:      http.MethodPatch,
		Path:        "/api/todos/{id}",
		Summary:     "Change a todo, or mark it done",
		Description: "Changes the fields in the body. A field that is not in the body keeps its value.",
	}, func(ctx context.Context, in *updateTodoInput) (*todoOutput, error) {
		todo, err := patch[service.Todo](ctx, svc, "todo", in.ID, in.RawBody, in.Body.Clear)
		if err != nil {
			return nil, err
		}
		return &todoOutput{Body: todo}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-todo",
		Method:        http.MethodDelete,
		Path:          "/api/todos/{id}",
		Summary:       "Delete a todo",
		Description:   "Marks the todo as deleted. The row stays in the database.",
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *idInput) (*struct{}, error) {
		if err := svc.DeleteTodo(ctx, in.ID, service.DeleteParams{Actor: service.SourceManual}); err != nil {
			return nil, toHTTPError("path", err)
		}
		return nil, nil
	})
}
```

In `internal/api/api.go`, replace this:

```go
	registerJournal(api, svc)
```

With this:

```go
	registerJournal(api, svc)
	registerTodos(api, svc)
```

- [ ] **Step 4: Run the API tests to verify that they pass**

Run:
```bash
make test PKG=./internal/api
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/api`.

- [ ] **Step 5: Commit**

Run:
```bash
git add internal/api/api.go internal/api/todos.go internal/api/todos_test.go
git commit -m "feat(api): add the todo routes"
```

- [ ] **Step 6: Write the failing tests for the tool**

Create `internal/mcp/todos_test.go`:

```go
package mcp_test

import (
	"context"
	"strings"
	"testing"
)

// "todo - buy cloudflare domain and set up tunnel": the user dictated the
// two actions, so the two rows are verified.
func TestTodoStoresATodoThatTheUserDictated(t *testing.T) {
	session, pool := connect(t)

	first := structured(t, call(t, session, "todo", map[string]any{"title": "Buy a Cloudflare domain", "project": "lifeos", "dictated": true}))
	second := structured(t, call(t, session, "todo", map[string]any{"title": "Set up the tunnel", "project": "lifeos", "dictated": true}))

	if first["id"] == second["id"] {
		t.Errorf("the two todos have one ID: %v", first["id"])
	}
	if first["title"] != "Buy a Cloudflare domain" || first["status"] != "open" || first["project"] != "lifeos" {
		t.Errorf("todo = %v", first)
	}
	if _, has := first["due_on"]; has {
		t.Errorf("the todo has a date: %v", first)
	}
	if first["source"] != "agent" || first["verified"] != true {
		t.Errorf("source, verified = %v, %v, want agent, true", first["source"], first["verified"])
	}
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM events WHERE action = 'create' AND record_type = 'todos' AND source = 'agent'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("create events = %d, want 2", n)
	}
}

// A todo that the agent finds in a journal entry or a document is a
// proposal. It must wait in the review queue until the user approves it.
func TestTodoIsAProposalUnlessDictated(t *testing.T) {
	session, _ := connect(t)

	added := structured(t, call(t, session, "todo", map[string]any{"title": "Renew the insurance", "due_on": "2026-12-01"}))

	if added["verified"] != false {
		t.Fatalf("verified = %v, want false when dictated is absent", added["verified"])
	}
	queue := structured(t, call(t, session, "review_queue", map[string]any{}))
	entries := queue["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("queue = %v, want the todo", queue)
	}
	entry := entries[0].(map[string]any)
	if entry["type"] != "todo" || entry["id"] != added["id"] || entry["summary"] != "todo: Renew the insurance, due 2026-12-01" {
		t.Errorf("entry = %v", entry)
	}

	verified := structured(t, call(t, session, "verify", map[string]any{
		"records": []map[string]any{{"type": "todo", "id": added["id"]}},
	}))
	if verified["verified"] != float64(1) {
		t.Errorf("verify returned %v, want 1", verified)
	}
}

// The tool todo adds. The tool update changes a todo and marks it done.
func TestUpdateMarksATodoDone(t *testing.T) {
	session, _ := connect(t)
	added := structured(t, call(t, session, "todo", map[string]any{"title": "Buy a domain", "due_on": "2026-10-20", "dictated": true}))

	refused := call(t, session, "update", map[string]any{"type": "todo", "id": added["id"], "fields": map[string]any{"status": "done"}})
	if !refused.IsError || !strings.Contains(text(refused), "dictated") {
		t.Fatalf("update with no dictated: IsError = %v, text = %q; want an error that names dictated", refused.IsError, text(refused))
	}

	updated := structured(t, call(t, session, "update", map[string]any{
		"type": "todo", "id": added["id"], "fields": map[string]any{"status": "done"}, "clear": []string{"due_on"}, "dictated": true,
	}))
	if updated["status"] != "done" || updated["done_at"] == nil || updated["title"] != "Buy a domain" {
		t.Errorf("update returned %v", updated)
	}
	if _, has := updated["due_on"]; has {
		t.Errorf("the todo still has a date: %v", updated)
	}

	got := structured(t, call(t, session, "get", map[string]any{"type": "todo", "id": added["id"]}))
	if got["status"] != "done" {
		t.Errorf("get returned %v", got)
	}
	unknown := call(t, session, "update", map[string]any{"type": "todo", "id": added["id"], "fields": map[string]any{"status": "later"}, "dictated": true})
	if !unknown.IsError || !strings.Contains(text(unknown), "status: must be one of open, done, dropped") {
		t.Errorf("update with an unknown status: IsError = %v, text = %q", unknown.IsError, text(unknown))
	}
	if result := call(t, session, "delete", map[string]any{"type": "todo", "id": added["id"]}); !result.IsError || !strings.Contains(text(result), "confirmed") {
		t.Errorf("delete with no confirmation: IsError = %v, text = %q", result.IsError, text(result))
	}
}

func TestTodoReturnsErrorsTheModelCanRead(t *testing.T) {
	session, pool := connect(t)
	tests := []struct {
		name     string
		args     map[string]any
		wantText string
	}{
		{"empty title", map[string]any{"title": " "}, "title"},
		{"missing title", map[string]any{"project": "lifeos"}, "title"},
		{"date that is not a date", map[string]any{"title": "x", "due_on": "next week"}, "due_on"},
		{"misspelled argument", map[string]any{"title": "x", "due": "2026-12-01"}, "due"},
		// A new todo is open. The tool has no status argument.
		{"status", map[string]any{"title": "x", "status": "done"}, "status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := call(t, session, "todo", tt.args)
			if !result.IsError {
				t.Fatalf("IsError = false, want an error result")
			}
			if !strings.Contains(text(result), tt.wantText) {
				t.Errorf("error text %q does not name %q", text(result), tt.wantText)
			}
		})
	}
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM todos").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("todos has %d rows after rejected calls, want 0", n)
	}

	pool.Close()
	result := call(t, session, "todo", map[string]any{"title": "x"})
	if !result.IsError || text(result) != "internal error" {
		t.Errorf("database failure: IsError = %v, text = %q; want only \"internal error\"", result.IsError, text(result))
	}
}
```

- [ ] **Step 7: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/mcp ARGS="-run Todo"
```
Expected: the run fails. The output contains `--- FAIL: TestTodoStoresATodoThatTheUserDictated` and `--- FAIL: TestUpdateMarksATodoDone`.

- [ ] **Step 8: Write the tool**

The tool adds one todo. Its description sends the model to `update` for a change.

Create `internal/mcp/todos.go`:

```go
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type todoInput struct {
	Title    string `json:"title" jsonschema:"One action, in a few words"`
	DueOn    string `json:"due_on,omitempty" jsonschema:"The day that the todo is due, as YYYY-MM-DD in India Standard Time. Leave it out when the user gave no date."`
	Project  string `json:"project,omitempty" jsonschema:"The project of the todo, such as lifeos. Leave it out when it is not clear."`
	Dictated bool   `json:"dictated,omitempty" jsonschema:"true only when the user stated this todo directly. false when you propose it from a journal entry, a document or a different source; the todo then waits in the review queue."`
}

func registerTodos(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "todo",
		Description: "Add one todo. For a request that has more than one action, call this tool one time for each action. To change a todo or to mark it done, use update with the type todo. Returns the stored todo with its ID.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in todoInput) (*sdk.CallToolResult, service.Todo, error) {
		todo, err := svc.AddTodo(ctx, service.AddTodoParams{
			Title:      in.Title,
			DueOn:      in.DueOn,
			Project:    in.Project,
			Provenance: service.Provenance{Source: service.SourceAgent, Verified: in.Dictated},
		})
		if err != nil {
			return nil, service.Todo{}, toToolError(err)
		}
		return nil, todo, nil
	})
}
```

In `internal/mcp/server.go`, replace this:

```go
	registerJournal(server, svc)
```

With this:

```go
	registerJournal(server, svc)
	registerTodos(server, svc)
```

- [ ] **Step 9: Tell the model about the new type in `get`, `update` and `delete`**

In `internal/mcp/records.go`, replace each occurrence of this (three places):

```go
Record type. One of: item, journal"
```

With this:

```go
Record type. One of: item, journal, todo"
```

In `internal/mcp/records.go`, replace this:

```go
For a journal: body, entry_date. Leave out a field
```

With this:

```go
For a journal: body, entry_date. For a todo: title, status (open, done or dropped), due_on, project. Leave out a field
```

In `internal/mcp/records.go`, replace this:

```go
For an item: rating, occurred_on. A journal has none."`
```

With this:

```go
For an item: rating, occurred_on. For a todo: due_on, project. A journal has none."`
```

- [ ] **Step 10: Run the MCP tests to verify that they pass**

Run:
```bash
make test PKG=./internal/mcp
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/mcp`.

- [ ] **Step 11: Commit**

Run:
```bash
git add internal/mcp/todos.go internal/mcp/todos_test.go internal/mcp/server.go internal/mcp/records.go
git commit -m "feat(mcp): add the todo tool"
```

---

### Task 9: Admin page for todos

**Files:**
- Create: `internal/admin/todos.templ`, `internal/admin/todos.go`
- Modify: `internal/admin/admin.go` (six routes), `internal/admin/items.templ` (one link), `internal/admin/review.templ` (one case)
- Test: `internal/admin/todos_test.go`

**Interfaces:**
- Consumes (Tasks 6 and 7): `service.AddTodo`, `GetTodo`, `ListTodos`, `UpdateTodo`, `DeleteTodo`, `service.Todo`, `service.TodoStatuses`, `service.UpdateTodoParams`. Consumes (Task 5): `pathID`, `render`, `listFailure`, `internalError`, `page`, `formatVerified`, the `switch` in `review.templ`, the test helpers `countEvents`, `get`, `postForm`, `newTestAdmin`.
- Produces (package `admin`):
  - The routes `GET /admin/todos`, `POST /admin/todos`, `POST /admin/todos/{id}/done`, `GET /admin/todos/{id}/edit`, `POST /admin/todos/{id}`, `POST /admin/todos/{id}/delete`
  - `todoPath(id int64) string`
  - Test helper in package `admin_test`: `addStoredTodo(t, pool, title string, verified bool) service.Todo`

- [ ] **Step 1: Write the failing tests**

Create `internal/admin/todos_test.go`:

```go
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

// addStoredTodo stores one todo through the service and returns it.
// verified false gives a todo as an agent proposes it.
func addStoredTodo(t *testing.T, pool *pgxpool.Pool, title string, verified bool) service.Todo {
	t.Helper()
	source := service.SourceManual
	if !verified {
		source = service.SourceAgent
	}
	todo, err := service.New(pool).AddTodo(t.Context(), service.AddTodoParams{
		Title:      title,
		DueOn:      "2026-10-20",
		Project:    "lifeos",
		Provenance: service.Provenance{Source: source, Verified: verified},
	})
	if err != nil {
		t.Fatalf("add %q: %v", title, err)
	}
	return todo
}

func TestTodosPageShowsTheFormAndAnEmptyList(t *testing.T) {
	h, _ := newTestAdmin(t)

	rec := get(h, "/admin/todos")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`hx-post="/admin/todos"`, `name="title"`, `name="project"`, `name="due_on"`, "No todos yet.", `href="/admin/todos"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %q", want)
		}
	}
}

func TestAddTodoStoresTheRowAndShowsIt(t *testing.T) {
	h, pool := newTestAdmin(t)

	rec := postForm(h, "/admin/todos", url.Values{"title": {"Buy a <b>domain</b>"}, "project": {"lifeos"}, "due_on": {""}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("HX-Trigger"); got != "todo-added" {
		t.Errorf("HX-Trigger = %q, want todo-added so that the form resets", got)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(body), `<div id="list">`) {
		t.Errorf("the response is not the #list fragment:\n%s", body)
	}
	for _, want := range []string{"Buy a &lt;b&gt;domain&lt;/b&gt;", "lifeos", "open", "manual", "yes"} {
		if !strings.Contains(body, want) {
			t.Errorf("the list does not contain %q", want)
		}
	}
	if strings.Contains(body, "<b>domain</b>") {
		t.Error("the list contains the title as raw HTML")
	}

	todos, err := service.New(pool).ListTodos(t.Context(), service.ListTodosParams{})
	if err != nil || len(todos) != 1 {
		t.Fatalf("todos = %v, %v, want one todo", todos, err)
	}
	if got := todos[0]; got.Source != service.SourceManual || !got.Verified || got.Status != "open" || got.DueOn != "" {
		t.Errorf("todo = %+v, want manual, verified, open, no date", got)
	}
}

func TestAddTodoShowsWhyTheServiceRefusedTheInput(t *testing.T) {
	h, pool := newTestAdmin(t)

	rec := postForm(h, "/admin/todos", url.Values{"title": {" "}})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200: htmx does not show a 4xx response", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "title: must not be empty") {
		t.Errorf("the response does not show the reason:\n%s", rec.Body)
	}
	if got := rec.Header().Get("HX-Trigger"); got != "" {
		t.Errorf("HX-Trigger = %q, want none so that the form keeps its input", got)
	}
	if n := countEvents(t, pool); n != 0 {
		t.Errorf("events = %d after refused input, want 0", n)
	}
}

// An open todo has a Done button. A todo that is done has none.
func TestMarkTodoDone(t *testing.T) {
	h, pool := newTestAdmin(t)
	todo := addStoredTodo(t, pool, "Buy a domain", true)
	path := fmt.Sprintf("/admin/todos/%d/done", todo.ID)
	if page := get(h, "/admin/todos").Body.String(); !strings.Contains(page, `hx-post="`+path+`"`) {
		t.Fatalf("the page has no Done button for the open todo:\n%s", page)
	}

	rec := postForm(h, path, nil)

	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(body), `<div id="list">`) {
		t.Fatalf("status = %d, want 200 with the #list fragment\n%s", rec.Code, body)
	}
	if strings.Contains(body, `hx-post="`+path+`"`) {
		t.Errorf("the list still has a Done button for the todo:\n%s", body)
	}
	got, err := service.New(pool).GetTodo(t.Context(), todo.ID)
	if err != nil || got.Status != "done" || got.DoneAt == nil || got.Title != "Buy a domain" || got.Project != "lifeos" || got.DueOn != "2026-10-20" {
		t.Errorf("todo = %+v, %v, want it done with its other fields unchanged", got, err)
	}

	// A second click, for example from a page that was open in two tabs.
	postForm(h, path, nil)
	if n := countEvents(t, pool); n != 2 {
		t.Errorf("events = %d, want 2 (create, one update)", n)
	}
	gone := postForm(h, "/admin/todos/9999/done", nil)
	if gone.Code != http.StatusOK || !strings.Contains(gone.Body.String(), "Todo 9999 is not in the list") {
		t.Errorf("unknown ID: status = %d, want 200 with a message\n%s", gone.Code, gone.Body)
	}
}

func TestEditTodoPageShowsTheStoredValuesAsText(t *testing.T) {
	h, pool := newTestAdmin(t)
	todo := addStoredTodo(t, pool, `Buy "a" <b>domain`, true)

	rec := get(h, fmt.Sprintf("/admin/todos/%d/edit", todo.ID))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		fmt.Sprintf(`action="/admin/todos/%d"`, todo.ID),
		`value="Buy &#34;a&#34; &lt;b&gt;domain"`,
		`<option value="open" selected>`,
		`value="lifeos"`,
		`value="2026-10-20"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %s", want)
		}
	}
	for _, path := range []string{"/admin/todos/9999/edit", "/admin/todos/abc/edit"} {
		if rec := get(h, path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, rec.Code)
		}
	}
}

func TestSaveTodoChangesTheTodoAndGoesBackToTheList(t *testing.T) {
	h, pool := newTestAdmin(t)
	todo := addStoredTodo(t, pool, "Buy a domain", false)

	// An empty project and an empty date remove the stored values.
	rec := postForm(h, fmt.Sprintf("/admin/todos/%d", todo.ID), url.Values{
		"title": {"Buy two domains"}, "status": {"dropped"}, "project": {""}, "due_on": {""},
	})

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/todos" {
		t.Fatalf("status = %d, Location = %q; want 303 to /admin/todos\n%s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	got, err := service.New(pool).GetTodo(t.Context(), todo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Buy two domains" || got.Status != "dropped" || got.Project != "" || got.DueOn != "" {
		t.Errorf("todo = %+v", got)
	}
	// An edit is not an approval: the todo stays in the review queue.
	if got.Verified || got.Source != service.SourceAgent {
		t.Errorf("verified, source = %v, %q; want no change", got.Verified, got.Source)
	}

	for _, path := range []string{"/admin/todos/9999", "/admin/todos/abc"} {
		if rec := postForm(h, path, url.Values{"title": {"x"}, "status": {"open"}}); rec.Code != http.StatusNotFound {
			t.Errorf("POST %s: status = %d, want 404", path, rec.Code)
		}
	}
}

func TestSaveTodoShowsWhyTheServiceRefusedTheInput(t *testing.T) {
	h, pool := newTestAdmin(t)
	todo := addStoredTodo(t, pool, "Buy a domain", true)

	rec := postForm(h, fmt.Sprintf("/admin/todos/%d", todo.ID), url.Values{
		"title": {"Typed title"}, "status": {"later"}, "project": {"typed project"}, "due_on": {"2026-11-01"},
	})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 with the form", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "status: must be one of open, done, dropped") {
		t.Errorf("the page does not show the reason:\n%s", body)
	}
	for _, want := range []string{`value="Typed title"`, `value="typed project"`, `value="2026-11-01"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the form lost the typed input %s", want)
		}
	}
	if got, _ := service.New(pool).GetTodo(t.Context(), todo.ID); got.Title != "Buy a domain" {
		t.Errorf("refused input changed the todo: %+v", got)
	}
}

// A click on Save with no change is not a write.
func TestSaveTodoWithNoChangeWritesNoEvent(t *testing.T) {
	h, pool := newTestAdmin(t)
	todo := addStoredTodo(t, pool, "Buy a domain", true)

	rec := postForm(h, fmt.Sprintf("/admin/todos/%d", todo.ID), url.Values{
		"title": {"Buy a domain"}, "status": {"open"}, "project": {"lifeos"}, "due_on": {"2026-10-20"},
	})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303\n%s", rec.Code, rec.Body)
	}
	if n := countEvents(t, pool); n != 1 {
		t.Errorf("events = %d, want 1 (the create)", n)
	}
}

func TestDeleteTodoRemovesTheTodoFromTheList(t *testing.T) {
	h, pool := newTestAdmin(t)
	addStoredTodo(t, pool, "Kept", true)
	gone := addStoredTodo(t, pool, "Gone", true)
	path := fmt.Sprintf("/admin/todos/%d/delete", gone.ID)
	if page := get(h, "/admin/todos").Body.String(); !strings.Contains(page, `hx-confirm="Delete Gone?"`) {
		t.Errorf("the Delete button does not ask for a confirmation:\n%s", page)
	}

	rec := postForm(h, path, nil)

	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(body), `<div id="list">`) {
		t.Fatalf("status = %d, want 200 with the #list fragment\n%s", rec.Code, body)
	}
	if strings.Contains(body, "Gone") || !strings.Contains(body, "Kept") {
		t.Errorf("the list must show Kept and not Gone:\n%s", body)
	}

	again := postForm(h, path, nil)
	if again.Code != http.StatusOK || !strings.Contains(again.Body.String(), fmt.Sprintf("Todo %d is not in the list", gone.ID)) {
		t.Errorf("second delete: status = %d, want 200 with a message\n%s", again.Code, again.Body)
	}
	if n := countEvents(t, pool); n != 3 {
		t.Errorf("events = %d, want 3 (two creates, one delete)", n)
	}
}

func TestDatabaseFailureOnTheTodoPages(t *testing.T) {
	h, pool := newTestAdmin(t)
	pool.Close()

	if rec := get(h, "/admin/todos"); rec.Code != http.StatusInternalServerError || strings.TrimSpace(rec.Body.String()) != "internal error" {
		t.Errorf("page: status = %d, body = %q; want 500 and only \"internal error\"", rec.Code, rec.Body)
	}
	clicks := map[string]url.Values{
		"/admin/todos":          {"title": {"x"}},
		"/admin/todos/1/done":   nil,
		"/admin/todos/1/delete": nil,
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

// A todo that an agent proposed waits in the review queue. Its Edit link
// must go to the todo.
func TestReviewPageLinksATodoToItsEditPage(t *testing.T) {
	h, pool := newTestAdmin(t)
	todo := addStoredTodo(t, pool, "Renew the insurance", false)

	body := get(h, "/admin/review").Body.String()

	for _, want := range []string{
		"todo: Renew the insurance, project lifeos, due 2026-10-20",
		`name="type" value="todo"`,
		fmt.Sprintf(`href="/admin/todos/%d/edit"`, todo.ID),
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %s", want)
		}
	}
	if strings.Contains(body, fmt.Sprintf(`href="/admin/items/%d/edit"`, todo.ID)) {
		t.Error("the Edit link of the todo goes to the item with the same ID")
	}

	rec := postForm(h, "/admin/review/verify", url.Values{"type": {"todo"}, "id": {fmt.Sprint(todo.ID)}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "No record waits for review.") {
		t.Errorf("after the approval the queue must be empty:\n%s", rec.Body)
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/admin ARGS="-run Todo"
```
Expected: the run fails. The output contains `--- FAIL: TestTodosPageShowsTheFormAndAnEmptyList` and `--- FAIL: TestReviewPageLinksATodoToItsEditPage`.

- [ ] **Step 3: Write the templates**

An open todo has a Done button. The edit form has the status as a list, so a todo that is done can become open again.

Create `internal/admin/todos.templ`:

```templ
package admin

import (
	"strconv"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

templ todosPage(todos []service.Todo) {
	@page("Todos") {
		<h1>Todos</h1>
		<form hx-post="/admin/todos" hx-target="#list" hx-swap="outerHTML" hx-on:todo-added="this.reset()">
			<label>
				Title
				<input type="text" name="title" required maxlength="300" size="40"/>
			</label>
			<label>
				Project
				<input type="text" name="project" maxlength="100" size="15"/>
			</label>
			<label>
				Due
				<input type="date" name="due_on"/>
			</label>
			<button type="submit">Add</button>
		</form>
		@todosList(todos, "")
	}
}

// todosList is the part of the page that a form post replaces.
templ todosList(todos []service.Todo, errorMessage string) {
	<div id="list">
		if errorMessage != "" {
			<p class="error" role="alert">{ errorMessage }</p>
		}
		if len(todos) == 0 {
			<p class="muted">No todos yet.</p>
		} else {
			<table>
				<thead>
					<tr>
						<th>ID</th>
						<th>Title</th>
						<th>Status</th>
						<th>Project</th>
						<th>Due</th>
						<th>Source</th>
						<th>Verified</th>
						<th>Added (IST)</th>
						<th></th>
					</tr>
				</thead>
				<tbody>
					for _, todo := range todos {
						<tr>
							<td>{ strconv.FormatInt(todo.ID, 10) }</td>
							<td>{ todo.Title }</td>
							<td>{ todo.Status }</td>
							<td>{ todo.Project }</td>
							<td>{ todo.DueOn }</td>
							<td>{ string(todo.Source) }</td>
							<td>{ formatVerified(todo.Verified) }</td>
							<td>{ todo.CreatedAt.In(service.IST).Format("2 Jan 2006 15:04") }</td>
							<td>
								if todo.Status == "open" {
									<button hx-post={ todoPath(todo.ID) + "/done" } hx-target="#list" hx-swap="outerHTML">Done</button>
								}
								<a href={ templ.URL(todoPath(todo.ID) + "/edit") }>Edit</a>
								<button
									hx-post={ todoPath(todo.ID) + "/delete" }
									hx-target="#list"
									hx-swap="outerHTML"
									hx-confirm={ "Delete " + todo.Title + "?" }
								>Delete</button>
							</td>
						</tr>
					}
				</tbody>
			</table>
		}
	</div>
}

// todoForm holds the values of the edit form as text. When the service
// refuses a value, the form shows again what Gautam typed.
type todoForm struct {
	ID      int64
	Title   string
	Status  string
	Project string
	DueOn   string
}

templ editTodoPage(form todoForm, errorMessage string) {
	@page("Edit todo") {
		<h1>Edit todo { strconv.FormatInt(form.ID, 10) }</h1>
		if errorMessage != "" {
			<p class="error" role="alert">{ errorMessage }</p>
		}
		<form method="post" action={ templ.URL(todoPath(form.ID)) }>
			<label>
				Title
				<input type="text" name="title" required maxlength="300" size="40" value={ form.Title }/>
			</label>
			<label>
				Status
				<select name="status" required>
					for _, status := range service.TodoStatuses {
						<option value={ status } selected?={ status == form.Status }>{ status }</option>
					}
				</select>
			</label>
			<label>
				Project
				<input type="text" name="project" maxlength="100" size="15" value={ form.Project }/>
			</label>
			<label>
				Due
				<input type="date" name="due_on" value={ form.DueOn }/>
			</label>
			<button type="submit">Save</button>
			<a href="/admin/todos">Cancel</a>
		</form>
	}
}

func todoPath(id int64) string {
	return "/admin/todos/" + strconv.FormatInt(id, 10)
}
```

- [ ] **Step 4: Write the handlers**

Create `internal/admin/todos.go`:

```go
package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

func (h *handlers) todosPage(w http.ResponseWriter, r *http.Request) {
	todos, err := h.svc.ListTodos(r.Context(), service.ListTodosParams{})
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, todosPage(todos))
}

// showTodosList renders the list again, with message above it.
func (h *handlers) showTodosList(w http.ResponseWriter, r *http.Request, message string) {
	todos, err := h.svc.ListTodos(r.Context(), service.ListTodosParams{})
	if err != nil {
		listFailure(w, r, err, "The list did not load: internal error. Reload the page.")
		return
	}
	render(w, r, todosList(todos, message))
}

// addTodo stores the todo from the form and renders the list again. A todo
// from the admin pages is one that Gautam typed, so it is manual and
// verified.
func (h *handlers) addTodo(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	message := ""
	_, err := h.svc.AddTodo(r.Context(), service.AddTodoParams{
		Title:      r.PostForm.Get("title"),
		Project:    r.PostForm.Get("project"),
		DueOn:      r.PostForm.Get("due_on"),
		Provenance: service.Provenance{Source: service.SourceManual, Verified: true},
	})
	var invalid *service.ValidationError
	switch {
	case err == nil:
		// htmx turns this header into an event; the form resets on it.
		w.Header().Set("HX-Trigger", "todo-added")
	case errors.As(err, &invalid):
		message = invalid.Error()
	default:
		listFailure(w, r, err, "The todo was not stored: internal error. Try again.")
		return
	}
	h.showTodosList(w, r, message)
}

// markTodoDone sets the status of one todo to done and renders the list
// again.
func (h *handlers) markTodoDone(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	done := "done"
	message := ""
	_, err := h.svc.UpdateTodo(r.Context(), id, service.UpdateTodoParams{Status: &done, Actor: service.SourceManual})
	switch {
	case err == nil:
	case errors.Is(err, service.ErrNotFound):
		message = fmt.Sprintf("Todo %d is not in the list. It was possibly deleted before.", id)
	default:
		listFailure(w, r, err, "The todo was not changed: internal error. Try again.")
		return
	}
	h.showTodosList(w, r, message)
}

func (h *handlers) editTodoPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	todo, err := h.svc.GetTodo(r.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, editTodoPage(todoForm{
		ID:      todo.ID,
		Title:   todo.Title,
		Status:  todo.Status,
		Project: todo.Project,
		DueOn:   todo.DueOn,
	}, ""))
}

// saveTodo stores the edit form and goes back to the list. When the
// service refuses a value, the form shows again with the reason. An empty
// project or date removes the stored value.
func (h *handlers) saveTodo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	form := todoForm{
		ID:      id,
		Title:   r.PostForm.Get("title"),
		Status:  r.PostForm.Get("status"),
		Project: r.PostForm.Get("project"),
		DueOn:   r.PostForm.Get("due_on"),
	}
	_, err := h.svc.UpdateTodo(r.Context(), id, form.params())
	var invalid *service.ValidationError
	switch {
	case err == nil:
		http.Redirect(w, r, "/admin/todos", http.StatusSeeOther)
	case errors.As(err, &invalid):
		render(w, r, editTodoPage(form, invalid.Error()))
	case errors.Is(err, service.ErrNotFound):
		http.NotFound(w, r)
	default:
		internalError(w, err)
	}
}

// params turns the form into the input of UpdateTodo. A change in the
// admin pages is one that Gautam typed.
func (f todoForm) params() service.UpdateTodoParams {
	params := service.UpdateTodoParams{Title: &f.Title, Status: &f.Status, Actor: service.SourceManual}
	if strings.TrimSpace(f.Project) == "" {
		params.Clear = append(params.Clear, "project")
	} else {
		params.Project = &f.Project
	}
	if f.DueOn == "" {
		params.Clear = append(params.Clear, "due_on")
	} else {
		params.DueOn = &f.DueOn
	}
	return params
}

// deleteTodo deletes one todo and renders the list again.
func (h *handlers) deleteTodo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	message := ""
	err := h.svc.DeleteTodo(r.Context(), id, service.DeleteParams{Actor: service.SourceManual})
	switch {
	case err == nil:
	case errors.Is(err, service.ErrNotFound):
		message = fmt.Sprintf("Todo %d is not in the list. It was possibly deleted before.", id)
	default:
		listFailure(w, r, err, "The todo was not deleted: internal error. Try again.")
		return
	}
	h.showTodosList(w, r, message)
}
```

- [ ] **Step 5: Add the routes, the link and the case of the review page**

In `internal/admin/admin.go`, replace this:

```go
	router.Get("/admin/review", h.reviewPage)
```

With this:

```go
	router.Get("/admin/todos", h.todosPage)
	router.Post("/admin/todos", h.addTodo)
	router.Post("/admin/todos/{id}/done", h.markTodoDone)
	router.Get("/admin/todos/{id}/edit", h.editTodoPage)
	router.Post("/admin/todos/{id}", h.saveTodo)
	router.Post("/admin/todos/{id}/delete", h.deleteTodo)
	router.Get("/admin/review", h.reviewPage)
```

In `internal/admin/items.templ`, replace this:

```templ
				<a href="/admin/journal">Journal</a>
```

With this:

```templ
				<a href="/admin/journal">Journal</a>
				<a href="/admin/todos">Todos</a>
```

In `internal/admin/review.templ`, replace this:

```templ
									case "journal":
										<a href={ templ.URL(journalPath(entry.ID) + "/edit") }>Edit</a>
```

With this:

```templ
									case "journal":
										<a href={ templ.URL(journalPath(entry.ID) + "/edit") }>Edit</a>
									case "todo":
										<a href={ templ.URL(todoPath(entry.ID) + "/edit") }>Edit</a>
```

- [ ] **Step 6: Run the admin tests to verify that they pass**

Run:
```bash
make test PKG=./internal/admin
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/admin`.

- [ ] **Step 7: Commit**

Run:
```bash
git add internal/admin/todos.templ internal/admin/todos.go internal/admin/todos_test.go internal/admin/admin.go internal/admin/items.templ internal/admin/review.templ
git commit -m "feat(admin): add the todos page"
```

---


### Task 10: Service: habits

**Files:**
- Create: `queries/habits.sql`, `internal/service/habits.go`
- Test: `internal/service/habits_test.go`

**Interfaces:**
- Consumes (Task 2): `checkText`. Consumes (plans 1 and 2): `Provenance`, `invalid`, `recordEvent`, `ErrNotFound`, the test helpers `newTestService`, `ptr`, `countRows`, `equal`, `allEvents`. Consumes (Task 1): the unique index `habits_name_idx`.
- Produces (package `service`):
  - `type Habit struct{ ID int64; Name string; Active bool; Source Source; SourceRef *int64; Verified bool; CreatedAt, UpdatedAt time.Time }` with the JSON names `id`, `name`, `active`, `source`, `source_ref`, `verified`, `created_at`, `updated_at`
  - `type AddHabitParams struct{ Name string; Provenance }`, `(*Service).AddHabit(ctx, AddHabitParams) (Habit, error)`
  - `(*Service).ListHabits(ctx) ([]Habit, error)`: every habit, active or not, by name
  - `type UpdateHabitParams struct{ Name *string; Active *bool; Actor Source }`, `(*Service).UpdateHabit(ctx, id int64, p UpdateHabitParams) (Habit, error)`
  - `habitName(name string) string`: the name in lower case with one space between words; `habitFromRow(store.Habit) Habit`
  - Test helpers: `addHabit(t, s, name string) Habit` (manual, verified), `habitNames([]Habit) []string`
  - sqlc: `InsertHabit`, `ListHabits`, `GetHabitForUpdate`, `UpdateHabit`; the type `store.Habit`

A habit has no delete function and is not in `RecordTypes`. See "Decisions in this plan".

- [ ] **Step 1: Write the failing tests**

Create `internal/service/habits_test.go`:

```go
package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// addHabit stores one habit as the admin page does.
func addHabit(t *testing.T, s *Service, name string) Habit {
	t.Helper()
	habit, err := s.AddHabit(context.Background(), AddHabitParams{Name: name, Provenance: Provenance{Source: SourceManual, Verified: true}})
	if err != nil {
		t.Fatalf("AddHabit(%q): %v", name, err)
	}
	return habit
}

func habitNames(habits []Habit) []string {
	out := make([]string, len(habits))
	for i, h := range habits {
		out[i] = h.Name
	}
	return out
}

// A habit is logged by its name, from a form, from Siri and from an agent.
// One spelling in the table makes "Gym", "gym " and "GYM" the same habit.
func TestAddHabitStoresTheNameInLowerCase(t *testing.T) {
	s, pool := newTestService(t)

	got, err := s.AddHabit(context.Background(), AddHabitParams{Name: "  Morning   Walk ", Provenance: Provenance{Source: SourceManual, Verified: true}})
	if err != nil {
		t.Fatalf("AddHabit: %v", err)
	}

	if got.ID == 0 || got.Name != "morning walk" || !got.Active {
		t.Errorf("habit = %+v, want the name morning walk, active", got)
	}
	if got.Source != SourceManual || !got.Verified {
		t.Errorf("source, verified = %q, %v", got.Source, got.Verified)
	}
	events := allEvents(t, pool)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if e := events[0]; e.Action != "create" || e.RecordType != "habits" || e.RecordID != got.ID || e.Source != "manual" || e.New["name"] != "morning walk" {
		t.Errorf("event = %+v, want a create of the habit", e)
	}
}

func TestAddHabitRejectsBadInput(t *testing.T) {
	s, pool := newTestService(t)
	addHabit(t, s, "gym")
	tests := []struct {
		name      string
		habit     string
		source    Source
		wantField string
	}{
		{"empty name", "", SourceManual, "name"},
		{"name of spaces", " \t", SourceManual, "name"},
		{"name too long", strings.Repeat("a", 51), SourceManual, "name"},
		{"name with a NUL character", "a\x00b", SourceManual, "name"},
		{"name that exists", "gym", SourceManual, "name"},
		{"name that exists in a different case", " GYM", SourceManual, "name"},
		{"unknown source", "read", "robot", "source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.AddHabit(context.Background(), AddHabitParams{Name: tt.habit, Provenance: Provenance{Source: tt.source, Verified: true}})
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
	if n := countRows(t, pool, "habits"); n != 1 {
		t.Errorf("habits has %d rows after refused input, want 1", n)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d after refused input, want 1", n)
	}
}

func TestListHabitsReturnsEveryHabitByName(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	empty, err := s.ListHabits(ctx)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("ListHabits with no habit = %#v, %v, want an empty non-nil slice", empty, err)
	}

	addHabit(t, s, "read")
	gym := addHabit(t, s, "gym")
	addHabit(t, s, "meditate")
	if _, err := s.UpdateHabit(ctx, gym.ID, UpdateHabitParams{Active: ptr(false), Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	habits, err := s.ListHabits(ctx)
	if err != nil {
		t.Fatalf("ListHabits: %v", err)
	}
	if want := []string{"gym", "meditate", "read"}; !equal(habitNames(habits), want) {
		t.Errorf("names = %v, want %v", habitNames(habits), want)
	}
	if habits[0].Active || !habits[1].Active {
		t.Errorf("active = %v, %v, want false for gym and true for meditate", habits[0].Active, habits[1].Active)
	}
}

func TestUpdateHabitRenamesAndRetiresAHabit(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added := addHabit(t, s, "gymm")

	renamed, err := s.UpdateHabit(ctx, added.ID, UpdateHabitParams{Name: ptr(" Gym "), Actor: SourceManual})
	if err != nil || renamed.Name != "gym" || !renamed.Active {
		t.Fatalf("rename = %+v, %v, want the name gym, active", renamed, err)
	}
	retired, err := s.UpdateHabit(ctx, added.ID, UpdateHabitParams{Active: ptr(false), Actor: SourceManual})
	if err != nil || retired.Active || retired.Name != "gym" {
		t.Fatalf("retire = %+v, %v, want the name gym, not active", retired, err)
	}

	events := allEvents(t, pool)
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3 (create, two updates)", len(events))
	}
	if e := events[1]; e.Action != "update" || e.RecordType != "habits" || e.Source != "manual" || e.Old["name"] != "gymm" || e.New["name"] != "gym" {
		t.Errorf("event = %+v, want an update with the old and the new name", e)
	}

	// The same values again are not a write.
	if _, err := s.UpdateHabit(ctx, added.ID, UpdateHabitParams{Name: ptr("GYM"), Active: ptr(false), Actor: SourceManual}); err != nil {
		t.Errorf("update with no change: %v", err)
	}
	if n := countRows(t, pool, "events"); n != 3 {
		t.Errorf("events = %d after an update with no change, want 3", n)
	}
}

func TestUpdateHabitRejectsBadInput(t *testing.T) {
	s, pool := newTestService(t)
	gym := addHabit(t, s, "gym")
	addHabit(t, s, "read")
	tests := []struct {
		name      string
		params    UpdateHabitParams
		wantField string
	}{
		{"no field", UpdateHabitParams{Actor: SourceManual}, "fields"},
		{"empty name", UpdateHabitParams{Name: ptr(" "), Actor: SourceManual}, "name"},
		{"name of a different habit", UpdateHabitParams{Name: ptr("Read"), Actor: SourceManual}, "name"},
		{"unknown actor", UpdateHabitParams{Name: ptr("run"), Actor: "robot"}, "source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.UpdateHabit(context.Background(), gym.ID, tt.params)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
	if _, err := s.UpdateHabit(context.Background(), 9999, UpdateHabitParams{Name: ptr("run"), Actor: SourceManual}); !errors.Is(err, ErrNotFound) {
		t.Errorf("absent habit: error = %v, want ErrNotFound", err)
	}
	if n := countRows(t, pool, "events"); n != 2 {
		t.Errorf("events = %d, want 2 (the two creates)", n)
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run Habit"
```
Expected: the run fails. The output contains `undefined: Habit`.

- [ ] **Step 3: Write the queries**

Create `queries/habits.sql`:

```sql
-- name: InsertHabit :one
INSERT INTO habits (name, source, source_ref, verified)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListHabits :many
SELECT * FROM habits
WHERE deleted_at IS NULL
ORDER BY name;

-- name: GetHabitForUpdate :one
SELECT * FROM habits
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- name: UpdateHabit :one
UPDATE habits
SET name = $2, active = $3
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;
```

- [ ] **Step 4: Write the habit functions**

The unique index of Task 1 refuses a second habit with the same name, also when two requests come at the same moment. The service turns that database error (code `23505`) into a `ValidationError` on `name`.

Create `internal/service/habits.go`:

```go
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

const maxHabitNameLength = 50

// Habit is one row of habits: the definition of a habit.
type Habit struct {
	ID int64 `json:"id"`
	// Name is in lower case. No two habits have the same name.
	Name string `json:"name"`
	// Active is false for a habit that Gautam does not follow now. Its
	// logs stay.
	Active    bool      `json:"active"`
	Source    Source    `json:"source"`
	SourceRef *int64    `json:"source_ref,omitempty"`
	Verified  bool      `json:"verified"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AddHabitParams is the input of AddHabit.
type AddHabitParams struct {
	Name string
	Provenance
}

// habitName returns name as the table holds it: in lower case, with one
// space between words. "Gym", "gym " and "GYM" are one habit.
func habitName(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), " ")
}

func checkHabitName(field, name string) error {
	if name == "" {
		return invalid(field, "must not be empty")
	}
	if utf8.RuneCountInString(name) > maxHabitNameLength {
		return invalid(field, fmt.Sprintf("must be at most %d characters", maxHabitNameLength))
	}
	return checkText(field, name)
}

// habitNameTaken reports whether err comes from the unique index on the
// habit names. The index is what stops two requests at the same moment.
func habitNameTaken(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

const habitNameTakenMessage = "a habit has this name already; if that habit is not active, make it active again"

// AddHabit validates p, stores one active habit and appends a create
// event, in one transaction.
func (s *Service) AddHabit(ctx context.Context, p AddHabitParams) (Habit, error) {
	name := habitName(p.Name)
	if err := checkHabitName("name", name); err != nil {
		return Habit{}, err
	}
	if err := p.Provenance.validate(); err != nil {
		return Habit{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Habit{}, fmt.Errorf("add habit: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	stored, err := q.InsertHabit(ctx, store.InsertHabitParams{
		Name:      name,
		Source:    string(p.Source),
		SourceRef: p.SourceRef,
		Verified:  p.Verified,
	})
	if habitNameTaken(err) {
		return Habit{}, invalid("name", habitNameTakenMessage)
	}
	if err != nil {
		return Habit{}, fmt.Errorf("add habit: %w", err)
	}
	habit := habitFromRow(stored)
	if err := recordEvent(ctx, q, "create", "habits", habit.ID, p.Source, nil, habit); err != nil {
		return Habit{}, fmt.Errorf("add habit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Habit{}, fmt.Errorf("add habit: %w", err)
	}
	return habit, nil
}

func habitFromRow(row store.Habit) Habit {
	return Habit{
		ID:        row.ID,
		Name:      row.Name,
		Active:    row.Active,
		Source:    Source(row.Source),
		SourceRef: row.SourceRef,
		Verified:  row.Verified,
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
}

// ListHabits returns every habit, active or not, by name.
func (s *Service) ListHabits(ctx context.Context) ([]Habit, error) {
	rows, err := store.New(s.pool).ListHabits(ctx)
	if err != nil {
		return nil, fmt.Errorf("list habits: %w", err)
	}
	habits := make([]Habit, 0, len(rows))
	for _, row := range rows {
		habits = append(habits, habitFromRow(row))
	}
	return habits, nil
}

// UpdateHabitParams is the input of UpdateHabit. A field that is nil keeps
// its stored value.
type UpdateHabitParams struct {
	Name   *string
	Active *bool
	// Actor is who makes the change. It goes into the event.
	Actor Source
}

// UpdateHabit renames one habit, or makes it active or not active, and
// appends an update event that holds the old row, in one transaction. A
// call that changes no value writes nothing and appends no event.
func (s *Service) UpdateHabit(ctx context.Context, id int64, p UpdateHabitParams) (Habit, error) {
	if err := (Provenance{Source: p.Actor}).validate(); err != nil {
		return Habit{}, err
	}
	if p.Name == nil && p.Active == nil {
		return Habit{}, invalid("fields", "must name at least one field to change")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Habit{}, fmt.Errorf("update habit: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	current, err := q.GetHabitForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Habit{}, ErrNotFound
	}
	if err != nil {
		return Habit{}, fmt.Errorf("update habit: %w", err)
	}
	old := habitFromRow(current)

	name, active := old.Name, old.Active
	if p.Name != nil {
		name = habitName(*p.Name)
		if err := checkHabitName("name", name); err != nil {
			return Habit{}, err
		}
	}
	if p.Active != nil {
		active = *p.Active
	}
	if name == old.Name && active == old.Active {
		return old, nil
	}
	stored, err := q.UpdateHabit(ctx, store.UpdateHabitParams{ID: id, Name: name, Active: active})
	if habitNameTaken(err) {
		return Habit{}, invalid("name", habitNameTakenMessage)
	}
	if err != nil {
		return Habit{}, fmt.Errorf("update habit: %w", err)
	}
	habit := habitFromRow(stored)
	if err := recordEvent(ctx, q, "update", "habits", id, p.Actor, old, habit); err != nil {
		return Habit{}, fmt.Errorf("update habit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Habit{}, fmt.Errorf("update habit: %w", err)
	}
	return habit, nil
}
```

- [ ] **Step 5: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./internal/service ARGS="-run Habit"
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/service`.

- [ ] **Step 6: Commit**

Run:
```bash
git add queries/habits.sql internal/service/habits.go internal/service/habits_test.go
git commit -m "feat(service): add, list and change habits"
```

---

### Task 11: Service: habit logs

**Files:**
- Create: `queries/habit_logs.sql`, `internal/service/habit_logs.go`, `internal/service/habit_logs_change.go`
- Modify: `internal/service/records.go` (one case in each function), `internal/service/review.go` (two lines and one case)
- Test: `internal/service/habit_logs_test.go`, `internal/service/habit_logs_change_test.go`

**Interfaces:**
- Consumes (Task 10): `Habit`, `habitName`, `AddHabit`, `UpdateHabit`, `UpdateHabitParams`, `addHabit`; the sqlc query `ListHabits`. Consumes (Task 2): `pastDay`, `today`, `checkLimit`. Consumes (Task 3): `deleteRecord`, `asRecord`, `RecordTypes`, the list of functions in `ReviewQueue`, `verifyRecord`, `addUnverified`. Consumes (Task 1): the unique index `habit_logs_habit_day_idx`.
- Produces (package `service`):
  - `type HabitLog struct{ ID, HabitID int64; Habit, LoggedOn string; Source Source; SourceRef *int64; Verified bool; CreatedAt, UpdatedAt time.Time }` with the JSON names `id`, `habit_id`, `habit`, `logged_on`, `source`, `source_ref`, `verified`, `created_at`, `updated_at`. `Habit` is the name of the habit.
  - `type LogHabitParams struct{ Habit, LoggedOn string; Provenance }`, `(*Service).LogHabit(ctx, LogHabitParams) (HabitLog, error)`
  - `(*Service).GetHabitLog(ctx, id int64) (HabitLog, error)`, `(*Service).ListHabitLogs(ctx, limit int) ([]HabitLog, error)`, `(*Service).DeleteHabitLog(ctx, id int64, p DeleteParams) error`
  - `type HabitStatus struct{ ID int64; Name string; Done bool }` and `type HabitChecklist struct{ Date string; Habits []HabitStatus }` with the JSON names `id`, `name`, `done`, `date`, `habits`; `(*Service).HabitsToday(ctx) (HabitChecklist, error)`
  - `RecordTypes` is `{"item", "journal", "todo", "habit_log"}`. `Update` refuses the type `habit_log` with a `ValidationError` on `type`.
  - `habitLogFromRow(row store.HabitLog, habit string) HabitLog`, `HabitLog.summary() string`, `unverifiedHabitLogs`, `verifyHabitLog`
  - Test helpers: `logHabit(t, s, habit, day string) HabitLog` (manual, verified), `inferHabitLog(t, s, habit, day string) HabitLog` (agent, not verified)
  - sqlc: `GetActiveHabitByName`, `InsertHabitLog`, `GetHabitLogOfDayForUpdate`, `VerifyHabitLog`, `GetHabitLog`, `ListHabitLogs`, `ListHabitsOnDay`, `GetHabitLogForUpdate`, `SoftDeleteHabitLog`, `ListUnverifiedHabitLogs`. A query that joins `habits` returns a row with the fields `HabitLog store.HabitLog` and `Habit string`.

- [ ] **Step 1: Write the failing tests for logs and the checklist**

Create `internal/service/habit_logs_test.go`:

```go
package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// logHabit logs a habit as a Shortcut or the admin page does: manual and
// verified. An empty day means today in IST.
func logHabit(t *testing.T, s *Service, habit, day string) HabitLog {
	t.Helper()
	log, err := s.LogHabit(context.Background(), LogHabitParams{
		Habit: habit, LoggedOn: day, Provenance: Provenance{Source: SourceManual, Verified: true},
	})
	if err != nil {
		t.Fatalf("LogHabit(%q, %q): %v", habit, day, err)
	}
	return log
}

// inferHabitLog logs a habit as an agent does when it reads "went to the
// gym" in a source: not verified.
func inferHabitLog(t *testing.T, s *Service, habit, day string) HabitLog {
	t.Helper()
	log, err := s.LogHabit(context.Background(), LogHabitParams{
		Habit: habit, LoggedOn: day, Provenance: Provenance{Source: SourceAgent},
	})
	if err != nil {
		t.Fatalf("LogHabit(%q, %q): %v", habit, day, err)
	}
	return log
}

func TestLogHabitStoresALogForTodayInIST(t *testing.T) {
	s, pool := newTestService(t) // the clock is 2026-10-10 20:00 UTC = 2026-10-11 01:30 IST
	gym := addHabit(t, s, "gym")

	// Siri hears "Gym". The name in the table is gym.
	got := logHabit(t, s, " Gym ", "")

	if got.ID == 0 || got.HabitID != gym.ID || got.Habit != "gym" || got.LoggedOn != "2026-10-11" {
		t.Errorf("log = %+v, want the habit gym on 2026-10-11", got)
	}
	if got.Source != SourceManual || !got.Verified {
		t.Errorf("source, verified = %q, %v", got.Source, got.Verified)
	}
	events := allEvents(t, pool)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (the habit, the log)", len(events))
	}
	if e := events[1]; e.Action != "create" || e.RecordType != "habit_logs" || e.RecordID != got.ID || e.Source != "manual" || e.New["habit"] != "gym" || e.New["logged_on"] != "2026-10-11" {
		t.Errorf("event = %+v, want a create of the log", e)
	}
}

func TestLogHabitAcceptsADayInThePastAndRefusesTomorrow(t *testing.T) {
	s, _ := newTestService(t)
	addHabit(t, s, "gym")

	if got := logHabit(t, s, "gym", "2026-10-09"); got.LoggedOn != "2026-10-09" {
		t.Errorf("logged_on = %q, want 2026-10-09", got.LoggedOn)
	}
	_, err := s.LogHabit(context.Background(), LogHabitParams{Habit: "gym", LoggedOn: "2026-10-12", Provenance: Provenance{Source: SourceManual, Verified: true}})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "logged_on" {
		t.Errorf("log for tomorrow in IST: error = %v, want a ValidationError on logged_on", err)
	}
}

// "Log gym" two times on one day is one visit to the gym. The second call
// must not fail, must not store a second row and must not append an event.
func TestLogHabitASecondTimeChangesNothing(t *testing.T) {
	s, pool := newTestService(t)
	addHabit(t, s, "gym")
	first := logHabit(t, s, "gym", "")

	second := logHabit(t, s, "gym", "2026-10-11")

	if second != first {
		t.Errorf("second log = %+v, want the first log %+v", second, first)
	}
	if n := countRows(t, pool, "habit_logs"); n != 1 {
		t.Errorf("habit_logs has %d rows, want 1", n)
	}
	if n := countRows(t, pool, "events"); n != 2 {
		t.Errorf("events = %d, want 2 (the habit, one log)", n)
	}
	if other := logHabit(t, s, "gym", "2026-10-10"); other.ID == first.ID {
		t.Error("a log for a different day returned the first log")
	}
}

// An agent reads "went to the gym" in the journal and proposes a log. Then
// Gautam says "log gym". His own log must count: it approves the proposal.
// The other order must change nothing: a guess does not touch his log.
func TestLogHabitByGautamApprovesALogThatAnAgentInferred(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	addHabit(t, s, "gym")
	addHabit(t, s, "read")
	proposed := inferHabitLog(t, s, "gym", "")
	if proposed.Verified {
		t.Fatal("the inferred log is verified")
	}

	got := logHabit(t, s, "gym", "")

	if got.ID != proposed.ID || !got.Verified || got.Source != SourceAgent {
		t.Errorf("log = %+v, want the log %d, verified, with its source unchanged", got, proposed.ID)
	}
	events := allEvents(t, pool)
	last := events[len(events)-1]
	if last.Action != "verify" || last.RecordType != "habit_logs" || last.RecordID != proposed.ID || last.Source != "manual" || last.Old["verified"] != false || last.New["verified"] != true {
		t.Errorf("last event = %+v, want a verify of the log by manual", last)
	}

	stated := logHabit(t, s, "read", "")
	before := countRows(t, pool, "events")
	again := inferHabitLog(t, s, "read", "")
	if again != stated {
		t.Errorf("inferred log over a verified log = %+v, want %+v unchanged", again, stated)
	}
	if after := countRows(t, pool, "events"); after != before {
		t.Errorf("events = %d, want %d", after, before)
	}
	if stored, err := s.GetHabitLog(ctx, stated.ID); err != nil || !stored.Verified {
		t.Errorf("stored = %+v, %v, want it verified", stored, err)
	}
}

func TestLogHabitRejectsBadInput(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	manual := Provenance{Source: SourceManual, Verified: true}

	_, err := s.LogHabit(ctx, LogHabitParams{Habit: "gym", Provenance: manual})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "habit" || !strings.Contains(invalid.Message, "there is no active habit") {
		t.Errorf("no habit in the table: error = %v, want a ValidationError on habit that says so", err)
	}

	addHabit(t, s, "gym")
	addHabit(t, s, "read")
	retired := addHabit(t, s, "run")
	if _, err := s.UpdateHabit(ctx, retired.ID, UpdateHabitParams{Active: ptr(false), Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		params      LogHabitParams
		wantField   string
		wantMessage string
	}{
		{"empty habit", LogHabitParams{Habit: " ", Provenance: manual}, "habit", "must not be empty"},
		// The error gives the names, so that an agent can ask Gautam which one he means.
		{"habit that is not in the table", LogHabitParams{Habit: "gymm", Provenance: manual}, "habit", `no active habit has the name "gymm"; the active habits are: gym, read`},
		{"habit that is not active", LogHabitParams{Habit: "run", Provenance: manual}, "habit", `no active habit has the name "run"`},
		{"date that is not a date", LogHabitParams{Habit: "gym", LoggedOn: "today", Provenance: manual}, "logged_on", "YYYY-MM-DD"},
		{"unknown source", LogHabitParams{Habit: "gym", Provenance: Provenance{Source: "robot"}}, "source", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.LogHabit(ctx, tt.params)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Fatalf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
			if !strings.Contains(invalid.Message, tt.wantMessage) {
				t.Errorf("message = %q, want it to contain %q", invalid.Message, tt.wantMessage)
			}
		})
	}
	if n := countRows(t, pool, "habit_logs"); n != 0 {
		t.Errorf("habit_logs has %d rows after refused input, want 0", n)
	}
}

// A Shortcut that Siri runs two times, or a double tap, sends two requests
// at the same moment. One row must be stored, and no request must fail.
func TestLogHabitAtTheSameMomentStoresOneLog(t *testing.T) {
	s, pool := newTestService(t)
	addHabit(t, s, "gym")

	const callers = 8
	ids := make([]int64, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log, err := s.LogHabit(context.Background(), LogHabitParams{Habit: "gym", Provenance: Provenance{Source: SourceManual, Verified: true}})
			ids[i], errs[i] = log.ID, err
		}()
	}
	wg.Wait()

	for i := range callers {
		if errs[i] != nil {
			t.Errorf("caller %d: %v", i, errs[i])
		}
		if ids[i] != ids[0] {
			t.Errorf("caller %d got the log %d, caller 0 got %d", i, ids[i], ids[0])
		}
	}
	if n := countRows(t, pool, "habit_logs"); n != 1 {
		t.Errorf("habit_logs has %d rows, want 1", n)
	}
	if n := countRows(t, pool, "events"); n != 2 {
		t.Errorf("events = %d, want 2 (the habit, one log)", n)
	}
}

func TestGetHabitLog(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	addHabit(t, s, "gym")
	added := logHabit(t, s, "gym", "")

	got, err := s.GetHabitLog(ctx, added.ID)
	if err != nil || got != added {
		t.Errorf("GetHabitLog = %+v, %v, want %+v", got, err, added)
	}
	if _, err := s.GetHabitLog(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Errorf("absent log: error = %v, want ErrNotFound", err)
	}
}

func TestListHabitLogsReturnsTheNewestDayFirst(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	empty, err := s.ListHabitLogs(ctx, 0)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("ListHabitLogs with no log = %#v, %v, want an empty non-nil slice", empty, err)
	}

	addHabit(t, s, "gym")
	addHabit(t, s, "read")
	logHabit(t, s, "gym", "2026-10-09")
	logHabit(t, s, "gym", "2026-10-11")
	logHabit(t, s, "read", "2026-10-10")

	logs, err := s.ListHabitLogs(ctx, 2)
	if err != nil {
		t.Fatalf("ListHabitLogs: %v", err)
	}
	if len(logs) != 2 || logs[0].Habit != "gym" || logs[0].LoggedOn != "2026-10-11" || logs[1].Habit != "read" || logs[1].LoggedOn != "2026-10-10" {
		t.Errorf("logs = %+v, want gym on 2026-10-11, then read on 2026-10-10", logs)
	}
	if _, err := s.ListHabitLogs(ctx, 201); err == nil {
		t.Error("ListHabitLogs accepted a limit of 201")
	}
}

// The checklist is what the Kindle and the admin page show. Reports count
// only verified rows, so a log that waits for review is not "done".
func TestHabitsTodayShowsTheActiveHabitsAndTheirVerifiedLogs(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	gym := addHabit(t, s, "gym")
	addHabit(t, s, "read")
	addHabit(t, s, "walk")
	retired := addHabit(t, s, "run")
	logHabit(t, s, "gym", "")
	inferHabitLog(t, s, "read", "")
	logHabit(t, s, "walk", "2026-10-10") // yesterday in IST
	logHabit(t, s, "run", "")
	if _, err := s.UpdateHabit(ctx, retired.ID, UpdateHabitParams{Active: ptr(false), Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	got, err := s.HabitsToday(ctx)
	if err != nil {
		t.Fatalf("HabitsToday: %v", err)
	}

	if got.Date != "2026-10-11" {
		t.Errorf("date = %q, want 2026-10-11 (today in IST)", got.Date)
	}
	want := []struct {
		name string
		done bool
	}{{"gym", true}, {"read", false}, {"walk", false}}
	if len(got.Habits) != len(want) {
		t.Fatalf("habits = %+v, want gym, read, walk", got.Habits)
	}
	for i, w := range want {
		if h := got.Habits[i]; h.Name != w.name || h.Done != w.done {
			t.Errorf("habit %d = %+v, want %s with done = %v", i, h, w.name, w.done)
		}
	}
	if got.Habits[0].ID != gym.ID {
		t.Errorf("ID of gym = %d, want %d", got.Habits[0].ID, gym.ID)
	}
}

func TestHabitsTodayReturnsAnEmptyListNotNil(t *testing.T) {
	s, _ := newTestService(t)

	got, err := s.HabitsToday(context.Background())
	if err != nil {
		t.Fatalf("HabitsToday: %v", err)
	}
	if got.Habits == nil || len(got.Habits) != 0 {
		t.Errorf("habits = %#v, want an empty non-nil slice so JSON shows []", got.Habits)
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run Habit"
```
Expected: the run fails. The output contains `undefined: HabitLog`.

- [ ] **Step 3: Write the queries**

`InsertHabitLog` returns no row when the habit has a live log for the day. `ON CONFLICT` names the partial unique index of Task 1, so two inserts at the same moment store one row.

Create `queries/habit_logs.sql`:

```sql
-- name: GetActiveHabitByName :one
SELECT * FROM habits
WHERE name = $1 AND active AND deleted_at IS NULL;

-- name: InsertHabitLog :one
-- Returns no row when the habit has a live log for the day already.
INSERT INTO habit_logs (habit_id, logged_on, source, source_ref, verified)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (habit_id, logged_on) WHERE deleted_at IS NULL DO NOTHING
RETURNING *;

-- name: GetHabitLogOfDayForUpdate :one
SELECT * FROM habit_logs
WHERE habit_id = $1 AND logged_on = $2 AND deleted_at IS NULL
FOR UPDATE;

-- name: VerifyHabitLog :one
UPDATE habit_logs
SET verified = true
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: GetHabitLog :one
SELECT sqlc.embed(habit_logs), habits.name AS habit
FROM habit_logs
JOIN habits ON habits.id = habit_logs.habit_id
WHERE habit_logs.id = $1 AND habit_logs.deleted_at IS NULL;

-- name: ListHabitLogs :many
SELECT sqlc.embed(habit_logs), habits.name AS habit
FROM habit_logs
JOIN habits ON habits.id = habit_logs.habit_id
WHERE habit_logs.deleted_at IS NULL
ORDER BY habit_logs.logged_on DESC, habit_logs.id DESC
LIMIT $1;

-- name: ListHabitsOnDay :many
-- The active habits, each with "done": the habit has a verified log on the day.
SELECT habits.id, habits.name,
       EXISTS (
           SELECT 1 FROM habit_logs
           WHERE habit_logs.habit_id = habits.id
             AND habit_logs.logged_on = $1
             AND habit_logs.verified
             AND habit_logs.deleted_at IS NULL
       ) AS done
FROM habits
WHERE habits.active AND habits.deleted_at IS NULL
ORDER BY habits.name;
```

- [ ] **Step 4: Write the log functions**

`LogHabit` has three results: a new log (a `create` event), the stored log with no write, or the stored log approved (a `verify` event).

Create `internal/service/habit_logs.go`:

```go
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// HabitLog is one row of habit_logs: Gautam did the habit on that day.
type HabitLog struct {
	ID      int64 `json:"id"`
	HabitID int64 `json:"habit_id"`
	// Habit is the name of the habit.
	Habit string `json:"habit"`
	// LoggedOn is the IST day, as YYYY-MM-DD.
	LoggedOn  string    `json:"logged_on"`
	Source    Source    `json:"source"`
	SourceRef *int64    `json:"source_ref,omitempty"`
	Verified  bool      `json:"verified"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LogHabitParams is the input of LogHabit.
type LogHabitParams struct {
	// Habit is the name of an active habit. Case and extra spaces do not
	// matter.
	Habit string
	// LoggedOn is optional: YYYY-MM-DD, not later than today in IST. An
	// empty value means today in IST.
	LoggedOn string
	Provenance
}

// LogHabit records that Gautam did a habit on a day, and appends a create
// event, in one transaction. A habit has one log for each day: when the
// log exists already, LogHabit returns it and writes nothing. The one
// exception: a verified call over a log that is not verified approves that
// log, because Gautam's own statement counts.
func (s *Service) LogHabit(ctx context.Context, p LogHabitParams) (HabitLog, error) {
	name := habitName(p.Habit)
	if name == "" {
		return HabitLog{}, invalid("habit", "must not be empty")
	}
	day, err := pastDay("logged_on", p.LoggedOn, s.now())
	if err != nil {
		return HabitLog{}, err
	}
	if err := p.Provenance.validate(); err != nil {
		return HabitLog{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return HabitLog{}, fmt.Errorf("log habit: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	habit, err := q.GetActiveHabitByName(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return HabitLog{}, unknownHabit(ctx, q, name)
	}
	if err != nil {
		return HabitLog{}, fmt.Errorf("log habit: %w", err)
	}

	action, old := "create", any(nil)
	row, err := q.InsertHabitLog(ctx, store.InsertHabitLogParams{
		HabitID:   habit.ID,
		LoggedOn:  day,
		Source:    string(p.Source),
		SourceRef: p.SourceRef,
		Verified:  p.Verified,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// The habit has a log for this day already.
		existing, err := q.GetHabitLogOfDayForUpdate(ctx, store.GetHabitLogOfDayForUpdateParams{HabitID: habit.ID, LoggedOn: day})
		if err != nil {
			return HabitLog{}, fmt.Errorf("log habit: %w", err)
		}
		if existing.Verified || !p.Verified {
			return habitLogFromRow(existing, habit.Name), nil
		}
		action, old = "verify", habitLogFromRow(existing, habit.Name)
		row, err = q.VerifyHabitLog(ctx, existing.ID)
		if err != nil {
			return HabitLog{}, fmt.Errorf("log habit: %w", err)
		}
	} else if err != nil {
		return HabitLog{}, fmt.Errorf("log habit: %w", err)
	}

	log := habitLogFromRow(row, habit.Name)
	if err := recordEvent(ctx, q, action, "habit_logs", log.ID, p.Source, old, log); err != nil {
		return HabitLog{}, fmt.Errorf("log habit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return HabitLog{}, fmt.Errorf("log habit: %w", err)
	}
	return log, nil
}

// unknownHabit returns the error for a name that no active habit has. The
// message lists the active habits, so that an agent can ask Gautam which
// one he means.
func unknownHabit(ctx context.Context, q *store.Queries, name string) error {
	habits, err := q.ListHabits(ctx)
	if err != nil {
		return fmt.Errorf("log habit: %w", err)
	}
	var active []string
	for _, habit := range habits {
		if habit.Active {
			active = append(active, habit.Name)
		}
	}
	if len(active) == 0 {
		return invalid("habit", "there is no active habit: add the habit in the admin pages first")
	}
	return invalid("habit", fmt.Sprintf("no active habit has the name %q; the active habits are: %s", name, strings.Join(active, ", ")))
}

func habitLogFromRow(row store.HabitLog, habit string) HabitLog {
	return HabitLog{
		ID:        row.ID,
		HabitID:   row.HabitID,
		Habit:     habit,
		LoggedOn:  row.LoggedOn.Format(time.DateOnly),
		Source:    Source(row.Source),
		SourceRef: row.SourceRef,
		Verified:  row.Verified,
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
}

// GetHabitLog returns the habit log with this ID. It returns ErrNotFound
// when there is no such log or the log is deleted.
func (s *Service) GetHabitLog(ctx context.Context, id int64) (HabitLog, error) {
	row, err := store.New(s.pool).GetHabitLog(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return HabitLog{}, ErrNotFound
	}
	if err != nil {
		return HabitLog{}, fmt.Errorf("get habit log: %w", err)
	}
	return habitLogFromRow(row.HabitLog, row.Habit), nil
}

// ListHabitLogs returns the habit logs that are not deleted, the newest
// day first. limit 0 means 50; the maximum is 200.
func (s *Service) ListHabitLogs(ctx context.Context, limit int) ([]HabitLog, error) {
	maxRows, err := checkLimit(limit)
	if err != nil {
		return nil, err
	}
	rows, err := store.New(s.pool).ListHabitLogs(ctx, maxRows)
	if err != nil {
		return nil, fmt.Errorf("list habit logs: %w", err)
	}
	logs := make([]HabitLog, 0, len(rows))
	for _, row := range rows {
		logs = append(logs, habitLogFromRow(row.HabitLog, row.Habit))
	}
	return logs, nil
}

// HabitStatus says whether one active habit is done on a day.
type HabitStatus struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// Done is true when the habit has a verified log on the day. A log
	// that waits for review does not count.
	Done bool `json:"done"`
}

// HabitChecklist is the state of every active habit on one day.
type HabitChecklist struct {
	// Date is the IST day, as YYYY-MM-DD.
	Date   string        `json:"date"`
	Habits []HabitStatus `json:"habits"`
}

// HabitsToday returns the active habits, by name, with their state for
// today in IST.
func (s *Service) HabitsToday(ctx context.Context) (HabitChecklist, error) {
	day := today(s.now())
	rows, err := store.New(s.pool).ListHabitsOnDay(ctx, day)
	if err != nil {
		return HabitChecklist{}, fmt.Errorf("habits today: %w", err)
	}
	list := HabitChecklist{Date: day.Format(time.DateOnly), Habits: make([]HabitStatus, 0, len(rows))}
	for _, row := range rows {
		list.Habits = append(list.Habits, HabitStatus{ID: row.ID, Name: row.Name, Done: row.Done})
	}
	return list, nil
}
```

- [ ] **Step 5: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./internal/service ARGS="-run Habit"
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/service`.

- [ ] **Step 6: Commit**

Run:
```bash
git add queries/habit_logs.sql internal/service/habit_logs.go internal/service/habit_logs_test.go
git commit -m "feat(service): log a habit, and read the logs and the checklist of today"
```

- [ ] **Step 7: Write the failing tests for delete, the record functions and the review**

Create `internal/service/habit_logs_change_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"
)

func TestDeleteHabitLog(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	addHabit(t, s, "gym")
	added := logHabit(t, s, "gym", "")

	err := s.DeleteHabitLog(ctx, added.ID, DeleteParams{Actor: SourceAgent})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "confirmed" {
		t.Fatalf("delete by an agent with no confirmation: error = %v, want a ValidationError on confirmed", err)
	}

	if err := s.DeleteHabitLog(ctx, added.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("DeleteHabitLog: %v", err)
	}
	if _, err := s.GetHabitLog(ctx, added.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get after delete: error = %v, want ErrNotFound", err)
	}
	if logs, _ := s.ListHabitLogs(ctx, 0); len(logs) != 0 {
		t.Errorf("list after delete = %v, want it empty", logs)
	}
	if today, _ := s.HabitsToday(ctx); len(today.Habits) != 1 || today.Habits[0].Done {
		t.Errorf("checklist after delete = %+v, want gym not done", today)
	}
	if n := countRows(t, pool, "habit_logs"); n != 1 {
		t.Errorf("habit_logs has %d rows, want 1: a delete keeps the row", n)
	}
	events := allEvents(t, pool)
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3 (the habit, create, delete)", len(events))
	}
	if e := events[2]; e.Action != "delete" || e.RecordType != "habit_logs" || e.RecordID != added.ID || e.Source != "manual" || e.Old["habit"] != "gym" || e.New != nil {
		t.Errorf("event = %+v, want a delete with the old log", e)
	}

	if err := s.DeleteHabitLog(ctx, added.ID, DeleteParams{Actor: SourceManual}); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: error = %v, want ErrNotFound", err)
	}
}

// "I did not go to the gym" deletes the log. A log for the same day after
// that is a new log, not an error.
func TestLogHabitAfterADeleteStoresANewLog(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	addHabit(t, s, "gym")
	first := logHabit(t, s, "gym", "")
	if err := s.DeleteHabitLog(ctx, first.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatal(err)
	}

	second := logHabit(t, s, "gym", "")

	if second.ID == first.ID {
		t.Errorf("the new log has the ID of the deleted log: %d", first.ID)
	}
	if n := countRows(t, pool, "habit_logs"); n != 2 {
		t.Errorf("habit_logs has %d rows, want 2 (one deleted, one live)", n)
	}
	if today, _ := s.HabitsToday(ctx); len(today.Habits) != 1 || !today.Habits[0].Done {
		t.Errorf("checklist = %+v, want gym done", today)
	}
}

func TestRecordFunctionsWorkOnAHabitLog(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	addHabit(t, s, "gym")
	added := logHabit(t, s, "gym", "")

	got, err := s.Get(ctx, "habit_log", added.ID)
	if log, ok := got.(HabitLog); err != nil || !ok || log != added {
		t.Fatalf("Get = %#v, %v, want the log", got, err)
	}

	// A log has no field to correct: the habit and the day are the log.
	_, err = s.Update(ctx, "habit_log", added.ID, UpdateParams{Fields: map[string]any{"logged_on": "2026-10-10"}, Actor: SourceManual})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "type" {
		t.Errorf("Update: error = %v, want a ValidationError on type", err)
	}
	if n := countRows(t, pool, "events"); n != 2 {
		t.Errorf("events = %d after the refused update, want 2", n)
	}

	if err := s.Delete(ctx, "habit_log", added.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "habit_log", added.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: error = %v, want ErrNotFound", err)
	}
}

func TestReviewQueueAndVerifyHandleAnInferredHabitLog(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	addHabit(t, s, "gym")
	item := addUnverified(t, s, "Dune")
	inferred := inferHabitLog(t, s, "gym", "2026-10-10")
	logHabit(t, s, "gym", "") // verified

	queue, err := s.ReviewQueue(ctx, 0)
	if err != nil {
		t.Fatalf("ReviewQueue: %v", err)
	}
	if len(queue) != 2 || queue[0].ID != item.ID {
		t.Fatalf("queue = %+v, want the item, then the log", queue)
	}
	if e := queue[1]; e.Type != "habit_log" || e.ID != inferred.ID || e.Summary != "habit: gym on 2026-10-10" || e.Source != SourceAgent {
		t.Errorf("entry = %+v", e)
	}

	n, err := s.Verify(ctx, []RecordRef{{"habit_log", inferred.ID}}, SourceManual)
	if err != nil || n != 1 {
		t.Fatalf("Verify = %d, %v, want 1", n, err)
	}
	if got, _ := s.GetHabitLog(ctx, inferred.ID); !got.Verified {
		t.Error("the log is not verified")
	}
	events := allEvents(t, pool)
	if e := events[len(events)-1]; e.Action != "verify" || e.RecordType != "habit_logs" || e.RecordID != inferred.ID || e.Old["verified"] != false || e.New["habit"] != "gym" {
		t.Errorf("event = %+v, want a verify of the log", e)
	}
}
```

- [ ] **Step 8: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run Habit"
```
Expected: the run fails. The output contains `s.DeleteHabitLog undefined`.

- [ ] **Step 9: Write the queries**

Add this at the end of `queries/habit_logs.sql`:

```sql
-- name: GetHabitLogForUpdate :one
SELECT sqlc.embed(habit_logs), habits.name AS habit
FROM habit_logs
JOIN habits ON habits.id = habit_logs.habit_id
WHERE habit_logs.id = $1 AND habit_logs.deleted_at IS NULL
FOR UPDATE OF habit_logs;

-- name: SoftDeleteHabitLog :one
UPDATE habit_logs
SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: ListUnverifiedHabitLogs :many
SELECT sqlc.embed(habit_logs), habits.name AS habit
FROM habit_logs
JOIN habits ON habits.id = habit_logs.habit_id
WHERE habit_logs.deleted_at IS NULL AND NOT habit_logs.verified
ORDER BY habit_logs.created_at, habit_logs.id
LIMIT $1;
```

- [ ] **Step 10: Write the delete and the review parts**

Create `internal/service/habit_logs_change.go`:

```go
package service

import (
	"context"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// DeleteHabitLog marks one habit log as deleted and appends a delete event
// that holds the row, in one transaction. After that, the habit can get a
// new log for the same day.
func (s *Service) DeleteHabitLog(ctx context.Context, id int64, p DeleteParams) error {
	return s.deleteRecord(ctx, "habit_logs", id, p, func(q *store.Queries) (any, error) {
		// The first query locks the row and gives the name of the habit.
		current, err := q.GetHabitLogForUpdate(ctx, id)
		if err != nil {
			return nil, err
		}
		row, err := q.SoftDeleteHabitLog(ctx, id)
		if err != nil {
			return nil, err
		}
		return habitLogFromRow(row, current.Habit), nil
	})
}

// summary is the one line that the review queue shows for a habit log.
func (l HabitLog) summary() string {
	return "habit: " + l.Habit + " on " + l.LoggedOn
}

// unverifiedHabitLogs returns the habit logs that wait for review, oldest
// first.
func unverifiedHabitLogs(ctx context.Context, q *store.Queries, limit int32) ([]ReviewEntry, error) {
	rows, err := q.ListUnverifiedHabitLogs(ctx, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]ReviewEntry, 0, len(rows))
	for _, row := range rows {
		log := habitLogFromRow(row.HabitLog, row.Habit)
		entries = append(entries, ReviewEntry{
			Type:      "habit_log",
			ID:        log.ID,
			Summary:   log.summary(),
			Source:    log.Source,
			CreatedAt: log.CreatedAt,
			UpdatedAt: log.UpdatedAt,
		})
	}
	return entries, nil
}

func verifyHabitLog(ctx context.Context, q *store.Queries, id int64) (before, after any, err error) {
	current, err := q.GetHabitLogForUpdate(ctx, id)
	if err != nil || current.HabitLog.Verified {
		return nil, nil, err
	}
	stored, err := q.VerifyHabitLog(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return habitLogFromRow(current.HabitLog, current.Habit), habitLogFromRow(stored, current.Habit), nil
}
```

- [ ] **Step 11: Add the type `habit_log` to the record functions**

In `internal/service/records.go`, replace this:

```go
var RecordTypes = []string{"item", "journal", "todo"}
```

With this:

```go
var RecordTypes = []string{"item", "journal", "todo", "habit_log"}
```

In `internal/service/records.go`, replace this:

```go
// todo.
```

With this:

```go
// todo, a HabitLog for the type habit_log.
```

In `internal/service/records.go`, replace this:

```go
	case "todo":
		return asRecord(s.GetTodo(ctx, id))
```

With this:

```go
	case "todo":
		return asRecord(s.GetTodo(ctx, id))
	case "habit_log":
		return asRecord(s.GetHabitLog(ctx, id))
```

In `internal/service/records.go`, replace this:

```go
		return asRecord(s.UpdateTodo(ctx, id, params))
```

With this:

```go
		return asRecord(s.UpdateTodo(ctx, id, params))
	case "habit_log":
		// The habit and the day are the log. A correction is a new log.
		return nil, invalid("type", "a habit_log has no field that you can change: delete the log, then log the habit for the correct day")
```

In `internal/service/records.go`, replace this:

```go
	case "todo":
		return s.DeleteTodo(ctx, id, p)
```

With this:

```go
	case "todo":
		return s.DeleteTodo(ctx, id, p)
	case "habit_log":
		return s.DeleteHabitLog(ctx, id, p)
```

- [ ] **Step 12: Add the type `habit_log` to the review queue and to `Verify`**

In `internal/service/review.go`, replace this:

```go
		unverifiedTodos,
```

With this:

```go
		unverifiedTodos,
		unverifiedHabitLogs,
```

In `internal/service/review.go`, replace this:

```go
		return "todos", before, after, err
```

With this:

```go
		return "todos", before, after, err
	case "habit_log":
		before, after, err = verifyHabitLog(ctx, q, ref.ID)
		return "habit_logs", before, after, err
```

- [ ] **Step 13: Run the service tests to verify that they pass**

Run:
```bash
make test PKG=./internal/service
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/service`.

- [ ] **Step 14: Commit**

Run:
```bash
git add queries/habit_logs.sql internal/service/habit_logs_change.go internal/service/habit_logs_change_test.go internal/service/records.go internal/service/review.go
git commit -m "feat(service): delete a habit log, and add habit logs to the record functions and the review"
```

---

### Task 12: REST API and MCP tool for habits

**Files:**
- Create: `internal/api/habits.go`, `internal/mcp/habits.go`
- Modify: `internal/api/api.go` (one line), `internal/mcp/server.go` (one line), `internal/mcp/records.go` (the argument descriptions)
- Test: `internal/api/habits_test.go`, `internal/mcp/habits_test.go`

**Interfaces:**
- Consumes (Tasks 10 and 11): `service.AddHabit` (tests only), `service.LogHabit`, `GetHabitLog`, `DeleteHabitLog`, `HabitsToday`, `service.HabitLog`, `service.HabitChecklist`. Consumes (Task 4): `idInput`, `toHTTPError`, `toToolError`, `todayIST` in the two test packages, the test helpers of the two packages.
- Produces:
  - Package `api`: the routes `POST /api/habits/log` (200), `GET /api/habits/today`, `GET /api/habits/logs/{id}`, `DELETE /api/habits/logs/{id}` (204)
  - Package `mcp`: the tool `log_habit` with the arguments `habit`, `logged_on` and `dictated`
  - Test helpers: `addHabit(t, pool, name string) service.Habit` in package `api_test`; `addHabit(t, pool, name string)` in package `mcp_test`

No route and no tool adds a habit. The tests add one through the service.

- [ ] **Step 1: Write the failing tests for the routes**

Create `internal/api/habits_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/api ARGS="-run Habit"
```
Expected: the run fails. The output contains `POST /api/habits/log: body is not a JSON object`.

- [ ] **Step 3: Write the routes**

Create `internal/api/habits.go`:

```go
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type logHabitInput struct {
	Body struct {
		Habit    string `json:"habit" doc:"The name of an active habit, such as gym"`
		LoggedOn string `json:"logged_on,omitempty" doc:"The day, as YYYY-MM-DD. The default is today in India Standard Time."`
	}
}

type habitLogOutput struct {
	Body service.HabitLog
}

type habitsTodayOutput struct {
	Body service.HabitChecklist
}

func registerHabits(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "log-habit",
		Method:      http.MethodPost,
		Path:        "/api/habits/log",
		Summary:     "Log a habit for a day",
		Description: "A habit has one log for each day. A second request for the same day changes nothing and returns the stored log.",
	}, func(ctx context.Context, in *logHabitInput) (*habitLogOutput, error) {
		// A request to the REST API comes from Gautam's own devices, so
		// the row is manual and verified.
		log, err := svc.LogHabit(ctx, service.LogHabitParams{
			Habit:      in.Body.Habit,
			LoggedOn:   in.Body.LoggedOn,
			Provenance: service.Provenance{Source: service.SourceManual, Verified: true},
		})
		if err != nil {
			return nil, toHTTPError("body", err)
		}
		return &habitLogOutput{Body: log}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "habits-today",
		Method:      http.MethodGet,
		Path:        "/api/habits/today",
		Summary:     "List the active habits with their state for today",
		Description: "A habit is done when it has a verified log for today in India Standard Time.",
	}, func(ctx context.Context, _ *struct{}) (*habitsTodayOutput, error) {
		list, err := svc.HabitsToday(ctx)
		if err != nil {
			return nil, toHTTPError("query", err)
		}
		return &habitsTodayOutput{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-habit-log",
		Method:      http.MethodGet,
		Path:        "/api/habits/logs/{id}",
		Summary:     "Read one habit log",
	}, func(ctx context.Context, in *idInput) (*habitLogOutput, error) {
		log, err := svc.GetHabitLog(ctx, in.ID)
		if err != nil {
			return nil, toHTTPError("path", err)
		}
		return &habitLogOutput{Body: log}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-habit-log",
		Method:        http.MethodDelete,
		Path:          "/api/habits/logs/{id}",
		Summary:       "Delete a habit log",
		Description:   "Marks the log as deleted. The row stays in the database. The habit can get a new log for the same day.",
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *idInput) (*struct{}, error) {
		if err := svc.DeleteHabitLog(ctx, in.ID, service.DeleteParams{Actor: service.SourceManual}); err != nil {
			return nil, toHTTPError("path", err)
		}
		return nil, nil
	})
}
```

In `internal/api/api.go`, replace this:

```go
	registerTodos(api, svc)
```

With this:

```go
	registerTodos(api, svc)
	registerHabits(api, svc)
```

- [ ] **Step 4: Run the API tests to verify that they pass**

Run:
```bash
make test PKG=./internal/api
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/api`.

- [ ] **Step 5: Commit**

Run:
```bash
git add internal/api/api.go internal/api/habits.go internal/api/habits_test.go
git commit -m "feat(api): add the habit routes"
```

- [ ] **Step 6: Write the failing tests for the tool**

Create `internal/mcp/habits_test.go`:

```go
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
```

- [ ] **Step 7: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/mcp ARGS="-run 'Habit|DailyRecords'"
```
Expected: the run fails. The output contains `--- FAIL: TestLogHabitStoresALogThatTheUserDictated` and `tool log_habit is not registered with a description`.

- [ ] **Step 8: Write the tool**

Create `internal/mcp/habits.go`:

```go
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type logHabitInput struct {
	Habit    string `json:"habit" jsonschema:"The name of one of the user's habits, such as gym. If no habit has this name, the error lists the habits: ask the user which one they mean."`
	LoggedOn string `json:"logged_on,omitempty" jsonschema:"The day that the user did the habit, as YYYY-MM-DD in India Standard Time. Leave it out for today."`
	Dictated bool   `json:"dictated,omitempty" jsonschema:"true only when the user stated directly that they did the habit. false when you inferred it from a journal entry or a different source; the log then waits in the review queue."`
}

func registerHabits(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "log_habit",
		Description: "Record that the user did a habit on a day. A habit has one log for each day: a second call for the same day changes nothing. This tool does not add a new habit; the user adds a habit in the admin pages. To remove a log, use delete with the type habit_log. Returns the stored log with its ID.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in logHabitInput) (*sdk.CallToolResult, service.HabitLog, error) {
		log, err := svc.LogHabit(ctx, service.LogHabitParams{
			Habit:      in.Habit,
			LoggedOn:   in.LoggedOn,
			Provenance: service.Provenance{Source: service.SourceAgent, Verified: in.Dictated},
		})
		if err != nil {
			return nil, service.HabitLog{}, toToolError(err)
		}
		return nil, log, nil
	})
}
```

In `internal/mcp/server.go`, replace this:

```go
	registerTodos(server, svc)
```

With this:

```go
	registerTodos(server, svc)
	registerHabits(server, svc)
```

- [ ] **Step 9: Tell the model about the new type in `get`, `update` and `delete`**

In `internal/mcp/records.go`, replace each occurrence of this (three places):

```go
Record type. One of: item, journal, todo"
```

With this:

```go
Record type. One of: item, journal, todo, habit_log"
```

In `internal/mcp/records.go`, replace this:

```go
Leave out a field to keep its value. attrs replaces every attr.
```

With this:

```go
A habit_log has no field to change: delete it, then log the habit again. Leave out a field to keep its value. attrs replaces every attr.
```

- [ ] **Step 10: Run the MCP tests to verify that they pass**

Run:
```bash
make test PKG=./internal/mcp
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/mcp`.

- [ ] **Step 11: Commit**

Run:
```bash
git add internal/mcp/habits.go internal/mcp/habits_test.go internal/mcp/server.go internal/mcp/records.go
git commit -m "feat(mcp): add the log_habit tool"
```

---

### Task 13: Admin page for habits

**Files:**
- Create: `internal/admin/habits.templ`, `internal/admin/habits.go`
- Modify: `internal/admin/admin.go` (five routes), `internal/admin/items.templ` (one link), `internal/admin/review.templ` (one case)
- Test: `internal/admin/habits_test.go`

**Interfaces:**
- Consumes (Tasks 10 and 11): `service.AddHabit`, `ListHabits`, `UpdateHabit`, `LogHabit`, `ListHabitLogs`, `GetHabitLog`, `DeleteHabitLog`, `HabitsToday`, `service.Habit`, `service.HabitLog`, `service.HabitChecklist`. Consumes (Task 5): `pathID`, `render`, `listFailure`, `internalError`, `page`, `formatVerified`, the `switch` in `review.templ`, the test helpers `todayIST`, `countEvents`, `get`, `postForm`, `newTestAdmin`.
- Produces (package `admin`):
  - The routes `GET /admin/habits`, `POST /admin/habits`, `POST /admin/habits/log`, `POST /admin/habits/{id}`, `POST /admin/habits/logs/{id}/delete`
  - `type habitsView struct{ Today service.HabitChecklist; Habits []service.Habit; Logs []service.HabitLog }`, `habitPath(id int64) string`
  - Test helpers in package `admin_test`: `addStoredHabit(t, pool, name string) service.Habit`, `logStored(t, pool, habit string, verified bool) service.HabitLog`

The page has one form to add a habit and three lists: the checklist of today, every habit with its name and its active state, and the logs. Each form post replaces the three lists.

- [ ] **Step 1: Write the failing tests**

Create `internal/admin/habits_test.go`:

```go
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
	for _, want := range []string{`hx-post="/admin/habits"`, `name="name"`, "No habits yet.", "No logs yet.", "Today (" + todayIST() + ")", `href="/admin/habits"`} {
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
	for _, want := range []string{`<td>gym</td>`, "not done", `name="habit" value="gym"`, `value="gym"`} {
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
	for _, want := range []string{`value="a&#34;&lt;b&gt;x"`, `<td>a&#34;&lt;b&gt;x</td>`} {
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
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/admin ARGS="-run Habit"
```
Expected: the run fails. The output contains `--- FAIL: TestHabitsPageShowsTheFormAndNoHabits` and `--- FAIL: TestReviewPageLinksAHabitLogToTheHabitsPage`.

- [ ] **Step 3: Write the templates**

Create `internal/admin/habits.templ`:

```templ
package admin

import (
	"strconv"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// habitsView is what the habits page shows below its form.
type habitsView struct {
	Today  service.HabitChecklist
	Habits []service.Habit
	Logs   []service.HabitLog
}

templ habitsPage(view habitsView) {
	@page("Habits") {
		<h1>Habits</h1>
		<form hx-post="/admin/habits" hx-target="#list" hx-swap="outerHTML" hx-on:habit-added="this.reset()">
			<label>
				New habit
				<input type="text" name="name" required maxlength="50" size="20"/>
			</label>
			<button type="submit">Add</button>
		</form>
		@habitsList(view, "")
	}
}

// habitsList is the part of the page that a form post replaces.
templ habitsList(view habitsView, errorMessage string) {
	<div id="list">
		if errorMessage != "" {
			<p class="error" role="alert">{ errorMessage }</p>
		}
		<h2>Today ({ view.Today.Date })</h2>
		if len(view.Today.Habits) == 0 {
			<p class="muted">No active habit.</p>
		} else {
			<table>
				<tbody>
					for _, habit := range view.Today.Habits {
						<tr>
							<td>{ habit.Name }</td>
							if habit.Done {
								<td>done</td>
								<td></td>
							} else {
								<td>not done</td>
								<td>
									<form hx-post="/admin/habits/log" hx-target="#list" hx-swap="outerHTML">
										<input type="hidden" name="habit" value={ habit.Name }/>
										<button type="submit">Log</button>
									</form>
								</td>
							}
						</tr>
					}
				</tbody>
			</table>
		}
		<h2>All habits</h2>
		if len(view.Habits) == 0 {
			<p class="muted">No habits yet.</p>
		} else {
			<table>
				<tbody>
					for _, habit := range view.Habits {
						<tr>
							<td>{ strconv.FormatInt(habit.ID, 10) }</td>
							<td>
								<form hx-post={ habitPath(habit.ID) } hx-target="#list" hx-swap="outerHTML">
									<input type="text" name="name" required maxlength="50" size="20" value={ habit.Name }/>
									<label>
										<input type="checkbox" name="active" value="true" checked?={ habit.Active }/>
										active
									</label>
									<button type="submit">Save</button>
								</form>
							</td>
						</tr>
					}
				</tbody>
			</table>
		}
		<h2>Logs</h2>
		if len(view.Logs) == 0 {
			<p class="muted">No logs yet.</p>
		} else {
			<table>
				<thead>
					<tr>
						<th>ID</th>
						<th>Habit</th>
						<th>Day</th>
						<th>Source</th>
						<th>Verified</th>
						<th>Logged (IST)</th>
						<th></th>
					</tr>
				</thead>
				<tbody>
					for _, log := range view.Logs {
						<tr>
							<td>{ strconv.FormatInt(log.ID, 10) }</td>
							<td>{ log.Habit }</td>
							<td>{ log.LoggedOn }</td>
							<td>{ string(log.Source) }</td>
							<td>{ formatVerified(log.Verified) }</td>
							<td>{ log.CreatedAt.In(service.IST).Format("2 Jan 2006 15:04") }</td>
							<td>
								<button
									hx-post={ "/admin/habits/logs/" + strconv.FormatInt(log.ID, 10) + "/delete" }
									hx-target="#list"
									hx-swap="outerHTML"
									hx-confirm={ "Delete the log of " + log.Habit + " on " + log.LoggedOn + "?" }
								>Delete</button>
							</td>
						</tr>
					}
				</tbody>
			</table>
		}
	</div>
}

func habitPath(id int64) string {
	return "/admin/habits/" + strconv.FormatInt(id, 10)
}
```

- [ ] **Step 4: Write the handlers**

`showHabits` holds the steps that each form post has after its write: the message for a refusal, then the three lists.

Create `internal/admin/habits.go`:

```go
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
```

- [ ] **Step 5: Add the routes, the link and the case of the review page**

chi matches the fixed path `/admin/habits/log` before the path with `{id}`.

In `internal/admin/admin.go`, replace this:

```go
	router.Get("/admin/review", h.reviewPage)
```

With this:

```go
	router.Get("/admin/habits", h.habitsPage)
	router.Post("/admin/habits", h.addHabit)
	router.Post("/admin/habits/log", h.logHabitToday)
	router.Post("/admin/habits/{id}", h.saveHabit)
	router.Post("/admin/habits/logs/{id}/delete", h.deleteHabitLog)
	router.Get("/admin/review", h.reviewPage)
```

In `internal/admin/items.templ`, replace this:

```templ
				<a href="/admin/todos">Todos</a>
```

With this:

```templ
				<a href="/admin/todos">Todos</a>
				<a href="/admin/habits">Habits</a>
```

A habit log has no edit page. Its link goes to the habits page, where Gautam can delete a log that is wrong.

In `internal/admin/review.templ`, replace this:

```templ
									case "todo":
										<a href={ templ.URL(todoPath(entry.ID) + "/edit") }>Edit</a>
```

With this:

```templ
									case "todo":
										<a href={ templ.URL(todoPath(entry.ID) + "/edit") }>Edit</a>
									case "habit_log":
										<a href="/admin/habits">Habits</a>
```

- [ ] **Step 6: Run the admin tests to verify that they pass**

Run:
```bash
make test PKG=./internal/admin
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/internal/admin`.

- [ ] **Step 7: Commit**

Run:
```bash
git add internal/admin/habits.templ internal/admin/habits.go internal/admin/habits_test.go internal/admin/admin.go internal/admin/items.templ internal/admin/review.templ
git commit -m "feat(admin): add the habits page"
```

---


### Task 14: End-to-end test, documents, smoke check, the real database and the confirmation gate

**Files:**
- Create: `cmd/lifeos/daily_test.go`
- Modify: `AGENTS.md` (one edit)

**Interfaces:**
- Consumes (package `main`, plans 1 and 2): the test helpers `startServer(t) (config.Config, *pgxpool.Pool)` and `send(t, method, url, contentType, body string) (int, string)` in `cmd/lifeos/lifecycle_test.go`; every route and tool of Tasks 4 to 13.
- Produces: the four tables in the real database, the new server on the real database, and a result for Gautam to confirm. Later plans start only after that confirmation.

- [ ] **Step 1: Write the end-to-end test**

This test adds no production code. It guards the wiring: each step uses a different adapter and reads what the step before it wrote.

Create `cmd/lifeos/daily_test.go`:

```go
package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// One day of records: a journal entry, a todo that the agent proposes from
// it, and a habit. Each step uses a different adapter, and each adapter
// must see what the others did.
func TestServeTakesADayOfRecordsThroughEveryAdapter(t *testing.T) {
	cfg, pool := startServer(t)
	ctx := context.Background()
	rest := "http://" + cfg.HTTPAddr
	const form = "application/x-www-form-urlencoded"
	const json = "application/json"

	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: "http://" + cfg.MCPAddr + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("MCP connect: %v", err)
	}
	defer session.Close()
	tool := func(name string, args map[string]any) map[string]any {
		t.Helper()
		result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("MCP %s: %v", name, err)
		}
		if result.IsError {
			t.Fatalf("MCP %s returned an error: %+v", name, result.Content)
		}
		return result.StructuredContent.(map[string]any)
	}

	// 1. MCP: the journal entry, in Gautam's words. The journal page of
	// the admin shows it, verified.
	entry := tool("journal", map[string]any{"body": "planned the tunnel.\nmust buy a domain"})
	if entry["verified"] != true {
		t.Fatalf("journal: %+v, want a verified entry", entry)
	}
	if status, body := send(t, "GET", rest+"/admin/journal", "", ""); status != 200 || !strings.Contains(body, "planned the tunnel.\nmust buy a domain") {
		t.Fatalf("GET /admin/journal: status %d, body %s", status, body)
	}

	// 2. MCP: the agent proposes a todo from the entry. REST shows it in
	// the review queue.
	todo := tool("todo", map[string]any{"title": "Buy a domain", "project": "lifeos"})
	todoID := int64(todo["id"].(float64))
	if status, body := send(t, "GET", rest+"/api/review", "", ""); status != 200 || !strings.Contains(body, "todo: Buy a domain, project lifeos") {
		t.Fatalf("GET /api/review: status %d, body %s", status, body)
	}

	// 3. Admin: Gautam approves the todo. REST marks it done.
	status, body := send(t, "POST", rest+"/admin/review/verify", form, fmt.Sprintf("type=todo&id=%d", todoID))
	if status != 200 || !strings.Contains(body, "No record waits for review.") {
		t.Fatalf("POST /admin/review/verify: status %d, body %s", status, body)
	}
	status, body = send(t, "PATCH", fmt.Sprintf("%s/api/todos/%d", rest, todoID), json, `{"status":"done"}`)
	if status != 200 || !strings.Contains(body, `"done_at"`) || !strings.Contains(body, `"verified":true`) {
		t.Fatalf("PATCH todo: status %d, body %s", status, body)
	}
	if got := tool("get", map[string]any{"type": "todo", "id": todoID}); got["status"] != "done" {
		t.Fatalf("MCP get of the todo: %+v, want it done", got)
	}

	// 4. Admin: Gautam adds a habit. REST logs it, as the Shortcut does.
	if status, body := send(t, "POST", rest+"/admin/habits", form, "name=Gym"); status != 200 || !strings.Contains(body, "not done") {
		t.Fatalf("POST /admin/habits: status %d, body %s", status, body)
	}
	status, body = send(t, "POST", rest+"/api/habits/log", json, `{"habit":"gym"}`)
	if status != 200 || !strings.Contains(body, `"habit":"gym"`) {
		t.Fatalf("POST /api/habits/log: status %d, body %s", status, body)
	}
	if status, body := send(t, "GET", rest+"/api/habits/today", "", ""); status != 200 || !strings.Contains(body, `"name":"gym","done":true`) {
		t.Fatalf("GET /api/habits/today: status %d, body %s", status, body)
	}

	// 5. MCP: the same habit and day again is the same log. A delete
	// that Gautam confirmed removes it, and the checklist shows that.
	log := tool("log_habit", map[string]any{"habit": "gym", "dictated": true})
	if log["source"] != "manual" {
		t.Fatalf("log_habit: %+v, want the log that the REST request stored", log)
	}
	tool("delete", map[string]any{"type": "habit_log", "id": log["id"], "confirmed": true})
	if _, body := send(t, "GET", rest+"/api/habits/today", "", ""); !strings.Contains(body, `"name":"gym","done":false`) {
		t.Errorf("GET /api/habits/today after the delete: %s", body)
	}

	// 6. Each write has its event, with the adapter that made it.
	rows, err := pool.Query(ctx, "SELECT action || ' ' || record_type || ' by ' || source FROM events ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	want := []string{
		"create journal_entries by agent",
		"create todos by agent",
		"verify todos by manual",
		"update todos by manual",
		"create habits by manual",
		"create habit_logs by manual",
		"delete habit_logs by agent",
	}
	if got := strings.Join(events, "\n"); got != strings.Join(want, "\n") {
		t.Errorf("events =\n%s\nwant\n%s", got, strings.Join(want, "\n"))
	}
}
```

- [ ] **Step 2: Run the test to verify that it passes, then see it fail one time**

Run:
```bash
make test PKG=./cmd/lifeos
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/cmd/lifeos`.

A test that never failed proves little. Break the wiring one time: remove the line that registers the habit routes.

Run:
```bash
sed -i '/registerHabits(api, svc)/d' internal/api/api.go
make test PKG=./cmd/lifeos
```
Expected: the run fails. The output contains `POST /api/habits/log: status 404`.

Run:
```bash
git checkout internal/api/api.go
make test PKG=./cmd/lifeos
```
Expected: the run passes: `ok  	github.com/Gautam-J/lifeos-core/cmd/lifeos`.

- [ ] **Step 3: Commit**

Run:
```bash
git add cmd/lifeos/daily_test.go
git commit -m "test: take a day of records through every adapter"
```

- [ ] **Step 4: Update `AGENTS.md`**

The edit adds one sentence to rule 9 and adds the rules 10 and 11.

In `AGENTS.md`, replace this:

```
Add each new record type to `RecordTypes`, to these three functions, to `Verify` and to `ReviewQueue`.
```

With this:

```
Add each new record type to `RecordTypes`, to these three functions, to `Verify` and to `ReviewQueue`. The record types are `item`, `journal`, `todo` and `habit_log`. A habit (the definition) is not a record type: only the admin pages add or change one.
10. **The journal holds Gautam's own words.** The service stores the body of a journal entry exactly as it comes and never rewords it. The `journal` tool writes a verified row, because no agent infers a journal entry. An admin handler changes the CR LF of a text area to LF, which is what Gautam typed.
11. **A habit has one log for each day.** A second log for the same habit and day is not a write: the service returns the stored log. The one exception: a verified log over a log that is not verified approves that log. "Done" for a day means a verified log.
```

- [ ] **Step 5: Run all tests, then commit**

Run:
```bash
make test
```
Expected: the run passes. The output contains `ok  	github.com/Gautam-J/lifeos-core/internal/service` and `ok  	github.com/Gautam-J/lifeos-core/internal/admin`.

Run:
```bash
git add AGENTS.md
git commit -m "docs: describe the journal and habit rules in AGENTS.md"
```

- [ ] **Step 6: Smoke check on a throwaway database**

Put `sudo` before `docker` if the shell reports `permission denied`. The directory `bin/` is in `.gitignore`.

Run:
```bash
cd /home/gautam/lifeos-core
make build
docker compose -f deploy/docker-compose.yml exec -T postgres psql -U lifeos -d postgres -c 'CREATE DATABASE lifeos_smoke'
sed 's|/lifeos?sslmode|/lifeos_smoke?sslmode|; s|8200|8290|; s|8100|8190|' config/lifeos.yaml > bin/smoke.yaml
set -a; . deploy/.env; set +a
bin/lifeos migrate -config bin/smoke.yaml
bin/lifeos serve -config bin/smoke.yaml > bin/smoke.log 2>&1 &
echo $! > bin/smoke.pid
sleep 2; cat bin/smoke.log
```
Expected: the run passes. The output contains `CREATE DATABASE`, `applied 00002_daily_records.sql` and `lifeos is listening http=127.0.0.1:8290 mcp=127.0.0.1:8190`.

Run:
```bash
cd /home/gautam/lifeos-core
J='Content-Type: application/json'
A='Accept: application/json, text/event-stream'
curl -s -X POST http://127.0.0.1:8190/mcp -H "$J" -H "$A" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"journal","arguments":{"body":"planned the tunnel.\nmust buy a domain"}}}' > /dev/null
curl -s -X POST http://127.0.0.1:8190/mcp -H "$J" -H "$A" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"todo","arguments":{"title":"Buy a domain","project":"lifeos"}}}' > /dev/null
curl -s http://127.0.0.1:8290/api/review; echo
curl -s -X POST http://127.0.0.1:8290/admin/review/verify -d 'type=todo&id=1' | tr -d '\n\t'; echo
curl -s -X PATCH http://127.0.0.1:8290/api/todos/1 -H "$J" -d '{"status":"done"}'; echo
curl -s -X POST http://127.0.0.1:8290/api/habits/log -H "$J" -d '{"habit":"gym"}'; echo
curl -s -o /dev/null -w 'add habit %{http_code}\n' -X POST http://127.0.0.1:8290/admin/habits -d 'name=Gym'
curl -s -X POST http://127.0.0.1:8290/api/habits/log -H "$J" -d '{"habit":"gym"}'; echo
curl -s -X POST http://127.0.0.1:8290/api/habits/log -H "$J" -d '{"habit":"gym"}' > /dev/null
curl -s http://127.0.0.1:8290/api/habits/today; echo
curl -s -X POST http://127.0.0.1:8190/mcp -H "$J" -H "$A" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"update","arguments":{"type":"journal","id":1,"fields":{"body":"Planned the tunnel."}}}}'; echo
for p in journal todos habits review; do curl -s -o /dev/null -w "/admin/$p %{http_code}\n" http://127.0.0.1:8290/admin/$p; done
docker compose -f deploy/docker-compose.yml exec -T postgres psql -U lifeos -d lifeos_smoke -At \
  -c 'SELECT action, record_type, source FROM events ORDER BY id'
```
Expected: the run passes. The output contains `todo: Buy a domain, project lifeos`, `No record waits for review.`, `"status":"done"`, `there is no active habit`, `add habit 200`, `"name":"gym","done":true`, `dictated: must be true to change a verified record`, `/admin/habits 200` and `create|habit_logs|manual`.

The output has these parts, in this order:
- The review queue has the todo, with `"source":"agent"`.
- The approve response is `<div id="list"><p class="muted">No record waits for review.</p></div>`.
- The PATCH response has `"status":"done"`, a `done_at` and `"verified":true`.
- The first habit log is refused with status 422, because no habit exists yet.
- After `add habit 200`, the log response has `"habit":"gym"`, `"source":"manual"` and `"verified":true`.
- The checklist has `"done":true` for gym.
- The MCP update of the journal entry has `"isError":true`: an agent cannot reword an entry.
- Each of the four admin pages answers 200.
- The events are these six lines. The second log of gym appended no event.

```
create|journal_entries|agent
create|todos|agent
verify|todos|manual
update|todos|manual
create|habits|manual
create|habit_logs|manual
```

Run:
```bash
cd /home/gautam/lifeos-core
kill "$(cat bin/smoke.pid)"; sleep 1
docker compose -f deploy/docker-compose.yml exec -T postgres psql -U lifeos -d postgres -c 'DROP DATABASE lifeos_smoke'
rm bin/smoke.yaml bin/smoke.log bin/smoke.pid
```
Expected: the run passes. The output contains `DROP DATABASE`.

- [ ] **Step 7: Apply the migration to the real database**

This is the first step that changes the real database. The migration adds four tables. It does not change a table that exists.

Run:
```bash
cd /home/gautam/lifeos-core
make migrate
```
Expected: the run passes. The output contains `applied 00002_daily_records.sql`.

From this point, do not edit `migrations/00002_daily_records.sql`. A change to a table needs a new migration file.

- [ ] **Step 8: Start the new server on the real database**

The tmux window `lifeos-serve` in the session `lifeos-core` runs the server of plan 2. Stop it and start the new build there.

Run:
```bash
tmux send-keys -t lifeos-core:lifeos-serve C-c
tmux send-keys -t lifeos-core:lifeos-serve 'make run' Enter
sleep 25; tmux capture-pane -p -t lifeos-core:lifeos-serve | tail -3
```
Expected: the run passes. The output contains `lifeos is listening http=127.0.0.1:8200 mcp=127.0.0.1:8100`.

If the window does not exist, run `make run` in a tmux pane that stays open.

- [ ] **Step 9: Hand over to Gautam and stop**

Give Gautam these three checks.

1. **Admin pages.** On the Mac, run `ssh -L 8200:127.0.0.1:8200 <the Pi>` and open `http://127.0.0.1:8200/admin/habits`.
   - Add the habit `gym`. Click Log. The checklist must show `done`, and the list of logs must show one log.
   - Open Journal. Add an entry that has two paragraphs. The list must show the two paragraphs. Click Edit, then Save with no change: the entry must stay the same.
   - Open Todos. Add a todo, then click Done. The status must change to `done`, and the Done button must go.
2. **MCP from Claude Code on the Pi.** Start a new `claude` session in the folder that has the `lifeos` server. A session that was open before Step 8 does not have the new tools.
   - Say "journal - tested the new tables today". The entry must show on the Journal page, verified.
   - Say "todo - buy cloudflare domain and set up tunnel, project lifeos". Two todos must show on the Todos page, verified.
   - Say "log gym". Claude must report the log of today, and the Habits page must still show one log for today.
   - Say "From my notes: 'renew the insurance before December'. Propose a todo for this; I did not state it as a todo." The todo must show at `http://127.0.0.1:8200/admin/review`. Click Approve.
3. **REST, as the Siri Shortcut does.** Run these two commands on the Pi. The first response must have `"habit":"gym"`. The second must have `"done":true` for gym.

```bash
curl -s -X POST http://127.0.0.1:8200/api/habits/log -H 'Content-Type: application/json' -d '{"habit": "gym"}'; echo
curl -s http://127.0.0.1:8200/api/habits/today; echo
```

The confirmation dialogs, the reset of a form after Add and the look of the pages depend on a real browser. No automated test covers them.

Stop here. Do not start the work in "After this plan" until Gautam confirms that the three checks pass.

---

## After this plan

These are the parts of Phase 2 that this plan does not cover. Each becomes its own plan.

| Plan | Content | Spec sections |
| --- | --- | --- |
| 4. Capture and search | `sources` with `capture` and the SHA-256 file store. `search`, which also lets an agent find a todo or a journal entry. The read-only Postgres user for `query`. The rejection of card numbers and secrets in captured text, in a journal body and in titles. | Access layer; S5 to S7, S12, S13; E1, E8 |
| 5. Money and importers | `accounts`, `transactions`, `budgets`. `lifeos import-csv` for a bank statement, Letterboxd and Goodreads, with duplicate detection. | Data model; S6, S11 |
| 6. Run and protect | A systemd unit for `lifeos serve` and `deploy/deploy.sh`. The admin password, the per-device API tokens and CSRF protection. Tunnel hostnames with Cloudflare Access; the Host header setting for the MCP server. Permanent delete in the admin pages. | Deployment and security; S25, S26 |

`docs/backlog.md` holds work that is in no plan yet: a schema for the `attrs` of each item kind, duplicate detection for items, a `description` field for todos, and a `projects` table.

Phase 2 ends with the gate in the design doc: one week of daily logging through the admin pages. The `wiki_*` tools, `brief`, OAuth for the MCP endpoint, the jobs and the connectors belong to Phases 3 to 5.
