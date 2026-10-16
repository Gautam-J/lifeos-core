package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Gautam-J/lifeos-core/internal/store"
)

// Purge removes one record for good. Only the admin pages call it, so the
// change is Gautam's own. The record must be deleted already: Purge
// returns ErrNotFound for a live record and for an ID that no record has.
//
// In one transaction, Purge removes the row, empties the values of the
// earlier events of the record and appends a purge event that has no
// value. The events stay, so the log shows that the record was there.
//
// A journal entry that a record names in its journal_ref cannot go: Purge
// returns a ValidationError that names the table of that record. The file
// of a file source leaves the file store, unless a different source uses
// it.
func (s *Service) Purge(ctx context.Context, recordType string, id int64) error {
	t, err := lookupRecordType(recordType)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("purge: %w", err)
	}
	defer tx.Rollback(ctx)
	q := store.New(tx)

	table := t.table
	if t.link != nil {
		if err := removeLinksOf(ctx, q, t, id); err != nil {
			return err
		}
	}
	file, err := t.purge(ctx, q, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	// 23503 is the code of Postgres for a foreign key that still points
	// to the row.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		message := t.purgeRefused
		if message == "" {
			message = "a record in %s still names this record: delete that record forever first"
		}
		return invalid("id", fmt.Sprintf(message, pgErr.TableName))
	}
	if err != nil {
		return fmt.Errorf("purge from %s: %w", table, err)
	}
	if err := q.EmptyEventValues(ctx, store.EmptyEventValuesParams{RecordType: table, RecordID: &id}); err != nil {
		return fmt.Errorf("purge from %s: %w", table, err)
	}
	if err := recordEvent(ctx, q, "purge", table, id, SourceManual, nil, nil); err != nil {
		return fmt.Errorf("purge from %s: %w", table, err)
	}
	// The file goes before the commit. When the file cannot go, the row
	// stays, and a second purge can do the work. The other order can
	// leave a file that no row names.
	if file != nil {
		others, err := q.CountSourcesOfFile(ctx, file)
		if err != nil {
			return fmt.Errorf("purge from %s: %w", table, err)
		}
		if others == 0 {
			if err := s.removeStoredFile(*file); err != nil {
				return fmt.Errorf("purge from %s: remove the file: %w", table, err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("purge from %s: %w", table, err)
	}
	return nil
}

// removeStoredFile removes one file of the file store. path is the path
// below Files.Dir. A file that is not there is not an error. os.Root
// refuses a path that leaves the file store.
func (s *Service) removeStoredFile(path string) error {
	root, err := os.OpenRoot(s.files.Dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	// The file is gone from the disk only after a sync of its folder. The
	// folder is not there after a restore of the database without the
	// file store: then there is nothing to sync.
	if err := syncDir(filepath.Join(s.files.Dir, filepath.Dir(path))); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
