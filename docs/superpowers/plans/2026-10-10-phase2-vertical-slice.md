# Phase 2 Vertical Slice (Catalog Items End to End) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One use case, "add a list item and read it back", works through every layer of Phase 2: config, migrations, service, sqlc store, REST API, MCP server and admin page, all from one `lifeos` binary.

**Architecture:** `internal/service` holds all logic and is the only write path. It validates input, records provenance and appends to `events` in one transaction. The REST API (chi + Huma), the MCP tools (official Go SDK) and the admin page (templ + htmx) are thin adapters: each reads its input, calls one service function and formats the result. `lifeos serve` runs the API and admin pages on one loopback listener and the MCP server on a second one.

**Tech Stack:** Go 1.26.0, chi v5.3.2, Huma v2.39.1, pgx v5.11.0, sqlc v1.31.1, goose v3.28.0, MCP Go SDK v1.8.0, templ v0.3.1070, htmx 2.0.11, go-yaml v1.19.2, Postgres 18 in Docker.

**Spec:** `docs/design.md` (sections "Data model", "Access layer", "Example workflows" S3 and S24, "Edge cases" E2, E3, E16) and `AGENTS.md`. Read both before you start a task.

**Scope:** This plan is the first of several for Phase 2. It stops at a confirmation gate (Task 7). The section "After the confirmation gate" lists the Phase 2 work that later plans cover. Do not start that work in this plan.

**Tested:** Every code block in this plan comes from a scratch copy of the repository. In that copy each test failed first, then passed after its code was added, in the task order below.

## Global Constraints

- **One logic layer.** All validation, provenance and event code is in `internal/service`. An adapter (`internal/api`, `internal/mcp`, `internal/admin`, `cmd/lifeos`) parses input, calls one service function and formats the result. No adapter calls a different adapter.
- **SQL only through sqlc.** Queries are in `queries/*.sql`. Only `internal/service` imports `internal/store`. No SQL strings in other non-test Go code.
- **Provenance on every write.** Each record row has `source` (`manual`, `import`, `connector` or `agent`), `source_ref`, `verified`, `created_at`, `updated_at`.
- **Verified rule.** A row that an agent inferred starts with `verified = false`. A row that Gautam dictated directly is verified.
- **One event for each write.** The service appends one `events` row in the same transaction as the write.
- **Soft delete.** A read leaves out rows where `deleted_at IS NOT NULL`.
- **Time.** Store `timestamptz` in UTC. Show times in IST. "Today" for a date-only field is the IST date.
- **Loopback only.** Every listener is on `127.0.0.1`. Do not add a Cloudflare Tunnel hostname in this plan.
- **Ports.** `http_addr` is `127.0.0.1:8200` and `mcp_addr` is `127.0.0.1:8100`. The design doc gives the API port 8000, but a different service on this Pi uses 8000.
- **Migrations.** `migrations/00001_init.sql` is applied on the Pi. Do not edit it. This plan adds no migration.
- **Versions.** Use the exact versions in "Tech Stack". The spec floor for the MCP Go SDK is v1.7. `go get` raises the `go` line in `go.mod` to `1.26.0` and downloads that toolchain; this is expected.
- **Generated code is not in git.** `internal/store/` (sqlc) and `*_templ.go` (templ) are in `.gitignore`. Never `git add` them. `make build` and `make test` generate them first.
- **Tests use real Postgres.** Run tests with `make test`, not `go test`. The Makefile reads `deploy/.env` and sets `LIFEOS_TEST_DATABASE_URL`. Each test creates its own database and drops it. The Postgres container from `deploy/docker-compose.yml` must be running.
- **Where to work.** Work in `/home/gautam/lifeos-core` on `main`. Do not make a branch or a worktree: `deploy/.env` is not in git and exists only in this checkout. Do not push.
- **Commits.** Conventional commit format, one concern for each commit, with the exact file lists in the steps.
- **Out of scope for this plan.** Update, delete, search, review queue, the other record tables, CSV importers, API tokens, the admin password, a systemd unit for `lifeos serve`, and the rejection of card numbers and secrets in free text (no field in this slice holds captured free text).

## Review Focus

These five conditions are implied by the spec, and no scenario in the spec tests them. Each has a test in the task that owns the code.

1. **A date between 00:00 and 05:30 IST.** The UTC date is one day behind then. An item dated "today" must be accepted, and "tomorrow" must be refused. Test: `TestAddItemUsesTheISTDateAsToday` (Task 2).
2. **Postgres is down during a request.** The caller must get a plain error with no internal text, and nothing must panic. Tests: `TestAddItemReportsADatabaseFailure` (Task 2), `TestDatabaseFailureGivesAPlain500` (Tasks 3 and 5), `TestDatabaseFailureGivesAPlainError` (Task 4).
3. **A misspelled or wrongly typed field.** A Shortcut typo or a model that invents an argument must get an error that names the field, and nothing must be stored. Tests: `TestAddItemRejectsBadRequests` (Task 3), `TestToolsReturnErrorsTheModelCanRead` (Task 4).
4. **HTML in a title.** The admin page must show it as text. Test: `TestItemsPageEscapesTheTitle` (Task 5).
5. **A listener that is open to the network, or a port that is in use.** The binary must refuse to start, with an error that names the config key. Tests: `TestLoadRejectsBadFiles` (Task 1), `TestListenNamesTheConfigKeyOfABusyPort` (Task 6).

One more condition has a test although it did not make the list of five: the item and its event must be stored together or not at all (`TestAddItemStoresNothingWhenTheEventFails`, Task 2).

## File Structure

| File | Task | Responsibility |
| --- | --- | --- |
| `Makefile` | 1, 2, 5, 6 | `tools`, `generate`, `build`, `test`, `migrate`, `run`; reads `deploy/.env` |
| `config/lifeos.yaml` | 1 | Listen addresses and the database URL |
| `internal/config/config.go` | 1 | Load and check `config/lifeos.yaml` |
| `migrations/migrations.go` | 1 | Embed the SQL files; `Up` applies them with goose |
| `internal/testdb/testdb.go` | 1 | Test helper: one migrated database for each test |
| `cmd/lifeos/db.go` | 1 | Pool settings from the config plus `POSTGRES_PASSWORD` |
| `cmd/lifeos/migrate.go` | 1 | `lifeos migrate` |
| `sqlc.yaml`, `queries/*.sql` | 2 | sqlc settings and the SQL that sqlc compiles |
| `internal/service/service.go` | 2 | `Service`, `New`, the IST zone |
| `internal/service/errors.go` | 2 | `ErrNotFound`, `ValidationError` |
| `internal/service/provenance.go` | 2 | `Source`, `Provenance` |
| `internal/service/events.go` | 2 | `recordEvent` |
| `internal/service/items.go` | 2 | `Item`, `AddItem` and its validation |
| `internal/service/items_read.go` | 2 | `GetItem`, `ListItems` |
| `internal/api/api.go`, `items.go` | 3 | REST routes for items; error mapping |
| `internal/mcp/server.go`, `items.go` | 4 | MCP server `lifeos`; tools `add_item` and `get` |
| `internal/admin/admin.go`, `items.templ` | 5 | Admin page for items; embedded htmx |
| `cmd/lifeos/serve.go` | 6 | `lifeos serve`: two listeners, clean shutdown |

---

### Task 1: Config, migration runner and `lifeos migrate`

**Files:**
- Create: `Makefile`, `config/lifeos.yaml`, `internal/config/config.go`, `migrations/migrations.go`, `internal/testdb/testdb.go`, `cmd/lifeos/db.go`, `cmd/lifeos/migrate.go`
- Modify: `cmd/lifeos/main.go` (delete one stub line), `go.mod`, `go.sum`
- Test: `internal/config/config_test.go`, `migrations/migrations_test.go`

**Interfaces:**
- Consumes: `migrations/00001_init.sql` (exists), `deploy/.env` with `POSTGRES_PASSWORD` (exists, not in git).
- Produces:
  - `config.Config{HTTPAddr, MCPAddr, DatabaseURL string}` and `config.Load(path string) (config.Config, error)`
  - `migrations.Up(ctx context.Context, db *sql.DB) ([]string, error)`: returns the file names it applied
  - `testdb.New(t *testing.T) *pgxpool.Pool`: a pool for a new, migrated database that is dropped when the test ends
  - `poolConfig(cfg config.Config) (*pgxpool.Config, error)` in package `main`
  - `make build`, `make test`, `make migrate`

- [ ] **Step 1: Check the prerequisites**

Run:
```bash
cd /home/gautam/lifeos-core
git status -sb
docker compose -f deploy/docker-compose.yml ps --format '{{.Name}}: {{.Status}}'
grep -c '^POSTGRES_PASSWORD=' deploy/.env
```
Expected: the branch is `main` with a clean tree, the line `lifeos-postgres-1: Up ... (healthy)` shows, and the last command prints `1`. If `docker` reports `permission denied`, put `sudo` before `docker`.

- [ ] **Step 2: Write the Makefile**

Create `Makefile`. The recipe lines start with a tab character.

```make
# Reads POSTGRES_PASSWORD from deploy/.env, which is not in git.
include deploy/.env
export POSTGRES_PASSWORD
# Tests create and drop their own databases on this server.
export LIFEOS_TEST_DATABASE_URL = postgres://lifeos:$(POSTGRES_PASSWORD)@127.0.0.1:5432/postgres?sslmode=disable

# Narrow a test run with: make test PKG=./internal/api ARGS="-run TestName -v"
PKG ?= ./...
ARGS ?=

.PHONY: build test migrate

build:
	go build -o bin/lifeos ./cmd/lifeos

test:
	go vet ./...
	go test $(PKG) $(ARGS)

migrate: build
	bin/lifeos migrate
```

- [ ] **Step 3: Add the YAML module and write the failing config test**

Run:
```bash
go get github.com/goccy/go-yaml@v1.19.2
```

Create `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lifeos.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const valid = `
http_addr: 127.0.0.1:8200
mcp_addr: 127.0.0.1:8100
database_url: postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable
`

func TestLoadReadsAllKeys(t *testing.T) {
	cfg, err := Load(writeConfig(t, valid))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		HTTPAddr:    "127.0.0.1:8200",
		MCPAddr:     "127.0.0.1:8100",
		DatabaseURL: "postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable",
	}
	if cfg != want {
		t.Errorf("Load = %+v, want %+v", cfg, want)
	}
}

func TestLoadRejectsBadFiles(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "address open to the network",
			content: strings.Replace(valid, "127.0.0.1:8200", "0.0.0.0:8200", 1),
			wantErr: "http_addr",
		},
		{
			name:    "address with no host",
			content: strings.Replace(valid, "127.0.0.1:8100", ":8100", 1),
			wantErr: "mcp_addr",
		},
		{
			name:    "missing address",
			content: strings.Replace(valid, "mcp_addr: 127.0.0.1:8100", "", 1),
			wantErr: "mcp_addr",
		},
		{
			name:    "missing database url",
			content: strings.Replace(valid, "database_url: postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable", "", 1),
			wantErr: "database_url",
		},
		{
			name:    "unknown key",
			content: valid + "htp_addr: 127.0.0.1:1\n",
			wantErr: "htp_addr",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, tt.content))
			if err == nil {
				t.Fatal("Load returned no error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not name %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadReportsAMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("Load returned no error for a missing file")
	}
}
```

- [ ] **Step 4: Run the test to verify that it fails**

Run: `make test PKG=./internal/config`
Expected: FAIL. The output contains `undefined: Load`.

- [ ] **Step 5: Write the config loader and the config file**

Create `internal/config/config.go`:

```go
// Package config loads config/lifeos.yaml, the one configuration file.
package config

import (
	"fmt"
	"net"
	"os"

	"github.com/goccy/go-yaml"
)

// Config is the content of config/lifeos.yaml.
type Config struct {
	// HTTPAddr is the listen address of the REST API and the admin pages.
	HTTPAddr string `yaml:"http_addr"`
	// MCPAddr is the listen address of the MCP server.
	MCPAddr string `yaml:"mcp_addr"`
	// DatabaseURL is the Postgres URL without a password. The password
	// comes from the POSTGRES_PASSWORD environment variable.
	DatabaseURL string `yaml:"database_url"`
}

// Load reads and checks the file at path. It rejects unknown keys, a missing
// value, and a listen address that is not on the loopback interface.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	var cfg Config
	if err := yaml.UnmarshalWithOptions(data, &cfg, yaml.Strict()); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("config: %s: database_url is empty", path)
	}
	for key, addr := range map[string]string{"http_addr": cfg.HTTPAddr, "mcp_addr": cfg.MCPAddr} {
		if err := checkLoopback(addr); err != nil {
			return Config{}, fmt.Errorf("config: %s: %s: %w", path, key, err)
		}
	}
	return cfg, nil
}

// checkLoopback accepts only host:port with a loopback IP address. Every
// LifeOS service listens on 127.0.0.1; the Cloudflare Tunnel is the way in.
func checkLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%q is not host:port", addr)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("%q is not a loopback address such as 127.0.0.1:8200", addr)
	}
	return nil
}
```

