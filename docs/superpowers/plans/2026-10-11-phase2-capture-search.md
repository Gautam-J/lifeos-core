# Phase 2 Capture, Search and Query Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Gautam and his agent can keep a text, a link or a file as a source, find a record by its words, and answer a question with a read-only SQL query; and LifeOS refuses a card number, an ID number and a secret in each text that it stores.

**Architecture:** One migration adds the table `sources` and makes `source_ref` a foreign key; a second migration gives the read-only Postgres user `lifeos_query` the right to SELECT. `internal/service` gets four parts: the check for secrets inside `checkText`, `Capture` with a file store on disk, `Search` over four tables, and `Query` on a second connection pool. `source` becomes the fifth record type. The REST API, the MCP tools and the admin pages stay thin adapters.

**Tech Stack:** No new dependency. Go 1.26.0 (`os.Root` keeps a path inside the inbox), chi v5.3.2, Huma v2.39.1, pgx v5.11.0, sqlc v1.31.1, goose v3.28.0, MCP Go SDK v1.8.0, templ v0.3.1070, htmx 2.0.11, Postgres 18 in Docker.

**Spec:** `docs/design.md` (section "Data model": the row `sources` and the provenance field `source_ref`; section "Access layer": the rows `capture`, `search` and `query`, and the rule "`query` connects as a Postgres user with SELECT rights only"; workflows S5 to S7, S12 and S13; edge cases E1, E8, E9 and E15) and `AGENTS.md`. Read both before you start a task.

**Scope:** This is plan 4 of Phase 2. Plans 1 to 3 are complete and confirmed. This plan stops at a confirmation gate (Task 16). The section "After this plan" lists the Phase 2 work that later plans cover. Do not start that work in this plan.

**Tested:** Every code block in this plan comes from a scratch copy of the repository. In that copy each test failed first, then passed after its code was added, in the task order below. Then the steps of this file were done one more time on a fresh clone, by a script that reads this file.

## Changes after the review

The plan was executed on 2026-10-11. One reviewer read the full change before the migrations went to the real database. Five findings were corrected, each with a test that failed first. A sixth change, the last row, came from Gautam's decision on a finding that the review left open. The code blocks below show the code before these corrections; the repository has the code after them.

| Change | Where | Why |
| --- | --- | --- |
| A second request for the same inbox file gets no internal error. | `internal/service/files.go` (Task 5) | Two requests at the same moment: one removed the file, and the other failed. |
| The file store writes a file and its folder to the disk before the file leaves the inbox. A stored file of the wrong size is replaced. | `internal/service/files.go` (Task 5) | A power cut could leave an empty stored file, and the inbox file was gone. |
| One row of a query has a limit of 2 MB. | `internal/service/query.go` (Task 14) | One statement could return a row of 1 GB and use all the memory of the Pi. |
| A card number must have a length that its card network uses. | `internal/service/secrets.go` (Task 1) | The IMEI of a phone has 15 digits and a Luhn check digit, and was refused as a card number. This replaces "13 to 19 digits" in the decisions below. |
| Migration `00004` gives `lifeos_query` the right to read the tables of now only. A later migration gives the right for its own table. | `migrations/00004_query_user.sql` (Task 13), `AGENTS.md` rule 13 | A later table with tokens or a password hash must not be open to the `query` tool by default. This replaces "also from a table that a later migration makes" below. |
| 12 digits in one piece are an Aadhaar number only when a name of Aadhaar is near them (`Aadhaar`, `aadhar`, `UID`, `UIDAI`, `आधार`, within 40 characters). The form `XXXX XXXX XXXX` is refused in each text, as before. | `internal/service/secrets.go` (Task 1) | The first rule refused about 1 of 10 of all numbers of 12 digits, such as the reference number of a UPI payment. Gautam chose this rule on 2026-10-11, after his checks. It replaces "The Aadhaar check has a cost" below. |

## Decisions in this plan

Gautam approved the table, the behaviour and the four decisions that are marked "approved" on 2026-10-11. The plan decides the other points as follows. Gautam can change a decision before the work starts.

**The table.** `sources` has the provenance columns, `deleted_at` and the `set_updated_at()` trigger, as each record table has.

| Column | Content |
| --- | --- |
| `kind` | `text`, `link` or `file` |
| `title` | Optional label; the file name of a file |
| `content` | The text of a `text` capture. Empty for a link and a file in this plan |
| `url` | The link of a `link` capture |
| `file_path`, `media_type`, `size_bytes` | For a `file` capture: the path below the file store, the type that the service finds in the bytes, the size |
| `sha256` | Hex SHA-256 of the text, the URL or the file bytes. Unique for each kind among the live rows |
| `processed_at` | Empty until a later inbox job reads the capture. Nothing sets it in this plan |

**Capture**

- **Capture depth (approved).** A link is stored as its URL: the service does not get the page. A file is stored as its bytes: the service does not read the text of a PDF. The plan with the inbox job adds these. A page fetch needs a guard against requests to private addresses, and `pdftotext` is not on the Pi.
- **A capture is verbatim.** The service stores a text exactly as it comes and does not trim it. `update` refuses the type `source`. To correct a capture, delete it and capture again.
- **The same content two times is one source (E1).** The second call returns the stored source and writes nothing, with no error. A unique index on `(kind, sha256)` makes this true for two requests at the same moment. A deleted source does not count: a new capture of its content is a new source.
- **A source is always verified.** It holds what came in; no agent inferred it. The `capture` tool has no `dictated` argument. `source` on the row says who captured: `manual` or `agent`.
- **The Go type is `Capture`; the record type is `source`.** The name `service.Source` is taken: it is the provenance value (`manual`, `agent`, ...).
- **The file store (approved).** `/var/lib/lifeos/files` holds the files, `/var/lib/lifeos/inbox` is the inbox, and the limit is 50 MB for each file. A file is at `<files_dir>/<first two characters of its SHA-256>/<SHA-256>`, so the same bytes are one file. The row has the original name in `title`.
- **A file comes in two ways.** The MCP tool `capture` gets a path in the inbox folder (S7) and moves the file. `POST /api/ingest/file` gets the bytes as the body of the request (S5, S6). `os.Root` refuses a path that leaves the inbox, also through a symbolic link.
- **A soft delete keeps the file.** Permanent removal is in plan 6.
- **`POST /api/ingest` and `POST /api/ingest/file` answer 200,** not 201, because a second request with the same content creates nothing.
- **`source_ref`.** `add_item`, `todo` and `log_habit` get an optional `source_ref`. The service refuses the ID of a source that does not exist or that is deleted. The REST API and the admin pages do not set it.
- **No REST route lists sources.** The admin page lists them. No page shows or downloads a stored file in this plan.

**Secrets**

- **What the service refuses (approved).** A card number (13 to 19 digits, the first digits of a card network, a correct Luhn check digit), an Aadhaar number (12 digits that do not start with 0 or 1, a correct Verhoeff check digit), a PAN (the format `AAAPA9999A` with a valid fourth letter), a private key block, and an access token of AWS, GitHub or Slack or an API key that starts with `sk-`.
- **What it cannot find (approved for bank accounts).** A bank account number and a password in plain words have no pattern. A link of the form `https://name:password@host` does have one, and the service refuses it.
- **The Aadhaar check has a cost (approved).** About 1 of 10 phone numbers in the form `91XXXXXXXXXX` has a correct check digit, and the service refuses it. The form `+91 XXXXX XXXXX` is never refused.
- **A number in groups.** `4111 1111 1111 1111` and `4111-1111-1111-1111` are found, also after a date. Three groups of four digits are an Aadhaar number only when no group of four digits is next to them.
- **Where the check runs.** In `checkText`, which each text field goes through: titles, a journal body, a project, a habit name, the attrs of an item (each name, each text and each number, at any depth), a captured text, a link, and the name of a file. A text file is checked too.
- **A refusal stores nothing and names only the kind.** The error says "must not hold a card number" and never repeats the number. The journal rule (AGENTS.md rule 10) holds: the service refuses the entry; it does not change it.
- **Known limits.** The service does not look inside a PDF, an image or a text file that is not UTF-8. A search and a query do not go through the check, because they store nothing. A row that was stored before this plan is not checked, but an update of a row checks each of its text fields.

**Search**

- **Words, not a search language.** A record matches when its text has every word of the query. Upper and lower case are the same, and a part of a word matches: `lonav` finds "Lonavala". A percent sign and an underscore are plain characters.
- **What is searched.** Items (kind, title, attrs), journal entries (body, day), todos (title, project) and sources (title, content, URL). Not habit logs: a question about habits is a count, and `query` answers it.
- **The newest hit is first,** across the types. A hit has the type, the ID, one line of summary and `verified`. A record that waits for review is a hit, with `verified` false.
- **No index.** Each search reads each live row. That is quick for the data of one person. When a search gets slow, a later plan adds a trigram index.
- **The summary of a todo now shows a status that is not `open`.** An agent that looks for a todo must see that it is done. The review queue uses the same line.
- **The REST parameter is `query`,** the same word as the MCP argument: `GET /api/search?query=dune&types=item,todo`.

**Query**

- **A second Postgres user.** `lifeos_query` can log in and can SELECT; it has no other right. `make query-user` makes it with the password `LIFEOS_QUERY_PASSWORD` from `deploy/.env`. A migration cannot do that: a user belongs to the server, not to one database, and its password must not be in git.
- **The database is the guard.** `Query` does not look at the SQL. The statement runs as `lifeos_query`, in a read-only transaction, as one prepared statement (so two statements in one text are refused), for 5 seconds at most.
- **A wrong config cannot open a write path.** `UseQueryPool` asks the database if its user can change a table, and refuses the pool when the answer is yes.
- **One connection for each query.** The connection closes after the statement, so a setting or a lock of one query cannot reach the next.
- **The answer is text.** Each value comes as Postgres writes it (`2026-10-11`, `t`, `1.50`); a NULL is a JSON null. At most 200 rows and about 256 kB; `truncated` says when there are more.
- **The error is for the agent.** A wrong statement gives the message of Postgres and "Correct the statement, then call query again", not "Ask the user".
- **`query` can read every table,** also `events`, which holds the old values of deleted rows. That is what the spec asks for; the agent is Gautam's own.
- **No REST route.** The spec gives `query` to agents only.
- **The role `dbeaver_ro` on the server is not part of LifeOS.** Do not change it.

**All parts**

- **The nightly backup gets the file store.** `deploy/backup.sh` puts `/var/lib/lifeos/files` in the same snapshot as the database dump. The restore test still compares only the row counts.
- **The real database gets the two migrations in Task 16.** `00003` adds a table and six foreign keys; each `source_ref` in the database is NULL now, so no row can break a key. `00004` gives rights.
- **Not in this plan.** The fetch of a page, the text of a PDF, the inbox job, a search page in the admin, a download of a stored file, permanent delete, a restore test for the files.

## Prerequisites

These are done on the Pi (2026-10-11). Step 1 of Task 1 checks them.

- The folders `/var/lib/lifeos/files` and `/var/lib/lifeos/inbox` exist and belong to `gautam`.
- `deploy/.env` has the line `LIFEOS_QUERY_PASSWORD=...`. Never print its value.

## Global Constraints

- **One logic layer.** All validation, provenance and event code is in `internal/service`. An adapter (`internal/api`, `internal/mcp`, `internal/admin`, `cmd/lifeos`) parses input, calls the service and formats the result. No adapter calls a different adapter.
- **SQL only through sqlc.** Queries are in `queries/*.sql`. Only `internal/service` imports `internal/store`. No SQL strings in other non-test Go code. The one exception is `Query`, which runs the statement of an agent.
- **One event for each write.** The service appends one `events` row in the same transaction as the write. A call that changes no value is not a write: a second capture of the same content appends no event.
- **Provenance.** The REST API and the admin pages write `manual` rows. An MCP tool writes `agent` rows. A source is always verified. For the other types the rules of plans 1 to 3 hold.
- **Soft delete.** A delete sets `deleted_at`. Every read, search, update, delete and verify leaves out rows where `deleted_at IS NOT NULL`. An agent needs `confirmed` to delete.
- **Secrets.** Never print, log or commit the value of a variable in `deploy/.env`. An error message never repeats the secret that it refuses.
- **Test tokens are built from parts.** A test writes `"AKIA" + "IOSFODNN7EXAMPLE"`, not the token in one piece. The secret scanner of GitHub refuses a push that has a token in one piece.
- **Tests never touch `/var/lib/lifeos`.** A test that needs a file store uses `t.TempDir()`.
- **Time.** Store `timestamptz` in UTC. Show times in IST. "Today" for a date-only field is the IST date.
- **Loopback only.** Every listener is on `127.0.0.1`. Do not add a Cloudflare Tunnel hostname in this plan.
- **Migrations.** `00001_init.sql` and `00002_daily_records.sql` are applied on the Pi. Do not edit them. This plan adds `00003_sources.sql` and `00004_query_user.sql`. Do not edit those after Task 16 applies them.
- **Generated code is not in git.** `internal/store/` (sqlc) and `*_templ.go` (templ) are in `.gitignore`. Never `git add` them. `make build` and `make test` generate them first.
- **Tests use real Postgres.** Run tests with `make test`, not `go test`. Each test creates its own database and drops it. The Postgres container from `deploy/docker-compose.yml` must be running. From Task 13, the tests also need the user `lifeos_query`.
- **Where to work.** Work in `/home/gautam/lifeos-core` on `main`. Do not make a branch or a worktree: `deploy/.env` is not in git and exists only in this checkout. Do not push.
- **Commits.** Conventional commit format, one concern for each commit, with the exact file lists in the steps.
- **Out of scope for this plan.** Money tables, CSV importers, API tokens, the admin password, CSRF protection, a systemd unit, permanent delete, the entries in `docs/backlog.md`, and the minor review findings from plans 1 to 3. One of those findings is closed as a side effect: a NUL character in the title or the attrs of an item now gives a validation error, because an item goes through `checkText`.

## Review Focus

These five conditions are implied by the spec, and no scenario in the spec tests them. Each has a test in the task that owns the code.

1. **The same thing is captured two times.** A double tap on the share sheet, or two requests at the same moment. One source must be stored, no request must fail, and a file must leave the inbox. Tests: `TestCaptureOfTheSameContentReturnsTheStoredSource` and `TestCaptureOfTheSameContentAtTheSameMomentStoresOneSource` (Task 4), `TestCaptureOfTheSameFileReturnsTheStoredSource` (Task 5).
2. **A normal number that looks like a secret.** A phone number, a date, an ISBN, an amount and a timestamp must be stored. If the check refuses them, Gautam cannot write his journal. Tests: `TestSecretKindAcceptsTextWithoutASecret` (Task 1), `TestTextWithoutASecretIsStored` (Task 2).
3. **A secret that does not come as plain text in a text field.** A card number as a JSON number in the attrs, in the name of a file, in a text file, in groups after a date; a token or a password in a link. Tests: `TestSecretKindFindsEachKindOfSecret` (Task 1), `TestAddItemRefusesASecret` (Task 2), `TestCaptureRejectsBadInput` (Task 4), `TestCaptureRejectsABadFile` (Task 5).
4. **A path that leaves the inbox.** A web page tells the agent to capture `../../home/gautam/.ssh/id_ed25519` (E9). `..`, a full path outside the inbox and a symbolic link must be refused, and the file outside must stay. Tests: `TestCaptureRejectsABadFile` (Task 5), `TestCaptureAFileFromTheInbox` (Task 9).
5. **A query that tries to change data, to stay, or to run without end.** A write in a `WITH`, two statements in one text, a setting that stays in the session, `pg_sleep`, and a config file that names the owner of the tables. Tests: `TestTheQueryUserCanReadAndCannotWrite` (Task 13), `TestQueryCannotChangeData`, `TestQueryStopsAStatementThatRunsTooLong`, `TestQueryStartsEachStatementInACleanSession` and `TestUseQueryPoolRefusesAUserThatCanWrite` (Task 14).

Four more conditions have tests although they did not make the list of five: a percent sign in a search (`TestSearchTakesTheQueryLiterally`, Task 11), a record that names a deleted source (`TestARecordCannotNameASourceThatDoesNotExist`, Task 7), a table from a later migration that `query` cannot read (`TestTheQueryUserCanReadATableThatIsMadeLater`, Task 13) or that the tool description does not name (`TestQueryDescriptionNamesEveryTable`, Task 14).

## File Structure

| File | Task | Responsibility |
| --- | --- | --- |
| `internal/service/secrets.go` | 1 | `secretKind`: finds a card number, an Aadhaar number, a PAN, a private key and a token |
| `internal/service/fields.go` | 2, 11 | `checkText` refuses a secret; `isText` |
| `internal/service/items.go` | 2, 7 | The title and the attrs of an item go through `checkText` |
| `migrations/00003_sources.sql` | 3 | The table `sources`, its unique index, the foreign keys of `source_ref` |
| `queries/sources.sql` | 4, 6 | The source queries |
| `internal/service/capture.go` | 4, 5 | `Capture` (the type and the function), `GetCapture`, `ListCaptures` |
| `internal/service/files.go` | 5, 8 | `Files`, `WithFiles`, the file store, the inbox |
| `internal/config/config.go`, `config/lifeos.yaml` | 5, 14 | The keys `files_dir`, `inbox_dir`, `max_file_mb`, `query_database_url` |
| `cmd/lifeos/serve.go`, `db.go` | 5, 14 | `lifeos serve` gives the service its file store and its query pool |
| `internal/service/capture_change.go` | 6, 11 | `DeleteCapture`; the summary of a source |
| `internal/service/records.go`, `review.go` | 6 | The case `source` in `Get`, `Update`, `Delete` and `Verify` |
| `internal/service/provenance.go` | 7 | `checkSourceRef` |
| `internal/api/sources.go` | 8 | `POST /api/ingest`, `POST /api/ingest/file`, `GET` and `DELETE /api/sources/{id}` |
| `internal/mcp/capture.go` | 9 | The tool `capture` |
| `internal/mcp/items.go`, `todos.go`, `habits.go`, `records.go` | 9 | The argument `source_ref`; the type `source` in the descriptions |
| `internal/admin/sources.templ`, `sources.go` | 10 | The sources page |
| `queries/search.sql`, `internal/service/search.go` | 11 | `Search` |
| `internal/api/search.go`, `internal/mcp/search.go` | 12 | `GET /api/search` and the tool `search` |
| `deploy/query-user.sh`, `Makefile` | 13 | `make query-user` makes the read-only user |
| `migrations/00004_query_user.sql` | 13 | The right to SELECT, for the tables of now and of later |
| `internal/testdb/testdb.go` | 13 | `QueryPool`: a pool of the read-only user for a test |
| `queries/query_user.sql`, `internal/service/query.go` | 14 | `UseQueryPool`, `Query` |
| `internal/mcp/query.go` | 14 | The tool `query` |
| `cmd/lifeos/capture_test.go` | 15 | A capture through every adapter over real HTTP |
| `deploy/backup.sh` | 15 | The backup includes the file store |
| `AGENTS.md`, `README.md` | 15 | The rules and the functions of this plan |

## How to read the steps

A step that changes a file uses one of these forms. Do each form exactly.

- **Create `path`:** make the new file with the content of the code block.
- **Replace the whole content of `path` with this:** the code block is the new content of a file that exists.
- **In `path`, replace this: ... With this: ...** The first code block is in the file one time. Put the second code block in its place. When a step has more than one of these for a file, do them from the first to the last.

Go code and the Makefile use tabs. Copy the code blocks with their tabs.

Put `sudo` before `docker` and before `make query-user` if the shell reports `permission denied` for Docker.

---

### Task 1: Find a secret in a text

**Files:**
- Create: `internal/service/secrets.go`
- Test: `internal/service/secrets_test.go`

**Interfaces:**
- Consumes: nothing from the service. This task is pure functions on a string.
- Produces (package `service`):
  - `secretKind(text string) string`: the kind of secret in `text`, in words for an error message, or `""`. The values are `"a card number"`, `"an Aadhaar number"`, `"a PAN"`, `"a private key"` and `"an access key or a token"`.
  - `luhnValid(digits string) bool`, `verhoeffValid(digits string) bool`, `isCardNumber(digits string) bool`, `isAadhaarNumber(digits string) bool`
  - Test values, built from parts: `awsKey`, `githubToken`, `githubPAT`, `slackToken`, `skKey`, `privateKey`. Task 2 and Task 4 use them.

- [ ] **Step 1: Check the prerequisites**

Run:
```bash
cd /home/gautam/lifeos-core
git status -sb
docker compose -f deploy/docker-compose.yml ps --format '{{.Name}}: {{.Status}}'
ls -d /var/lib/lifeos/files /var/lib/lifeos/inbox
grep -c '^LIFEOS_QUERY_PASSWORD=.' deploy/.env
make test
```
Expected: the run passes. The output contains `lifeos-postgres-1: Up` and `/var/lib/lifeos/inbox`.

The branch is `main`. No tracked file is changed, except that this plan file can be new. The line of `grep` is `1`. `make test` prints `ok` for seven packages.

- [ ] **Step 2: Write the failing tests**

The tests build each token from parts. Read "Test tokens are built from parts" in the Global Constraints before you change a test value.

Create `internal/service/secrets_test.go`:

```go
package service

import (
	"strings"
	"testing"
)

// The tests build each token from parts. A token that is written in one
// piece makes the secret scanner of GitHub refuse the push.
var (
	awsKey      = "AKIA" + "IOSFODNN7EXAMPLE"
	githubToken = "ghp" + "_" + strings.Repeat("a1B2", 9)
	githubPAT   = "github" + "_pat_" + strings.Repeat("a1B2_", 5)
	slackToken  = "xox" + "b-" + "123456789012-abcdefghij"
	skKey       = "sk" + "-" + "ant-api03-" + strings.Repeat("x9", 12)
	privateKey  = "-----BEGIN RSA " + "PRIVATE KEY-----\nMIIEow\n-----END RSA " + "PRIVATE KEY-----"
)

func TestSecretKindFindsEachKindOfSecret(t *testing.T) {
	cases := []struct{ name, text, want string }{
		{"Visa", "my card is 4111111111111111", "a card number"},
		{"Visa with 13 digits", "4222222222222", "a card number"},
		{"Visa with 19 digits", "4111111111111111110", "a card number"},
		{"Mastercard", "5555555555554444", "a card number"},
		{"Mastercard that starts with 2", "2223003122003222", "a card number"},
		{"American Express", "378282246310005", "a card number"},
		{"Discover", "6011111111111117", "a card number"},
		{"JCB", "3530111333300000", "a card number"},
		{"Diners Club", "30569309025904", "a card number"},
		{"RuPay", "6521500000000006", "a card number"},
		{"RuPay that starts with 508", "5085000000000007", "a card number"},
		{"groups of four with spaces", "pay with 4111 1111 1111 1111 today", "a card number"},
		{"groups of four with hyphens", "4111-1111-1111-1111", "a card number"},
		{"the groups of American Express", "3782 822463 10005", "a card number"},
		{"the groups of Diners Club", "3056 930902 5904", "a card number"},
		{"a card after a date", "2026-10-11 4111 1111 1111 1111", "a card number"},
		{"a card between letters", "card:4111111111111111.", "a card number"},
		{"a card on its own line", "notes\n5555 5555 5555 4444\nend", "a card number"},

		{"Aadhaar", "234567890124", "an Aadhaar number"},
		{"Aadhaar in groups", "my Aadhaar is 2345 6789 0124", "an Aadhaar number"},
		{"Aadhaar with hyphens", "2345-6789-0124", "an Aadhaar number"},
		{"Aadhaar after a date", "2026-10-11 2345 6789 0124", "an Aadhaar number"},

		{"PAN of a person", "PAN ABCPE1234F", "a PAN"},
		{"PAN of a company", "AAACR5055K", "a PAN"},

		{"private key", privateKey, "a private key"},
		{"AWS access key", "key " + awsKey, "an access key or a token"},
		{"GitHub token", githubToken, "an access key or a token"},
		{"GitHub fine-grained token", githubPAT, "an access key or a token"},
		{"Slack token", slackToken, "an access key or a token"},
		{"API key that starts with sk-", "export KEY=" + skKey, "an access key or a token"},
	}
	for _, c := range cases {
		if got := secretKind(c.text); got != c.want {
			t.Errorf("%s: secretKind = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSecretKindAcceptsTextWithoutASecret(t *testing.T) {
	cases := []struct{ name, text string }{
		{"empty", ""},
		{"plain words", "spent the entire day planning a second brain"},
		{"a card number with a wrong check digit", "4111111111111112"},
		{"16 digits that no card network uses", "1234567890123452"},
		{"20 digits", "41111111111111111111"},
		{"five groups of four", "4111 1111 1111 1111 1111"},
		{"a phone number", "call 9876543210"},
		{"a phone number with a country code", "+91 98765 43210"},
		{"12 digits with a wrong check digit", "234567890123"},
		{"12 digits that start with 1", "123456789012"},
		{"three of the four groups of a longer number", "9999 2345 6789 0124"},
		{"a date and a time", "2026-10-11 07:30"},
		{"an ISBN", "9780441172719"},
		{"a timestamp in milliseconds", "1791676800000"},
		{"an amount", "paid 12,50,000 rupees on 2026-10-11"},
		{"ten capital letters and digits that are not a PAN", "ABCDE1234F"},
		{"a PAN inside a longer word", "XABCPE1234FX"},
		{"a word that starts with sk-", "sk-learn is a library"},
		{"the words private key", "I lost my private key yesterday"},
		{"a short token prefix", "ghp_short"},
	}
	for _, c := range cases {
		if got := secretKind(c.text); got != "" {
			t.Errorf("%s: secretKind = %q, want no secret", c.name, got)
		}
	}
}

func TestLuhnAndVerhoeffCheckTheLastDigit(t *testing.T) {
	if !luhnValid("4111111111111111") || luhnValid("4111111111111112") {
		t.Error("luhnValid gives the wrong answer for the Visa test number")
	}
	if !verhoeffValid("234567890124") || verhoeffValid("234567890123") {
		t.Error("verhoeffValid gives the wrong answer")
	}
	// Verhoeff, not Luhn, finds two neighbours that changed places.
	if verhoeffValid("234567890214") {
		t.Error("verhoeffValid accepts two digits that changed places")
	}
}
```

- [ ] **Step 3: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run 'Secret|Luhn'"
```
Expected: the run fails. The output contains `undefined: secretKind`.

- [ ] **Step 4: Write the code**

Create `internal/service/secrets.go`:

```go
package service

import (
	"regexp"
	"strconv"
	"strings"
)

// LifeOS never stores a card number, a government ID number or a secret
// (AGENTS.md rule 7). The functions in this file find the ones that have a
// pattern. A password in plain words and a bank account number have no
// pattern, so no code can find them.

var (
	privateKeyPattern = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)
	tokenPatterns     = []*regexp.Regexp{
		regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`),  // AWS access key ID
		regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}`),   // GitHub token
		regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,}`), // GitHub fine-grained token
		regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}`), // Slack token
		regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}`),        // API key of an AI service
	}
	// The fourth letter of a PAN says who holds it: P is a person, C a
	// company, and so on.
	panPattern = regexp.MustCompile(`\b[A-Z]{3}[ABCFGHJLPT][A-Z][0-9]{4}[A-Z]\b`)
	// A run is groups of digits with one space or one hyphen between them.
	digitRunPattern = regexp.MustCompile(`[0-9]+(?:[ -][0-9]+)*`)
	groupSeparator  = regexp.MustCompile(`[ -]`)
)

// The ways that a card number and an Aadhaar number are written in groups.
var (
	cardGroupings = [][]int{
		{4, 4, 4, 4},    // most cards
		{4, 4, 4, 4, 3}, // a card with 19 digits
		{4, 6, 5},       // American Express
		{4, 6, 4},       // Diners Club
	}
	aadhaarGrouping = []int{4, 4, 4}
)

// secretKind returns what kind of secret text holds, in words for an error
// message, or "" when it finds none.
func secretKind(text string) string {
	if privateKeyPattern.MatchString(text) {
		return "a private key"
	}
	for _, pattern := range tokenPatterns {
		if pattern.MatchString(text) {
			return "an access key or a token"
		}
	}
	for _, run := range digitRunPattern.FindAllString(text, -1) {
		if kind := numberKind(groupSeparator.Split(run, -1)); kind != "" {
			return kind
		}
	}
	if panPattern.MatchString(text) {
		return "a PAN"
	}
	return ""
}

// numberKind looks at one run of digit groups. A number is one group, or
// neighbouring groups in one of the known ways to write it.
func numberKind(groups []string) string {
	for i, group := range groups {
		if isCardNumber(group) {
			return "a card number"
		}
		if isAadhaarNumber(group) {
			return "an Aadhaar number"
		}
		for _, grouping := range cardGroupings {
			if number, ok := grouped(groups, i, grouping); ok && isCardNumber(number) {
				return "a card number"
			}
		}
		if number, ok := grouped(groups, i, aadhaarGrouping); ok && isAadhaarNumber(number) {
			return "an Aadhaar number"
		}
	}
	return ""
}

// grouped returns the digits of the groups that start at index first, when
// their lengths are the lengths in grouping. A neighbour of four digits
// means that the groups are a part of a longer number, so that is no match.
func grouped(groups []string, first int, grouping []int) (string, bool) {
	last := first + len(grouping) - 1
	if last >= len(groups) {
		return "", false
	}
	for i, length := range grouping {
		if len(groups[first+i]) != length {
			return "", false
		}
	}
	if first > 0 && len(groups[first-1]) == 4 {
		return "", false
	}
	if last+1 < len(groups) && len(groups[last+1]) == 4 {
		return "", false
	}
	return strings.Join(groups[first:last+1], ""), true
}

// isCardNumber reports whether digits has the length of a card number, the
// first digits of a card network and a correct Luhn check digit.
func isCardNumber(digits string) bool {
	if len(digits) < 13 || len(digits) > 19 {
		return false
	}
	two, _ := strconv.Atoi(digits[:2])
	four, _ := strconv.Atoi(digits[:4])
	network := digits[0] == '4' || // Visa
		digits[0] == '6' || // Discover, RuPay, Maestro, UnionPay
		(two >= 50 && two <= 58) || // Mastercard, Maestro, RuPay
		(four >= 2221 && four <= 2720) || // Mastercard
		two == 30 || (two >= 34 && two <= 39) || // Diners Club, American Express, JCB
		two == 81 || two == 82 // RuPay
	return network && luhnValid(digits)
}

// isAadhaarNumber reports whether digits is 12 digits that do not start
// with 0 or 1 and that have a correct Verhoeff check digit.
func isAadhaarNumber(digits string) bool {
	return len(digits) == 12 && digits[0] >= '2' && verhoeffValid(digits)
}

// luhnValid reports whether the last digit is the Luhn check digit of the
// digits before it.
func luhnValid(digits string) bool {
	sum := 0
	for i := 0; i < len(digits); i++ {
		n := int(digits[len(digits)-1-i] - '0')
		if i%2 == 1 {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
	}
	return sum%10 == 0
}

// The tables of the Verhoeff algorithm: the multiplication in the group
// D5, and the permutation that depends on the position of a digit.
var (
	verhoeffMultiply = [10][10]int{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{1, 2, 3, 4, 0, 6, 7, 8, 9, 5},
		{2, 3, 4, 0, 1, 7, 8, 9, 5, 6},
		{3, 4, 0, 1, 2, 8, 9, 5, 6, 7},
		{4, 0, 1, 2, 3, 9, 5, 6, 7, 8},
		{5, 9, 8, 7, 6, 0, 4, 3, 2, 1},
		{6, 5, 9, 8, 7, 1, 0, 4, 3, 2},
		{7, 6, 5, 9, 8, 2, 1, 0, 4, 3},
		{8, 7, 6, 5, 9, 3, 2, 1, 0, 4},
		{9, 8, 7, 6, 5, 4, 3, 2, 1, 0},
	}
	verhoeffPermute = [8][10]int{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{1, 5, 7, 6, 2, 8, 3, 0, 9, 4},
		{5, 8, 0, 3, 7, 9, 6, 1, 4, 2},
		{8, 9, 1, 6, 0, 4, 3, 5, 2, 7},
		{9, 4, 5, 3, 1, 2, 6, 8, 7, 0},
		{4, 2, 8, 6, 5, 7, 3, 9, 0, 1},
		{2, 7, 9, 3, 8, 0, 6, 4, 1, 5},
		{7, 0, 4, 6, 9, 1, 3, 2, 5, 8},
	}
)

// verhoeffValid reports whether the last digit is the Verhoeff check digit
// of the digits before it. Aadhaar uses this algorithm.
func verhoeffValid(digits string) bool {
	check := 0
	for i := 0; i < len(digits); i++ {
		n := int(digits[len(digits)-1-i] - '0')
		check = verhoeffMultiply[check][verhoeffPermute[i%8][n]]
	}
	return check == 0
}
```

- [ ] **Step 5: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./internal/service ARGS="-run 'Secret|Luhn' -v"
```
Expected: the run passes. The output contains `PASS: TestSecretKindFindsEachKindOfSecret`, `PASS: TestSecretKindAcceptsTextWithoutASecret` and `PASS: TestLuhnAndVerhoeffCheckTheLastDigit`.

- [ ] **Step 6: Commit**

Run:
```bash
git add internal/service/secrets.go internal/service/secrets_test.go
git commit -m "feat(service): find card numbers, ID numbers and secrets in text"
```

---

### Task 2: Refuse a secret in each record

**Files:**
- Modify: `internal/service/fields.go` (`checkText`), `internal/service/items.go` (the title and the attrs)
- Test: `internal/service/secrets_records_test.go`

**Interfaces:**
- Consumes (Task 1): `secretKind(text string) string`; the test values `awsKey`, `githubToken`, `slackToken`, `privateKey`.
- Consumes (plans 1 to 3, package `service`): `checkText(field, value string) error`, which the journal body, the title and the project of a todo, and a habit name go through already; `AddItemParams.toRow`, which `AddItem` and `UpdateItem` use; the test helpers `newTestService`, `dune()`, `buyDomain()`, `addItem`, `addJournal`, `addTodo`, `addHabit`, `ptr`, `countRows`.
- Produces (package `service`):
  - `checkText(field, value string) error` now also refuses a text that holds a secret. The message is `must not hold <kind>: LifeOS does not store one. Take it out, then send the text again`.
  - `checkAttr(value any) error`: checks each name, each text and each number of an attr value, at any depth. The field of its error is `attrs`.
  - An item title and the attrs of an item go through `checkText`. So a NUL character in them is a `ValidationError` now.
  - Test helpers: the constant `testCard` (`4111 1111 1111 1111`), `wantSecretRefused(t, name, err, field, kind, secret)`.

- [ ] **Step 1: Write the failing tests**

Create `internal/service/secrets_records_test.go`:

```go
package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const testCard = "4111 1111 1111 1111"

// wantSecretRefused checks that err is a ValidationError for field that
// names the kind of secret and does not repeat the secret.
func wantSecretRefused(t *testing.T, name string, err error, field, kind, secret string) {
	t.Helper()
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Errorf("%s: error = %v, want a ValidationError", name, err)
		return
	}
	if invalid.Field != field {
		t.Errorf("%s: field = %q, want %q", name, invalid.Field, field)
	}
	if !strings.Contains(invalid.Message, "must not hold "+kind) {
		t.Errorf("%s: message = %q, want it to name %s", name, invalid.Message, kind)
	}
	if strings.Contains(invalid.Error(), secret) {
		t.Errorf("%s: the error repeats the secret: %q", name, invalid.Error())
	}
}

