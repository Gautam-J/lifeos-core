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