Create `config/lifeos.yaml`:

```yaml
# The one LifeOS configuration file.

# Listen addresses. Each must be on 127.0.0.1: the Cloudflare Tunnel is the
# only way in. The design doc gives the API port 8000, but another service on
# this Pi uses 8000.
http_addr: 127.0.0.1:8200
mcp_addr: 127.0.0.1:8100

# Postgres URL without a password. The password comes from POSTGRES_PASSWORD
# in the environment (deploy/.env).
database_url: postgres://lifeos@127.0.0.1:5432/lifeos?sslmode=disable
```

- [ ] **Step 6: Run the test to verify that it passes**

Run:
```bash
go mod tidy
make test PKG=./internal/config
```
Expected: `ok  	github.com/Gautam-J/lifeos-core/internal/config`

- [ ] **Step 7: Commit**

```bash
git add Makefile go.mod go.sum config/lifeos.yaml internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): load config/lifeos.yaml and add the Makefile"
```

- [ ] **Step 8: Add pgx and goose, then write the test database helper and the failing migration test**

Run:
```bash
go get github.com/jackc/pgx/v5@v5.11.0 github.com/pressly/goose/v3@v3.28.0
```
Expected: the output shows `go: downloading go1.26.0` the first time, and the `go` line in `go.mod` becomes `go 1.26.0`.

Create `internal/testdb/testdb.go`. Each test gets its own database, so tests in different packages can run at the same time.

```go
// Package testdb gives each test its own migrated Postgres database.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/Gautam-J/lifeos-core/migrations"
)

// New creates a database with a random name, applies every migration, and
// returns a pool for it. The database is dropped when the test ends.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("LIFEOS_TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("LIFEOS_TEST_DATABASE_URL is not set; run the tests with `make test`")
	}
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("testdb: connect: %v", err)
	}
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatalf("testdb: random name: %v", err)
	}
	name := "lifeos_test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("testdb: create database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("testdb: drop database: %v", err)
		}
		admin.Close(ctx)
	})

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("testdb: parse url: %v", err)
	}
	cfg.ConnConfig.Database = name

	db := stdlib.OpenDB(*cfg.ConnConfig)
	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("testdb: migrate: %v", err)
	}
	db.Close()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("testdb: open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
```