func TestAddItemRefusesASecret(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	cases := []struct {
		name        string
		change      func(*AddItemParams)
		field, kind string
		secret      string
	}{
		{"a card number in the title", func(p *AddItemParams) { p.Title = "card " + testCard }, "title", "a card number", testCard},
		{"a token in an attr", func(p *AddItemParams) { p.Attrs = map[string]any{"note": awsKey} }, "attrs", "an access key or a token", awsKey},
		{"a card number as a number in an attr", func(p *AddItemParams) { p.Attrs = map[string]any{"card": float64(4111111111111111)} }, "attrs", "a card number", "4111111111111111"},
		{"a PAN deep in the attrs", func(p *AddItemParams) {
			p.Attrs = map[string]any{"people": []any{map[string]any{"pan": "ABCPE1234F"}}}
		}, "attrs", "a PAN", "ABCPE1234F"},
		{"an Aadhaar number in the name of an attr", func(p *AddItemParams) { p.Attrs = map[string]any{"2345 6789 0124": true} }, "attrs", "an Aadhaar number", "2345 6789 0124"},
	}
	for _, c := range cases {
		p := dune()
		c.change(&p)
		_, err := s.AddItem(ctx, p)
		wantSecretRefused(t, c.name, err, c.field, c.kind, c.secret)
	}
	if n := countRows(t, pool, "catalog_items") + countRows(t, pool, "events"); n != 0 {
		t.Errorf("rows in catalog_items and events = %d, want 0", n)
	}
}

func TestAddItemRefusesTextThatPostgresCannotStore(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	for name, change := range map[string]func(*AddItemParams){
		"a NUL in the title":        func(p *AddItemParams) { p.Title = "Dune\x00" },
		"a NUL in an attr":          func(p *AddItemParams) { p.Attrs = map[string]any{"director": "Denis\x00"} },
		"a title that is not UTF-8": func(p *AddItemParams) { p.Title = "Dune\xff" },
	} {
		p := dune()
		change(&p)
		_, err := s.AddItem(ctx, p)
		var invalid *ValidationError
		if !errors.As(err, &invalid) {
			t.Errorf("%s: error = %v, want a ValidationError", name, err)
		}
	}
	if n := countRows(t, pool, "catalog_items"); n != 0 {
		t.Errorf("rows in catalog_items = %d, want 0", n)
	}
}

func TestUpdateItemRefusesASecret(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	item := addItem(t, s, "movie", "Dune")

	_, err := s.UpdateItem(ctx, item.ID, UpdateItemParams{Title: ptr("Dune " + testCard), Actor: SourceManual})
	wantSecretRefused(t, "title", err, "title", "a card number", testCard)
	_, err = s.UpdateItem(ctx, item.ID, UpdateItemParams{Attrs: map[string]any{"key": githubToken}, Actor: SourceManual})
	wantSecretRefused(t, "attrs", err, "attrs", "an access key or a token", githubToken)

	got, err := s.GetItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if got.Title != "Dune" || len(got.Attrs) != 0 {
		t.Errorf("item = %+v, want it as it was", got)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want only the create event", n)
	}
}

func TestJournalRefusesASecret(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	_, err := s.AddJournalEntry(ctx, AddJournalEntryParams{
		Body:       "new card today:\n" + testCard + "\nexpires in 2030",
		Provenance: Provenance{Source: SourceManual, Verified: true},
	})
	wantSecretRefused(t, "add", err, "body", "a card number", testCard)
	if n := countRows(t, pool, "journal_entries"); n != 0 {
		t.Errorf("journal entries = %d, want 0", n)
	}

	entry := addJournal(t, s, "a quiet day", "")
	_, err = s.UpdateJournalEntry(ctx, entry.ID, UpdateJournalEntryParams{Body: ptr("the key is " + privateKey), Actor: SourceManual})
	wantSecretRefused(t, "update", err, "body", "a private key", "MIIEow")
	if got, _ := s.GetJournalEntry(ctx, entry.ID); got.Body != "a quiet day" {
		t.Errorf("body = %q, want it as it was", got.Body)
	}
}

func TestTodoRefusesASecret(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	p := buyDomain()
	p.Title = "pay with " + testCard
	_, err := s.AddTodo(ctx, p)
	wantSecretRefused(t, "title", err, "title", "a card number", testCard)

	p = buyDomain()
	p.Project = slackToken
	_, err = s.AddTodo(ctx, p)
	wantSecretRefused(t, "project", err, "project", "an access key or a token", slackToken)
	if n := countRows(t, pool, "todos"); n != 0 {
		t.Errorf("todos = %d, want 0", n)
	}

	todo := addTodo(t, s, "Buy a domain")
	_, err = s.UpdateTodo(ctx, todo.ID, UpdateTodoParams{Title: ptr("Aadhaar 2345 6789 0124"), Actor: SourceManual})
	wantSecretRefused(t, "update", err, "title", "an Aadhaar number", "2345 6789 0124")
}

func TestHabitNameRefusesASecret(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	_, err := s.AddHabit(ctx, AddHabitParams{Name: testCard, Provenance: Provenance{Source: SourceManual, Verified: true}})
	wantSecretRefused(t, "add", err, "name", "a card number", testCard)
	if n := countRows(t, pool, "habits"); n != 0 {
		t.Errorf("habits = %d, want 0", n)
	}

	habit := addHabit(t, s, "gym")
	_, err = s.UpdateHabit(ctx, habit.ID, UpdateHabitParams{Name: ptr(testCard), Actor: SourceManual})
	wantSecretRefused(t, "rename", err, "name", "a card number", testCard)
}

func TestTextWithoutASecretIsStored(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	// A phone number, a date, an amount and an ISBN are not secrets.
	body := "called +91 98765 43210 on 2026-10-11, paid 12,50,000, ISBN 9780441172719"
	entry, err := s.AddJournalEntry(ctx, AddJournalEntryParams{Body: body, Provenance: Provenance{Source: SourceManual, Verified: true}})
	if err != nil {
		t.Fatalf("AddJournalEntry: %v", err)
	}
	if entry.Body != body {
		t.Errorf("body = %q, want it as written", entry.Body)
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run 'Secret|PostgresCannotStore|WithoutASecret'"
```
Expected: the run fails. The output contains `a card number in the title: error = <nil>, want a ValidationError` and `TestJournalRefusesASecret`.

- [ ] **Step 3: Add the check to `checkText`**

`checkText` is the one function that each text field goes through. The check goes there, so a new text field gets it when it calls `checkText`.

In `internal/service/fields.go`, replace this:

```go
// checkText refuses text that Postgres cannot store.
func checkText(field, value string) error {
```

With this:

```go
// checkText refuses text that Postgres cannot store, and text that holds a
// secret (AGENTS.md rule 7). The message names the kind of secret and does
// not repeat it. Each text that a caller gives goes through this check
// before the service stores it.
func checkText(field, value string) error {
```

In `internal/service/fields.go`, replace this:

```go
		return invalid(field, "must be valid text without a NUL character")
	}
```

With this:

```go
		return invalid(field, "must be valid text without a NUL character")
	}
	if kind := secretKind(value); kind != "" {
		return invalid(field, "must not hold "+kind+": LifeOS does not store one. Take it out, then send the text again")
	}
```

- [ ] **Step 4: Send the title and the attrs of an item through the check**

An item did not call `checkText` until now. The attrs can hold a card number as a JSON number, so `checkAttr` checks numbers too.

In `internal/service/items.go`, replace this:

```go
	"math"
	"strings"
```

With this:

```go
	"math"
	"strconv"
	"strings"
```

In `internal/service/items.go`, replace this:

```go
	}
	if p.Rating != nil {
```

With this:

```go
	}
	if err := checkText("title", title); err != nil {
		return row, err
	}
	if p.Rating != nil {
```

In `internal/service/items.go`, replace this:

```go
	}
	attrsJSON, err := json.Marshal(attrs)
```

With this:

```go
	}
	if err := checkAttr(attrs); err != nil {
		return row, err
	}
	attrsJSON, err := json.Marshal(attrs)
```

In `internal/service/items.go`, replace this:

```go
	}, nil
}
```

With this:

```go
	}, nil
}

// checkAttr checks each name, each text and each number in an attr value,
// at any depth. A card number can come as a JSON number.
func checkAttr(value any) error {
	switch v := value.(type) {
	case string:
		return checkText("attrs", v)
	case float64:
		return checkText("attrs", strconv.FormatFloat(v, 'f', -1, 64))
	case []any:
		for _, inner := range v {
			if err := checkAttr(inner); err != nil {
				return err
			}
		}
	case map[string]any:
		for name, inner := range v {
			if err := checkText("attrs", name); err != nil {
				return err
			}
			if err := checkAttr(inner); err != nil {
				return err
			}
		}
	}
	return nil
}
```

- [ ] **Step 5: Run the tests of the service to verify that they pass**

Run:
```bash
make test PKG=./internal/service
```
Expected: the run passes. The last line starts with `ok`.

- [ ] **Step 6: Commit**

Run:
```bash
git add internal/service/fields.go internal/service/items.go internal/service/secrets_records_test.go
git commit -m "feat(service): refuse a secret in each text that a caller gives"
```

---

### Task 3: Migration: the table `sources`

**Files:**
- Create: `migrations/00003_sources.sql`
- Modify: `migrations/migrations_test.go`

**Interfaces:**
- Consumes (plans 1 and 3): `testdb.New(t) *pgxpool.Pool`, which gives a new database with every migration applied; the function `set_updated_at()`; the column `source_ref bigint` on `catalog_items`, `journal_entries`, `todos`, `habits` and `habit_logs`.
- Produces:
  - The table `sources` with the columns `id`, `kind`, `title`, `content`, `url`, `file_path`, `media_type`, `size_bytes`, `sha256`, `processed_at`, the provenance columns and `deleted_at`; the trigger `sources_set_updated_at`; the constraint `sources_kind_fields`.
  - The unique index `sources_kind_sha256_idx` on `sources (kind, sha256)`, which counts only rows where `deleted_at IS NULL`.
  - One foreign key `<table>_source_ref_fkey` to `sources (id)` on each of the six tables.

- [ ] **Step 1: Write the failing tests**

In `migrations/migrations_test.go`, replace this:

```go
func TestUpCreatesTheTables(t *testing.T) {
	pool := testdb.New(t) // testdb.New already ran migrations.Up once
	ctx := context.Background()

	for _, table := range []string{"events", "catalog_items", "journal_entries", "todos", "habits", "habit_logs"} {
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = $1)",
			table).Scan(&exists)
```

With this:

```go
func TestUpCreatesTheTables(t *testing.T) {
	pool := testdb.New(t) // testdb.New already ran migrations.Up once
	ctx := context.Background()

	for _, table := range []string{"events", "catalog_items", "journal_entries", "todos", "habits", "habit_logs", "sources"} {
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = $1)",
			table).Scan(&exists)
