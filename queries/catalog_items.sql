-- name: InsertCatalogItem :one
INSERT INTO catalog_items (kind, title, rating, occurred_on, attrs, source, journal_ref)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetCatalogItem :one
SELECT * FROM catalog_items
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListCatalogItems :many
SELECT * FROM catalog_items
WHERE deleted_at IS NULL
  AND (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind'))
ORDER BY id DESC
LIMIT sqlc.arg('max_rows');

-- name: GetCatalogItemForUpdate :one
SELECT * FROM catalog_items
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- name: UpdateCatalogItem :one
UPDATE catalog_items
SET kind = $2, title = $3, rating = $4, occurred_on = $5, attrs = $6
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteCatalogItem :one
UPDATE catalog_items
SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: PurgeCatalogItem :one
-- Removes a row for good. Only a row that is deleted already can go.
DELETE FROM catalog_items
WHERE id = $1 AND deleted_at IS NOT NULL
RETURNING id;

-- name: ListDeletedCatalogItems :many
SELECT * FROM catalog_items
WHERE deleted_at IS NOT NULL
ORDER BY deleted_at DESC, id DESC
LIMIT $1;
