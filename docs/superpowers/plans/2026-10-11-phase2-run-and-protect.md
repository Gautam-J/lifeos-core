# Phase 2 Run and Protect Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** LifeOS runs on the Pi as a service that starts at boot, Gautam reaches the admin pages and the REST API from the internet through Cloudflare Access, and he can remove a deleted record for good.

**Architecture:** `internal/service` gets `Purge` and `ListDeleted`, and one admin page calls them. `lifeos serve` puts `http.CrossOriginProtection` of the Go standard library in front of the admin pages and the API. A systemd unit runs the binary, `deploy/deploy.sh` ships a new version, and `deploy/check-access.sh` proves that Cloudflare Access guards each public hostname. LifeOS itself gets no login: Cloudflare Access is the only lock.

**Tech Stack:** No new dependency and no migration. Go 1.26.0 (`http.CrossOriginProtection`, `os.Root`), chi v5.3.2, Huma v2.39.1, pgx v5.11.0, sqlc v1.31.1, templ v0.3.1070, htmx 2.0.11, Postgres 18 in Docker, systemd 252, cloudflared 2026.10.0, Cloudflare Access (Zero Trust).

**Spec:** `docs/design.md` (section "Deployment and security"; workflows S25 "Delete a record" and S26 "Ship new code"; edge case E13) and `AGENTS.md`. Read both before you start a task. The section "Decisions in this plan" lists where this plan differs from the design doc: those decisions take precedence.

**Scope:** This is plan 5 of 5 of Phase 2. Plans 1 to 4 are complete and confirmed. After this plan, the gate of Phase 2 starts: one week of daily logging through the admin pages. Money tables, the reconciliation with a bank statement and the one-time imports are in `docs/backlog.md`, not in a plan.

**Tested:** Each code block of Tasks 1 to 6 and of Task 9 comes from a scratch copy of the repository. In that copy each test failed first, then passed after its code was added, in the task order below. Then the steps of those tasks were done one more time on a fresh clone, by a script that reads this file. Tasks 7 and 8 change the real system, so they were not done before:

- The unit file of Task 6 ran as a throwaway unit (`systemd-run`) with its protection settings, on other ports and a scratch database. It stored a file.
- The tunnel rules of Task 8 were checked on a scratch copy of the tunnel config with `cloudflared tunnel ingress rule`.
- The script of Task 8 ran with `--dry-run`. Its three calls that make something (two applications, one service token) are from the Cloudflare API reference and from the shape of the application that exists. They did not run. The API token can lack the right to write.

## Changes after the review

One fresh reviewer read the code of Tasks 1 to 6 and the steps of Tasks 7 and 8, before Task 7. The review found no critical defect. These points changed. The code blocks of Tasks 1 to 6 below show the code before these changes; the repository has the code after them.

| Finding | Change | Commit or place |
| --- | --- | --- |
| The Task 8 script copied the whole list of the first policy that it found, and did not show it. | It copies only the email rules, stops when the source policy has a `require` or an `exclude` part, and prints each address with its name masked. It stops for an application that has no policy. | Task 8, Step 3 |
| The Task 8 rollback took the newest backup by file time. `cp -p` keeps the time of the source, so a second run could pick a backup that has the new rules. | The backup is made after the check for the two hostnames, and its path is in `bin/cloudflared-config.backup`. | Task 8, Steps 6 and 7 |
| `deploy/check-access.sh` took 401 and 403 from each hostname as proof of Access. A firewall rule of Cloudflare can give 403. | Only an answer that Access alone gives is proof: 302 to its login page, and 401 for the application that takes a service token. `deploy/test.sh` checks this against a table of answers; `make test` runs it. | `fix(deploy): take only an answer of Access as proof in the Access check` |
| The tunnel sent each request that the edge let through. | Each of the two tunnel rules has an `access` part: `cloudflared` checks the token of Access itself. A removed application fails closed. | Task 8, Step 6 |
| The server process got each value of `deploy/.env`, could read the home folder and could use the Docker socket. | The unit gives it the two database passwords, the binary, the config and the file store, and nothing else of these. | `chore(deploy): give the server only what it needs` |
| `Requires=docker.service` stopped the server for good when Docker restarted. | `Wants=docker.service`. Thirty starts in ten minutes before the unit fails. | The same commit |
| A deleted file source stayed for good when the folder of its file was gone, as after a restore without the file store. | `Purge` accepts a folder that is not there. | `fix(service): remove a deleted source when the folder of its file is gone` |
| `deploy/deploy.sh` did not reload systemd, so a pulled change of the unit file did not apply. | `sudo systemctl daemon-reload` before the restart. | `fix(deploy): reload systemd before the restart` |

Two results of the run on 2026-10-11 differ from the steps below:

- **Task 8, Step 4.** The Cloudflare API token made the two applications and could not make the service token (HTTP 403). Gautam made the token `lifeos-pi-check` in the dashboard and put its two values into `deploy/.env`. After that, `deploy/check-access.sh` printed five `ok` lines.
- **Task 8, Step 5.** Without the service token, the check before the tunnel rules printed three `ok` lines and no `FAIL` line.

Gautam confirmed the gate of Task 10 on 2026-10-11: the login from a browser works, and the page "Deleted" removed two records for good.

One statement of this plan was wrong: "A second capture of the file repairs it" (Review Focus, the capture at the same moment as a permanent delete). For a file from the inbox, the copy in the inbox is gone after the capture, so nothing is left to capture again. The limit stays, and it is in the list of open points.

## Decisions in this plan

Gautam approved these on 2026-10-11.

**No login in LifeOS (option A).** The design doc gives each hostname two locks: Cloudflare Access, then an admin password or an API token of the app. This plan builds only the first lock. `lifeos` has no password, no session and no token table. Thus:

- One wrong Access setting opens all data. `deploy/check-access.sh` is the guard: it fails when a hostname answers a request that has no login. `deploy/deploy.sh` runs it after each deploy.
- The Cloudflare account is the one lock. Protect it with two-factor login.
- A request to `127.0.0.1:8200` on the Pi needs no login, as before.

**Hostnames.** `admin.<domain>` for the admin pages and `api.<domain>` for the REST API, where `<domain>` is the domain that the tunnel has already. The two names are in `deploy/.env` (`LIFEOS_ADMIN_HOST`, `LIFEOS_API_HOST`), not in git. The tunnel sends only `/` and `/admin...` of the first hostname and only `/api...` of the second one to `lifeos`.

- Admin: an Access application with the same people as an application that exists (a login with an email code), session 730 hours.
- API: an Access application with the decision "Service Auth". A request needs a valid Access service token in the headers `CF-Access-Client-Id` and `CF-Access-Client-Secret`. Each service token of the Cloudflare account gets through. Gautam makes one token for each device in the Cloudflare dashboard, and removes it there.
- The Pi gets one service token, `lifeos-pi-check`, for `deploy/check-access.sh`. It does not expire.

**The API port.** The design doc says `8000`. A different service of this Pi uses `8000`, so `http_addr` is `127.0.0.1:8200` (plan 1).

**Where the service runs.** In `/home/gautam/lifeos-core` as the user `gautam`, as the backup unit does. The design doc says `/srv/lifeos` with a service account; that is more setup with no gain for one user.

**Permanent delete.**

- One page, `/admin/deleted`, lists the deleted records of all five types. Only this page calls `Purge`. No REST route and no MCP tool can.
- `Purge` takes only a record that is deleted already (`deleted_at` is set). Thus a permanent delete is always the second of two steps.
- In one transaction, `Purge` removes the row, empties `old_value` and `new_value` of each earlier event of the record, and appends a `purge` event that has no value. The event rows stay. This is the one exception to "`events` is append-only".
- A source that a record names in `source_ref` cannot go. The error names the table of that record. Remove that record for good first.
- The file of a file source goes too, unless a different row of `sources` (live or deleted) has the same `file_path`. The file goes before the commit: when the file cannot go, the row stays.
- Not in this plan: a way to get a deleted record back.

**Requests from another site (CSRF).** The browser sends the Access login with each request to the hostname, also with one that a page of a different site makes. `http.CrossOriginProtection` refuses a POST, PATCH or DELETE that a browser sent from a different origin (`Sec-Fetch-Site`, or `Origin` for an old browser). A program sends neither header and goes through. No token in the forms.

**The MCP port stays local.** It gets no hostname in this plan. Phase 3 gives it one: a login through "Managed OAuth" of Cloudflare Access, and a check of the Access token inside `lifeos`, because the MCP tools can change and delete all data.

**Deploy.** `deploy/deploy.sh` dumps the database before each `lifeos migrate` and keeps the last five dumps in `/var/backups/lifeos/deploy/`. The design doc asks for a fresh dump before a migration that drops or renames a column (S26); a dump before each deploy needs no decision by a person.

**Not in this plan.**

- A Postgres user for the app that is not a superuser.
- Old backups: a record that Gautam removes for good stays in the restic snapshots that were made before.
- A change of `docs/design.md`. Its table of hostnames still shows the admin password and the API token.
- `EnvironmentFile` gives the service all values of `deploy/.env`, also the ones for the backup. Phase 4 starts `claude -p` from a job: that code must give its child process an environment without them.

## Prerequisites

Step 1 of Task 1 checks these.

- Plan 4 is complete: the database of the Pi is at migration version 4, and the Postgres user `lifeos_query` exists.
- `deploy/.env` has `POSTGRES_PASSWORD`, `LIFEOS_QUERY_PASSWORD`, `NTFY_BASE_URL`, `CLOUDFLARE_ACCOUNT_ID` and `CLOUDFLARE_API_TOKEN`. Never print a value. The Cloudflare token ends on 2026-10-29; only Task 8 needs it.
- `cloudflared` runs as a systemd service with the config `/etc/cloudflared/config.yml`, and `~/.cloudflared/cert.pem` exists (Phase 1).
- The user `gautam` can run `sudo` without a password.

## Global Constraints

- **One logic layer.** All validation, provenance and event code is in `internal/service`. An adapter (`internal/api`, `internal/mcp`, `internal/admin`, `cmd/lifeos`) parses input, calls the service and formats the result. No adapter calls a different adapter.
- **SQL only through sqlc.** Queries are in `queries/*.sql`. Only `internal/service` imports `internal/store`. No SQL strings in other non-test Go code.
- **One event for each write.** The service appends one `events` row in the same transaction as the write.
- **Permanent removal only in the admin pages.** Do not add a REST route or an MCP tool that calls `Purge`.
- **Secrets.** Never print, log or commit the value of a variable in `deploy/.env`. A script that makes a secret writes it to `deploy/.env` and prints only its name.
- **The domain is not in git.** Write a hostname only to `deploy/.env` and to `/etc/cloudflared/config.yml`. A file in git reads `LIFEOS_ADMIN_HOST` and `LIFEOS_API_HOST`.
- **A hostname comes after its Access application.** Never add a hostname to the tunnel, and never make its DNS record, before its Access application has a policy. Run `deploy/check-access.sh` after each change.
- **Tests never touch `/var/lib/lifeos`.** A test that needs a file store uses `t.TempDir()`.
- **Time.** Store `timestamptz` in UTC. Show times in IST.
- **Loopback only.** Every listener is on `127.0.0.1`. The MCP port gets no hostname in this plan.
- **No migration.** This plan changes no table. Do not edit `00001` to `00004`: they are applied on the Pi.
- **Generated code is not in git.** `internal/store/` (sqlc) and `*_templ.go` (templ) are in `.gitignore`. Never `git add` them. `make build` and `make test` generate them first.
- **Tests use real Postgres.** Run tests with `make test`, not `go test`. Each test creates its own database and drops it.
- **Where to work.** Work in `/home/gautam/lifeos-core` on `main`. Do not make a branch or a worktree: `deploy/.env` is not in git and exists only in this checkout. Do not push.
- **Commits.** Conventional commit format, one concern for each commit, with the exact file lists in the steps.
- **Ask first.** Task 7 needs `sudo` to install the unit. Task 8 changes Cloudflare and the tunnel. Each of the two tasks starts with a step that gets Gautam's OK.

## Review Focus

Each line is a condition that a person meets in use. The test that pins it is in the task that owns the code.

1. **A double click on "Delete forever".** The same request comes two times at the same moment. One removes the record, the other finds none, and no internal error shows. Test: `TestPurgeOfTheSameRecordAtTheSameMomentRemovesItOneTime` (Task 1).
2. **The same file comes in again after its permanent delete.** The file store must take the file again. Test: `TestCaptureAfterAPurgeStoresTheFileAgain` (Task 2).
3. **A deleted log of a habit that is retired.** The list of deleted records must show it, or Gautam cannot remove it. Test: `TestListDeletedShowsTheLogOfAHabitThatIsRetired` (Task 3).
4. **A form post from a frame that has no origin** (`Origin: null`). The server must refuse it as a request from another site. Test: the case "an admin form from a frame that has no origin" in `TestServeRefusesAPostFromAnotherSite` (Task 5).
5. **A hostname that answers without a login.** `deploy/check-access.sh` must print `OPEN` and fail. Steps 6 and 7 of Task 6 run the script against a hostname that is open and against one that Access guards.

The reviewer must also read these, which no test covers:

- **The order in `deploy/deploy.sh`.** A failed build or a failed dump must stop the script before `lifeos migrate`.
- **A capture of the same file at the same moment as its permanent delete.** `Capture` can find the file, then `Purge` removes it, then `Capture` stores its row. The row then has no file. A second capture of the file repairs it. This plan accepts that limit for one user.
- **The order in Task 8.** No step may make a DNS record or a tunnel rule for a hostname before its Access application has a policy.

## File Structure

| File | Task | Responsibility |
| --- | --- | --- |
| `queries/catalog_items.sql`, `journal_entries.sql`, `todos.sql`, `habit_logs.sql`, `sources.sql` | 1, 2, 3 | `Purge<Row>` and `ListDeleted<Rows>` for each record table; `CountSourcesOfFile` |
| `queries/events.sql` | 1 | `EmptyEventValues` |
| `internal/service/purge.go` | 1, 2 | `Purge`, `purgeRow`, `removeStoredFile` |
| `internal/service/deleted.go` | 3 | `DeletedRecord`, `ListDeleted` |
| `internal/service/purge_test.go`, `purge_source_test.go`, `deleted_test.go` | 1, 2, 3 | Tests of the service |
| `internal/admin/deleted.go`, `deleted.templ` | 4 | The page `/admin/deleted` and its form post |
| `internal/admin/admin.go` | 4, 5 | The two routes of the page; the redirect of `/`, `/admin` and `/admin/` |
| `internal/admin/items.templ` | 4 | The link "Deleted" in the menu of each page |
| `internal/admin/deleted_test.go`, `home_test.go` | 4, 5 | Tests of the admin pages |
| `cmd/lifeos/serve.go` | 5 | `http.CrossOriginProtection` in front of the router |
| `cmd/lifeos/protect_test.go` | 5 | Test through the real HTTP server |
| `deploy/systemd/lifeos.service` | 6 | The unit of `lifeos serve` |
| `deploy/deploy.sh` | 6 | Pull, build, dump, migrate, restart, check |
| `deploy/check-access.sh` | 6 | Fails when a public hostname answers a request that has no login |
| `deploy/.env.example` | 6 | The four new keys |
| `bin/cloudflare-access.py` | 8 | One-time setup of Access. Not in git (`/bin/` is in `.gitignore`) |
| `AGENTS.md`, `README.md` | 9 | Rules 5, 6, 9 and 12, the Deploy section; README sections 6, 7 and 13 |