```

In `migrations/migrations_test.go`, replace this:

```go
func TestRecordTablesHaveTheProvenanceColumns(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	for _, table := range []string{"catalog_items", "journal_entries", "todos", "habits", "habit_logs"} {
		for _, column := range []string{"source", "source_ref", "verified", "created_at", "updated_at", "deleted_at"} {
			var exists bool
			err := pool.QueryRow(ctx,
				"SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2)",
```

With this:

```go
func TestRecordTablesHaveTheProvenanceColumns(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	for _, table := range []string{"catalog_items", "journal_entries", "todos", "habits", "habit_logs", "sources"} {
		for _, column := range []string{"source", "source_ref", "verified", "created_at", "updated_at", "deleted_at"} {
			var exists bool
			err := pool.QueryRow(ctx,
				"SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2)",
```

In `migrations/migrations_test.go`, replace this:

```go
		t.Errorf("second Up applied %v, want nothing", applied)
	}
}
```

With this:

```go
		t.Errorf("second Up applied %v, want nothing", applied)
	}
}

// The database keeps one live capture for each kind and content. A deleted
// capture does not count, and the same hash can be a text and a link.
func TestSourcesRefuseASecondLiveCaptureOfTheSameContent(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	const addText = "INSERT INTO sources (kind, content, sha256, source) VALUES ('text', 'a note', 'abc', 'manual') RETURNING id"

	var first int64
	if err := pool.QueryRow(ctx, addText).Scan(&first); err != nil {
		t.Fatalf("first capture: %v", err)
	}
	if _, err := pool.Exec(ctx, addText); err == nil {
		t.Error("a second live capture of the same text was stored")
	}
	if _, err := pool.Exec(ctx, "INSERT INTO sources (kind, url, sha256, source) VALUES ('link', 'https://example.com', 'abc', 'manual')"); err != nil {
		t.Errorf("a link with the hash of a text: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE sources SET deleted_at = now() WHERE id = $1", first); err != nil {
		t.Fatalf("delete the first capture: %v", err)
	}
	if _, err := pool.Exec(ctx, addText); err != nil {
		t.Errorf("a capture of the text of a deleted capture: %v", err)
	}
}

// A row holds what its kind needs: a text has content, a link has a URL, a
// file has a path.
func TestSourcesRefuseARowThatDoesNotFitItsKind(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	for name, insert := range map[string]string{
		"an unknown kind":       "INSERT INTO sources (kind, content, sha256, source) VALUES ('video', 'x', 'a', 'manual')",
		"a text with no text":   "INSERT INTO sources (kind, sha256, source) VALUES ('text', 'b', 'manual')",
		"a text with a URL":     "INSERT INTO sources (kind, content, url, sha256, source) VALUES ('text', 'x', 'https://example.com', 'c', 'manual')",
		"a link with no URL":    "INSERT INTO sources (kind, sha256, source) VALUES ('link', 'd', 'manual')",
		"a file with no path":   "INSERT INTO sources (kind, sha256, source) VALUES ('file', 'e', 'manual')",
		"a file with a URL":     "INSERT INTO sources (kind, file_path, url, sha256, source) VALUES ('file', 'ab/abc', 'https://example.com', 'f', 'manual')",
		"a row with no hash":    "INSERT INTO sources (kind, content, source) VALUES ('text', 'x', 'manual')",
		"an unknown provenance": "INSERT INTO sources (kind, content, sha256, source) VALUES ('text', 'x', 'g', 'robot')",
	} {
		if _, err := pool.Exec(ctx, insert); err == nil {
			t.Errorf("%s was stored", name)
		}
	}
	for name, insert := range map[string]string{
		"a text": "INSERT INTO sources (kind, content, sha256, source) VALUES ('text', 'x', 'h', 'manual')",
		"a link": "INSERT INTO sources (kind, url, sha256, source) VALUES ('link', 'https://example.com', 'i', 'manual')",
		"a file": "INSERT INTO sources (kind, title, file_path, media_type, size_bytes, sha256, source) VALUES ('file', 'policy.pdf', 'ab/abc', 'application/pdf', 3, 'j', 'manual')",
	} {
		if _, err := pool.Exec(ctx, insert); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// source_ref on each record table names a row of sources.
func TestSourceRefMustNameASource(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	var source int64
	err := pool.QueryRow(ctx, "INSERT INTO sources (kind, content, sha256, source) VALUES ('text', 'a note', 'abc', 'manual') RETURNING id").Scan(&source)
	if err != nil {
		t.Fatalf("add a source: %v", err)
	}
	// The habit of the habit log. It gets the ID 1.
	if _, err := pool.Exec(ctx, "INSERT INTO habits (name, source) VALUES ('gym', 'manual')"); err != nil {
		t.Fatalf("add a habit: %v", err)
	}

	inserts := map[string]string{
		"catalog_items":   "INSERT INTO catalog_items (kind, title, source, source_ref) VALUES ('book', 'Dune', 'agent', $1)",
		"journal_entries": "INSERT INTO journal_entries (body, entry_date, source, source_ref) VALUES ('a day', '2026-10-11', 'agent', $1)",
		"todos":           "INSERT INTO todos (title, source, source_ref) VALUES ('Buy a domain', 'agent', $1)",
		"habits":          "INSERT INTO habits (name, source, source_ref) VALUES ('read', 'agent', $1)",
		"habit_logs":      "INSERT INTO habit_logs (habit_id, logged_on, source, source_ref) VALUES (1, '2026-10-11', 'agent', $1)",
		"sources":         "INSERT INTO sources (kind, content, sha256, source, source_ref) VALUES ('text', 'a part', 'def', 'agent', $1)",
	}
	for table, insert := range inserts {
		if _, err := pool.Exec(ctx, insert, source+1000); err == nil {
			t.Errorf("%s: a row with a source_ref that names no source was stored", table)
		}
		if _, err := pool.Exec(ctx, insert, source); err != nil {
			t.Errorf("%s: a row with the source_ref of a source: %v", table, err)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./migrations
```
Expected: the run fails. The output contains `table sources does not exist after Up`.

- [ ] **Step 3: Write the migration**

Create `migrations/00003_sources.sql`:

```sql
-- +goose Up

-- Verbatim captures: a text, a link or a file. The service never changes a
-- capture. A record that an agent takes from a capture names the row in
-- its source_ref.
CREATE TABLE sources (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind         text NOT NULL CHECK (kind IN ('text', 'link', 'file')),
    title        text,                -- optional label; the file name of a file
    content      text,                -- text: the text exactly as it came
    url          text,                -- link: the URL as it came
    file_path    text,                -- file: the path below the file store, made from the SHA-256
    media_type   text,                -- file: the type that the service found in the bytes
    size_bytes   bigint,              -- file: the number of bytes
    sha256       text NOT NULL,       -- hex SHA-256 of the text, the URL or the file bytes
    processed_at timestamptz,         -- null until the inbox job reads the capture

    source       text NOT NULL CHECK (source IN ('manual', 'import', 'connector', 'agent')),
    source_ref   bigint,
    verified     boolean NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz,

    -- A later plan can fill content for a link or a file, so only a text
    -- must have it.
    CONSTRAINT sources_kind_fields CHECK (
        (kind = 'text' AND content IS NOT NULL AND url IS NULL AND file_path IS NULL) OR
        (kind = 'link' AND url IS NOT NULL AND file_path IS NULL) OR
        (kind = 'file' AND file_path IS NOT NULL AND url IS NULL)
    )
);

CREATE TRIGGER sources_set_updated_at
    BEFORE UPDATE ON sources
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- One live capture for each kind and content. The same thing captured two
-- times gives the row that exists.
CREATE UNIQUE INDEX sources_kind_sha256_idx ON sources (kind, sha256) WHERE deleted_at IS NULL;

-- source_ref was a plain number until now, because sources did not exist.
ALTER TABLE catalog_items   ADD CONSTRAINT catalog_items_source_ref_fkey   FOREIGN KEY (source_ref) REFERENCES sources (id);
ALTER TABLE journal_entries ADD CONSTRAINT journal_entries_source_ref_fkey FOREIGN KEY (source_ref) REFERENCES sources (id);
ALTER TABLE todos           ADD CONSTRAINT todos_source_ref_fkey           FOREIGN KEY (source_ref) REFERENCES sources (id);
ALTER TABLE habits          ADD CONSTRAINT habits_source_ref_fkey          FOREIGN KEY (source_ref) REFERENCES sources (id);
ALTER TABLE habit_logs      ADD CONSTRAINT habit_logs_source_ref_fkey      FOREIGN KEY (source_ref) REFERENCES sources (id);
ALTER TABLE sources         ADD CONSTRAINT sources_source_ref_fkey         FOREIGN KEY (source_ref) REFERENCES sources (id);

-- +goose Down
ALTER TABLE habit_logs      DROP CONSTRAINT habit_logs_source_ref_fkey;
ALTER TABLE habits          DROP CONSTRAINT habits_source_ref_fkey;
ALTER TABLE todos           DROP CONSTRAINT todos_source_ref_fkey;
ALTER TABLE journal_entries DROP CONSTRAINT journal_entries_source_ref_fkey;
ALTER TABLE catalog_items   DROP CONSTRAINT catalog_items_source_ref_fkey;
DROP TABLE sources;
```

- [ ] **Step 4: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./migrations
```
Expected: the run passes. The last line starts with `ok`.

- [ ] **Step 5: Commit**

Run:
```bash
git add migrations/00003_sources.sql migrations/migrations_test.go
git commit -m "feat(db): add the sources table and the source_ref foreign keys"
```

---

### Task 4: Service: capture a text or a link

**Files:**
- Create: `queries/sources.sql`, `internal/service/capture.go`
- Test: `internal/service/capture_test.go`

**Interfaces:**
- Consumes (Task 3): the table `sources` and its unique index on `(kind, sha256)`.
- Consumes (Tasks 1 and 2): `checkText`, which refuses a secret; the test values `testCard` and `awsKey`.
- Consumes (plans 1 to 3, package `service`): `Service` with the field `pool`, `Source` and its values, `Provenance` and its method `validate() error`, `invalid(field, message string) error`, `ErrNotFound`, `recordEvent(ctx, q, action, recordType string, recordID int64, source Source, oldValue, newValue any) error`, `checkLimit(limit int) (int32, error)`, `maxTitleLength` (300). Test helpers: `newTestService`, `countRows`, `allEvents`, `equal`.
- Produces (package `service`):
  - `type Capture struct{ ID int64; Kind, Title, Content, URL, FilePath, MediaType string; SizeBytes int64; SHA256 string; ProcessedAt *time.Time; Source Source; SourceRef *int64; Verified bool; CreatedAt, UpdatedAt time.Time }` with the JSON names `id`, `kind`, `title`, `content`, `url`, `file_path`, `media_type`, `size_bytes`, `sha256`, `processed_at`, `source`, `source_ref`, `verified`, `created_at`, `updated_at`. Each optional field is left out of the JSON when it is empty.
  - `type CaptureParams struct{ Text, URL, Title string; Source Source }`. Task 5 adds three fields.
  - `(*Service).Capture(ctx, CaptureParams) (Capture, error)`, `(*Service).GetCapture(ctx, id int64) (Capture, error)`, `(*Service).ListCaptures(ctx, limit int) ([]Capture, error)`
  - `(*Service).storeSource(ctx, store.InsertSourceParams) (Capture, error)`: inserts the row with its create event, or returns the live source that has the same kind and hash. Task 5 calls it for a file.
  - `captureTitle(title string) (*string, error)`, `checkLink(link string) error`, `hashHex(data []byte) string`, `captureFromRow(store.Source) Capture`
  - The constants `maxCaptureTextBytes` (1 MB) and `maxURLLength` (2000)
  - Test helpers: `captureText(t, s, text string) Capture`, `hashOf(text string) string`
  - sqlc: `InsertSource` (no row when a live source has the kind and the hash), `GetSourceByHash`, `GetSource`, `ListSources`; the type `store.Source`

- [ ] **Step 1: Write the failing tests**

Create `internal/service/capture_test.go`:

```go
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// captureText stores one text as the REST API does.
func captureText(t *testing.T, s *Service, text string) Capture {
	t.Helper()
	capture, err := s.Capture(context.Background(), CaptureParams{Text: text, Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture(%q): %v", text, err)
	}
	return capture
}

func hashOf(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// A capture is verbatim. The service must not trim or change the text.
func TestCaptureStoresATextExactlyAsItCame(t *testing.T) {
	s, pool := newTestService(t)
	const text = "  Renewal on 12 March.\n\nCall the agent first.  \n"

	got, err := s.Capture(context.Background(), CaptureParams{Text: text, Title: "  Insurance note ", Source: SourceAgent})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	if got.ID == 0 {
		t.Error("Capture returned ID 0")
	}
	if got.Kind != "text" || got.Content != text || got.Title != "Insurance note" {
		t.Errorf("kind, content, title = %q, %q, %q", got.Kind, got.Content, got.Title)
	}
	if got.SHA256 != hashOf(text) {
		t.Errorf("sha256 = %q, want the hash of the text as it came", got.SHA256)
	}
	if got.URL != "" || got.FilePath != "" || got.ProcessedAt != nil {
		t.Errorf("url, file_path, processed_at = %q, %q, %v, want them empty", got.URL, got.FilePath, got.ProcessedAt)
	}
	// A capture is what came in, not a fact that an agent inferred.
	if got.Source != SourceAgent || !got.Verified {
		t.Errorf("source, verified = %q, %v, want agent, true", got.Source, got.Verified)
	}
	if n := countRows(t, pool, "sources"); n != 1 {
		t.Errorf("sources = %d, want 1", n)
	}
	stored, err := s.GetCapture(context.Background(), got.ID)
	if err != nil {
		t.Fatalf("GetCapture: %v", err)
	}
	if stored.Content != text {
		t.Errorf("stored content = %q, want %q", stored.Content, text)
	}
}

func TestCaptureStoresALink(t *testing.T) {
	s, _ := newTestService(t)
	const link = "https://example.com/a?b=c#d"

	got, err := s.Capture(context.Background(), CaptureParams{URL: "  " + link + "\n", Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if got.Kind != "link" || got.URL != link || got.Content != "" {
		t.Errorf("kind, url, content = %q, %q, %q", got.Kind, got.URL, got.Content)
	}
	if got.SHA256 != hashOf(link) {
		t.Errorf("sha256 = %q, want the hash of the URL", got.SHA256)
	}
	if got.Source != SourceManual || !got.Verified {
		t.Errorf("source, verified = %q, %v, want manual, true", got.Source, got.Verified)
	}
}

func TestCaptureAppendsACreateEvent(t *testing.T) {
	s, pool := newTestService(t)
	capture := captureText(t, s, "a note")

	events := allEvents(t, pool)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Action != "create" || e.RecordType != "sources" || e.RecordID != capture.ID || e.Source != "manual" {
		t.Errorf("event = %+v", e)
	}
	if e.Old != nil || e.New["content"] != "a note" || e.New["kind"] != "text" {
		t.Errorf("old, new = %v, %v", e.Old, e.New)
	}
}

// The same thing captured two times is one source (E1 in the design doc).
func TestCaptureOfTheSameContentReturnsTheStoredSource(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	first := captureText(t, s, "a note")

	again, err := s.Capture(ctx, CaptureParams{Text: "a note", Title: "a new title", Source: SourceAgent})
	if err != nil {
		t.Fatalf("second Capture: %v", err)
	}
	if again.ID != first.ID || again.Title != "" || again.Source != SourceManual {
		t.Errorf("second capture = %+v, want the first source as it was", again)
	}
	if n := countRows(t, pool, "sources"); n != 1 {
		t.Errorf("sources = %d, want 1", n)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want only the first create event", n)
	}

	// A text that differs in one space is a different text.
	if other := captureText(t, s, "a note "); other.ID == first.ID {
		t.Error("a different text got the ID of the first source")
	}
	// A link and a text with the same characters are two sources.
	link, err := s.Capture(ctx, CaptureParams{URL: "https://example.com", Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture of the link: %v", err)
	}
	if text := captureText(t, s, "https://example.com"); text.ID == link.ID {
		t.Error("a text got the ID of a link")
	}
	if linkAgain, _ := s.Capture(ctx, CaptureParams{URL: "https://example.com", Source: SourceManual}); linkAgain.ID != link.ID {
		t.Errorf("second capture of the link has the ID %d, want %d", linkAgain.ID, link.ID)
	}
}

func TestCaptureRejectsBadInput(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	cases := []struct {
		name  string
		p     CaptureParams
		field string
	}{
		{"nothing to capture", CaptureParams{Source: SourceManual}, "text"},
		{"a text and a link", CaptureParams{Text: "a note", URL: "https://example.com", Source: SourceManual}, "text"},
		{"a text of only spaces", CaptureParams{Text: " \n\t ", Source: SourceManual}, "text"},
		{"a text with a NUL", CaptureParams{Text: "a\x00b", Source: SourceManual}, "text"},
		{"a text that is not UTF-8", CaptureParams{Text: "a\xffb", Source: SourceManual}, "text"},
		{"a text that is too long", CaptureParams{Text: strings.Repeat("a", maxCaptureTextBytes+1), Source: SourceManual}, "text"},
		{"a text with a card number", CaptureParams{Text: "card " + testCard, Source: SourceManual}, "text"},
		{"a link with no scheme", CaptureParams{URL: "example.com/a", Source: SourceManual}, "url"},
		{"a link to a file on the Pi", CaptureParams{URL: "file:///etc/passwd", Source: SourceManual}, "url"},
		{"a link with no host", CaptureParams{URL: "https:///a", Source: SourceManual}, "url"},
		{"a link with a space", CaptureParams{URL: "https://example.com/a b", Source: SourceManual}, "url"},
		{"a link that is too long", CaptureParams{URL: "https://example.com/" + strings.Repeat("a", maxURLLength), Source: SourceManual}, "url"},
		{"a link with a password", CaptureParams{URL: "https://gautam:hunter2@example.com/a", Source: SourceManual}, "url"},
		{"a link with a user name", CaptureParams{URL: "https://gautam@example.com/a", Source: SourceManual}, "url"},
		{"a link with a token", CaptureParams{URL: "https://example.com/?key=" + awsKey, Source: SourceManual}, "url"},
		{"a title that is too long", CaptureParams{Text: "a note", Title: strings.Repeat("a", maxTitleLength+1), Source: SourceManual}, "title"},
		{"a title with a card number", CaptureParams{Text: "a note", Title: testCard, Source: SourceManual}, "title"},
		{"an unknown source", CaptureParams{Text: "a note", Source: "robot"}, "source"},
	}
	for _, c := range cases {
		_, err := s.Capture(ctx, c.p)
		var invalid *ValidationError
		if !errors.As(err, &invalid) {
			t.Errorf("%s: error = %v, want a ValidationError", c.name, err)
			continue
		}
		if invalid.Field != c.field {
			t.Errorf("%s: field = %q, want %q", c.name, invalid.Field, c.field)
		}
	}
	if n := countRows(t, pool, "sources") + countRows(t, pool, "events"); n != 0 {
		t.Errorf("rows in sources and events = %d, want 0", n)
	}
}

func TestCaptureAcceptsATextOfTheMaximumLength(t *testing.T) {
	s, _ := newTestService(t)
	text := strings.Repeat("a", maxCaptureTextBytes)
	if got := captureText(t, s, text); len(got.Content) != maxCaptureTextBytes {
		t.Errorf("stored %d bytes, want %d", len(got.Content), maxCaptureTextBytes)
	}
}

func TestCaptureStoresNothingWhenTheEventFails(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "ALTER TABLE events RENAME TO events_gone"); err != nil {
		t.Fatalf("rename events: %v", err)
	}

	if _, err := s.Capture(ctx, CaptureParams{Text: "a note", Source: SourceManual}); err == nil {
		t.Fatal("Capture gave no error when the event could not be stored")
	}
	if n := countRows(t, pool, "sources"); n != 0 {
		t.Errorf("sources = %d, want 0: the source and its event are one transaction", n)
	}
}

func TestGetCapture(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	if _, err := s.GetCapture(ctx, 12345); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetCapture of an unknown ID: error = %v, want ErrNotFound", err)
	}
}

func TestListCapturesReturnsTheNewestFirst(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	empty, err := s.ListCaptures(ctx, 0)
	if err != nil {
		t.Fatalf("ListCaptures: %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("ListCaptures with no source = %v, want an empty list, not nil", empty)
	}

	captureText(t, s, "first")
	captureText(t, s, "second")
	captureText(t, s, "third")

	all, err := s.ListCaptures(ctx, 0)
	if err != nil {
		t.Fatalf("ListCaptures: %v", err)
	}
	var got []string
	for _, c := range all {
		got = append(got, c.Content)
	}
	if !equal(got, []string{"third", "second", "first"}) {
		t.Errorf("order = %v, want third, second, first", got)
	}
	two, err := s.ListCaptures(ctx, 2)
	if err != nil || len(two) != 2 {
		t.Errorf("ListCaptures(2) = %d sources, error %v", len(two), err)
	}
	if _, err := s.ListCaptures(ctx, 201); err == nil {
		t.Error("ListCaptures(201) gave no error")
	}
}

// Two requests with the same content at the same moment: the unique index
// lets one insert through, and the other request gets the stored source.
func TestCaptureOfTheSameContentAtTheSameMomentStoresOneSource(t *testing.T) {
	s, pool := newTestService(t)
	const callers = 8

	ids := make(chan int64, callers)
	for range callers {
		go func() {
			capture, err := s.Capture(context.Background(), CaptureParams{Text: "a note", Source: SourceManual})
			if err != nil {
				t.Errorf("Capture: %v", err)
			}
			ids <- capture.ID
		}()
	}
	first := <-ids
	for range callers - 1 {
		if id := <-ids; id != first {
			t.Errorf("a caller got the ID %d, and a different caller got %d", id, first)
		}
	}
	if n := countRows(t, pool, "sources"); n != 1 {
		t.Errorf("sources = %d, want 1", n)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want 1", n)
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run Capture"
```
Expected: the run fails. The output contains `undefined: Capture`.

- [ ] **Step 3: Write the queries**

`ON CONFLICT ... DO NOTHING` is what makes a second capture of the same content safe when two requests come at the same moment.

Create `queries/sources.sql`:

```sql
-- name: InsertSource :one
-- Returns no row when a live source has this kind and content already.
INSERT INTO sources (kind, title, content, url, file_path, media_type, size_bytes, sha256, source, source_ref, verified)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (kind, sha256) WHERE deleted_at IS NULL DO NOTHING
RETURNING *;

-- name: GetSourceByHash :one
SELECT * FROM sources
WHERE kind = $1 AND sha256 = $2 AND deleted_at IS NULL;

-- name: GetSource :one
SELECT * FROM sources
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListSources :many
SELECT * FROM sources
WHERE deleted_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT $1;
```

- [ ] **Step 4: Write the code**

Create `internal/service/capture.go`:

```go
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

const (
	// maxCaptureTextBytes is the largest text of one capture: 1 MB.
	maxCaptureTextBytes = 1 << 20
	maxURLLength        = 2000
)

// Capture is one row of sources: a text, a link or a file, stored exactly
// as it came. Its record type is source.
type Capture struct {
	ID int64 `json:"id"`
	// Kind is text, link or file.
	Kind  string `json:"kind"`
	Title string `json:"title,omitempty"`
	// Content is the text of a text capture.
	Content string `json:"content,omitempty"`
	// URL is the link of a link capture.
	URL string `json:"url,omitempty"`
	// FilePath is the path of a file capture, below the file store.
	FilePath  string `json:"file_path,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	// SHA256 is the hash of the text, the URL or the file bytes, in hex.
	SHA256 string `json:"sha256"`
	// ProcessedAt is nil until the inbox job reads the capture.
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
	Source      Source     `json:"source"`
	SourceRef   *int64     `json:"source_ref,omitempty"`
	Verified    bool       `json:"verified"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// CaptureParams is the input of Capture. Exactly one of Text and URL is
// set.
type CaptureParams struct {
	// Text is stored as it is. The service does not trim it.
	Text string
	// URL is a link with the scheme http or https. The service stores the
	// link; it does not get the page.
	URL string
	// Title is optional: a label for the capture.
	Title string
	// Source says who captures: manual for Gautam, agent for an MCP tool.
	Source Source
}

// Capture validates p, stores one source and appends a create event, in one
// transaction. The same content captured two times is one source: the
// second call returns the stored source and writes nothing. A source is
// always verified, because it holds what came in and no agent inferred it.
func (s *Service) Capture(ctx context.Context, p CaptureParams) (Capture, error) {
	row, err := p.toRow()
	if err != nil {
		return Capture{}, err
	}
	return s.storeSource(ctx, row)
}

// storeSource inserts row, or returns the live source that has the same
// kind and hash.
func (s *Service) storeSource(ctx context.Context, row store.InsertSourceParams) (Capture, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Capture{}, fmt.Errorf("capture: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	stored, err := q.InsertSource(ctx, row)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, err := q.GetSourceByHash(ctx, store.GetSourceByHashParams{Kind: row.Kind, Sha256: row.Sha256})
		if err != nil {
			return Capture{}, fmt.Errorf("capture: %w", err)
		}
		return captureFromRow(existing), nil
	}
	if err != nil {
		return Capture{}, fmt.Errorf("capture: %w", err)
	}
	capture := captureFromRow(stored)
	if err := recordEvent(ctx, q, "create", "sources", capture.ID, Source(row.Source), nil, capture); err != nil {
		return Capture{}, fmt.Errorf("capture: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Capture{}, fmt.Errorf("capture: %w", err)
	}
	return capture, nil
}

// toRow checks every field and returns the row to insert.
func (p CaptureParams) toRow() (store.InsertSourceParams, error) {
	row := store.InsertSourceParams{Source: string(p.Source), Verified: true}

	link := strings.TrimSpace(p.URL)
	if (p.Text == "") == (link == "") {
		return row, invalid("text", "give a text or a url, and not the two together")
	}
	title, err := captureTitle(p.Title)
	if err != nil {
		return row, err
	}
	row.Title = title
	if err := (Provenance{Source: p.Source}).validate(); err != nil {
		return row, err
	}

	if link != "" {
		if err := checkLink(link); err != nil {
			return row, err
		}
		row.Kind, row.Url, row.Sha256 = "link", &link, hashHex([]byte(link))
		return row, nil
	}

	if strings.TrimSpace(p.Text) == "" {
		return row, invalid("text", "must not be empty")
	}
	if len(p.Text) > maxCaptureTextBytes {
		return row, invalid("text", "must be at most 1 MB")
	}
	if err := checkText("text", p.Text); err != nil {
		return row, err
	}
	text := p.Text
	row.Kind, row.Content, row.Sha256 = "text", &text, hashHex([]byte(text))
	return row, nil
}

// captureTitle returns the optional title as the table holds it: nil when
// the caller gave none.
func captureTitle(title string) (*string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(title) > maxTitleLength {
		return nil, invalid("title", fmt.Sprintf("must be at most %d characters", maxTitleLength))
	}
	if err := checkText("title", title); err != nil {
		return nil, err
	}
	return &title, nil
}

// checkLink accepts an absolute http or https URL.
func checkLink(link string) error {
	if utf8.RuneCountInString(link) > maxURLLength {
		return invalid("url", fmt.Sprintf("must be at most %d characters", maxURLLength))
	}
	if err := checkText("url", link); err != nil {
		return err
	}
	parsed, err := url.Parse(link)
	if err != nil || strings.ContainsAny(link, " \t\r\n") {
		return invalid("url", "must be a link, such as https://example.com/page")
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return invalid("url", "must start with http:// or https:// and name a host")
	}
	// https://name:password@host has a password in it (AGENTS.md rule 7).
	if parsed.User != nil {
		return invalid("url", "must not hold a user name or a password")
	}
	return nil
}

func hashHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func captureFromRow(row store.Source) Capture {
	text := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	c := Capture{
		ID:        row.ID,
		Kind:      row.Kind,
		Title:     text(row.Title),
		Content:   text(row.Content),
		URL:       text(row.Url),
		FilePath:  text(row.FilePath),
		MediaType: text(row.MediaType),
		SHA256:    row.Sha256,
		Source:    Source(row.Source),
		SourceRef: row.SourceRef,
		Verified:  row.Verified,
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
	if row.SizeBytes != nil {
		c.SizeBytes = *row.SizeBytes
	}
	if row.ProcessedAt != nil {
		at := row.ProcessedAt.UTC()
		c.ProcessedAt = &at
	}
	return c
}

// GetCapture returns the source with this ID. It returns ErrNotFound when
// there is no such source or the source is deleted.
func (s *Service) GetCapture(ctx context.Context, id int64) (Capture, error) {
	row, err := store.New(s.pool).GetSource(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Capture{}, ErrNotFound
	}
	if err != nil {
		return Capture{}, fmt.Errorf("get source: %w", err)
	}
	return captureFromRow(row), nil
}

// ListCaptures returns the sources that are not deleted, the newest first.
// limit 0 means 50; the maximum is 200.
func (s *Service) ListCaptures(ctx context.Context, limit int) ([]Capture, error) {
	maxRows, err := checkLimit(limit)
	if err != nil {
		return nil, err
	}
	rows, err := store.New(s.pool).ListSources(ctx, maxRows)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	captures := make([]Capture, 0, len(rows))
	for _, row := range rows {
		captures = append(captures, captureFromRow(row))
	}
	return captures, nil
}
```

- [ ] **Step 5: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./internal/service ARGS="-run Capture"
```
Expected: the run passes. The last line starts with `ok`.

- [ ] **Step 6: Commit**

Run:
```bash
git add queries/sources.sql internal/service/capture.go internal/service/capture_test.go
git commit -m "feat(service): capture a text or a link as a source"
```

---

### Task 5: Service: capture a file into the file store

**Files:**
- Create: `internal/service/files.go`
- Modify: `internal/service/capture.go`, `internal/service/service.go`, `internal/config/config.go`, `config/lifeos.yaml`, `cmd/lifeos/serve.go`
- Test: `internal/service/capture_file_test.go`, `internal/config/config_test.go`, `cmd/lifeos/serve_test.go`

**Interfaces:**
- Consumes (Task 4): `CaptureParams`, `(*Service).Capture`, `(*Service).storeSource`, `captureTitle`, `hashHex`; the test helper `captureText`.
- Consumes (Tasks 1 and 2): `secretKind`, `testCard`.
- Consumes (plan 1): `config.Config`, `config.Load(path string) (Config, error)`, `serve(args []string) error` and `serveOn` in `cmd/lifeos/serve.go`.
- Produces (package `service`):
  - `type Files struct{ Dir, Inbox string; MaxBytes int64 }` and `(*Service).WithFiles(files Files) *Service`, which returns the same service. The field `Service.files`.
  - Three new fields of `CaptureParams`: `InboxPath string`, `Upload io.Reader`, `FileName string`. Exactly one of `Text`, `URL`, `InboxPath` and `Upload` is set.
  - `(*Service).captureFile(ctx, CaptureParams) (Capture, error)`, `(*Service).openInboxFile(path string) (root *os.Root, rel string, file *os.File, err error)`, `(*Service).storeFile(r io.Reader, field string) (storedFile, error)`, `sizeText(n int64) string`
  - Test helpers: `newFileService(t) (*Service, *pgxpool.Pool)`, `putInInbox(t, s, name string, content []byte) string`, `storedFiles(t, s) []string`, `exists(path string) bool`, the variable `pdf`, the constant `testMaxFileBytes` (4096)
- Produces (package `config`): the fields `FilesDir string`, `InboxDir string`, `MaxFileMB int64` for the keys `files_dir`, `inbox_dir`, `max_file_mb`.
- Produces (package `main`): `fileStore(cfg config.Config) (service.Files, error)`.

- [ ] **Step 1: Write the failing tests of the service**

Create `internal/service/capture_file_test.go`:

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

	"github.com/jackc/pgx/v5/pgxpool"
)

// testMaxFileBytes is the file limit of the tests: small, so that a test
// can go over it.
const testMaxFileBytes = 4096

// pdf is the start of a PDF file. The service finds the type in the bytes.
var pdf = []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\n")

// newFileService returns a service with a file store and an inbox in new
// temporary folders.
func newFileService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	s, pool := newTestService(t)
	s.WithFiles(Files{Dir: t.TempDir(), Inbox: t.TempDir(), MaxBytes: testMaxFileBytes})
	return s, pool
}

// putInInbox writes one file into the inbox and returns its full path.
func putInInbox(t *testing.T, s *Service, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(s.files.Inbox, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("make the folder of %s: %v", name, err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// storedFiles returns the paths of the files in the file store, below Dir.
func storedFiles(t *testing.T, s *Service) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(s.files.Dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(s.files.Dir, path)
		paths = append(paths, rel)
		return err
	})
	if err != nil {
		t.Fatalf("walk the file store: %v", err)
	}
	return paths
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestCaptureMovesAFileFromTheInboxIntoTheFileStore(t *testing.T) {
	s, pool := newFileService(t)
	inboxPath := putInInbox(t, s, "policy.pdf", pdf)
	hash := hashHex(pdf)

	got, err := s.Capture(context.Background(), CaptureParams{InboxPath: "policy.pdf", Source: SourceAgent})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	if got.Kind != "file" || got.Title != "policy.pdf" || got.MediaType != "application/pdf" {
		t.Errorf("kind, title, media_type = %q, %q, %q", got.Kind, got.Title, got.MediaType)
	}
	if got.SizeBytes != int64(len(pdf)) || got.SHA256 != hash {
		t.Errorf("size_bytes, sha256 = %d, %q", got.SizeBytes, got.SHA256)
	}
	if want := hash[:2] + "/" + hash; got.FilePath != want {
		t.Errorf("file_path = %q, want %q", got.FilePath, want)
	}
	if got.Content != "" || got.URL != "" {
		t.Errorf("content, url = %q, %q, want them empty", got.Content, got.URL)
	}
	if got.Source != SourceAgent || !got.Verified {
		t.Errorf("source, verified = %q, %v, want agent, true", got.Source, got.Verified)
	}

	stored, err := os.ReadFile(filepath.Join(s.files.Dir, got.FilePath))
	if err != nil {
		t.Fatalf("read the stored file: %v", err)
	}
	if !bytes.Equal(stored, pdf) {
		t.Error("the stored file does not have the bytes of the inbox file")
	}
	if exists(inboxPath) {
		t.Error("the file is still in the inbox")
	}
	if files := storedFiles(t, s); len(files) != 1 {
		t.Errorf("files in the file store = %v, want only the stored file", files)
	}
	events := allEvents(t, pool)
	if len(events) != 1 || events[0].Action != "create" || events[0].RecordType != "sources" {
		t.Errorf("events = %+v, want one create event for sources", events)
	}
}

func TestCaptureAcceptsAFullPathAndAFolderInTheInbox(t *testing.T) {
	s, _ := newFileService(t)
	ctx := context.Background()

	full := putInInbox(t, s, "a.pdf", pdf)
	got, err := s.Capture(ctx, CaptureParams{InboxPath: full, Title: "Policy 2026", Source: SourceAgent})
	if err != nil {
		t.Fatalf("Capture with a full path: %v", err)
	}
	if got.Title != "Policy 2026" {
		t.Errorf("title = %q, want the title of the caller", got.Title)
	}

	putInInbox(t, s, "scans/b.bin", []byte{0, 1, 2, 3})
	got, err = s.Capture(ctx, CaptureParams{InboxPath: "scans/b.bin", Source: SourceAgent})
	if err != nil {
		t.Fatalf("Capture of a file in a folder: %v", err)
	}
	if got.Title != "b.bin" || got.MediaType != "application/octet-stream" {
		t.Errorf("title, media_type = %q, %q", got.Title, got.MediaType)
	}
}

func TestCaptureStoresAnUpload(t *testing.T) {
	s, _ := newFileService(t)

	got, err := s.Capture(context.Background(), CaptureParams{Upload: bytes.NewReader(pdf), FileName: "receipt.pdf", Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if got.Kind != "file" || got.Title != "receipt.pdf" || got.MediaType != "application/pdf" || got.Source != SourceManual {
		t.Errorf("capture = %+v", got)
	}
	if !exists(filepath.Join(s.files.Dir, got.FilePath)) {
		t.Error("the upload is not in the file store")
	}

	// An upload can come without a name.
	noName, err := s.Capture(context.Background(), CaptureParams{Upload: strings.NewReader("\x89PNG\r\n\x1a\n...."), Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture without a name: %v", err)
	}
	if noName.Title != "" || noName.MediaType != "image/png" {
		t.Errorf("title, media_type = %q, %q", noName.Title, noName.MediaType)
	}
}

// The same file captured two times is one source and one stored file (E1).
func TestCaptureOfTheSameFileReturnsTheStoredSource(t *testing.T) {
	s, pool := newFileService(t)
	ctx := context.Background()

	first, err := s.Capture(ctx, CaptureParams{Upload: bytes.NewReader(pdf), FileName: "receipt.pdf", Source: SourceManual})
	if err != nil {
		t.Fatalf("first Capture: %v", err)
	}
	inboxPath := putInInbox(t, s, "the-same.pdf", pdf)
	again, err := s.Capture(ctx, CaptureParams{InboxPath: "the-same.pdf", Source: SourceAgent})
	if err != nil {
		t.Fatalf("second Capture: %v", err)
	}

	if again.ID != first.ID || again.Title != "receipt.pdf" {
		t.Errorf("second capture = %+v, want the first source as it was", again)
	}
	if n := countRows(t, pool, "sources"); n != 1 {
		t.Errorf("sources = %d, want 1", n)
	}
	if files := storedFiles(t, s); len(files) != 1 {
		t.Errorf("files in the file store = %v, want 1", files)
	}
	// The stored file has the bytes, so the inbox file can go.
	if exists(inboxPath) {
		t.Error("the file of the second capture is still in the inbox")
	}
}

func TestCaptureRejectsABadFile(t *testing.T) {
	s, pool := newFileService(t)
	ctx := context.Background()

	outside := filepath.Join(t.TempDir(), "outside.pdf")
	if err := os.WriteFile(outside, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(s.files.Inbox, "link.pdf")); err != nil {
		t.Fatal(err)
	}
	putInInbox(t, s, "folder/a.pdf", pdf)
	putInInbox(t, s, "empty.pdf", nil)
	putInInbox(t, s, "big.bin", bytes.Repeat([]byte{1}, testMaxFileBytes+1))
	putInInbox(t, s, "card.txt", []byte("my card\n"+testCard+"\n"))
	putInInbox(t, s, "ok.pdf", pdf)
	big := bytes.Repeat([]byte{1}, testMaxFileBytes+1)

	cases := []struct {
		name  string
		p     CaptureParams
		field string
	}{
		{"a path above the inbox", CaptureParams{InboxPath: "../outside.pdf", Source: SourceAgent}, "path"},
		{"a full path that is not in the inbox", CaptureParams{InboxPath: outside, Source: SourceAgent}, "path"},
		{"a link in the inbox to a file outside", CaptureParams{InboxPath: "link.pdf", Source: SourceAgent}, "path"},
		{"a file that does not exist", CaptureParams{InboxPath: "missing.pdf", Source: SourceAgent}, "path"},
		{"a folder", CaptureParams{InboxPath: "folder", Source: SourceAgent}, "path"},
		{"an empty file", CaptureParams{InboxPath: "empty.pdf", Source: SourceAgent}, "path"},
		{"a file over the limit", CaptureParams{InboxPath: "big.bin", Source: SourceAgent}, "path"},
		{"a text file with a card number", CaptureParams{InboxPath: "card.txt", Source: SourceAgent}, "path"},
		{"an empty upload", CaptureParams{Upload: strings.NewReader(""), Source: SourceManual}, "file"},
		{"an upload over the limit", CaptureParams{Upload: bytes.NewReader(big), Source: SourceManual}, "file"},
		{"an upload with a card number", CaptureParams{Upload: strings.NewReader("card " + testCard), Source: SourceManual}, "file"},
		{"an upload with a name that is too long", CaptureParams{Upload: bytes.NewReader(pdf), FileName: strings.Repeat("a", maxTitleLength+1), Source: SourceManual}, "title"},
		{"an upload with a card number in its name", CaptureParams{Upload: bytes.NewReader(pdf), FileName: testCard + ".pdf", Source: SourceManual}, "title"},
		{"a file and a text", CaptureParams{InboxPath: "ok.pdf", Text: "a note", Source: SourceAgent}, "text"},
		{"a file and an upload", CaptureParams{InboxPath: "ok.pdf", Upload: bytes.NewReader(pdf), Source: SourceAgent}, "text"},
	}
	for _, c := range cases {
		_, err := s.Capture(ctx, c.p)
		var invalid *ValidationError
		if !errors.As(err, &invalid) {
			t.Errorf("%s: error = %v, want a ValidationError", c.name, err)
			continue
		}
		if invalid.Field != c.field {
			t.Errorf("%s: field = %q, want %q", c.name, invalid.Field, c.field)
		}
	}

	if n := countRows(t, pool, "sources") + countRows(t, pool, "events"); n != 0 {
		t.Errorf("rows in sources and events = %d, want 0", n)
	}
	if files := storedFiles(t, s); len(files) != 0 {
		t.Errorf("files in the file store = %v, want none: a refused file leaves no copy", files)
	}
	// A refused file stays in the inbox, so that Gautam can look at it.
	for _, name := range []string{"big.bin", "card.txt", "ok.pdf", "empty.pdf"} {
		if !exists(filepath.Join(s.files.Inbox, name)) {
			t.Errorf("%s is not in the inbox any more", name)
		}
	}
	if !exists(outside) {
		t.Error("the file outside the inbox is gone")
	}
}

// The limit is the largest size that Capture accepts.
func TestCaptureAcceptsAFileOfTheMaximumSize(t *testing.T) {
	s, _ := newFileService(t)
	content := bytes.Repeat([]byte{1}, testMaxFileBytes)

	got, err := s.Capture(context.Background(), CaptureParams{Upload: bytes.NewReader(content), Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if got.SizeBytes != testMaxFileBytes {
		t.Errorf("size_bytes = %d, want %d", got.SizeBytes, testMaxFileBytes)
	}
}

func TestCaptureOfAFileNeedsTheFileStore(t *testing.T) {
	s, _ := newTestService(t) // no WithFiles

	_, err := s.Capture(context.Background(), CaptureParams{Upload: bytes.NewReader(pdf), Source: SourceManual})
	var invalid *ValidationError
	if err == nil || errors.As(err, &invalid) {
		t.Errorf("error = %v, want an error that is not a ValidationError", err)
	}
}

func TestCaptureKeepsTheInboxFileWhenTheEventFails(t *testing.T) {
	s, pool := newFileService(t)
	ctx := context.Background()
	inboxPath := putInInbox(t, s, "policy.pdf", pdf)
	if _, err := pool.Exec(ctx, "ALTER TABLE events RENAME TO events_gone"); err != nil {
		t.Fatalf("rename events: %v", err)
	}

	if _, err := s.Capture(ctx, CaptureParams{InboxPath: "policy.pdf", Source: SourceAgent}); err == nil {
		t.Fatal("Capture gave no error when the event could not be stored")
	}
	if n := countRows(t, pool, "sources"); n != 0 {
		t.Errorf("sources = %d, want 0", n)
	}
	if !exists(inboxPath) {
		t.Error("the inbox file is gone, and no source holds it")
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run Capture"
```
Expected: the run fails. The output contains `s.WithFiles undefined`.

- [ ] **Step 3: Give the service its file store**

In `internal/service/service.go`, replace this:

```go
type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}
```

With this:

```go
type Service struct {
	pool  *pgxpool.Pool
	now   func() time.Time
	files Files
}
```

- [ ] **Step 4: Write the file store**

A refused file leaves no copy: the file is in a temporary name until each check passed. The file goes to its place before the row is stored; when the insert fails, a file with no row stays. That file is harmless: the next capture of the same bytes uses it.

Create `internal/service/files.go`:

```go
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// Files says where the service keeps the files of file captures.
type Files struct {
	// Dir is the file store. A file is at Dir/<the first two characters
	// of its SHA-256>/<its SHA-256>, so the same bytes are one file.
	Dir string
	// Inbox is the folder where a file waits for its capture.
	Inbox string
	// MaxBytes is the largest file that Capture accepts.
	MaxBytes int64
}

// WithFiles sets the file store and returns s. Without it, Capture stores a
// text and a link, and it refuses a file.
func (s *Service) WithFiles(files Files) *Service {
	s.files = files
	return s
}

// captureFile stores the file of p in the file store and its row in
// sources. A file from the inbox leaves the inbox after the row is stored.
func (s *Service) captureFile(ctx context.Context, p CaptureParams) (Capture, error) {
	if err := (Provenance{Source: p.Source}).validate(); err != nil {
		return Capture{}, err
	}
	if s.files.Dir == "" {
		return Capture{}, errors.New("capture: the service has no file store")
	}

	field, name, reader := "file", p.FileName, p.Upload
	var inbox *os.Root
	var inboxPath string
	if p.InboxPath != "" {
		root, rel, file, err := s.openInboxFile(p.InboxPath)
		if err != nil {
			return Capture{}, err
		}
		defer root.Close()
		defer file.Close()
		field, name, reader = "path", filepath.Base(rel), file
		inbox, inboxPath = root, rel
	}

	// The file name is the title when the caller gives no title.
	title := p.Title
	if strings.TrimSpace(title) == "" {
		title = name
	}
	titleOrNil, err := captureTitle(title)
	if err != nil {
		return Capture{}, err
	}
	stored, err := s.storeFile(reader, field)
	if err != nil {
		return Capture{}, err
	}
	capture, err := s.storeSource(ctx, store.InsertSourceParams{
		Kind:      "file",
		Title:     titleOrNil,
		FilePath:  &stored.path,
		MediaType: &stored.mediaType,
		SizeBytes: &stored.size,
		Sha256:    stored.sha256,
		Source:    string(p.Source),
		Verified:  true,
	})
	if err != nil {
		return Capture{}, err
	}
	// The file store has the bytes now. This is also true when the source
	// existed already.
	if inbox != nil {
		if err := inbox.Remove(inboxPath); err != nil {
			return Capture{}, fmt.Errorf("capture: remove the file from the inbox: %w", err)
		}
	}
	return capture, nil
}

// openInboxFile opens one file of the inbox. path is a path below the
// inbox, or a full path that is in the inbox. os.Root refuses a path that
// leaves the inbox, also through a symbolic link.
func (s *Service) openInboxFile(path string) (root *os.Root, rel string, file *os.File, err error) {
	rel = path
	if filepath.IsAbs(path) {
		rel, err = filepath.Rel(s.files.Inbox, path)
		if err != nil {
			return nil, "", nil, invalid("path", "must name a file in the inbox folder")
		}
	}
	root, err = os.OpenRoot(s.files.Inbox)
	if err != nil {
		return nil, "", nil, fmt.Errorf("capture: open the inbox: %w", err)
	}
	file, err = root.Open(rel)
	if err != nil {
		root.Close()
		if errors.Is(err, fs.ErrNotExist) {
			return nil, "", nil, invalid("path", "no file has this path in the inbox folder")
		}
		return nil, "", nil, invalid("path", "must name a file in the inbox folder")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		root.Close()
		return nil, "", nil, invalid("path", "must name a file, not a folder")
	}
	return root, rel, file, nil
}

// storedFile is one file in the file store.
type storedFile struct {
	// path is the path below Files.Dir.
	path      string
	sha256    string
	mediaType string
	size      int64
}

// storeFile copies r into the file store and returns where the file is.
// field is the name of the input field, for an error. It refuses an empty
// file, a file over the limit and a text file that holds a secret; a
// refused file leaves no copy.
func (s *Service) storeFile(r io.Reader, field string) (storedFile, error) {
	tmp, err := os.CreateTemp(s.files.Dir, "incoming-*")
	if err != nil {
		return storedFile{}, fmt.Errorf("capture: store the file: %w", err)
	}
	// After the rename, this removes nothing.
	defer os.Remove(tmp.Name())

	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(r, s.files.MaxBytes+1))
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return storedFile{}, fmt.Errorf("capture: store the file: %w", err)
	}
	if size == 0 {
		return storedFile{}, invalid(field, "must not be an empty file")
	}
	if size > s.files.MaxBytes {
		return storedFile{}, invalid(field, "must be a file of at most "+sizeText(s.files.MaxBytes))
	}

	content, err := os.ReadFile(tmp.Name())
	if err != nil {
		return storedFile{}, fmt.Errorf("capture: store the file: %w", err)
	}
	mediaType := http.DetectContentType(content)
	// The service can read a text file, so the rule on secrets applies.
	if strings.HasPrefix(mediaType, "text/") {
		if kind := secretKind(string(content)); kind != "" {
			return storedFile{}, invalid(field, "must not hold "+kind+": LifeOS does not store one")
		}
	}

	sum := hex.EncodeToString(hash.Sum(nil))
	stored := storedFile{path: sum[:2] + "/" + sum, sha256: sum, mediaType: mediaType, size: size}
	final := filepath.Join(s.files.Dir, stored.path)
	if _, err := os.Stat(final); err == nil {
		// The file store has these bytes already.
		return stored, nil
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o700); err != nil {
		return storedFile{}, fmt.Errorf("capture: store the file: %w", err)
	}
	if err := os.Rename(tmp.Name(), final); err != nil {
		return storedFile{}, fmt.Errorf("capture: store the file: %w", err)
	}
	return stored, nil
}

// sizeText returns a number of bytes in words: "50 MB" or "4096 bytes".
func sizeText(n int64) string {
	if n >= 1<<20 {
		return fmt.Sprintf("%d MB", n>>20)
	}
	return fmt.Sprintf("%d bytes", n)
}
```

- [ ] **Step 5: Send a file from `Capture` to the file store**

In `internal/service/capture.go`, replace this:

```go
	"fmt"
	"net/url"
```

With this:

```go
	"fmt"
	"io"
	"net/url"
```

In `internal/service/capture.go`, replace this:

```go
// CaptureParams is the input of Capture. Exactly one of Text and URL is
// set.
type CaptureParams struct {
```

With this:

```go
// CaptureParams is the input of Capture. Exactly one of Text, URL,
// InboxPath and Upload is set.
type CaptureParams struct {
```

In `internal/service/capture.go`, replace this:

```go
	URL string
	// Title is optional: a label for the capture.
	Title string
```

With this:

```go
	URL string
	// InboxPath names a file in the inbox folder: a path below the inbox,
	// or a full path that is in the inbox. Capture moves the file into the
	// file store.
	InboxPath string
	// Upload is the bytes of a file that came with the request.
	Upload io.Reader
	// FileName is optional: the name of the upload.
	FileName string
	// Title is optional: a label for the capture. A file with no title
	// gets its file name.
	Title string
```

In `internal/service/capture.go`, replace this:

```go
func (s *Service) Capture(ctx context.Context, p CaptureParams) (Capture, error) {
	row, err := p.toRow()
```

With this:

```go
func (s *Service) Capture(ctx context.Context, p CaptureParams) (Capture, error) {
	given := 0
	for _, isGiven := range []bool{p.Text != "", strings.TrimSpace(p.URL) != "", p.InboxPath != "", p.Upload != nil} {
		if isGiven {
			given++
		}
	}
	if given != 1 {
		return Capture{}, invalid("text", "give one of these, and only one: a text, a url, a file")
	}
	if p.InboxPath != "" || p.Upload != nil {
		return s.captureFile(ctx, p)
	}
	row, err := p.toRow()
```

In `internal/service/capture.go`, replace this:

```go
// toRow checks every field and returns the row to insert.
func (p CaptureParams) toRow() (store.InsertSourceParams, error) {
```

With this:

```go
// toRow checks every field of a text or a link and returns the row to
// insert.
func (p CaptureParams) toRow() (store.InsertSourceParams, error) {
```

In `internal/service/capture.go`, replace this:

```go
	link := strings.TrimSpace(p.URL)
	if (p.Text == "") == (link == "") {
		return row, invalid("text", "give a text or a url, and not the two together")
	}
	title, err := captureTitle(p.Title)
```

With this:

```go
	link := strings.TrimSpace(p.URL)
	title, err := captureTitle(p.Title)
```

- [ ] **Step 6: Run the tests of the service to verify that they pass**

Run:
```bash
make test PKG=./internal/service ARGS="-run Capture"
```
Expected: the run passes. The last line starts with `ok`.

- [ ] **Step 7: Commit**

Run:
```bash
git add internal/service/files.go internal/service/capture.go internal/service/service.go internal/service/capture_file_test.go
git commit -m "feat(service): capture a file into the file store"
```

- [ ] **Step 8: Write the failing tests of the config and of the start**

In `internal/config/config_test.go`, replace this:

```go
database_url: postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable
`
```

With this:

```go
database_url: postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable
files_dir: /var/lib/lifeos/files
inbox_dir: /var/lib/lifeos/inbox
max_file_mb: 50
`
```

In `internal/config/config_test.go`, replace this:

```go
		DatabaseURL: "postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable",
	}
```

With this:

```go
		DatabaseURL: "postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable",
		FilesDir:    "/var/lib/lifeos/files",
		InboxDir:    "/var/lib/lifeos/inbox",
		MaxFileMB:   50,
	}
```

In `internal/config/config_test.go`, replace this:

```go
		{
			name:    "unknown key",
```

With this:

```go
		{
			name:    "file store that is not a full path",
			content: strings.Replace(valid, "files_dir: /var/lib/lifeos/files", "files_dir: files", 1),
			wantErr: "files_dir",
		},
		{
			name:    "missing inbox",
			content: strings.Replace(valid, "inbox_dir: /var/lib/lifeos/inbox", "", 1),
			wantErr: "inbox_dir",
		},
		{
			name:    "inbox that is the file store",
			content: strings.Replace(valid, "inbox_dir: /var/lib/lifeos/inbox", "inbox_dir: /var/lib/lifeos/files", 1),
			wantErr: "inbox_dir",
		},
		{
			name:    "missing file limit",
			content: strings.Replace(valid, "max_file_mb: 50", "", 1),
			wantErr: "max_file_mb",
		},
		{
			name:    "file limit below 1",
			content: strings.Replace(valid, "max_file_mb: 50", "max_file_mb: -1", 1),
			wantErr: "max_file_mb",
		},
		{
			name:    "unknown key",
```

In `cmd/lifeos/serve_test.go`, replace this:

```go
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
```

With this:

```go
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
```

In `cmd/lifeos/serve_test.go`, replace this:

```go
		})
	}
}
```

With this:

```go
		})
	}
}

func TestFileStoreNeedsTheTwoFolders(t *testing.T) {
	dir := t.TempDir()
	notAFolder := filepath.Join(dir, "a-file")
	if err := os.WriteFile(notAFolder, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	files, err := fileStore(config.Config{FilesDir: dir, InboxDir: t.TempDir(), MaxFileMB: 50})
	if err != nil {
		t.Fatalf("fileStore: %v", err)
	}
	if files.Dir != dir || files.MaxBytes != 50<<20 {
		t.Errorf("files = %+v, want the folder and 50 MB in bytes", files)
	}

	for key, cfg := range map[string]config.Config{
		"files_dir": {FilesDir: filepath.Join(dir, "absent"), InboxDir: dir, MaxFileMB: 50},
		"inbox_dir": {FilesDir: dir, InboxDir: notAFolder, MaxFileMB: 50},
	} {
		_, err := fileStore(cfg)
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("error = %v, want an error that names %s", err, key)
		}
	}
}
```

- [ ] **Step 9: Run the tests to verify that they fail**

Run:
```bash
make test PKG="./internal/config ./cmd/lifeos"
```
Expected: the run fails. The output contains `unknown field FilesDir in struct literal of type Config` and `undefined: fileStore`.

- [ ] **Step 10: Add the three keys to the config**

In `internal/config/config.go`, replace this:

```go
	"os"
```

With this:

```go
	"os"
	"path/filepath"
```

In `internal/config/config.go`, replace this:

```go
	DatabaseURL string `yaml:"database_url"`
}
```

With this:

```go
	DatabaseURL string `yaml:"database_url"`
	// FilesDir is the file store: the folder that holds the captured
	// files.
	FilesDir string `yaml:"files_dir"`
	// InboxDir is the folder where a file waits for its capture.
	InboxDir string `yaml:"inbox_dir"`
	// MaxFileMB is the largest file that a capture accepts, in megabytes.
	MaxFileMB int64 `yaml:"max_file_mb"`
}
```

In `internal/config/config.go`, replace this:

```go
// Load reads and checks the file at path. It rejects unknown keys, a missing
// value, and a listen address that is not on the loopback interface.
func Load(path string) (Config, error) {
```

With this:

```go
// Load reads and checks the file at path. It rejects unknown keys, a missing
// value, a listen address that is not on the loopback interface, and a
// folder that is not a full path.
func Load(path string) (Config, error) {
```

In `internal/config/config.go`, replace this:

```go
		}
	}
```

With this:

```go
		}
	}
	for key, dir := range map[string]string{"files_dir": cfg.FilesDir, "inbox_dir": cfg.InboxDir} {
		if !filepath.IsAbs(dir) {
			return Config{}, fmt.Errorf("config: %s: %s: %q is not a full path such as /var/lib/lifeos/files", path, key, dir)
		}
	}
	if filepath.Clean(cfg.InboxDir) == filepath.Clean(cfg.FilesDir) {
		return Config{}, fmt.Errorf("config: %s: inbox_dir: must not be the folder of files_dir", path)
	}
	if cfg.MaxFileMB < 1 {
		return Config{}, fmt.Errorf("config: %s: max_file_mb: must be 1 or more", path)
	}
```

In `config/lifeos.yaml`, replace this:

```yaml
database_url: postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable
```

With this:

```yaml
database_url: postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable

# The file store and the inbox of captured files. The two folders must
# exist. The nightly backup copies files_dir.
files_dir: /var/lib/lifeos/files
inbox_dir: /var/lib/lifeos/inbox
# The largest file that a capture accepts, in megabytes.
max_file_mb: 50
```

- [ ] **Step 11: Give the file store to the service at the start**

`fileStore` checks the two folders at the start, so that a wrong path stops `lifeos serve` and not the first capture.

In `cmd/lifeos/serve.go`, replace this:

```go
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
```

With this:

```go
		return err
	}
	files, err := fileStore(cfg)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
```

In `cmd/lifeos/serve.go`, replace this:

```go
	}
	slog.Info("lifeos is listening", "http", httpListener.Addr().String(), "mcp", mcpListener.Addr().String())
	return serveOn(ctx, httpListener, mcpListener, service.New(pool))
}
```

With this:

```go
	}
	slog.Info("lifeos is listening", "http", httpListener.Addr().String(), "mcp", mcpListener.Addr().String())
	return serveOn(ctx, httpListener, mcpListener, service.New(pool).WithFiles(files))
}

// fileStore returns the settings of the file store. It checks that the two
// folders exist, so that a wrong path stops the start and not a capture.
func fileStore(cfg config.Config) (service.Files, error) {
	for _, folder := range []struct{ key, dir string }{{"files_dir", cfg.FilesDir}, {"inbox_dir", cfg.InboxDir}} {
		info, err := os.Stat(folder.dir)
		if err != nil {
			return service.Files{}, fmt.Errorf("%s: %w", folder.key, err)
		}
		if !info.IsDir() {
			return service.Files{}, fmt.Errorf("%s: %s is not a folder", folder.key, folder.dir)
		}
	}
	return service.Files{Dir: cfg.FilesDir, Inbox: cfg.InboxDir, MaxBytes: cfg.MaxFileMB << 20}, nil
}
```

- [ ] **Step 12: Run the tests to verify that they pass**

Run:
```bash
make test PKG="./internal/config ./cmd/lifeos"
```
Expected: the run passes. The output contains `internal/config` and `cmd/lifeos`.

- [ ] **Step 13: Commit**

Run:
```bash
git add internal/config/config.go internal/config/config_test.go config/lifeos.yaml cmd/lifeos/serve.go cmd/lifeos/serve_test.go
git commit -m "feat(config): add the file store and the inbox to the config"
```

---

### Task 6: Service: `source` is a record type

**Files:**
- Create: `internal/service/capture_change.go`
- Modify: `queries/sources.sql`, `internal/service/records.go`, `internal/service/review.go`
- Test: `internal/service/capture_change_test.go`

**Interfaces:**
- Consumes (Tasks 4 and 5): `Capture`, `CaptureParams`, `(*Service).Capture`, `(*Service).GetCapture`, `(*Service).ListCaptures`, `captureFromRow`; the test helpers `captureText`, `newFileService`, `storedFiles`, `exists`, `pdf`.
- Consumes (plans 2 and 3, package `service`): `RecordTypes`, `Get`, `Update`, `Delete`, `asRecord`, `verifyRecord` in `review.go`, `DeleteParams{Actor Source; Confirmed bool}`, `(*Service).deleteRecord(ctx, table string, id int64, p DeleteParams, remove func(q *store.Queries) (any, error)) error`, and the test `TestEveryRecordTypeHasEveryEntryPoint`, which runs for each type in `RecordTypes`.
- Produces (package `service`):
  - `RecordTypes` is `{"item", "journal", "todo", "habit_log", "source"}`.
  - `(*Service).DeleteCapture(ctx, id int64, p DeleteParams) error`
  - `Get` returns a `Capture` for the type `source`. `Update` refuses the type `source` with a `ValidationError` on the field `type`. `Verify` accepts a source and changes nothing.
  - sqlc: `SoftDeleteSource`

- [ ] **Step 1: Write the failing tests**

Create `internal/service/capture_change_test.go`:

```go
package service

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestDeleteCaptureKeepsTheRowAndTheFile(t *testing.T) {
	s, pool := newFileService(t)
	ctx := context.Background()
	added, err := s.Capture(ctx, CaptureParams{Upload: bytes.NewReader(pdf), FileName: "policy.pdf", Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	// An agent deletes only after Gautam said yes.
	err = s.DeleteCapture(ctx, added.ID, DeleteParams{Actor: SourceAgent})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "confirmed" {
		t.Fatalf("delete by an agent without a yes: error = %v, want a ValidationError on confirmed", err)
	}

	if err := s.DeleteCapture(ctx, added.ID, DeleteParams{Actor: SourceAgent, Confirmed: true}); err != nil {
		t.Fatalf("DeleteCapture: %v", err)
	}
	if _, err := s.GetCapture(ctx, added.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetCapture after the delete: error = %v, want ErrNotFound", err)
	}
	if n := countRows(t, pool, "sources"); n != 1 {
		t.Errorf("sources has %d rows, want 1: a delete keeps the row", n)
	}
	// Only the admin pages remove a record for good, so the file stays.
	if !exists(filepath.Join(s.files.Dir, added.FilePath)) {
		t.Error("the stored file is gone after a soft delete")
	}
	events := allEvents(t, pool)
	if e := events[len(events)-1]; e.Action != "delete" || e.RecordType != "sources" || e.RecordID != added.ID || e.Source != "agent" || e.Old["title"] != "policy.pdf" || e.New != nil {
		t.Errorf("event = %+v, want a delete of the source with the old row", e)
	}
	if err := s.DeleteCapture(ctx, added.ID, DeleteParams{Actor: SourceManual}); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: error = %v, want ErrNotFound", err)
	}
	if all, _ := s.ListCaptures(ctx, 0); len(all) != 0 {
		t.Errorf("ListCaptures = %+v, want no source", all)
	}
}

// A deleted source does not block a new capture of the same content.
func TestCaptureAfterADeleteStoresANewSource(t *testing.T) {
	s, pool := newFileService(t)
	ctx := context.Background()

	text := captureText(t, s, "a note")
	file, err := s.Capture(ctx, CaptureParams{Upload: bytes.NewReader(pdf), Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	for _, id := range []int64{text.ID, file.ID} {
		if err := s.DeleteCapture(ctx, id, DeleteParams{Actor: SourceManual}); err != nil {
			t.Fatalf("DeleteCapture(%d): %v", id, err)
		}
	}

	textAgain := captureText(t, s, "a note")
	fileAgain, err := s.Capture(ctx, CaptureParams{Upload: bytes.NewReader(pdf), Source: SourceManual})
	if err != nil {
		t.Fatalf("second Capture of the file: %v", err)
	}
	if textAgain.ID == text.ID || fileAgain.ID == file.ID {
		t.Errorf("new IDs = %d, %d, want IDs that differ from %d, %d", textAgain.ID, fileAgain.ID, text.ID, file.ID)
	}
	if fileAgain.FilePath != file.FilePath {
		t.Errorf("file_path = %q, want %q: the same bytes are one stored file", fileAgain.FilePath, file.FilePath)
	}
	if n := countRows(t, pool, "sources"); n != 4 {
		t.Errorf("sources has %d rows, want 4 (two deleted, two live)", n)
	}
	if files := storedFiles(t, s); len(files) != 1 {
		t.Errorf("files in the file store = %v, want 1", files)
	}
}

func TestRecordFunctionsWorkOnASource(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	added := captureText(t, s, "a note")

	got, err := s.Get(ctx, "source", added.ID)
	if capture, ok := got.(Capture); err != nil || !ok || capture.ID != added.ID || capture.Content != "a note" {
		t.Fatalf("Get = %#v, %v, want the source", got, err)
	}

	// A capture is verbatim, so it has no field to correct.
	_, err = s.Update(ctx, "source", added.ID, UpdateParams{Fields: map[string]any{"content": "a new note"}, Actor: SourceManual})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "type" {
		t.Errorf("Update: error = %v, want a ValidationError on type", err)
	}

	// A source is verified from the start: there is nothing to approve.
	n, err := s.Verify(ctx, []RecordRef{{"source", added.ID}}, SourceManual)
	if err != nil || n != 0 {
		t.Errorf("Verify = %d, %v, want 0 and no error", n, err)
	}
	if queue, _ := s.ReviewQueue(ctx, 0); len(queue) != 0 {
		t.Errorf("review queue = %+v, want it empty", queue)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events = %d, want only the create event", n)
	}

	if err := s.Delete(ctx, "source", added.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "source", added.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: error = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run 'Capture|Source|RecordType'"
```
Expected: the run fails. The output contains `DeleteCapture undefined`.

- [ ] **Step 3: Write the query**

In `queries/sources.sql`, replace this:

```sql
LIMIT $1;
```

With this:

```sql
LIMIT $1;

-- name: SoftDeleteSource :one
UPDATE sources
SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;
```

- [ ] **Step 4: Write the delete**

Create `internal/service/capture_change.go`:

```go
package service

import (
	"context"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// DeleteCapture marks one source as deleted and appends a delete event that
// holds the row, in one transaction. The file of a file capture stays in
// the file store: only the admin pages remove a record for good.
func (s *Service) DeleteCapture(ctx context.Context, id int64, p DeleteParams) error {
	return s.deleteRecord(ctx, "sources", id, p, func(q *store.Queries) (any, error) {
		row, err := q.SoftDeleteSource(ctx, id)
		if err != nil {
			return nil, err
		}
		return captureFromRow(row), nil
	})
}
```

- [ ] **Step 5: Add the type to the record functions**

In `internal/service/records.go`, replace this:

```go
// tool gets the type as input and calls the functions in this file.
var RecordTypes = []string{"item", "journal", "todo", "habit_log"}
```

With this:

```go
// tool gets the type as input and calls the functions in this file.
var RecordTypes = []string{"item", "journal", "todo", "habit_log", "source"}
```

In `internal/service/records.go`, replace this:

```go
// the type item, a JournalEntry for the type journal, a Todo for the type
// todo, a HabitLog for the type habit_log.
func (s *Service) Get(ctx context.Context, recordType string, id int64) (any, error) {
```

With this:

```go
// the type item, a JournalEntry for the type journal, a Todo for the type
// todo, a HabitLog for the type habit_log, a Capture for the type source.
func (s *Service) Get(ctx context.Context, recordType string, id int64) (any, error) {
```

In `internal/service/records.go`, replace this:

```go
		return asRecord(s.GetHabitLog(ctx, id))
	}
```

With this:

```go
		return asRecord(s.GetHabitLog(ctx, id))
	case "source":
		return asRecord(s.GetCapture(ctx, id))
	}
```

In `internal/service/records.go`, replace this:

```go
		return nil, invalid("type", "a habit_log has no field that you can change: delete the log, then log the habit for the correct day")
	}
```

With this:

```go
		return nil, invalid("type", "a habit_log has no field that you can change: delete the log, then log the habit for the correct day")
	case "source":
		// A source holds what came in. A correction is a new capture.
		return nil, invalid("type", "a source has no field that you can change: delete the source, then capture the correct text, link or file")
	}
```

In `internal/service/records.go`, replace this:

```go
		return s.DeleteHabitLog(ctx, id, p)
	}
```

With this:

```go
		return s.DeleteHabitLog(ctx, id, p)
	case "source":
		return s.DeleteCapture(ctx, id, p)
	}
```

- [ ] **Step 6: Add the type to `Verify`**

A source is verified from the start. The case reads the source, so that `Verify` reports a source that does not exist.

In `internal/service/review.go`, replace this:

```go
		return "habit_logs", before, after, err
	}
```

With this:

```go
		return "habit_logs", before, after, err
	case "source":
		// A source is verified from the start, so there is nothing to
		// approve. The read finds a source that does not exist.
		_, err = q.GetSource(ctx, ref.ID)
		return "sources", nil, nil, err
	}
```

- [ ] **Step 7: Run the tests of the service to verify that they pass**

Run the full package: `TestEveryRecordTypeHasEveryEntryPoint` checks each entry point for the new type.

Run:
```bash
make test PKG=./internal/service
```
Expected: the run passes. The last line starts with `ok`.

- [ ] **Step 8: Commit**

Run:
```bash
git add queries/sources.sql internal/service/capture_change.go internal/service/capture_change_test.go internal/service/records.go internal/service/review.go
git commit -m "feat(service): make source a record type"
```

---

### Task 7: Service: a record names its source

**Files:**
- Modify: `internal/service/provenance.go`, `internal/service/items.go`, `internal/service/journal.go`, `internal/service/todos.go`, `internal/service/habits.go`, `internal/service/habit_logs.go`
- Test: `internal/service/source_ref_test.go`

**Interfaces:**
- Consumes (Tasks 4 and 6): `captureText`, `(*Service).DeleteCapture`; sqlc `GetSource`.
- Consumes (plans 1 to 3, package `service`): `Provenance{Source Source; SourceRef *int64; Verified bool}`, which each `Add...Params` holds; `AddItem`, `AddJournalEntry`, `AddTodo`, `AddHabit`, `LogHabit`, each with a variable `q := store.New(tx)`; `UpdateItem`.
- Produces (package `service`): `(Provenance).checkSourceRef(ctx context.Context, q *store.Queries) error`. It returns a `ValidationError` on the field `source_ref` when `SourceRef` names no live source. Each of the five functions calls it in its transaction.

- [ ] **Step 1: Write the failing tests**

Create `internal/service/source_ref_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"
)

// addWithSourceRef calls each function that takes a source_ref. It returns
// the name of each function with the source_ref of the stored row, or the
// error.
func addWithSourceRef(s *Service, ref int64) map[string]func() (*int64, error) {
	ctx := context.Background()
	p := Provenance{Source: SourceAgent, SourceRef: &ref}
	return map[string]func() (*int64, error){
		"AddItem": func() (*int64, error) {
			item, err := s.AddItem(ctx, AddItemParams{Kind: "book", Title: "Dune", Provenance: p})
			return item.SourceRef, err
		},
		"AddJournalEntry": func() (*int64, error) {
			entry, err := s.AddJournalEntry(ctx, AddJournalEntryParams{Body: "a day", Provenance: p})
			return entry.SourceRef, err
		},
		"AddTodo": func() (*int64, error) {
			todo, err := s.AddTodo(ctx, AddTodoParams{Title: "Renew the policy", Provenance: p})
			return todo.SourceRef, err
		},
		"AddHabit": func() (*int64, error) {
			habit, err := s.AddHabit(ctx, AddHabitParams{Name: "read", Provenance: p})
			return habit.SourceRef, err
		},
		"LogHabit": func() (*int64, error) {
			log, err := s.LogHabit(ctx, LogHabitParams{Habit: "gym", Provenance: p})
			return log.SourceRef, err
		},
	}
}

func TestARecordCanNameTheSourceThatItCameFrom(t *testing.T) {
	s, _ := newTestService(t)
	addHabit(t, s, "gym")
	source := captureText(t, s, "Read Dune. Renew the policy. Went to the gym.")

	for name, add := range addWithSourceRef(s, source.ID) {
		ref, err := add()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if ref == nil || *ref != source.ID {
			t.Errorf("%s: source_ref = %v, want %d", name, ref, source.ID)
		}
	}
}

func TestARecordCannotNameASourceThatDoesNotExist(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	addHabit(t, s, "gym")
	deleted := captureText(t, s, "a note")
	if err := s.DeleteCapture(ctx, deleted.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("DeleteCapture: %v", err)
	}
	before := countRows(t, pool, "events")

	for what, ref := range map[string]int64{"an unknown source": 9999, "a deleted source": deleted.ID} {
		for name, add := range addWithSourceRef(s, ref) {
			_, err := add()
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != "source_ref" {
				t.Errorf("%s with %s: error = %v, want a ValidationError on source_ref", name, what, err)
			}
		}
	}
	for _, table := range []string{"catalog_items", "journal_entries", "todos", "habit_logs"} {
		if n := countRows(t, pool, table); n != 0 {
			t.Errorf("%s has %d rows, want 0", table, n)
		}
	}
	if after := countRows(t, pool, "events"); after != before {
		t.Errorf("events = %d, want %d: a refused call appends no event", after, before)
	}
}

func TestAnUpdateKeepsTheSourceRef(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	source := captureText(t, s, "Read Dune.")
	item, err := s.AddItem(ctx, AddItemParams{Kind: "book", Title: "Dune", Provenance: Provenance{Source: SourceAgent, SourceRef: &source.ID}})
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	changed, err := s.UpdateItem(ctx, item.ID, UpdateItemParams{Rating: ptr(4.0), Actor: SourceManual})
	if err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}
	if changed.SourceRef == nil || *changed.SourceRef != source.ID {
		t.Errorf("source_ref after the update = %v, want %d", changed.SourceRef, source.ID)
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run 'Source'"
```
Expected: the run fails. The output contains `AddItem with an unknown source: error = add item: ERROR: insert or update on table "catalog_items" violates foreign key constraint` and `AddTodo with a deleted source: error = <nil>, want a ValidationError on source_ref`.

- [ ] **Step 3: Write the check**

The foreign key refuses an unknown ID too. Its error does not name the field, and it accepts a deleted source.

In `internal/service/provenance.go`, replace this:

```go
package service

// Source says how a record came into LifeOS.
type Source string
```

With this:

```go
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// Source says how a record came into LifeOS.
type Source string
```

In `internal/service/provenance.go`, replace this:

```go
	return invalid("source", "must be one of manual, import, connector, agent")
}
```

With this:

```go
	return invalid("source", "must be one of manual, import, connector, agent")
}

// checkSourceRef refuses a SourceRef that names no live source. Call it in
// the transaction of the write. The foreign key refuses an unknown ID too,
// but its error does not name the field, and it accepts a deleted source.
func (p Provenance) checkSourceRef(ctx context.Context, q *store.Queries) error {
	if p.SourceRef == nil {
		return nil
	}
	_, err := q.GetSource(ctx, *p.SourceRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return invalid("source_ref", "must be the ID of a source that exists")
	}
	if err != nil {
		return fmt.Errorf("check source_ref: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Call the check in each function that adds a record**

In `internal/service/items.go`, replace this:

```go
	q := store.New(tx)
```

With this:

```go
	q := store.New(tx)
	if err := p.checkSourceRef(ctx, q); err != nil {
		return Item{}, err
	}
```

In `internal/service/journal.go`, replace this:

```go
	q := store.New(tx)
```

With this:

```go
	q := store.New(tx)
	if err := p.checkSourceRef(ctx, q); err != nil {
		return JournalEntry{}, err
	}
```

In `internal/service/todos.go`, replace this:

```go
	q := store.New(tx)
```

With this:

```go
	q := store.New(tx)
	if err := p.checkSourceRef(ctx, q); err != nil {
		return Todo{}, err
	}
```

In `internal/service/habits.go`, replace this:

```go
	defer tx.Rollback(ctx)
	q := store.New(tx)

	stored, err := q.InsertHabit(ctx, store.InsertHabitParams{
```

With this:

```go
	defer tx.Rollback(ctx)
	q := store.New(tx)
	if err := p.checkSourceRef(ctx, q); err != nil {
		return Habit{}, err
	}

	stored, err := q.InsertHabit(ctx, store.InsertHabitParams{
```

In `internal/service/habit_logs.go`, replace this:

```go
	q := store.New(tx)
```

With this:

```go
	q := store.New(tx)
	if err := p.checkSourceRef(ctx, q); err != nil {
		return HabitLog{}, err
	}
```

- [ ] **Step 5: Run the tests of the service to verify that they pass**

Run:
```bash
make test PKG=./internal/service
```
Expected: the run passes. The last line starts with `ok`.

- [ ] **Step 6: Commit**

Run:
```bash
git add internal/service/provenance.go internal/service/items.go internal/service/journal.go internal/service/todos.go internal/service/habits.go internal/service/habit_logs.go internal/service/source_ref_test.go
git commit -m "feat(service): check the source_ref of a new record"
```

---

### Task 8: REST: the ingest routes and the source routes

**Files:**
- Create: `internal/api/sources.go`
- Modify: `internal/api/api.go` (one line), `internal/service/files.go` (`MaxFileBytes`)
- Test: `internal/api/sources_test.go`

**Interfaces:**
- Consumes (Tasks 4 to 6, package `service`): `Capture`, `CaptureParams{Text, URL, InboxPath string; Upload io.Reader; FileName, Title string; Source Source}`, `(*Service).Capture`, `(*Service).GetCapture`, `(*Service).DeleteCapture`, `Files`, `(*Service).WithFiles`, `DeleteParams`, `SourceManual`.
- Consumes (plans 1 to 3, package `api`): `Register(router chi.Router, svc *service.Service)`, `toHTTPError(where string, err error) error`, `idInput`. Test helpers in `api_test`: `newTestAPI(t) (http.Handler, *pgxpool.Pool)`, `do(t, h, method, path, body string) (int, map[string]any, string)`.
- Produces:
  - `(*Service).MaxFileBytes() int64`
  - `POST /api/ingest` with the JSON body `{"text", "url", "title"}`: 200 and the source
  - `POST /api/ingest/file?name=&title=` with the file as the body: 200 and the source; 413 for a body over the limit
  - `GET /api/sources/{id}`: 200 and the source. `DELETE /api/sources/{id}`: 204
  - `registerSources(api huma.API, svc *service.Service)`
  - Test helpers in `api_test`: `newFileAPI(t) (h http.Handler, filesDir string)`, `upload(t, h, path, contentType string, content []byte) (int, string)`, the variable `pdf`, the constant `maxUploadBytes` (4096)

- [ ] **Step 1: Write the failing tests**

Create `internal/api/sources_test.go`:

```go
package api_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/Gautam-J/lifeos-core/internal/api"
	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

// maxUploadBytes is the file limit of these tests.
const maxUploadBytes = 4096

var pdf = []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\n")

// newFileAPI returns the API of a service that has a file store. filesDir
// is the folder of the file store.
func newFileAPI(t *testing.T) (h http.Handler, filesDir string) {
	t.Helper()
	filesDir = t.TempDir()
	svc := service.New(testdb.New(t)).WithFiles(service.Files{Dir: filesDir, Inbox: t.TempDir(), MaxBytes: maxUploadBytes})
	router := chi.NewRouter()
	api.Register(router, svc)
	return router, filesDir
}

// upload sends the bytes of one file as the body of a request.
func upload(t *testing.T, h http.Handler, path, contentType string, content []byte) (int, string) {
	t.Helper()
	req := httptest.NewRequest("POST", path, bytes.NewReader(content))
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestIngestATextThenReadIt(t *testing.T) {
	h, _ := newTestAPI(t)

	status, added, raw := do(t, h, "POST", "/api/ingest", `{"text":"  Renewal on 12 March.\n","title":"Insurance"}`)
	if status != http.StatusOK {
		t.Fatalf("POST status = %d, want 200\n%s", status, raw)
	}
	if added["kind"] != "text" || added["content"] != "  Renewal on 12 March.\n" || added["title"] != "Insurance" {
		t.Errorf("source = %s, want the text exactly as it was sent", raw)
	}
	if added["source"] != "manual" || added["verified"] != true {
		t.Errorf("source, verified = %v, %v, want manual, true", added["source"], added["verified"])
	}

	path := fmt.Sprintf("/api/sources/%d", int64(added["id"].(float64)))
	status, got, raw := do(t, h, "GET", path, "")
	if status != http.StatusOK || got["content"] != added["content"] || got["sha256"] != added["sha256"] {
		t.Errorf("GET: status = %d, source = %s", status, raw)
	}

	// The same text a second time is the same source.
	status, again, raw := do(t, h, "POST", "/api/ingest", `{"text":"  Renewal on 12 March.\n"}`)
	if status != http.StatusOK || again["id"] != added["id"] {
		t.Errorf("second POST: status = %d, source = %s, want the first source", status, raw)
	}
}

func TestIngestALink(t *testing.T) {
	h, _ := newTestAPI(t)

	status, added, raw := do(t, h, "POST", "/api/ingest", `{"url":"https://example.com/article"}`)
	if status != http.StatusOK || added["kind"] != "link" || added["url"] != "https://example.com/article" {
		t.Errorf("status = %d, source = %s", status, raw)
	}
	if _, hasContent := added["content"]; hasContent {
		t.Errorf("a link has content: %s", raw)
	}
}

func TestIngestRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantInBody string
	}{
		{"nothing to capture", `{}`, 422, "body.text"},
		{"a text and a link", `{"text":"a note","url":"https://example.com"}`, 422, "body.text"},
		{"a text of spaces", `{"text":"   "}`, 422, "body.text"},
		{"a card number", `{"text":"card 4111 1111 1111 1111"}`, 422, "a card number"},
		{"a link with no scheme", `{"url":"example.com"}`, 422, "body.url"},
		{"a title that is too long", `{"text":"a note","title":"` + strings.Repeat("a", 301) + `"}`, 422, "body.title"},
		{"a field that a caller must not set", `{"text":"a note","verified":false}`, 422, "verified"},
		{"a path on the Pi", `{"path":"/etc/passwd"}`, 422, "path"},
		{"text of the wrong type", `{"text":5}`, 422, "body.text"},
	}
	h, pool := newTestAPI(t)
	for _, tt := range tests {
		status, _, raw := do(t, h, "POST", "/api/ingest", tt.body)
		if status != tt.wantStatus {
			t.Errorf("%s: status = %d, want %d\n%s", tt.name, status, tt.wantStatus, raw)
		}
		if !strings.Contains(raw, tt.wantInBody) {
			t.Errorf("%s: body does not contain %q\n%s", tt.name, tt.wantInBody, raw)
		}
		if strings.Contains(raw, "4111 1111") {
			t.Errorf("%s: the answer repeats the card number\n%s", tt.name, raw)
		}
	}
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM sources").Scan(&n); err != nil || n != 0 {
		t.Errorf("sources = %d, %v, want 0", n, err)
	}
}

func TestIngestAFileThenReadIt(t *testing.T) {
	h, filesDir := newFileAPI(t)

	status, raw := upload(t, h, "/api/ingest/file?name=policy.pdf", "application/pdf", pdf)
	if status != http.StatusOK {
		t.Fatalf("POST status = %d, want 200\n%s", status, raw)
	}
	for _, want := range []string{`"kind":"file"`, `"title":"policy.pdf"`, `"media_type":"application/pdf"`, fmt.Sprintf(`"size_bytes":%d`, len(pdf)), `"source":"manual"`, `"verified":true`} {
		if !strings.Contains(raw, want) {
			t.Errorf("the answer does not contain %s\n%s", want, raw)
		}
	}
	_, got, _ := do(t, h, "GET", "/api/sources/1", "")
	stored, err := os.ReadFile(filepath.Join(filesDir, got["file_path"].(string)))
	if err != nil || !bytes.Equal(stored, pdf) {
		t.Errorf("the stored file: error = %v, same bytes = %v", err, bytes.Equal(stored, pdf))
	}

	// A title and a different content type. The service finds the type in
	// the bytes; it does not use the header.
	status, raw = upload(t, h, "/api/ingest/file?name=scan.bin&title=Receipt", "text/plain", []byte("\x89PNG\r\n\x1a\n...."))
	if status != http.StatusOK || !strings.Contains(raw, `"title":"Receipt"`) || !strings.Contains(raw, `"media_type":"image/png"`) {
		t.Errorf("status = %d\n%s", status, raw)
	}
}

func TestIngestAFileRejectsBadRequests(t *testing.T) {
	h, filesDir := newFileAPI(t)

	tests := []struct {
		name       string
		content    []byte
		wantStatus int
		wantInBody string
	}{
		{"an empty file", nil, 400, "request body is required"},
		{"a file over the limit", bytes.Repeat([]byte{1}, maxUploadBytes+1), 413, ""},
		{"a text file with a card number", []byte("card 4111 1111 1111 1111"), 422, "a card number"},
	}
	for _, tt := range tests {
		status, raw := upload(t, h, "/api/ingest/file?name=a.bin", "application/octet-stream", tt.content)
		if status != tt.wantStatus {
			t.Errorf("%s: status = %d, want %d\n%s", tt.name, status, tt.wantStatus, raw)
		}
		if !strings.Contains(raw, tt.wantInBody) {
			t.Errorf("%s: body does not contain %q\n%s", tt.name, tt.wantInBody, raw)
		}
	}
	// The largest file that the limit permits.
	if status, raw := upload(t, h, "/api/ingest/file", "application/octet-stream", bytes.Repeat([]byte{1}, maxUploadBytes)); status != http.StatusOK {
		t.Errorf("a file of the maximum size: status = %d\n%s", status, raw)
	}
	entries, err := os.ReadDir(filesDir)
	if err != nil || len(entries) != 1 {
		t.Errorf("entries in the file store = %d, %v, want only the folder of the one stored file", len(entries), err)
	}
}

func TestReadAndDeleteASource(t *testing.T) {
	h, pool := newTestAPI(t)
	_, added, _ := do(t, h, "POST", "/api/ingest", `{"text":"a note"}`)
	path := fmt.Sprintf("/api/sources/%d", int64(added["id"].(float64)))

	if status, _, raw := do(t, h, "DELETE", path, ""); status != http.StatusNoContent || raw != "" {
		t.Fatalf("DELETE: status = %d, body = %q, want 204 and no body", status, raw)
	}
	for _, method := range []string{"GET", "DELETE"} {
		if status, _, _ := do(t, h, method, path, ""); status != http.StatusNotFound {
			t.Errorf("%s after the delete: status = %d, want 404", method, status)
		}
	}
	if status, _, _ := do(t, h, "GET", "/api/sources/9999", ""); status != http.StatusNotFound {
		t.Errorf("GET of an unknown source: status = %d, want 404", status)
	}
	if status, _, _ := do(t, h, "PATCH", path, `{"content":"x"}`); status != http.StatusMethodNotAllowed {
		t.Errorf("PATCH: status = %d, want 405: a source has no field to change", status)
	}
	// The delete is manual and has its event.
	var source string
	err := pool.QueryRow(t.Context(), "SELECT source FROM events WHERE action = 'delete' AND record_type = 'sources'").Scan(&source)
	if err != nil || source != "manual" {
		t.Errorf("delete event: source = %q, %v, want manual", source, err)
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/api ARGS="-run 'Ingest|Source'"
```
Expected: the run fails. The output contains `POST /api/ingest: body is not a JSON object`.

The route does not exist yet, so the answer is the text `404 page not found` and not JSON.

- [ ] **Step 3: Give the limit of a file to the adapter**

In `internal/service/files.go`, replace this:

```go
	return s
}
```

With this:

```go
	return s
}

// MaxFileBytes returns the largest file that Capture accepts.
func (s *Service) MaxFileBytes() int64 {
	return s.files.MaxBytes
}
```

- [ ] **Step 4: Write the routes**

Huma answers 413 when the body has the number of bytes in `MaxBodyBytes` or more. So the limit of the route is one byte above the largest file.

Create `internal/api/sources.go`:

```go
package api

import (
	"bytes"
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type ingestInput struct {
	Body struct {
		Text  string `json:"text,omitempty" doc:"A text. It is stored exactly as it is written."`
		URL   string `json:"url,omitempty" doc:"A link that starts with http:// or https://. LifeOS stores the link; it does not get the page."`
		Title string `json:"title,omitempty" doc:"A label for the capture"`
	}
}

type ingestFileInput struct {
	Name  string `query:"name" doc:"The name of the file, such as policy.pdf"`
	Title string `query:"title" doc:"A label for the capture. The default is the name of the file."`
	// RawBody is the file. The service finds the type of the file in its
	// bytes, so the Content-Type header can have any value.
	RawBody []byte `contentType:"application/octet-stream"`
}

type sourceOutput struct {
	Body service.Capture
}

func registerSources(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "ingest",
		Method:      http.MethodPost,
		Path:        "/api/ingest",
		Summary:     "Capture a text or a link",
		Description: "Stores the text or the link as a source. Give one of text and url. The same content a second time returns the source that exists.",
	}, func(ctx context.Context, in *ingestInput) (*sourceOutput, error) {
		// A request to the REST API comes from Gautam's own devices.
		capture, err := svc.Capture(ctx, service.CaptureParams{
			Text:   in.Body.Text,
			URL:    in.Body.URL,
			Title:  in.Body.Title,
			Source: service.SourceManual,
		})
		if err != nil {
			return nil, toHTTPError("body", err)
		}
		return &sourceOutput{Body: capture}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ingest-file",
		Method:      http.MethodPost,
		Path:        "/api/ingest/file",
		Summary:     "Capture a file",
		Description: "Stores the body of the request as a file in the file store. The same file a second time returns the source that exists.",
		// Huma answers 413 for a body of this size or more, so the limit
		// is one byte above the largest file.
		MaxBodyBytes: svc.MaxFileBytes() + 1,
	}, func(ctx context.Context, in *ingestFileInput) (*sourceOutput, error) {
		capture, err := svc.Capture(ctx, service.CaptureParams{
			Upload:   bytes.NewReader(in.RawBody),
			FileName: in.Name,
			Title:    in.Title,
			Source:   service.SourceManual,
		})
		if err != nil {
			return nil, toHTTPError("body", err)
		}
		return &sourceOutput{Body: capture}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-source",
		Method:      http.MethodGet,
		Path:        "/api/sources/{id}",
		Summary:     "Read one source",
	}, func(ctx context.Context, in *idInput) (*sourceOutput, error) {
		capture, err := svc.GetCapture(ctx, in.ID)
		if err != nil {
			return nil, toHTTPError("path", err)
		}
		return &sourceOutput{Body: capture}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-source",
		Method:        http.MethodDelete,
		Path:          "/api/sources/{id}",
		Summary:       "Delete a source",
		Description:   "Marks the source as deleted. The row and the file stay.",
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *idInput) (*struct{}, error) {
		if err := svc.DeleteCapture(ctx, in.ID, service.DeleteParams{Actor: service.SourceManual}); err != nil {
			return nil, toHTTPError("path", err)
		}
		return nil, nil
	})
}
```

- [ ] **Step 5: Register the routes**

In `internal/api/api.go`, replace this:

```go
	registerHabits(api, svc)
}
```

With this:

```go
	registerHabits(api, svc)
	registerSources(api, svc)
}
```

- [ ] **Step 6: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./internal/api ARGS="-run 'Ingest|Source'"
```
Expected: the run passes. The last line starts with `ok`.

- [ ] **Step 7: Commit**

Run:
```bash
git add internal/api/sources.go internal/api/sources_test.go internal/api/api.go internal/service/files.go
git commit -m "feat(api): add the ingest and source routes"
```

---

### Task 9: MCP: the tool `capture` and the argument `source_ref`

**Files:**
- Create: `internal/mcp/capture.go`
- Modify: `internal/mcp/server.go`, `internal/mcp/records.go`, `internal/mcp/items.go`, `internal/mcp/todos.go`, `internal/mcp/habits.go`
- Test: `internal/mcp/capture_test.go`, `internal/mcp/items_test.go` (the helper `connectTo`)

**Interfaces:**
- Consumes (Tasks 4 to 7, package `service`): `Capture`, `CaptureParams`, `(*Service).Capture`, `Files`, `(*Service).WithFiles`, `Provenance.SourceRef`, the record type `source` in `Get`, `Update` and `Delete`.
- Consumes (plans 1 to 3, package `mcp`): `NewServer(svc *service.Service) *sdk.Server`, `toToolError(err error) error`, the tools `add_item`, `todo`, `log_habit`, `get`, `update`, `delete`. Test helpers in `mcp_test`: `connect(t) (*sdk.ClientSession, *pgxpool.Pool)`, `call`, `text`, `structured`, `addHabit(t, pool, name)`.
- Produces:
  - The tool `capture` with the arguments `text`, `url`, `path`, `title`. It writes an `agent` source.
  - The argument `source_ref` on `add_item`, `todo` and `log_habit`.
  - The type `source` in the descriptions of `get`, `update` and `delete`.
  - `registerCapture(server *sdk.Server, svc *service.Service)`
  - Test helper in `mcp_test`: `connectTo(t, svc *service.Service) *sdk.ClientSession`. `connect` uses it.

- [ ] **Step 1: Write the failing tests**

`connect` gets a second form, `connectTo`, for a test that needs a service with a file store.

In `internal/mcp/items_test.go`, replace this:

```go
	t.Helper()
	ctx := context.Background()
	pool := testdb.New(t)
	server := lifeosmcp.NewServer(service.New(pool))
```

With this:

```go
	t.Helper()
	pool := testdb.New(t)
	return connectTo(t, service.New(pool)), pool
}

// connectTo starts the lifeos MCP server for svc in this process and
// returns a client session that talks to it.
func connectTo(t *testing.T, svc *service.Service) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	server := lifeosmcp.NewServer(svc)
```

In `internal/mcp/items_test.go`, replace this:

```go
	t.Cleanup(func() { session.Close() })
	return session, pool
}
```

With this:

```go
	t.Cleanup(func() { session.Close() })
	return session
}
```

In `internal/mcp/items_test.go`, replace this:

```go
	}
	for _, name := range []string{"add_item", "get"} {
		if !found[name] {
```

With this:

```go
	}
	for _, name := range []string{"add_item", "get", "capture"} {
		if !found[name] {
```

Create `internal/mcp/capture_test.go`:

```go
package mcp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

func TestCaptureATextThenGetAndDeleteIt(t *testing.T) {
	session, _ := connect(t)

	added := structured(t, call(t, session, "capture", map[string]any{"text": "  Renewal on 12 March.\n", "title": "Insurance"}))
	if added["kind"] != "text" || added["content"] != "  Renewal on 12 March.\n" || added["title"] != "Insurance" {
		t.Errorf("source = %v, want the text exactly as it was sent", added)
	}
	// A capture is what came in. No agent inferred it, so it is verified.
	if added["source"] != "agent" || added["verified"] != true {
		t.Errorf("source, verified = %v, %v, want agent, true", added["source"], added["verified"])
	}

	got := structured(t, call(t, session, "get", map[string]any{"type": "source", "id": added["id"]}))
	if got["content"] != added["content"] || got["sha256"] != added["sha256"] {
		t.Errorf("get = %v, want the source", got)
	}

	again := structured(t, call(t, session, "capture", map[string]any{"text": "  Renewal on 12 March.\n"}))
	if again["id"] != added["id"] {
		t.Errorf("second capture has the ID %v, want %v", again["id"], added["id"])
	}

	update := call(t, session, "update", map[string]any{"type": "source", "id": added["id"], "fields": map[string]any{"content": "x"}})
	if !update.IsError || !strings.Contains(text(update), "a source has no field that you can change") {
		t.Errorf("update of a source = %q, want it refused", text(update))
	}
	refused := call(t, session, "delete", map[string]any{"type": "source", "id": added["id"]})
	if !refused.IsError || !strings.Contains(text(refused), "confirmed") {
		t.Errorf("delete without confirmed = %q, want it refused", text(refused))
	}
	deleted := structured(t, call(t, session, "delete", map[string]any{"type": "source", "id": added["id"], "confirmed": true}))
	if deleted["deleted"] != true {
		t.Errorf("delete = %v", deleted)
	}
}

func TestCaptureALink(t *testing.T) {
	session, _ := connect(t)

	added := structured(t, call(t, session, "capture", map[string]any{"url": "https://example.com/article"}))
	if added["kind"] != "link" || added["url"] != "https://example.com/article" {
		t.Errorf("source = %v", added)
	}
}

func TestCaptureAFileFromTheInbox(t *testing.T) {
	inbox, files := t.TempDir(), t.TempDir()
	svc := service.New(testdb.New(t)).WithFiles(service.Files{Dir: files, Inbox: inbox, MaxBytes: 4096})
	session := connectTo(t, svc)
	pdf := []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\n")
	if err := os.WriteFile(filepath.Join(inbox, "policy.pdf"), pdf, 0o600); err != nil {
		t.Fatal(err)
	}

	added := structured(t, call(t, session, "capture", map[string]any{"path": "policy.pdf"}))
	if added["kind"] != "file" || added["title"] != "policy.pdf" || added["media_type"] != "application/pdf" {
		t.Errorf("source = %v", added)
	}
	if _, err := os.Stat(filepath.Join(files, added["file_path"].(string))); err != nil {
		t.Errorf("the file is not in the file store: %v", err)
	}
	if _, err := os.Stat(filepath.Join(inbox, "policy.pdf")); err == nil {
		t.Error("the file is still in the inbox")
	}

	outside := call(t, session, "capture", map[string]any{"path": "/etc/hostname"})
	if !outside.IsError || !strings.Contains(text(outside), "path: must name a file in the inbox folder") {
		t.Errorf("capture of a file outside the inbox = %q, want it refused", text(outside))
	}
}

func TestCaptureRejectsBadInput(t *testing.T) {
	session, pool := connect(t)

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"nothing to capture", map[string]any{}, "text: give one of these"},
		{"a text and a link", map[string]any{"text": "a note", "url": "https://example.com"}, "text: give one of these"},
		{"a card number", map[string]any{"text": "card 4111 1111 1111 1111"}, "text: must not hold a card number"},
		{"a link with no scheme", map[string]any{"url": "example.com"}, "url: must start with http:// or https://"},
		{"an argument that the tool does not have", map[string]any{"text": "a note", "dictated": true}, "dictated"},
	}
	for _, tt := range tests {
		result := call(t, session, "capture", tt.args)
		if !result.IsError || !strings.Contains(text(result), tt.want) {
			t.Errorf("%s: result = %q, want an error with %q", tt.name, text(result), tt.want)
		}
		if strings.Contains(text(result), "4111 1111") {
			t.Errorf("%s: the error repeats the card number: %q", tt.name, text(result))
		}
	}
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM sources").Scan(&n); err != nil || n != 0 {
		t.Errorf("sources = %d, %v, want 0", n, err)
	}
}

func TestARecordNamesTheSourceThatItComesFrom(t *testing.T) {
	session, pool := connect(t)
	addHabit(t, pool, "gym")
	source := structured(t, call(t, session, "capture", map[string]any{"text": "Read Dune. Renew the policy. Went to the gym."}))

	item := structured(t, call(t, session, "add_item", map[string]any{"kind": "book", "title": "Dune", "source_ref": source["id"]}))
	todo := structured(t, call(t, session, "todo", map[string]any{"title": "Renew the policy", "source_ref": source["id"]}))
	log := structured(t, call(t, session, "log_habit", map[string]any{"habit": "gym", "source_ref": source["id"]}))
	for name, record := range map[string]map[string]any{"add_item": item, "todo": todo, "log_habit": log} {
		if record["source_ref"] != source["id"] || record["verified"] != false {
			t.Errorf("%s: source_ref, verified = %v, %v, want %v, false", name, record["source_ref"], record["verified"], source["id"])
		}
	}

	unknown := call(t, session, "todo", map[string]any{"title": "Call the agent", "source_ref": 9999})
	if !unknown.IsError || !strings.Contains(text(unknown), "source_ref: must be the ID of a source that exists") {
		t.Errorf("a todo with an unknown source_ref = %q, want it refused", text(unknown))
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/mcp ARGS="-run 'Capture|Source|ListsTheTools'"
```
Expected: the run fails. The output contains `tool capture is not registered`.

- [ ] **Step 3: Write the tool**

The description tells the model what a source is for, and what it is not for.

Create `internal/mcp/capture.go`:

```go
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type captureInput struct {
	Text  string `json:"text,omitempty" jsonschema:"A text to keep, word for word. Do not correct, shorten or summarize it."`
	URL   string `json:"url,omitempty" jsonschema:"A link that starts with http:// or https://. LifeOS stores the link; it does not get the page."`
	Path  string `json:"path,omitempty" jsonschema:"The path of a file in the inbox folder on the Pi. LifeOS moves the file into its file store."`
	Title string `json:"title,omitempty" jsonschema:"A short label for the capture. A file with no title gets its file name."`
}

func registerCapture(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "capture",
		Description: "Keep a text, a link or a file exactly as it is, as a source. Give one of text, url and path. Use it for material that the user wants to keep, such as an article, a note or a document. Do not use it for a journal entry (use journal) or for one fact (use add_item, todo or log_habit). Never send a password, a card number or an ID number: LifeOS refuses them. The same content a second time returns the source that exists. When you take a record from a source, give the ID of the source as source_ref to add_item, todo or log_habit. Returns the stored source with its ID.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in captureInput) (*sdk.CallToolResult, service.Capture, error) {
		capture, err := svc.Capture(ctx, service.CaptureParams{
			Text:      in.Text,
			URL:       in.URL,
			InboxPath: in.Path,
			Title:     in.Title,
			Source:    service.SourceAgent,
		})
		if err != nil {
			return nil, service.Capture{}, toToolError(err)
		}
		return nil, capture, nil
	})
}
```

- [ ] **Step 4: Register the tool**

In `internal/mcp/server.go`, replace this:

```go
	registerHabits(server, svc)
	return server
```

With this:

```go
	registerHabits(server, svc)
	registerCapture(server, svc)
	return server
```

- [ ] **Step 5: Name the type `source` in the record tools**

In `internal/mcp/records.go`, replace this:

```go
type getInput struct {
	Type string `json:"type" jsonschema:"Record type. One of: item, journal, todo, habit_log"`
	ID   int64  `json:"id" jsonschema:"Record ID"`
```

With this:

```go
type getInput struct {
	Type string `json:"type" jsonschema:"Record type. One of: item, journal, todo, habit_log, source"`
	ID   int64  `json:"id" jsonschema:"Record ID"`
```

In `internal/mcp/records.go`, replace this:

```go
type updateInput struct {
	Type     string         `json:"type" jsonschema:"Record type. One of: item, journal, todo, habit_log"`
	ID       int64          `json:"id" jsonschema:"Record ID"`
	Fields   map[string]any `json:"fields,omitempty" jsonschema:"The fields to change, with their new values. For an item: kind, title, rating, occurred_on, attrs. For a journal: body, entry_date. For a todo: title, status (open, done or dropped), due_on, project. A habit_log has no field to change: delete it, then log the habit again. Leave out a field to keep its value. attrs replaces every attr."`
	Clear    []string       `json:"clear,omitempty" jsonschema:"Names of optional fields to empty. For an item: rating, occurred_on. For a todo: due_on, project. A journal has none."`
```

With this:

```go
type updateInput struct {
	Type     string         `json:"type" jsonschema:"Record type. One of: item, journal, todo, habit_log, source"`
	ID       int64          `json:"id" jsonschema:"Record ID"`
	Fields   map[string]any `json:"fields,omitempty" jsonschema:"The fields to change, with their new values. For an item: kind, title, rating, occurred_on, attrs. For a journal: body, entry_date. For a todo: title, status (open, done or dropped), due_on, project. A habit_log has no field to change: delete it, then log the habit again. A source has no field to change: delete it, then capture again. Leave out a field to keep its value. attrs replaces every attr."`
	Clear    []string       `json:"clear,omitempty" jsonschema:"Names of optional fields to empty. For an item: rating, occurred_on. For a todo: due_on, project. A journal has none."`
```

In `internal/mcp/records.go`, replace this:

```go
type deleteInput struct {
	Type      string `json:"type" jsonschema:"Record type. One of: item, journal, todo, habit_log"`
	ID        int64  `json:"id" jsonschema:"Record ID"`
```

With this:

```go
type deleteInput struct {
	Type      string `json:"type" jsonschema:"Record type. One of: item, journal, todo, habit_log, source"`
	ID        int64  `json:"id" jsonschema:"Record ID"`
```

- [ ] **Step 6: Add `source_ref` to the three tools that add a record**

In `internal/mcp/items.go`, replace this:

```go
	Attrs      map[string]any `json:"attrs,omitempty" jsonschema:"More fields for this kind, such as author, director or year"`
	Dictated   bool           `json:"dictated,omitempty" jsonschema:"true only when the user stated this item directly. false when you inferred it from a source; the row then waits in the review queue."`
```

With this:

```go
	Attrs      map[string]any `json:"attrs,omitempty" jsonschema:"More fields for this kind, such as author, director or year"`
	SourceRef  *int64         `json:"source_ref,omitempty" jsonschema:"The ID of the source that this item comes from, when you took the item from a capture"`
	Dictated   bool           `json:"dictated,omitempty" jsonschema:"true only when the user stated this item directly. false when you inferred it from a source; the row then waits in the review queue."`
```

In `internal/mcp/items.go`, replace this:

```go
			Attrs:      in.Attrs,
			Provenance: service.Provenance{Source: service.SourceAgent, Verified: in.Dictated},
		})
```

With this:

```go
			Attrs:      in.Attrs,
			Provenance: service.Provenance{Source: service.SourceAgent, SourceRef: in.SourceRef, Verified: in.Dictated},
		})
```

In `internal/mcp/todos.go`, replace this:

```go
type todoInput struct {
	Title    string `json:"title" jsonschema:"One action, in a few words"`
	DueOn    string `json:"due_on,omitempty" jsonschema:"The day that the todo is due, as YYYY-MM-DD in India Standard Time. Leave it out when the user gave no date."`
	Project  string `json:"project,omitempty" jsonschema:"The project of the todo, such as lifeos. Leave it out when it is not clear."`
	Dictated bool   `json:"dictated,omitempty" jsonschema:"true only when the user stated this todo directly. false when you propose it from a journal entry, a document or a different source; the todo then waits in the review queue."`
}
```

With this:

```go
type todoInput struct {
	Title     string `json:"title" jsonschema:"One action, in a few words"`
	DueOn     string `json:"due_on,omitempty" jsonschema:"The day that the todo is due, as YYYY-MM-DD in India Standard Time. Leave it out when the user gave no date."`
	Project   string `json:"project,omitempty" jsonschema:"The project of the todo, such as lifeos. Leave it out when it is not clear."`
	SourceRef *int64 `json:"source_ref,omitempty" jsonschema:"The ID of the source that this todo comes from, when you took the todo from a capture"`
	Dictated  bool   `json:"dictated,omitempty" jsonschema:"true only when the user stated this todo directly. false when you propose it from a journal entry, a document or a different source; the todo then waits in the review queue."`
}
```

In `internal/mcp/todos.go`, replace this:

```go
			Project:    in.Project,
			Provenance: service.Provenance{Source: service.SourceAgent, Verified: in.Dictated},
		})
```

With this:

```go
			Project:    in.Project,
			Provenance: service.Provenance{Source: service.SourceAgent, SourceRef: in.SourceRef, Verified: in.Dictated},
		})
```

In `internal/mcp/habits.go`, replace this:

```go
type logHabitInput struct {
	Habit    string `json:"habit" jsonschema:"The name of one of the user's habits, such as gym. If no habit has this name, the error lists the habits: ask the user which one they mean."`
	LoggedOn string `json:"logged_on,omitempty" jsonschema:"The day that the user did the habit, as YYYY-MM-DD in India Standard Time. Leave it out for today."`
	Dictated bool   `json:"dictated,omitempty" jsonschema:"true only when the user stated directly that they did the habit. false when you inferred it from a journal entry or a different source; the log then waits in the review queue."`
}
```

With this:

```go
type logHabitInput struct {
	Habit     string `json:"habit" jsonschema:"The name of one of the user's habits, such as gym. If no habit has this name, the error lists the habits: ask the user which one they mean."`
	LoggedOn  string `json:"logged_on,omitempty" jsonschema:"The day that the user did the habit, as YYYY-MM-DD in India Standard Time. Leave it out for today."`
	SourceRef *int64 `json:"source_ref,omitempty" jsonschema:"The ID of the source that this log comes from, when you took the log from a capture"`
	Dictated  bool   `json:"dictated,omitempty" jsonschema:"true only when the user stated directly that they did the habit. false when you inferred it from a journal entry or a different source; the log then waits in the review queue."`
}
```

In `internal/mcp/habits.go`, replace this:

```go
			LoggedOn:   in.LoggedOn,
			Provenance: service.Provenance{Source: service.SourceAgent, Verified: in.Dictated},
		})
```

With this:

```go
			LoggedOn:   in.LoggedOn,
			Provenance: service.Provenance{Source: service.SourceAgent, SourceRef: in.SourceRef, Verified: in.Dictated},
		})
```

- [ ] **Step 7: Run the tests of the package to verify that they pass**

Run:
```bash
make test PKG=./internal/mcp
```
Expected: the run passes. The last line starts with `ok`.

- [ ] **Step 8: Commit**

Run:
```bash
git add internal/mcp/capture.go internal/mcp/capture_test.go internal/mcp/items_test.go internal/mcp/server.go internal/mcp/records.go internal/mcp/items.go internal/mcp/todos.go internal/mcp/habits.go
git commit -m "feat(mcp): add the capture tool and the source_ref argument"
```

---

### Task 10: Admin: the sources page

**Files:**
- Create: `internal/admin/sources.templ`, `internal/admin/sources.go`
- Modify: `internal/admin/admin.go` (three routes), `internal/admin/items.templ` (one link)
- Test: `internal/admin/sources_test.go`

**Interfaces:**
- Consumes (Tasks 4 to 6, package `service`): `Capture`, `CaptureParams`, `(*Service).Capture`, `(*Service).ListCaptures`, `(*Service).DeleteCapture`, `(*Service).GetCapture`, `Files`, `(*Service).WithFiles`.
- Consumes (plans 1 to 3, package `admin`): `handlers` with the field `svc`, `render`, `page(title string)`, `internalError`, `listFailure`, `pathID(r) (int64, bool)`, `formText(r, name string) string`, which changes CR LF to LF. Test helpers in `admin_test`: `newTestAdmin(t)`, `get(h, path)`, `postForm(h, path, form)`.
- Produces:
  - `GET /admin/sources`, `POST /admin/sources` (the fields `text`, `url`, `title`), `POST /admin/sources/{id}/delete`
  - The link "Sources" in the navigation of each page
  - `preview(text string) string`, `fileSize(n int64) string`, `sourcePath(id int64) string`

- [ ] **Step 1: Write the failing tests**

Create `internal/admin/sources_test.go`:

```go
package admin_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

func TestSourcesPageShowsTheFormAndAnEmptyList(t *testing.T) {
	h, _ := newTestAdmin(t)

	rec := get(h, "/admin/sources")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`hx-post="/admin/sources"`, `<textarea name="text"`, `name="url"`, `name="title"`, "No sources yet.", `href="/admin/sources"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %q", want)
		}
	}
}

// A browser sends each line break of a textarea as CR LF. The stored text
// must have the line breaks that Gautam typed.
func TestAddSourceStoresTheTextOfTheForm(t *testing.T) {
	h, pool := newTestAdmin(t)

	rec := postForm(h, "/admin/sources", url.Values{"text": {"  Renewal on 12 March.\r\n\r\nCall first.  \r\n"}, "url": {""}, "title": {"Insurance"}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("HX-Trigger"); got != "source-added" {
		t.Errorf("HX-Trigger = %q, want source-added so that the form resets", got)
	}
	page := rec.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(page), `<div id="list">`) {
		t.Errorf("the response is not the #list fragment:\n%s", page)
	}
	for _, want := range []string{"Insurance", "Renewal on 12 March.", "text", "manual"} {
		if !strings.Contains(page, want) {
			t.Errorf("the list does not contain %q", want)
		}
	}

	var content, source string
	var verified bool
	if err := pool.QueryRow(t.Context(), "SELECT content, source, verified FROM sources").Scan(&content, &source, &verified); err != nil {
		t.Fatalf("read the row (want exactly one): %v", err)
	}
	if content != "  Renewal on 12 March.\n\nCall first.  \n" {
		t.Errorf("stored content = %q", content)
	}
	if source != "manual" || !verified {
		t.Errorf("source, verified = %q, %v, want manual, true", source, verified)
	}
}

func TestAddSourceStoresALink(t *testing.T) {
	h, pool := newTestAdmin(t)

	rec := postForm(h, "/admin/sources", url.Values{"text": {""}, "url": {"https://example.com/article"}})

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `href="https://example.com/article"`) {
		t.Errorf("status = %d, want 200 and the link in the list\n%s", rec.Code, rec.Body)
	}
	var kind string
	if err := pool.QueryRow(t.Context(), "SELECT kind FROM sources").Scan(&kind); err != nil || kind != "link" {
		t.Errorf("kind = %q, %v, want link", kind, err)
	}
}

func TestAddSourceShowsWhyTheServiceRefusedTheInput(t *testing.T) {
	tests := []struct {
		name     string
		form     url.Values
		wantText string
	}{
		{"nothing", url.Values{"text": {""}, "url": {""}}, "text: give one of these"},
		{"a text and a link", url.Values{"text": {"a note"}, "url": {"https://example.com"}}, "text: give one of these"},
		{"a card number", url.Values{"text": {"card 4111 1111 1111 1111"}}, "text: must not hold a card number"},
		{"a link with no scheme", url.Values{"url": {"example.com"}}, "url: must start with http:// or https://"},
	}
	h, pool := newTestAdmin(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postForm(h, "/admin/sources", tt.form)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 so that htmx swaps the list", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), tt.wantText) {
				t.Errorf("the response does not contain %q:\n%s", tt.wantText, rec.Body)
			}
			if rec.Header().Get("HX-Trigger") != "" {
				t.Error("HX-Trigger is set, so the form loses what Gautam typed")
			}
			if strings.Contains(rec.Body.String(), "4111 1111") {
				t.Error("the page repeats the card number")
			}
		})
	}
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM sources").Scan(&n); err != nil || n != 0 {
		t.Errorf("sources = %d, %v, want 0", n, err)
	}
}

// The list shows the start of a long text, and a file by its name.
func TestSourcesListShowsEachKind(t *testing.T) {
	h, pool := newTestAdmin(t)
	svc := service.New(pool).WithFiles(service.Files{Dir: t.TempDir(), Inbox: t.TempDir(), MaxBytes: 4096})
	long := "Start of the article. " + strings.Repeat("word ", 100) + "THE END"
	for _, p := range []service.CaptureParams{
		{Text: long, Source: service.SourceAgent},
		{Upload: bytes.NewReader([]byte("%PDF-1.7\n")), FileName: "policy.pdf", Source: service.SourceManual},
	} {
		if _, err := svc.Capture(t.Context(), p); err != nil {
			t.Fatalf("Capture: %v", err)
		}
	}

	page := get(h, "/admin/sources").Body.String()

	for _, want := range []string{"Start of the article.", "…", "agent", "policy.pdf", "application/pdf", "9 bytes"} {
		if !strings.Contains(page, want) {
			t.Errorf("the list does not contain %q", want)
		}
	}
	if strings.Contains(page, "THE END") {
		t.Error("the list shows the end of a long text")
	}
}

func TestDeleteSourceRemovesItFromTheList(t *testing.T) {
	h, pool := newTestAdmin(t)
	capture, err := service.New(pool).Capture(t.Context(), service.CaptureParams{Text: "a note to delete", Source: service.SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	path := fmt.Sprintf("/admin/sources/%d/delete", capture.ID)

	rec := postForm(h, path, nil)

	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "a note to delete") || !strings.Contains(rec.Body.String(), "No sources yet.") {
		t.Errorf("status = %d, want 200 and an empty list\n%s", rec.Code, rec.Body)
	}
	if _, err := service.New(pool).GetCapture(t.Context(), capture.ID); err != service.ErrNotFound {
		t.Errorf("GetCapture after the delete: error = %v, want ErrNotFound", err)
	}

	// A second click, or a click in a stale tab.
	rec = postForm(h, path, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "is not in the list") {
		t.Errorf("second delete: status = %d\n%s", rec.Code, rec.Body)
	}
	if rec := postForm(h, "/admin/sources/abc/delete", nil); rec.Code != http.StatusNotFound {
		t.Errorf("delete with a bad ID: status = %d, want 404", rec.Code)
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/admin ARGS="-run Source"
```
Expected: the run fails. The output contains `status = 404, want 200`.

- [ ] **Step 3: Write the page**

The page captures a text or a link. A file comes through the REST API or the `capture` tool.

Create `internal/admin/sources.templ`:

```templ
package admin

import (
	"strconv"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

templ sourcesPage(captures []service.Capture) {
	@page("Sources") {
		<h1>Sources</h1>
		<p class="muted">A source is a text or a link that LifeOS keeps exactly as it is. Fill in the text or the link, not the two.</p>
		<form hx-post="/admin/sources" hx-target="#list" hx-swap="outerHTML" hx-on:source-added="this.reset()">
			<label>
				Text
				<textarea name="text" cols="60" rows="6"></textarea>
			</label>
			<label>
				Link
				<input type="url" name="url" size="40" placeholder="https://"/>
			</label>
			<label>
				Title (optional)
				<input type="text" name="title" maxlength="300" size="30"/>
			</label>
			<button type="submit">Capture</button>
		</form>
		@sourcesList(captures, "")
	}
}

// sourcesList is the part of the page that a form post replaces.
templ sourcesList(captures []service.Capture, errorMessage string) {
	<div id="list">
		if errorMessage != "" {
			<p class="error" role="alert">{ errorMessage }</p>
		}
		if len(captures) == 0 {
			<p class="muted">No sources yet.</p>
		} else {
			<table>
				<thead>
					<tr>
						<th>ID</th>
						<th>Kind</th>
						<th>Title</th>
						<th>Content</th>
						<th>Source</th>
						<th>Captured (IST)</th>
						<th></th>
					</tr>
				</thead>
				<tbody>
					for _, capture := range captures {
						<tr>
							<td>{ strconv.FormatInt(capture.ID, 10) }</td>
							<td>{ capture.Kind }</td>
							<td>{ capture.Title }</td>
							<td>
								switch capture.Kind {
									case "link":
										<a href={ templ.URL(capture.URL) } rel="noreferrer">{ capture.URL }</a>
									case "file":
										{ capture.MediaType }, { fileSize(capture.SizeBytes) }
									default:
										{ preview(capture.Content) }
								}
							</td>
							<td>{ string(capture.Source) }</td>
							<td>{ capture.CreatedAt.In(service.IST).Format("2 Jan 2006 15:04") }</td>
							<td>
								<button
									hx-post={ sourcePath(capture.ID) + "/delete" }
									hx-target="#list"
									hx-swap="outerHTML"
									hx-confirm={ "Delete source " + strconv.FormatInt(capture.ID, 10) + "?" }
								>Delete</button>
							</td>
						</tr>
					}
				</tbody>
			</table>
		}
	</div>
}

func sourcePath(id int64) string {
	return "/admin/sources/" + strconv.FormatInt(id, 10)
}
```

- [ ] **Step 4: Write the handlers**

Create `internal/admin/sources.go`:

```go
package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// previewLength is the number of characters of a text that the list shows.
const previewLength = 200

// preview returns the start of text as one line.
func preview(text string) string {
	line := []rune(strings.Join(strings.Fields(text), " "))
	if len(line) <= previewLength {
		return string(line)
	}
	return strings.TrimSpace(string(line[:previewLength])) + "…"
}

// fileSize returns a number of bytes in words: "3 MB", "12 kB" or
// "9 bytes".
func fileSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%d MB", n>>20)
	case n >= 1<<10:
		return fmt.Sprintf("%d kB", n>>10)
	}
	return fmt.Sprintf("%d bytes", n)
}

func (h *handlers) sourcesPage(w http.ResponseWriter, r *http.Request) {
	captures, err := h.svc.ListCaptures(r.Context(), 0)
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, sourcesPage(captures))
}

// showSourcesList renders the list again, with message above it.
func (h *handlers) showSourcesList(w http.ResponseWriter, r *http.Request, message string) {
	captures, err := h.svc.ListCaptures(r.Context(), 0)
	if err != nil {
		listFailure(w, r, err, "The list did not load: internal error. Reload the page.")
		return
	}
	render(w, r, sourcesList(captures, message))
}

// addSource stores the text or the link from the form and renders the list
// again. A capture from the admin pages is one that Gautam made, so it is
// manual.
func (h *handlers) addSource(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	message := ""
	_, err := h.svc.Capture(r.Context(), service.CaptureParams{
		Text:   formText(r, "text"),
		URL:    r.PostForm.Get("url"),
		Title:  r.PostForm.Get("title"),
		Source: service.SourceManual,
	})
	var invalid *service.ValidationError
	switch {
	case err == nil:
		// htmx turns this header into an event; the form resets on it.
		w.Header().Set("HX-Trigger", "source-added")
	case errors.As(err, &invalid):
		message = invalid.Error()
	default:
		listFailure(w, r, err, "The source was not stored: internal error. Try again.")
		return
	}
	h.showSourcesList(w, r, message)
}

// deleteSource deletes one source and renders the list again.
func (h *handlers) deleteSource(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	message := ""
	err := h.svc.DeleteCapture(r.Context(), id, service.DeleteParams{Actor: service.SourceManual})
	switch {
	case err == nil:
	case errors.Is(err, service.ErrNotFound):
		message = fmt.Sprintf("Source %d is not in the list. It was possibly deleted before.", id)
	default:
		listFailure(w, r, err, "The source was not deleted: internal error. Try again.")
		return
	}
	h.showSourcesList(w, r, message)
}
```

- [ ] **Step 5: Add the routes and the link**

In `internal/admin/admin.go`, replace this:

```go
	router.Post("/admin/habits/logs/{id}/delete", h.deleteHabitLog)
	router.Get("/admin/review", h.reviewPage)
```

With this:

```go
	router.Post("/admin/habits/logs/{id}/delete", h.deleteHabitLog)
	router.Get("/admin/sources", h.sourcesPage)
	router.Post("/admin/sources", h.addSource)
	router.Post("/admin/sources/{id}/delete", h.deleteSource)
	router.Get("/admin/review", h.reviewPage)
```

In `internal/admin/items.templ`, replace this:

```templ
				<a href="/admin/habits">Habits</a>
				<a href="/admin/review">Review</a>
```

With this:

```templ
				<a href="/admin/habits">Habits</a>
				<a href="/admin/sources">Sources</a>
				<a href="/admin/review">Review</a>
```

- [ ] **Step 6: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./internal/admin ARGS="-run Source"
```
Expected: the run passes. The last line starts with `ok`.

- [ ] **Step 7: Commit**

`sources_templ.go` is generated. Do not add it.

Run:
```bash
git add internal/admin/sources.templ internal/admin/sources.go internal/admin/sources_test.go internal/admin/admin.go internal/admin/items.templ
git commit -m "feat(admin): add the sources page"
```

---

### Task 11: Service: search

**Files:**
- Create: `queries/search.sql`, `internal/service/search.go`
- Modify: `internal/service/fields.go` (`isText`), `internal/service/capture_change.go` (the summary of a source), `internal/service/todos_change.go` (the status in the summary of a todo)
- Test: `internal/service/search_test.go`

**Interfaces:**
- Consumes (Tasks 4 to 6): `Capture`, `captureFromRow`, `(*Service).Capture`; the test helpers `newFileService`, `testCard`.
- Consumes (plans 1 to 3, package `service`): `itemFromRow(store.CatalogItem) (Item, error)`, `journalEntryFromRow`, `todoFromRow`, the methods `summary() string` of `Item`, `JournalEntry` and `Todo`, `shorten(text string, max int) string`, `RecordRef{Type string; ID int64}`, `checkLimit`. Test helpers: `dune()`, `buyDomain()`, `addItem`, `addUnverified`, `addJournal`, `addTodo` (it gives each todo the project `lifeos` and the due day `2026-10-20`), `setStatus`.
- Produces (package `service`):
  - `SearchTypes` (`{"item", "journal", "todo", "source"}`), the constants `maxQueryLength` (200) and `maxQueryWords` (10)
  - `type SearchParams struct{ Query string; Types []string; Limit int }`
  - `type SearchHit struct{ Type string; ID int64; Summary string; Verified bool; CreatedAt time.Time }` with the JSON names `type`, `id`, `summary`, `verified`, `created_at`
  - `(*Service).Search(ctx, SearchParams) ([]SearchHit, error)`
  - `isText(value string) bool`; `(Capture).summary() string`; the summary of a todo has its status when the status is not `open`
  - sqlc: `SearchCatalogItems`, `SearchJournalEntries`, `SearchTodos`, `SearchSources`, each with the parameters `Patterns []string` and `MaxRows int32`
  - Test helpers: `search(t, s, query string, types ...string) []SearchHit`, `wantHits(t, name, got, want ...RecordRef)`, `hitRefs`

- [ ] **Step 1: Write the failing tests**

Create `internal/service/search_test.go`:

```go
package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// hitRefs returns the type and the ID of each hit, in order.
func hitRefs(hits []SearchHit) []RecordRef {
	refs := []RecordRef{}
	for _, hit := range hits {
		refs = append(refs, RecordRef{hit.Type, hit.ID})
	}
	return refs
}

func search(t *testing.T, s *Service, query string, types ...string) []SearchHit {
	t.Helper()
	hits, err := s.Search(context.Background(), SearchParams{Query: query, Types: types})
	if err != nil {
		t.Fatalf("Search(%q): %v", query, err)
	}
	return hits
}

func wantHits(t *testing.T, name string, got []SearchHit, want ...RecordRef) {
	t.Helper()
	refs := hitRefs(got)
	if len(refs) != len(want) {
		t.Errorf("%s: hits = %v, want %v", name, refs, want)
		return
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Errorf("%s: hits = %v, want %v", name, refs, want)
			return
		}
	}
}

func TestSearchFindsARecordOfEachType(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	item, err := s.AddItem(ctx, dune()) // Dune: Part Two, director Denis Villeneuve
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	entry := addJournal(t, s, "Went to Lonavala with Asha.\nIt rained all day.", "2026-10-09")
	p := buyDomain()
	p.Title, p.Project = "Renew the car insurance", "home"
	todo, err := s.AddTodo(ctx, p)
	if err != nil {
		t.Fatalf("AddTodo: %v", err)
	}
	text, err := s.Capture(ctx, CaptureParams{Text: "The policy renews on 12 March.", Title: "Insurance note", Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	link, err := s.Capture(ctx, CaptureParams{URL: "https://example.com/lonavala-guide", Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	wantHits(t, "a word of a title", search(t, s, "dune"), RecordRef{"item", item.ID})
	wantHits(t, "a word of an attr", search(t, s, "villeneuve"), RecordRef{"item", item.ID})
	wantHits(t, "the kind and a word", search(t, s, "movie dune"), RecordRef{"item", item.ID})
	wantHits(t, "a word of a body", search(t, s, "rained"), RecordRef{"journal", entry.ID})
	wantHits(t, "the day of an entry", search(t, s, "2026-10-09"), RecordRef{"journal", entry.ID})
	wantHits(t, "a project", search(t, s, "home"), RecordRef{"todo", todo.ID})
	wantHits(t, "a word of a text", search(t, s, "renews"), RecordRef{"source", text.ID})
	wantHits(t, "a part of a link", search(t, s, "example.com"), RecordRef{"source", link.ID})

	// The newest record is first.
	wantHits(t, "a word in two types", search(t, s, "insurance"), RecordRef{"source", text.ID}, RecordRef{"todo", todo.ID})
	wantHits(t, "a part of a word, in a different case", search(t, s, "LONAV"), RecordRef{"source", link.ID}, RecordRef{"journal", entry.ID})
	wantHits(t, "one type", search(t, s, "lonavala", "journal"), RecordRef{"journal", entry.ID})
	wantHits(t, "two types", search(t, s, "insurance", "todo", "item"), RecordRef{"todo", todo.ID})

	// Every word must be in the record. The words can be in any order.
	wantHits(t, "two words far apart", search(t, s, "rained lonavala"), RecordRef{"journal", entry.ID})
	wantHits(t, "a word that no record has", search(t, s, "lonavala snow"))

	hits := search(t, s, "dune")
	if hits[0].Summary != "movie: Dune: Part Two, rating 4.5, on 2026-10-09" || !hits[0].Verified || hits[0].CreatedAt.IsZero() {
		t.Errorf("hit = %+v, want the summary of the item", hits[0])
	}
	if got := search(t, s, "renews")[0].Summary; got != "text: Insurance note: The policy renews on 12 March." {
		t.Errorf("summary of a text = %q", got)
	}
	if got := search(t, s, "example.com")[0].Summary; got != "link: https://example.com/lonavala-guide" {
		t.Errorf("summary of a link = %q", got)
	}
}

func TestSearchLeavesOutADeletedRecordAndShowsARecordThatWaits(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	deleted := addItem(t, s, "book", "Dune")
	waits := addUnverified(t, s, "Dune Messiah")
	if err := s.DeleteItem(ctx, deleted.ID, DeleteParams{Actor: SourceManual}); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}

	hits := search(t, s, "dune")
	wantHits(t, "dune", hits, RecordRef{"item", waits.ID})
	if hits[0].Verified {
		t.Error("the hit says that the record is verified, but it waits for review")
	}
}

// A percent sign and an underscore are plain characters in a search.
func TestSearchTakesTheQueryLiterally(t *testing.T) {
	s, _ := newTestService(t)
	percent := addTodo(t, s, "Get 50% off the domain")
	underscore := addTodo(t, s, "Rename max_file_mb")
	addTodo(t, s, "Buy a domain")

	wantHits(t, "a percent sign", search(t, s, "50%"), RecordRef{"todo", percent.ID})
	wantHits(t, "only a percent sign", search(t, s, "%"), RecordRef{"todo", percent.ID})
	wantHits(t, "an underscore", search(t, s, "max_file"), RecordRef{"todo", underscore.ID})
	wantHits(t, "a backslash", search(t, s, `\`))
	wantHits(t, "a quote", search(t, s, `'; DROP TABLE todos; --`))
	// A search stores nothing, so the rule on secrets does not apply.
	wantHits(t, "a card number", search(t, s, testCard))
}

func TestSearchReturnsTheNewestHitsUpToTheLimit(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	first := addTodo(t, s, "Call the bank")
	entry := addJournal(t, s, "the bank was closed", "")
	last := addTodo(t, s, "Call the bank again")

	hits, err := s.Search(ctx, SearchParams{Query: "bank", Limit: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	wantHits(t, "limit 2", hits, RecordRef{"todo", last.ID}, RecordRef{"journal", entry.ID})
	wantHits(t, "no limit", search(t, s, "bank"), RecordRef{"todo", last.ID}, RecordRef{"journal", entry.ID}, RecordRef{"todo", first.ID})

	none := search(t, s, "nothing-has-this-word")
	if none == nil || len(none) != 0 {
		t.Errorf("hits = %v, want an empty list, not nil", none)
	}
}

func TestSearchRejectsBadInput(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	cases := []struct {
		name  string
		p     SearchParams
		field string
	}{
		{"no query", SearchParams{}, "query"},
		{"a query of spaces", SearchParams{Query: " \t\n"}, "query"},
		{"a query that is too long", SearchParams{Query: strings.Repeat("a", maxQueryLength+1)}, "query"},
		{"too many words", SearchParams{Query: strings.Repeat("word ", maxQueryWords+1)}, "query"},
		{"a NUL", SearchParams{Query: "a\x00b"}, "query"},
		{"a type that search does not cover", SearchParams{Query: "gym", Types: []string{"habit_log"}}, "types"},
		{"an unknown type", SearchParams{Query: "gym", Types: []string{"item", "song"}}, "types"},
		{"a limit over the maximum", SearchParams{Query: "gym", Limit: 201}, "limit"},
	}
	for _, c := range cases {
		_, err := s.Search(ctx, c.p)
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Field != c.field {
			t.Errorf("%s: error = %v, want a ValidationError on %s", c.name, err, c.field)
		}
	}
	if _, err := s.Search(ctx, SearchParams{Query: strings.Repeat("word ", maxQueryWords)}); err != nil {
		t.Errorf("a query with the maximum number of words: %v", err)
	}
}

// An agent that looks for a todo must see that it is done already.
func TestSearchShowsTheStatusOfATodoThatIsNotOpen(t *testing.T) {
	s, _ := newTestService(t)
	open := addTodo(t, s, "Call the bank")
	done := addTodo(t, s, "Call the bank about the card")
	setStatus(t, s, done.ID, "done")

	hits := search(t, s, "bank")
	wantHits(t, "bank", hits, RecordRef{"todo", done.ID}, RecordRef{"todo", open.ID})
	// The test helper gives each todo a project and a due day.
	if hits[0].Summary != "todo: Call the bank about the card, done, project lifeos, due 2026-10-20" || hits[1].Summary != "todo: Call the bank, project lifeos, due 2026-10-20" {
		t.Errorf("summaries = %q, %q", hits[0].Summary, hits[1].Summary)
	}
}

func TestSearchShowsAFileByItsName(t *testing.T) {
	s, _ := newFileService(t)
	file, err := s.Capture(context.Background(), CaptureParams{Upload: strings.NewReader("%PDF-1.7\n"), FileName: "policy-2026.pdf", Source: SourceManual})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	hits := search(t, s, "policy")
	wantHits(t, "policy", hits, RecordRef{"source", file.ID})
	if hits[0].Summary != "file: policy-2026.pdf (application/pdf)" {
		t.Errorf("summary = %q", hits[0].Summary)
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run Search"
```
Expected: the run fails. The output contains `undefined: SearchHit`.

- [ ] **Step 3: Write the queries**

`ILIKE ALL` is true when the text matches each pattern of the list.

Create `queries/search.sql`:

```sql
-- Each search query gets the words of the search as patterns: one word
-- between % signs for each pattern. A row matches when its text matches
-- every pattern. ILIKE does not look at upper and lower case.

-- name: SearchCatalogItems :many
SELECT * FROM catalog_items
WHERE deleted_at IS NULL
  AND (kind || ' ' || title || ' ' || attrs::text) ILIKE ALL (sqlc.arg(patterns)::text[])
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(max_rows);

-- name: SearchJournalEntries :many
SELECT * FROM journal_entries
WHERE deleted_at IS NULL
  AND (body || ' ' || entry_date::text) ILIKE ALL (sqlc.arg(patterns)::text[])
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(max_rows);

-- name: SearchTodos :many
SELECT * FROM todos
WHERE deleted_at IS NULL
  AND (title || ' ' || coalesce(project, '')) ILIKE ALL (sqlc.arg(patterns)::text[])
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(max_rows);

-- name: SearchSources :many
SELECT * FROM sources
WHERE deleted_at IS NULL
  AND (coalesce(title, '') || ' ' || coalesce(content, '') || ' ' || coalesce(url, '')) ILIKE ALL (sqlc.arg(patterns)::text[])
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(max_rows);
```

- [ ] **Step 4: Split the check of a text**

A search query is text that Postgres must be able to compare, but the rule on secrets does not apply to it: a search stores nothing.

In `internal/service/fields.go`, replace this:

```go
func checkText(field, value string) error {
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return invalid(field, "must be valid text without a NUL character")
```

With this:

```go
func checkText(field, value string) error {
	if !isText(value) {
		return invalid(field, "must be valid text without a NUL character")
```

In `internal/service/fields.go`, replace this:

```go
	return nil
}
```

With this:

```go
	return nil
}

// isText reports whether value is text that Postgres can store and compare.
func isText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}
```

- [ ] **Step 5: Give a source and a todo that is not open their summaries**

In `internal/service/capture_change.go`, replace this:

```go
	})
}
```

With this:

```go
	})
}

