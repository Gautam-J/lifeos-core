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
