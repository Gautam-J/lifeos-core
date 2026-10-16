package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

// DownTo undoes the migrations after version. Only tests use it.
func DownTo(ctx context.Context, db *sql.DB, version int64) error {
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files)
	if err != nil {
		return err
	}
	_, err = provider.DownTo(ctx, version)
	return err
}