Create `migrations/migrations_test.go`:

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

	for _, table := range []string{"events", "catalog_items"} {
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

- [ ] **Step 9: Run the test to verify that it fails**

Run:
```bash
go mod tidy
make test PKG=./migrations
```
Expected: FAIL. The output contains `no non-test Go files` for the `migrations` package, because `migrations.Up` does not exist yet.

- [ ] **Step 10: Write the migration runner**

Create `migrations/migrations.go`:

```go
// Package migrations holds the goose SQL migrations and applies them.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var files embed.FS

// Up applies every migration that db does not have yet. It returns the file
// names of the migrations it applied, oldest first.
func Up(ctx context.Context, db *sql.DB) ([]string, error) {
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files)
	if err != nil {
		return nil, fmt.Errorf("migrations: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrations: %w", err)
	}
	applied := make([]string, 0, len(results))
	for _, r := range results {
		applied = append(applied, r.Source.Path)
	}
	return applied, nil
}
```

- [ ] **Step 11: Run the tests to verify that they pass**

Run:
```bash
go mod tidy
make test
```
Expected: `ok` for `internal/config` and for `migrations`. The first run compiles pgx and goose and takes about 90 seconds on the Pi.

- [ ] **Step 12: Write the `migrate` subcommand**

Create `cmd/lifeos/db.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/config"
)

// poolConfig turns database_url into pool settings and adds the password
// from the POSTGRES_PASSWORD environment variable.
func poolConfig(cfg config.Config) (*pgxpool.Config, error) {
	pc, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("database_url: %w", err)
	}
	if password := os.Getenv("POSTGRES_PASSWORD"); password != "" {
		pc.ConnConfig.Password = password
	}
	return pc, nil
}
```

Create `cmd/lifeos/migrate.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/Gautam-J/lifeos-core/internal/config"
	"github.com/Gautam-J/lifeos-core/migrations"
)

func migrate(args []string) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	configPath := flags.String("config", "config/lifeos.yaml", "path to the config file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	pc, err := poolConfig(cfg)
	if err != nil {
		return err
	}
	db := stdlib.OpenDB(*pc.ConnConfig)
	defer db.Close()

	applied, err := migrations.Up(context.Background(), db)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		fmt.Println("database is up to date")
		return nil
	}
	for _, name := range applied {
		fmt.Println("applied", name)
	}
	return nil
}
```

In `cmd/lifeos/main.go`, delete this one line (the new file replaces the stub):

```go
func migrate(args []string) error   { return errNotImplemented }
```

Then run `gofmt -w cmd/lifeos/main.go`.

- [ ] **Step 13: Run the real command**

Run:
```bash
make test
make migrate
```
Expected: the tests pass, and the last line is `database is up to date` (the Pi database is at version 1 already).

Then check the failure path:
```bash
POSTGRES_PASSWORD=wrong bin/lifeos migrate; echo "exit=$?"
```
Expected: one line that starts with `lifeos migrate: migrations:` and contains `password authentication failed`, then `exit=1`.

- [ ] **Step 14: Commit**

```bash
git add go.mod go.sum migrations/migrations.go migrations/migrations_test.go internal/testdb/testdb.go cmd/lifeos/db.go cmd/lifeos/migrate.go cmd/lifeos/main.go
git commit -m "feat(db): apply goose migrations with lifeos migrate"
```

---

### Task 2: Service for catalog items, with the sqlc store

**Files:**
- Create: `sqlc.yaml`, `queries/catalog_items.sql`, `queries/events.sql`, `internal/service/service.go`, `internal/service/errors.go`, `internal/service/provenance.go`, `internal/service/events.go`, `internal/service/items.go`, `internal/service/items_read.go`
- Modify: `Makefile`, `.gitignore`
- Test: `internal/service/items_test.go`, `internal/service/items_read_test.go`

**Interfaces:**
- Consumes: `testdb.New(t *testing.T) *pgxpool.Pool` (Task 1); tables `catalog_items` and `events` from `migrations/00001_init.sql`.
- Produces (package `service`):
  - `New(pool *pgxpool.Pool) *Service`
  - `var IST *time.Location`
  - `var ErrNotFound error`; `type ValidationError struct{ Field, Message string }` (use `errors.As` with `*ValidationError`)
  - `type Source string` with `SourceManual`, `SourceImport`, `SourceConnector`, `SourceAgent`
  - `type Provenance struct{ Source Source; SourceRef *int64; Verified bool }`
  - `var ItemKinds []string` (`movie`, `book`, `place`, `game`)
  - `type Item struct` with JSON tags `id`, `kind`, `title`, `rating`, `occurred_on`, `attrs`, `source`, `source_ref`, `verified`, `created_at`, `updated_at`
  - `type AddItemParams struct{ Kind, Title string; Rating *float64; OccurredOn string; Attrs map[string]any; Provenance }`
  - `(*Service).AddItem(ctx context.Context, p AddItemParams) (Item, error)`
  - `(*Service).GetItem(ctx context.Context, id int64) (Item, error)`
  - `type ListItemsParams struct{ Kind string; Limit int }`
  - `(*Service).ListItems(ctx context.Context, p ListItemsParams) ([]Item, error)`: newest first, never nil
  - `make tools`, `make generate`

- [ ] **Step 1: Add sqlc to the Makefile and install it**

Replace the content of `Makefile` with this version. It adds `SQLC_VERSION`, `SQLC`, the targets `tools` and `generate`, and makes `build` and `test` run `generate` first.

```make
# Reads POSTGRES_PASSWORD from deploy/.env, which is not in git.
include deploy/.env
export POSTGRES_PASSWORD
# Tests create and drop their own databases on this server.
export LIFEOS_TEST_DATABASE_URL = postgres://lifeos:$(POSTGRES_PASSWORD)@127.0.0.1:5432/postgres?sslmode=disable

# Narrow a test run with: make test PKG=./internal/api ARGS="-run TestName -v"
PKG ?= ./...
ARGS ?=

SQLC_VERSION = 1.31.1
SQLC = $(shell go env GOPATH)/bin/sqlc

.PHONY: tools generate build test migrate

# Installs the code generators that are not Go modules of this repo.
tools:
	curl -fsSL https://github.com/sqlc-dev/sqlc/releases/download/v$(SQLC_VERSION)/sqlc_$(SQLC_VERSION)_linux_arm64.tar.gz | tar -xz -C $(dir $(SQLC)) sqlc

# Generated code is not in git. Each target that compiles runs this first.
generate:
	$(SQLC) generate

build: generate
	go build -o bin/lifeos ./cmd/lifeos

test: generate
	go vet ./...
	go test $(PKG) $(ARGS)

migrate: build
	bin/lifeos migrate
```

Run:
```bash
make tools
~/go/bin/sqlc version
```
Expected: `v1.31.1`

- [ ] **Step 2: Write the sqlc settings and the queries**

Create `sqlc.yaml`. The overrides make sqlc use plain Go types: `*float64` for a nullable `numeric`, `time.Time` for `timestamptz`, and pointers for nullable columns.

```yaml
version: "2"
sql:
  - engine: postgresql
    schema: migrations
    queries: queries
    gen:
      go:
        package: store
        out: internal/store
        sql_package: pgx/v5
        emit_pointers_for_null_types: true
        overrides:
          - db_type: pg_catalog.numeric
            nullable: true
            go_type: { type: float64, pointer: true }
          - db_type: date
            nullable: true
            go_type: { import: time, type: Time, pointer: true }
          - db_type: timestamptz
            go_type: { import: time, type: Time }
          - db_type: timestamptz
            nullable: true
            go_type: { import: time, type: Time, pointer: true }
```

Create `queries/catalog_items.sql`:

```sql
-- name: InsertCatalogItem :one
INSERT INTO catalog_items (kind, title, rating, occurred_on, attrs, source, source_ref, verified)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetCatalogItem :one
SELECT * FROM catalog_items
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListCatalogItems :many
SELECT * FROM catalog_items
WHERE deleted_at IS NULL
  AND (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind'))
ORDER BY id DESC
LIMIT sqlc.arg('max_rows');
```

Create `queries/events.sql`:

```sql
-- name: InsertEvent :exec
INSERT INTO events (action, record_type, record_id, source, old_value, new_value)
VALUES ($1, $2, $3, $4, $5, $6);
```

Add this line to the end of `.gitignore`:

```
/internal/store/
```

- [ ] **Step 3: Generate the store package**

Run:
```bash
make generate
ls internal/store
git status --short
```
Expected: `ls` shows `catalog_items.sql.go  db.go  events.sql.go  models.go`. `git status` does not show `internal/store/`.

- [ ] **Step 4: Commit the sqlc setup**

```bash
git add Makefile sqlc.yaml queries/catalog_items.sql queries/events.sql .gitignore
git commit -m "build: generate the store package with sqlc"
```

- [ ] **Step 5: Write the failing test for `AddItem`**

Create `internal/service/items_test.go`. The test clock is 2026-10-10 20:00 UTC, which is 01:30 on 2026-10-11 in IST; one test uses that difference.

```go
package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

// testNow is 2026-10-10 20:00 UTC, which is 2026-10-11 01:30 in IST.
var testNow = time.Date(2026, 10, 10, 20, 0, 0, 0, time.UTC)

func newTestService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.New(t)
	s := New(pool)
	s.now = func() time.Time { return testNow }
	return s, pool
}

func ptr[T any](v T) *T { return &v }

// dune is a valid input. Tests copy it and change one field.
func dune() AddItemParams {
	return AddItemParams{
		Kind:       "movie",
		Title:      "Dune: Part Two",
		Rating:     ptr(4.5),
		OccurredOn: "2026-10-09",
		Attrs:      map[string]any{"director": "Denis Villeneuve", "year": 2024},
		Provenance: Provenance{Source: SourceAgent, Verified: true},
	}
}

func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestAddItemStoresTheItem(t *testing.T) {
	s, pool := newTestService(t)

	got, err := s.AddItem(context.Background(), dune())
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	if got.ID == 0 {
		t.Error("AddItem returned ID 0")
	}
	if got.Kind != "movie" || got.Title != "Dune: Part Two" {
		t.Errorf("kind, title = %q, %q", got.Kind, got.Title)
	}
	if got.Rating == nil || *got.Rating != 4.5 {
		t.Errorf("rating = %v, want 4.5", got.Rating)
	}
	if got.OccurredOn != "2026-10-09" {
		t.Errorf("occurred_on = %q, want 2026-10-09", got.OccurredOn)
	}
	if got.Attrs["director"] != "Denis Villeneuve" || got.Attrs["year"] != float64(2024) {
		t.Errorf("attrs = %v", got.Attrs)
	}
	if got.Source != SourceAgent || !got.Verified {
		t.Errorf("source, verified = %q, %v", got.Source, got.Verified)
	}
	if got.CreatedAt.IsZero() || !got.CreatedAt.Equal(got.UpdatedAt) {
		t.Errorf("created_at, updated_at = %v, %v", got.CreatedAt, got.UpdatedAt)
	}
	if got.CreatedAt.Location() != time.UTC {
		t.Errorf("created_at is in %v, want UTC", got.CreatedAt.Location())
	}
	if n := countRows(t, pool, "catalog_items"); n != 1 {
		t.Errorf("catalog_items has %d rows, want 1", n)
	}
}

func TestAddItemAcceptsTheMinimumInput(t *testing.T) {
	s, _ := newTestService(t)

	item, err := s.AddItem(context.Background(), AddItemParams{
		Kind:       "book",
		Title:      "  Dune  ",
		Provenance: Provenance{Source: SourceAgent},
	})
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if item.Title != "Dune" {
		t.Errorf("title = %q, want the spaces trimmed", item.Title)
	}
	if item.Rating != nil || item.OccurredOn != "" {
		t.Errorf("rating, occurred_on = %v, %q, want none", item.Rating, item.OccurredOn)
	}
	if item.Attrs == nil || len(item.Attrs) != 0 {
		t.Errorf("attrs = %v, want an empty object", item.Attrs)
	}
	if item.Verified {
		t.Error("verified = true, want false for a row the agent inferred")
	}
}

func TestAddItemAppendsACreateEvent(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	item, err := s.AddItem(ctx, dune())
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	var (
		action, recordType, source string
		recordID                   int64
		hasOld                     bool
		newTitle                   string
	)
	err = pool.QueryRow(ctx, `
		SELECT action, record_type, record_id, source, old_value IS NOT NULL, new_value->>'title'
		FROM events`).Scan(&action, &recordType, &recordID, &source, &hasOld, &newTitle)
	if err != nil {
		t.Fatalf("read the event (want exactly one row): %v", err)
	}
	if action != "create" || recordType != "catalog_items" || recordID != item.ID {
		t.Errorf("event = %s %s %d, want create catalog_items %d", action, recordType, recordID, item.ID)
	}
	if source != "agent" {
		t.Errorf("event source = %q, want agent", source)
	}
	if hasOld {
		t.Error("a create event has an old value")
	}
	if newTitle != "Dune: Part Two" {
		t.Errorf("event new_value title = %q", newTitle)
	}
	if n := countRows(t, pool, "events"); n != 1 {
		t.Errorf("events has %d rows, want 1", n)
	}
}

func TestAddItemRejectsBadInput(t *testing.T) {
	tests := []struct {
		name      string
		change    func(*AddItemParams)
		wantField string
	}{
		{"no kind", func(p *AddItemParams) { p.Kind = "" }, "kind"},
		{"unknown kind", func(p *AddItemParams) { p.Kind = "film" }, "kind"},
		{"empty title", func(p *AddItemParams) { p.Title = "" }, "title"},
		{"title of spaces", func(p *AddItemParams) { p.Title = " \t\n" }, "title"},
		{"title too long", func(p *AddItemParams) { p.Title = strings.Repeat("a", 301) }, "title"},
		{"rating above 5", func(p *AddItemParams) { p.Rating = ptr(7.0) }, "rating"},
		{"rating below 0", func(p *AddItemParams) { p.Rating = ptr(-0.5) }, "rating"},
		{"rating not a half step", func(p *AddItemParams) { p.Rating = ptr(4.25) }, "rating"},
		{"rating not a number", func(p *AddItemParams) { p.Rating = ptr(math.NaN()) }, "rating"},
		{"date in the wrong format", func(p *AddItemParams) { p.OccurredOn = "09-10-2026" }, "occurred_on"},
		{"date that does not exist", func(p *AddItemParams) { p.OccurredOn = "2026-02-30" }, "occurred_on"},
		{"date in the future", func(p *AddItemParams) { p.OccurredOn = "2099-01-01" }, "occurred_on"},
		{"unknown source", func(p *AddItemParams) { p.Source = "robot" }, "source"},
		{"no source", func(p *AddItemParams) { p.Source = "" }, "source"},
	}

	s, pool := newTestService(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := dune()
			tt.change(&p)

			_, err := s.AddItem(context.Background(), p)

			var invalid *ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("error = %v, want a *ValidationError", err)
			}
			if invalid.Field != tt.wantField {
				t.Errorf("field = %q, want %q (message: %s)", invalid.Field, tt.wantField, invalid.Message)
			}
		})
	}
	if n := countRows(t, pool, "catalog_items"); n != 0 {
		t.Errorf("catalog_items has %d rows after rejected input, want 0", n)
	}
	if n := countRows(t, pool, "events"); n != 0 {
		t.Errorf("events has %d rows after rejected input, want 0", n)
	}
}

func TestAddItemAcceptsATitleOfTheMaximumLength(t *testing.T) {
	s, _ := newTestService(t)
	p := dune()
	p.Title = strings.Repeat("é", 300) // 300 characters, 600 bytes

	if _, err := s.AddItem(context.Background(), p); err != nil {
		t.Errorf("AddItem with a 300-character title: %v", err)
	}
}

// Between 00:00 and 05:30 IST the UTC date is one day behind. "Today" for
// Gautam is the IST date, so the service must accept it.
func TestAddItemUsesTheISTDateAsToday(t *testing.T) {
	s, _ := newTestService(t) // the clock is 2026-10-10 20:00 UTC = 2026-10-11 01:30 IST
	ctx := context.Background()

	today := dune()
	today.OccurredOn = "2026-10-11"
	if _, err := s.AddItem(ctx, today); err != nil {
		t.Errorf("AddItem for today in IST: %v", err)
	}

	tomorrow := dune()
	tomorrow.OccurredOn = "2026-10-12"
	var invalid *ValidationError
	if _, err := s.AddItem(ctx, tomorrow); !errors.As(err, &invalid) || invalid.Field != "occurred_on" {
		t.Errorf("AddItem for tomorrow in IST: error = %v, want a ValidationError on occurred_on", err)
	}
}

func TestAddItemStoresNothingWhenTheEventFails(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "DROP TABLE events"); err != nil {
		t.Fatal(err)
	}

	if _, err := s.AddItem(ctx, dune()); err == nil {
		t.Fatal("AddItem returned no error although the event insert must fail")
	}
	if n := countRows(t, pool, "catalog_items"); n != 0 {
		t.Errorf("catalog_items has %d rows, want 0: the item must roll back with its event", n)
	}
}

func TestAddItemReportsADatabaseFailure(t *testing.T) {
	s, pool := newTestService(t)
	pool.Close()

	_, err := s.AddItem(context.Background(), dune())

	if err == nil {
		t.Fatal("AddItem returned no error with a closed pool")
	}
	var invalid *ValidationError
	if errors.As(err, &invalid) {
		t.Errorf("a database failure came back as a ValidationError: %v", err)
	}
}
```

- [ ] **Step 6: Run the test to verify that it fails**

Run: `make test PKG=./internal/service`
Expected: FAIL. The output contains `undefined: Service`.

- [ ] **Step 7: Write the service package**

Create `internal/service/service.go`:

```go
// Package service holds all LifeOS logic: validation, provenance and events.
// API handlers, MCP tools and CLI subcommands are thin adapters that call it.
package service

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// IST is the zone Gautam lives in. Times are stored in UTC; "today" for a
// date-only field is the IST date. India has no daylight saving time.
var IST = time.FixedZone("IST", 5*60*60+30*60)

// Service is the single write path to the database.
type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// New returns a Service that uses pool.
func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, now: time.Now}
}
```

Create `internal/service/errors.go`:

```go
package service

import "errors"

// ErrNotFound means that no live record has the requested ID.
var ErrNotFound = errors.New("not found")

// ValidationError means that the service refused the input. Field is the
// name of the input field, as the adapters spell it in JSON.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

func invalid(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}
```

Create `internal/service/provenance.go`:

```go
package service

// Source says how a record came into LifeOS.
type Source string

const (
	SourceManual    Source = "manual"    // Gautam, through a form, a Shortcut or a CLI command
	SourceImport    Source = "import"    // a CSV import
	SourceConnector Source = "connector" // a connector that pulls or receives data
	SourceAgent     Source = "agent"     // an LLM agent, through an MCP tool
)

// Provenance is recorded on every row that the service writes.
type Provenance struct {
	Source Source
	// SourceRef is the ID of the sources row the record came from, if any.
	SourceRef *int64
	// Verified is false for a row that an agent inferred. It stays false
	// until Gautam approves the row. A row that Gautam dictated is verified.
	Verified bool
}

func (p Provenance) validate() error {
	switch p.Source {
	case SourceManual, SourceImport, SourceConnector, SourceAgent:
		return nil
	}
	return invalid("source", "must be one of manual, import, connector, agent")
}
```

Create `internal/service/events.go`:

```go
package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// recordEvent appends one row to the events log. Call it in the same
// transaction as the write it describes. oldValue and newValue are encoded
// as JSON; pass nil when there is no value.
func recordEvent(ctx context.Context, q *store.Queries, action, recordType string, recordID int64, source Source, oldValue, newValue any) error {
	oldJSON, err := marshalOrNil(oldValue)
	if err != nil {
		return fmt.Errorf("event old value: %w", err)
	}
	newJSON, err := marshalOrNil(newValue)
	if err != nil {
		return fmt.Errorf("event new value: %w", err)
	}
	return q.InsertEvent(ctx, store.InsertEventParams{
		Action:     action,
		RecordType: recordType,
		RecordID:   &recordID,
		Source:     string(source),
		OldValue:   oldJSON,
		NewValue:   newJSON,
	})
}

func marshalOrNil(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
```

Create `internal/service/items.go`:

```go
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// ItemKinds are the lists that catalog_items holds.
var ItemKinds = []string{"movie", "book", "place", "game"}

const maxTitleLength = 300

// Item is one row of catalog_items.
type Item struct {
	ID    int64  `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	// Rating is 0 to 5 in steps of 0.5.
	Rating *float64 `json:"rating,omitempty"`
	// OccurredOn is the day watched, read, visited or played, as YYYY-MM-DD.
	OccurredOn string         `json:"occurred_on,omitempty"`
	Attrs      map[string]any `json:"attrs"`
	Source     Source         `json:"source"`
	SourceRef  *int64         `json:"source_ref,omitempty"`
	Verified   bool           `json:"verified"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// AddItemParams is the input of AddItem.
type AddItemParams struct {
	Kind  string
	Title string
	// Rating is optional: 0 to 5 in steps of 0.5.
	Rating *float64
	// OccurredOn is optional: YYYY-MM-DD, not later than today in IST.
	OccurredOn string
	// Attrs is optional: more fields for this kind, such as author or year.
	Attrs map[string]any
	Provenance
}

// AddItem validates p, stores one catalog item and appends a create event,
// in one transaction.
func (s *Service) AddItem(ctx context.Context, p AddItemParams) (Item, error) {
	row, err := p.toRow(s.now())
	if err != nil {
		return Item{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("add item: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	stored, err := q.InsertCatalogItem(ctx, row)
	if err != nil {
		return Item{}, fmt.Errorf("add item: %w", err)
	}
	item, err := itemFromRow(stored)
	if err != nil {
		return Item{}, fmt.Errorf("add item: %w", err)
	}
	if err := recordEvent(ctx, q, "create", "catalog_items", item.ID, p.Source, nil, item); err != nil {
		return Item{}, fmt.Errorf("add item: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Item{}, fmt.Errorf("add item: %w", err)
	}
	return item, nil
}

// toRow checks every field and returns the row to insert.
func (p AddItemParams) toRow(now time.Time) (store.InsertCatalogItemParams, error) {
	var row store.InsertCatalogItemParams

	if err := checkKind(p.Kind); err != nil {
		return row, err
	}
	title := strings.TrimSpace(p.Title)
	if title == "" {
		return row, invalid("title", "must not be empty")
	}
	if utf8.RuneCountInString(title) > maxTitleLength {
		return row, invalid("title", fmt.Sprintf("must be at most %d characters", maxTitleLength))
	}
	if p.Rating != nil {
		r := *p.Rating
		if math.IsNaN(r) || r < 0 || r > 5 || math.Mod(r*2, 1) != 0 {
			return row, invalid("rating", "must be from 0 to 5 in steps of 0.5")
		}
	}
	var occurredOn *time.Time
	if p.OccurredOn != "" {
		day, err := time.Parse(time.DateOnly, p.OccurredOn)
		if err != nil {
			return row, invalid("occurred_on", "must be a date as YYYY-MM-DD")
		}
		if day.Format(time.DateOnly) > now.In(IST).Format(time.DateOnly) {
			return row, invalid("occurred_on", "must not be in the future")
		}
		occurredOn = &day
	}
	if err := p.Provenance.validate(); err != nil {
		return row, err
	}
	attrs := p.Attrs
	if attrs == nil {
		attrs = map[string]any{}
	}
	attrsJSON, err := json.Marshal(attrs)
	if err != nil {
		return row, invalid("attrs", "must be a JSON object")
	}

	return store.InsertCatalogItemParams{
		Kind:       p.Kind,
		Title:      title,
		Rating:     p.Rating,
		OccurredOn: occurredOn,
		Attrs:      attrsJSON,
		Source:     string(p.Source),
		SourceRef:  p.SourceRef,
		Verified:   p.Verified,
	}, nil
}

func checkKind(kind string) error {
	for _, k := range ItemKinds {
		if kind == k {
			return nil
		}
	}
	return invalid("kind", "must be one of "+strings.Join(ItemKinds, ", "))
}

func itemFromRow(row store.CatalogItem) (Item, error) {
	item := Item{
		ID:        row.ID,
		Kind:      row.Kind,
		Title:     row.Title,
		Rating:    row.Rating,
		Source:    Source(row.Source),
		SourceRef: row.SourceRef,
		Verified:  row.Verified,
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
	if row.OccurredOn != nil {
		item.OccurredOn = row.OccurredOn.Format(time.DateOnly)
	}
	if err := json.Unmarshal(row.Attrs, &item.Attrs); err != nil {
		return Item{}, fmt.Errorf("attrs of catalog item %d: %w", row.ID, err)
	}
	return item, nil
}
```

- [ ] **Step 8: Run the test to verify that it passes**

Run:
```bash
go mod tidy
make test PKG=./internal/service
```
Expected: `ok  	github.com/Gautam-J/lifeos-core/internal/service`. The run takes about 10 seconds, because each test creates a database.

- [ ] **Step 9: Commit**

```bash
git add go.mod go.sum internal/service/service.go internal/service/errors.go internal/service/provenance.go internal/service/events.go internal/service/items.go internal/service/items_test.go
git commit -m "feat(service): add catalog items with provenance and events"
```

- [ ] **Step 10: Write the failing test for `GetItem` and `ListItems`**

Create `internal/service/items_read_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"
)

func addItem(t *testing.T, s *Service, kind, title string) Item {
	t.Helper()
	item, err := s.AddItem(context.Background(), AddItemParams{
		Kind:       kind,
		Title:      title,
		Provenance: Provenance{Source: SourceManual, Verified: true},
	})
	if err != nil {
		t.Fatalf("AddItem %q: %v", title, err)
	}
	return item
}

func titles(items []Item) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Title
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGetItemReturnsTheStoredItem(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	added, err := s.AddItem(ctx, dune())
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	got, err := s.GetItem(ctx, added.ID)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if got.ID != added.ID || got.Title != "Dune: Part Two" || got.OccurredOn != "2026-10-09" {
		t.Errorf("GetItem = %+v", got)
	}
	if got.Rating == nil || *got.Rating != 4.5 || got.Attrs["year"] != float64(2024) {
		t.Errorf("rating, attrs = %v, %v", got.Rating, got.Attrs)
	}
}

func TestGetItemReturnsNotFound(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	deleted := addItem(t, s, "book", "Deleted book")
	if _, err := pool.Exec(ctx, "UPDATE catalog_items SET deleted_at = now() WHERE id = $1", deleted.ID); err != nil {
		t.Fatal(err)
	}

	for name, id := range map[string]int64{"unknown ID": 9999, "zero": 0, "negative": -1, "deleted item": deleted.ID} {
		t.Run(name, func(t *testing.T) {
			if _, err := s.GetItem(ctx, id); !errors.Is(err, ErrNotFound) {
				t.Errorf("GetItem(%d) error = %v, want ErrNotFound", id, err)
			}
		})
	}
}

func TestListItemsReturnsTheNewestFirst(t *testing.T) {
	s, _ := newTestService(t)
	addItem(t, s, "book", "First")
	addItem(t, s, "movie", "Second")
	addItem(t, s, "book", "Third")

	items, err := s.ListItems(context.Background(), ListItemsParams{})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if want := []string{"Third", "Second", "First"}; !equal(titles(items), want) {
		t.Errorf("titles = %v, want %v", titles(items), want)
	}
}

func TestListItemsFiltersByKind(t *testing.T) {
	s, _ := newTestService(t)
	addItem(t, s, "book", "First")
	addItem(t, s, "movie", "Second")
	addItem(t, s, "book", "Third")

	items, err := s.ListItems(context.Background(), ListItemsParams{Kind: "book"})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if want := []string{"Third", "First"}; !equal(titles(items), want) {
		t.Errorf("titles = %v, want %v", titles(items), want)
	}
}

func TestListItemsLeavesOutDeletedItems(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	addItem(t, s, "book", "Kept")
	deleted := addItem(t, s, "book", "Deleted")
	if _, err := pool.Exec(ctx, "UPDATE catalog_items SET deleted_at = now() WHERE id = $1", deleted.ID); err != nil {
		t.Fatal(err)
	}

	items, err := s.ListItems(ctx, ListItemsParams{})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if want := []string{"Kept"}; !equal(titles(items), want) {
		t.Errorf("titles = %v, want %v", titles(items), want)
	}
}

func TestListItemsAppliesTheLimit(t *testing.T) {
	s, _ := newTestService(t)
	addItem(t, s, "book", "First")
	addItem(t, s, "book", "Second")
	addItem(t, s, "book", "Third")

	items, err := s.ListItems(context.Background(), ListItemsParams{Limit: 2})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if want := []string{"Third", "Second"}; !equal(titles(items), want) {
		t.Errorf("titles = %v, want %v", titles(items), want)
	}
}

func TestListItemsReturnsAnEmptyListNotNil(t *testing.T) {
	s, _ := newTestService(t)

	items, err := s.ListItems(context.Background(), ListItemsParams{})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if items == nil || len(items) != 0 {
		t.Errorf("items = %#v, want an empty non-nil slice so JSON shows []", items)
	}
}

func TestListItemsRejectsBadInput(t *testing.T) {
	s, _ := newTestService(t)
	tests := []struct {
		name      string
		params    ListItemsParams
		wantField string
	}{
		{"unknown kind", ListItemsParams{Kind: "film"}, "kind"},
		{"limit above the maximum", ListItemsParams{Limit: 201}, "limit"},
		{"negative limit", ListItemsParams{Limit: -1}, "limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.ListItems(context.Background(), tt.params)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Field != tt.wantField {
				t.Errorf("error = %v, want a ValidationError on %s", err, tt.wantField)
			}
		})
	}
}
```

- [ ] **Step 11: Run the test to verify that it fails**

Run: `make test PKG=./internal/service`
Expected: FAIL. The output contains `s.GetItem undefined`.

- [ ] **Step 12: Write the read functions**

Create `internal/service/items_read.go`:

```go
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// GetItem returns the catalog item with this ID. It returns ErrNotFound when
// there is no such item or the item is deleted.
func (s *Service) GetItem(ctx context.Context, id int64) (Item, error) {
	row, err := store.New(s.pool).GetCatalogItem(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("get item: %w", err)
	}
	return itemFromRow(row)
}

// ListItemsParams is the input of ListItems.
type ListItemsParams struct {
	// Kind is optional: an empty string lists every kind.
	Kind string
	// Limit is optional: 0 means 50. The maximum is 200.
	Limit int
}

// ListItems returns the catalog items that are not deleted, newest first.
func (s *Service) ListItems(ctx context.Context, p ListItemsParams) ([]Item, error) {
	if p.Limit == 0 {
		p.Limit = defaultListLimit
	}
	if p.Limit < 1 || p.Limit > maxListLimit {
		return nil, invalid("limit", fmt.Sprintf("must be from 1 to %d", maxListLimit))
	}
	arg := store.ListCatalogItemsParams{MaxRows: int32(p.Limit)}
	if p.Kind != "" {
		if err := checkKind(p.Kind); err != nil {
			return nil, err
		}
		arg.Kind = &p.Kind
	}

	rows, err := store.New(s.pool).ListCatalogItems(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		item, err := itemFromRow(row)
		if err != nil {
			return nil, fmt.Errorf("list items: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}
```

- [ ] **Step 13: Run the tests to verify that they pass**

Run: `make test`
Expected: `ok` for `internal/config`, `internal/service` and `migrations`.

- [ ] **Step 14: Commit**

```bash
git add internal/service/items_read.go internal/service/items_read_test.go
git commit -m "feat(service): read and list catalog items"
```

---

### Task 3: REST API for items

**Files:**
- Create: `internal/api/api.go`, `internal/api/items.go`
- Modify: `go.mod`, `go.sum`
- Test: `internal/api/items_test.go`

**Interfaces:**
- Consumes (Task 2): `service.New`, `(*Service).AddItem`, `GetItem`, `ListItems`, `service.AddItemParams`, `service.ListItemsParams`, `service.Item`, `service.Provenance`, `service.SourceManual`, `*service.ValidationError`, `service.ErrNotFound`; `testdb.New` (Task 1).
- Produces: `api.Register(router chi.Router, svc *service.Service)`. It adds:
  - `POST /api/items`: 201 with the item; the row is `manual` and verified
  - `GET /api/items/{id}`: 200 with the item, or 404
  - `GET /api/items?kind=&limit=`: 200 with `{"items": [...]}`
  - `GET /api/openapi.json` and `GET /api/docs` (from Huma)
  - 422 with `errors[].location` = `body.<field>` or `query.<field>` when the service refuses the input

- [ ] **Step 1: Add chi and Huma, then write the failing test**

Run:
```bash
go get github.com/go-chi/chi/v5@v5.3.2 github.com/danielgtaylor/huma/v2@v2.39.1
```

Create `internal/api/items_test.go`:

```go
package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/api"
	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

func newTestAPI(t *testing.T) (http.Handler, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.New(t)
	router := chi.NewRouter()
	api.Register(router, service.New(pool))
	return router, pool
}

// do sends one request and returns the status and the decoded JSON body.
func do(t *testing.T, h http.Handler, method, path, body string) (int, map[string]any, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	raw := rec.Body.String()
	var decoded map[string]any
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			t.Fatalf("%s %s: body is not a JSON object: %v\n%s", method, path, err, raw)
		}
	}
	return rec.Code, decoded, raw
}

const duneJSON = `{"kind":"movie","title":"Dune: Part Two","rating":4.5,"occurred_on":"2024-03-01","attrs":{"year":2024}}`

func TestAddItemThenReadIt(t *testing.T) {
	h, _ := newTestAPI(t)

	status, added, raw := do(t, h, "POST", "/api/items", duneJSON)
	if status != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201\n%s", status, raw)
	}
	if added["source"] != "manual" || added["verified"] != true {
		t.Errorf("source, verified = %v, %v, want manual, true", added["source"], added["verified"])
	}
	id, ok := added["id"].(float64)
	if !ok || id < 1 {
		t.Fatalf("id = %v, want a positive number", added["id"])
	}

	status, got, raw := do(t, h, "GET", fmt.Sprintf("/api/items/%d", int64(id)), "")
	if status != http.StatusOK {
		t.Fatalf("GET status = %d, want 200\n%s", status, raw)
	}
	if got["title"] != "Dune: Part Two" || got["rating"] != 4.5 || got["occurred_on"] != "2024-03-01" {
		t.Errorf("item = %s", raw)
	}
}

func TestAddItemRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantInBody string
	}{
		{"rating the service refuses", `{"kind":"movie","title":"Dune","rating":7}`, 422, "body.rating"},
		{"kind the service refuses", `{"kind":"film","title":"Dune"}`, 422, "body.kind"},
		{"missing title", `{"kind":"movie"}`, 422, "title"},
		{"misspelled field", `{"kind":"movie","title":"Dune","ratting":4}`, 422, "ratting"},
		{"rating of the wrong type", `{"kind":"movie","title":"Dune","rating":"four"}`, 422, "rating"},
		{"body that is not JSON", `kind=movie`, 400, ""},
		{"empty body", ``, 400, ""},
	}
	h, pool := newTestAPI(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/items", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d\n%s", rec.Code, tt.wantStatus, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), tt.wantInBody) {
				t.Errorf("body does not name %q:\n%s", tt.wantInBody, rec.Body)
			}
		})
	}
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM catalog_items").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("catalog_items has %d rows after rejected requests, want 0", n)
	}
}

func TestGetItemErrors(t *testing.T) {
	h, _ := newTestAPI(t)

	if status, _, raw := do(t, h, "GET", "/api/items/9999", ""); status != http.StatusNotFound {
		t.Errorf("unknown ID: status = %d, want 404\n%s", status, raw)
	}
	if status, _, raw := do(t, h, "GET", "/api/items/abc", ""); status != http.StatusUnprocessableEntity {
		t.Errorf("ID that is not a number: status = %d, want 422\n%s", status, raw)
	}
}

func TestListItems(t *testing.T) {
	h, _ := newTestAPI(t)

	status, empty, raw := do(t, h, "GET", "/api/items", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", status, raw)
	}
	if items, ok := empty["items"].([]any); !ok || len(items) != 0 {
		t.Errorf("empty list = %s, want {\"items\":[]}", raw)
	}

	do(t, h, "POST", "/api/items", `{"kind":"book","title":"First"}`)
	do(t, h, "POST", "/api/items", `{"kind":"movie","title":"Second"}`)
	do(t, h, "POST", "/api/items", `{"kind":"book","title":"Third"}`)

	_, books, raw := do(t, h, "GET", "/api/items?kind=book&limit=1", "")
	items, _ := books["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["title"] != "Third" {
		t.Errorf("kind=book&limit=1 = %s, want only Third", raw)
	}

	status, _, raw = do(t, h, "GET", "/api/items?kind=film", "")
	if status != http.StatusUnprocessableEntity || !strings.Contains(raw, "query.kind") {
		t.Errorf("kind=film: status = %d, want 422 that names query.kind\n%s", status, raw)
	}
}

// A database failure must give a plain 500. The response must not show
// internal error text.
func TestDatabaseFailureGivesAPlain500(t *testing.T) {
	h, pool := newTestAPI(t)
	pool.Close()

	status, _, raw := do(t, h, "POST", "/api/items", duneJSON)

	if status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", status)
	}
	if strings.Contains(raw, "pool") || strings.Contains(raw, "add item") {
		t.Errorf("the response shows internal error text:\n%s", raw)
	}
}
```

- [ ] **Step 2: Run the test to verify that it fails**

Run:
```bash
go mod tidy
make test PKG=./internal/api
```
Expected: FAIL. The output contains `no non-test Go files` for `internal/api`, because `api.Register` does not exist yet.

- [ ] **Step 3: Write the adapter**

Create `internal/api/api.go`. Huma checks the shape and the types of a request. The service checks the values. Do not copy the value rules into Huma tags: the service is the one source of the rules.

```go
// Package api is the REST adapter. Handlers read the request, call one
// service function and write the result. They hold no logic.
package api

import (
	"errors"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// Register adds every /api route to router.
func Register(router chi.Router, svc *service.Service) {
	config := huma.DefaultConfig("LifeOS API", "0.1.0")
	config.DocsPath = "/api/docs"
	config.OpenAPIPath = "/api/openapi"
	config.SchemasPath = "/api/schemas"
	api := humachi.New(router, config)

	registerItems(api, svc)
}

// toHTTPError turns a service error into a Huma error. where is the part of
// the request that held the input: "body" or "query". It logs an unknown
// error and hides its text, because the text can name internals.
func toHTTPError(where string, err error) error {
	var invalid *service.ValidationError
	if errors.As(err, &invalid) {
		return huma.Error422UnprocessableEntity("validation failed", &huma.ErrorDetail{
			Location: where + "." + invalid.Field,
			Message:  invalid.Message,
		})
	}
	if errors.Is(err, service.ErrNotFound) {
		return huma.Error404NotFound("not found")
	}
	slog.Error("api: request failed", "error", err)
	return huma.Error500InternalServerError("internal error")
}
```

Create `internal/api/items.go`:

```go
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type addItemInput struct {
	Body struct {
		Kind       string         `json:"kind" doc:"One of movie, book, place, game"`
		Title      string         `json:"title"`
		Rating     *float64       `json:"rating,omitempty" doc:"0 to 5 in steps of 0.5"`
		OccurredOn string         `json:"occurred_on,omitempty" doc:"Day watched, read, visited or played, as YYYY-MM-DD"`
		Attrs      map[string]any `json:"attrs,omitempty" doc:"More fields for this kind, such as author or year"`
	}
}

type itemOutput struct {
	Body service.Item
}

type getItemInput struct {
	ID int64 `path:"id"`
}

type listItemsInput struct {
	Kind  string `query:"kind" doc:"One of movie, book, place, game. Empty lists every kind."`
	Limit int    `query:"limit" doc:"1 to 200. The default is 50."`
}

type listItemsOutput struct {
	Body struct {
		Items []service.Item `json:"items"`
	}
}

func registerItems(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{
		OperationID:   "add-item",
		Method:        http.MethodPost,
		Path:          "/api/items",
		Summary:       "Add a list item",
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *addItemInput) (*itemOutput, error) {
		// A request to the REST API comes from Gautam's own devices, so
		// the row is manual and verified.
		item, err := svc.AddItem(ctx, service.AddItemParams{
			Kind:       in.Body.Kind,
			Title:      in.Body.Title,
			Rating:     in.Body.Rating,
			OccurredOn: in.Body.OccurredOn,
			Attrs:      in.Body.Attrs,
			Provenance: service.Provenance{Source: service.SourceManual, Verified: true},
		})
		if err != nil {
			return nil, toHTTPError("body", err)
		}
		return &itemOutput{Body: item}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-item",
		Method:      http.MethodGet,
		Path:        "/api/items/{id}",
		Summary:     "Read one list item",
	}, func(ctx context.Context, in *getItemInput) (*itemOutput, error) {
		item, err := svc.GetItem(ctx, in.ID)
		if err != nil {
			return nil, toHTTPError("path", err)
		}
		return &itemOutput{Body: item}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-items",
		Method:      http.MethodGet,
		Path:        "/api/items",
		Summary:     "List items, newest first",
	}, func(ctx context.Context, in *listItemsInput) (*listItemsOutput, error) {
		items, err := svc.ListItems(ctx, service.ListItemsParams{Kind: in.Kind, Limit: in.Limit})
		if err != nil {
			return nil, toHTTPError("query", err)
		}
		out := &listItemsOutput{}
		out.Body.Items = items
		return out, nil
	})
}
```

- [ ] **Step 4: Run the tests to verify that they pass**

Run:
```bash
go mod tidy
make test
```
Expected: `ok` for `internal/api`, `internal/config`, `internal/service` and `migrations`.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/api/api.go internal/api/items.go internal/api/items_test.go
git commit -m "feat(api): add REST endpoints for catalog items"
```

---

### Task 4: MCP tools `add_item` and `get`

**Files:**
- Create: `internal/mcp/server.go`, `internal/mcp/items.go`
- Modify: `go.mod`, `go.sum`
- Test: `internal/mcp/items_test.go`

**Interfaces:**
- Consumes (Task 2): `service.New`, `(*Service).AddItem`, `GetItem`, `service.AddItemParams`, `service.Item`, `service.Provenance`, `service.SourceAgent`, `*service.ValidationError`, `service.ErrNotFound`; `testdb.New` (Task 1).
- Produces (package `mcp`, import it as `lifeosmcp` and import the SDK as `sdk`):
  - `NewServer(svc *service.Service) *sdk.Server`: the server named `lifeos`
  - `Handler(svc *service.Service) http.Handler`: the server over streamable HTTP
  - Tool `add_item` with arguments `kind`, `title`, `rating`, `occurred_on`, `attrs`, `dictated`. The row is `agent`, and verified only when `dictated` is true.
  - Tool `get` with arguments `type` (only `item`) and `id`
  - A failed call returns a result with `IsError` true and a text that the model can read

- [ ] **Step 1: Add the MCP Go SDK, then write the failing test**

Run:
```bash
go get github.com/modelcontextprotocol/go-sdk@v1.8.0
```

Create `internal/mcp/items_test.go`. The test connects a client to the server in the same process, with no network.

```go
package mcp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	lifeosmcp "github.com/Gautam-J/lifeos-core/internal/mcp"
	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

// connect starts the lifeos MCP server in this process and returns a client
// session that talks to it.
func connect(t *testing.T) (*sdk.ClientSession, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool := testdb.New(t)
	server := lifeosmcp.NewServer(service.New(pool))

	clientSide, serverSide := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverSide, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, clientSide, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session, pool
}

func call(t *testing.T, session *sdk.ClientSession, tool string, args map[string]any) *sdk.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", tool, err)
	}
	return result
}

