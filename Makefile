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
