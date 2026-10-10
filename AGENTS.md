# AGENTS.md

How to work on the `lifeos-core` code. The full design is in [docs/design.md](docs/design.md); read the section you are changing before you change it.

This repo is the service code only. It holds no personal data. Using LifeOS (skills, wiki, prompts) lives in the separate `lifeos-wiki` repo.

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
internal/store/        sqlc-generated queries, from queries/*.sql
internal/api/          chi + Huma handlers
internal/mcp/          tools on the official MCP Go SDK
internal/admin/        templ + htmx admin pages
internal/connectors/   health, caldav, imap, owntracks, csv: one plugin each
internal/jobs/         briefs, inbox processing, lint
queries/               SQL that sqlc compiles
migrations/            goose SQL migrations
config/lifeos.yaml     models, schedules, connectors, wiki and prompt paths, notifications
dashboards/            Grafana definitions, versioned with the schema they query
deploy/                docker-compose.yml, systemd units and timers, deploy.sh
docs/design.md         the design doc, exported
```

Create a folder when its first file lands; do not add empty packages.

## Rules

1. **All logic lives in `internal/service`.** Validation, provenance and event logging happen there and nowhere else.
2. **Adapters are thin.** API handlers, MCP tools and CLI subcommands parse input, call one service function and format the result. No adapter calls another adapter. Handlers deal with HTTP; tools carry the model-facing descriptions.
3. **SQL only through sqlc.** Write queries in `queries/*.sql` and call the generated code in `internal/store`. No SQL strings in Go, and only `internal/service` calls `internal/store`.
4. **Every write records provenance.** Each record row carries `source` (`manual`, `import`, `connector` or `agent`), `source_ref`, `verified`, `created_at` and `updated_at`. Rows an agent infers start with `verified = false`; rows Gautam dictates directly are verified.
5. **Every write appends to `events`.** One row per write, with the old value on updates and deletes. `events` is append-only.
6. **Deletes are soft.** Set `deleted_at`; permanent removal happens only in the admin pages.
7. **Never store** passwords, card or bank account numbers, or government ID numbers. The service layer rejects them.
8. **Store times in UTC** (`timestamptz`); display in IST.

## Commands

```sh
go build -o bin/lifeos ./cmd/lifeos   # build the binary (bin/ is gitignored)
go vet ./...              # static checks
go test ./...             # tests
go run ./cmd/lifeos       # list subcommands
```

`lifeos serve` runs the API, admin pages and MCP server. `lifeos migrate`, `lifeos job <name>` and `lifeos import-csv <importer> <file>` are run by hand or by systemd timers.

## Migrations

- One goose SQL file per change, `migrations/NNNNN_name.sql`, with `-- +goose Up` and `-- +goose Down`.
- Every new record table gets the provenance columns, `deleted_at`, and the `set_updated_at()` trigger. Copy them from `catalog_items` in `00001_init.sql`.
- Never edit a migration that has been applied; add a new one.
- A migration that drops or renames a column runs only after a fresh `pg_dump`.

## Deploy

`deploy/deploy.sh` on the Pi: `git pull`, `go build`, `lifeos migrate`, restart the service. Every service listens on `127.0.0.1` only; the Cloudflare Tunnel is the single way in.
