-- name: CountDay :one
-- What one IST day holds. day_start and day_end are the two ends of the day.
SELECT
    (SELECT count(*) FROM journal_entries
      WHERE journal_entries.deleted_at IS NULL
        AND journal_entries.entry_date = sqlc.arg('day')::date) AS journal_entries,
    (SELECT count(*) FROM todos
      WHERE todos.deleted_at IS NULL AND todos.status = 'done'
        AND todos.done_at >= sqlc.arg('day_start')::timestamptz
        AND todos.done_at < sqlc.arg('day_end')::timestamptz) AS todos_done,
    (SELECT count(*) FROM catalog_items
      WHERE catalog_items.deleted_at IS NULL
        AND catalog_items.created_at >= sqlc.arg('day_start')::timestamptz
        AND catalog_items.created_at < sqlc.arg('day_end')::timestamptz) AS items_added,
    (SELECT count(*) FROM sources
      WHERE sources.deleted_at IS NULL
        AND sources.created_at >= sqlc.arg('day_start')::timestamptz
        AND sources.created_at < sqlc.arg('day_end')::timestamptz) AS captures;
