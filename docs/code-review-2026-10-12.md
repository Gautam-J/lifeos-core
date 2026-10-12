# Code review, 2026-10-12

A review of each feature of `lifeos-core` at commit `0a8bd7b` (release v0.1.0): the code quality, and how each feature compares with the usual way to build it. No item here is done. An item becomes work when it has a GitHub issue ([AGENTS.md](../AGENTS.md), Issues).

The times are estimates for the change with its tests.

## Verdict

The base is strong. The findings are about growth: the lists, the backups and the number of record types.

Checks that ran on the day of the review:

- `gofmt` and `go vet`: clean.
- `govulncheck`: "No vulnerabilities found."
- Layering: no package outside `internal/service` imports `internal/store`, and no SQL string is outside sqlc.
- Tests: 13,691 lines of tests for 8,461 lines of code.
- `systemd-analyze security lifeos.service`: 7.3 MEDIUM.

## 1. Defects (fix first)

| ID | Defect | Evidence | Fix | Time |
| --- | --- | --- | --- | --- |
| D1 | Admin lists stop at 50 rows; REST stops at 200. Row 51 of the journal is not reachable in the admin pages. | `internal/admin/journal.go:20`, `admin.go:124`, `todos.go:13`, `sources.go:37`. No query has an offset or a cursor. | Pagination by ID (the next page starts after the last ID) and an "Older" link. | 3 h |
| D2 | A deleted record has no restore. `/admin/deleted` gives only "Delete forever". | The service has no `Restore`. | `Restore`, a `restore` event and a button. Refuse when a live duplicate exists. | 2 h |
| D3 | The local API has no login. Each program on the Pi can write `manual`, verified rows, and can purge. | `curl 127.0.0.1:8200/api/openapi.json` gave 200 with no token. Only the MCP server checks the token. | Move `AccessCheck` to `internal/access` and use it for the three hostnames. | 3 h |
| D4 | A 50 MB upload is in memory two times. | `internal/api/sources.go:26` holds the body as `[]byte`. `internal/service/files.go:176` reads the full file to find its type. | Stream the body. Read 512 bytes for the type. Read the full file only for text. | 1.5 h |
| D5 | The backup removes no old snapshot and checks none. | `deploy/backup.sh:22-24` has no `restic forget --prune` and no `restic check`. | Add a retention rule and a weekly check. | 30 min |

D3 needs a decision. Agents with a shell run on the Pi, so `curl` bypasses rules 4 and 6 of AGENTS.md. The cost of the fix: `deploy.sh` and each local `curl` then need a token, or a `/healthz` route without the check.

## 2. Small defects

- **M1.** The version is fixed text: `internal/api/api.go:20` and `internal/mcp/server.go:18` say `"0.1.0"`. It becomes wrong at the next `cz bump`. Use `version.Version`. 10 min.
- **M2.** `restore-test.sh` has no timer, and it compares only row counts, not the file store. 45 min.
- **M3.** `/api/docs` loads its script from unpkg.com (the default of Huma, with a pinned hash). Set `DocsPath = ""` or serve the file locally. 15 min.
- **M4.** The wiki lock is a `sync.Mutex` in one process (`internal/service/service.go:23`). A Phase 4 job that writes a review page is a second process. Use a file lock before Phase 4. 1 h.
- **M5.** `decodeFields` reads the error text of `encoding/json` (`internal/service/records.go:163`). A Go release can change that text. S3 removes this code.

## 3. Structure

- **S1. One registry for the record types.** One type has five names (`habit_log`, `habit_logs`, `habitlog`, `HabitLog`, "habit log"). Nine places in the service and two in admin hold a case or a list for each type, and rule 9 of AGENTS.md is a checklist because of that. One struct value per type replaces the nine places, so a new type becomes one file. Estimate: 300 lines less. 1 day.
- **S2. One helper for a write transaction.** 15 functions repeat begin, rollback, commit and the same error wrap. A helper `s.write(ctx, "add item", fn)` removes that. If `fn` must return the event, a write without an event does not compile. 3 h.
- **S3. A three-state field type for updates** (absent, null, value). It removes `Clear`, `clearableItemFields`, the conflict checks, `decodeFields` and the cast `record.(T)` in `api.patch`. REST `PATCH` then follows RFC 7396, where null empties a field. That is a behavior change; the MCP tool can keep `clear`. 4 h.
- **S4. Admin handlers.** Five copies of the add, save and delete flow differ only in the noun, the service call and the list. One helper removes about 120 lines. `listFailure` renders `itemsFailure` for each type. 2 h.
- **S5. Small duplicates.** `internal/service/items.go:111` copies `pastDay` and `items_read.go:44` copies `checkLimit`. `recordWikiEvent` copies `recordEvent`. `wiki_write.go` (485 lines) holds four concerns: write, git, index and list. 1 h.