## How to read the steps

A step that changes a file uses one of these forms. Do each form exactly.

- **Create `path`:** make the new file with the content of the code block.
- **In `path`, replace this: ... With this: ...** The first code block is in the file one time. Put the second code block in its place. When a step has more than one of these for a file, do them from the first to the last.

Go code uses tabs. Copy the code blocks with their tabs.

Put `sudo` before `docker` if the shell reports `permission denied` for Docker. For a script that calls Docker itself, use `sg docker -c "<the command>"`.

---

### Task 1: Remove a deleted record for good

**Files:**
- Create: `internal/service/purge.go`
- Modify: `queries/catalog_items.sql`, `queries/journal_entries.sql`, `queries/todos.sql`, `queries/habit_logs.sql`, `queries/events.sql`
- Test: `internal/service/purge_test.go`

**Interfaces:**
- Consumes (plans 1 to 4, package `service`): `checkRecordType(recordType string) error`, `errNoCode(recordType string) error`, `recordEvent(ctx, q, action, recordType string, recordID int64, source Source, oldValue, newValue any) error`, `ErrNotFound`, `SourceManual`, `(*Service).Delete(ctx, recordType string, id int64, p DeleteParams) error`, `(*Service).Update`. The test helpers `newTestService`, `addItem`, `addJournal`, `addTodo`, `addHabit`, `logHabit`, `countRows`, `allEvents`, `equal`.
- Produces (package `service`):
  - `(*Service).Purge(ctx context.Context, recordType string, id int64) error`. It returns a `ValidationError` for the field `type` when `recordType` is not a record type, and `ErrNotFound` when no deleted record has the ID. In this task it works for `item`, `journal`, `todo` and `habit_log`; Task 2 adds `source`.
  - `purgeRow(ctx context.Context, q *store.Queries, recordType string, id int64) (table string, err error)`. Task 2 changes its results.
  - Queries: `PurgeCatalogItem`, `PurgeJournalEntry`, `PurgeTodo`, `PurgeHabitLog` (each `:one`, returns the `id`), `EmptyEventValues` (`:exec`, parameters `RecordType string`, `RecordID *int64`).
  - Test helper: `deleteByHand(t *testing.T, s *Service, recordType string, id int64)`.

- [ ] **Step 1: Check the prerequisites**

Run:
```bash
cd /home/gautam/lifeos-core
git status -sb
docker compose -f deploy/docker-compose.yml ps --format '{{.Name}}: {{.Status}}'
grep -c -E '^(POSTGRES_PASSWORD|LIFEOS_QUERY_PASSWORD|NTFY_BASE_URL|CLOUDFLARE_ACCOUNT_ID|CLOUDFLARE_API_TOKEN)=.' deploy/.env
systemctl is-active cloudflared
ls ~/.cloudflared/cert.pem /etc/cloudflared/config.yml
sudo -n true && echo "sudo: no password"
make test
```
Expected: the run passes. The output contains `lifeos-postgres-1: Up`, `cert.pem` and `sudo: no password`.

The branch is `main`. No tracked file is changed, except that this plan file can be new. The line of `grep` is `5`. `make test` prints `ok` for seven packages.

- [ ] **Step 2: Write the failing tests**

Create `internal/service/purge_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"
)

// deleteByHand marks one record as deleted, as the admin pages do.
func deleteByHand(t *testing.T, s *Service, recordType string, id int64) {
	t.Helper()
	if err := s.Delete(context.Background(), recordType, id, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("Delete %s %d: %v", recordType, id, err)
	}
}

func TestPurgeRemovesADeletedRecordOfEachType(t *testing.T) {
	s, pool := newTestService(t)
	addHabit(t, s, "gym")
	records := []struct {
		recordType, table string
		id                int64
	}{
		{"item", "catalog_items", addItem(t, s, "book", "Dune").ID},
		{"journal", "journal_entries", addJournal(t, s, "a private day", "2026-10-09").ID},
		{"todo", "todos", addTodo(t, s, "Buy a domain").ID},
		{"habit_log", "habit_logs", logHabit(t, s, "gym", "2026-10-09").ID},
	}

	for _, r := range records {
		deleteByHand(t, s, r.recordType, r.id)
		if err := s.Purge(context.Background(), r.recordType, r.id); err != nil {
			t.Errorf("Purge %s: %v", r.recordType, err)
			continue
		}
		if n := countRows(t, pool, r.table); n != 0 {
			t.Errorf("%s has %d rows after the purge, want 0", r.table, n)
		}
	}
}

func TestPurgeEmptiesTheValuesOfTheEarlierEvents(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	secret := addItem(t, s, "book", "A private title")
	kept := addItem(t, s, "book", "Dune")
	_, err := s.Update(ctx, "item", secret.ID, UpdateParams{Fields: map[string]any{"title": "A second private title"}, Actor: SourceManual})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	deleteByHand(t, s, "item", secret.ID)

	if err := s.Purge(ctx, "item", secret.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	var actions []string
	for _, e := range allEvents(t, pool) {
		if e.RecordID == kept.ID {
			if e.New["title"] != "Dune" {
				t.Errorf("the event of the other record lost its value: %v", e.New)
			}
			continue
		}
		actions = append(actions, e.Action)
		if e.Old != nil || e.New != nil {
			t.Errorf("the %s event keeps a value: old %v, new %v", e.Action, e.Old, e.New)
		}
		if e.RecordType != "catalog_items" {
			t.Errorf("the %s event has the record type %q, want catalog_items", e.Action, e.RecordType)
		}
	}
	if want := []string{"create", "update", "delete", "purge"}; !equal(actions, want) {
		t.Errorf("the events of the record are %v, want %v", actions, want)
	}
	events := allEvents(t, pool)
	if last := events[len(events)-1]; last.Action != "purge" || last.Source != "manual" {
		t.Errorf("the last event is %s by %s, want purge by manual", last.Action, last.Source)
	}
}

func TestPurgeRefusesARecordThatIsNotDeleted(t *testing.T) {
	s, pool := newTestService(t)
	item := addItem(t, s, "book", "Dune")

	err := s.Purge(context.Background(), "item", item.ID)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if n := countRows(t, pool, "catalog_items"); n != 1 {
		t.Errorf("catalog_items has %d rows, want 1", n)
	}
	events := allEvents(t, pool)
	if len(events) != 1 || events[0].New["title"] != "Dune" {
		t.Errorf("the events changed: %+v", events)
	}
}

func TestPurgeLeavesTheOtherDeletedRecords(t *testing.T) {
	s, pool := newTestService(t)
	first := addItem(t, s, "book", "Dune")
	second := addItem(t, s, "book", "Emma")
	deleteByHand(t, s, "item", first.ID)
	deleteByHand(t, s, "item", second.ID)

	if err := s.Purge(context.Background(), "item", first.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	if n := countRows(t, pool, "catalog_items"); n != 1 {
		t.Errorf("catalog_items has %d rows, want 1", n)
	}
	// A todo with the same ID is a different record.
	todo := addTodo(t, s, "Buy a domain")
	deleteByHand(t, s, "todo", todo.ID)
	if err := s.Purge(context.Background(), "item", second.ID); err != nil {
		t.Fatalf("Purge the second item: %v", err)
	}
	if n := countRows(t, pool, "todos"); n != 1 {
		t.Errorf("todos has %d rows, want 1", n)
	}
	for _, e := range allEvents(t, pool) {
		if e.RecordType == "todos" && e.Action == "create" && e.New["title"] != "Buy a domain" {
			t.Errorf("the create event of the todo lost its value: %v", e.New)
		}
	}
}

func TestPurgeRefusesATypeThatIsNotARecordTypeAndARecordThatDoesNotExist(t *testing.T) {
	s, _ := newTestService(t)

	err := s.Purge(context.Background(), "habit", 1)
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "type" {
		t.Errorf("type habit: error = %v, want a ValidationError for type", err)
	}

	if err := s.Purge(context.Background(), "item", 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("item 999: error = %v, want ErrNotFound", err)
	}
}

// A double click sends the same request two times.
func TestPurgeOfTheSameRecordAtTheSameMomentRemovesItOneTime(t *testing.T) {
	s, pool := newTestService(t)
	item := addItem(t, s, "book", "Dune")
	deleteByHand(t, s, "item", item.ID)

	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- s.Purge(context.Background(), "item", item.ID) }()
	}
	var done, missing int
	for range 2 {
		switch err := <-errs; {
		case err == nil:
			done++
		case errors.Is(err, ErrNotFound):
			missing++
		default:
			t.Errorf("Purge: %v", err)
		}
	}

	if done != 1 || missing != 1 {
		t.Errorf("%d calls removed the record and %d found none, want 1 and 1", done, missing)
	}
	var purges int
	for _, e := range allEvents(t, pool) {
		if e.Action == "purge" {
			purges++
		}
	}
	if purges != 1 {
		t.Errorf("the log has %d purge events, want 1", purges)
	}
}
```

- [ ] **Step 3: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run Purge"
```
Expected: the run fails. The output contains `s.Purge undefined`.

- [ ] **Step 4: Add the queries**

Each record table gets a query that removes a row. The condition `deleted_at IS NOT NULL` is the rule "only a record that is deleted already".

In `queries/catalog_items.sql`, replace this:

```sql
SET verified = true
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;
```

With this:

```sql
SET verified = true
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: PurgeCatalogItem :one
-- Removes a row for good. Only a row that is deleted already can go.
DELETE FROM catalog_items
WHERE id = $1 AND deleted_at IS NOT NULL
RETURNING id;
```

In `queries/journal_entries.sql`, replace this:

```sql
SET verified = true
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;
```

With this:

```sql
SET verified = true
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: PurgeJournalEntry :one
-- Removes a row for good. Only a row that is deleted already can go.
DELETE FROM journal_entries
WHERE id = $1 AND deleted_at IS NOT NULL
RETURNING id;
```

In `queries/todos.sql`, replace this:

```sql
SET verified = true
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;
```

With this:

```sql
SET verified = true
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: PurgeTodo :one
-- Removes a row for good. Only a row that is deleted already can go.
DELETE FROM todos
WHERE id = $1 AND deleted_at IS NOT NULL
RETURNING id;
```

In `queries/habit_logs.sql`, replace this:

```sql
ORDER BY habit_logs.created_at, habit_logs.id
LIMIT $1;
```

With this:

```sql
ORDER BY habit_logs.created_at, habit_logs.id
LIMIT $1;

-- name: PurgeHabitLog :one
-- Removes a row for good. Only a row that is deleted already can go.
DELETE FROM habit_logs
WHERE id = $1 AND deleted_at IS NOT NULL
RETURNING id;
```

In `queries/events.sql`, replace this:

```sql
VALUES ($1, $2, $3, $4, $5, $6);
```

With this:

```sql
VALUES ($1, $2, $3, $4, $5, $6);

