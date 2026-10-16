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

-- name: ListEntitiesOfTheDays :many
-- The live entities that the records of a span of days link to: the
-- journal entries, habit logs and items of the days, the sources stored in
-- the span, and the todos that the caller lists.
SELECT entities.*
FROM entities
WHERE entities.deleted_at IS NULL
  AND entities.archived_at IS NULL
  AND EXISTS (
    SELECT 1 FROM record_entities re
    WHERE re.entity_id = entities.id
      AND (
        re.journal_id IN (SELECT id FROM journal_entries
                          WHERE deleted_at IS NULL
                            AND entry_date >= sqlc.arg('from_day')::date AND entry_date <= sqlc.arg('to_day')::date)
        OR re.habit_log_id IN (SELECT id FROM habit_logs
                               WHERE deleted_at IS NULL
                                 AND logged_on >= sqlc.arg('from_day')::date AND logged_on <= sqlc.arg('to_day')::date)
        OR re.item_id IN (SELECT id FROM catalog_items
                          WHERE deleted_at IS NULL
                            AND occurred_on >= sqlc.arg('from_day')::date AND occurred_on <= sqlc.arg('to_day')::date)
        OR re.source_id IN (SELECT id FROM sources
                            WHERE deleted_at IS NULL
                              AND created_at >= sqlc.arg('span_start')::timestamptz AND created_at < sqlc.arg('span_end')::timestamptz)
        OR re.todo_id = ANY (sqlc.arg('todo_ids')::bigint[])
      ))
ORDER BY entities.kind, lower(entities.name), entities.id;
