-- name: InsertEvent :exec
INSERT INTO events (action, record_type, record_id, source, old_value, new_value)
VALUES ($1, $2, $3, $4, $5, $6);
