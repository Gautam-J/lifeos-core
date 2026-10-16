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
cmd/lifeos/            main package: serve, migrate, job, import-csv, instructions, version
internal/service/      all logic: validation, provenance, events
internal/store/        sqlc-generated queries, from queries/*.sql (not in git)
internal/config/       loads config/lifeos.yaml
internal/version/      the release of the binary; the Makefile sets it from `git describe`
internal/testdb/       test helper: one migrated database for each test
internal/testaccess/   test helper: a stand-in for Cloudflare Access that signs tokens
internal/api/          chi + Huma handlers
internal/mcp/          tools on the official MCP Go SDK
internal/admin/        templ + htmx admin pages; the styles and the fonts are in static/
internal/connectors/   health, caldav, imap, owntracks, csv: one plugin each
internal/jobs/         briefs, inbox processing, lint
queries/               SQL that sqlc compiles
migrations/            goose SQL migrations and the Go code that applies them
config/lifeos.yaml     listen addresses, database URLs, file store, wiki folder; later models, schedules, connectors, notifications
sqlc.yaml              sqlc settings
Makefile               tools, query-user, generate, build, test, migrate, run
dashboards/            Grafana definitions, versioned with the schema they query
monitoring/            the config files of Prometheus, Grafana, Loki, Tempo and the OTel collector, and later of the other monitoring services
deploy/                docker-compose.yml, systemd units and timers, backup scripts, query-user.sh, postgres-test-init.sh, deploy.sh, check-access.sh, wiki-push.sh, wiki-push-repair.sh, pi-textfile.sh, test.sh
skills/                drafts of agent skills; no agent loads them (skills/README.md)
prompts/               the prompts of the brief jobs (Phase 4)
docs/design.md         the design doc, exported
.github/               the forms of a GitHub issue: bug, feature request
DESIGN.md              the look of the admin pages: colors, type, spacing, components
CHANGELOG.md           the changes of each release; `cz bump` writes it
.cz.toml               the commitizen settings and the number of the last release
assets/                the pictures of README.md; architecture.svg is the system design, so change it when a part of the system changes
```

Create a folder when its first file lands; do not add empty packages.

## Rules

1. **All logic lives in `internal/service`.** Validation, provenance and event logging happen there and nowhere else.
2. **Adapters are thin.** API handlers, MCP tools and CLI subcommands parse input, call one service function and format the result. No adapter calls another adapter. Handlers deal with HTTP; tools carry the model-facing descriptions.
3. **SQL only through sqlc.** Write queries in `queries/*.sql` and call the generated code in `internal/store`. No SQL strings in Go, and only `internal/service` calls `internal/store`. The one exception is `Query`, which runs the SQL that an agent wrote (rule 13).
4. **Every write records provenance.** Each record row carries `source` (`manual`, `import`, `connector` or `agent`), `created_at` and `updated_at`. The adapter sets the source: the REST API and the admin pages write `manual` rows, an MCP tool writes `agent` rows. A record that an agent writes is a record at once: there is no approval step (#40). The guards are the events (rule 5), the soft delete with a confirmation (rule 6), and the reply of the agent, which names each record that it wrote with its ID. A record that a journal entry reports names the entry in `journal_ref` (`habit_logs`, `todos`, `catalog_items`); the service refuses a `journal_ref` that names no live entry.
5. **Every write appends to `events`.** One row per write, with the old value on updates and deletes. `events` is append-only. A call that changes no value is not a write: it stores nothing and appends no event. The one exception is `Purge`: it empties the old and the new value of each earlier event of the record that it removes. The rows stay, and `Purge` appends a `purge` event that has no value.
6. **Deletes are soft.** Set `deleted_at`; permanent removal happens only in the admin pages. The service refuses a delete from an agent unless the call says that Gautam confirmed it. The page `/admin/deleted` lists the deleted records (`ListDeleted`) and removes one for good (`Purge`). `Purge` takes only a record that is deleted already. No REST route and no MCP tool calls it. `Purge` refuses a journal entry that a record names in `journal_ref`.
7. **Never store** passwords, card or bank account numbers, or government ID numbers. `checkText` in `internal/service/fields.go` refuses the ones that have a pattern: a card number, an Aadhaar number, a PAN, a private key and an access token (`secrets.go`). Send each text that a caller gives through `checkText` before the service stores it. The error names the kind of secret and never repeats it. A password in plain words and a bank account number have no pattern: the tool descriptions tell the agent not to send them.
8. **Store times in UTC** (`timestamptz`); display in IST.
9. **A record type has two entry points.** A REST route or an admin page knows its type and calls the function of that type (`GetItem`, `UpdateItem`, `DeleteItem`). The MCP tools `get`, `update` and `delete` get the type as input and call `Get`, `Update` and `Delete` in `internal/service/records.go`. The REST `PATCH` route calls `Update` too, because only a map of fields can tell a null from a field that is absent. These refuse a field that a caller must not set, such as `source`. Add each new record type to `RecordTypes`, to these three functions, to `Purge` and to `ListDeleted`. The record types are `item`, `journal`, `todo`, `habit_log` and `source`. A type that has text also gets a query in `queries/search.sql` and a case in `Search`. A habit (the definition) is not a record type: only the admin pages add or change one.
10. **The journal holds Gautam's own words.** The service stores the body of a journal entry exactly as it comes and never rewords it. An admin handler changes the CR LF of a text area to LF, which is what Gautam typed.
11. **A habit has one log for each day.** A second log for the same habit and day is not a write: the service returns the stored log. "Done" for a day means a live log.
12. **A source is verbatim.** A source is one row of `sources`: a text, a link or a file that `Capture` stores exactly as it comes. The service never changes the content of a source, so `update` refuses the type `source`. `MarkProcessed` sets the time `processed_at`, when the work on the source is complete, and it changes nothing else. The same content a second time is not a write: `Capture` returns the stored source. No record comes from a source: its ideas go into the wiki and cite `(src:N)`. A file is in the file store at `<files_dir>/<first two characters of its SHA-256>/<SHA-256>`; a soft delete keeps the file. `Purge` removes the file, unless a different source uses it.
13. **`query` reads and never writes.** `Query` runs one SQL statement of an agent on a connection of the Postgres user `lifeos_query`, which has only the right to SELECT, in a read-only transaction. The database refuses a change; the Go code does not look at the SQL. `UseQueryPool` refuses a pool whose user can change a table. A new table is closed to `query` until its migration gives the right: `GRANT SELECT ON <table> TO lifeos_query`. Give the right to each table that an agent can read, and add the name of the table to the description of the `query` tool, which a test checks. Give no right to a table that holds a secret, such as a token or a password hash; the description names that table as one that `query` cannot read.
14. **The wiki has one writer, and it holds no fact of its own.** A wiki page is a Markdown file below `wiki_dir`, in a git repository. `WikiWrite`, `WikiAppend` and `WikiDelete` are the only code that changes a file there. Each one writes `index.md` again, appends one event (`record_type` is `wiki`, no record ID, the path and the hash as the value) and makes one commit. When the commit fails, the files go back to what they were. A fact is a record in the database; a page holds analysis and cites the row, as `(journal:57)`. The prefixes are in `WikiCitationPrefixes`: add the prefix of each new record type there and to `citationIsLive`. A write refuses a new citation of a row that is not live; `WikiLint` reports a citation whose row was deleted later. `WikiWrite` needs the hash of the stored page, so that one session does not overwrite the change of a different session. No REST route and no admin page writes the wiki. The rules for an agent are in the tool descriptions, in the `next` field of the results of `journal` and `capture`, and in `internal/mcp/instructions.go`. The Claude app does not give the server instructions to the model, so no rule can be only there.

## Admin pages

- **The look comes from `DESIGN.md`.** The tokens (colors, type, radius, spacing) are the custom properties at the top of `internal/admin/static/app.css`. Use a token; do not write a new color or size into a rule. A serif font is for a headline and for a large number, the sans font for all other text, and the coral color for the one primary button of a form and for the callout card.
- **Each page uses `page(title, section)`** in `layout.templ`: the menu, the frame and the footer. `section` is the menu entry that the page belongs to. Add a new page to `menu` there.
- **Each page must work on a phone, a tablet, a laptop and a 4K monitor.** A list is a table with the class `cards`: below 768px each row becomes a card, and a cell shows the label of its `data-label` attribute. Sizes are in `rem`, because the root size grows on a wide monitor.
- **The summary page `/admin` shows today and yesterday in IST**, from the one service function `Overview`. Its forms post to the routes of the record types. htmx sends `HX-Target: today`, and the handler then answers with the summary (`onToday`), not with its list. The two forms have `hx-preserve`, so a write does not remove what Gautam typed in the other form.
- **The address of a static file has the hash of its content** (`asset`), and the server lets the browser keep only such an address. Without that, Cloudflare and the browser show the old styles for hours after a deploy. A font has no hash: give a new font a new file name.
- **The pages have a light and a dark theme.** They follow the setting of the system. The footer has the choice System, Light or Dark; `static/theme.js` keeps a choice in `localStorage` of the browser and sets `data-theme` on the page. The server stores no theme and gets none. The dark values of the tokens are in `app.css` two times, for the system setting and for the choice; a test checks that the two blocks are equal. Give each new color a value in the two themes.
- A page loads no file from a different site. The fonts are in `static/fonts`, with their license.

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
- **Tests use their own Postgres server.** It is the container `postgres-test` of `deploy/docker-compose.yml`, on port 5433. Its data is in memory: one run of the tests writes about 2.5 GB, which must not go to the SD card, and the tests must not use the server that holds the real data. The container makes the read-only user when it starts; the Makefile reads `deploy/.env`. Each test creates its own database and drops it. `go test` without `make` fails, because `LIFEOS_TEST_DATABASE_URL` is not set.
- **Narrow a test run** with `make test PKG=./internal/api ARGS="-run TestName -v"`.

`lifeos serve` runs the API and admin pages on `http_addr` and the MCP server on `mcp_addr`, path `/mcp`. On the Pi it runs as the systemd unit `deploy/systemd/lifeos.service`; `make run` is for work on the code. It needs the folders `files_dir`, `inbox_dir` and `wiki_dir` of the config file, the program `git`, and four values in the environment: the two passwords `POSTGRES_PASSWORD` and `LIFEOS_QUERY_PASSWORD`, and `CF_ACCESS_TEAM` and `LIFEOS_MCP_AUD` for the token check of the MCP server. `lifeos migrate`, `lifeos job <name>` and `lifeos import-csv <importer> <file>` are run by hand or by systemd timers. Each command reads `config/lifeos.yaml`, or the file given with `-config`.

## Issues

The GitHub issues of this repository are the one list of the work that is in no plan: each bug and each feature request. No file of the repository holds a backlog.

- **One issue for one problem or one request.** A bug has the label `bug`. A request has the label `enhancement`. The two forms in `.github/ISSUE_TEMPLATE/` set the label.
- **Read the list with `gh issue list`.** Correct a bug with a test that fails first.
- **The commit closes the issue.** Put `Fixes #12` into the body of the commit message, or `Closes #12` for a request. GitHub closes the issue when the commit is on `main` there.
- **Keep private content out of an issue**: no journal text, no hostname, no domain and no value of Cloudflare Access. An issue becomes public when the repository becomes public.

## Releases

The version comes from the commit messages, so each commit must follow the conventional format. commitizen (`cz`) makes a release; its settings are in `.cz.toml`.

- **Which number changes.** A `fix` raises the patch number: 0.1.0 to 0.1.1. A `feat` raises the minor number: 0.1.1 to 0.2.0. A `docs`, `chore`, `test` or `build` commit alone makes no release. Before 1.0.0, a breaking change raises the minor number too.
- **One deploy is one release.** When the commits of a change are on `main` and `make test` passes, run `cz bump`. It writes the new number into `.cz.toml`, adds the release to `CHANGELOG.md`, makes the commit `chore(release): v<number>` and makes the tag `v<number>`. Then run `deploy/deploy.sh`. Push with `git push --follow-tags`, so that the tag goes with the commit.
- **Tag first, then build.** The Makefile gives the result of `git describe` to `internal/version`. `lifeos version` prints it, and the footer of the admin pages shows it. A build that comes before `cz bump` has the name of the release before, with the number of commits after it.
- **Do not add a release to `CHANGELOG.md` by hand.** `cz bump` adds it. A correction of the text of an earlier release stays, because `cz bump` only adds.
- **1.0.0 is a manual step**, for the time when each planned feature is live. Remove `major_version_zero` from `.cz.toml`, then run `cz bump 1.0.0`.

Install commitizen one time, for your user only (Debian refuses a system-wide `pip install`):

```sh
python3 -m venv ~/.local/share/commitizen
~/.local/share/commitizen/bin/pip install commitizen
ln -s ~/.local/share/commitizen/bin/cz ~/.local/bin/cz
```

## Migrations

- One goose SQL file per change, `migrations/NNNNN_name.sql`, with `-- +goose Up` and `-- +goose Down`.
- Every new record table gets the provenance columns (`source`, `created_at`, `updated_at`), `deleted_at`, and the `set_updated_at()` trigger. Copy them from `catalog_items` in `00001_init.sql`, without `source_ref` and `verified`, which 00005 and 00006 dropped. Give `lifeos_query` the right to read the table (rule 13).
- Never edit a migration that has been applied; add a new one.
- A migration that drops or renames a column runs only after a fresh `pg_dump`.

## Deploy

`deploy/deploy.sh` on the Pi: `git pull`, build, dump the database, `lifeos migrate`, restart `lifeos.service`, then `deploy/check-access.sh`. Every service listens on `127.0.0.1` only; the Cloudflare Tunnel is the single way in.

**LifeOS has no login of its own.** Cloudflare Access is the only lock on the four public hostnames: one for the admin pages (a login with an email code), one for the REST API (an Access service token for each device), one for the MCP server (the OAuth login that Access gives to an MCP client) and one for the dashboards of Grafana (a login with an email code). The names are in `deploy/.env`. So:

- Never add a hostname to the tunnel before its Access application exists.
- Run `deploy/check-access.sh` after each change of the tunnel or of an Access application. It fails when a hostname answers a request that has no login. Only an answer that Access alone gives is proof: the redirect to its login page, and 401 for the API hostname and the MCP hostname. `deploy/test.sh` holds the cases.
- Give each tunnel rule of a LifeOS hostname an `access` part: `required: true`, the team name and the audience tag of its Access application. Then `cloudflared` checks the token of Access itself, and the hostname stays closed when its application is removed.
- The unit gives the server only the two database passwords, the two Access values of the MCP server, the binary, the config and the folder `/var/lib/lifeos`, which holds the file store, the inbox and the repository of the wiki. Keep the wiki there: a process that can write a git repository can make git run a program for the next user of that repository, so the server must not write a repository in the home folder. Add each new key of `deploy/.env` to `UnsetEnvironment` in `deploy/systemd/lifeos.service`; `deploy/test.sh` checks the list against `deploy/.env.example`.
- **Grafana has no login of its own.** It logs a person in from the token of Access (`GF_AUTH_JWT_*`): it checks the signature with the keys of the team (`CF_ACCESS_TEAM`) and the audience tag of the dash application (`LIFEOS_DASH_AUD`), and gives the user of the email the role Admin. A request with no token, such as one from the Pi itself, is the anonymous user, also an admin. So Grafana listens on `127.0.0.1` only, and its hostname has the Access login. Keep `GF_SECURITY_CSRF_ALWAYS_CHECK`: with no login cookie, Grafana does not check the origin of a write, and a page of a different site can then write with the Access login of the browser. Prometheus deletes no data: `--storage.tsdb.retention.time=100y` and no size limit. Loki and Tempo delete no data either (`docs/telemetry.md`). The nightly backup has the data of the four, without the plugins of Grafana, which Grafana downloads again.
- **The OTel hostname has no Access application**, as ntfy has none: Claude Code sends one header only. Its lock is the bearer token `OTEL_BEARER_TOKEN`, which the OTel collector checks, and the tunnel sends only `/v1/metrics`, `/v1/logs` and `/v1/traces`. `deploy/check-access.sh` checks that a request with no token or a wrong token gets 401. The collector removes secrets with a copy of the patterns of `internal/service/secrets.go`: change both together (rule 7).
- `serve.go` puts `http.CrossOriginProtection` in front of the admin pages, the API and the MCP server. It refuses a request that changes data and that a browser sent from a different origin. Keep it: without it, a page of another site can use the Access login of the browser.
- **The MCP server checks the token of Access itself.** `AccessCheck` in `internal/mcp/access.go` answers 403 to each request that has no valid token of the Access application of the MCP hostname. The team name and the audience tag come from `CF_ACCESS_TEAM` and `LIFEOS_MCP_AUD`; the server does not start without them. Each client uses the public hostname, also Claude Code on the Pi: a request straight to `mcp_addr` has no token. The check replaces the Host guard of the MCP library, which is off because the tunnel sends the public hostname to a loopback address. So never turn the check off, and add no MCP route that does not go through `Handler`. The tunnel rule of the MCP hostname sends only the path `/mcp`.
- **The server does not push the wiki.** `deploy/wiki-push.sh` copies the commits to GitHub each hour (`lifeos-wiki-push.timer`). It fetches from the live repository into a mirror that the server cannot write, and it pushes from the mirror. It pushes only commits that change files below `wiki/`: the server writes nothing else, and a clone of the wiki can hold settings that a program reads, such as `.claude/settings.json`. Never run a git command inside the live repository as a user that has keys: the server can write that repository (see above). GitHub gets commits only from the mirror. A commit that reaches GitHub from a different place stops the push, and ntfy gets the failure. `deploy/wiki-push-repair.sh` is the fix: it keeps that commit on a branch `from-github-<time>` and puts the history of the Pi on `main`. It does not merge, because a merge is a write of the wiki that the server did not make (rule 14).
