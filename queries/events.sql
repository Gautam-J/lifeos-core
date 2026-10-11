-- name: InsertEvent :exec
INSERT INTO events (action, record_type, record_id, source, old_value, new_value)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: EmptyEventValues :exec
-- Takes the content of one record out of the log. The rows stay, with
-- their time and their action.
UPDATE events
SET old_value = NULL, new_value = NULL
WHERE record_type = $1 AND record_id = $2;
