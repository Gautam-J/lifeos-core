-- The citation checks of the wiki. A wiki page cites a row as (prefix:id).
-- Each query says whether a live row has the ID.

-- name: LiveSourceExists :one
SELECT EXISTS (SELECT 1 FROM sources WHERE id = $1 AND deleted_at IS NULL);

-- name: LiveJournalEntryExists :one
SELECT EXISTS (SELECT 1 FROM journal_entries WHERE id = $1 AND deleted_at IS NULL);

-- name: LiveItemOfKindExists :one
SELECT EXISTS (SELECT 1 FROM catalog_items WHERE id = $1 AND kind = $2 AND deleted_at IS NULL);

-- name: LiveTodoExists :one
SELECT EXISTS (SELECT 1 FROM todos WHERE id = $1 AND deleted_at IS NULL);

-- name: LiveHabitLogExists :one
SELECT EXISTS (SELECT 1 FROM habit_logs WHERE id = $1 AND deleted_at IS NULL);
