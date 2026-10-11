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