-- name: EmptyEventValues :exec
-- Takes the content of one record out of the log. The rows stay, with
-- their time and their action.
UPDATE events
SET old_value = NULL, new_value = NULL
WHERE record_type = $1 AND record_id = $2;
```

- [ ] **Step 5: Write the code**

Create `internal/service/purge.go`:

```go
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// Purge removes one record for good. Only the admin pages call it, so the
// change is Gautam's own. The record must be deleted already: Purge
// returns ErrNotFound for a live record and for an ID that no record has.
//
// In one transaction, Purge removes the row, empties the values of the
// earlier events of the record and appends a purge event that has no
// value. The events stay, so the log shows that the record was there.
func (s *Service) Purge(ctx context.Context, recordType string, id int64) error {
	if err := checkRecordType(recordType); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("purge: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	table, err := purgeRow(ctx, q, recordType, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("purge from %s: %w", table, err)
	}
	if err := q.EmptyEventValues(ctx, store.EmptyEventValuesParams{RecordType: table, RecordID: &id}); err != nil {
		return fmt.Errorf("purge from %s: %w", table, err)
	}
	if err := recordEvent(ctx, q, "purge", table, id, SourceManual, nil, nil); err != nil {
		return fmt.Errorf("purge from %s: %w", table, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("purge from %s: %w", table, err)
	}
	return nil
}

// purgeRow removes the row of one deleted record and returns its table.
func purgeRow(ctx context.Context, q *store.Queries, recordType string, id int64) (table string, err error) {
	switch recordType {
	case "item":
		_, err = q.PurgeCatalogItem(ctx, id)
		return "catalog_items", err
	case "journal":
		_, err = q.PurgeJournalEntry(ctx, id)
		return "journal_entries", err
	case "todo":
		_, err = q.PurgeTodo(ctx, id)
		return "todos", err
	case "habit_log":
		_, err = q.PurgeHabitLog(ctx, id)
		return "habit_logs", err
	}
	return "", errNoCode(recordType)
}
```

- [ ] **Step 6: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./internal/service ARGS="-run Purge -v"
```
Expected: the run passes. The output contains `PASS: TestPurgeRemovesADeletedRecordOfEachType`, `PASS: TestPurgeEmptiesTheValuesOfTheEarlierEvents`, `PASS: TestPurgeRefusesARecordThatIsNotDeleted`, `PASS: TestPurgeLeavesTheOtherDeletedRecords`, `PASS: TestPurgeRefusesATypeThatIsNotARecordTypeAndARecordThatDoesNotExist` and `PASS: TestPurgeOfTheSameRecordAtTheSameMomentRemovesItOneTime`.

- [ ] **Step 7: Commit**

Run:
```bash
git add internal/service/purge.go internal/service/purge_test.go queries/catalog_items.sql queries/journal_entries.sql queries/todos.sql queries/habit_logs.sql queries/events.sql
git commit -m "feat(service): remove a deleted record for good"
```

---

### Task 2: Remove a deleted source and its file

**Files:**
- Modify: `internal/service/purge.go`, `queries/sources.sql`
- Test: `internal/service/purge_source_test.go`

**Interfaces:**
- Consumes (Task 1): `(*Service).Purge`, `purgeRow`, `deleteByHand`.
- Consumes (plan 4, package `service`): `(*Service).Capture(ctx, CaptureParams) (Capture, error)` with `CaptureParams{Upload io.Reader, FileName string, Source Source}`, `(*Service).GetCapture`, `syncDir(path string) error`, `invalid(field, message string) error`, the field `s.files.Dir`. The test helpers `newFileService`, `storedFiles`, `captureText`, the test value `pdf`.
- Produces (package `service`):
  - `Purge` now takes the type `source`. A source that a record names in `source_ref` gives a `ValidationError` for the field `id`; its message is `a record in <table> names this source in its source_ref: delete that record forever first`. The file of a file source leaves the file store when no other row of `sources` has the same `file_path`.
  - `purgeRow(ctx, q, recordType string, id int64) (table string, file *string, err error)`: `file` is the path of the stored file below `Files.Dir`, for a file source.
  - `(*Service).removeStoredFile(path string) error`.
  - Queries: `PurgeSource` (`:one`, returns the row), `CountSourcesOfFile` (`:one`, parameter `*string`, returns `int64`).
  - Test helper: `captureUpload(t *testing.T, s *Service, name string) Capture`.

- [ ] **Step 1: Write the failing tests**

Create `internal/service/purge_source_test.go`:

```go
package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureUpload stores pdf as a file capture and returns the source.
func captureUpload(t *testing.T, s *Service, name string) Capture {
	t.Helper()
	capture, err := s.Capture(context.Background(), CaptureParams{Upload: bytes.NewReader(pdf), FileName: name, Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture %s: %v", name, err)
	}
	return capture
}

func TestPurgeRemovesADeletedSourceAndItsFile(t *testing.T) {
	s, pool := newFileService(t)
	source := captureUpload(t, s, "policy.pdf")
	deleteByHand(t, s, "source", source.ID)
	if got := storedFiles(t, s); len(got) != 1 {
		t.Fatalf("the file store has %v after the delete, want 1 file: a delete keeps the file", got)
	}

	if err := s.Purge(context.Background(), "source", source.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	if n := countRows(t, pool, "sources"); n != 0 {
		t.Errorf("sources has %d rows, want 0", n)
	}
	if got := storedFiles(t, s); len(got) != 0 {
		t.Errorf("the file store has %v, want no file", got)
	}
	for _, e := range allEvents(t, pool) {
		if e.Old != nil || e.New != nil {
			t.Errorf("the %s event keeps a value: old %v, new %v", e.Action, e.Old, e.New)
		}
	}
}

func TestPurgeKeepsAFileThatAnotherSourceUses(t *testing.T) {
	s, pool := newFileService(t)
	ctx := context.Background()
	first := captureUpload(t, s, "policy.pdf")
	deleteByHand(t, s, "source", first.ID)
	// The same bytes again: a new source that uses the same file.
	second := captureUpload(t, s, "policy-copy.pdf")
	if second.ID == first.ID {
		t.Fatalf("the second capture gave the deleted source %d", first.ID)
	}

	if err := s.Purge(ctx, "source", first.ID); err != nil {
		t.Fatalf("Purge the first source: %v", err)
	}
	if got := storedFiles(t, s); len(got) != 1 {
		t.Fatalf("the file store has %v, want the file of the second source", got)
	}

	// A deleted source also keeps the file for a later purge of its own.
	deleteByHand(t, s, "source", second.ID)
	third := captureUpload(t, s, "policy-third.pdf")
	if err := s.Purge(ctx, "source", second.ID); err != nil {
		t.Fatalf("Purge the second source: %v", err)
	}
	if got := storedFiles(t, s); len(got) != 1 {
		t.Errorf("the file store has %v, want the file of the third source", got)
	}
	if n := countRows(t, pool, "sources"); n != 1 {
		t.Errorf("sources has %d rows, want 1", n)
	}
	if _, err := s.GetCapture(ctx, third.ID); err != nil {
		t.Errorf("GetCapture of the third source: %v", err)
	}
}

func TestPurgeRemovesATextSourceFromTheTableAndFromTheEvents(t *testing.T) {
	s, pool := newTestService(t)
	source := captureText(t, s, "a private note that must go")
	deleteByHand(t, s, "source", source.ID)

	if err := s.Purge(context.Background(), "source", source.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	if n := countRows(t, pool, "sources"); n != 0 {
		t.Errorf("sources has %d rows, want 0", n)
	}
	var left int
	err := pool.QueryRow(context.Background(), "SELECT count(*) FROM events WHERE old_value::text LIKE '%private note%' OR new_value::text LIKE '%private note%'").Scan(&left)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if left != 0 {
		t.Errorf("%d events keep the text of the source", left)
	}
}

func TestPurgeRefusesASourceThatARecordNames(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	source := captureText(t, s, "Renew the policy before March.")
	todo, err := s.AddTodo(ctx, AddTodoParams{Title: "Renew the policy", Provenance: Provenance{Source: SourceAgent, SourceRef: &source.ID}})
	if err != nil {
		t.Fatalf("AddTodo: %v", err)
	}
	deleteByHand(t, s, "source", source.ID)

	err = s.Purge(ctx, "source", source.ID)

	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "id" {
		t.Fatalf("error = %v, want a ValidationError for id", err)
	}
	if !strings.Contains(invalid.Message, "todos") {
		t.Errorf("the message %q does not name the table todos", invalid.Message)
	}
	if n := countRows(t, pool, "sources"); n != 1 {
		t.Errorf("sources has %d rows, want 1: a refused purge removes nothing", n)
	}
	if events := allEvents(t, pool); events[0].New["content"] != "Renew the policy before March." {
		t.Errorf("a refused purge emptied the create event: %v", events[0].New)
	}

	// After the record is gone, the source can go.
	deleteByHand(t, s, "todo", todo.ID)
	if err := s.Purge(ctx, "todo", todo.ID); err != nil {
		t.Fatalf("Purge the todo: %v", err)
	}
	if err := s.Purge(ctx, "source", source.ID); err != nil {
		t.Errorf("Purge the source after the todo: %v", err)
	}
}

func TestPurgeOfASourceWhoseFileIsGoneAlreadyRemovesTheRow(t *testing.T) {
	s, pool := newFileService(t)
	source := captureUpload(t, s, "policy.pdf")
	deleteByHand(t, s, "source", source.ID)
	for _, path := range storedFiles(t, s) {
		if err := os.Remove(filepath.Join(s.files.Dir, path)); err != nil {
			t.Fatalf("remove %s: %v", path, err)
		}
	}

	if err := s.Purge(context.Background(), "source", source.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	if n := countRows(t, pool, "sources"); n != 0 {
		t.Errorf("sources has %d rows, want 0", n)
	}
}

func TestPurgeKeepsTheRowWhenTheFileCannotBeRemoved(t *testing.T) {
	s, pool := newFileService(t)
	source := captureUpload(t, s, "policy.pdf")
	deleteByHand(t, s, "source", source.ID)
	// The service loses its file store, as after a wrong config change.
	dir := s.files.Dir
	s.files.Dir = filepath.Join(dir, "no-such-folder")

	err := s.Purge(context.Background(), "source", source.ID)

	var invalid *ValidationError
	if err == nil || errors.As(err, &invalid) || errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want an internal error", err)
	}
	if !strings.Contains(err.Error(), "remove the file") {
		t.Errorf("error = %v, want one that says that the file was not removed", err)
	}
	if n := countRows(t, pool, "sources"); n != 1 {
		t.Errorf("sources has %d rows, want 1: the row stays when the file stays", n)
	}
	s.files.Dir = dir
	if got := storedFiles(t, s); len(got) != 1 {
		t.Errorf("the file store has %v, want the file", got)
	}
}

// The file store keeps a file by its content. After a purge took the file
// away, the same file can come in again.
func TestCaptureAfterAPurgeStoresTheFileAgain(t *testing.T) {
	s, _ := newFileService(t)
	first := captureUpload(t, s, "policy.pdf")
	deleteByHand(t, s, "source", first.ID)
	if err := s.Purge(context.Background(), "source", first.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	second := captureUpload(t, s, "policy.pdf")

	if second.ID == first.ID {
		t.Errorf("the capture gave the removed source %d", first.ID)
	}
	if got := storedFiles(t, s); len(got) != 1 {
		t.Errorf("the file store has %v, want the file of the new source", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run 'Purge|AfterAPurge'"
```
Expected: the run fails. The output contains `no code for the record type "source"` and `TestCaptureAfterAPurgeStoresTheFileAgain`.

- [ ] **Step 3: Add the queries**

In `queries/sources.sql`, replace this:

```sql
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;
```

With this:

```sql
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: PurgeSource :one
-- Removes a row for good. Only a row that is deleted already can go.
DELETE FROM sources
WHERE id = $1 AND deleted_at IS NOT NULL
RETURNING *;

-- name: CountSourcesOfFile :one
-- Counts the rows, live and deleted, that use one file of the file store.
SELECT count(*) FROM sources
WHERE file_path = $1;
```

- [ ] **Step 4: Write the code**

Postgres gives the error code `23503` when a foreign key still points to the row. `pgErr.TableName` is the table of the record that names the source.

In `internal/service/purge.go`, replace this:

```go
	"fmt"

	"github.com/jackc/pgx/v5"
```

With this:

```go
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
```

In `internal/service/purge.go`, replace this:

```go
// value. The events stay, so the log shows that the record was there.
func (s *Service) Purge(ctx context.Context, recordType string, id int64) error {
```

With this:

```go
// value. The events stay, so the log shows that the record was there.
//
// A source that a record names in its source_ref cannot go: Purge returns
// a ValidationError that names the table of that record. The file of a
// file source leaves the file store, unless a different source uses it.
func (s *Service) Purge(ctx context.Context, recordType string, id int64) error {
```

In `internal/service/purge.go`, replace this:

```go
	table, err := purgeRow(ctx, q, recordType, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
```

With this:

```go
	table, file, err := purgeRow(ctx, q, recordType, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	// 23503 is the code of Postgres for a foreign key that still points
	// to the row.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return invalid("id", "a record in "+pgErr.TableName+" names this source in its source_ref: delete that record forever first")
	}
```

In `internal/service/purge.go`, replace this:

```go
	}
	if err := tx.Commit(ctx); err != nil {
```

With this:

```go
	}
	// The file goes before the commit. When the file cannot go, the row
	// stays, and a second purge can do the work. The other order can
	// leave a file that no row names.
	if file != nil {
		others, err := q.CountSourcesOfFile(ctx, file)
		if err != nil {
			return fmt.Errorf("purge from %s: %w", table, err)
		}
		if others == 0 {
			if err := s.removeStoredFile(*file); err != nil {
				return fmt.Errorf("purge from %s: remove the file: %w", table, err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
```

In `internal/service/purge.go`, replace this:

```go
// purgeRow removes the row of one deleted record and returns its table.
func purgeRow(ctx context.Context, q *store.Queries, recordType string, id int64) (table string, err error) {
	switch recordType {
```

With this:

```go
// purgeRow removes the row of one deleted record. It returns the table of
// the record, and the path of its file when the record is a file source.
func purgeRow(ctx context.Context, q *store.Queries, recordType string, id int64) (table string, file *string, err error) {
	switch recordType {
```

In `internal/service/purge.go`, replace this:

```go
		_, err = q.PurgeCatalogItem(ctx, id)
		return "catalog_items", err
	case "journal":
		_, err = q.PurgeJournalEntry(ctx, id)
		return "journal_entries", err
	case "todo":
		_, err = q.PurgeTodo(ctx, id)
		return "todos", err
	case "habit_log":
		_, err = q.PurgeHabitLog(ctx, id)
		return "habit_logs", err
	}
	return "", errNoCode(recordType)
}
```

With this:

```go
		_, err = q.PurgeCatalogItem(ctx, id)
		return "catalog_items", nil, err
	case "journal":
		_, err = q.PurgeJournalEntry(ctx, id)
		return "journal_entries", nil, err
	case "todo":
		_, err = q.PurgeTodo(ctx, id)
		return "todos", nil, err
	case "habit_log":
		_, err = q.PurgeHabitLog(ctx, id)
		return "habit_logs", nil, err
	case "source":
		row, err := q.PurgeSource(ctx, id)
		return "sources", row.FilePath, err
	}
	return "", nil, errNoCode(recordType)
}

// removeStoredFile removes one file of the file store. path is the path
// below Files.Dir. A file that is not there is not an error. os.Root
// refuses a path that leaves the file store.
func (s *Service) removeStoredFile(path string) error {
	root, err := os.OpenRoot(s.files.Dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	// The file is gone from the disk only after a sync of its folder.
	return syncDir(filepath.Join(s.files.Dir, filepath.Dir(path)))
}
```

- [ ] **Step 5: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./internal/service ARGS="-run 'Purge|AfterAPurge' -v"
```
Expected: the run passes. The output contains `PASS: TestPurgeRemovesADeletedSourceAndItsFile`, `PASS: TestPurgeKeepsAFileThatAnotherSourceUses`, `PASS: TestPurgeRemovesATextSourceFromTheTableAndFromTheEvents`, `PASS: TestPurgeRefusesASourceThatARecordNames`, `PASS: TestPurgeOfASourceWhoseFileIsGoneAlreadyRemovesTheRow`, `PASS: TestPurgeKeepsTheRowWhenTheFileCannotBeRemoved` and `PASS: TestCaptureAfterAPurgeStoresTheFileAgain`.

- [ ] **Step 6: Commit**

Run:
```bash
git add internal/service/purge.go internal/service/purge_source_test.go queries/sources.sql
git commit -m "feat(service): remove a deleted source and its file for good"
```

---

### Task 3: List the deleted records

**Files:**
- Create: `internal/service/deleted.go`
- Modify: `queries/catalog_items.sql`, `queries/journal_entries.sql`, `queries/todos.sql`, `queries/habit_logs.sql`, `queries/sources.sql`
- Test: `internal/service/deleted_test.go`

**Interfaces:**
- Consumes (Tasks 1 and 2): `Purge`, `deleteByHand`.
- Consumes (plans 1 to 4, package `service`): `checkLimit(limit int) (int32, error)` (0 means 50, the maximum is 200), `RecordRef`, `itemFromRow`, `journalEntryFromRow`, `todoFromRow`, `habitLogFromRow(row store.HabitLog, habit string)`, `captureFromRow`, the method `summary()` of `Item`, `JournalEntry`, `Todo`, `HabitLog` and `Capture`, `(*Service).UpdateHabit(ctx, id int64, UpdateHabitParams{Active *bool, Actor Source})`.
- Produces (package `service`):
  - `type DeletedRecord struct { Type string; ID int64; Summary string; DeletedAt time.Time }`
  - `(*Service).ListDeleted(ctx context.Context, limit int) ([]DeletedRecord, error)`: the deleted records of all five types, the last deleted first. An empty result is an empty list, not nil.
  - Queries: `ListDeletedCatalogItems`, `ListDeletedJournalEntries`, `ListDeletedTodos`, `ListDeletedSources`, `ListDeletedHabitLogs` (each `:many`, parameter `int32`).
  - Test helper: `deletedRefs(records []DeletedRecord) []RecordRef`.

- [ ] **Step 1: Write the failing tests**

Create `internal/service/deleted_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"
)

// deletedRefs returns the type and the ID of each record, in order.
func deletedRefs(records []DeletedRecord) []RecordRef {
	refs := []RecordRef{}
	for _, r := range records {
		refs = append(refs, RecordRef{Type: r.Type, ID: r.ID})
	}
	return refs
}

func TestListDeletedGivesTheDeletedRecordsOfEachTypeWithTheLastDeletedFirst(t *testing.T) {
	s, _ := newTestService(t)
	addHabit(t, s, "gym")
	item := addItem(t, s, "book", "Dune")
	entry := addJournal(t, s, "a private day", "2026-10-09")
	todo := addTodo(t, s, "Buy a domain")
	log := logHabit(t, s, "gym", "2026-10-09")
	source := captureText(t, s, "a note")
	addItem(t, s, "book", "Emma")
	order := []RecordRef{
		{"todo", todo.ID}, {"item", item.ID}, {"source", source.ID}, {"habit_log", log.ID}, {"journal", entry.ID},
	}
	for _, ref := range order {
		deleteByHand(t, s, ref.Type, ref.ID)
	}

	got, err := s.ListDeleted(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListDeleted: %v", err)
	}

	want := []RecordRef{
		{"journal", entry.ID}, {"habit_log", log.ID}, {"source", source.ID}, {"item", item.ID}, {"todo", todo.ID},
	}
	refs := deletedRefs(got)
	if len(refs) != len(want) {
		t.Fatalf("ListDeleted gave %v, want %v", refs, want)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("ListDeleted gave %v, want %v", refs, want)
		}
	}
	summaries := map[string]string{
		"journal":   "journal 2026-10-09: a private day",
		"habit_log": "habit: gym on 2026-10-09",
		"source":    "text: a note",
		"item":      "book: Dune",
		"todo":      "todo: Buy a domain, project lifeos, due 2026-10-20",
	}
	for _, r := range got {
		if r.Summary != summaries[r.Type] {
			t.Errorf("the summary of the %s is %q, want %q", r.Type, r.Summary, summaries[r.Type])
		}
		if r.DeletedAt.IsZero() {
			t.Errorf("the %s has no time of the delete", r.Type)
		}
	}
}