// text joins the text content of a tool result.
func text(result *sdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func structured(t *testing.T, result *sdk.CallToolResult) map[string]any {
	t.Helper()
	if result.IsError {
		t.Fatalf("tool returned an error: %s", text(result))
	}
	m, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content is %T, want a JSON object", result.StructuredContent)
	}
	return m
}

func TestServerListsTheTools(t *testing.T) {
	session, _ := connect(t)

	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	found := map[string]bool{}
	for _, tool := range list.Tools {
		found[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %s has no description", tool.Name)
		}
	}
	for _, name := range []string{"add_item", "get"} {
		if !found[name] {
			t.Errorf("tool %s is not registered", name)
		}
	}
}

func TestAddItemThenGet(t *testing.T) {
	session, _ := connect(t)

	added := structured(t, call(t, session, "add_item", map[string]any{
		"kind": "movie", "title": "Dune: Part Two", "rating": 4, "occurred_on": "2024-03-01", "dictated": true,
	}))
	if added["source"] != "agent" || added["verified"] != true {
		t.Errorf("source, verified = %v, %v, want agent, true", added["source"], added["verified"])
	}

	got := structured(t, call(t, session, "get", map[string]any{"type": "item", "id": added["id"]}))
	if got["title"] != "Dune: Part Two" || got["rating"] != float64(4) {
		t.Errorf("get returned %v", got)
	}
}

