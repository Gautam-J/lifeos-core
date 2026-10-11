# AGENTS.md

How to work on the `lifeos-core` code. The full design is in [docs/design.md](docs/design.md); read the section you are changing before you change it.

This repo is the service code only. It holds no personal data. The wiki pages are in the separate `lifeos-wiki` repo, which the server writes (rule 14).

## Stack

One Go binary, `lifeos`, runs on a Raspberry Pi 4 (linux/arm64) against Postgres.

| Concern | Choice |
| --- | --- |
| HTTP router and REST API | chi + Huma (request validation from Go types, OpenAPI spec) |
| Database driver | pgx |
| Queries | sqlc: SQL in `queries/*.sql`, generated Go in `internal/store/` |
| Migrations | goose SQL files in `migrations/` |
| MCP server | official MCP Go SDK (`github.com/modelcontextprotocol/go-sdk`), v1.7 or later |
| Admin pages | templ + htmx, server-rendered, served by the same binary |
| Config | one file, `config/lifeos.yaml` |

## Layout

```
cmd/lifeos/            main package: serve, migrate, job, import-csv
internal/service/      all logic: validation, provenance, events
internal/store/        sqlc-generated queries, from queries/*.sql (not in git)
internal/config/       loads config/lifeos.yaml
internal/testdb/       test helper: one migrated database for each test
internal/api/          chi + Huma handlers
internal/mcp/          tools on the official MCP Go SDK
internal/admin/        templ + htmx admin pages
internal/connectors/   health, caldav, imap, owntracks, csv: one plugin each
internal/jobs/         briefs, inbox processing, lint
queries/               SQL that sqlc compiles
migrations/            goose SQL migrations and the Go code that applies them
config/lifeos.yaml     listen addresses, database URLs, file store, wiki folder; later models, schedules, connectors, notifications
sqlc.yaml              sqlc settings
Makefile               tools, query-user, generate, build, test, migrate, run
dashboards/            Grafana definitions, versioned with the schema they query
deploy/                docker-compose.yml, systemd units and timers, backup scripts, query-user.sh, deploy.sh, check-access.sh, test.sh
docs/design.md         the design doc, exported
```

Create a folder when its first file lands; do not add empty packages.

## Rules