No Go file is near 1,000 lines.

## 4. Standards: security and operations

- **O1. No CI.** Tests run only by hand on the Pi. Add a GitHub Actions workflow: generate, vet, test with a Postgres container, `govulncheck`, `cz check`. Add Dependabot. 2 h.
- **O2. The server connects to Postgres as the superuser** (`POSTGRES_USER: lifeos`). Use a role without that right for `serve`. Then privileges can make `events` append-only; now it is a convention. 2 h.
- **O3. `lifeos_query` has no limit in the database.** Add `statement_timeout`, `default_transaction_read_only` and a connection limit to the role. Now only the Go code stops a long query. 20 min.
- **O4. systemd.** The unit runs as the login user. Add a service user, `SystemCallFilter`, `RestrictAddressFamilies`, `CapabilityBoundingSet=` and `MemoryMax`. 1 h.
- **O5. The admin pages send no security header.** Add `nosniff`, `Referrer-Policy` and a CSP with `frame-ancestors 'none'`. One htmx event filter needs `unsafe-eval` or a rewrite. 1 h.

## 5. Standards: API, MCP and data

- **A1. No MCP tool has annotations.** Set `readOnlyHint` on the seven read tools and `destructiveHint` on `delete` and `wiki_delete`. Clients can use them for permission prompts. 30 min.
- **A2. A retry of `POST /api/journal` stores a duplicate.** The standard is an `Idempotency-Key` header. `capture` and `log_habit` are safe already. 2 h.
- **A3. Search uses `ILIKE '%word%'` with no index and no rank.** The standard is a `pg_trgm` index, or `tsvector` with `ts_rank`. It is fast enough today. Do it before issue #13. 3 h.
- **A4. An event records only `manual` or `agent`.** The Access token names the email or the device. Also, `verify` from an agent has no guard like `confirmed` on `delete`. Show the approvals that came through an agent on the Review page. 2 h.
- **A5. Each test makes a database and runs all migrations.** A template database removes the time of the migrations (1 h). It does not cut the 2.5 GB of writes: each `CREATE DATABASE` copies 7.7 MB, and a template keeps that copy. One database for each test package, emptied between tests, cuts the time and the writes (3 h). Issue #23 has the two options.

## 6. Small hardening (each below 1 hour)

- **H1.** Add `/healthz` and a request log. `deploy/deploy.sh:32` uses `/admin/items` as the health check.
- **H2.** `make tools` runs `curl | tar` with no checksum (`Makefile:27`).
- **H3.** No index on `source_ref`, `events(occurred_at)` or `todos(status, due_on)`.
- **H4.** `deploy.sh` keeps no earlier binary, so there is no fast rollback.
- **H5.** The token check has no `nbf` check. The code is correct for what it checks: algorithm, issuer, audience and expiry.

## 7. New features (not in the open issues #1 to #20)

- **F1. MCP prompts.** Ship the drafts in `skills/` as MCP prompts. They become slash commands in each client. 3 h.
- **F2. Record history.** A page that shows the events of one record with the old and new values. The data is in `events` already. 4 h.
- **F3. Link snapshot.** `capture` stores only the URL, and links go dead. Get the page text at capture time. The table constraint permits `content` for a link already. It needs a guard against requests to local addresses. 1 day.
- **F4. Admin search and a read-only wiki view.** `Search` and `WikiRead` exist; no page uses them. 4 h.
- **F5. `lifeos export`.** One archive of JSON and files that a person can read without Postgres. 3 h.

## Not verified

- The fsync setting of the wiki repository. The server writes page files with no fsync (the file store does one). A power cut can damage the repository; the hourly mirror limits the loss.