func TestListDeletedLeavesOutARecordThatIsPurged(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	first := addItem(t, s, "book", "Dune")
	second := addItem(t, s, "book", "Emma")
	deleteByHand(t, s, "item", first.ID)
	deleteByHand(t, s, "item", second.ID)
	if err := s.Purge(ctx, "item", first.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	got, err := s.ListDeleted(ctx, 0)
	if err != nil {
		t.Fatalf("ListDeleted: %v", err)
	}

	if refs := deletedRefs(got); len(refs) != 1 || refs[0] != (RecordRef{"item", second.ID}) {
		t.Errorf("ListDeleted gave %v, want only item %d", refs, second.ID)
	}
}

func TestListDeletedObeysTheLimit(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	first := addItem(t, s, "book", "Dune")
	todo := addTodo(t, s, "Buy a domain")
	last := addItem(t, s, "book", "Emma")
	for _, ref := range []RecordRef{{"item", first.ID}, {"todo", todo.ID}, {"item", last.ID}} {
		deleteByHand(t, s, ref.Type, ref.ID)
	}

	got, err := s.ListDeleted(ctx, 2)
	if err != nil {
		t.Fatalf("ListDeleted: %v", err)
	}
	if refs := deletedRefs(got); len(refs) != 2 || refs[0] != (RecordRef{"item", last.ID}) || refs[1] != (RecordRef{"todo", todo.ID}) {
		t.Errorf("ListDeleted with limit 2 gave %v, want item %d, then todo %d", refs, last.ID, todo.ID)
	}

	_, err = s.ListDeleted(ctx, -1)
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "limit" {
		t.Errorf("limit -1: error = %v, want a ValidationError for limit", err)
	}

	none, err := newEmptyList(t)
	if err != nil || none == nil || len(none) != 0 {
		t.Errorf("an empty database gave %v, %v; want an empty list that is not nil", none, err)
	}
}

// newEmptyList returns the deleted records of a database that has none.
func newEmptyList(t *testing.T) ([]DeletedRecord, error) {
	t.Helper()
	s, _ := newTestService(t)
	return s.ListDeleted(context.Background(), 0)
}

func TestListDeletedShowsTheLogOfAHabitThatIsRetired(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	habit := addHabit(t, s, "gym")
	log := logHabit(t, s, "gym", "2026-10-09")
	deleteByHand(t, s, "habit_log", log.ID)
	if _, err := s.UpdateHabit(ctx, habit.ID, UpdateHabitParams{Active: ptr(false), Actor: SourceManual}); err != nil {
		t.Fatalf("UpdateHabit: %v", err)
	}

	got, err := s.ListDeleted(ctx, 0)
	if err != nil {
		t.Fatalf("ListDeleted: %v", err)
	}

	if len(got) != 1 || got[0].Summary != "habit: gym on 2026-10-09" {
		t.Errorf("ListDeleted gave %+v, want the log of gym", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run ListDeleted"
```
Expected: the run fails. The output contains `undefined: DeletedRecord`.

- [ ] **Step 3: Add the queries**

In `queries/catalog_items.sql`, replace this:

```sql
RETURNING id;
```

With this:

```sql
RETURNING id;

-- name: ListDeletedCatalogItems :many
SELECT * FROM catalog_items
WHERE deleted_at IS NOT NULL
ORDER BY deleted_at DESC, id DESC
LIMIT $1;
```

In `queries/journal_entries.sql`, replace this:

```sql
RETURNING id;
```

With this:

```sql
RETURNING id;

-- name: ListDeletedJournalEntries :many
SELECT * FROM journal_entries
WHERE deleted_at IS NOT NULL
ORDER BY deleted_at DESC, id DESC
LIMIT $1;
```

In `queries/todos.sql`, replace this:

```sql
RETURNING id;
```

With this:

```sql
RETURNING id;

-- name: ListDeletedTodos :many
SELECT * FROM todos
WHERE deleted_at IS NOT NULL
ORDER BY deleted_at DESC, id DESC
LIMIT $1;
```

In `queries/sources.sql`, replace this:

```sql
WHERE file_path = $1;
```

With this:

```sql
WHERE file_path = $1;

-- name: ListDeletedSources :many
SELECT * FROM sources
WHERE deleted_at IS NOT NULL
ORDER BY deleted_at DESC, id DESC
LIMIT $1;
```

The query for the habit logs gets the name of the habit, as `ListUnverifiedHabitLogs` does. It has no condition on the habit, so the log of a retired habit is in the list.

In `queries/habit_logs.sql`, replace this:

```sql
RETURNING id;
```

With this:

```sql
RETURNING id;

-- name: ListDeletedHabitLogs :many
SELECT sqlc.embed(habit_logs), habits.name AS habit
FROM habit_logs
JOIN habits ON habits.id = habit_logs.habit_id
WHERE habit_logs.deleted_at IS NOT NULL
ORDER BY habit_logs.deleted_at DESC, habit_logs.id DESC
LIMIT $1;
```

- [ ] **Step 4: Write the code**

Create `internal/service/deleted.go`:

```go
package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// DeletedRecord is one record that is deleted and not yet removed for
// good.
type DeletedRecord struct {
	Type string
	ID   int64
	// Summary is one line that says what the record holds.
	Summary   string
	DeletedAt time.Time
}

// ListDeleted returns the deleted records of every type, the last deleted
// first. limit 0 means 50; the maximum is 200. Purge removes one of them
// for good.
func (s *Service) ListDeleted(ctx context.Context, limit int) ([]DeletedRecord, error) {
	maxRows, err := checkLimit(limit)
	if err != nil {
		return nil, err
	}

	q := store.New(s.pool)
	records := []DeletedRecord{}
	for _, deleted := range []func(context.Context, *store.Queries, int32) ([]DeletedRecord, error){
		deletedItems,
		deletedJournalEntries,
		deletedTodos,
		deletedHabitLogs,
		deletedSources,
	} {
		more, err := deleted(ctx, q, maxRows)
		if err != nil {
			return nil, fmt.Errorf("list deleted: %w", err)
		}
		records = append(records, more...)
	}
	// Each type gave its last deleted records. Put them in one order.
	sort.SliceStable(records, func(i, j int) bool { return records[i].DeletedAt.After(records[j].DeletedAt) })
	if len(records) > int(maxRows) {
		records = records[:maxRows]
	}
	return records, nil
}

func deletedRecord(recordType string, id int64, summary string, deletedAt *time.Time) DeletedRecord {
	return DeletedRecord{Type: recordType, ID: id, Summary: summary, DeletedAt: *deletedAt}
}

func deletedItems(ctx context.Context, q *store.Queries, limit int32) ([]DeletedRecord, error) {
	rows, err := q.ListDeletedCatalogItems(ctx, limit)
	if err != nil {
		return nil, err
	}
	records := make([]DeletedRecord, 0, len(rows))
	for _, row := range rows {
		item, err := itemFromRow(row)
		if err != nil {
			return nil, err
		}
		records = append(records, deletedRecord("item", row.ID, item.summary(), row.DeletedAt))
	}
	return records, nil
}

func deletedJournalEntries(ctx context.Context, q *store.Queries, limit int32) ([]DeletedRecord, error) {
	rows, err := q.ListDeletedJournalEntries(ctx, limit)
	if err != nil {
		return nil, err
	}
	records := make([]DeletedRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, deletedRecord("journal", row.ID, journalEntryFromRow(row).summary(), row.DeletedAt))
	}
	return records, nil
}

func deletedTodos(ctx context.Context, q *store.Queries, limit int32) ([]DeletedRecord, error) {
	rows, err := q.ListDeletedTodos(ctx, limit)
	if err != nil {
		return nil, err
	}
	records := make([]DeletedRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, deletedRecord("todo", row.ID, todoFromRow(row).summary(), row.DeletedAt))
	}
	return records, nil
}

func deletedHabitLogs(ctx context.Context, q *store.Queries, limit int32) ([]DeletedRecord, error) {
	rows, err := q.ListDeletedHabitLogs(ctx, limit)
	if err != nil {
		return nil, err
	}
	records := make([]DeletedRecord, 0, len(rows))
	for _, row := range rows {
		log := habitLogFromRow(row.HabitLog, row.Habit)
		records = append(records, deletedRecord("habit_log", log.ID, log.summary(), row.HabitLog.DeletedAt))
	}
	return records, nil
}

func deletedSources(ctx context.Context, q *store.Queries, limit int32) ([]DeletedRecord, error) {
	rows, err := q.ListDeletedSources(ctx, limit)
	if err != nil {
		return nil, err
	}
	records := make([]DeletedRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, deletedRecord("source", row.ID, captureFromRow(row).summary(), row.DeletedAt))
	}
	return records, nil
}
```

- [ ] **Step 5: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./internal/service ARGS="-run 'ListDeleted|Purge' -v"
```
Expected: the run passes. The output contains `PASS: TestListDeletedGivesTheDeletedRecordsOfEachTypeWithTheLastDeletedFirst`, `PASS: TestListDeletedLeavesOutARecordThatIsPurged`, `PASS: TestListDeletedObeysTheLimit` and `PASS: TestListDeletedShowsTheLogOfAHabitThatIsRetired`.

- [ ] **Step 6: Commit**

Run:
```bash
git add internal/service/deleted.go internal/service/deleted_test.go queries/catalog_items.sql queries/journal_entries.sql queries/todos.sql queries/habit_logs.sql queries/sources.sql
git commit -m "feat(service): list the deleted records of every type"
```

---

### Task 4: The admin page for deleted records

**Files:**
- Create: `internal/admin/deleted.go`, `internal/admin/deleted.templ`
- Modify: `internal/admin/admin.go` (two routes), `internal/admin/items.templ` (the menu)
- Test: `internal/admin/deleted_test.go`

**Interfaces:**
- Consumes (Tasks 1 to 3, package `service`): `(*Service).Purge(ctx, recordType string, id int64) error`, `(*Service).ListDeleted(ctx, limit int) ([]DeletedRecord, error)`, `DeletedRecord`.
- Consumes (plans 1 to 4, package `admin`): `handlers`, `render`, `listFailure`, `internalError`, the templ component `page(title string)`. The test helpers `newTestAdmin`, `get`, `postForm`, `addStored`.
- Produces:
  - `GET /admin/deleted`: the page with the list.
  - `POST /admin/deleted/purge` with the form fields `type` and `id`: removes the record and answers with the fragment `<div id="list">`. An `id` that is not a number gives status 400. A refusal of the service shows as a message above the list, with status 200.
  - The menu of each admin page has the link "Deleted".
  - Test helpers (package `admin_test`): `addDeleted(t, pool, title string) service.Item`, `countItems(t, pool) int`.

- [ ] **Step 1: Write the failing tests**

Create `internal/admin/deleted_test.go`:

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

// addDeleted stores one item and deletes it, as the items page does.
func addDeleted(t *testing.T, pool *pgxpool.Pool, title string) service.Item {
	t.Helper()
	item := addStored(t, pool, title, true)
	if err := service.New(pool).DeleteItem(t.Context(), item.ID, service.DeleteParams{Actor: service.SourceManual}); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}
	return item
}

func countItems(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM catalog_items").Scan(&n); err != nil {
		t.Fatalf("count the items: %v", err)
	}
	return n
}

