-- name: InsertEntity :one
INSERT INTO entities (kind, name, aliases, wiki_path, source)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetEntity :one
SELECT * FROM entities
WHERE id = $1 AND deleted_at IS NULL;

-- name: FindEntitiesByName :many
-- The live entities that have this name or this alias, in any case. kind is
-- optional: without it the search covers every kind.
SELECT * FROM entities
WHERE deleted_at IS NULL
  AND (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind'))
  AND (lower(name) = lower(sqlc.arg('name'))
       OR lower(sqlc.arg('name')) IN (SELECT lower(a) FROM unnest(aliases) AS a))
ORDER BY id;

-- name: ListEntityNames :many
-- The names of entities, also of a deleted one: a todo keeps showing the
-- name of its project.
SELECT id, name FROM entities
WHERE id = ANY (sqlc.arg('ids')::bigint[]);

-- name: ListEntities :many
-- The live entities. kind is optional. An archived entity is left out
-- unless include_archived is true.
SELECT * FROM entities
WHERE deleted_at IS NULL
  AND (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind'))
  AND (sqlc.arg('include_archived')::boolean OR archived_at IS NULL)
ORDER BY kind, lower(name), id
LIMIT sqlc.arg('max_rows');

-- name: GetEntityForUpdate :one
SELECT * FROM entities
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- name: UpdateEntity :one
UPDATE entities
SET name = $2, aliases = $3, wiki_path = $4, archived_at = $5
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteEntity :one
UPDATE entities
SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: PurgeEntity :one
-- Removes a row for good. Only a row that is deleted already can go.
DELETE FROM entities
WHERE id = $1 AND deleted_at IS NOT NULL
RETURNING id;

-- name: ListDeletedEntities :many
SELECT * FROM entities
WHERE deleted_at IS NOT NULL
ORDER BY deleted_at DESC, id DESC
LIMIT $1;

-- name: LiveEntityExists :one
SELECT EXISTS (SELECT 1 FROM entities WHERE id = $1 AND deleted_at IS NULL);

-- name: SearchEntities :many
SELECT * FROM entities
WHERE deleted_at IS NULL
  AND (name || ' ' || array_to_string(aliases, ' ')) ILIKE ALL (sqlc.arg(patterns)::text[])
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(max_rows);

-- name: InsertRecordEntity :one
-- A link that exists already is not a write: the query returns no row.
INSERT INTO record_entities (entity_id, journal_id, todo_id, item_id, habit_log_id, source_id, source)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: DeleteRecordEntity :one
DELETE FROM record_entities
WHERE entity_id = sqlc.arg('entity_id')
  AND journal_id   IS NOT DISTINCT FROM sqlc.narg('journal_id')::bigint
  AND todo_id      IS NOT DISTINCT FROM sqlc.narg('todo_id')::bigint
  AND item_id      IS NOT DISTINCT FROM sqlc.narg('item_id')::bigint
  AND habit_log_id IS NOT DISTINCT FROM sqlc.narg('habit_log_id')::bigint
  AND source_id    IS NOT DISTINCT FROM sqlc.narg('source_id')::bigint
RETURNING *;

-- name: DeleteLinksOfRecord :many
-- Purge of a record removes its links.
DELETE FROM record_entities
WHERE journal_id   IS NOT DISTINCT FROM sqlc.narg('journal_id')::bigint
  AND todo_id      IS NOT DISTINCT FROM sqlc.narg('todo_id')::bigint
  AND item_id      IS NOT DISTINCT FROM sqlc.narg('item_id')::bigint
  AND habit_log_id IS NOT DISTINCT FROM sqlc.narg('habit_log_id')::bigint
  AND source_id    IS NOT DISTINCT FROM sqlc.narg('source_id')::bigint
RETURNING *;

-- name: ListEntitiesOfRecord :many
-- The live entities that one record links to, in the order of the links.
SELECT sqlc.embed(entities), record_entities.id AS link_id
FROM record_entities
JOIN entities ON entities.id = record_entities.entity_id
WHERE entities.deleted_at IS NULL
  AND record_entities.journal_id   IS NOT DISTINCT FROM sqlc.narg('journal_id')::bigint
  AND record_entities.todo_id      IS NOT DISTINCT FROM sqlc.narg('todo_id')::bigint
  AND record_entities.item_id      IS NOT DISTINCT FROM sqlc.narg('item_id')::bigint
  AND record_entities.habit_log_id IS NOT DISTINCT FROM sqlc.narg('habit_log_id')::bigint
  AND record_entities.source_id    IS NOT DISTINCT FROM sqlc.narg('source_id')::bigint
ORDER BY record_entities.id;

-- name: ListLinksOfEntity :many
-- The links of one entity, the newest first.
SELECT * FROM record_entities
WHERE entity_id = $1
ORDER BY id DESC;

-- name: CountCoLinks :one
-- How many records link to the entity and to at least one of the others.
SELECT count(DISTINCT a.id) FROM record_entities a
JOIN record_entities b ON b.entity_id = ANY (sqlc.arg('others')::bigint[])
  AND (a.journal_id = b.journal_id OR a.todo_id = b.todo_id OR a.item_id = b.item_id
       OR a.habit_log_id = b.habit_log_id OR a.source_id = b.source_id)
WHERE a.entity_id = sqlc.arg('entity_id');

-- name: DeleteLinksAlreadyOnEntity :many
-- A merge drops the links of one entity to a record that the other entity
-- links to already.
DELETE FROM record_entities d
WHERE d.entity_id = sqlc.arg('drop_id')
  AND EXISTS (
    SELECT 1 FROM record_entities k
    WHERE k.entity_id = sqlc.arg('keep_id')
      AND (k.journal_id = d.journal_id OR k.todo_id = d.todo_id OR k.item_id = d.item_id
           OR k.habit_log_id = d.habit_log_id OR k.source_id = d.source_id))
RETURNING d.*;

-- name: MoveLinks :many
UPDATE record_entities
SET entity_id = sqlc.arg('keep_id')
WHERE entity_id = sqlc.arg('drop_id')
RETURNING *;

-- name: ListTodosOfProject :many
SELECT * FROM todos
WHERE project_id = $1
ORDER BY id
FOR UPDATE;

-- name: MoveTodosOfProject :exec
UPDATE todos SET project_id = sqlc.arg('keep_id')
WHERE project_id = sqlc.arg('drop_id');