// summary is the one line that a search shows for a source.
func (c Capture) summary() string {
	s := c.Kind + ": "
	if c.Title != "" && c.Kind != "file" {
		s += c.Title + ": "
	}
	switch c.Kind {
	case "link":
		return s + c.URL
	case "file":
		return s + c.Title + " (" + c.MediaType + ")"
	}
	return s + shorten(c.Content, 80)
}
```

In `internal/service/todos_change.go`, replace this:

```go
	s := "todo: " + t.Title
	if t.Project != "" {
```

With this:

```go
	s := "todo: " + t.Title
	if t.Status != "open" {
		s += ", " + t.Status
	}
	if t.Project != "" {
```

- [ ] **Step 6: Write the search**

Create `internal/service/search.go`:

```go
package service

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// SearchTypes are the record types that Search looks in. A habit log has no
// text; the query tool answers a question about habits.
var SearchTypes = []string{"item", "journal", "todo", "source"}

const (
	maxQueryLength = 200
	maxQueryWords  = 10
)

// SearchParams is the input of Search.
type SearchParams struct {
	// Query is one or more words. A record matches when its text has
	// every word. Upper and lower case are the same, and a part of a word
	// matches: "lonav" finds "Lonavala".
	Query string
	// Types is optional: the record types to look in. Empty means every
	// type in SearchTypes.
	Types []string
	// Limit is the maximum number of hits. 0 means 50; the maximum is 200.
	Limit int
}

// SearchHit is one record that matches a search.
type SearchHit struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
	// Summary is one line that says what the record holds.
	Summary string `json:"summary"`
	// Verified is false for a record that waits in the review queue.
	Verified  bool      `json:"verified"`
	CreatedAt time.Time `json:"created_at"`
}

// Search returns the records that have every word of the query, the newest
// first. It leaves out a deleted record. It writes nothing.
func (s *Service) Search(ctx context.Context, p SearchParams) ([]SearchHit, error) {
	if !isText(p.Query) {
		return nil, invalid("query", "must be valid text without a NUL character")
	}
	if utf8.RuneCountInString(p.Query) > maxQueryLength {
		return nil, invalid("query", fmt.Sprintf("must be at most %d characters", maxQueryLength))
	}
	words := strings.Fields(p.Query)
	if len(words) == 0 {
		return nil, invalid("query", "must not be empty")
	}
	if len(words) > maxQueryWords {
		return nil, invalid("query", fmt.Sprintf("must have at most %d words", maxQueryWords))
	}
	types := p.Types
	if len(types) == 0 {
		types = SearchTypes
	}
	for _, recordType := range types {
		if !slices.Contains(SearchTypes, recordType) {
			return nil, invalid("types", fmt.Sprintf("%q is not a type that search looks in; the types are: %s", recordType, strings.Join(SearchTypes, ", ")))
		}
	}
	maxRows, err := checkLimit(p.Limit)
	if err != nil {
		return nil, err
	}

	patterns := make([]string, len(words))
	for i, word := range words {
		patterns[i] = "%" + likeEscaper.Replace(word) + "%"
	}
	q := store.New(s.pool)
	hits := []SearchHit{}
	for _, recordType := range SearchTypes {
		if !slices.Contains(types, recordType) {
			continue
		}
		more, err := searchType(ctx, q, recordType, patterns, maxRows)
		if err != nil {
			return nil, fmt.Errorf("search: %w", err)
		}
		hits = append(hits, more...)
	}
	// Each type gave its newest hits. Put them in one order.
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].CreatedAt.After(hits[j].CreatedAt) })
	if len(hits) > int(maxRows) {
		hits = hits[:maxRows]
	}
	return hits, nil
}