1. **All logic lives in `internal/service`.** Validation, provenance and event logging happen there and nowhere else.
2. **Adapters are thin.** API handlers, MCP tools and CLI subcommands parse input, call one service function and format the result. No adapter calls another adapter. Handlers deal with HTTP; tools carry the model-facing descriptions.
3. **SQL only through sqlc.** Write queries in `queries/*.sql` and call the generated code in `internal/store`. No SQL strings in Go, and only `internal/service` calls `internal/store`. The one exception is `Query`, which runs the SQL that an agent wrote (rule 13).
4. **Every write records provenance.** Each record row carries `source` (`manual`, `import`, `connector` or `agent`), `source_ref`, `verified`, `created_at` and `updated_at`. Rows an agent infers start with `verified = false`; rows Gautam dictates directly are verified. The adapter sets this: the REST API and the admin pages write `manual`, verified rows; an MCP tool writes `agent` rows that are verified only when the tool input says the user dictated the row. An update keeps `verified`. The service refuses a change that an agent infers to a verified row: the agent asks Gautam, then sends the change as dictated. Only `Verify` sets `verified` to `true`.
5. **Every write appends to `events`.** One row per write, with the old value on updates and deletes. `events` is append-only. A call that changes no value is not a write: it stores nothing and appends no event. The one exception is `Purge`: it empties the old and the new value of each earlier event of the record that it removes. The rows stay, and `Purge` appends a `purge` event that has no value.
6. **Deletes are soft.** Set `deleted_at`; permanent removal happens only in the admin pages. The service refuses a delete from an agent unless the call says that Gautam confirmed it. The page `/admin/deleted` lists the deleted records (`ListDeleted`) and removes one for good (`Purge`). `Purge` takes only a record that is deleted already. No REST route and no MCP tool calls it.
7. **Never store** passwords, card or bank account numbers, or government ID numbers. `checkText` in `internal/service/fields.go` refuses the ones that have a pattern: a card number, an Aadhaar number, a PAN, a private key and an access token (`secrets.go`). Send each text that a caller gives through `checkText` before the service stores it. The error names the kind of secret and never repeats it. A password in plain words and a bank account number have no pattern: the tool descriptions tell the agent not to send them.
8. **Store times in UTC** (`timestamptz`); display in IST.
9. **A record type has two entry points.** A REST route or an admin page knows its type and calls the function of that type (`GetItem`, `UpdateItem`, `DeleteItem`). The MCP tools `get`, `update` and `delete` get the type as input and call `Get`, `Update` and `Delete` in `internal/service/records.go`. The REST `PATCH` route calls `Update` too, because only a map of fields can tell a null from a field that is absent. These refuse a field that a caller must not set, such as `verified` or `source`. Add each new record type to `RecordTypes`, to these three functions, to `Verify`, to `ReviewQueue`, to `Purge` and to `ListDeleted`. The record types are `item`, `journal`, `todo`, `habit_log` and `source`. A type that has text also gets a query in `queries/search.sql` and a case in `Search`. A habit (the definition) is not a record type: only the admin pages add or change one.
10. **The journal holds Gautam's own words.** The service stores the body of a journal entry exactly as it comes and never rewords it. The `journal` tool writes a verified row, because no agent infers a journal entry. An admin handler changes the CR LF of a text area to LF, which is what Gautam typed.
11. **A habit has one log for each day.** A second log for the same habit and day is not a write: the service returns the stored log. The one exception: a verified log over a log that is not verified approves that log. "Done" for a day means a verified log.
12. **A source is verbatim.** A source is one row of `sources`: a text, a link or a file that `Capture` stores exactly as it comes. The service never changes the content of a source, so `update` refuses the type `source`. `MarkProcessed` sets the time `processed_at`, when the work on the source is complete, and it changes nothing else. The same content a second time is not a write: `Capture` returns the stored source. A source is always verified, because it holds what came in and no agent inferred it. A record that an agent takes from a source names it in `source_ref`. A file is in the file store at `<files_dir>/<first two characters of its SHA-256>/<SHA-256>`; a soft delete keeps the file. `Purge` removes the file, unless a different source uses it. `Purge` refuses a source that a record names in `source_ref`.
13. **`query` reads and never writes.** `Query` runs one SQL statement of an agent on a connection of the Postgres user `lifeos_query`, which has only the right to SELECT, in a read-only transaction. The database refuses a change; the Go code does not look at the SQL. `UseQueryPool` refuses a pool whose user can change a table. A new table is closed to `query` until its migration gives the right: `GRANT SELECT ON <table> TO lifeos_query`. Give the right to each table that an agent can read, and add the name of the table to the description of the `query` tool, which a test checks. Give no right to a table that holds a secret, such as a token or a password hash; the description names that table as one that `query` cannot read.
14. **The wiki has one writer, and it holds no fact of its own.** A wiki page is a Markdown file below `wiki_dir`, in a git repository. `WikiWrite`, `WikiAppend` and `WikiDelete` are the only code that changes a file there. Each one writes `index.md` again, appends one event (`record_type` is `wiki`, no record ID, the path and the hash as the value) and makes one commit. When the commit fails, the files go back to what they were. A fact is a record in the database; a page holds analysis and cites the row, as `(journal:57)`. The prefixes are in `WikiCitationPrefixes`: add the prefix of each new record type there and to `citationIsLive`. A write refuses a new citation of a row that is not live; `WikiLint` reports a citation whose row was deleted later. `WikiWrite` needs the hash of the stored page, so that one session does not overwrite the change of a different session. No REST route and no admin page writes the wiki. The rules for an agent are in the tool descriptions, in the `next` field of the results of `journal` and `capture`, and in `internal/mcp/instructions.go`. The Claude app does not give the server instructions to the model, so no rule can be only there.

