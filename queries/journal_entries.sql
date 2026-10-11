-- name: InsertJournalEntry :one
INSERT INTO journal_entries (body, entry_date, source, source_ref, verified)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetJournalEntry :one
SELECT * FROM journal_entries
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListJournalEntries :many
SELECT * FROM journal_entries
WHERE deleted_at IS NULL
ORDER BY entry_date DESC, id DESC
LIMIT $1;

-- name: GetJournalEntryForUpdate :one
SELECT * FROM journal_entries
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- name: UpdateJournalEntry :one
UPDATE journal_entries
SET body = $2, entry_date = $3
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteJournalEntry :one
UPDATE journal_entries
SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: ListUnverifiedJournalEntries :many
SELECT * FROM journal_entries
WHERE deleted_at IS NULL AND NOT verified
ORDER BY created_at, id
LIMIT $1;

-- name: VerifyJournalEntry :one
UPDATE journal_entries
SET verified = true
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: PurgeJournalEntry :one
-- Removes a row for good. Only a row that is deleted already can go.
DELETE FROM journal_entries
WHERE id = $1 AND deleted_at IS NOT NULL
RETURNING id;

-- name: ListDeletedJournalEntries :many
SELECT * FROM journal_entries
WHERE deleted_at IS NOT NULL
ORDER BY deleted_at DESC, id DESC
LIMIT $1;
