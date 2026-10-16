-- name: GetActiveHabitByName :one
SELECT * FROM habits
WHERE name = $1 AND active AND deleted_at IS NULL;

-- name: InsertHabitLog :one
-- Returns no row when the habit has a live log for the day already.
INSERT INTO habit_logs (habit_id, logged_on, source, source_ref, verified)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (habit_id, logged_on) WHERE deleted_at IS NULL DO NOTHING
RETURNING *;

-- name: GetHabitLogOfDayForUpdate :one
SELECT * FROM habit_logs
WHERE habit_id = $1 AND logged_on = $2 AND deleted_at IS NULL
FOR UPDATE;

-- name: VerifyHabitLog :one
UPDATE habit_logs
SET verified = true
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: GetHabitLog :one
SELECT sqlc.embed(habit_logs), habits.name AS habit
FROM habit_logs
JOIN habits ON habits.id = habit_logs.habit_id
WHERE habit_logs.id = $1 AND habit_logs.deleted_at IS NULL;

-- name: ListHabitLogs :many
SELECT sqlc.embed(habit_logs), habits.name AS habit
FROM habit_logs
JOIN habits ON habits.id = habit_logs.habit_id
WHERE habit_logs.deleted_at IS NULL
ORDER BY habit_logs.logged_on DESC, habit_logs.id DESC
LIMIT $1;

-- name: ListHabitsOnDay :many
-- The active habits, each with "done": the habit has a verified log on the day.
SELECT habits.id, habits.name,
       EXISTS (
           SELECT 1 FROM habit_logs
           WHERE habit_logs.habit_id = habits.id
             AND habit_logs.logged_on = $1
             AND habit_logs.verified
             AND habit_logs.deleted_at IS NULL
       ) AS done
FROM habits
WHERE habits.active AND habits.deleted_at IS NULL
ORDER BY habits.name;

-- name: GetHabitLogForUpdate :one
SELECT sqlc.embed(habit_logs), habits.name AS habit
FROM habit_logs
JOIN habits ON habits.id = habit_logs.habit_id
WHERE habit_logs.id = $1 AND habit_logs.deleted_at IS NULL
FOR UPDATE OF habit_logs;

-- name: SoftDeleteHabitLog :one
UPDATE habit_logs
SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: PurgeHabitLog :one
-- Removes a row for good. Only a row that is deleted already can go.
DELETE FROM habit_logs
WHERE id = $1 AND deleted_at IS NOT NULL
RETURNING id;

-- name: ListDeletedHabitLogs :many
SELECT sqlc.embed(habit_logs), habits.name AS habit
FROM habit_logs
JOIN habits ON habits.id = habit_logs.habit_id
WHERE habit_logs.deleted_at IS NOT NULL
ORDER BY habit_logs.deleted_at DESC, habit_logs.id DESC
LIMIT $1;
