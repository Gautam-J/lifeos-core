# lifeos-core

LifeOS is a self-hosted second brain. One Go binary, `lifeos`, runs on a Raspberry Pi 4 and stores personal records in Postgres. A person uses the admin pages. A program uses the REST API. An AI agent uses the MCP server (MCP is the Model Context Protocol, which gives an agent a set of tools).

This repo holds the service code only. It holds no personal data.

- **Design:** [docs/design.md](docs/design.md)
- **Rules for code changes:** [AGENTS.md](AGENTS.md)
- **Work that is not planned yet:** [docs/backlog.md](docs/backlog.md)

## Status

| Phase | State |
| --- | --- |
| 1 · Foundation | Done: Docker, Postgres, ntfy, nightly backup, restore test. |
| 2 · Core data | In work. Lists, journal, todos, habits, the review queue, capture, search, the read-only query and the three interfaces are built. Money tables and CSV importers are not built. |
| 3 to 5 | Not started. |

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
- "Done" for a day means a verified log.

**Use case.** You add the habits `gym` and `read` one time. Each evening you open `/admin/habits` and press the button for each habit of today. Or you say "I went to the gym"; the agent calls `log_habit`.

### 5. Review queue

Each record has a `source` (`manual`, `import`, `connector` or `agent`) and a `verified` flag. A record that you write or dictate is verified. A record that an agent infers is not verified, and it waits in the review queue until you approve it.

**Use case.** The agent reads your journal entry "finished Project Hail Mary" and proposes a book with `dictated` set to false. The book shows in `/admin/review`. You press **Approve**, or you correct the record first.

### 6. Corrections and deletes

- **Update.** A change sets only the fields that you give. An agent cannot apply an inferred change to a verified record: it asks you first, then sends the change as dictated.
- **Delete.** A delete is soft: the row gets a `deleted_at` time and leaves every list. An agent can delete a record only after you said yes.
- A habit log has no field that you can change. Delete the log, then log the habit for the correct day.

**Use case.** You say "that was 3 stars, not 4". The agent calls `update` with the type `item`, the ID and `rating: 3`.

### 7. Event log

Each write appends one row to the `events` table in the same transaction: the action (`create`, `update`, `delete` or `verify`), the source, the old value and the new value. A call that changes no value appends nothing.

**Use case.** A rating looks wrong. Read the `events` rows of that record to see who changed the rating, when, and what the value was before.

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

## Interfaces

`lifeos serve` opens two listeners. Each is on `127.0.0.1` only.

| Listener | Default address | Serves |
| --- | --- | --- |
| `http_addr` | `127.0.0.1:8200` | Admin pages and REST API |
| `mcp_addr` | `127.0.0.1:8100` | MCP server, path `/mcp` |

There is no login yet. Do not put these listeners on a public address.

### Admin pages

| Page | What you do there |
| --- | --- |
| `/admin/items` | Add a list item, see the list, edit or delete an item. |
| `/admin/journal` | Write an entry, see the entries, edit or delete an entry. |
| `/admin/todos` | Add a todo, mark it done, edit or delete it. |
| `/admin/habits` | Log a habit for today, add, rename or retire a habit, delete a log. |
| `/admin/sources` | Capture a text or a link, see the sources, delete a source. |
| `/admin/review` | See the records that wait, and approve them. |

A record that the admin pages write is `manual` and verified.

### REST API

The interactive documentation is at `/api/docs`. The OpenAPI spec is at `/api/openapi.json`. A record that the REST API writes is `manual` and verified.

| Record | Routes |
| --- | --- |
| List items | `POST /api/items`, `GET /api/items?kind=&limit=`, `GET`, `PATCH`, `DELETE /api/items/{id}` |
| Journal | `POST /api/journal`, `GET`, `PATCH`, `DELETE /api/journal/{id}` |
| Todos | `POST /api/todos`, `GET /api/todos?status=&limit=`, `GET`, `PATCH`, `DELETE /api/todos/{id}` |
| Habits | `POST /api/habits/log`, `GET /api/habits/today`, `GET`, `DELETE /api/habits/logs/{id}` |
| Sources | `POST /api/ingest` (a text or a link), `POST /api/ingest/file?name=` (the file is the body), `GET`, `DELETE /api/sources/{id}` |
| Search | `GET /api/search?query=&types=&limit=` |
| Review | `GET /api/review?limit=`, `POST /api/review/verify` |