## Commands

```sh
make tools      # one time: install sqlc into ~/go/bin
make query-user # one time: make the read-only Postgres user lifeos_query
make generate   # run sqlc and templ: writes internal/store/ and *_templ.go
make build      # generate, then build bin/lifeos
make test       # generate, deploy/test.sh, go vet, then all tests
make migrate    # build, then apply migrations to the database in config/lifeos.yaml
make run        # build, then run lifeos serve
```

- **Generated code is not in git.** After a fresh clone, run `make generate` before `go build` or `go vet`.
- **Tests need Postgres and the read-only user.** Start the container from `deploy/docker-compose.yml`, then run `make query-user` one time; the Makefile reads `deploy/.env`. Each test creates its own database and drops it. `go test` without `make` fails, because `LIFEOS_TEST_DATABASE_URL` is not set.
- **Narrow a test run** with `make test PKG=./internal/api ARGS="-run TestName -v"`.

`lifeos serve` runs the API and admin pages on `http_addr` and the MCP server on `mcp_addr`, path `/mcp`. On the Pi it runs as the systemd unit `deploy/systemd/lifeos.service`; `make run` is for work on the code. It needs the folders `files_dir`, `inbox_dir` and `wiki_dir` of the config file, the program `git`, and the two passwords `POSTGRES_PASSWORD` and `LIFEOS_QUERY_PASSWORD` in the environment. `lifeos migrate`, `lifeos job <name>` and `lifeos import-csv <importer> <file>` are run by hand or by systemd timers. Each command reads `config/lifeos.yaml`, or the file given with `-config`.

## Migrations

- One goose SQL file per change, `migrations/NNNNN_name.sql`, with `-- +goose Up` and `-- +goose Down`.
- Every new record table gets the provenance columns, `deleted_at`, and the `set_updated_at()` trigger. Copy them from `catalog_items` in `00001_init.sql`. Give `source_ref` a foreign key to `sources`, as `00003_sources.sql` does, and give `lifeos_query` the right to read the table (rule 13).
- Never edit a migration that has been applied; add a new one.
- A migration that drops or renames a column runs only after a fresh `pg_dump`.

## Deploy

`deploy/deploy.sh` on the Pi: `git pull`, build, dump the database, `lifeos migrate`, restart `lifeos.service`, then `deploy/check-access.sh`. Every service listens on `127.0.0.1` only; the Cloudflare Tunnel is the single way in.

**LifeOS has no login of its own.** Cloudflare Access is the only lock on the two public hostnames: one for the admin pages (a login with an email code) and one for the REST API (an Access service token for each device). The names are in `deploy/.env`. So:

- Never add a hostname to the tunnel before its Access application exists.
- Run `deploy/check-access.sh` after each change of the tunnel or of an Access application. It fails when a hostname answers a request that has no login. Only an answer that Access alone gives is proof: the redirect to its login page, and 401 for the API hostname. `deploy/test.sh` holds the cases.
- Give each tunnel rule of a LifeOS hostname an `access` part: `required: true`, the team name and the audience tag of its Access application. Then `cloudflared` checks the token of Access itself, and the hostname stays closed when its application is removed.
- The unit gives the server only the two database passwords, the binary, the config and the folder `/var/lib/lifeos`, which holds the file store, the inbox and the repository of the wiki. Keep the wiki there: a process that can write a git repository can make git run a program for the next user of that repository, so the server must not write a repository in the home folder. Add each new key of `deploy/.env` to `UnsetEnvironment` in `deploy/systemd/lifeos.service`; `deploy/test.sh` checks the list against `deploy/.env.example`.
- `serve.go` puts `http.CrossOriginProtection` in front of the admin pages and the API. It refuses a request that changes data and that a browser sent from a different origin. Keep it: without it, a page of another site can use the Access login of the browser.
- The MCP listener has no public hostname. Give it one only after the MCP handler checks the token of Access itself (Phase 3).
