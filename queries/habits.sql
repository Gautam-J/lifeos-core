-- name: InsertHabit :one
INSERT INTO habits (name, source, source_ref)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListHabits :many
SELECT * FROM habits
WHERE deleted_at IS NULL
ORDER BY name;

-- name: GetHabitForUpdate :one
SELECT * FROM habits
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- name: UpdateHabit :one
UPDATE habits
SET name = $2, active = $3
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;