A list returns 50 rows by default; the maximum is 200.

```sh
curl -s -X POST http://127.0.0.1:8200/api/todos \
  -H 'Content-Type: application/json' \
  -d '{"title": "renew the car insurance", "due_on": "2026-10-20"}'
```

### MCP tools

A record that an MCP tool writes has the source `agent`. It is verified only when the tool input says that you dictated it.

| Tool | What it does |
| --- | --- |
| `add_item` | Adds a movie, book, place or game. |
| `journal` | Stores a journal entry, word for word. The entry is always verified. |
| `todo` | Adds one todo. |
| `log_habit` | Logs a habit for a day. A second call with `dictated: true` approves a proposed log. |
| `capture` | Keeps a text, a link or a file from the inbox folder as a source. |
| `search` | Finds records by their words, the newest first. |
| `query` | Runs one read-only SQL statement. |
| `get` | Reads one record by type and ID. |
| `update` | Corrects one record. |
| `delete` | Deletes one record after you said yes (`confirmed: true`). |
| `review_queue` | Lists the records that wait for approval, oldest first. |
| `verify` | Approves records that you approved in the conversation. |

The record types for `get`, `update`, `delete` and `verify` are `item`, `journal`, `todo`, `habit_log` and `source`. `add_item`, `todo` and `log_habit` take an optional `source_ref`.

## Run it

You need Go 1.26, Docker, and a `deploy/.env` file with `POSTGRES_PASSWORD`, `LIFEOS_QUERY_PASSWORD` and `NTFY_BASE_URL`. The file is not in git. The backup scripts also need `RESTIC_PASSWORD`, the B2 settings and `NTFY_TOKEN`. You also need the two folders of the file store: `files_dir` and `inbox_dir` in `config/lifeos.yaml`.

```sh
docker compose -f deploy/docker-compose.yml up -d   # Postgres and ntfy
make tools      # one time: install sqlc into ~/go/bin (linux/arm64 binary)
make query-user # one time: make the read-only Postgres user lifeos_query
make migrate    # generate, build, apply the migrations
make run        # run lifeos serve
```

Then open <http://127.0.0.1:8200/admin/items>.

| Command | Result |
| --- | --- |
| `make generate` | Runs sqlc and templ. Generated code is not in git. |
| `make build` | Builds `bin/lifeos`. |
| `make test` | Runs `go vet` and all tests. Tests need the Postgres container. |
| `make test PKG=./internal/api ARGS="-run TestName -v"` | Runs a part of the tests. |

Settings are in [config/lifeos.yaml](config/lifeos.yaml): the two listen addresses, the two database URLs and the file store. The passwords come from `POSTGRES_PASSWORD` and `LIFEOS_QUERY_PASSWORD` in the environment.

## Not built yet

- **The page of a link and the text of a file.** LifeOS stores a link as its URL and a file as its bytes. It does not get the page, and it does not read the text of a PDF. No page shows or downloads a stored file.
- **The inbox job** that reads each new source and proposes records from it.
- **Lists in the REST API.** No route lists journal entries or sources. The admin pages list them, and the agent uses `search` and `query`.
- **Money tables and CSV importers.** `lifeos import-csv` and `lifeos job` return "not implemented".
- **Login, tokens and the systemd unit** for `lifeos serve`, and permanent delete in the admin pages.

## Repository layout

```
cmd/lifeos/          main package: serve, migrate
internal/service/    all logic: validation, provenance, events
internal/api/        REST handlers (chi + Huma)
internal/mcp/        MCP tools
internal/admin/      admin pages (templ + htmx)
internal/config/     loads config/lifeos.yaml
internal/testdb/     one migrated database for each test
queries/             SQL that sqlc compiles into internal/store/
migrations/          goose SQL migrations
deploy/              docker-compose.yml, backup scripts, query-user.sh, systemd units
docs/                design doc, backlog, implementation plans
```
