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