// A row that the agent inferred must wait for review. The safe default when
// the model leaves out "dictated" is not verified.
func TestAddItemIsNotVerifiedUnlessDictated(t *testing.T) {
	session, _ := connect(t)

	added := structured(t, call(t, session, "add_item", map[string]any{"kind": "book", "title": "Dune"}))

	if added["verified"] != false {
		t.Errorf("verified = %v, want false when dictated is absent", added["verified"])
	}
}

func TestToolsReturnErrorsTheModelCanRead(t *testing.T) {
	session, pool := connect(t)
	tests := []struct {
		name     string
		tool     string
		args     map[string]any
		wantText string
	}{
		{"rating the service refuses", "add_item", map[string]any{"kind": "movie", "title": "Dune", "rating": 7}, "rating"},
		{"kind the service refuses", "add_item", map[string]any{"kind": "film", "title": "Dune"}, "kind"},
		{"misspelled argument", "add_item", map[string]any{"kind": "movie", "title": "Dune", "ratting": 4}, "ratting"},
		{"missing title", "add_item", map[string]any{"kind": "movie"}, "title"},
		{"rating of the wrong type", "add_item", map[string]any{"kind": "movie", "title": "Dune", "rating": "four"}, "rating"},
		{"unknown ID", "get", map[string]any{"type": "item", "id": 9999}, "not found"},
		{"unknown type", "get", map[string]any{"type": "journal", "id": 1}, "type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := call(t, session, tt.tool, tt.args)
			if !result.IsError {
				t.Fatalf("IsError = false, want an error result")
			}
			if !strings.Contains(text(result), tt.wantText) {
				t.Errorf("error text %q does not name %q", text(result), tt.wantText)
			}
		})
	}
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM catalog_items").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("catalog_items has %d rows after rejected calls, want 0", n)
	}
}

