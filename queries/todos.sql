-- name: InsertTodo :one
INSERT INTO todos (title, due_on, project, source, journal_ref)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetTodo :one
SELECT * FROM todos
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListTodos :many
SELECT * FROM todos
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY (status = 'open') DESC, id DESC
LIMIT sqlc.arg('max_rows');

-- name: GetTodoForUpdate :one
SELECT * FROM todos
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- name: UpdateTodo :one
UPDATE todos
SET title = $2, status = $3, due_on = $4, project = $5, done_at = $6, journal_ref = $7
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteTodo :one
UPDATE todos
SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: PurgeTodo :one
-- Removes a row for good. Only a row that is deleted already can go.
DELETE FROM todos
WHERE id = $1 AND deleted_at IS NOT NULL
RETURNING id;

-- name: ListDeletedTodos :many
SELECT * FROM todos
WHERE deleted_at IS NOT NULL
ORDER BY deleted_at DESC, id DESC
LIMIT $1;

-- name: CountOpenTodos :one
-- The open todos, and how many of them are due on the day or before it.
SELECT count(*) AS open,
       count(*) FILTER (WHERE due_on <= sqlc.arg('day')::date) AS due
FROM todos
WHERE deleted_at IS NULL AND status = 'open';

-- name: ListNextTodos :many
-- The open todos in the order of the work: the oldest due date first, then
-- the todos that have no date, the newest first.
SELECT * FROM todos
WHERE deleted_at IS NULL AND status = 'open'
ORDER BY due_on NULLS LAST, id DESC
LIMIT sqlc.arg('max_rows');