// likeEscaper makes a word a plain text in a LIKE pattern: the percent
// sign and the underscore lose their special meaning.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// searchType returns the newest hits of one record type.
func searchType(ctx context.Context, q *store.Queries, recordType string, patterns []string, maxRows int32) ([]SearchHit, error) {
	var hits []SearchHit
	switch recordType {
	case "item":
		rows, err := q.SearchCatalogItems(ctx, store.SearchCatalogItemsParams{Patterns: patterns, MaxRows: maxRows})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			item, err := itemFromRow(row)
			if err != nil {
				return nil, err
			}
			hits = append(hits, SearchHit{"item", item.ID, item.summary(), item.Verified, item.CreatedAt})
		}
	case "journal":
		rows, err := q.SearchJournalEntries(ctx, store.SearchJournalEntriesParams{Patterns: patterns, MaxRows: maxRows})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			entry := journalEntryFromRow(row)
			hits = append(hits, SearchHit{"journal", entry.ID, entry.summary(), entry.Verified, entry.CreatedAt})
		}
	case "todo":
		rows, err := q.SearchTodos(ctx, store.SearchTodosParams{Patterns: patterns, MaxRows: maxRows})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			todo := todoFromRow(row)
			hits = append(hits, SearchHit{"todo", todo.ID, todo.summary(), todo.Verified, todo.CreatedAt})
		}
	case "source":
		rows, err := q.SearchSources(ctx, store.SearchSourcesParams{Patterns: patterns, MaxRows: maxRows})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			capture := captureFromRow(row)
			hits = append(hits, SearchHit{"source", capture.ID, capture.summary(), capture.Verified, capture.CreatedAt})
		}
	}
	return hits, nil
}
```

- [ ] **Step 7: Run the tests of the service to verify that they pass**

Run:
```bash
make test PKG=./internal/service
```
Expected: the run passes. The last line starts with `ok`.

- [ ] **Step 8: Commit**

Run:
```bash
git add queries/search.sql internal/service/search.go internal/service/search_test.go internal/service/fields.go internal/service/capture_change.go internal/service/todos_change.go
git commit -m "feat(service): search the records by their words"
```

---

### Task 12: REST and MCP: search

**Files:**
- Create: `internal/api/search.go`, `internal/mcp/search.go`
- Modify: `internal/api/api.go` (one line), `internal/mcp/server.go` (one line)
- Test: `internal/api/search_test.go`, `internal/mcp/search_test.go`, `internal/mcp/items_test.go` (the list of tools)

**Interfaces:**
- Consumes (Task 11, package `service`): `SearchParams{Query string; Types []string; Limit int}`, `SearchHit`, `(*Service).Search`.
- Consumes (Tasks 8 and 9): `registerSources` in `internal/api/api.go`, `registerCapture` in `internal/mcp/server.go`, the routes and the tools that the tests use to add records.
- Produces:
  - `GET /api/search?query=&types=&limit=`: 200 and `{"hits": [...]}`. `types` is a list with a comma between its values.
  - The tool `search` with the arguments `query`, `types`, `limit`. Its output is `{"hits": [...]}`.
  - `registerSearch` in each of the two packages

- [ ] **Step 1: Write the failing tests of the route**

Create `internal/api/search_test.go`:

```go
package api_test