func TestDatabaseFailureGivesAPlainError(t *testing.T) {
	session, pool := connect(t)
	pool.Close()

	result := call(t, session, "add_item", map[string]any{"kind": "movie", "title": "Dune"})

	if !result.IsError {
		t.Fatal("IsError = false, want an error result")
	}
	if got := text(result); got != "internal error" {
		t.Errorf("error text = %q, want only \"internal error\"", got)
	}
}
```

- [ ] **Step 2: Run the test to verify that it fails**

Run:
```bash
go mod tidy
make test PKG=./internal/mcp
```
Expected: FAIL. The output contains `no non-test Go files` for `internal/mcp`, because `NewServer` does not exist yet.

- [ ] **Step 3: Write the adapter**

Create `internal/mcp/server.go`:

```go
// Package mcp is the MCP adapter. Each tool reads its input, calls one
// service function and returns the result. Tools hold no logic; they carry
// the descriptions that the model reads.
package mcp

import (
	"errors"
	"log/slog"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// NewServer returns the MCP server named lifeos with every tool registered.
func NewServer(svc *service.Service) *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{Name: "lifeos", Version: "0.1.0"}, nil)
	registerItems(server, svc)
	return server
}

// Handler serves the lifeos MCP server over streamable HTTP.
func Handler(svc *service.Service) http.Handler {
	server := NewServer(svc)
	return sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil)
}

// toToolError turns a service error into the error text that the model
// reads. It logs an unknown error and hides its text.
func toToolError(err error) error {
	var invalid *service.ValidationError
	if errors.As(err, &invalid) {
		return errors.New("invalid input: " + invalid.Error() + ". Ask the user; do not guess a value.")
	}
	if errors.Is(err, service.ErrNotFound) {
		return errors.New("not found")
	}
	slog.Error("mcp: tool failed", "error", err)
	return errors.New("internal error")
}
```

Create `internal/mcp/items.go`. The `jsonschema` tag is the description that the model reads for each argument. The SDK builds the input schema from this struct and refuses unknown arguments.

```go
package mcp

import (
	"context"
	"errors"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

type addItemInput struct {
	Kind       string         `json:"kind" jsonschema:"One of movie, book, place, game. If the kind is not clear, ask the user. Never guess."`
	Title      string         `json:"title" jsonschema:"Title or name of the item"`
	Rating     *float64       `json:"rating,omitempty" jsonschema:"The user's rating, 0 to 5 in steps of 0.5"`
	OccurredOn string         `json:"occurred_on,omitempty" jsonschema:"Day watched, read, visited or played, as YYYY-MM-DD in India Standard Time"`
	Attrs      map[string]any `json:"attrs,omitempty" jsonschema:"More fields for this kind, such as author, director or year"`
	Dictated   bool           `json:"dictated,omitempty" jsonschema:"true only when the user stated this item directly. false when you inferred it from a source; the row then waits in the review queue."`
}

type getInput struct {
	Type string `json:"type" jsonschema:"Record type. The only type is: item"`
	ID   int64  `json:"id" jsonschema:"Record ID"`
}

func registerItems(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "add_item",
		Description: "Add a movie, book, place or game to the user's lists. Returns the stored item with its ID.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in addItemInput) (*sdk.CallToolResult, service.Item, error) {
		item, err := svc.AddItem(ctx, service.AddItemParams{
			Kind:       in.Kind,
			Title:      in.Title,
			Rating:     in.Rating,
			OccurredOn: in.OccurredOn,
			Attrs:      in.Attrs,
			Provenance: service.Provenance{Source: service.SourceAgent, Verified: in.Dictated},
		})
		if err != nil {
			return nil, service.Item{}, toToolError(err)
		}
		return nil, item, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "get",
		Description: "Read one record by type and ID.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in getInput) (*sdk.CallToolResult, service.Item, error) {
		if in.Type != "item" {
			return nil, service.Item{}, errors.New("invalid input: type: must be item")
		}
		item, err := svc.GetItem(ctx, in.ID)
		if err != nil {
			return nil, service.Item{}, toToolError(err)
		}
		return nil, item, nil
	})
}
```

- [ ] **Step 4: Run the tests to verify that they pass**

Run:
```bash
go mod tidy
make test
```
Expected: `ok` for `internal/api`, `internal/config`, `internal/mcp`, `internal/service` and `migrations`.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/mcp/server.go internal/mcp/items.go internal/mcp/items_test.go
git commit -m "feat(mcp): add the add_item and get tools"
```

---

### Task 5: Admin page for items

**Files:**
- Create: `internal/admin/admin.go`, `internal/admin/items.templ`, `internal/admin/static/htmx.min.js` (downloaded)
- Modify: `Makefile`, `.gitignore`, `go.mod`, `go.sum`
- Test: `internal/admin/items_test.go`

**Interfaces:**
- Consumes (Task 2): `service.New`, `(*Service).AddItem`, `ListItems`, `service.AddItemParams`, `service.ListItemsParams`, `service.Item`, `service.ItemKinds`, `service.IST`, `service.Provenance`, `service.SourceManual`, `*service.ValidationError`; `testdb.New` (Task 1).
- Produces: `admin.Register(router chi.Router, svc *service.Service)`. It adds:
  - `GET /admin/items`: the page with the form and the list
  - `POST /admin/items` (form fields `kind`, `title`, `rating`, `occurred_on`): 200 with the `<div id="list">` fragment. On success the response has the header `HX-Trigger: item-added`. When the service refuses the input, the fragment shows the reason and there is no `HX-Trigger` header. The row is `manual` and verified.
  - `GET /admin/static/htmx.min.js`

- [ ] **Step 1: Add templ as a Go tool and download htmx**

Run:
```bash
go get -tool github.com/a-h/templ/cmd/templ@v0.3.1070
go tool templ version
```
Expected: `v0.3.1070`. The first `go tool templ` run compiles templ and takes about one minute on the Pi.

Run:
```bash
mkdir -p internal/admin/static
curl -fsSL https://unpkg.com/htmx.org@2.0.11/dist/htmx.min.js -o internal/admin/static/htmx.min.js
echo "d6fdc75f204e6bdefa99b69bf1e6d4ac69b8a364f77929f45c13476b4000f717  internal/admin/static/htmx.min.js" | sha256sum -c
```
Expected: `internal/admin/static/htmx.min.js: OK`. This file is a third-party library that the binary embeds. It goes into git.

- [ ] **Step 2: Add templ to the Makefile and to `.gitignore`**

Replace the content of `Makefile` with this version. The one change is the line `go tool templ generate` in the `generate` target.

```make
# Reads POSTGRES_PASSWORD from deploy/.env, which is not in git.
include deploy/.env
export POSTGRES_PASSWORD
# Tests create and drop their own databases on this server.
export LIFEOS_TEST_DATABASE_URL = postgres://lifeos:$(POSTGRES_PASSWORD)@127.0.0.1:5432/postgres?sslmode=disable

# Narrow a test run with: make test PKG=./internal/api ARGS="-run TestName -v"
PKG ?= ./...
ARGS ?=

SQLC_VERSION = 1.31.1
SQLC = $(shell go env GOPATH)/bin/sqlc

.PHONY: tools generate build test migrate

# Installs the code generators that are not Go modules of this repo.
tools:
	curl -fsSL https://github.com/sqlc-dev/sqlc/releases/download/v$(SQLC_VERSION)/sqlc_$(SQLC_VERSION)_linux_arm64.tar.gz | tar -xz -C $(dir $(SQLC)) sqlc

# Generated code is not in git. Each target that compiles runs this first.
generate:
	$(SQLC) generate
	go tool templ generate

build: generate
	go build -o bin/lifeos ./cmd/lifeos

test: generate
	go vet ./...
	go test $(PKG) $(ARGS)

migrate: build
	bin/lifeos migrate
```

Add this line to the end of `.gitignore`:

```
*_templ.go
```

- [ ] **Step 3: Write the failing test**

Create `internal/admin/items_test.go`:

