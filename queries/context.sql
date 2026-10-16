-- name: ListJournalEntriesBetween :many
-- The entries of two days in the order that they were written.
SELECT * FROM journal_entries
WHERE deleted_at IS NULL
  AND entry_date >= sqlc.arg('from_day')::date
  AND entry_date <= sqlc.arg('to_day')::date
ORDER BY entry_date, created_at, id;

-- name: ListOpenTodosDueBy :many
-- The open todos due on the day or before it, the oldest due date first.
SELECT * FROM todos
WHERE deleted_at IS NULL AND status = 'open' AND due_on <= sqlc.arg('day')::date
ORDER BY due_on, id;

-- name: ListOpenTodosDueOn :many
SELECT * FROM todos
WHERE deleted_at IS NULL AND status = 'open' AND due_on = sqlc.arg('day')::date
ORDER BY id;

-- name: ListTodosDoneBetween :many
-- The todos that became done in a span of time, the oldest first.
SELECT * FROM todos
WHERE deleted_at IS NULL AND status = 'done'
  AND done_at >= sqlc.arg('span_start')::timestamptz
  AND done_at < sqlc.arg('span_end')::timestamptz
ORDER BY done_at, id;

-- name: CountSourcesNotProcessed :one
SELECT count(*) FROM sources
WHERE deleted_at IS NULL AND processed_at IS NULL;