import (
	"net/http"
	"strings"
	"testing"
)

// hitTypes returns the type of each hit, in order.
func hitTypes(body map[string]any) []string {
	types := []string{}
	for _, hit := range body["hits"].([]any) {
		types = append(types, hit.(map[string]any)["type"].(string))
	}
	return types
}

func TestSearchFindsRecordsOfEachType(t *testing.T) {
	h, _ := newTestAPI(t)
	do(t, h, "POST", "/api/items", `{"kind":"place","title":"Lonavala"}`)
	do(t, h, "POST", "/api/journal", `{"body":"Went to Lonavala. It rained."}`)
	do(t, h, "POST", "/api/todos", `{"title":"Book a room in Lonavala"}`)
	do(t, h, "POST", "/api/ingest", `{"url":"https://example.com/lonavala-guide"}`)

	status, body, raw := do(t, h, "GET", "/api/search?query=lonav", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", status, raw)
	}
	// The newest record is first.
	if got := strings.Join(hitTypes(body), " "); got != "source todo journal item" {
		t.Errorf("types of the hits = %q, want source todo journal item\n%s", got, raw)
	}
	first := body["hits"].([]any)[0].(map[string]any)
	if first["summary"] != "link: https://example.com/lonavala-guide" || first["verified"] != true || first["id"] == nil {
		t.Errorf("first hit = %v", first)
	}

	_, body, raw = do(t, h, "GET", "/api/search?query=lonavala&types=journal,todo&limit=1", "")
	if got := strings.Join(hitTypes(body), " "); got != "todo" {
		t.Errorf("types with a filter and a limit = %q, want todo\n%s", got, raw)
	}
	_, body, raw = do(t, h, "GET", "/api/search?query=rained+lonavala", "")
	if got := strings.Join(hitTypes(body), " "); got != "journal" {
		t.Errorf("types for two words = %q, want journal\n%s", got, raw)
	}
	if _, _, raw := do(t, h, "GET", "/api/search?query=snow", ""); !strings.Contains(raw, `"hits":[]`) {
		t.Errorf("no hit: body = %s, want an empty list", raw)
	}
}

func TestSearchRejectsBadRequests(t *testing.T) {
	h, _ := newTestAPI(t)
	tests := []struct {
		name, path, wantInBody string
	}{
		{"no query", "/api/search", "query.query"},
		{"a query of spaces", "/api/search?query=+++", "query.query"},
		{"a type that search does not look in", "/api/search?query=gym&types=habit_log", "query.types"},
		{"a limit over the maximum", "/api/search?query=gym&limit=201", "query.limit"},
		{"a limit that is not a number", "/api/search?query=gym&limit=many", "query.limit"},
	}
	for _, tt := range tests {
		status, _, raw := do(t, h, "GET", tt.path, "")
		if status != http.StatusUnprocessableEntity || !strings.Contains(raw, tt.wantInBody) {
			t.Errorf("%s: status = %d, want 422 with %q\n%s", tt.name, status, tt.wantInBody, raw)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/api ARGS="-run Search"
```
Expected: the run fails. The output contains `GET /api/search?query=lonav: body is not a JSON object`.

The route does not exist yet, so the answer is the text `404 page not found` and not JSON.

- [ ] **Step 3: Write the route and register it**

Create `internal/api/search.go`:

```go
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type searchInput struct {
	Query string   `query:"query" doc:"One or more words. A record matches when it has every word. A part of a word matches, and upper and lower case are the same."`
	Types []string `query:"types" doc:"The record types to look in, with a comma between them: item, journal, todo, source. Empty looks in every type."`
	Limit int      `query:"limit" doc:"1 to 200. The default is 50."`
}

type searchOutput struct {
	Body struct {
		Hits []service.SearchHit `json:"hits"`
	}
}

func registerSearch(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "search",
		Method:      http.MethodGet,
		Path:        "/api/search",
		Summary:     "Find records by their words, the newest first",
		Description: "Looks in list items, journal entries, todos and sources. A hit has the type, the ID and one line that says what the record holds. Read the full record with the GET route of its type.",
	}, func(ctx context.Context, in *searchInput) (*searchOutput, error) {
		hits, err := svc.Search(ctx, service.SearchParams{Query: in.Query, Types: in.Types, Limit: in.Limit})
		if err != nil {
			return nil, toHTTPError("query", err)
		}
		out := &searchOutput{}
		out.Body.Hits = hits
		return out, nil
	})
}
```

In `internal/api/api.go`, replace this:

```go
	registerSources(api, svc)
}
```

With this:

```go
	registerSources(api, svc)
	registerSearch(api, svc)
}
```

- [ ] **Step 4: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./internal/api ARGS="-run Search"
```
Expected: the run passes. The last line starts with `ok`.

- [ ] **Step 5: Commit**

Run:
```bash
git add internal/api/search.go internal/api/search_test.go internal/api/api.go
git commit -m "feat(api): add the search route"
```

- [ ] **Step 6: Write the failing tests of the tool**

In `internal/mcp/items_test.go`, replace this:

```go
	}
	for _, name := range []string{"add_item", "get", "capture"} {
		if !found[name] {
```

With this:

```go
	}
	for _, name := range []string{"add_item", "get", "capture", "search"} {
		if !found[name] {
```

Create `internal/mcp/search_test.go`:

```go
package mcp_test

import (
	"strings"
	"testing"
)

func TestSearchFindsTheRecordToChange(t *testing.T) {
	session, _ := connect(t)
	call(t, session, "todo", map[string]any{"title": "Renew the car insurance", "dictated": true})
	call(t, session, "journal", map[string]any{"body": "Paid the insurance today."})
	proposed := structured(t, call(t, session, "todo", map[string]any{"title": "Call the insurance agent"}))

	found := structured(t, call(t, session, "search", map[string]any{"query": "INSURANCE"}))
	hits := found["hits"].([]any)
	if len(hits) != 3 {
		t.Fatalf("hits = %v, want 3", hits)
	}
	// The newest record is first. It waits for approval.
	first := hits[0].(map[string]any)
	if first["type"] != "todo" || first["id"] != proposed["id"] || first["summary"] != "todo: Call the insurance agent" || first["verified"] != false {
		t.Errorf("first hit = %v", first)
	}

	// The agent finds the todo, then marks it done with its ID.
	todos := structured(t, call(t, session, "search", map[string]any{"query": "renew insurance", "types": []string{"todo"}}))
	hits = todos["hits"].([]any)
	if len(hits) != 1 {
		t.Fatalf("hits for two words in todos = %v, want 1", hits)
	}
	done := structured(t, call(t, session, "update", map[string]any{
		"type": "todo", "id": hits[0].(map[string]any)["id"], "fields": map[string]any{"status": "done"}, "dictated": true,
	}))
	if done["status"] != "done" {
		t.Errorf("todo after the update = %v", done)
	}

	none := structured(t, call(t, session, "search", map[string]any{"query": "snow"}))
	if list, ok := none["hits"].([]any); !ok || len(list) != 0 {
		t.Errorf("hits = %v, want an empty list", none["hits"])
	}
}

func TestSearchRejectsBadInput(t *testing.T) {
	session, _ := connect(t)

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"a query of spaces", map[string]any{"query": "  "}, "query: must not be empty"},
		{"a type that search does not look in", map[string]any{"query": "gym", "types": []string{"habit_log"}}, `types: "habit_log" is not a type that search looks in`},
		{"a limit over the maximum", map[string]any{"query": "gym", "limit": 201}, "limit: must be from 1 to 200"},
		{"no query", map[string]any{}, "query"},
	}
	for _, tt := range tests {
		result := call(t, session, "search", tt.args)
		if !result.IsError || !strings.Contains(text(result), tt.want) {
			t.Errorf("%s: result = %q, want an error with %q", tt.name, text(result), tt.want)
		}
	}
}
```

- [ ] **Step 7: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/mcp ARGS="-run 'Search|ListsTheTools'"
```
Expected: the run fails. The output contains `tool search is not registered`.

- [ ] **Step 8: Write the tool and register it**

The description says what an agent uses `search` for: to find the ID of a record before it reads or changes the record.

Create `internal/mcp/search.go`:

```go
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type searchInput struct {
	Query string   `json:"query" jsonschema:"One or more words. A record matches when it has every word. A part of a word matches, and upper and lower case are the same. Use few words: each word that you add gives fewer hits."`
	Types []string `json:"types,omitempty" jsonschema:"The record types to look in: item, journal, todo, source. Leave it out to look in every type."`
	Limit int      `json:"limit,omitempty" jsonschema:"1 to 200. The default is 50."`
}

type searchOutput struct {
	Hits []service.SearchHit `json:"hits"`
}

func registerSearch(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "search",
		Description: "Find records by their words: list items, journal entries, todos and sources. Returns the newest hits first. A hit has the type, the ID and one line that says what the record holds; to read the full record, use get. A hit with verified set to false waits for the user's approval: tell the user when you use such a record. Use this tool to find the ID of a record before you call update or delete.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in searchInput) (*sdk.CallToolResult, searchOutput, error) {
		hits, err := svc.Search(ctx, service.SearchParams{Query: in.Query, Types: in.Types, Limit: in.Limit})
		if err != nil {
			return nil, searchOutput{}, toToolError(err)
		}
		return nil, searchOutput{Hits: hits}, nil
	})
}
```

In `internal/mcp/server.go`, replace this:

```go
	registerCapture(server, svc)
	return server
```

With this:

```go
	registerCapture(server, svc)
	registerSearch(server, svc)
	return server
```

- [ ] **Step 9: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./internal/mcp ARGS="-run 'Search|ListsTheTools'"
```
Expected: the run passes. The last line starts with `ok`.

- [ ] **Step 10: Commit**

Run:
```bash
git add internal/mcp/search.go internal/mcp/search_test.go internal/mcp/items_test.go internal/mcp/server.go
git commit -m "feat(mcp): add the search tool"
```

---

### Task 13: The read-only Postgres user

**Files:**
- Create: `deploy/query-user.sh`, `migrations/00004_query_user.sql`
- Modify: `Makefile`, `internal/testdb/testdb.go`, `migrations/migrations_test.go`

**Interfaces:**
- Consumes: `deploy/.env` with `LIFEOS_QUERY_PASSWORD`; the Postgres container of `deploy/docker-compose.yml`, where `lifeos` is the superuser; `testdb.New(t) *pgxpool.Pool`.
- Produces:
  - `make query-user`: makes the Postgres user `lifeos_query` with the password of `deploy/.env`, or sets the password again. A second run changes nothing.
  - The Makefile exports `LIFEOS_QUERY_PASSWORD` to each command that it runs.
  - Migration `00004`: `lifeos_query` can SELECT from each table of the schema `public`, also from a table that a later migration makes. The migration fails when the user does not exist.
  - `testdb.QueryPool(t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool`: a pool for the database of `pool` that connects as `lifeos_query`.

- [ ] **Step 1: Write the script**

A migration cannot make this user: a user belongs to the server, not to one database, and its password must not be in git. `psql` reads the password from its environment, so the value is in no command line.

Create `deploy/query-user.sh`:

```bash
#!/bin/bash
# Makes the read-only Postgres user lifeos_query, or sets its password again.
# The query tool connects as this user, so a query cannot change data. Run
# it one time before the first migration, and again after a change of the
# password. `make query-user` runs it.
# Needs LIFEOS_QUERY_PASSWORD in deploy/.env.
set -euo pipefail

cd "$(dirname "$0")"
set -a; . ./.env; set +a
: "${LIFEOS_QUERY_PASSWORD:?set LIFEOS_QUERY_PASSWORD in deploy/.env}"

# psql gets the password from its environment, so the value is in no command
# line. A user is not a part of one database: migrations give it its rights.
docker compose exec -T -e LIFEOS_QUERY_PASSWORD postgres \
    psql -U lifeos -d postgres -v ON_ERROR_STOP=1 -q <<'SQL'
\getenv password LIFEOS_QUERY_PASSWORD
SELECT 'CREATE ROLE lifeos_query LOGIN'
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'lifeos_query') \gexec
ALTER ROLE lifeos_query PASSWORD :'password';
SQL
echo "the Postgres user lifeos_query is ready"
```

Run:
```bash
chmod +x deploy/query-user.sh
```

- [ ] **Step 2: Add the command and the password to the Makefile**

In `Makefile`, replace this:

```makefile
# Reads POSTGRES_PASSWORD from deploy/.env, which is not in git.
include deploy/.env
export POSTGRES_PASSWORD
# Tests create and drop their own databases on this server.
```

With this:

```makefile
# Reads the two database passwords from deploy/.env, which is not in git.
include deploy/.env
export POSTGRES_PASSWORD
# The password of the read-only user lifeos_query, for the query tool.
export LIFEOS_QUERY_PASSWORD
# Tests create and drop their own databases on this server.
```

In `Makefile`, replace this:

```makefile
.PHONY: tools generate build test migrate run
```

With this:

```makefile
.PHONY: tools query-user generate build test migrate run
```

In `Makefile`, replace this:

```makefile
	curl -fsSL https://github.com/sqlc-dev/sqlc/releases/download/v$(SQLC_VERSION)/sqlc_$(SQLC_VERSION)_linux_arm64.tar.gz | tar -xz -C $(dir $(SQLC)) sqlc
```

With this:

```makefile
	curl -fsSL https://github.com/sqlc-dev/sqlc/releases/download/v$(SQLC_VERSION)/sqlc_$(SQLC_VERSION)_linux_arm64.tar.gz | tar -xz -C $(dir $(SQLC)) sqlc

# One time, before the first migration: makes the read-only Postgres user
# lifeos_query. Run it again after a change of LIFEOS_QUERY_PASSWORD.
query-user:
	deploy/query-user.sh
```

- [ ] **Step 3: Make the user**

Run:
```bash
make query-user
```
Expected: the run passes. The output contains `the Postgres user lifeos_query is ready`.

Run it a second time: the output must be the same.

- [ ] **Step 4: Commit**

Run:
```bash
git add deploy/query-user.sh Makefile
git commit -m "feat(deploy): add the command that makes the read-only Postgres user"
```

- [ ] **Step 5: Write the failing tests**

The tests connect as `lifeos_query` with a plain connection, with no read-only transaction. So they show what the rights of the user permit.

In `internal/testdb/testdb.go`, replace this:

```go
	return pool
}
```

With this:

```go
	return pool
}

// QueryPool returns a pool for the database of pool that connects as the
// read-only user lifeos_query. The password comes from the
// LIFEOS_QUERY_PASSWORD environment variable; `make query-user` makes the
// user.
func QueryPool(t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	password := os.Getenv("LIFEOS_QUERY_PASSWORD")
	if password == "" {
		t.Fatal("LIFEOS_QUERY_PASSWORD is not set; run the tests with `make test`")
	}
	cfg := pool.Config().Copy()
	cfg.ConnConfig.User = "lifeos_query"
	cfg.ConnConfig.Password = password

	queryPool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("testdb: open the query pool: %v", err)
	}
	t.Cleanup(queryPool.Close)
	return queryPool
}
```

In `migrations/migrations_test.go`, replace this:

```go
import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/Gautam-J/lifeos-core/internal/testdb"
	"github.com/Gautam-J/lifeos-core/migrations"
)
```

With this:

```go
import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/Gautam-J/lifeos-core/internal/testdb"
	"github.com/Gautam-J/lifeos-core/migrations"
)
```

In `migrations/migrations_test.go`, replace this:

```go
			t.Errorf("%s: a row with the source_ref of a source: %v", table, err)
		}
	}
}
```

With this:

```go
			t.Errorf("%s: a row with the source_ref of a source: %v", table, err)
		}
	}
}

// The query tool connects as lifeos_query. That user reads every table and
// changes none: the database refuses the change, not the Go code.
func TestTheQueryUserCanReadAndCannotWrite(t *testing.T) {
	pool := testdb.New(t)
	queryPool := testdb.QueryPool(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "INSERT INTO todos (title, source) VALUES ('Buy a domain', 'manual')"); err != nil {
		t.Fatalf("add a todo: %v", err)
	}

	var user string
	if err := queryPool.QueryRow(ctx, "SELECT current_user").Scan(&user); err != nil || user != "lifeos_query" {
		t.Fatalf("current_user = %q, %v, want lifeos_query", user, err)
	}
	tables, err := pool.Query(ctx, "SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
	if err != nil {
		t.Fatalf("list the tables: %v", err)
	}
	names, err := pgx.CollectRows(tables, pgx.RowTo[string])
	if err != nil || len(names) < 8 {
		t.Fatalf("tables = %v, %v, want 8 or more", names, err)
	}
	for _, table := range names {
		var n int
		if err := queryPool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Errorf("lifeos_query cannot read %s: %v", table, err)
		}
	}

	for name, change := range map[string]string{
		"an insert":          "INSERT INTO todos (title, source) VALUES ('x', 'manual')",
		"an update":          "UPDATE todos SET title = 'x'",
		"a delete":           "DELETE FROM todos",
		"a truncate":         "TRUNCATE events",
		"a drop":             "DROP TABLE habit_logs",
		"a new table":        "CREATE TABLE notes (id int)",
		"a change of a role": "ALTER ROLE lifeos_query SUPERUSER",
	} {
		if _, err := queryPool.Exec(ctx, change); err == nil {
			t.Errorf("lifeos_query did %s", name)
		}
	}
	var title string
	if err := pool.QueryRow(ctx, "SELECT title FROM todos").Scan(&title); err != nil || title != "Buy a domain" {
		t.Errorf("the todo = %q, %v, want it as it was", title, err)
	}
}

// A table from a later migration must be readable with no new GRANT.
func TestTheQueryUserCanReadATableThatIsMadeLater(t *testing.T) {
	pool := testdb.New(t)
	queryPool := testdb.QueryPool(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "CREATE TABLE later (id int)"); err != nil {
		t.Fatalf("make a table: %v", err)
	}

	var n int
	if err := queryPool.QueryRow(ctx, "SELECT count(*) FROM later").Scan(&n); err != nil {
		t.Errorf("lifeos_query cannot read a table that is made after the migration: %v", err)
	}
}
```

- [ ] **Step 6: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./migrations
```
Expected: the run fails. The output contains `lifeos_query cannot read` and `permission denied for table`.

- [ ] **Step 7: Write the migration**

Create `migrations/00004_query_user.sql`:

```sql
-- +goose Up

-- The read-only user of the query tool can read every table and change
-- none. `make query-user` makes the user; a user is not a part of one
-- database, so this migration cannot make it with its password.
GRANT SELECT ON ALL TABLES IN SCHEMA public TO lifeos_query;

-- A table that a later migration makes gets the same right.
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO lifeos_query;

-- +goose Down
ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE SELECT ON TABLES FROM lifeos_query;
REVOKE SELECT ON ALL TABLES IN SCHEMA public FROM lifeos_query;
```

- [ ] **Step 8: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./migrations
```
Expected: the run passes. The last line starts with `ok`.

- [ ] **Step 9: Commit**

Run:
```bash
git add migrations/00004_query_user.sql internal/testdb/testdb.go migrations/migrations_test.go
git commit -m "feat(db): let the read-only user read every table"
```

---

### Task 14: The read-only query

**Files:**
- Create: `queries/query_user.sql`, `internal/service/query.go`, `internal/mcp/query.go`
- Modify: `internal/service/service.go`, `internal/config/config.go`, `config/lifeos.yaml`, `cmd/lifeos/db.go`, `cmd/lifeos/serve.go`, `internal/mcp/server.go`, `internal/mcp/search.go`
- Test: `internal/service/query_test.go`, `internal/config/config_test.go`, `cmd/lifeos/serve_test.go`, `internal/mcp/query_test.go`, `internal/mcp/items_test.go`

**Interfaces:**
- Consumes (Task 13): the user `lifeos_query` and its rights; `testdb.QueryPool(t, pool)`; `LIFEOS_QUERY_PASSWORD` in the environment of each `make` command.
- Consumes (Tasks 5, 9, 11 and 12): `fileStore` and the line `service.New(pool).WithFiles(files)` in `cmd/lifeos/serve.go`; `connectTo(t, svc)` in `mcp_test`; `isText`; `registerSearch` and the description of the tool `search`.
- Consumes (plans 1 to 3): `poolConfig(cfg config.Config) (*pgxpool.Config, error)` in `cmd/lifeos/db.go`; `toToolError`; the test helpers `newTestService`, `addTodo`, `ptr`, `equal`, `countRows`, `addHabit` (in `mcp_test`).
- Produces (package `service`):
  - `(*Service).UseQueryPool(ctx context.Context, pool *pgxpool.Pool) error`: sets the pool of `Query`. It returns an error, and sets nothing, when the user of the pool can change a table.
  - `type QueryResult struct{ Columns []string; Rows [][]*string; Truncated bool }` with the JSON names `columns`, `rows`, `truncated`
  - `(*Service).Query(ctx context.Context, sql string) (QueryResult, error)`. A statement that Postgres refuses gives a `ValidationError` on the field `sql`.
  - The fields `Service.queryPool` and `Service.queryTimeout`; the constants `maxSQLLength` (10000), `maxQueryRows` (200), `maxQueryResultBytes` (256 kB), `defaultQueryTimeout` (5 seconds)
  - sqlc: `CanChangeATable`
- Produces (package `config`): the field `QueryDatabaseURL string` for the key `query_database_url`.
- Produces (package `main`): `queryPoolConfig(cfg config.Config) (*pgxpool.Config, error)`.
- Produces (package `mcp`): the tool `query` with the argument `sql`; `registerQuery`; the constant `queryDescription`.