```go
package admin_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/admin"
	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

func newTestAdmin(t *testing.T) (http.Handler, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.New(t)
	router := chi.NewRouter()
	admin.Register(router, service.New(pool))
	return router, pool
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func postForm(h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestItemsPageShowsTheFormAndAnEmptyList(t *testing.T) {
	h, _ := newTestAdmin(t)

	rec := get(h, "/admin/items")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{`hx-post="/admin/items"`, `name="title"`, `<option value="movie">`, "No items yet.", "/admin/static/htmx.min.js"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %q", want)
		}
	}
}

func TestAddItemStoresTheRowAndShowsIt(t *testing.T) {
	h, pool := newTestAdmin(t)

	rec := postForm(h, "/admin/items", url.Values{
		"kind": {"movie"}, "title": {"Dune: Part Two"}, "rating": {"4.5"}, "occurred_on": {"2024-03-01"},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("HX-Trigger"); got != "item-added" {
		t.Errorf("HX-Trigger = %q, want item-added so that the form resets", got)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(body), `<div id="list">`) {
		t.Errorf("the response is not the #list fragment:\n%s", body)
	}
	for _, want := range []string{"Dune: Part Two", "4.5", "2024-03-01", "manual", "yes"} {
		if !strings.Contains(body, want) {
			t.Errorf("the list does not contain %q", want)
		}
	}

	var source string
	var verified bool
	if err := pool.QueryRow(t.Context(), "SELECT source, verified FROM catalog_items").Scan(&source, &verified); err != nil {
		t.Fatalf("read the row (want exactly one): %v", err)
	}
	if source != "manual" || !verified {
		t.Errorf("source, verified = %q, %v, want manual, true", source, verified)
	}

	if page := get(h, "/admin/items").Body.String(); !strings.Contains(page, "Dune: Part Two") {
		t.Error("the full page does not list the new item")
	}
}

func TestAddItemShowsWhyTheServiceRefusedTheInput(t *testing.T) {
	tests := []struct {
		name     string
		form     url.Values
		wantText string
	}{
		{"rating above 5", url.Values{"kind": {"movie"}, "title": {"Dune"}, "rating": {"7"}}, "rating: must be from 0 to 5"},
		{"rating that is not a number", url.Values{"kind": {"movie"}, "title": {"Dune"}, "rating": {"four"}}, "rating: must be a number"},
		{"no title", url.Values{"kind": {"movie"}, "title": {""}}, "title: must not be empty"},
		{"no kind", url.Values{"title": {"Dune"}}, "kind: must be one of"},
		{"date in the future", url.Values{"kind": {"movie"}, "title": {"Dune"}, "occurred_on": {"2099-01-01"}}, "occurred_on: must not be in the future"},
	}
	h, pool := newTestAdmin(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postForm(h, "/admin/items", tt.form)

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
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM catalog_items").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("catalog_items has %d rows after refused input, want 0", n)
	}
}

// A title is text that Gautam or an import supplied. The page must show
// it as text and must not run it as HTML.
func TestItemsPageEscapesTheTitle(t *testing.T) {
	h, _ := newTestAdmin(t)
	postForm(h, "/admin/items", url.Values{"kind": {"movie"}, "title": {`<script>alert("x")</script>`}})

	body := get(h, "/admin/items").Body.String()

	if strings.Contains(body, `<script>alert(`) {
		t.Error("the page contains the title as raw HTML")
	}
	if !strings.Contains(body, "&lt;script&gt;alert(") {
		t.Errorf("the page does not contain the escaped title")
	}
}

func TestStaticServesHtmx(t *testing.T) {
	h, _ := newTestAdmin(t)

	rec := get(h, "/admin/static/htmx.min.js")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want JavaScript", ct)
	}
	if !strings.Contains(rec.Body.String(), "htmx") {
		t.Error("the file does not look like htmx")
	}
}

func TestDatabaseFailureGivesAPlain500(t *testing.T) {
	h, pool := newTestAdmin(t)
	pool.Close()

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"page": get(h, "/admin/items"),
		"post": postForm(h, "/admin/items", url.Values{"kind": {"movie"}, "title": {"Dune"}}),
	} {
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status = %d, want 500", name, rec.Code)
		}
		if got := strings.TrimSpace(rec.Body.String()); got != "internal error" {
			t.Errorf("%s: body = %q, want only \"internal error\"", name, got)
		}
	}
}
```

- [ ] **Step 4: Run the test to verify that it fails**

Run:
```bash
go mod tidy
make test PKG=./internal/admin
```
Expected: FAIL. The output contains `no non-test Go files` for `internal/admin`, because `admin.Register` does not exist yet.

- [ ] **Step 5: Write the template**

Create `internal/admin/items.templ`. The form posts with htmx and replaces `#list`. The form is outside `#list`, so it keeps what Gautam typed when the service refuses the input. templ escapes every `{ ... }` value.

```templ
package admin

import (
	"strconv"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

templ page(title string) {
	<!DOCTYPE html>
	<html lang="en">
		<head>
			<meta charset="utf-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1"/>
			<title>{ title } · LifeOS</title>
			<script src="/admin/static/htmx.min.js"></script>
			<style>
				body { font-family: system-ui, sans-serif; max-width: 60rem; margin: 2rem auto; padding: 0 1rem; }
				form { display: flex; flex-wrap: wrap; gap: 0.5rem; align-items: end; margin-bottom: 1.5rem; }
				label { display: flex; flex-direction: column; font-size: 0.85rem; }
				table { border-collapse: collapse; width: 100%; }
				th, td { text-align: left; padding: 0.4rem 0.6rem; border-bottom: 1px solid #ddd; }
				.error { color: #b00020; }
				.muted { color: #666; }
			</style>
		</head>
		<body>
			{ children... }
		</body>
	</html>
}

templ itemsPage(items []service.Item) {
	@page("Items") {
		<h1>Items</h1>
		<form hx-post="/admin/items" hx-target="#list" hx-swap="outerHTML" hx-on:item-added="this.reset()">
			<label>
				Kind
				<select name="kind" required>
					for _, kind := range service.ItemKinds {
						<option value={ kind }>{ kind }</option>
					}
				</select>
			</label>
			<label>
				Title
				<input type="text" name="title" required maxlength="300" size="40"/>
			</label>
			<label>
				Rating
				<input type="number" name="rating" min="0" max="5" step="0.5"/>
			</label>
			<label>
				Date
				<input type="date" name="occurred_on"/>
			</label>
			<button type="submit">Add</button>
		</form>
		@itemsList(items, "")
	}
}

// itemsList is the part of the page that a form post replaces.
templ itemsList(items []service.Item, errorMessage string) {
	<div id="list">
		if errorMessage != "" {
			<p class="error" role="alert">{ errorMessage }</p>
		}
		if len(items) == 0 {
			<p class="muted">No items yet.</p>
		} else {
			<table>
				<thead>
					<tr>
						<th>ID</th>
						<th>Kind</th>
						<th>Title</th>
						<th>Rating</th>
						<th>Date</th>
						<th>Source</th>
						<th>Verified</th>
						<th>Added (IST)</th>
					</tr>
				</thead>
				<tbody>
					for _, item := range items {
						<tr>
							<td>{ strconv.FormatInt(item.ID, 10) }</td>
							<td>{ item.Kind }</td>
							<td>{ item.Title }</td>
							<td>{ formatRating(item.Rating) }</td>
							<td>{ item.OccurredOn }</td>
							<td>{ string(item.Source) }</td>
							<td>{ formatVerified(item.Verified) }</td>
							<td>{ item.CreatedAt.In(service.IST).Format("2 Jan 2006 15:04") }</td>
						</tr>
					}
				</tbody>
			</table>
		}
	</div>
}

func formatRating(rating *float64) string {
	if rating == nil {
		return ""
	}
	return strconv.FormatFloat(*rating, 'f', -1, 64)
}

func formatVerified(verified bool) string {
	if verified {
		return "yes"
	}
	return "no"
}
```

- [ ] **Step 6: Write the handlers**

Create `internal/admin/admin.go`. A refused input gives status 200, because htmx does not show the body of a 4xx response.

```go
// Package admin is the adapter for the server-rendered admin pages (templ
// and htmx). Handlers read the form, call one service function and render
// the result. They hold no logic.
package admin

import (
	"context"
	"embed"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

//go:embed static
var static embed.FS

// Register adds every /admin route to router.
func Register(router chi.Router, svc *service.Service) {
	h := &handlers{svc: svc}
	router.Get("/admin/items", h.itemsPage)
	router.Post("/admin/items", h.addItem)
	router.Handle("/admin/static/*", http.StripPrefix("/admin/", http.FileServerFS(static)))
}

type handlers struct {
	svc *service.Service
}

func (h *handlers) itemsPage(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListItems(r.Context(), service.ListItemsParams{})
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, itemsPage(items))
}

// addItem stores the item from the form and renders the list again. When
// the service refuses the input, the list shows the reason and the form
// keeps what Gautam typed.
func (h *handlers) addItem(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	message := ""
	params, err := itemFromForm(r)
	if err == nil {
		_, err = h.svc.AddItem(r.Context(), params)
	}
	var invalid *service.ValidationError
	switch {
	case err == nil:
		// htmx turns this header into an event; the form resets on it.
		w.Header().Set("HX-Trigger", "item-added")
	case errors.As(err, &invalid):
		message = invalid.Error()
	default:
		internalError(w, err)
		return
	}

	items, err := h.svc.ListItems(r.Context(), service.ListItemsParams{})
	if err != nil {
		internalError(w, err)
		return
	}
	render(w, r, itemsList(items, message))
}

// itemFromForm reads the form fields. A row from the admin pages is one
// that Gautam typed, so it is manual and verified.
func itemFromForm(r *http.Request) (service.AddItemParams, error) {
	params := service.AddItemParams{
		Kind:       r.PostForm.Get("kind"),
		Title:      r.PostForm.Get("title"),
		OccurredOn: r.PostForm.Get("occurred_on"),
		Provenance: service.Provenance{Source: service.SourceManual, Verified: true},
	}
	if raw := r.PostForm.Get("rating"); raw != "" {
		rating, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return params, &service.ValidationError{Field: "rating", Message: "must be a number"}
		}
		params.Rating = &rating
	}
	return params, nil
}

type component interface {
	Render(ctx context.Context, w io.Writer) error
}

func render(w http.ResponseWriter, r *http.Request, c component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Render(r.Context(), w); err != nil {
		slog.Error("admin: render failed", "error", err)
	}
}

func internalError(w http.ResponseWriter, err error) {
	slog.Error("admin: request failed", "error", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
```

- [ ] **Step 7: Run the tests to verify that they pass**

Run:
```bash
go mod tidy
make test
git status --short
```
Expected: `ok` for `internal/admin`, `internal/api`, `internal/config`, `internal/mcp`, `internal/service` and `migrations`. `git status` does not show `internal/admin/items_templ.go`.

- [ ] **Step 8: Commit**

```bash
git add Makefile .gitignore go.mod go.sum internal/admin/admin.go internal/admin/items.templ internal/admin/items_test.go internal/admin/static/htmx.min.js
git commit -m "feat(admin): add the items page with templ and htmx"
```

---

### Task 6: `lifeos serve` and the end-to-end test

**Files:**
- Create: `cmd/lifeos/serve.go`
- Modify: `cmd/lifeos/main.go` (delete one stub line), `Makefile`, `AGENTS.md`
- Test: `cmd/lifeos/serve_test.go`

**Interfaces:**
- Consumes: `config.Load`, `config.Config` and `poolConfig` (Task 1); `service.New` (Task 2); `api.Register` (Task 3); `lifeosmcp.Handler` (Task 4); `admin.Register` (Task 5); `testdb.New` (Task 1).
- Produces (package `main`):
  - `listen(cfg config.Config) (httpListener, mcpListener net.Listener, err error)`
  - `serveOn(ctx context.Context, httpListener, mcpListener net.Listener, svc *service.Service) error`: returns nil after `ctx` is cancelled
  - `lifeos serve [-config path]`: API and admin pages on `http_addr`, MCP on `mcp_addr` at the path `/mcp`
  - `make run`

- [ ] **Step 1: Write the failing test**

Create `cmd/lifeos/serve_test.go`. The first test sends one item through REST and one through MCP over real HTTP, then checks that the admin page shows the two items and that each write has its event.

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/config"
	"github.com/Gautam-J/lifeos-core/internal/service"
	"github.com/Gautam-J/lifeos-core/internal/testdb"
)

// freeLoopbackConfig returns a config whose two addresses are free ports.
func freeLoopbackConfig(t *testing.T) config.Config {
	t.Helper()
	addrs := make([]string, 2)
	for i := range addrs {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addrs[i] = l.Addr().String()
		l.Close()
	}
	return config.Config{HTTPAddr: addrs[0], MCPAddr: addrs[1]}
}

// One item goes in through each adapter. Each adapter must then see the
// two items, and each write must have its event: the three adapters use
// one service and one database.
func TestServeRunsOneFlowThroughEveryAdapter(t *testing.T) {
	pool := testdb.New(t)
	cfg := freeLoopbackConfig(t)
	httpListener, mcpListener, err := listen(cfg)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveOn(ctx, httpListener, mcpListener, service.New(pool)) }()

	// 1. REST: add an item.
	resp, err := http.Post("http://"+cfg.HTTPAddr+"/api/items", "application/json",
		strings.NewReader(`{"kind":"movie","title":"Added through REST","rating":4}`))
	if err != nil {
		t.Fatalf("POST /api/items: %v", err)
	}
	var added struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&added); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated || added.ID == 0 {
		t.Fatalf("POST /api/items: status %d, id %d", resp.StatusCode, added.ID)
	}

	// 2. MCP over HTTP: read that item, then add a second one.
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: "http://" + cfg.MCPAddr + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("MCP connect: %v", err)
	}
	got, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "get", Arguments: map[string]any{"type": "item", "id": added.ID}})
	if err != nil || got.IsError {
		t.Fatalf("MCP get: err %v, result %+v", err, got)
	}
	if title := got.StructuredContent.(map[string]any)["title"]; title != "Added through REST" {
		t.Errorf("MCP get title = %v, want the item that REST added", title)
	}
	second, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "add_item", Arguments: map[string]any{
		"kind": "book", "title": "Added through MCP", "dictated": true,
	}})
	if err != nil || second.IsError {
		t.Fatalf("MCP add_item: err %v, result %+v", err, second)
	}
	session.Close()

	// 3. Admin page: the two items show.
	page, err := http.Get("http://" + cfg.HTTPAddr + "/admin/items")
	if err != nil {
		t.Fatalf("GET /admin/items: %v", err)
	}
	html, _ := io.ReadAll(page.Body)
	page.Body.Close()
	for _, title := range []string{"Added through REST", "Added through MCP"} {
		if !strings.Contains(string(html), title) {
			t.Errorf("the admin page does not show %q", title)
		}
	}

	// 4. Each write has its event, with the source of its adapter.
	rows, err := pool.Query(ctx, "SELECT source FROM events ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	var sources []string
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, source)
	}
	if fmt.Sprint(sources) != "[manual agent]" {
		t.Errorf("event sources = %v, want [manual agent]", sources)
	}

	// 5. A cancelled context stops the two servers.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serveOn returned %v after cancel, want nil", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("serveOn did not return within 15 s after cancel")
	}
}

