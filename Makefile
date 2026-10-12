# Reads the database passwords and the Access values from deploy/.env, which
# is not in git.
include deploy/.env
export POSTGRES_PASSWORD
# The password of the read-only user lifeos_query, for the query tool.
export LIFEOS_QUERY_PASSWORD
# The team name and the audience tag of Cloudflare Access, for the token
# check of the MCP server. `make run` needs them.
export CF_ACCESS_TEAM
export LIFEOS_MCP_AUD
# Tests create and drop their own databases on the server postgres-test of
# deploy/docker-compose.yml. Its data is in memory, not on the SD card, and
# it is not the server that holds the real data.
export LIFEOS_TEST_DATABASE_URL = postgres://lifeos:$(POSTGRES_PASSWORD)@127.0.0.1:5433/postgres?sslmode=disable

# Narrow a test run with: make test PKG=./internal/api ARGS="-run TestName -v"
PKG ?= ./...
ARGS ?=

SQLC_VERSION = 1.31.1
SQLC = $(shell go env GOPATH)/bin/sqlc

.PHONY: tools query-user generate build test migrate run

# Installs the code generators that are not Go modules of this repo.
tools:
	curl -fsSL https://github.com/sqlc-dev/sqlc/releases/download/v$(SQLC_VERSION)/sqlc_$(SQLC_VERSION)_linux_arm64.tar.gz | tar -xz -C $(dir $(SQLC)) sqlc

# One time, before the first migration: makes the read-only Postgres user
# lifeos_query. Run it again after a change of LIFEOS_QUERY_PASSWORD.
query-user:
	deploy/query-user.sh

# Generated code is not in git. Each target that compiles runs this first.
generate:
	$(SQLC) generate
	go tool templ generate

# The binary gets its release from the tags: v0.1.0 for a build of that
# tag, v0.1.0-3-gabc1234 for a build three commits later.
VERSION := $(shell git describe --tags --always --dirty)

build: generate
	go build -ldflags "-X github.com/Gautam-J/lifeos-core/internal/version.Version=$(VERSION)" -o bin/lifeos ./cmd/lifeos

test: generate
	deploy/test.sh
	go vet ./...
	go test $(PKG) $(ARGS)

migrate: build
	bin/lifeos migrate

run: build
	bin/lifeos serve
