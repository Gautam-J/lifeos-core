-- name: InsertCatalogItem :one
INSERT INTO catalog_items (kind, title, rating, occurred_on, attrs, source, source_ref, verified)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
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