func TestDeletedPageListsTheDeletedRecords(t *testing.T) {
	h, pool := newTestAdmin(t)
	gone := addDeleted(t, pool, `<i>Gone</i>`)
	addStored(t, pool, "Stays", true)

	rec := get(h, "/admin/deleted")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"movie: &lt;i&gt;Gone&lt;/i&gt;, rating 4.5, on 2024-03-01",
		`hx-post="/admin/deleted/purge"`,
		`name="type" value="item"`,
		fmt.Sprintf(`name="id" value="%d"`, gone.ID),
		fmt.Sprintf(`hx-confirm="Delete item %d forever? You cannot get it back."`, gone.ID),
		`<a href="/admin/deleted">Deleted</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %s", want)
		}
	}
	if strings.Contains(body, "<i>Gone") {
		t.Error("the page contains the title as raw HTML")
	}
	if strings.Contains(body, "Stays") {
		t.Error("the page lists a record that is not deleted")
	}
}

func TestDeletedPageSaysWhenNothingIsDeleted(t *testing.T) {
	h, _ := newTestAdmin(t)

	body := get(h, "/admin/deleted").Body.String()

	if !strings.Contains(body, "No record is deleted.") {
		t.Errorf("the page does not say that the list is empty:\n%s", body)
	}
}

func TestPurgeRecordRemovesTheRecordAndShowsTheList(t *testing.T) {
	h, pool := newTestAdmin(t)
	first := addDeleted(t, pool, "First")
	addDeleted(t, pool, "Second")

	rec := postForm(h, "/admin/deleted/purge", url.Values{"type": {"item"}, "id": {fmt.Sprint(first.ID)}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(body), `<div id="list">`) {
		t.Errorf("the response is not the #list fragment:\n%s", body)
	}
	if strings.Contains(body, "First") || !strings.Contains(body, "Second") {
		t.Errorf("the list must show Second and not First:\n%s", body)
	}
	if n := countItems(t, pool); n != 1 {
		t.Errorf("catalog_items has %d rows, want 1", n)
	}
}

func TestPurgeRecordShowsAMessageWhenTheRecordIsNotDeleted(t *testing.T) {
	h, pool := newTestAdmin(t)
	live := addStored(t, pool, "Stays", true)

	rec := postForm(h, "/admin/deleted/purge", url.Values{"type": {"item"}, "id": {fmt.Sprint(live.ID)}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body)
	}
	want := fmt.Sprintf("item %d is not in the list. It was possibly removed before.", live.ID)
	if body := rec.Body.String(); !strings.Contains(body, want) || !strings.Contains(body, `role="alert"`) {
		t.Errorf("the response does not show the message %q:\n%s", want, body)
	}
	if n := countItems(t, pool); n != 1 {
		t.Errorf("catalog_items has %d rows, want 1", n)
	}
}

func TestPurgeRecordShowsWhyTheServiceRefused(t *testing.T) {
	h, pool := newTestAdmin(t)
	svc := service.New(pool)
	source, err := svc.Capture(t.Context(), service.CaptureParams{Text: "Renew the policy.", Source: service.SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	_, err = svc.AddTodo(t.Context(), service.AddTodoParams{Title: "Renew the policy", Provenance: service.Provenance{Source: service.SourceAgent, SourceRef: &source.ID}})
	if err != nil {
		t.Fatalf("AddTodo: %v", err)
	}
	if err := svc.DeleteCapture(t.Context(), source.ID, service.DeleteParams{Actor: service.SourceManual}); err != nil {
		t.Fatalf("DeleteCapture: %v", err)
	}

	rec := postForm(h, "/admin/deleted/purge", url.Values{"type": {"source"}, "id": {fmt.Sprint(source.ID)}})

	body := rec.Body.String()
	if !strings.Contains(body, "a record in todos names this source") {
		t.Errorf("the response does not say why the source stays:\n%s", body)
	}
	if !strings.Contains(body, "text: Renew the policy.") {
		t.Errorf("the list does not show the source any more:\n%s", body)
	}

	rec = postForm(h, "/admin/deleted/purge", url.Values{"type": {"habit"}, "id": {"1"}})
	if body := rec.Body.String(); !strings.Contains(body, "type: must be one of") {
		t.Errorf("a wrong type does not show the reason:\n%s", body)
	}
}

func TestPurgeRecordRefusesAnIDThatIsNotANumber(t *testing.T) {
	h, _ := newTestAdmin(t)

	rec := postForm(h, "/admin/deleted/purge", url.Values{"type": {"item"}, "id": {"one"}})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/admin ARGS="-run 'Deleted|PurgeRecord'"
```
Expected: the run fails. The output contains `status = 404, want 200` and `status = 404, want 400`.

- [ ] **Step 3: Write the handlers**

Create `internal/admin/deleted.go`:

```go
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
```

- [ ] **Step 4: Write the page**

The form has `hx-confirm`, so the browser asks before it sends the request. This is the confirmation of its own that the design doc asks for (S25).

Create `internal/admin/deleted.templ`:

```templ
package admin

import (
	"strconv"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

templ deletedPage(records []service.DeletedRecord) {
	@page("Deleted") {
		<h1>Deleted records</h1>
		<p class="muted">A deleted record stays in the database until you remove it here. "Delete forever" removes the record, the content of its history and the file of a file source. You cannot get it back.</p>
		@deletedList(records, "")
	}
}

// deletedList is the part of the page that a removal replaces.
templ deletedList(records []service.DeletedRecord, errorMessage string) {
	<div id="list">
		if errorMessage != "" {
			<p class="error" role="alert">{ errorMessage }</p>
		}
		if len(records) == 0 {
			<p class="muted">No record is deleted.</p>
		} else {
			<table>
				<thead>
					<tr>
						<th>Type</th>
						<th>ID</th>
						<th>Record</th>
						<th>Deleted (IST)</th>
						<th></th>
					</tr>
				</thead>
				<tbody>
					for _, record := range records {
						<tr>
							<td>{ record.Type }</td>
							<td>{ strconv.FormatInt(record.ID, 10) }</td>
							<td>{ record.Summary }</td>
							<td>{ record.DeletedAt.In(service.IST).Format("2 Jan 2006 15:04") }</td>
							<td>
								<form
									hx-post="/admin/deleted/purge"
									hx-target="#list"
									hx-swap="outerHTML"
									hx-confirm={ "Delete " + record.Type + " " + strconv.FormatInt(record.ID, 10) + " forever? You cannot get it back." }
								>
									<input type="hidden" name="type" value={ record.Type }/>
									<input type="hidden" name="id" value={ strconv.FormatInt(record.ID, 10) }/>
									<button type="submit">Delete forever</button>
								</form>
							</td>
						</tr>
					}
				</tbody>
			</table>
		}
	</div>
}
```

- [ ] **Step 5: Add the routes and the menu link**

In `internal/admin/admin.go`, replace this:

```go
	router.Post("/admin/review/verify", h.verifyRecord)
	router.Handle("/admin/static/*", http.StripPrefix("/admin/", http.FileServerFS(static)))
```

With this:

```go
	router.Post("/admin/review/verify", h.verifyRecord)
	router.Get("/admin/deleted", h.deletedPage)
	router.Post("/admin/deleted/purge", h.purgeRecord)
	router.Handle("/admin/static/*", http.StripPrefix("/admin/", http.FileServerFS(static)))
```

In `internal/admin/items.templ`, replace this:

```templ
				<a href="/admin/review">Review</a>
			</nav>
```

With this:

```templ
				<a href="/admin/review">Review</a>
				<a href="/admin/deleted">Deleted</a>
			</nav>
```

- [ ] **Step 6: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./internal/admin ARGS="-run 'Deleted|PurgeRecord' -v"
```
Expected: the run passes. The output contains `PASS: TestDeletedPageListsTheDeletedRecords`, `PASS: TestDeletedPageSaysWhenNothingIsDeleted`, `PASS: TestPurgeRecordRemovesTheRecordAndShowsTheList`, `PASS: TestPurgeRecordShowsAMessageWhenTheRecordIsNotDeleted`, `PASS: TestPurgeRecordShowsWhyTheServiceRefused` and `PASS: TestPurgeRecordRefusesAnIDThatIsNotANumber`.

- [ ] **Step 7: Commit**

Run:
```bash
git add internal/admin/deleted.go internal/admin/deleted.templ internal/admin/deleted_test.go internal/admin/admin.go internal/admin/items.templ
git commit -m "feat(admin): a page that removes a deleted record for good"
```

---

### Task 5: Refuse a request from another site, and open the admin pages from the bare address

**Files:**
- Modify: `cmd/lifeos/serve.go` (`serveOn`), `internal/admin/admin.go` (`Register`, `home`)
- Test: `cmd/lifeos/protect_test.go`, `internal/admin/home_test.go`

**Interfaces:**
- Consumes (plans 1 to 4): `serveOn(ctx, httpListener, mcpListener net.Listener, svc *service.Service) error` in `cmd/lifeos/serve.go`; the test helper `startServer(t) (config.Config, *pgxpool.Pool)`; `admin.Register(router chi.Router, svc *service.Service)`; the test helpers `newTestAdmin` and `get`.
- Produces:
  - The HTTP server of `http_addr` answers 403 to a POST, PATCH, PUT or DELETE that a browser sent from a different origin. It decides by the header `Sec-Fetch-Site`, and by `Origin` when that header is absent. A request that has neither header goes through. A GET always goes through.
  - `GET /`, `GET /admin` and `GET /admin/` answer 303 with `Location: /admin/items`.
  - Test helper (package `main`): `post(t, url, contentType, body string, headers map[string]string) int`.

- [ ] **Step 1: Write the failing tests**

Create `cmd/lifeos/protect_test.go`:

```go
package main

import (
	"net/http"
	"strings"
	"testing"
)

// post makes one POST with the headers that a browser adds, and returns
// the status.
func post(t *testing.T, url, contentType, body string, headers map[string]string) int {
	t.Helper()
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", contentType)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// A page of a different site can make the browser of Gautam send a POST,
// and the browser adds the login of Cloudflare Access to it. The server
// must refuse that request. A browser says where a request comes from in
// Sec-Fetch-Site, and an old browser in Origin.
func TestServeRefusesAPostFromAnotherSite(t *testing.T) {
	cfg, pool := startServer(t)
	rest := "http://" + cfg.HTTPAddr
	const form = "application/x-www-form-urlencoded"
	const item = `{"kind":"book","title":"Dune"}`

	refused := []struct {
		name, url, contentType, body string
		headers                      map[string]string
	}{
		{"an admin form from another site", rest + "/admin/items", form, "kind=book&title=Dune", map[string]string{"Sec-Fetch-Site": "cross-site"}},
		{"an admin form from an old browser", rest + "/admin/items", form, "kind=book&title=Dune", map[string]string{"Origin": "https://evil.example"}},
		{"an admin form from a frame that has no origin", rest + "/admin/items", form, "kind=book&title=Dune", map[string]string{"Origin": "null"}},
		{"an API call from another site", rest + "/api/items", "application/json", item, map[string]string{"Sec-Fetch-Site": "cross-site"}},
		{"an API call from a page of the same domain", rest + "/api/items", "application/json", item, map[string]string{"Sec-Fetch-Site": "same-site"}},
	}
	for _, r := range refused {
		if status := post(t, r.url, r.contentType, r.body, r.headers); status != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", r.name, status)
		}
	}
	var stored int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM catalog_items").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 0 {
		t.Fatalf("a refused request stored %d items", stored)
	}

	accepted := []struct {
		name, url, contentType, body string
		headers                      map[string]string
	}{
		{"an admin form from its own page", rest + "/admin/items", form, "kind=book&title=Dune", map[string]string{"Sec-Fetch-Site": "same-origin"}},
		{"an admin form from an old browser on its own page", rest + "/admin/items", form, "kind=book&title=Emma", map[string]string{"Origin": rest}},
		{"an API call from a program, which sends no such header", rest + "/api/items", "application/json", item, nil},
	}
	for _, a := range accepted {
		if status := post(t, a.url, a.contentType, a.body, a.headers); status >= 300 {
			t.Errorf("%s: status = %d, want success", a.name, status)
		}
	}
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM catalog_items").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 3 {
		t.Errorf("the accepted requests stored %d items, want 3", stored)
	}

	// A GET changes nothing, so a link from another site can open a page.
	req, err := http.NewRequest("GET", rest+"/admin/items", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("a GET from another site: status = %d, want 200", resp.StatusCode)
	}
}
```

Create `internal/admin/home_test.go`:

```go
package admin_test

import (
	"net/http"
	"testing"
)