func TestListenNamesTheConfigKeyOfABusyPort(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()

	for _, key := range []string{"http_addr", "mcp_addr"} {
		t.Run(key, func(t *testing.T) {
			cfg := freeLoopbackConfig(t)
			if key == "http_addr" {
				cfg.HTTPAddr = busy.Addr().String()
			} else {
				cfg.MCPAddr = busy.Addr().String()
			}

			_, _, err := listen(cfg)

			if err == nil {
				t.Fatal("listen returned no error for a port that is in use")
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error %q does not name %s", err, key)
			}
			// The free port of this config must be released again.
			free := cfg.MCPAddr
			if key == "mcp_addr" {
				free = cfg.HTTPAddr
			}
			l, err := net.Listen("tcp", free)
			if err != nil {
				t.Errorf("the other listener stayed open: %v", err)
			} else {
				l.Close()
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify that it fails**

Run: `make test PKG=./cmd/lifeos`
Expected: FAIL. The output contains `undefined: listen`.

- [ ] **Step 3: Write the `serve` subcommand**

Create `cmd/lifeos/serve.go`:

```go
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/admin"
	"github.com/Gautam-J/lifeos-core/internal/api"
	"github.com/Gautam-J/lifeos-core/internal/config"
	lifeosmcp "github.com/Gautam-J/lifeos-core/internal/mcp"
	"github.com/Gautam-J/lifeos-core/internal/service"
)

func serve(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := flags.String("config", "config/lifeos.yaml", "path to the config file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	pc, err := poolConfig(cfg)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("database: %w", err)
	}

	httpListener, mcpListener, err := listen(cfg)
	if err != nil {
		return err
	}
	slog.Info("lifeos is listening", "http", httpListener.Addr().String(), "mcp", mcpListener.Addr().String())
	return serveOn(ctx, httpListener, mcpListener, service.New(pool))
}

// listen opens the two listeners before any request is served, so that a
// port that is in use stops the start with an error that names the key.
func listen(cfg config.Config) (httpListener, mcpListener net.Listener, err error) {
	httpListener, err = net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("http_addr: %w", err)
	}
	mcpListener, err = net.Listen("tcp", cfg.MCPAddr)
	if err != nil {
		httpListener.Close()
		return nil, nil, fmt.Errorf("mcp_addr: %w", err)
	}
	return httpListener, mcpListener, nil
}

// serveOn serves the REST API and the admin pages on httpListener and the
// MCP server on mcpListener. It returns nil after ctx is cancelled and the
// servers have shut down.
func serveOn(ctx context.Context, httpListener, mcpListener net.Listener, svc *service.Service) error {
	router := chi.NewRouter()
	api.Register(router, svc)
	admin.Register(router, svc)

	mcpMux := http.NewServeMux()
	mcpMux.Handle("/mcp", lifeosmcp.Handler(svc))

	servers := []*http.Server{
		{Handler: router, ReadHeaderTimeout: 10 * time.Second},
		{Handler: mcpMux, ReadHeaderTimeout: 10 * time.Second},
	}
	failed := make(chan error, len(servers))
	for i, listener := range []net.Listener{httpListener, mcpListener} {
		go func() { failed <- servers[i].Serve(listener) }()
	}

	select {
	case err := <-failed:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var errs []error
	for _, server := range servers {
		if err := server.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
```

In `cmd/lifeos/main.go`, delete this one line (the new file replaces the stub):

```go
func serve(args []string) error     { return errNotImplemented }
```

Then run `gofmt -w cmd/lifeos/main.go`.

- [ ] **Step 4: Add the `run` target to the Makefile**

Replace the content of `Makefile` with this version. The changes are `run` in the `.PHONY` line and the `run` target at the end.

```make
# Reads POSTGRES_PASSWORD from deploy/.env, which is not in git.
include deploy/.env
export POSTGRES_PASSWORD
# Tests create and drop their own databases on this server.
export LIFEOS_TEST_DATABASE_URL = postgres://lifeos:$(POSTGRES_PASSWORD)@127.0.0.1:5432/postgres?sslmode=disable

# Narrow a test run with: make test PKG=./internal/api ARGS="-run TestName -v"
PKG ?= ./...
ARGS ?=

SQLC_VERSION = 1.31.1
SQLC = $(shell go env GOPATH)/bin/sqlc

.PHONY: tools generate build test migrate run

# Installs the code generators that are not Go modules of this repo.
tools:
	curl -fsSL https://github.com/sqlc-dev/sqlc/releases/download/v$(SQLC_VERSION)/sqlc_$(SQLC_VERSION)_linux_arm64.tar.gz | tar -xz -C $(dir $(SQLC)) sqlc

# Generated code is not in git. Each target that compiles runs this first.
generate:
	$(SQLC) generate
	go tool templ generate

build: generate
	go build -o bin/lifeos ./cmd/lifeos

test: generate
	go vet ./...
	go test $(PKG) $(ARGS)

migrate: build
	bin/lifeos migrate

run: build
	bin/lifeos serve
```

- [ ] **Step 5: Run the tests to verify that they pass**

Run:
```bash
go mod tidy
make test
```
Expected: `ok` for `cmd/lifeos`, `internal/admin`, `internal/api`, `internal/config`, `internal/mcp`, `internal/service` and `migrations`.

- [ ] **Step 6: Commit**

```bash
git add Makefile go.mod go.sum cmd/lifeos/serve.go cmd/lifeos/serve_test.go cmd/lifeos/main.go
git commit -m "feat: serve the API, admin pages and MCP server"
```

- [ ] **Step 7: Update `AGENTS.md`**

Make these four edits in `AGENTS.md`.

Edit 1. In the "Layout" code block, replace this line:

```
internal/store/        sqlc-generated queries, from queries/*.sql
```

with these three lines:

```
internal/store/        sqlc-generated queries, from queries/*.sql (not in git)
internal/config/       loads config/lifeos.yaml
internal/testdb/       test helper: one migrated database for each test
```

Edit 2. In the same code block, replace these two lines:

```
migrations/            goose SQL migrations
config/lifeos.yaml     models, schedules, connectors, wiki and prompt paths, notifications
```

with these four lines:

```
migrations/            goose SQL migrations and the Go code that applies them
config/lifeos.yaml     listen addresses, database URL; later models, schedules, connectors, notifications
sqlc.yaml              sqlc settings
Makefile               tools, generate, build, test, migrate, run
```

Edit 3. In rule 4, after the sentence that ends with "rows Gautam dictates directly are verified.", add this sentence:

```
The adapter sets this: the REST API and the admin pages write `manual`, verified rows; an MCP tool writes `agent` rows that are verified only when the tool input says the user dictated the row.
```

Edit 4. Replace the full "Commands" section (from the line `## Commands` to the line before `## Migrations`) with this text:

````markdown
## Commands

```sh
make tools      # one time: install sqlc into ~/go/bin
make generate   # run sqlc and templ: writes internal/store/ and *_templ.go
make build      # generate, then build bin/lifeos
make test       # generate, go vet, then all tests
make migrate    # build, then apply migrations to the database in config/lifeos.yaml
make run        # build, then run lifeos serve
```

- **Generated code is not in git.** After a fresh clone, run `make generate` before `go build` or `go vet`.
- **Tests need Postgres.** Start the container from `deploy/docker-compose.yml`; the Makefile reads `deploy/.env`. Each test creates its own database and drops it. `go test` without `make` fails, because `LIFEOS_TEST_DATABASE_URL` is not set.
- **Narrow a test run** with `make test PKG=./internal/api ARGS="-run TestName -v"`.

`lifeos serve` runs the API and admin pages on `http_addr` and the MCP server on `mcp_addr`, path `/mcp`. `lifeos migrate`, `lifeos job <name>` and `lifeos import-csv <importer> <file>` are run by hand or by systemd timers. Each command reads `config/lifeos.yaml`, or the file given with `-config`.
````

- [ ] **Step 8: Commit**

```bash
git add AGENTS.md
git commit -m "docs: describe the make targets and the generated code in AGENTS.md"
```

---

### Task 7: Smoke check on the Pi and the confirmation gate

This task has no new code. It runs the real binary against a throwaway database, then starts it on the real database for Gautam.

**Files:** none

**Interfaces:**
- Consumes: `bin/lifeos` with `migrate` and `serve` (Tasks 1 to 6).
- Produces: a result for Gautam to confirm. Later plans start only after that confirmation.

- [ ] **Step 1: Make a throwaway database and config**

Run (put `sudo` before `docker` if the shell reports `permission denied`):
```bash
cd /home/gautam/lifeos-core
make build
docker compose -f deploy/docker-compose.yml exec -T postgres psql -U lifeos -d postgres -c 'CREATE DATABASE lifeos_smoke'
sed 's|/lifeos?sslmode|/lifeos_smoke?sslmode|; s|8200|8290|; s|8100|8190|' config/lifeos.yaml > /tmp/lifeos-smoke.yaml
set -a; . deploy/.env; set +a
bin/lifeos migrate -config /tmp/lifeos-smoke.yaml
```
Expected: `CREATE DATABASE`, then `applied 00001_init.sql`.

- [ ] **Step 2: Start the server**

Run:
```bash
bin/lifeos serve -config /tmp/lifeos-smoke.yaml > /tmp/lifeos-smoke.log 2>&1 &
echo $! > /tmp/lifeos-smoke.pid
sleep 2; cat /tmp/lifeos-smoke.log
```
Expected: one line that contains `lifeos is listening http=127.0.0.1:8290 mcp=127.0.0.1:8190`.

- [ ] **Step 3: Check the REST API**

Run:
```bash
curl -s -X POST http://127.0.0.1:8290/api/items -H 'Content-Type: application/json' \
  -d '{"kind":"movie","title":"Dune: Part Two","rating":4,"occurred_on":"2024-03-01"}'; echo
curl -s -X POST http://127.0.0.1:8290/api/items -H 'Content-Type: application/json' \
  -d '{"kind":"movie","title":"Dune","rating":7}'; echo
curl -s -o /dev/null -w 'openapi %{http_code}\n' http://127.0.0.1:8290/api/openapi.json
```
Expected:
- The first response contains `"id":1`, `"source":"manual"` and `"verified":true`.
- The second response contains `"status":422` and `"location":"body.rating"`.
- The third line is `openapi 200`.

- [ ] **Step 4: Check the admin page**

Run:
```bash
curl -s http://127.0.0.1:8290/admin/items | grep -o '<td>[^<]*</td>' | tr '\n' ' '; echo
curl -s -X POST http://127.0.0.1:8290/admin/items -d 'kind=book&title=Dune&rating=5' -D - -o /dev/null | grep -i 'hx-trigger'
```
Expected:
- The first line starts with `<td>1</td> <td>movie</td> <td>Dune: Part Two</td> <td>4</td> <td>2024-03-01</td> <td>manual</td> <td>yes</td>`.
- The second line is `Hx-Trigger: item-added`.

- [ ] **Step 5: Check the MCP server and the events**

Run:
```bash
curl -s -X POST http://127.0.0.1:8190/mcp -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"0"}}}'
docker compose -f deploy/docker-compose.yml exec -T postgres psql -U lifeos -d lifeos_smoke -At \
  -c 'SELECT action, record_type, record_id, source FROM events ORDER BY id'
```
Expected:
- The first response contains `"serverInfo":{"name":"lifeos","version":"0.1.0"}`.
- The query prints `create|catalog_items|1|manual` and `create|catalog_items|2|manual`.

- [ ] **Step 6: Stop the server and drop the throwaway database**

Run:
```bash
kill "$(cat /tmp/lifeos-smoke.pid)"; sleep 1
docker compose -f deploy/docker-compose.yml exec -T postgres psql -U lifeos -d postgres -c 'DROP DATABASE lifeos_smoke'
rm /tmp/lifeos-smoke.yaml /tmp/lifeos-smoke.log /tmp/lifeos-smoke.pid
```
Expected: `DROP DATABASE`.

- [ ] **Step 7: Start the server on the real database**

Run this in a tmux pane that stays open:
```bash
cd /home/gautam/lifeos-core
make run
```
Expected: one line that contains `lifeos is listening http=127.0.0.1:8200 mcp=127.0.0.1:8100`.

- [ ] **Step 8: Hand over to Gautam and stop**

Give Gautam these three checks. Tell Gautam to add real items only: this slice has no delete, so a test row stays in the database.

1. **Admin page.** On the Mac, run `ssh -L 8200:127.0.0.1:8200 <the Pi>`. Open `http://127.0.0.1:8200/admin/items` in a browser. Add one item. The list must show it at the top, and the form must become empty. Then enter a rating of 7; the page must show `rating: must be from 0 to 5 in steps of 0.5`, and the form must keep the input.
2. **MCP from Claude Code on the Pi.** Run `claude mcp add --transport http --scope local lifeos http://127.0.0.1:8100/mcp`. Start `claude` and ask it to add an item, for example a book that Gautam read. The admin page must show the item with source `agent`. To remove the server later, run `claude mcp remove lifeos`.
3. **REST.** Run `curl -s 'http://127.0.0.1:8200/api/items?limit=5'` on the Pi. The response must list the new items, newest first.

The form reset and the error display in check 1 depend on htmx in a real browser. No automated test covers them.

Stop here. Do not start the work in "After the confirmation gate" until Gautam confirms that the three checks pass.

---

## After the confirmation gate

These are the parts of Phase 2 that this plan does not cover. Each becomes its own plan after Gautam confirms Task 7. Each plan produces working software on its own.

| Plan | Content | Spec sections |
| --- | --- | --- |
| A. Records | Migrations, queries and service functions for `sources` (with `capture` and the SHA-256 file store), `journal_entries`, `habits` and `habit_logs`, `todos`, `accounts`, `transactions` and `budgets`. Generic `update` (old value in `events`), soft `delete`, `search`, the review queue and `verify`. The read-only Postgres role for `query`. Rejection of card numbers and secrets in captured text. | Data model; Access layer table; E1, E3, E4, E8 |
| B. Adapters | A REST endpoint and an MCP tool for each capability in plan A (`capture`, `journal`, `log_habit`, `todo`, `update`, `delete`, `search`, `get` for every type, `query`, `review_queue`, `verify`). Admin pages: review queue, record lists, edit forms, permanent delete. | Access layer table; S1 to S4, S12, S13, S21, S24, S25 |
| C. Importers | `lifeos import-csv` for a bank statement, Letterboxd and Goodreads, with duplicate detection. | Connectors and capture; S11 |
| D. Run and protect | A systemd unit for `lifeos serve` and `deploy/deploy.sh`. The admin password and the per-device API tokens. Tunnel hostnames `admin.` and `api.` with Cloudflare Access. | Deployment and security |

Phase 2 ends with the gate in the design doc: one week of daily logging through the admin pages. The `wiki_*` tools, `brief`, OAuth for the MCP endpoint, the jobs and the connectors belong to Phases 3 to 5.
