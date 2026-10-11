-- name: InsertTodo :one
INSERT INTO todos (title, due_on, project, source, source_ref, verified)
VALUES ($1, $2, $3, $4, $5, $6)
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
SET title = $2, status = $3, due_on = $4, project = $5, done_at = $6
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteTodo :one
UPDATE todos
SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: ListUnverifiedTodos :many
SELECT * FROM todos
WHERE deleted_at IS NULL AND NOT verified
ORDER BY created_at, id
LIMIT $1;

-- name: VerifyTodo :one
UPDATE todos
SET verified = true
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: PurgeTodo :one
-- Removes a row for good. Only a row that is deleted already can go.
DELETE FROM todos
WHERE id = $1 AND deleted_at IS NOT NULL
RETURNING id;