// The address that Gautam types has no path. It must open the admin pages.
func TestTheBareAddressOpensTheItemsPage(t *testing.T) {
	h, _ := newTestAdmin(t)

	for _, path := range []string{"/", "/admin", "/admin/"} {
		rec := get(h, path)
		if rec.Code != http.StatusSeeOther {
			t.Errorf("GET %s: status = %d, want 303", path, rec.Code)
		}
		if got := rec.Header().Get("Location"); got != "/admin/items" {
			t.Errorf("GET %s: Location = %q, want /admin/items", path, got)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG="./cmd/lifeos ./internal/admin" ARGS="-run 'AnotherSite|BareAddress'"
```
Expected: the run fails. The output contains `an admin form from another site: status = 200, want 403` and `GET /: status = 404, want 303`.

- [ ] **Step 3: Put the protection in front of the router**

`http.CrossOriginProtection` is in the Go standard library from Go 1.25. It needs no token in a form and no state.

In `cmd/lifeos/serve.go`, replace this:

```go
	servers := []*http.Server{
		{Handler: router, ReadHeaderTimeout: 10 * time.Second},
		{Handler: mcpMux, ReadHeaderTimeout: 10 * time.Second},
```

With this:

```go
	// Cloudflare Access is the login, and the browser sends that login
	// with each request, also with one that a page of a different site
	// makes. This refuses a request that changes data and that a browser
	// sent from a different origin. A program, such as a Shortcut, sends
	// no header about its origin and goes through.
	sameOrigin := http.NewCrossOriginProtection()

	servers := []*http.Server{
		{Handler: sameOrigin.Handler(router), ReadHeaderTimeout: 10 * time.Second},
		{Handler: mcpMux, ReadHeaderTimeout: 10 * time.Second},
```

- [ ] **Step 4: Add the redirect**

In `internal/admin/admin.go`, replace this:

```go
	h := &handlers{svc: svc}
	router.Get("/admin/items", h.itemsPage)
```

With this:

```go
	h := &handlers{svc: svc}
	// The address that Gautam types has no path.
	for _, path := range []string{"/", "/admin", "/admin/"} {
		router.Get(path, home)
	}
	router.Get("/admin/items", h.itemsPage)
```

In `internal/admin/admin.go`, replace this:

```go
	svc *service.Service
}
```

With this:

```go
	svc *service.Service
}

// home sends the browser to the first admin page.
func home(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/items", http.StatusSeeOther)
}
```

- [ ] **Step 5: Run the tests to verify that they pass**

Run:
```bash
make test PKG="./cmd/lifeos ./internal/admin" ARGS="-run 'AnotherSite|BareAddress' -v"
```
Expected: the run passes. The output contains `PASS: TestServeRefusesAPostFromAnotherSite` and `PASS: TestTheBareAddressOpensTheItemsPage`.

- [ ] **Step 6: Commit**

Run:
```bash
git add cmd/lifeos/serve.go cmd/lifeos/protect_test.go internal/admin/admin.go internal/admin/home_test.go
git commit -m "feat(serve): refuse a request from another site that changes data"
```

---

### Task 6: The service unit, the deploy script and the Access check

**Files:**
- Create: `deploy/systemd/lifeos.service`, `deploy/deploy.sh`, `deploy/check-access.sh`
- Modify: `deploy/.env.example`

**Interfaces:**
- Consumes: `bin/lifeos serve` and `bin/lifeos migrate` (plans 1 to 4); `deploy/systemd/lifeos-notify@.service` (Phase 1), which sends the result of a unit to ntfy; `NTFY_BASE_URL` in `deploy/.env`.
- Produces:
  - `deploy/systemd/lifeos.service`: runs `/home/gautam/lifeos-core/bin/lifeos serve` as `gautam`, with the environment of `deploy/.env`. Task 7 installs it.
  - `deploy/deploy.sh`: no argument. Task 7 runs it.
  - `deploy/check-access.sh [env-file]`: exit 0 when each hostname of `LIFEOS_ADMIN_HOST` and `LIFEOS_API_HOST` stops a request that has no login, or when the two are empty. Exit 1 for a line that starts with `OPEN` or `FAIL`. The default env file is `deploy/.env`.
  - `deploy/.env.example`: the keys `LIFEOS_ADMIN_HOST`, `LIFEOS_API_HOST`, `CF_ACCESS_CLIENT_ID`, `CF_ACCESS_CLIENT_SECRET`.

These files have no Go test. Steps 5 to 7 run the check script against hostnames that exist.

- [ ] **Step 1: Write the unit file**

`ProtectSystem=strict` makes the file system read-only for the service; `ReadWritePaths` opens the file store and the inbox again. `Restart=on-failure` starts the server again after a crash, and at boot until Postgres answers.

Create `deploy/systemd/lifeos.service`:

```
[Unit]
Description=LifeOS server (REST API, admin pages, MCP server)
Requires=docker.service
After=docker.service network-online.target
Wants=network-online.target
# Postgres starts slowly after a boot: ten starts in five minutes, then
# the unit fails and ntfy gets a message.
StartLimitIntervalSec=300
StartLimitBurst=10
OnFailure=lifeos-notify@%n.service

[Service]
User=gautam
WorkingDirectory=/home/gautam/lifeos-core
# The two database passwords. systemd reads KEY=value lines; it does not
# run the file as a shell does.
EnvironmentFile=/home/gautam/lifeos-core/deploy/.env
ExecStart=/home/gautam/lifeos-core/bin/lifeos serve
Restart=on-failure
RestartSec=10
# The server writes only to the file store and the inbox.
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=/var/lib/lifeos

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 2: Write the Access check**

Create `deploy/check-access.sh`:

```bash
#!/bin/bash
# Checks that Cloudflare Access guards each public hostname of LifeOS.
# LifeOS has no login of its own: a hostname that answers a request that
# has no login is open to the internet. Run this after each change of the
# tunnel or of an Access application; deploy.sh runs it after each deploy.
#
# Needs LIFEOS_ADMIN_HOST and LIFEOS_API_HOST in deploy/.env. With
# CF_ACCESS_CLIENT_ID and CF_ACCESS_CLIENT_SECRET (an Access service token),
# it also checks that the API answers a request that has the token, and
# that the API hostname gives no admin page.
# Usage: check-access.sh [env-file]   (default: deploy/.env)
set -euo pipefail

env_file=$(realpath "${1:-$(dirname "$0")/.env}")
set -a; . "$env_file"; set +a

if [ -z "${LIFEOS_ADMIN_HOST:-}" ] && [ -z "${LIFEOS_API_HOST:-}" ]; then
    echo "no public hostname is set in $env_file: nothing to check"
    exit 0
fi

failed=0

# guarded <url>: a request that has no login must stop at Cloudflare Access.
# Access answers 302 to its login page, or 401 or 403. LifeOS gives none of
# these to a GET.
guarded() {
    local url=$1 code target
    read -r code target < <(curl -s -o /dev/null --max-time 20 -w '%{http_code} %{redirect_url}\n' "$url" || true)
    case "$code" in
        302)
            if [[ $target == https://*.cloudflareaccess.com/* ]]; then
                echo "ok    $url: Access asks for a login"
                return
            fi ;;
        401|403)
            echo "ok    $url: Access refuses the request ($code)"
            return ;;
        000)
            echo "FAIL  $url: no answer"
            failed=1
            return ;;
    esac
    echo "OPEN  $url: status $code for a request that has no login"
    failed=1
}

# with_token <url> <status>: a request that has the service token must get
# this status from LifeOS.
with_token() {
    local url=$1 want=$2 code
    code=$(curl -s -o /dev/null --max-time 20 -w '%{http_code}' \
        -H "CF-Access-Client-Id: $CF_ACCESS_CLIENT_ID" \
        -H "CF-Access-Client-Secret: $CF_ACCESS_CLIENT_SECRET" "$url" || true)
    if [ "$code" = "$want" ]; then
        echo "ok    $url: status $code with the service token"
    else
        echo "FAIL  $url: status $code with the service token, want $want"
        failed=1
    fi
}

if [ -n "${LIFEOS_ADMIN_HOST:-}" ]; then
    guarded "https://$LIFEOS_ADMIN_HOST/admin/items"
    # The admin hostname gives no API route.
    guarded "https://$LIFEOS_ADMIN_HOST/api/items"
fi
if [ -n "${LIFEOS_API_HOST:-}" ]; then
    guarded "https://$LIFEOS_API_HOST/api/items"
    if [ -n "${CF_ACCESS_CLIENT_ID:-}" ] && [ -n "${CF_ACCESS_CLIENT_SECRET:-}" ]; then
        with_token "https://$LIFEOS_API_HOST/api/items" 200
        # The API hostname gives no admin page.
        with_token "https://$LIFEOS_API_HOST/admin/items" 404
    fi
fi

exit "$failed"
```

- [ ] **Step 3: Write the deploy script**

The script stops at the first command that fails (`set -e`). So a failed build or a failed dump comes before `lifeos migrate` and stops it.

Create `deploy/deploy.sh`:

```bash
#!/bin/bash
# Ships the code of this repository on the Pi: pull, build, dump the
# database, apply the migrations, start the service again, then check that
# Cloudflare Access guards the public hostnames.
# Run it on the Pi as the user of the service: deploy/deploy.sh
set -euo pipefail

cd "$(dirname "$0")/.."
set -a; . deploy/.env; set +a

git pull --ff-only
# The service runs the old binary until the restart.
make build

# A migration can drop or rename a column, so each deploy dumps the
# database before it applies one. The last five dumps stay.
dump_dir=/var/backups/lifeos/deploy
mkdir -p "$dump_dir"
dump="$dump_dir/before-$(date +%Y%m%d-%H%M%S).dump"
docker compose -f deploy/docker-compose.yml exec -T postgres pg_dump -U lifeos -Fc lifeos > "$dump.part"
mv "$dump.part" "$dump"
ls -1t "$dump_dir"/before-*.dump | tail -n +6 | xargs -r rm --

bin/lifeos migrate
sudo systemctl restart lifeos.service

# The server listens after it has a connection to Postgres.
addr=$(sed -n 's/^http_addr: *//p' config/lifeos.yaml)
for _ in $(seq 30); do
    if curl -fsS -o /dev/null "http://$addr/admin/items"; then
        echo "lifeos answers on $addr"
        deploy/check-access.sh
        exit 0
    fi
    sleep 1
done
echo "lifeos does not answer on $addr: see journalctl -u lifeos" >&2
exit 1
```

- [ ] **Step 4: Add the keys to the example env file**

In `deploy/.env.example`, replace this:

```
# ntfy (docker-compose.yml): public URL of the ntfy server.
```

With this:

```
# Public hostnames of LifeOS (check-access.sh): the admin pages and the REST
# API, each behind its own Cloudflare Access application. Leave them empty
# when LifeOS has no public hostname.
LIFEOS_ADMIN_HOST=
LIFEOS_API_HOST=
# Cloudflare Access service token of the Pi (check-access.sh): lets the check
# call the API through the tunnel. Leave them empty to skip that part.
CF_ACCESS_CLIENT_ID=
CF_ACCESS_CLIENT_SECRET=

# ntfy (docker-compose.yml): public URL of the ntfy server.
```

- [ ] **Step 5: Check the three files**

Run:
```bash
chmod 755 deploy/deploy.sh deploy/check-access.sh
mkdir -p bin
bash -n deploy/deploy.sh
bash -n deploy/check-access.sh
systemd-analyze verify deploy/systemd/lifeos.service
printf 'LIFEOS_ADMIN_HOST=\nLIFEOS_API_HOST=\n' > bin/none.env
deploy/check-access.sh bin/none.env
rm bin/none.env
```
Expected: the run passes. The output contains `nothing to check`.

- [ ] **Step 6: Run the check against a hostname that is open**

The ntfy server of the Pi has no Access application: the ntfy app of the phone cannot do a browser login. So its hostname answers a request that has no login, and the check must fail for it.

Run:
```bash
ntfy_host=$(sed -n 's#^NTFY_BASE_URL=https://##p' deploy/.env)
printf 'LIFEOS_ADMIN_HOST=\nLIFEOS_API_HOST=%s\n' "$ntfy_host" > bin/open.env
deploy/check-access.sh bin/open.env
```
Expected: the run fails. The output contains `OPEN`.

- [ ] **Step 7: Run the check against a hostname that Access guards**

The hostname `sysinfo.` of the same domain has an Access application (Phase 1).

Run:
```bash
ntfy_host=$(sed -n 's#^NTFY_BASE_URL=https://##p' deploy/.env)
printf 'LIFEOS_ADMIN_HOST=sysinfo.%s\nLIFEOS_API_HOST=\n' "${ntfy_host#ntfy.}" > bin/guarded.env
deploy/check-access.sh bin/guarded.env
rm bin/open.env bin/guarded.env
```
Expected: the run passes. The output contains `Access asks for a login`.

- [ ] **Step 8: Commit**

Run:
```bash
git add deploy/systemd/lifeos.service deploy/deploy.sh deploy/check-access.sh deploy/.env.example
git commit -m "feat(deploy): run lifeos serve under systemd, with a deploy script and an Access check"
```

---

### Task 7: Run the server as a systemd service

This task changes the real system: it stops the server of the tmux window and installs the unit. It changes no file in git.

**Files:**
- Uses: `deploy/systemd/lifeos.service`, `deploy/deploy.sh`, `deploy/check-access.sh` (Task 6)

**Interfaces:**
- Consumes (Tasks 1 to 6): the unit file, the deploy script, the page `/admin/deleted`, the route `POST /admin/deleted/purge`, the protection of Task 5. From plan 4: `POST /api/ingest/file?name=` (answers 200 with the source as JSON) and `DELETE /api/sources/{id}` (answers 204).
- Produces: the systemd unit `lifeos.service`, enabled and active, on `127.0.0.1:8200` and `127.0.0.1:8100`. The folder `/var/backups/lifeos/deploy/` with one dump.

- [ ] **Step 1: Get Gautam's OK**

Tell Gautam: this task uses `sudo` to install and start `lifeos.service`, and it stops the `make run` process of the tmux window. The admin pages do not answer for about one minute. Continue only after a yes.

- [ ] **Step 2: Run all tests and build**

Run:
```bash
cd /home/gautam/lifeos-core
git status --short
make test
make build
```
Expected: the run passes. The output contains `go build -o bin/lifeos ./cmd/lifeos`.

`git status --short` prints nothing, except that this plan file can be new. `make test` prints `ok` for seven packages.

- [ ] **Step 3: Stop the server of the tmux window**

Run:
```bash
tmux send-keys -t lifeos-core:lifeos-serve C-c 2>/dev/null || true
sleep 3
echo "listeners: $(ss -ltn | grep -c -E '127\.0\.0\.1:(8200|8100) ')"
```
Expected: the run passes. The output contains `listeners: 0`.

If the number is not 0, a different process has a port. Find it with `ss -ltnp | grep -E ':(8200|8100) '` and stop it.

- [ ] **Step 4: Install and start the unit**

Run:
```bash
sudo systemctl link /home/gautam/lifeos-core/deploy/systemd/lifeos.service
sudo systemctl enable --now lifeos.service
sleep 5
systemctl is-active lifeos.service
systemctl is-enabled lifeos.service
journalctl -u lifeos.service -n 3 -o cat --no-pager
```
Expected: the run passes. The output contains `active`, `enabled` and `lifeos is listening`.

- [ ] **Step 5: Prove that the service works with its protection settings**

A small text file goes in through the API, then goes out for good through the admin route. This proves that the unit can write to `/var/lib/lifeos`, and it proves Tasks 1 to 5 on the real database. The database keeps three events of this source, with no values: `create`, `delete` and `purge`.

Run:
```bash
base=http://127.0.0.1:8200
curl -s -o /dev/null -w 'admin page: %{http_code}\n' $base/admin/deleted
curl -s -o /dev/null -w 'post from another site: %{http_code}\n' -X POST -H 'Sec-Fetch-Site: cross-site' -d 'kind=book&title=x' $base/admin/items
before=$(find /var/lib/lifeos/files -type f | wc -l)
printf 'LifeOS service check %s\n' "$(date +%s)" > bin/service-check.txt
id=$(curl -s -X POST --data-binary @bin/service-check.txt -H 'Content-Type: application/octet-stream' "$base/api/ingest/file?name=service-check.txt" | python3 -I -c 'import json, sys; print(json.load(sys.stdin)["id"])')
during=$(find /var/lib/lifeos/files -type f | wc -l)
curl -s -o /dev/null -w 'delete: %{http_code}\n' -X DELETE "$base/api/sources/$id"
curl -s -o /dev/null -w 'delete forever: %{http_code}\n' -X POST -d "type=source&id=$id" $base/admin/deleted/purge
after=$(find /var/lib/lifeos/files -type f | wc -l)
echo "file store: $((during - before)) more after the capture, $((after - before)) more after the permanent delete"
rm bin/service-check.txt
```
Expected: the run passes. The output contains `admin page: 200`, `post from another site: 403`, `delete: 204`, `delete forever: 200` and `file store: 1 more after the capture, 0 more after the permanent delete`.

- [ ] **Step 6: Prove that the service comes back after a crash**

Run:
```bash
sudo systemctl kill -s KILL lifeos.service
sleep 15
systemctl is-active lifeos.service
curl -s -o /dev/null -w 'admin page: %{http_code}\n' http://127.0.0.1:8200/admin/items
```
Expected: the run passes. The output contains `active` and `admin page: 200`.

- [ ] **Step 7: Ship with the deploy script**

No hostname is in `deploy/.env` yet, so the Access check has nothing to check.

Run:
```bash
deploy/deploy.sh
ls /var/backups/lifeos/deploy/
```
Expected: the run passes. The output contains `database is up to date`, `lifeos answers on 127.0.0.1:8200`, `nothing to check` and `before-`.

If Docker reports `permission denied`, run `sg docker -c deploy/deploy.sh`.

---

### Task 8: Two public hostnames behind Cloudflare Access

This task changes Cloudflare and the tunnel of the Pi. It changes no file in git. Do the steps in their order: an Access application comes before its DNS record, and the DNS record comes before the tunnel rule.

**Files:**
- Create: `bin/cloudflare-access.py` (not in git)
- Modify: `deploy/.env` (not in git), `/etc/cloudflared/config.yml` (not in git)

**Interfaces:**
- Consumes: `deploy/check-access.sh` (Task 6); `lifeos.service` on `http_addr` (Task 7); `CLOUDFLARE_ACCOUNT_ID`, `CLOUDFLARE_API_TOKEN` and `NTFY_BASE_URL` in `deploy/.env`; `~/.cloudflared/cert.pem`.
- Produces: `LIFEOS_ADMIN_HOST`, `LIFEOS_API_HOST`, `CF_ACCESS_CLIENT_ID` and `CF_ACCESS_CLIENT_SECRET` in `deploy/.env`. Two Access applications, one service token, two DNS records and two tunnel rules.

- [ ] **Step 1: Get Gautam's OK**

Tell Gautam what this task does: it makes two Access applications and one service token in Cloudflare, makes two DNS records, changes `/etc/cloudflared/config.yml` and starts `cloudflared` again. The restart stops `ssh.`, `sysinfo.` and `ntfy.` for some seconds, so an SSH session through the tunnel can drop. Continue only after a yes.

- [ ] **Step 2: Put the two hostnames into `deploy/.env`**

The domain is the one of the ntfy hostname.

Run:
```bash
cd /home/gautam/lifeos-core
domain=$(sed -n 's#^NTFY_BASE_URL=https://ntfy\.##p' deploy/.env)
grep -q '^LIFEOS_ADMIN_HOST=' deploy/.env || printf '\n# Public hostnames of LifeOS (check-access.sh).\nLIFEOS_ADMIN_HOST=admin.%s\nLIFEOS_API_HOST=api.%s\n' "$domain" "$domain" >> deploy/.env
echo "hostnames: $(grep -c -E '^LIFEOS_(ADMIN|API)_HOST=(admin|api)\.[a-z0-9.-]+$' deploy/.env)"
```
Expected: the run passes. The output contains `hostnames: 2`.

- [ ] **Step 3: Write the setup script and do a dry run**

The script is for one use and is not in git: `/bin/` is in `.gitignore`. It prints no secret.

Create `bin/cloudflare-access.py`:

```python
#!/usr/bin/env python3
"""One-time setup of Cloudflare Access for the two public hostnames of LifeOS.

It reads deploy/.env and makes what is missing:
  - one Access application for LIFEOS_ADMIN_HOST, for the email addresses
    of an application that exists already (a login with an email code);
  - one Access application for LIFEOS_API_HOST, for a request that has a
    valid Access service token;
  - one service token for the Pi. Its two values go to the end of
    deploy/.env, for deploy/check-access.sh.

It prints no secret. An application has no policy for a moment after it is
made: Access refuses each request then, which is the safe side.

usage, from the root of the repository:
    python3 -I bin/cloudflare-access.py [--dry-run]
    python3 -I bin/cloudflare-access.py --aud    # the audience tag of each application
"""
import json
import pathlib
import sys
import urllib.error
import urllib.request

ENV = pathlib.Path("deploy/.env")
env = dict(line.split("=", 1) for line in ENV.read_text().splitlines() if "=" in line and not line.startswith("#"))
dry = "--dry-run" in sys.argv
base = f"https://api.cloudflare.com/client/v4/accounts/{env['CLOUDFLARE_ACCOUNT_ID']}/access"
admin_host, api_host = env["LIFEOS_ADMIN_HOST"], env["LIFEOS_API_HOST"]


def call(method, path, body=None):
    request = urllib.request.Request(
        base + path,
        method=method,
        data=None if body is None else json.dumps(body).encode(),
        headers={"Authorization": "Bearer " + env["CLOUDFLARE_API_TOKEN"], "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            return json.load(response)["result"]
    except urllib.error.HTTPError as error:
        sys.exit(f"{method} {path}: HTTP {error.code}: {json.load(error).get('errors')}")


apps = {app["domain"]: app for app in call("GET", "/apps")}

if "--aud" in sys.argv:
    for host in (admin_host, api_host):
        if host not in apps:
            sys.exit(f"no application for {host}")
        print(apps[host]["aud"])
    sys.exit(0)


def masked(address):
    name, _, domain = address.partition("@")
    return name[:1] + "***@" + domain


# The admin pages are for the email addresses of an application that
# exists. Only the email rules are copied. A policy that also has a
# "require" or an "exclude" part says more than its list of addresses, so
# it is not a safe source.
source = next(
    (
        (app, policy)
        for app in apps.values()
        if app["domain"] not in (admin_host, api_host)
        for policy in app.get("policies") or []
        if policy["decision"] == "allow" and any("email" in rule for rule in policy["include"])
    ),
    None,
)
if source is None:
    sys.exit("no Access application has an allow policy for an email address: make the two applications by hand")
source_app, source_policy = source
if source_policy.get("require") or source_policy.get("exclude"):
    sys.exit(f"the policy {source_policy['name']!r} of {source_app['name']!r} has a require or an exclude part: make the admin application by hand")
people = [rule for rule in source_policy["include"] if set(rule) == {"email"}]
skipped = len(source_policy["include"]) - len(people)
print(
    f"people      {len(people)} email address(es) from the policy {source_policy['name']!r} of {source_app['name']!r}: "
    + ", ".join(masked(rule["email"]["email"]) for rule in people)
    + (f"; {skipped} rule(s) of a different kind are not copied" if skipped else "")
)


def application(host, name, settings, policy):
    if host in apps:
        if not apps[host].get("policies"):
            sys.exit(f"the application for {host} exists and has no policy, so Access refuses each request: delete the application in Cloudflare, then run this again")
        print(f"exists      application for {host}")
        return
    if dry:
        print(f"would make  application {name!r} for {host}, policy {policy['name']!r} ({policy['decision']})")
        return
    app = call("POST", "/apps", {"type": "self_hosted", "name": name, "domain": host, "session_duration": "730h", **settings})
    call("POST", f"/apps/{app['id']}/policies", {**policy, "precedence": 1})
    print(f"made        application {name!r} for {host}, policy {policy['name']!r} ({policy['decision']})")


# The login cookie is not for a request that a different site starts, and
# not for a script of the page.
application(
    admin_host,
    "LifeOS admin",
    {"app_launcher_visible": True, "same_site_cookie_attribute": "lax", "http_only_cookie_attribute": True},
    {"name": "Allow Gautam", "decision": "allow", "include": people},
)
# non_identity is the decision "Service Auth": a request gets through with a
# valid service token, and no person can log in. Without a token the answer
# is 401, not the login page.
application(
    api_host,
    "LifeOS API",
    {"app_launcher_visible": False, "service_auth_401_redirect": True},
    {"name": "Devices with a service token", "decision": "non_identity", "include": [{"any_valid_service_token": {}}]},
)

TOKEN = "lifeos-pi-check"
if env.get("CF_ACCESS_CLIENT_ID") and env.get("CF_ACCESS_CLIENT_SECRET"):
    print("exists      service token values in deploy/.env")
elif any(token["name"] == TOKEN for token in call("GET", "/service_tokens")):
    sys.exit(f"the service token {TOKEN!r} exists in Cloudflare, but deploy/.env does not have its two values: delete the token in Cloudflare, then run this again")
elif dry:
    print(f"would make  service token {TOKEN!r}")
else:
    token = call("POST", "/service_tokens", {"name": TOKEN, "duration": "forever"})
    with ENV.open("a") as file:
        file.write(
            "\n# Cloudflare Access service token of the Pi (check-access.sh).\n"
            f"CF_ACCESS_CLIENT_ID={token['client_id']}\nCF_ACCESS_CLIENT_SECRET={token['client_secret']}\n"
        )
    print(f"made        service token {TOKEN!r}; its two values are at the end of deploy/.env")
```

Run:
```bash
python3 -I bin/cloudflare-access.py --dry-run
```
Expected: the run passes. The output contains `people      1 email address(es)`, `would make  application 'LifeOS admin'`, `would make  application 'LifeOS API'` and `would make  service token 'lifeos-pi-check'`.

The line `people` shows the source policy and each address with its name masked. Only these addresses can log in to the admin pages. Stop if the line shows an address that is not Gautam's.

- [ ] **Step 4: Make the two applications and the service token**

Run:
```bash
python3 -I bin/cloudflare-access.py
python3 -I bin/cloudflare-access.py --dry-run
echo "token values: $(grep -c -E '^CF_ACCESS_CLIENT_(ID|SECRET)=.' deploy/.env)"
```
Expected: the run passes. The output contains `made        application 'LifeOS admin'`, `made        application 'LifeOS API'`, `made        service token 'lifeos-pi-check'`, `exists      service token values in deploy/.env` and `token values: 2`.

If the script stops with `HTTP 403`, the API token has no right to write. Stop and tell Gautam. He has two options: give the token the permissions "Access: Apps and Policies: Edit" and "Access: Service Tokens: Edit" in the Cloudflare dashboard, or make the two applications and the token by hand there, with the settings that the script shows.

- [ ] **Step 5: Make the DNS records and check that Access guards them**

Each hostname has its Access application now, so its DNS record is safe.

Run:
```bash
set -a; . deploy/.env; set +a
tunnel=$(sed -n 's/^tunnel: *//p' /etc/cloudflared/config.yml)
cloudflared tunnel route dns "$tunnel" "$LIFEOS_ADMIN_HOST"
cloudflared tunnel route dns "$tunnel" "$LIFEOS_API_HOST"
sleep 30
deploy/check-access.sh
```
Expected: the run fails. The output contains `Access asks for a login`, `Access refuses the request (401)` and `status 404 with the service token, want 200`.

The one `FAIL` line is correct at this moment: the request with the service token gets through Access, and the tunnel has no rule for the hostname yet. Three lines start with `ok    ` for a request that has no login. No line starts with `OPEN`. If one does, do not do Step 6: an Access application is wrong. Remove its DNS record in the Cloudflare dashboard and tell Gautam.

- [ ] **Step 6: Add the two tunnel rules**

The rule of the admin hostname takes only `/` and the paths below `/admin`. The rule of the API hostname takes only the paths below `/api`. Each other path gets the last rule of the config, which answers 404.

Each rule has an `access` part. With it, `cloudflared` itself checks the signed token that Access adds to a request, and it refuses a request that has no valid token for this application. So the hostname stays closed when its Access application is removed or gets a wrong policy. `lifeos` still has no login of its own.

The backup of the config is made only after the check that the config has no rule for the two hostnames. Its path goes to `bin/cloudflared-config.backup` for Step 7.

Run:
```bash
set -a; . deploy/.env; set +a
config=/etc/cloudflared/config.yml
addr=$(sed -n 's/^http_addr: *//p' config/lifeos.yaml)
team=$(curl -s -o /dev/null -w '%{redirect_url}' "https://$LIFEOS_ADMIN_HOST/admin/items" | sed -n -E 's#^https://([a-z0-9-]+)\.cloudflareaccess\.com/.*#\1#p')
aud=$(python3 -I bin/cloudflare-access.py --aud)
admin_aud=$(echo "$aud" | sed -n 1p)
api_aud=$(echo "$aud" | sed -n 2p)
if [ -z "$team" ] || [ -z "$admin_aud" ] || [ -z "$api_aud" ]; then
    echo "the team name or an audience tag is missing"
    exit 1
fi
if grep -q -F -e "$LIFEOS_ADMIN_HOST" -e "$LIFEOS_API_HOST" "$config"; then
    echo "the config has a rule for one of the two hostnames already"
    exit 1
fi
backup=$config.bak-$(date +%Y%m%d-%H%M%S)
sudo cp -p "$config" "$backup"
echo "$backup" > bin/cloudflared-config.backup
sudo python3 -I - "$config" "$LIFEOS_ADMIN_HOST" "$LIFEOS_API_HOST" "$addr" "$team" "$admin_aud" "$api_aud" <<'RULES'
import sys

path, admin, api, addr, team, admin_aud, api_aud = sys.argv[1:8]
text = open(path).read()
last = "  - service: http_status:404"
if text.count(last) != 1:
    sys.exit("the config does not have the last rule in the form that this step expects")


def rule(host, regex, aud):
    return f"""  - hostname: {host}
    path: {regex}
    service: http://{addr}
    originRequest:
      access:
        required: true
        teamName: {team}
        audTag:
          - {aud}
"""


rules = rule(admin, "^/(admin(/.*)?)?$", admin_aud) + rule(api, "^/api(/.*)?$", api_aud)
open(path, "w").write(text.replace(last, rules + last))
RULES
cloudflared tunnel --config "$config" ingress validate
echo "rules with a token check: $(grep -c 'required: true' "$config")"
for url in "https://$LIFEOS_ADMIN_HOST/admin/items" "https://$LIFEOS_ADMIN_HOST/api/items" "https://$LIFEOS_API_HOST/api/items" "https://$LIFEOS_API_HOST/admin/items" "https://$LIFEOS_API_HOST/"; do
    printf '%s: ' "${url#https://}"
    cloudflared tunnel --config "$config" ingress rule "$url" | grep 'service:'
done
```
Expected: the run passes. The output contains `OK`, `rules with a token check: 2` and `service: http://127.0.0.1:8200`.

The five lines of the loop are, in this order: the address of `lifeos`, `http_status:404`, the address of `lifeos`, `http_status:404`, `http_status:404`.

- [ ] **Step 7: Start cloudflared again and run the check**

When `cloudflared` does not start, the script puts the old config back.

Run:
```bash
config=/etc/cloudflared/config.yml
backup=$(cat bin/cloudflared-config.backup)
sudo systemctl restart cloudflared
sleep 20
if ! systemctl is-active --quiet cloudflared; then
    sudo cp -p "$backup" "$config"
    sudo systemctl restart cloudflared
    echo "cloudflared did not start: the old config is back"
    exit 1
fi
echo "cloudflared: $(systemctl is-active cloudflared)"
deploy/check-access.sh
```
Expected: the run passes. The output contains `cloudflared: active`, `Access asks for a login`, `status 200 with the service token` and `status 404 with the service token`.

Five lines start with `ok    `. If a line starts with `OPEN`, put the backup config back (`sudo cp -p "$(cat bin/cloudflared-config.backup)" /etc/cloudflared/config.yml`), start `cloudflared` again and tell Gautam.

---

### Task 9: Documentation

**Files:**
- Modify: `AGENTS.md`, `README.md`

**Interfaces:**
- Consumes: the results of Tasks 1 to 8. Do this task only after Task 8 is complete: the text describes the two hostnames as a fact. If Task 8 is not complete, leave out the changes that name the public hostnames and tell Gautam.
- Produces: `AGENTS.md` rules 5, 6, 9 and 12 with `Purge` and `ListDeleted`, and the Deploy section with the four rules for the public hostnames. `README.md` with the permanent delete in sections 6 and 7, the new section 13, and the lists of pages and of missing parts.

- [ ] **Step 1: Change `AGENTS.md`**

In `AGENTS.md`, replace this:

```markdown
dashboards/            Grafana definitions, versioned with the schema they query
deploy/                docker-compose.yml, systemd units and timers, backup scripts, query-user.sh
docs/design.md         the design doc, exported
```

With this:

```markdown
dashboards/            Grafana definitions, versioned with the schema they query
deploy/                docker-compose.yml, systemd units and timers, backup scripts, query-user.sh, deploy.sh, check-access.sh
docs/design.md         the design doc, exported
```

In `AGENTS.md`, replace this:

```markdown
4. **Every write records provenance.** Each record row carries `source` (`manual`, `import`, `connector` or `agent`), `source_ref`, `verified`, `created_at` and `updated_at`. Rows an agent infers start with `verified = false`; rows Gautam dictates directly are verified. The adapter sets this: the REST API and the admin pages write `manual`, verified rows; an MCP tool writes `agent` rows that are verified only when the tool input says the user dictated the row. An update keeps `verified`. The service refuses a change that an agent infers to a verified row: the agent asks Gautam, then sends the change as dictated. Only `Verify` sets `verified` to `true`.
5. **Every write appends to `events`.** One row per write, with the old value on updates and deletes. `events` is append-only. A call that changes no value is not a write: it stores nothing and appends no event.
6. **Deletes are soft.** Set `deleted_at`; permanent removal happens only in the admin pages. The service refuses a delete from an agent unless the call says that Gautam confirmed it.
7. **Never store** passwords, card or bank account numbers, or government ID numbers. `checkText` in `internal/service/fields.go` refuses the ones that have a pattern: a card number, an Aadhaar number, a PAN, a private key and an access token (`secrets.go`). Send each text that a caller gives through `checkText` before the service stores it. The error names the kind of secret and never repeats it. A password in plain words and a bank account number have no pattern: the tool descriptions tell the agent not to send them.
8. **Store times in UTC** (`timestamptz`); display in IST.
9. **A record type has two entry points.** A REST route or an admin page knows its type and calls the function of that type (`GetItem`, `UpdateItem`, `DeleteItem`). The MCP tools `get`, `update` and `delete` get the type as input and call `Get`, `Update` and `Delete` in `internal/service/records.go`. The REST `PATCH` route calls `Update` too, because only a map of fields can tell a null from a field that is absent. These refuse a field that a caller must not set, such as `verified` or `source`. Add each new record type to `RecordTypes`, to these three functions, to `Verify` and to `ReviewQueue`. The record types are `item`, `journal`, `todo`, `habit_log` and `source`. A type that has text also gets a query in `queries/search.sql` and a case in `Search`. A habit (the definition) is not a record type: only the admin pages add or change one.
10. **The journal holds Gautam's own words.** The service stores the body of a journal entry exactly as it comes and never rewords it. The `journal` tool writes a verified row, because no agent infers a journal entry. An admin handler changes the CR LF of a text area to LF, which is what Gautam typed.
11. **A habit has one log for each day.** A second log for the same habit and day is not a write: the service returns the stored log. The one exception: a verified log over a log that is not verified approves that log. "Done" for a day means a verified log.
12. **A source is verbatim.** A source is one row of `sources`: a text, a link or a file that `Capture` stores exactly as it comes. The service never changes a source, so `update` refuses the type `source`. The same content a second time is not a write: `Capture` returns the stored source. A source is always verified, because it holds what came in and no agent inferred it. A record that an agent takes from a source names it in `source_ref`. A file is in the file store at `<files_dir>/<first two characters of its SHA-256>/<SHA-256>`; a soft delete keeps the file.
13. **`query` reads and never writes.** `Query` runs one SQL statement of an agent on a connection of the Postgres user `lifeos_query`, which has only the right to SELECT, in a read-only transaction. The database refuses a change; the Go code does not look at the SQL. `UseQueryPool` refuses a pool whose user can change a table. A new table is closed to `query` until its migration gives the right: `GRANT SELECT ON <table> TO lifeos_query`. Give the right to each table that an agent can read, and add the name of the table to the description of the `query` tool, which a test checks. Give no right to a table that holds a secret, such as a token or a password hash; the description names that table as one that `query` cannot read.
```

With this:

```markdown
4. **Every write records provenance.** Each record row carries `source` (`manual`, `import`, `connector` or `agent`), `source_ref`, `verified`, `created_at` and `updated_at`. Rows an agent infers start with `verified = false`; rows Gautam dictates directly are verified. The adapter sets this: the REST API and the admin pages write `manual`, verified rows; an MCP tool writes `agent` rows that are verified only when the tool input says the user dictated the row. An update keeps `verified`. The service refuses a change that an agent infers to a verified row: the agent asks Gautam, then sends the change as dictated. Only `Verify` sets `verified` to `true`.
5. **Every write appends to `events`.** One row per write, with the old value on updates and deletes. `events` is append-only. A call that changes no value is not a write: it stores nothing and appends no event. The one exception is `Purge`: it empties the old and the new value of each earlier event of the record that it removes. The rows stay, and `Purge` appends a `purge` event that has no value.
6. **Deletes are soft.** Set `deleted_at`; permanent removal happens only in the admin pages. The service refuses a delete from an agent unless the call says that Gautam confirmed it. The page `/admin/deleted` lists the deleted records (`ListDeleted`) and removes one for good (`Purge`). `Purge` takes only a record that is deleted already. No REST route and no MCP tool calls it.
7. **Never store** passwords, card or bank account numbers, or government ID numbers. `checkText` in `internal/service/fields.go` refuses the ones that have a pattern: a card number, an Aadhaar number, a PAN, a private key and an access token (`secrets.go`). Send each text that a caller gives through `checkText` before the service stores it. The error names the kind of secret and never repeats it. A password in plain words and a bank account number have no pattern: the tool descriptions tell the agent not to send them.
8. **Store times in UTC** (`timestamptz`); display in IST.
9. **A record type has two entry points.** A REST route or an admin page knows its type and calls the function of that type (`GetItem`, `UpdateItem`, `DeleteItem`). The MCP tools `get`, `update` and `delete` get the type as input and call `Get`, `Update` and `Delete` in `internal/service/records.go`. The REST `PATCH` route calls `Update` too, because only a map of fields can tell a null from a field that is absent. These refuse a field that a caller must not set, such as `verified` or `source`. Add each new record type to `RecordTypes`, to these three functions, to `Verify`, to `ReviewQueue`, to `Purge` and to `ListDeleted`. The record types are `item`, `journal`, `todo`, `habit_log` and `source`. A type that has text also gets a query in `queries/search.sql` and a case in `Search`. A habit (the definition) is not a record type: only the admin pages add or change one.
10. **The journal holds Gautam's own words.** The service stores the body of a journal entry exactly as it comes and never rewords it. The `journal` tool writes a verified row, because no agent infers a journal entry. An admin handler changes the CR LF of a text area to LF, which is what Gautam typed.
11. **A habit has one log for each day.** A second log for the same habit and day is not a write: the service returns the stored log. The one exception: a verified log over a log that is not verified approves that log. "Done" for a day means a verified log.
12. **A source is verbatim.** A source is one row of `sources`: a text, a link or a file that `Capture` stores exactly as it comes. The service never changes a source, so `update` refuses the type `source`. The same content a second time is not a write: `Capture` returns the stored source. A source is always verified, because it holds what came in and no agent inferred it. A record that an agent takes from a source names it in `source_ref`. A file is in the file store at `<files_dir>/<first two characters of its SHA-256>/<SHA-256>`; a soft delete keeps the file. `Purge` removes the file, unless a different source uses it. `Purge` refuses a source that a record names in `source_ref`.
13. **`query` reads and never writes.** `Query` runs one SQL statement of an agent on a connection of the Postgres user `lifeos_query`, which has only the right to SELECT, in a read-only transaction. The database refuses a change; the Go code does not look at the SQL. `UseQueryPool` refuses a pool whose user can change a table. A new table is closed to `query` until its migration gives the right: `GRANT SELECT ON <table> TO lifeos_query`. Give the right to each table that an agent can read, and add the name of the table to the description of the `query` tool, which a test checks. Give no right to a table that holds a secret, such as a token or a password hash; the description names that table as one that `query` cannot read.
```

In `AGENTS.md`, replace this:

```markdown
`lifeos serve` runs the API and admin pages on `http_addr` and the MCP server on `mcp_addr`, path `/mcp`. It needs the folders `files_dir` and `inbox_dir` of the config file, and the two passwords `POSTGRES_PASSWORD` and `LIFEOS_QUERY_PASSWORD` in the environment. `lifeos migrate`, `lifeos job <name>` and `lifeos import-csv <importer> <file>` are run by hand or by systemd timers. Each command reads `config/lifeos.yaml`, or the file given with `-config`.
```

With this:

```markdown
`lifeos serve` runs the API and admin pages on `http_addr` and the MCP server on `mcp_addr`, path `/mcp`. On the Pi it runs as the systemd unit `deploy/systemd/lifeos.service`; `make run` is for work on the code. It needs the folders `files_dir` and `inbox_dir` of the config file, and the two passwords `POSTGRES_PASSWORD` and `LIFEOS_QUERY_PASSWORD` in the environment. `lifeos migrate`, `lifeos job <name>` and `lifeos import-csv <importer> <file>` are run by hand or by systemd timers. Each command reads `config/lifeos.yaml`, or the file given with `-config`.
```

In `AGENTS.md`, replace this:

```markdown
`deploy/deploy.sh` on the Pi: `git pull`, `go build`, `lifeos migrate`, restart the service. Every service listens on `127.0.0.1` only; the Cloudflare Tunnel is the single way in.
```

With this:

```markdown
`deploy/deploy.sh` on the Pi: `git pull`, build, dump the database, `lifeos migrate`, restart `lifeos.service`, then `deploy/check-access.sh`. Every service listens on `127.0.0.1` only; the Cloudflare Tunnel is the single way in.

**LifeOS has no login of its own.** Cloudflare Access is the only lock on the two public hostnames: one for the admin pages (a login with an email code) and one for the REST API (an Access service token for each device). The names are in `deploy/.env`. So:

- Never add a hostname to the tunnel before its Access application exists.
- Run `deploy/check-access.sh` after each change of the tunnel or of an Access application. It fails when a hostname answers a request that has no login.
- `serve.go` puts `http.CrossOriginProtection` in front of the admin pages and the API. It refuses a request that changes data and that a browser sent from a different origin. Keep it: without it, a page of another site can use the Access login of the browser.
- The MCP listener has no public hostname. Give it one only after the MCP handler checks the token of Access itself (Phase 3).
```

- [ ] **Step 2: Change `README.md`**

In `README.md`, replace this:

```markdown
| 1 · Foundation | Done: Docker, Postgres, ntfy, nightly backup, restore test. |
| 2 · Core data | In work. Lists, journal, todos, habits, the review queue, capture, search, the read-only query and the three interfaces are built. Money tables and CSV importers are not built. |
| 3 to 5 | Not started. |
```

With this:

```markdown
| 1 · Foundation | Done: Docker, Postgres, ntfy, nightly backup, restore test. |
| 2 · Core data | Built: lists, journal, todos, habits, the review queue, capture, search, the read-only query, permanent delete, the three interfaces, the service unit and the public hostnames. The gate is one week of daily logging. Money tables and imports are in [docs/backlog.md](docs/backlog.md). |
| 3 to 5 | Not started. |
```

In `README.md`, replace this:

```markdown
- **Delete.** A delete is soft: the row gets a `deleted_at` time and leaves every list. An agent can delete a record only after you said yes.
- A habit log has no field that you can change. Delete the log, then log the habit for the correct day.
```

With this:

```markdown
- **Delete.** A delete is soft: the row gets a `deleted_at` time and leaves every list. An agent can delete a record only after you said yes.
- **Delete forever.** The page `/admin/deleted` lists the deleted records. "Delete forever" removes the row, empties the old and the new value of each event of that record, and removes the file of a file source unless a different source uses it. Only this page can do it: no agent tool and no REST route can. A source that a record names in `source_ref` stays until that record is gone. The backups that were made before keep the record.
- A habit log has no field that you can change. Delete the log, then log the habit for the correct day.
```

In `README.md`, replace this:

```markdown
Each write appends one row to the `events` table in the same transaction: the action (`create`, `update`, `delete` or `verify`), the source, the old value and the new value. A call that changes no value appends nothing.
```

With this:

```markdown
Each write appends one row to the `events` table in the same transaction: the action (`create`, `update`, `delete`, `verify` or `purge`), the source, the old value and the new value. A call that changes no value appends nothing. The log only grows, with one exception: a permanent delete empties the values of the events of its record.
```

In `README.md`, replace this:

```markdown
## Interfaces
```

With this:

```markdown
### 13. Service, deploy and public hostnames

- **Service.** `deploy/systemd/lifeos.service` starts `lifeos serve` at boot and starts it again after a failure. After ten failed starts in five minutes, ntfy gets a message. The logs are in the journal: `journalctl -u lifeos`.
- **Deploy.** `deploy/deploy.sh` pulls the code, builds it, dumps the database to `/var/backups/lifeos/deploy/`, applies the migrations, starts the service again and runs the check below.
- **Public hostnames.** The Cloudflare Tunnel gives the admin pages one hostname and the REST API a second one. Cloudflare Access guards each: a login with an email code for the admin pages, an Access service token for each device for the API. LifeOS has no login of its own, so Access is the only lock.
- **Check.** `deploy/check-access.sh` fails when a hostname answers a request that has no login.
- **Requests from another site.** The server refuses a request that changes data and that a browser sent from a different origin. A program, such as an iOS Shortcut, is not a browser and goes through.

**Use case.** You add a hostname to the tunnel and forget its Access application. `deploy/check-access.sh` prints `OPEN` for that hostname and exits with an error.

## Interfaces
```

In `README.md`, replace this:

```markdown
There is no login yet. Do not put these listeners on a public address.
```

With this:

```markdown
LifeOS has no login of its own. The admin pages and the REST API are public only through the Cloudflare Tunnel, each behind Cloudflare Access (section 13). The MCP listener has no public hostname.
```

In `README.md`, replace this:

```markdown
| `/admin/review` | See the records that wait, and approve them. |
```

With this:

```markdown
| `/admin/review` | See the records that wait, and approve them. |
| `/admin/deleted` | See the deleted records, and remove one for good. |
```

In `README.md`, replace this:

```markdown
| Command | Result |
```

With this:

````markdown
On the Pi, the server runs as a systemd unit and `make run` is only for work on the code:

```sh
sudo systemctl link "$PWD/deploy/systemd/lifeos.service"   # one time
sudo systemctl enable --now lifeos.service                 # one time
deploy/deploy.sh                                           # after each change of the code
```

The unit file holds the path `/home/gautam/lifeos-core` and the user `gautam`.

| Command | Result |
````

In `README.md`, replace this:

```markdown
- **Lists in the REST API.** No route lists journal entries or sources. The admin pages list them, and the agent uses `search` and `query`.
- **Money tables and CSV importers.** `lifeos import-csv` and `lifeos job` return "not implemented".
- **Login, tokens and the systemd unit** for `lifeos serve`, and permanent delete in the admin pages.
```

With this:

```markdown
- **Lists in the REST API.** No route lists journal entries or sources. The admin pages list them, and the agent uses `search` and `query`.
- **Money tables, the reconciliation with a bank statement and the one-time imports.** They are in [docs/backlog.md](docs/backlog.md). `lifeos import-csv` and `lifeos job` return "not implemented".
- **A public hostname for the MCP server.** It comes in Phase 3, with a login through Cloudflare Access and a check of the Access token in `lifeos`.
- **A way to get a deleted record back.** The page `/admin/deleted` only removes.
```

In `README.md`, replace this:

```markdown
migrations/          goose SQL migrations
deploy/              docker-compose.yml, backup scripts, query-user.sh, systemd units
docs/                design doc, backlog, implementation plans
```

With this:

```markdown
migrations/          goose SQL migrations
deploy/              docker-compose.yml, backup scripts, query-user.sh, deploy.sh, check-access.sh, systemd units
docs/                design doc, backlog, implementation plans
```

- [ ] **Step 3: Run all tests**

Run:
```bash
make test ARGS="-count=1"
```
Expected: the run passes. The output contains `ok  	github.com/Gautam-J/lifeos-core/internal/service` and `ok  	github.com/Gautam-J/lifeos-core/cmd/lifeos`.

`make test` prints `ok` for seven packages.

- [ ] **Step 4: Commit**

Run:
```bash
git add AGENTS.md README.md
git commit -m "docs: describe permanent delete, the service unit and the public hostnames"
```

---

### Task 10: Confirmation gate

Stop here and ask Gautam to do these checks. Do not start other work before his answer.

- [ ] **Step 1: Gautam opens the admin pages from the phone**

Open `https://<LIFEOS_ADMIN_HOST>/` on the phone, not on the Wi-Fi of the Pi. Cloudflare asks for the email address and sends a code. After the code, the page "Items" shows.

- [ ] **Step 2: Gautam removes one record for good**

Add an item on the page "Items". Delete it. Open "Deleted": the item is in the list. Select "Delete forever" and confirm. The list is empty again.

- [ ] **Step 3: Gautam checks the Cloudflare account**

The Cloudflare account is the one lock on all data. Check that it has two-factor login.

- [ ] **Step 4: Report**

Tell Gautam:

- The server runs as `lifeos.service`. The tmux window `lifeos-serve` is not necessary any more.
- A new version of the code goes live with `deploy/deploy.sh`.
- An iOS Shortcut needs its own Access service token: Cloudflare dashboard, Zero Trust, Access, Service Auth. The Shortcut sends its two values in the headers `CF-Access-Client-Id` and `CF-Access-Client-Secret` to `https://<LIFEOS_API_HOST>/api/...`.
- The gate of Phase 2 starts now: one week of daily logging through the admin pages.

## After this plan

Phase 2 has no more plan. These parts wait:

| Where | Content |
| --- | --- |
| `docs/backlog.md` | Money tables with entry by hand, the reconciliation with a bank statement, the one-time imports, a schema for the `attrs` of each item kind, duplicate detection for items, a `description` field for todos, a `projects` table |
| Phase 3 | A public hostname for the MCP server: first a test of "Managed OAuth" of Cloudflare Access with the Claude connector, then a check of the Access token in the MCP handler, then the tunnel rule with the Host header that the MCP SDK accepts |
| Phase 4 | The inbox job (the page of a link, the text of a PDF, `processed_at`), a page that shows a stored file, the monitoring section of the design doc. A job that starts `claude -p` must give it an environment without the secrets of `deploy/.env` |
| Not planned | A way to get a deleted record back. A Postgres user for the app that is not a superuser |
