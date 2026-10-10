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