- [ ] **Step 1: Write the failing tests of the service**

Create `internal/service/query_test.go`:

```go
package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

// newQueryService returns a service with a query pool that connects as the
// read-only user.
func newQueryService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	s, pool := newTestService(t)
	if err := s.UseQueryPool(context.Background(), testdb.QueryPool(t, pool)); err != nil {
		t.Fatalf("UseQueryPool: %v", err)
	}
	return s, pool
}

// cells returns the values of a result as text. A NULL is "NULL".
func cells(result QueryResult) [][]string {
	out := [][]string{}
	for _, row := range result.Rows {
		texts := []string{}
		for _, value := range row {
			if value == nil {
				texts = append(texts, "NULL")
			} else {
				texts = append(texts, *value)
			}
		}
		out = append(out, texts)
	}
	return out
}

func wantSQLRefused(t *testing.T, name string, err error, wantInMessage string) {
	t.Helper()
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "sql" || !strings.Contains(invalid.Message, wantInMessage) {
		t.Errorf("%s: error = %v, want a ValidationError on sql with %q", name, err, wantInMessage)
	}
}

func TestQueryReturnsTheColumnsAndTheRowsAsText(t *testing.T) {
	s, _ := newQueryService(t)
	ctx := context.Background()
	addTodo(t, s, "Buy a domain") // project lifeos, due 2026-10-20
	done := addTodo(t, s, "Call the bank")
	if _, err := s.UpdateTodo(ctx, done.ID, UpdateTodoParams{Status: ptr("done"), Actor: SourceManual}); err != nil {
		t.Fatalf("UpdateTodo: %v", err)
	}

	got, err := s.Query(ctx, "SELECT title, status, due_on, done_at IS NULL AS waits, verified, deleted_at FROM todos ORDER BY id")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !equal(got.Columns, []string{"title", "status", "due_on", "waits", "verified", "deleted_at"}) {
		t.Errorf("columns = %v", got.Columns)
	}
	rows := cells(got)
	if len(rows) != 2 ||
		!equal(rows[0], []string{"Buy a domain", "open", "2026-10-20", "t", "t", "NULL"}) ||
		!equal(rows[1], []string{"Call the bank", "done", "2026-10-20", "f", "t", "NULL"}) {
		t.Errorf("rows = %v", rows)
	}
	if got.Truncated {
		t.Error("truncated = true for two rows")
	}

	count, err := s.Query(ctx, "SELECT status, count(*) AS n FROM todos WHERE deleted_at IS NULL GROUP BY status ORDER BY status")
	if err != nil {
		t.Fatalf("Query with GROUP BY: %v", err)
	}
	if rows := cells(count); len(rows) != 2 || !equal(rows[0], []string{"done", "1"}) || !equal(rows[1], []string{"open", "1"}) {
		t.Errorf("counts = %v", rows)
	}

	// A value of each type comes as the text that psql shows.
	typed, err := s.Query(ctx, `SELECT 1.50::numeric, '\xdeadbeef'::bytea, '{"a": [1, 2]}'::jsonb, ARRAY['x', 'y'], '2026-10-11 07:30:00+05:30'::timestamptz AT TIME ZONE 'UTC', NULL::int`)
	if err != nil {
		t.Fatalf("Query with a value of each type: %v", err)
	}
	if rows := cells(typed); len(rows) != 1 || !equal(rows[0], []string{"1.50", `\xdeadbeef`, `{"a": [1, 2]}`, "{x,y}", "2026-10-11 02:00:00", "NULL"}) {
		t.Errorf("typed values = %q", rows)
	}

	none, err := s.Query(ctx, "SELECT id FROM todos WHERE false")
	if err != nil || none.Rows == nil || len(none.Rows) != 0 || !equal(none.Columns, []string{"id"}) {
		t.Errorf("no row: result = %+v, %v, want the column and an empty list", none, err)
	}
}

// The database refuses a change. The Go code does not look at the SQL. The
// read-only transaction refuses first; the migration tests show that the
// rights of lifeos_query refuse the same statements without it.
func TestQueryCannotChangeData(t *testing.T) {
	s, pool := newQueryService(t)
	ctx := context.Background()
	todo := addTodo(t, s, "Buy a domain")
	before := countRows(t, pool, "events")

	for name, c := range map[string]struct{ sql, want string }{
		"an insert":               {"INSERT INTO todos (title, source) VALUES ('x', 'manual')", "read-only transaction"},
		"an update":               {"UPDATE todos SET title = 'x'", "read-only transaction"},
		"a delete":                {"DELETE FROM todos", "read-only transaction"},
		"a delete in a WITH":      {"WITH gone AS (DELETE FROM todos RETURNING id) SELECT * FROM gone", "read-only transaction"},
		"a drop":                  {"DROP TABLE todos", "read-only transaction"},
		"a temporary table":       {"CREATE TEMP TABLE notes (id int)", "read-only transaction"},
		"a second statement":      {"SELECT 1; DELETE FROM todos", "multiple commands"},
		"a new role":              {"CREATE ROLE helper SUPERUSER", "read-only transaction"},
		"a file of the server":    {"SELECT pg_read_file('/etc/passwd')", "permission denied"},
		"a program on the server": {"COPY (SELECT 1) TO PROGRAM 'id'", "permission denied"},
	} {
		_, err := s.Query(ctx, c.sql)
		wantSQLRefused(t, name, err, c.want)
	}

	got, err := s.GetTodo(ctx, todo.ID)
	if err != nil || got.Title != "Buy a domain" {
		t.Errorf("the todo = %+v, %v, want it as it was", got, err)
	}
	if after := countRows(t, pool, "events"); after != before {
		t.Errorf("events = %d, want %d", after, before)
	}
}

func TestQueryRejectsBadInput(t *testing.T) {
	s, _ := newQueryService(t)
	ctx := context.Background()

	for name, c := range map[string]struct{ sql, want string }{
		"no statement":              {"  \n", "must not be empty"},
		"a statement too long":      {"SELECT '" + strings.Repeat("a", maxSQLLength) + "'", "at most"},
		"a NUL":                     {"SELECT 'a\x00b'", "valid text"},
		"a syntax error":            {"SELEC 1", `syntax error at or near "SELEC"`},
		"a table that is not there": {"SELECT * FROM songs", `relation "songs" does not exist`},
		"a parameter":               {"SELECT $1", "bind message supplies 0 parameters"},
	} {
		_, err := s.Query(ctx, c.sql)
		wantSQLRefused(t, name, err, c.want)
	}
}

func TestQueryCutsALargeResult(t *testing.T) {
	s, _ := newQueryService(t)
	ctx := context.Background()

	full, err := s.Query(ctx, "SELECT generate_series(1, 200)")
	if err != nil || len(full.Rows) != maxQueryRows || full.Truncated {
		t.Errorf("200 rows: rows = %d, truncated = %v, %v, want 200, false", len(full.Rows), full.Truncated, err)
	}
	cut, err := s.Query(ctx, "SELECT generate_series(1, 201)")
	if err != nil || len(cut.Rows) != maxQueryRows || !cut.Truncated {
		t.Errorf("201 rows: rows = %d, truncated = %v, %v, want 200, true", len(cut.Rows), cut.Truncated, err)
	}
	// Ten rows of 100 kB are more than the limit on the size.
	wide, err := s.Query(ctx, "SELECT repeat('x', 100000) FROM generate_series(1, 10)")
	if err != nil || len(wide.Rows) >= 10 || len(wide.Rows) == 0 || !wide.Truncated {
		t.Errorf("wide rows: rows = %d, truncated = %v, %v, want some rows, fewer than 10, true", len(wide.Rows), wide.Truncated, err)
	}
}

func TestQueryStopsAStatementThatRunsTooLong(t *testing.T) {
	s, _ := newQueryService(t)
	s.queryTimeout = 300 * time.Millisecond

	start := time.Now()
	_, err := s.Query(context.Background(), "SELECT pg_sleep(30)")
	wantSQLRefused(t, "a long statement", err, "took longer than 300ms")
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Query came back after %s, want less than 5 seconds", elapsed)
	}
	// The next query works.
	if got, err := s.Query(context.Background(), "SELECT 1"); err != nil || len(got.Rows) != 1 {
		t.Errorf("query after the timeout: %+v, %v", got, err)
	}
}

// One query must not leave a setting or a lock for the next query.
func TestQueryStartsEachStatementInACleanSession(t *testing.T) {
	s, _ := newQueryService(t)
	ctx := context.Background()

	for _, sql := range []string{
		"SELECT set_config('lifeos.note', 'left behind', false)",
		"SET statement_timeout = 1",
		"SELECT pg_advisory_lock(42)",
	} {
		if _, err := s.Query(ctx, sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	got, err := s.Query(ctx, "SELECT current_setting('lifeos.note', true), current_setting('statement_timeout'), (SELECT count(*) FROM pg_locks WHERE locktype = 'advisory')")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if rows := cells(got); len(rows) != 1 || !equal(rows[0], []string{"NULL", "0", "0"}) {
		t.Errorf("the session of the next query = %v, want no note, no timeout, no lock", rows)
	}
}

func TestUseQueryPoolRefusesAUserThatCanWrite(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	// pool connects as the owner of the tables.
	err := s.UseQueryPool(ctx, pool)
	if err == nil || !strings.Contains(err.Error(), "read-only user") {
		t.Fatalf("UseQueryPool with the owner: error = %v, want it refused", err)
	}
	_, err = s.Query(ctx, "SELECT 1")
	var invalid *ValidationError
	if err == nil || errors.As(err, &invalid) {
		t.Errorf("Query with no query pool: error = %v, want an error that is not a ValidationError", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/service ARGS="-run Query"
```
Expected: the run fails. The output contains `undefined: QueryResult`.

- [ ] **Step 3: Write the query that asks if a user can change a table**

Create `queries/query_user.sql`:

```sql
-- name: CanChangeATable :one
-- Reports whether the user of the connection can change a row of a table
-- in the schema public. The pool of the query tool must give false.
SELECT EXISTS (
    SELECT 1 FROM pg_catalog.pg_tables
    WHERE schemaname = 'public'
      AND has_table_privilege(current_user, format('%I.%I', schemaname, tablename), 'INSERT, UPDATE, DELETE, TRUNCATE')
)::boolean AS can_change;
```

- [ ] **Step 4: Give the service its query pool**

In `internal/service/service.go`, replace this:

```go
	files Files
}
```

With this:

```go
	files Files
	// queryPool connects as the read-only user. Only Query uses it.
	queryPool    *pgxpool.Pool
	queryTimeout time.Duration
}
```

In `internal/service/service.go`, replace this:

```go
func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, now: time.Now}
}
```

With this:

```go
func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, now: time.Now, queryTimeout: defaultQueryTimeout}
}
```

- [ ] **Step 5: Write `UseQueryPool` and `Query`**

`Query` is the one function that runs SQL from a caller. It does not look at the statement. Three things of the database stop a change: the rights of `lifeos_query`, the read-only transaction, and the prepared statement, which holds one statement only.

Create `internal/service/query.go`:

```go
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

const (
	maxSQLLength = 10000
	// maxQueryRows and maxQueryResultBytes keep an answer small enough for
	// the context of a model.
	maxQueryRows        = 200
	maxQueryResultBytes = 256 << 10
	defaultQueryTimeout = 5 * time.Second
)

// QueryResult is the answer of Query.
type QueryResult struct {
	Columns []string `json:"columns"`
	// Rows holds each value as the text that Postgres gives, such as
	// 2026-10-11 for a date and t for true. A nil value is a NULL.
	Rows [][]*string `json:"rows"`
	// Truncated is true when the query has more rows than Rows holds.
	Truncated bool `json:"truncated"`
}

// UseQueryPool sets the pool that Query uses. The user of the pool must
// have no right to change a table: UseQueryPool refuses a pool that can
// write, so a wrong database URL cannot make Query a way to change data.
func (s *Service) UseQueryPool(ctx context.Context, pool *pgxpool.Pool) error {
	canChange, err := store.New(pool).CanChangeATable(ctx)
	if err != nil {
		return fmt.Errorf("query pool: %w", err)
	}
	if canChange {
		return errors.New("query pool: its user can change a table; use the read-only user lifeos_query")
	}
	s.queryPool = pool
	return nil
}

// Query runs one SQL statement as the read-only user and returns its rows.
// It is the one function that takes SQL from a caller: an agent writes the
// statement, so the database, not this code, is what stops a change. The
// statement runs in a read-only transaction, for at most 5 seconds.
func (s *Service) Query(ctx context.Context, sql string) (QueryResult, error) {
	if s.queryPool == nil {
		return QueryResult{}, errors.New("query: the service has no query pool")
	}
	if strings.TrimSpace(sql) == "" {
		return QueryResult{}, invalid("sql", "must not be empty")
	}
	if len(sql) > maxSQLLength {
		return QueryResult{}, invalid("sql", fmt.Sprintf("must be at most %d characters", maxSQLLength))
	}
	if !isText(sql) {
		return QueryResult{}, invalid("sql", "must be valid text without a NUL character")
	}

	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	conn, err := s.queryPool.Acquire(ctx)
	if err != nil {
		return QueryResult{}, fmt.Errorf("query: %w", err)
	}
	// A statement can leave a setting or a lock in its session. The
	// connection closes after each query, so the next query starts clean.
	// The close also ends the transaction.
	defer func() {
		conn.Conn().Close(context.Background())
		conn.Release()
	}()
	if _, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}); err != nil {
		return QueryResult{}, fmt.Errorf("query: %w", err)
	}

	// ExecParams sends the statement as one prepared statement with no
	// parameter, so Postgres refuses a text that has two statements. It
	// gives each value in the text format, as psql shows it.
	reader := conn.Conn().PgConn().ExecParams(ctx, sql, nil, nil, nil, nil)
	result := QueryResult{Columns: []string{}, Rows: [][]*string{}}
	for _, field := range reader.FieldDescriptions() {
		result.Columns = append(result.Columns, field.Name)
	}
	size := 0
	for reader.NextRow() {
		if len(result.Rows) == maxQueryRows || size > maxQueryResultBytes {
			result.Truncated = true
			break
		}
		raw := reader.Values()
		row := make([]*string, len(raw))
		for i, value := range raw {
			if value != nil {
				text := string(value)
				row[i] = &text
				size += len(value)
			}
		}
		result.Rows = append(result.Rows, row)
	}
	// After a cut, the rows that the result holds are the answer.
	if _, err := reader.Close(); err != nil && !result.Truncated {
		return QueryResult{}, s.queryError(ctx, err)
	}
	return result, nil
}

// queryError turns an error of the statement into a ValidationError, so
// that the agent reads what is wrong with its SQL.
func (s *Service) queryError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return invalid("sql", fmt.Sprintf("took longer than %s: ask for fewer rows", s.queryTimeout))
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return invalid("sql", "PostgreSQL refused the statement: "+pgErr.Message)
	}
	return fmt.Errorf("query: %w", err)
}
```

- [ ] **Step 6: Run the tests to verify that they pass**

Run:
```bash
make test PKG=./internal/service ARGS="-run Query"
```
Expected: the run passes. The last line starts with `ok`.

- [ ] **Step 7: Commit**

Run:
```bash
git add queries/query_user.sql internal/service/query.go internal/service/query_test.go internal/service/service.go
git commit -m "feat(service): run a read-only query"
```

- [ ] **Step 8: Write the failing tests of the config and of the start**

In `internal/config/config_test.go`, replace this:

```go
database_url: postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable
files_dir: /var/lib/lifeos/files
```

With this:

```go
database_url: postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable
query_database_url: postgres://lifeos_query@127.0.0.1:5432/lifeos?sslmode=disable
files_dir: /var/lib/lifeos/files
```

In `internal/config/config_test.go`, replace this:

```go
	want := Config{
		HTTPAddr:    "127.0.0.1:8200",
		MCPAddr:     "127.0.0.1:8100",
		DatabaseURL: "postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable",
		FilesDir:    "/var/lib/lifeos/files",
		InboxDir:    "/var/lib/lifeos/inbox",
		MaxFileMB:   50,
	}
```

With this:

```go
	want := Config{
		HTTPAddr:         "127.0.0.1:8200",
		MCPAddr:          "127.0.0.1:8100",
		DatabaseURL:      "postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable",
		QueryDatabaseURL: "postgres://lifeos_query@127.0.0.1:5432/lifeos?sslmode=disable",
		FilesDir:         "/var/lib/lifeos/files",
		InboxDir:         "/var/lib/lifeos/inbox",
		MaxFileMB:        50,
	}
```

In `internal/config/config_test.go`, replace this:

```go
			wantErr: "database_url",
		},
```

With this:

```go
			wantErr: "database_url",
		},
		{
			name:    "missing query database url",
			content: strings.Replace(valid, "query_database_url: postgres://lifeos_query@127.0.0.1:5432/lifeos?sslmode=disable", "", 1),
			wantErr: "query_database_url",
		},
```

In `cmd/lifeos/serve_test.go`, replace this:

```go
		}
	}
}
```

With this:

```go
		}
	}
}

func TestQueryPoolConfigTakesThePasswordFromTheEnvironment(t *testing.T) {
	t.Setenv("LIFEOS_QUERY_PASSWORD", "from-the-environment")

	pc, err := queryPoolConfig(config.Config{QueryDatabaseURL: "postgres://lifeos_query@127.0.0.1:5432/lifeos?sslmode=disable"})
	if err != nil {
		t.Fatalf("queryPoolConfig: %v", err)
	}
	if pc.ConnConfig.User != "lifeos_query" || pc.ConnConfig.Password != "from-the-environment" || pc.MaxConns != 2 {
		t.Errorf("user, max connections = %q, %d; the password is from the environment: %v",
			pc.ConnConfig.User, pc.MaxConns, pc.ConnConfig.Password == "from-the-environment")
	}

	_, err = queryPoolConfig(config.Config{QueryDatabaseURL: "not a url"})
	if err == nil || !strings.Contains(err.Error(), "query_database_url") {
		t.Errorf("error = %v, want an error that names query_database_url", err)
	}
}
```

- [ ] **Step 9: Run the tests to verify that they fail**

Run:
```bash
make test PKG="./internal/config ./cmd/lifeos"
```
Expected: the run fails. The output contains `unknown field QueryDatabaseURL in struct literal of type Config` and `undefined: queryPoolConfig`.

- [ ] **Step 10: Add the key to the config**

In `internal/config/config.go`, replace this:

```go
	DatabaseURL string `yaml:"database_url"`
	// FilesDir is the file store: the folder that holds the captured
```

With this:

```go
	DatabaseURL string `yaml:"database_url"`
	// QueryDatabaseURL is the Postgres URL of the read-only user, for the
	// query tool. The password comes from the LIFEOS_QUERY_PASSWORD
	// environment variable.
	QueryDatabaseURL string `yaml:"query_database_url"`
	// FilesDir is the file store: the folder that holds the captured
```

In `internal/config/config.go`, replace this:

```go
		return Config{}, fmt.Errorf("config: %s: database_url is empty", path)
	}
```

With this:

```go
		return Config{}, fmt.Errorf("config: %s: database_url is empty", path)
	}
	if cfg.QueryDatabaseURL == "" {
		return Config{}, fmt.Errorf("config: %s: query_database_url is empty", path)
	}
```

In `config/lifeos.yaml`, replace this:

```yaml
# The file store and the inbox of captured files. The two folders must
```

With this:

```yaml
# The same database as the read-only user lifeos_query, for the query tool.
# The password comes from LIFEOS_QUERY_PASSWORD in the environment
# (deploy/.env). `make query-user` makes the user.
query_database_url: postgres://lifeos_query@127.0.0.1:5432/lifeos?sslmode=disable

# The file store and the inbox of captured files. The two folders must
```

- [ ] **Step 11: Open the query pool at the start**

`lifeos serve` stops at the start when the user of `query_database_url` can change a table.

In `cmd/lifeos/db.go`, replace this:

```go
	return pc, nil
}
```

With this:

```go
	return pc, nil
}

// queryPoolConfig turns query_database_url into pool settings and adds the
// password from the LIFEOS_QUERY_PASSWORD environment variable. Each query
// uses a connection one time, so the pool is small.
func queryPoolConfig(cfg config.Config) (*pgxpool.Config, error) {
	pc, err := pgxpool.ParseConfig(cfg.QueryDatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("query_database_url: %w", err)
	}
	if password := os.Getenv("LIFEOS_QUERY_PASSWORD"); password != "" {
		pc.ConnConfig.Password = password
	}
	pc.MaxConns = 2
	return pc, nil
}
```

In `cmd/lifeos/serve.go`, replace this:

```go
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
```

With this:

```go
		return err
	}
	queryConfig, err := queryPoolConfig(cfg)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
```

In `cmd/lifeos/serve.go`, replace this:

```go
	}

	httpListener, mcpListener, err := listen(cfg)
	if err != nil {
```

With this:

```go
	}

	queryPool, err := pgxpool.NewWithConfig(ctx, queryConfig)
	if err != nil {
		return fmt.Errorf("query_database_url: %w", err)
	}
	defer queryPool.Close()
	svc := service.New(pool).WithFiles(files)
	if err := svc.UseQueryPool(ctx, queryPool); err != nil {
		return fmt.Errorf("query_database_url: %w", err)
	}

	httpListener, mcpListener, err := listen(cfg)
	if err != nil {
```

In `cmd/lifeos/serve.go`, replace this:

```go
	}
	slog.Info("lifeos is listening", "http", httpListener.Addr().String(), "mcp", mcpListener.Addr().String())
	return serveOn(ctx, httpListener, mcpListener, service.New(pool).WithFiles(files))
}
```

With this:

```go
	}
	slog.Info("lifeos is listening", "http", httpListener.Addr().String(), "mcp", mcpListener.Addr().String())
	return serveOn(ctx, httpListener, mcpListener, svc)
}
```

- [ ] **Step 12: Run the tests to verify that they pass**

Run:
```bash
make test PKG="./internal/config ./cmd/lifeos"
```
Expected: the run passes. The output contains `internal/config` and `cmd/lifeos`.

- [ ] **Step 13: Commit**

Run:
```bash
git add internal/config/config.go internal/config/config_test.go config/lifeos.yaml cmd/lifeos/db.go cmd/lifeos/serve.go cmd/lifeos/serve_test.go
git commit -m "feat(config): connect the query pool as the read-only user"
```

- [ ] **Step 14: Write the failing tests of the tool**

`TestQueryDescriptionNamesEveryTable` reads the tables from the database. A later migration that adds a table makes this test fail until the description names the table.

In `internal/mcp/items_test.go`, replace this:

```go
	}
	for _, name := range []string{"add_item", "get", "capture", "search"} {
		if !found[name] {
```

With this:

```go
	}
	for _, name := range []string{"add_item", "get", "capture", "search", "query"} {
		if !found[name] {
```

Create `internal/mcp/query_test.go`:

```go
package mcp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

// connectWithQuery returns a session for a service that has a query pool.
func connectWithQuery(t *testing.T) (*sdk.ClientSession, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.New(t)
	svc := service.New(pool)
	if err := svc.UseQueryPool(context.Background(), testdb.QueryPool(t, pool)); err != nil {
		t.Fatalf("UseQueryPool: %v", err)
	}
	return connectTo(t, svc), pool
}

func TestQueryAnswersAQuestionWithACount(t *testing.T) {
	session, pool := connectWithQuery(t)
	addHabit(t, pool, "gym")
	for _, day := range []string{"2026-01-05", "2026-01-06", "2026-01-08"} {
		call(t, session, "log_habit", map[string]any{"habit": "gym", "logged_on": day, "dictated": true})
	}
	call(t, session, "log_habit", map[string]any{"habit": "gym", "logged_on": "2026-01-09"}) // waits for approval

	got := structured(t, call(t, session, "query", map[string]any{
		"sql": "SELECT h.name, count(*) AS days, min(l.logged_on) AS first_day FROM habit_logs l JOIN habits h ON h.id = l.habit_id WHERE l.deleted_at IS NULL AND l.verified GROUP BY h.name",
	}))

	columns := got["columns"].([]any)
	rows := got["rows"].([]any)
	if len(columns) != 3 || columns[1] != "days" || len(rows) != 1 {
		t.Fatalf("result = %v, want three columns and one row", got)
	}
	if row := rows[0].([]any); row[0] != "gym" || row[1] != "3" || row[2] != "2026-01-05" {
		t.Errorf("row = %v, want gym, 3, 2026-01-05 as text", row)
	}
	if got["truncated"] != false {
		t.Errorf("truncated = %v, want false", got["truncated"])
	}

	// A NULL is a JSON null.
	null := structured(t, call(t, session, "query", map[string]any{"sql": "SELECT NULL::text AS nothing"}))
	if row := null["rows"].([]any)[0].([]any); row[0] != nil {
		t.Errorf("NULL = %v, want null", row[0])
	}
}

func TestQueryCannotChangeData(t *testing.T) {
	session, pool := connectWithQuery(t)
	call(t, session, "todo", map[string]any{"title": "Buy a domain", "dictated": true})

	for name, tt := range map[string]struct{ sql, want string }{
		"a delete":           {"DELETE FROM todos", "sql: PostgreSQL refused the statement: cannot execute DELETE in a read-only transaction"},
		"a second statement": {"SELECT 1; UPDATE todos SET title = 'x'", "cannot insert multiple commands into a prepared statement"},
		"a syntax error":     {"SELEC 1", "syntax error"},
		"no statement":       {"", "sql: must not be empty"},
	} {
		result := call(t, session, "query", map[string]any{"sql": tt.sql})
		if !result.IsError || !strings.Contains(text(result), tt.want) {
			t.Errorf("%s: result = %q, want an error with %q", name, text(result), tt.want)
		}
		// The agent wrote the SQL. It must not ask the user to correct it.
		if !strings.Contains(text(result), "Correct the statement, then call query again.") || strings.Contains(text(result), "Ask the user") {
			t.Errorf("%s: result = %q, want it to tell the agent to correct the statement", name, text(result))
		}
	}
	var title string
	if err := pool.QueryRow(context.Background(), "SELECT title FROM todos").Scan(&title); err != nil || title != "Buy a domain" {
		t.Errorf("the todo = %q, %v, want it as it was", title, err)
	}
}

// The description is how the model knows the tables. A migration that adds
// a table must add its name to the description.
func TestQueryDescriptionNamesEveryTable(t *testing.T) {
	session, pool := connectWithQuery(t)
	ctx := context.Background()

	list, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	description := ""
	for _, tool := range list.Tools {
		if tool.Name == "query" {
			description = tool.Description
		}
	}
	rows, err := pool.Query(ctx, "SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
	if err != nil {
		t.Fatalf("list the tables: %v", err)
	}
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(tables) < 8 {
		t.Fatalf("tables = %v, %v, want 8 or more", tables, err)
	}
	for _, table := range tables {
		if !strings.Contains(description, table) {
			t.Errorf("the description of query does not name the table %s", table)
		}
	}
}
```

- [ ] **Step 15: Run the tests to verify that they fail**

Run:
```bash
make test PKG=./internal/mcp ARGS="-run 'Query|ListsTheTools'"
```
Expected: the run fails. The output contains `tool query is not registered`.

- [ ] **Step 16: Write the tool**

The description is the only way that a model learns the tables. The error of a wrong statement tells the agent to correct the statement: the agent wrote it, so the text of `toToolError` ("Ask the user") is wrong here.

Create `internal/mcp/query.go`:

```go
package mcp

import (
	"context"
	"errors"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type queryInput struct {
	SQL string `json:"sql" jsonschema:"One SELECT statement for PostgreSQL, with no parameter. Give the columns that you need and a LIMIT."`
}

// queryDescription names each table. A test checks it against the tables
// of the database, so a new table needs a new name here.
const queryDescription = "Run one read-only SQL statement on the user's data. Use it for a question that needs a count, a sum, a range of dates or a join, such as \"how many books did I read this year\" or \"on which days of last week did I go to the gym\". To find a record by a word, use search. " +
	"The tables are: catalog_items (the lists: kind, title, rating, occurred_on, attrs), journal_entries (body, entry_date), todos (title, status, due_on, project, done_at), habits (name, active), habit_logs (habit_id, logged_on), sources (kind, title, content, url), events (the log of every write) and goose_db_version (the migrations). For the columns of a table, read information_schema.columns. " +
	"Each record table has id, source, verified, created_at, updated_at and deleted_at. Add deleted_at IS NULL to leave out the deleted rows. Add verified to count only the rows that the user approved. A date is a day in India Standard Time; a timestamp is in UTC. " +
	"Returns the names of the columns and at most 200 rows, with each value as text. When truncated is true, the statement has more rows: count or sum in SQL, or add a LIMIT. The connection can only read: a statement that changes data fails. To change a record, use update or delete."

type queryOutput = service.QueryResult

func registerQuery(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "query",
		Description: queryDescription,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in queryInput) (*sdk.CallToolResult, queryOutput, error) {
		result, err := svc.Query(ctx, in.SQL)
		// The agent wrote the statement, so the agent corrects it. The
		// text of toToolError tells the agent to ask the user.
		var invalid *service.ValidationError
		if errors.As(err, &invalid) {
			return nil, queryOutput{}, errors.New(invalid.Error() + ". Correct the statement, then call query again.")
		}
		if err != nil {
			return nil, queryOutput{}, toToolError(err)
		}
		return nil, result, nil
	})
}
```

- [ ] **Step 17: Register the tool, and name it in the description of `search`**

In `internal/mcp/server.go`, replace this:

```go
	registerSearch(server, svc)
	return server
```

With this:

```go
	registerSearch(server, svc)
	registerQuery(server, svc)
	return server
```

In `internal/mcp/search.go`, replace this:

```go
		Name:        "search",
		Description: "Find records by their words: list items, journal entries, todos and sources. Returns the newest hits first. A hit has the type, the ID and one line that says what the record holds; to read the full record, use get. A hit with verified set to false waits for the user's approval: tell the user when you use such a record. Use this tool to find the ID of a record before you call update or delete.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in searchInput) (*sdk.CallToolResult, searchOutput, error) {
```

With this:

```go
		Name:        "search",
		Description: "Find records by their words: list items, journal entries, todos and sources. Returns the newest hits first. A hit has the type, the ID and one line that says what the record holds; to read the full record, use get. A hit with verified set to false waits for the user's approval: tell the user when you use such a record. Use this tool to find the ID of a record before you call update or delete. For a count, a sum, a list by date or a question about habits, use query.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in searchInput) (*sdk.CallToolResult, searchOutput, error) {
```

- [ ] **Step 18: Run the tests of the package to verify that they pass**

Run:
```bash
make test PKG=./internal/mcp
```
Expected: the run passes. The last line starts with `ok`.

- [ ] **Step 19: Commit**

Run:
```bash
git add internal/mcp/query.go internal/mcp/query_test.go internal/mcp/items_test.go internal/mcp/server.go internal/mcp/search.go
git commit -m "feat(mcp): add the query tool"
```

---

### Task 15: End to end, the backup and the docs

**Files:**
- Create: `cmd/lifeos/capture_test.go`
- Modify: `cmd/lifeos/lifecycle_test.go` (`startServer`), `deploy/backup.sh`, `AGENTS.md`, `README.md`

**Interfaces:**
- Consumes (Tasks 2 to 14): every route, tool and page of this plan; `service.Files`, `(*Service).WithFiles`, `(*Service).UseQueryPool`, `testdb.QueryPool`.
- Consumes (plan 2, package `main`): `startServer(t) (config.Config, *pgxpool.Pool)`, `send(t, method, url, contentType, body string) (int, string)`.
- Produces: `startServer` now gives its service a file store in a temporary folder and a query pool. `deploy/backup.sh` puts the file store in each snapshot.

- [ ] **Step 1: Give the test server a file store and a query pool**

In `cmd/lifeos/lifecycle_test.go`, replace this:

```go
// startServer runs serveOn on free ports with a new database. The servers
// stop when the test ends.
func startServer(t *testing.T) (config.Config, *pgxpool.Pool) {
```

With this:

```go
// startServer runs serveOn on free ports with a new database, a file store
// in a temporary folder and the read-only query pool. The servers stop when
// the test ends.
func startServer(t *testing.T) (config.Config, *pgxpool.Pool) {
```

In `cmd/lifeos/lifecycle_test.go`, replace this:

```go
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveOn(ctx, httpListener, mcpListener, service.New(pool)) }()
	t.Cleanup(func() {
```

With this:

```go
	}
	svc := service.New(pool).WithFiles(service.Files{Dir: t.TempDir(), Inbox: t.TempDir(), MaxBytes: 1 << 20})
	if err := svc.UseQueryPool(context.Background(), testdb.QueryPool(t, pool)); err != nil {
		t.Fatalf("UseQueryPool: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveOn(ctx, httpListener, mcpListener, svc) }()
	t.Cleanup(func() {
```

- [ ] **Step 2: Write the end-to-end test**

This test sends real HTTP requests to the two listeners. It checks that the three adapters use one service: each adapter finds what the others stored.

Create `cmd/lifeos/capture_test.go`:

```go
package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// A link, a receipt and a note come in through three adapters. The agent
// finds the note, takes a book from it and names the note as its source.
// Each adapter must find what the others stored.
func TestServeTakesACaptureThroughEveryAdapter(t *testing.T) {
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
	call := func(name string, args map[string]any) *sdk.CallToolResult {
		t.Helper()
		result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("MCP %s: %v", name, err)
		}
		return result
	}
	tool := func(name string, args map[string]any) map[string]any {
		t.Helper()
		result := call(name, args)
		if result.IsError {
			t.Fatalf("MCP %s returned an error: %+v", name, result.Content[0])
		}
		return result.StructuredContent.(map[string]any)
	}

	// 1. REST: the share sheet sends a link, then a file.
	status, body := send(t, "POST", rest+"/api/ingest", json, `{"url":"https://example.com/lonavala-guide"}`)
	if status != 200 || !strings.Contains(body, `"kind":"link"`) {
		t.Fatalf("POST /api/ingest: status %d, body %s", status, body)
	}
	status, body = send(t, "POST", rest+"/api/ingest/file?name=receipt.pdf", "application/pdf", "%PDF-1.7\n1 0 obj\nendobj\n")
	if status != 200 || !strings.Contains(body, `"media_type":"application/pdf"`) || !strings.Contains(body, `"title":"receipt.pdf"`) {
		t.Fatalf("POST /api/ingest/file: status %d, body %s", status, body)
	}

	// 2. Admin: Gautam types a note.
	status, body = send(t, "POST", rest+"/admin/sources", form, "text=Read+Dune+on+the+train+to+Lonavala.&title=Trip+note")
	if status != 200 || !strings.Contains(body, "Trip note") || !strings.Contains(body, "receipt.pdf") || !strings.Contains(body, "lonavala-guide") {
		t.Fatalf("POST /admin/sources: status %d, body %s", status, body)
	}

	// 3. MCP: the agent looks for Lonavala. The newest source is first.
	hits := tool("search", map[string]any{"query": "lonavala"})["hits"].([]any)
	if len(hits) != 2 {
		t.Fatalf("search: hits = %v, want the note and the link", hits)
	}
	note := hits[0].(map[string]any)
	if note["type"] != "source" || note["summary"] != "text: Trip note: Read Dune on the train to Lonavala." {
		t.Fatalf("first hit = %v, want the note", note)
	}
	link := hits[1].(map[string]any)

	// 4. MCP: the agent reads the note, and proposes a book that names
	// the note as its source. REST shows the book with its source.
	if got := tool("get", map[string]any{"type": "source", "id": note["id"]}); got["content"] != "Read Dune on the train to Lonavala." {
		t.Fatalf("get of the note: %v", got)
	}
	book := tool("add_item", map[string]any{"kind": "book", "title": "Dune", "source_ref": note["id"]})
	if book["source_ref"] != note["id"] || book["verified"] != false {
		t.Fatalf("add_item: %v, want a proposed book with the source_ref of the note", book)
	}
	status, body = send(t, "GET", rest+"/api/search?query=dune", "", "")
	if status != 200 || !strings.Contains(body, `"summary":"book: Dune"`) || !strings.Contains(body, "Trip note") {
		t.Fatalf("GET /api/search: status %d, body %s", status, body)
	}

	// 5. MCP: the same link a second time is the same source.
	if again := tool("capture", map[string]any{"url": "https://example.com/lonavala-guide"}); again["id"] != link["id"] || again["source"] != "manual" {
		t.Fatalf("second capture of the link: %v, want the source %v as it was", again, link["id"])
	}

	// 6. MCP: a question that needs a count. The query cannot change data.
	counts := tool("query", map[string]any{"sql": "SELECT kind, count(*) FROM sources WHERE deleted_at IS NULL GROUP BY kind ORDER BY kind"})
	if got := fmt.Sprint(counts["rows"]); got != "[[file 1] [link 1] [text 1]]" {
		t.Fatalf("query: rows = %s, want one source of each kind", got)
	}
	if refused := call("query", map[string]any{"sql": "DELETE FROM sources"}); !refused.IsError {
		t.Fatal("query: a DELETE gave no error")
	}

	// 7. Each adapter refuses a card number, and none repeats it.
	const card = "4111 1111 1111 1111"
	status, body = send(t, "POST", rest+"/api/ingest", json, `{"text":"my card is `+card+`"}`)
	if status != 422 || !strings.Contains(body, "a card number") || strings.Contains(body, card) {
		t.Fatalf("REST with a card number: status %d, body %s", status, body)
	}
	refused := call("journal", map[string]any{"body": "new card: " + card})
	if text := refused.Content[0].(*sdk.TextContent).Text; !refused.IsError || !strings.Contains(text, "a card number") || strings.Contains(text, card) {
		t.Fatalf("MCP journal with a card number: %q", text)
	}
	status, body = send(t, "POST", rest+"/admin/todos", form, "title=pay+with+"+strings.ReplaceAll(card, " ", "+"))
	if status != 200 || !strings.Contains(body, "title: must not hold a card number") || strings.Contains(body, card) {
		t.Fatalf("admin todo with a card number: status %d, body %s", status, body)
	}

	// 8. The log has one event for each write: three sources and a book.
	var events, withCard int
	err = pool.QueryRow(ctx, "SELECT count(*), count(*) FILTER (WHERE new_value::text LIKE '%4111%') FROM events").Scan(&events, &withCard)
	if err != nil || events != 4 || withCard != 0 {
		t.Fatalf("events = %d, with the card number = %d, %v, want 4, 0", events, withCard, err)
	}
}
```

- [ ] **Step 3: Run the tests of the command**

The test passes at the first run, because Tasks 1 to 14 made each part. If it fails, an adapter is not connected: read the message, and correct the task that owns that adapter.

Run:
```bash
make test PKG=./cmd/lifeos ARGS="-v -run 'Capture|Records|Life|Flow'"
```
Expected: the run passes. The output contains `PASS: TestServeTakesACaptureThroughEveryAdapter` and `PASS: TestServeTakesADayOfRecordsThroughEveryAdapter`.

- [ ] **Step 4: Commit**

Run:
```bash
git add cmd/lifeos/capture_test.go cmd/lifeos/lifecycle_test.go
git commit -m "test: take a capture through every adapter"
```

- [ ] **Step 5: Put the file store in the backup**

The file store goes into the same snapshot as the database dump, so `restore-test.sh` finds the dump in the latest snapshot as before.

In `deploy/backup.sh`, replace this:

```bash
#!/bin/bash
# Nightly backup: pg_dump of the lifeos database into two restic repositories,
# one on the Pi and one in Backblaze B2.
# Needs RESTIC_PASSWORD and the B2 settings in deploy/.env.
```

With this:

```bash
#!/bin/bash
# Nightly backup: pg_dump of the lifeos database and the file store into two
# restic repositories, one on the Pi and one in Backblaze B2.
# Needs RESTIC_PASSWORD and the B2 settings in deploy/.env.
```

In `deploy/backup.sh`, replace this:

```bash
dump_dir=/var/backups/lifeos/dump
mkdir -p "$dump_dir"
```

With this:

```bash
dump_dir=/var/backups/lifeos/dump
# The captured files: files_dir in config/lifeos.yaml.
files_dir=/var/lib/lifeos/files
mkdir -p "$dump_dir"
```

In `deploy/backup.sh`, replace this:

```bash
restic backup --tag postgres "$dump_dir"
# Second copy off the Pi, through the S3-compatible API of B2.
restic -r "s3:https://$B2_ENDPOINT/$B2_BUCKET/restic" backup --tag postgres "$dump_dir"
```

With this:

```bash
restic backup --tag postgres "$dump_dir" "$files_dir"
# Second copy off the Pi, through the S3-compatible API of B2.
restic -r "s3:https://$B2_ENDPOINT/$B2_BUCKET/restic" backup --tag postgres "$dump_dir" "$files_dir"
```

Run:
```bash
bash -n deploy/backup.sh
grep -c '"$dump_dir" "$files_dir"' deploy/backup.sh
```
Expected: the run passes. The output contains `2`.

- [ ] **Step 6: Commit**

Run:
```bash
git add deploy/backup.sh
git commit -m "feat(deploy): back up the file store"
```

- [ ] **Step 7: Write the rules into `AGENTS.md`**

In `AGENTS.md`, replace this:

```markdown
migrations/            goose SQL migrations and the Go code that applies them
config/lifeos.yaml     listen addresses, database URL; later models, schedules, connectors, notifications
sqlc.yaml              sqlc settings
Makefile               tools, generate, build, test, migrate, run
dashboards/            Grafana definitions, versioned with the schema they query
deploy/                docker-compose.yml, systemd units and timers, deploy.sh
docs/design.md         the design doc, exported
```

With this:

```markdown
migrations/            goose SQL migrations and the Go code that applies them
config/lifeos.yaml     listen addresses, database URLs, file store; later models, schedules, connectors, notifications
sqlc.yaml              sqlc settings
Makefile               tools, query-user, generate, build, test, migrate, run
dashboards/            Grafana definitions, versioned with the schema they query
deploy/                docker-compose.yml, systemd units and timers, backup scripts, query-user.sh
docs/design.md         the design doc, exported
```

In `AGENTS.md`, replace this:

```markdown
2. **Adapters are thin.** API handlers, MCP tools and CLI subcommands parse input, call one service function and format the result. No adapter calls another adapter. Handlers deal with HTTP; tools carry the model-facing descriptions.
3. **SQL only through sqlc.** Write queries in `queries/*.sql` and call the generated code in `internal/store`. No SQL strings in Go, and only `internal/service` calls `internal/store`.
4. **Every write records provenance.** Each record row carries `source` (`manual`, `import`, `connector` or `agent`), `source_ref`, `verified`, `created_at` and `updated_at`. Rows an agent infers start with `verified = false`; rows Gautam dictates directly are verified. The adapter sets this: the REST API and the admin pages write `manual`, verified rows; an MCP tool writes `agent` rows that are verified only when the tool input says the user dictated the row. An update keeps `verified`. The service refuses a change that an agent infers to a verified row: the agent asks Gautam, then sends the change as dictated. Only `Verify` sets `verified` to `true`.
```

With this:

```markdown
2. **Adapters are thin.** API handlers, MCP tools and CLI subcommands parse input, call one service function and format the result. No adapter calls another adapter. Handlers deal with HTTP; tools carry the model-facing descriptions.
3. **SQL only through sqlc.** Write queries in `queries/*.sql` and call the generated code in `internal/store`. No SQL strings in Go, and only `internal/service` calls `internal/store`. The one exception is `Query`, which runs the SQL that an agent wrote (rule 13).
4. **Every write records provenance.** Each record row carries `source` (`manual`, `import`, `connector` or `agent`), `source_ref`, `verified`, `created_at` and `updated_at`. Rows an agent infers start with `verified = false`; rows Gautam dictates directly are verified. The adapter sets this: the REST API and the admin pages write `manual`, verified rows; an MCP tool writes `agent` rows that are verified only when the tool input says the user dictated the row. An update keeps `verified`. The service refuses a change that an agent infers to a verified row: the agent asks Gautam, then sends the change as dictated. Only `Verify` sets `verified` to `true`.
```

In `AGENTS.md`, replace this:

```markdown
6. **Deletes are soft.** Set `deleted_at`; permanent removal happens only in the admin pages. The service refuses a delete from an agent unless the call says that Gautam confirmed it.
7. **Never store** passwords, card or bank account numbers, or government ID numbers. The service layer rejects them.
8. **Store times in UTC** (`timestamptz`); display in IST.
9. **A record type has two entry points.** A REST route or an admin page knows its type and calls the function of that type (`GetItem`, `UpdateItem`, `DeleteItem`). The MCP tools `get`, `update` and `delete` get the type as input and call `Get`, `Update` and `Delete` in `internal/service/records.go`. The REST `PATCH` route calls `Update` too, because only a map of fields can tell a null from a field that is absent. These refuse a field that a caller must not set, such as `verified` or `source`. Add each new record type to `RecordTypes`, to these three functions, to `Verify` and to `ReviewQueue`. The record types are `item`, `journal`, `todo` and `habit_log`. A habit (the definition) is not a record type: only the admin pages add or change one.
10. **The journal holds Gautam's own words.** The service stores the body of a journal entry exactly as it comes and never rewords it. The `journal` tool writes a verified row, because no agent infers a journal entry. An admin handler changes the CR LF of a text area to LF, which is what Gautam typed.
11. **A habit has one log for each day.** A second log for the same habit and day is not a write: the service returns the stored log. The one exception: a verified log over a log that is not verified approves that log. "Done" for a day means a verified log.
```

With this:

```markdown
6. **Deletes are soft.** Set `deleted_at`; permanent removal happens only in the admin pages. The service refuses a delete from an agent unless the call says that Gautam confirmed it.
7. **Never store** passwords, card or bank account numbers, or government ID numbers. `checkText` in `internal/service/fields.go` refuses the ones that have a pattern: a card number, an Aadhaar number, a PAN, a private key and an access token (`secrets.go`). Send each text that a caller gives through `checkText` before the service stores it. The error names the kind of secret and never repeats it. A password in plain words and a bank account number have no pattern: the tool descriptions tell the agent not to send them.
8. **Store times in UTC** (`timestamptz`); display in IST.
9. **A record type has two entry points.** A REST route or an admin page knows its type and calls the function of that type (`GetItem`, `UpdateItem`, `DeleteItem`). The MCP tools `get`, `update` and `delete` get the type as input and call `Get`, `Update` and `Delete` in `internal/service/records.go`. The REST `PATCH` route calls `Update` too, because only a map of fields can tell a null from a field that is absent. These refuse a field that a caller must not set, such as `verified` or `source`. Add each new record type to `RecordTypes`, to these three functions, to `Verify` and to `ReviewQueue`. The record types are `item`, `journal`, `todo`, `habit_log` and `source`. A type that has text also gets a query in `queries/search.sql` and a case in `Search`. A habit (the definition) is not a record type: only the admin pages add or change one.
10. **The journal holds Gautam's own words.** The service stores the body of a journal entry exactly as it comes and never rewords it. The `journal` tool writes a verified row, because no agent infers a journal entry. An admin handler changes the CR LF of a text area to LF, which is what Gautam typed.
11. **A habit has one log for each day.** A second log for the same habit and day is not a write: the service returns the stored log. The one exception: a verified log over a log that is not verified approves that log. "Done" for a day means a verified log.
12. **A source is verbatim.** A source is one row of `sources`: a text, a link or a file that `Capture` stores exactly as it comes. The service never changes a source, so `update` refuses the type `source`. The same content a second time is not a write: `Capture` returns the stored source. A source is always verified, because it holds what came in and no agent inferred it. A record that an agent takes from a source names it in `source_ref`. A file is in the file store at `<files_dir>/<first two characters of its SHA-256>/<SHA-256>`; a soft delete keeps the file.
13. **`query` reads and never writes.** `Query` runs one SQL statement of an agent on a connection of the Postgres user `lifeos_query`, which has only the right to SELECT, in a read-only transaction. The database refuses a change; the Go code does not look at the SQL. `UseQueryPool` refuses a pool whose user can change a table. A new table gets the SELECT right from the default privileges of migration `00004`; add its name to the description of the `query` tool, which a test checks.
```

In `AGENTS.md`, replace this:

```markdown
make tools      # one time: install sqlc into ~/go/bin
make generate   # run sqlc and templ: writes internal/store/ and *_templ.go
```

With this:

```markdown
make tools      # one time: install sqlc into ~/go/bin
make query-user # one time: make the read-only Postgres user lifeos_query
make generate   # run sqlc and templ: writes internal/store/ and *_templ.go
```

In `AGENTS.md`, replace this:

```markdown
- **Generated code is not in git.** After a fresh clone, run `make generate` before `go build` or `go vet`.
- **Tests need Postgres.** Start the container from `deploy/docker-compose.yml`; the Makefile reads `deploy/.env`. Each test creates its own database and drops it. `go test` without `make` fails, because `LIFEOS_TEST_DATABASE_URL` is not set.
- **Narrow a test run** with `make test PKG=./internal/api ARGS="-run TestName -v"`.

`lifeos serve` runs the API and admin pages on `http_addr` and the MCP server on `mcp_addr`, path `/mcp`. `lifeos migrate`, `lifeos job <name>` and `lifeos import-csv <importer> <file>` are run by hand or by systemd timers. Each command reads `config/lifeos.yaml`, or the file given with `-config`.
```

With this:

```markdown
- **Generated code is not in git.** After a fresh clone, run `make generate` before `go build` or `go vet`.
- **Tests need Postgres and the read-only user.** Start the container from `deploy/docker-compose.yml`, then run `make query-user` one time; the Makefile reads `deploy/.env`. Each test creates its own database and drops it. `go test` without `make` fails, because `LIFEOS_TEST_DATABASE_URL` is not set.
- **Narrow a test run** with `make test PKG=./internal/api ARGS="-run TestName -v"`.

`lifeos serve` runs the API and admin pages on `http_addr` and the MCP server on `mcp_addr`, path `/mcp`. It needs the folders `files_dir` and `inbox_dir` of the config file, and the two passwords `POSTGRES_PASSWORD` and `LIFEOS_QUERY_PASSWORD` in the environment. `lifeos migrate`, `lifeos job <name>` and `lifeos import-csv <importer> <file>` are run by hand or by systemd timers. Each command reads `config/lifeos.yaml`, or the file given with `-config`.
```

In `AGENTS.md`, replace this:

```markdown
- One goose SQL file per change, `migrations/NNNNN_name.sql`, with `-- +goose Up` and `-- +goose Down`.
- Every new record table gets the provenance columns, `deleted_at`, and the `set_updated_at()` trigger. Copy them from `catalog_items` in `00001_init.sql`.
- Never edit a migration that has been applied; add a new one.
```

With this:

```markdown
- One goose SQL file per change, `migrations/NNNNN_name.sql`, with `-- +goose Up` and `-- +goose Down`.
- Every new record table gets the provenance columns, `deleted_at`, and the `set_updated_at()` trigger. Copy them from `catalog_items` in `00001_init.sql`. Give `source_ref` a foreign key to `sources`, as `00003_sources.sql` does.
- Never edit a migration that has been applied; add a new one.
```

- [ ] **Step 8: Describe the new functions in `README.md`**

In `README.md`, replace this:

```markdown
| 1 · Foundation | Done: Docker, Postgres, ntfy, nightly backup, restore test. |
| 2 · Core data | In work. Lists, journal, todos, habits, the review queue and the three interfaces are built. Search, capture, money tables and CSV importers are not built. |
| 3 to 5 | Not started. |
```

With this:

```markdown
| 1 · Foundation | Done: Docker, Postgres, ntfy, nightly backup, restore test. |
| 2 · Core data | In work. Lists, journal, todos, habits, the review queue, capture, search, the read-only query and the three interfaces are built. Money tables and CSV importers are not built. |
| 3 to 5 | Not started. |
```

In `README.md`, replace this:

```markdown
Each function below has one use case. All three interfaces call the same service code, so the same checks apply to each of them.
```

With this:

```markdown
Each function below has one use case. All three interfaces call the same service code, so the same checks apply to each of them.

The record types are `item`, `journal`, `todo`, `habit_log` and `source`.
```

In `README.md`, replace this:

```markdown
### 8. Backup, restore test and notifications

- `deploy/backup.sh` dumps the database into two restic repositories: one on the Pi and one in Backblaze B2. A systemd timer runs it each night at 02:00.
- `deploy/restore-test.sh [local|b2]` loads the latest backup into a scratch database and compares the row count of each table with production.
```

With this:

```markdown
### 8. Capture: sources

A source is a text, a link or a file that LifeOS keeps exactly as it comes. The same content a second time gives the source that exists. A source is always verified, and it has no field that you can change.

- **Text:** stored as it is written, 1 MB at most.
- **Link:** LifeOS stores the URL. It does not get the page.
- **File:** stored in the file store, with its name, its type and its SHA-256. The limit is `max_file_mb` (50 MB). LifeOS does not read the text of a PDF.
- A record that an agent takes from a source names the source in `source_ref`.

**Use case.** You share an article link from the phone; a Shortcut posts it to `/api/ingest`. In a Claude Code session on the Pi you put a policy PDF in the inbox folder and say "ingest this"; the agent calls `capture` with the path.

### 9. Search

A search finds list items, journal entries, todos and sources by their words. A record matches when it has every word. Upper and lower case are the same, and a part of a word matches. The newest hit is first.

**Use case.** You ask "when did I last go to Lonavala?". The agent calls `search` with `lonavala`, reads the hits, and answers with the date and the ID of the record.

### 10. Read-only query

The agent can run one SQL statement on the data. The statement runs as the Postgres user `lifeos_query`, which can only read, for 5 seconds at most. The answer has 200 rows at most.

**Use case.** You ask "on how many days did I go to the gym this month?". The agent calls `query` with a `SELECT count(*)` on `habit_logs`.

### 11. Secrets are refused

The service refuses a text that holds a card number, an Aadhaar number, a PAN, a private key or an access token. It stores nothing, and the error names the kind of secret without the secret itself. This applies to each text field: a title, a journal body, an attr, a captured text and a text file.

A password in plain words and a bank account number have no pattern, so the service cannot find them. A phone number in the form `91XXXXXXXXXX` is refused in about 1 of 10 cases, because it looks like an Aadhaar number: write it as `+91 XXXXX XXXXX`.

**Use case.** You paste a note that has your card number into the journal form. The page shows "body: must not hold a card number" and keeps your text, so you can remove the number and save.

### 12. Backup, restore test and notifications

- `deploy/backup.sh` dumps the database and copies the file store into two restic repositories: one on the Pi and one in Backblaze B2. A systemd timer runs it each night at 02:00.
- `deploy/restore-test.sh [local|b2]` loads the latest backup into a scratch database and compares the row count of each table with production.
```

In `README.md`, replace this:

```markdown
| `/admin/habits` | Log a habit for today, add, rename or retire a habit, delete a log. |
| `/admin/review` | See the records that wait, and approve them. |
```

With this:

```markdown
| `/admin/habits` | Log a habit for today, add, rename or retire a habit, delete a log. |
| `/admin/sources` | Capture a text or a link, see the sources, delete a source. |
| `/admin/review` | See the records that wait, and approve them. |
```

In `README.md`, replace this:

```markdown
| Habits | `POST /api/habits/log`, `GET /api/habits/today`, `GET`, `DELETE /api/habits/logs/{id}` |
| Review | `GET /api/review?limit=`, `POST /api/review/verify` |
```

With this:

```markdown
| Habits | `POST /api/habits/log`, `GET /api/habits/today`, `GET`, `DELETE /api/habits/logs/{id}` |
| Sources | `POST /api/ingest` (a text or a link), `POST /api/ingest/file?name=` (the file is the body), `GET`, `DELETE /api/sources/{id}` |
| Search | `GET /api/search?query=&types=&limit=` |
| Review | `GET /api/review?limit=`, `POST /api/review/verify` |
```

In `README.md`, replace this:

```markdown
| `log_habit` | Logs a habit for a day. A second call with `dictated: true` approves a proposed log. |
| `get` | Reads one record by type and ID. |
```

With this:

```markdown
| `log_habit` | Logs a habit for a day. A second call with `dictated: true` approves a proposed log. |
| `capture` | Keeps a text, a link or a file from the inbox folder as a source. |
| `search` | Finds records by their words, the newest first. |
| `query` | Runs one read-only SQL statement. |
| `get` | Reads one record by type and ID. |
```

In `README.md`, replace this:

```markdown
The record types for `get`, `update`, `delete` and `verify` are `item`, `journal`, `todo` and `habit_log`.
```

With this:

```markdown
The record types for `get`, `update`, `delete` and `verify` are `item`, `journal`, `todo`, `habit_log` and `source`. `add_item`, `todo` and `log_habit` take an optional `source_ref`.
```

In `README.md`, replace this:

```markdown
You need Go 1.26, Docker, and a `deploy/.env` file with `POSTGRES_PASSWORD` and `NTFY_BASE_URL`. The file is not in git. The backup scripts also need `RESTIC_PASSWORD`, the B2 settings and `NTFY_TOKEN`.
```

With this:

```markdown
You need Go 1.26, Docker, and a `deploy/.env` file with `POSTGRES_PASSWORD`, `LIFEOS_QUERY_PASSWORD` and `NTFY_BASE_URL`. The file is not in git. The backup scripts also need `RESTIC_PASSWORD`, the B2 settings and `NTFY_TOKEN`. You also need the two folders of the file store: `files_dir` and `inbox_dir` in `config/lifeos.yaml`.
```

In `README.md`, replace this:

```markdown
make tools      # one time: install sqlc into ~/go/bin (linux/arm64 binary)
make migrate    # generate, build, apply the migrations
```

With this:

```markdown
make tools      # one time: install sqlc into ~/go/bin (linux/arm64 binary)
make query-user # one time: make the read-only Postgres user lifeos_query
make migrate    # generate, build, apply the migrations
```

In `README.md`, replace this:

```markdown
Settings are in [config/lifeos.yaml](config/lifeos.yaml): the two listen addresses and the database URL. The password comes from `POSTGRES_PASSWORD` in the environment.
```

With this:

```markdown
Settings are in [config/lifeos.yaml](config/lifeos.yaml): the two listen addresses, the two database URLs and the file store. The passwords come from `POSTGRES_PASSWORD` and `LIFEOS_QUERY_PASSWORD` in the environment.
```

In `README.md`, replace this:

```markdown
- **Search and lists for the agent.** No MCP tool lists or searches records. The REST API has no route that lists journal entries.
- **Capture** of raw text into an inbox, and the read-only `query` database user.
- **Rejection of secrets.** The service does not yet refuse passwords, card numbers or government ID numbers in text.
- **Money tables and CSV importers.** `lifeos import-csv` and `lifeos job` return "not implemented".
```

With this:

```markdown
- **The page of a link and the text of a file.** LifeOS stores a link as its URL and a file as its bytes. It does not get the page, and it does not read the text of a PDF. No page shows or downloads a stored file.
- **The inbox job** that reads each new source and proposes records from it.
- **Lists in the REST API.** No route lists journal entries or sources. The admin pages list them, and the agent uses `search` and `query`.
- **Money tables and CSV importers.** `lifeos import-csv` and `lifeos job` return "not implemented".
```

In `README.md`, replace this:

```markdown
migrations/          goose SQL migrations
deploy/              docker-compose.yml, backup scripts, systemd units
docs/                design doc, backlog, implementation plans
```

With this:

```markdown
migrations/          goose SQL migrations
deploy/              docker-compose.yml, backup scripts, query-user.sh, systemd units
docs/                design doc, backlog, implementation plans
```

- [ ] **Step 9: Run all tests, then commit**

Run:
```bash
make test
```
Expected: the run passes. The output contains `internal/service`, `internal/api`, `internal/mcp`, `internal/admin` and `migrations`.

Run:
```bash
git add AGENTS.md README.md
git commit -m "docs: describe capture, search and query"
```

---

### Task 16: Run the new build on the Pi

**Files:** none. This task changes the real database and the server that runs.

**Interfaces:**
- Consumes: the code of Tasks 1 to 15, all committed; the user `lifeos_query` of Task 13; the folders `/var/lib/lifeos/files` and `/var/lib/lifeos/inbox`; the tmux session `lifeos-core` with the window `lifeos-serve`, where the server of plan 3 runs.
- Produces: the real database with the migrations `00003` and `00004`; the new server on 127.0.0.1:8200 and 127.0.0.1:8100.

- [ ] **Step 1: Smoke check on a throwaway database**

The check uses its own database, its own ports and its own folders below `bin/`, which is in `.gitignore`. It does not touch the real database or `/var/lib/lifeos`.

Run:
```bash
cd /home/gautam/lifeos-core
make build
docker compose -f deploy/docker-compose.yml exec -T postgres psql -U lifeos -d postgres -c 'CREATE DATABASE lifeos_smoke'
mkdir -p bin/smoke-files bin/smoke-inbox
sed "s|/lifeos?sslmode|/lifeos_smoke?sslmode|; s|8200|8290|; s|8100|8190|; s|/var/lib/lifeos/files|$PWD/bin/smoke-files|; s|/var/lib/lifeos/inbox|$PWD/bin/smoke-inbox|" config/lifeos.yaml > bin/smoke.yaml
set -a; . deploy/.env; set +a
bin/lifeos migrate -config bin/smoke.yaml
bin/lifeos serve -config bin/smoke.yaml > bin/smoke.log 2>&1 &
echo $! > bin/smoke.pid
sleep 2; cat bin/smoke.log
```
Expected: the run passes. The output contains `CREATE DATABASE`, `applied 00003_sources.sql`, `applied 00004_query_user.sql` and `lifeos is listening http=127.0.0.1:8290 mcp=127.0.0.1:8190`.

Run:
```bash
cd /home/gautam/lifeos-core
J='Content-Type: application/json'
A='Accept: application/json, text/event-stream'
mcp() { curl -s -X POST http://127.0.0.1:8190/mcp -H "$J" -H "$A" -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":$2}}"; echo; }
curl -s -X POST http://127.0.0.1:8290/api/ingest -H "$J" -d '{"url":"https://example.com/lonavala-guide"}'; echo
printf '%%PDF-1.7\n1 0 obj\nendobj\n' > bin/receipt.pdf
curl -s -X POST 'http://127.0.0.1:8290/api/ingest/file?name=receipt.pdf' -H 'Content-Type: application/pdf' --data-binary @bin/receipt.pdf; echo
printf '%%PDF-1.7\n1 0 obj\nendobj\nmore\n' > bin/smoke-inbox/policy.pdf
mcp capture '{"path":"policy.pdf","title":"Insurance policy"}'
mcp capture '{"text":"Read Dune on the train to Lonavala."}'
mcp add_item '{"kind":"book","title":"Dune","source_ref":4}'
mcp search '{"query":"lonavala"}'
mcp query '{"sql":"SELECT kind, count(*) FROM sources WHERE deleted_at IS NULL GROUP BY kind ORDER BY kind"}'
mcp query '{"sql":"DELETE FROM sources"}'
curl -s -X POST http://127.0.0.1:8290/api/journal -H "$J" -d '{"body":"my card is 4111 1111 1111 1111"}'; echo
curl -s 'http://127.0.0.1:8290/api/search?query=dune'; echo
curl -s -o /dev/null -w '/admin/sources %{http_code}\n' http://127.0.0.1:8290/admin/sources
echo "files in the store: $(find bin/smoke-files -type f | wc -l), files in the inbox: $(ls bin/smoke-inbox | wc -l)"
docker compose -f deploy/docker-compose.yml exec -T postgres psql -U lifeos -d lifeos_smoke -At \
  -c 'SELECT action, record_type, source FROM events ORDER BY id'
```
Expected: the run passes. The output contains `"kind":"link"`, `"media_type":"application/pdf"`, `"title":"Insurance policy"`, `"source_ref":4`, `"summary":"text: Read Dune on the train to Lonavala."`, `"rows":[["file","2"],["link","1"],["text","1"]]`, `cannot execute DELETE in a read-only transaction`, `must not hold a card number`, `"summary":"book: Dune"`, `/admin/sources 200`, `files in the store: 2, files in the inbox: 0` and `create|catalog_items|agent`.

The output has these parts, in this order:
- The link is source 1 and the receipt is source 2, each with `"source":"manual"` and `"verified":true`.
- The policy is source 3, with `"source":"agent"`. The file left the inbox.
- The note is source 4. The book names it: `"source_ref":4`, and `"verified":false`.
- The search has two hits: the note first, then the link.
- The count has one row for each kind. The `DELETE` has `"isError":true`.
- The journal entry with the card number is refused with the status 422. The answer does not repeat the number.
- The REST search for `dune` has the book and the note.
- The events are these five lines. The refused calls appended no event.

```
create|sources|manual
create|sources|manual
create|sources|agent
create|sources|agent
create|catalog_items|agent
```

Run:
```bash
cd /home/gautam/lifeos-core
kill "$(cat bin/smoke.pid)"; sleep 1
docker compose -f deploy/docker-compose.yml exec -T postgres psql -U lifeos -d postgres -c 'DROP DATABASE lifeos_smoke'
rm -r bin/smoke.yaml bin/smoke.log bin/smoke.pid bin/smoke-files bin/smoke-inbox bin/receipt.pdf
git status --short
```
Expected: the run passes. The output contains `DROP DATABASE`.

`git status --short` prints nothing: the smoke check left no file in the repository.

- [ ] **Step 2: Apply the migrations to the real database**

This is the first step that changes the real database. Migration `00003` adds the table `sources` and six foreign keys. Each `source_ref` in the database is NULL, so no row can break a key. Migration `00004` gives `lifeos_query` the right to read.

Run:
```bash
cd /home/gautam/lifeos-core
make migrate
```
Expected: the run passes. The output contains `applied 00003_sources.sql` and `applied 00004_query_user.sql`.

From this point, do not edit `migrations/00003_sources.sql` or `migrations/00004_query_user.sql`. A change to a table needs a new migration file.

- [ ] **Step 3: Start the new server on the real database**

The tmux window `lifeos-serve` in the session `lifeos-core` runs the server of plan 3. Stop it and start the new build there. `make run` gives the server the two passwords from `deploy/.env`.

Run:
```bash
tmux send-keys -t lifeos-core:lifeos-serve C-c
tmux send-keys -t lifeos-core:lifeos-serve 'make run' Enter
sleep 30; tmux capture-pane -p -t lifeos-core:lifeos-serve | tail -3
curl -s -o /dev/null -w '/admin/sources %{http_code}\n' http://127.0.0.1:8200/admin/sources
```
Expected: the run passes. The output contains `lifeos is listening http=127.0.0.1:8200 mcp=127.0.0.1:8100` and `/admin/sources 200`.

If the window does not exist, run `make run` in a tmux pane that stays open. If the server stops with `query_database_url`, run `make query-user`, then start the server again.

- [ ] **Step 4: Hand over to Gautam and stop**

Give Gautam these four checks.

1. **Admin pages.** On the Mac, run `ssh -L 8200:127.0.0.1:8200 <the Pi>` and open `http://127.0.0.1:8200/admin/sources`.
   - Capture a text that has two paragraphs, with a title. The list must show the start of the text.
   - Capture a link. The list must show the link. Capture the same link again: the list must still have one row for it.
   - Type a text with the test card number `4111 1111 1111 1111` and click Capture. The page must show `text: must not hold a card number`, and the form must keep the text.
   - Open Journal and add an entry that has a phone number as `+91 98765 43210`, a date and an amount. The entry must be stored.
2. **MCP from Claude Code on the Pi.** Start a new `claude` session in the folder that has the `lifeos` server. A session that was open before Step 3 does not have the new tools.
   - Say "capture this note: the car insurance renews on 12 March, policy with Acme". Then say "what do I have about insurance?". Claude must find the note with `search`.
   - Say "how many todos are open, and how many are done?". Claude must answer with a number from `query`.
   - Copy a PDF to `/var/lib/lifeos/inbox/` and say "capture the file <its name> from the inbox". The file must show on the Sources page with its type and size, and it must leave the inbox.
   - Say "From that note, propose a todo to renew the insurance; I did not state it as a todo." The todo must show at `http://127.0.0.1:8200/admin/review`, and `source_ref` must name the note: ask Claude to show the todo.
3. **REST, as the share sheet Shortcut does.** Run these two commands on the Pi. The first response must have `"kind":"link"`. The second must have a hit for the link.

```bash
curl -s -X POST http://127.0.0.1:8200/api/ingest -H 'Content-Type: application/json' -d '{"url": "https://example.com/article"}'; echo
curl -s 'http://127.0.0.1:8200/api/search?query=example.com'; echo
```

4. **The backup.** The backup of tonight (02:00) is the first that has the file store. Its ntfy message must say that `lifeos-backup.service` succeeded. To test it now, run `sudo systemctl start lifeos-backup.service` and wait for the message.

The confirmation dialog of Delete, the reset of the form after Capture and the look of the page depend on a real browser. No automated test covers them.

Stop here. Do not start the work in "After this plan" until Gautam confirms that the checks pass.

---

## After this plan

These are the parts of Phase 2 that this plan does not cover. Each becomes its own plan.

| Plan | Content | Spec sections |
| --- | --- | --- |
| 5. Money and importers | `accounts`, `transactions`, `budgets`. `lifeos import-csv` for a bank statement, Letterboxd and Goodreads, with duplicate detection. Each new table: add its name to the description of the `query` tool. | Data model; S6, S11 |
| 6. Run and protect | A systemd unit for `lifeos serve` and `deploy/deploy.sh` (with `make query-user`). The admin password, the per-device API tokens and CSRF protection. Tunnel hostnames with Cloudflare Access; the Host header setting for the MCP server. Permanent delete in the admin pages, which also removes a stored file. | Deployment and security; S25, S26 |

These parts of capture belong to the plan with the inbox job (Phase 4 in the design doc): the fetch of the page of a link, with a guard against requests to private addresses; the text of a PDF (`pdftotext` needs an install on the Pi); the function that sets `processed_at`; a page that shows or downloads a stored file.

`docs/backlog.md` holds work that is in no plan yet: a schema for the `attrs` of each item kind, duplicate detection for items, a `description` field for todos, and a `projects` table.

Phase 2 ends with the gate in the design doc: one week of daily logging through the admin pages. The `wiki_*` tools, `brief`, OAuth for the MCP endpoint, the jobs and the connectors belong to Phases 3 to 5.
