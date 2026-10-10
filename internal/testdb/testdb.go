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
