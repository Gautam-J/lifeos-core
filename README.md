# lifeos-core

LifeOS is a self-hosted second brain. One Go binary, `lifeos`, runs on a Raspberry Pi 4 and stores personal records in Postgres. A person uses the admin pages. A program uses the REST API. An AI agent uses the MCP server (MCP is the Model Context Protocol, which gives an agent a set of tools).

This repo holds the service code only. It holds no personal data.

- **Design:** [docs/design.md](docs/design.md)
- **Rules for code changes:** [AGENTS.md](AGENTS.md)
- **Bugs and feature requests:** [the issues on GitHub](https://github.com/Gautam-J/lifeos-core/issues). A bug has the label `bug`, and a request has the label `enhancement`.
- **Changes of each release:** [CHANGELOG.md](CHANGELOG.md)

## Architecture

![The system design of LifeOS: three clients, Cloudflare Access and the tunnel, the lifeos binary on the Raspberry Pi with its three adapters and its service layer, the three stores, and the backup, the push of the wiki and the notifications](assets/architecture.svg)

Read the diagram from the top to the bottom:

1. **Clients.** A browser, a program and an AI agent each use one hostname.
2. **Cloudflare.** Access guards each hostname. The tunnel carries a request to the Pi only after Access let it pass.
3. **The `lifeos` binary.** The admin pages, the REST API and the MCP server are thin adapters. Each one calls the service layer, which holds all logic and is the only write path.
4. **Storage.** Postgres holds the records and the event log. The file store holds the captured files. A git repository holds the wiki.
5. **Operations.** A timer makes the backup each night, a timer pushes the wiki to GitHub each hour, and ntfy sends a message when a unit fails.

A box with a dashed border shows a part that is not built yet.

## Status

| Phase | State |
| --- | --- |
| 1 · Foundation | Done: Docker, Postgres, ntfy, nightly backup, restore test. |
| 2 · Core data | Built: lists, journal, todos, habits, capture, search, the read-only query, permanent delete, the three interfaces, the service unit and the public hostnames. The week of daily use is now the gate of Phase 3. Money tables and imports are [issues](https://github.com/Gautam-J/lifeos-core/issues). |
| 3 · Agents and phone | Built: the wiki tools, the public hostname of the MCP server with the login of Cloudflare Access, and the push of the wiki to GitHub. The gate is open: one week of daily use from the phone through the agent. |
| 4 to 5 | Not started. |

The section [Not built yet](#not-built-yet) lists what is missing.

## What you can do

Each function below has one use case. All three interfaces call the same service code, so the same checks apply to each of them.

The record types are `item`, `journal`, `todo`, `habit_log` and `source`.

### 1. Lists: movies, books, places, games

A list item has a kind (`movie`, `book`, `place` or `game`), a title, an optional rating from 0 to 5 in steps of 0.5, an optional day, and free `attrs` such as author or year.

**Use case.** You watched a movie last night. Open `/admin/items`, select `movie`, type the title, give 4 stars and the day. You can also tell the agent "add Dune, 4 stars, yesterday"; the agent calls `add_item`.

### 2. Journal

A journal entry has a body and the day that the entry is about. The service stores the body exactly as you wrote it. The default day is today in India Standard Time (IST). A day in the future is refused.

**Use case.** At the end of the day, type some lines in `/admin/journal`. Or dictate the text to the agent, which calls `journal`. The agent must send your words without a change, and it must not write an entry of its own.

### 3. Todos

A todo has a title, a status (`open`, `done` or `dropped`), an optional due day and an optional project. The time of completion (`done_at`) follows the status. Lists show the open todos first.

**Use case.** In a conversation you say "remind me to renew the car insurance by the 20th". The agent calls `todo` with the title and `due_on`. Later you press **Done** in `/admin/todos`.

### 4. Habits

A habit is a name, such as `gym`. A habit log records that you did the habit on one day. A habit has one log for each day, so a second log for the same day stores nothing.

- You add, rename and retire a habit in `/admin/habits` only. To retire a habit, clear its **active** box. An agent cannot add a habit.
- A log for an unknown or retired habit is refused. The error lists the active habits.
- "Done" for a day means a live log, also a log that the agent wrote.

**Use case.** You add the habits `gym` and `read` one time. Each evening you open `/admin/habits` and press the button for each habit of today. Or you say "I went to the gym"; the agent calls `log_habit`.

### 5. Provenance and the journal link

Each record has a `source` (`manual`, `import`, `connector` or `agent`), `created_at` and `updated_at`. A record that an agent writes is a record at once: nothing waits for approval. The agent names each record that it wrote, with its ID, and the events keep each old value, so a wrong record is easy to correct or delete.

A habit log, a todo and an item that a journal entry reports name the entry in `journal_ref`. The lists of the admin pages show it as `journal:57`, with a link to the entry.

**Use case.** You write "went for a run, and booked the bike service". The agent stores the entry, logs the habit `run` for the day of the entry, closes the todo "Book the bike service", and gives both the link to the entry. Its reply names the log and the todo with their IDs.

### 6. Corrections and deletes

- **Update.** A change sets only the fields that you give. The event of the change keeps the old value.
- **Delete.** A delete is soft: the row gets a `deleted_at` time and leaves every list. An agent can delete a record only after you said yes.
- **Delete forever.** The page `/admin/deleted` lists the deleted records. "Delete forever" removes the row, empties the old and the new value of each event of that record, and removes the file of a file source unless a different source uses it. Only this page can do it: no agent tool and no REST route can. A journal entry that a record names in `journal_ref` stays until that record is gone. The backups and the deploy dumps that were made before keep the record.
- A habit log has no field that you can change. Delete the log, then log the habit for the correct day.

**Use case.** You say "that was 3 stars, not 4". The agent calls `update` with the type `item`, the ID and `rating: 3`.

### 7. Event log

Each write appends one row to the `events` table in the same transaction: the action (such as `create`, `update`, `delete`, `purge` or `process`; rows from before #40 can have `verify`), the source, the old value and the new value. A write of a wiki page appends a row too: its value is the path and the hash of the page, and the text is in the git history of the wiki. A call that changes no value appends nothing. The log only grows, with one exception: a permanent delete empties the values of the events of its record.

**Use case.** A rating looks wrong. Read the `events` rows of that record to see who changed the rating, when, and what the value was before.

### 8. Capture: sources

A source is a text, a link or a file that LifeOS keeps exactly as it comes. The same content a second time gives the source that exists. A source has no field that you can change.

- **Text:** stored as it is written, 1 MB at most.
- **Link:** LifeOS stores the URL. It does not get the page.
- **File:** stored in the file store, with its name, its type and its SHA-256. The limit is `max_file_mb` (50 MB). LifeOS does not read the text of a PDF.
- No record comes from a source. The agent puts its ideas into the wiki and cites `(src:N)`.

**Use case.** You share an article link from the phone; a Shortcut posts it to `/api/ingest`. In a Claude Code session on the Pi you put a policy PDF in the inbox folder and say "ingest this"; the agent calls `capture` with the path.

### 9. Search

A search finds list items, journal entries, todos and sources by their words. A record matches when it has every word. Upper and lower case are the same, and a part of a word matches. The newest hit is first.

**Use case.** You ask "when did I last go to Lonavala?". The agent calls `search` with `lonavala`, reads the hits, and answers with the date and the ID of the record.

### 10. Read-only query

The agent can run one SQL statement on the data. The statement runs as the Postgres user `lifeos_query`, which can only read, for 5 seconds at most. The answer has 200 rows at most, and one row has 2 MB at most. A new table is closed to the query until its migration gives the right to read it.

**Use case.** You ask "on how many days did I go to the gym this month?". The agent calls `query` with a `SELECT count(*)` on `habit_logs`.

### 11. Secrets are refused

The service refuses a text that holds a card number, an Aadhaar number, a PAN, a private key or an access token. It stores nothing, and the error names the kind of secret without the secret itself. This applies to each text field: a title, a journal body, an attr, a captured text and a text file.

How the service finds each kind:

- **Card number:** the first digits of a card network, a length that this network uses, and a correct check digit. Groups such as `4111 1111 1111 1111` count. The IMEI of a phone is not a card number.
- **Aadhaar number:** the printed form `XXXX XXXX XXXX` with a correct check digit, in each text. 12 digits in one piece count only when a name of Aadhaar is near them (`Aadhaar`, `aadhar`, `UID`, `UIDAI`, `आधार`, within 40 characters). So the reference number of a UPI payment and a phone number such as `919876543210` are stored.
- **PAN:** the format `AAAPA9999A` in capital letters.
- **Private key and token:** a private key block, an access key of AWS, a token of GitHub or Slack, and an API key that starts with `sk-`.
- **Link:** a link of the form `https://name:password@host` is refused too.

A password in plain words and a bank account number have no pattern, so the service cannot find them. The service does not look inside a PDF or an image.

**Use case.** You paste a note that has your card number into the journal form. The page shows "body: must not hold a card number" and keeps your text, so you can remove the number and save.

### 12. Backup, restore test and notifications

- `deploy/backup.sh` dumps the database and copies the file store into two restic repositories: one on the Pi and one in Backblaze B2. A systemd timer runs it each night at 02:00.
- `deploy/restore-test.sh [local|b2]` loads the latest backup into a scratch database and compares the row count of each table with production.
- `deploy/notify.sh <topic> <message>` sends a message to the ntfy server on the Pi. The backup unit sends its result this way.

**Use case.** The backup fails in the night. Your phone gets a high-priority ntfy message with the last 15 log lines of the unit.

### 13. Service, deploy and public hostnames

- **Service.** `deploy/systemd/lifeos.service` starts `lifeos serve` at boot and starts it again after a failure. When it cannot start for about five minutes, ntfy gets a message. The unit gives the server only the two database passwords, the two Access values of the MCP server, the binary, the config and the folder of the file store and the wiki: not the other values of `deploy/.env`, not the home folder and not the Docker socket. The logs are in the journal: `journalctl -u lifeos`.
- **Deploy.** `deploy/deploy.sh` pulls the code, builds it, dumps the database to `/var/backups/lifeos/deploy/`, applies the migrations, starts the service again and runs the check below.
- **Public hostnames.** The Cloudflare Tunnel gives the admin pages one hostname, the REST API a second one and the MCP server a third one. Cloudflare Access guards each: a login with an email code for the admin pages, an Access service token for each device for the API, and an OAuth login for an MCP client. LifeOS has no login of its own, so Access is the only lock. `cloudflared` checks the token of Access on each request of the three hostnames, so a hostname stays closed when its Access application is removed.
- **MCP server.** The MCP server also checks the token of Access itself. It answers 403 to each request that has no valid token, also to a request from the Pi. The agent in the Claude app and Claude Code on the Pi use the same hostname.
- **Check.** `deploy/check-access.sh` fails when a hostname answers a request that has no login.
- **Release.** `lifeos version` prints the release of the binary, such as `v0.1.0`. The footer of each admin page shows it too, so a bug report can name the release. A build that comes after a release has a longer name, such as `v0.1.0-3-gabc1234`: three commits after `v0.1.0`.
- **Requests from another site.** The server refuses a request that changes data and that a browser sent from a different origin. A program, such as an iOS Shortcut, is not a browser and goes through.

**Use case.** You add a hostname to the tunnel and forget its Access application. `deploy/check-access.sh` prints `OPEN` for that hostname and exits with an error.

### 14. Wiki

The wiki holds your analysis and your insights: what the facts mean and how they connect. A fact is a record in the database, and a wiki page cites the row, as `(journal:57)` or `(src:12)`. A page is a Markdown file in the git repository of `wiki_dir`, and each write is one commit there.

- **Page types.** The folder gives the type: `people/`, `projects/`, `topics/` and `reviews/`. `index.md` lists each page with its summary; LifeOS writes it. `log.md` is the timeline of the work on the wiki.
- **One writer.** Only the MCP tools change a page. The server sets the dates, checks the text for secrets, and refuses a new citation of a row that is not there.
- **No lost change.** A change of a page needs the hash of the version that the agent read. When a different session changed the page, the write is refused, and the agent reads the page again and merges.
- **Lint.** `wiki_lint` reports a file that is not a valid page, a citation of a deleted record, a broken link, a page that is marked as contested, a page that no other page links to, a page with no change for 90 days and a page of more than 150 lines. It changes nothing.
- **Sources.** `mark_processed` records that the work on a source is complete.
- **History.** The git history of the wiki keeps each version of each page, also of a deleted page.
- **Copy on GitHub.** Each hour, `deploy/wiki-push.sh` copies the new commits to the private GitHub repository of the wiki. The server has no key and does not push. A clone of that repository on a different computer is for reading. A commit from there, or a change on github.com, stops the push, and ntfy gets a message. Then run `deploy/wiki-push-repair.sh`: it keeps your commit on a branch `from-github-<time>` on GitHub and puts the history of the Pi on `main`. Give the change to the agent after that, so that the server writes it.

**Use case.** You tell the agent "journal - met Asha about the tunnel". The agent stores your words with `journal`. The result names the next step for the agent: it finds `people/asha-rao.md` with `wiki_search`, and it adds one dated line with `wiki_append` that cites the entry.

## Interfaces

`lifeos serve` opens two listeners. Each is on `127.0.0.1` only.

| Listener | Default address | Serves |
| --- | --- | --- |
| `http_addr` | `127.0.0.1:8200` | Admin pages and REST API |
| `mcp_addr` | `127.0.0.1:8100` | MCP server, path `/mcp` |

LifeOS has no login of its own. The admin pages, the REST API and the MCP server are public only through the Cloudflare Tunnel, each behind Cloudflare Access (section 13). The MCP server answers only a request that has a token of Access, so a client uses the public hostname and not the listener.

### Admin pages

| Page | What you do there |
| --- | --- |
| `/admin` | The summary of today and yesterday: write a journal entry, add a todo, log a habit, mark a todo done, and see each write of the two days. |
| `/admin/items` | Add a list item, see the list, edit or delete an item. |
| `/admin/journal` | Write an entry, see the entries, edit or delete an entry. |
| `/admin/todos` | Add a todo, mark it done, edit or delete it. |
| `/admin/habits` | Log a habit for today, add, rename or retire a habit, delete a log. |
| `/admin/sources` | Capture a text or a link, see the sources, delete a source. |
| `/admin/deleted` | See the deleted records, and remove one for good. |

A record that the admin pages write is `manual`. The lists of todos, habit logs and items link each record that a journal entry reports to that entry.

The pages have a light and a dark theme. They follow the setting of your system. To set one, use **System**, **Light** or **Dark** at the bottom of a page; the browser keeps the choice, and the server stores nothing.

### REST API

The interactive documentation is at `/api/docs`. The OpenAPI spec is at `/api/openapi.json`. A record that the REST API writes is `manual`.

| Record | Routes |
| --- | --- |
| List items | `POST /api/items`, `GET /api/items?kind=&limit=`, `GET`, `PATCH`, `DELETE /api/items/{id}` |
| Journal | `POST /api/journal`, `GET`, `PATCH`, `DELETE /api/journal/{id}` |
| Todos | `POST /api/todos`, `GET /api/todos?status=&limit=`, `GET`, `PATCH`, `DELETE /api/todos/{id}` |
| Habits | `POST /api/habits/log`, `GET /api/habits/today`, `GET`, `DELETE /api/habits/logs/{id}` |
| Sources | `POST /api/ingest` (a text or a link), `POST /api/ingest/file?name=` (the file is the body), `GET`, `DELETE /api/sources/{id}` |
| Search | `GET /api/search?query=&types=&limit=` |

A list returns 50 rows by default; the maximum is 200.

```sh
curl -s -X POST http://127.0.0.1:8200/api/todos \
  -H 'Content-Type: application/json' \
  -d '{"title": "renew the car insurance", "due_on": "2026-10-20"}'
```

### MCP tools

To connect the Claude app, add a custom connector at <https://claude.ai/customize/connectors> with the URL `https://<LIFEOS_MCP_HOST>/mcp`, and log in with the email code of Cloudflare Access. Claude Code that has the same claude.ai login gets the connector with no second login.

A record that an MCP tool writes has the source `agent`.

| Tool | What it does |
| --- | --- |
| `add_item` | Adds a movie, book, place or game. |
| `journal` | Stores a journal entry, word for word. Its result names the next step: log the habits, close the todos and add the items that the entry reports, each with `journal_ref`, then the wiki. |
| `todo` | Adds one todo. |
| `log_habit` | Logs a habit for a day. A second call for the same day returns the stored log. |
| `capture` | Keeps a text, a link or a file from the inbox folder as a source. |
| `search` | Finds records by their words, the newest first. |
| `query` | Runs one read-only SQL statement. |
| `get` | Reads one record by type and ID. |
| `update` | Corrects one record. |
| `delete` | Deletes one record after you said yes (`confirmed: true`). |
| `mark_processed` | Records that the work on a source is complete. |
| `wiki_search` | Finds wiki pages by their words, or lists each page. |
| `wiki_read` | Reads one page, `index.md` or `log.md`, with its hash. |
| `wiki_write` | Creates a page, or replaces its text when the call has the hash of the stored page. |
| `wiki_append` | Adds text at the end of a page or of `log.md`. |
| `wiki_delete` | Deletes one page after you said yes (`confirmed: true`). |
| `wiki_lint` | Reports the problems of the wiki that code can find. |

The record types for `get`, `update` and `delete` are `item`, `journal`, `todo`, `habit_log` and `source`. `add_item`, `todo`, `log_habit` and `update` of a todo take an optional `journal_ref`: the ID of the journal entry that reports the record.

The rules for an agent are in the tool descriptions. The results of `journal` and `capture` have a field `next` that names the next step. The server also sends short instructions when a client connects; Claude Code reads them, the Claude app does not. `lifeos instructions` prints them: paste the text into the instructions of a Claude app project. Then a plain message about your life, with no keyword such as "journal" or "todo", reaches LifeOS.

## Run it

You need Go 1.26, Docker, and a `deploy/.env` file with `POSTGRES_PASSWORD`, `LIFEOS_QUERY_PASSWORD`, `NTFY_BASE_URL`, and `CF_ACCESS_TEAM` and `LIFEOS_MCP_AUD` for the token check of the MCP server. Docker Compose also needs `GRAFANA_ADMIN_PASSWORD`, `LIFEOS_DASH_HOST` and `LIFEOS_DASH_AUD` for Grafana, and `OTEL_BEARER_TOKEN` for the OTel collector ([docs/telemetry.md](docs/telemetry.md)). The file is not in git. The backup scripts also need `RESTIC_PASSWORD`, the B2 settings and `NTFY_TOKEN`. You also need the two folders of the file store, `files_dir` and `inbox_dir` in `config/lifeos.yaml`, and the wiki folder `wiki_dir`, which is in a clone of the wiki repository.

```sh
docker compose -f deploy/docker-compose.yml up -d   # Postgres, the Postgres server of the tests, ntfy and the monitoring services
git clone git@github.com:Gautam-J/lifeos-wiki.git /var/lib/lifeos/lifeos-wiki   # one time: the wiki that the server writes
make tools      # one time: install sqlc into ~/go/bin (linux/arm64 binary)
make query-user # one time: make the read-only Postgres user lifeos_query
make migrate    # generate, build, apply the migrations
make run        # run lifeos serve
```

Then open <http://127.0.0.1:8200/admin>.

On the Pi, the server runs as a systemd unit and `make run` is only for work on the code:

```sh
sudo systemctl link "$PWD/deploy/systemd/lifeos.service"   # one time
sudo systemctl enable --now lifeos.service                 # one time
deploy/deploy.sh                                           # after each change of the code
```

The push of the wiki to GitHub needs a mirror, one time, and its timer:

```sh
git init --bare /var/backups/lifeos/wiki.git
git -C /var/backups/lifeos/wiki.git remote add origin git@github.com:Gautam-J/lifeos-wiki.git
git -C /var/backups/lifeos/wiki.git fetch origin
sudo systemctl link "$PWD/deploy/systemd/lifeos-wiki-push.service" "$PWD/deploy/systemd/lifeos-wiki-push.timer"
sudo systemctl enable --now lifeos-wiki-push.timer
```

The unit file holds the path `/home/gautam/lifeos-core` and the user `gautam`.

| Command | Result |
| --- | --- |
| `make generate` | Runs sqlc and templ. Generated code is not in git. |
| `make build` | Builds `bin/lifeos`. |
| `make test` | Runs `deploy/test.sh`, `go vet` and all tests. Tests need the container `postgres-test`, which keeps its data in memory. |
| `make test PKG=./internal/api ARGS="-run TestName -v"` | Runs a part of the tests. |

Settings are in [config/lifeos.yaml](config/lifeos.yaml): the two listen addresses, the two database URLs, the file store and the wiki folder. The passwords come from `POSTGRES_PASSWORD` and `LIFEOS_QUERY_PASSWORD` in the environment, and the team name and the audience tag of Cloudflare Access from `CF_ACCESS_TEAM` and `LIFEOS_MCP_AUD`.

## Not built yet

- **The page of a link and the text of a file.** LifeOS stores a link as its URL and a file as its bytes. It does not get the page, and it does not read the text of a PDF. No page shows or downloads a stored file.
- **The inbox job** that reads each new source and proposes records from it.
- **Lists in the REST API.** No route lists journal entries or sources. The admin pages list them, and the agent uses `search` and `query`.
- **Money tables, the reconciliation with a bank statement and the one-time imports.** They are [issues](https://github.com/Gautam-J/lifeos-core/issues) 5, 6 and 7. `lifeos import-csv` and `lifeos job` return "not implemented".
- **A way to get a deleted record back.** The page `/admin/deleted` only removes.

## Repository layout

```
cmd/lifeos/          main package: serve, migrate, instructions, version
internal/service/    all logic: validation, provenance, events
internal/api/        REST handlers (chi + Huma)
internal/mcp/        MCP tools
internal/admin/      admin pages (templ + htmx)
internal/config/     loads config/lifeos.yaml
internal/version/    the release of the binary; the Makefile sets it
internal/testdb/     one migrated database for each test
internal/testaccess/ a stand-in for Cloudflare Access, for tests
queries/             SQL that sqlc compiles into internal/store/
migrations/          goose SQL migrations
deploy/              docker-compose.yml, backup scripts, query-user.sh, deploy.sh, check-access.sh, wiki-push.sh, wiki-push-repair.sh, pi-textfile.sh, systemd units
monitoring/          the config files of Prometheus, Grafana, Loki, Tempo and the OTel collector
dashboards/          the Grafana dashboards; Grafana loads them at its start
skills/              drafts of agent skills; no agent loads them
prompts/             the prompts of the brief jobs
docs/                design doc, implementation plans
.github/             the forms of a GitHub issue: bug, feature request
assets/              the pictures of this file: architecture.svg
CHANGELOG.md         the changes of each release; `cz bump` writes it
```
