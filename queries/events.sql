-- name: InsertEvent :exec
INSERT INTO events (action, record_type, record_id, source, old_value, new_value)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: EmptyEventValues :exec
-- Takes the content of one record out of the log. The rows stay, with
-- their time and their action.
UPDATE events
SET old_value = NULL, new_value = NULL
WHERE record_type = $1 AND record_id = $2;

-- name: ListEventsSince :many
-- The newest event first. A wiki event has no record ID: its value names
-- the page.
SELECT id, occurred_at, action, record_type, record_id, source,
       (CASE WHEN record_type = 'wiki'
             THEN COALESCE(new_value->>'path', old_value->>'path', '')
             ELSE '' END)::text AS wiki_path
FROM events
WHERE occurred_at >= sqlc.arg('since')::timestamptz
ORDER BY occurred_at DESC, id DESC
LIMIT sqlc.arg('max_rows');
