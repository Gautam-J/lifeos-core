-- name: InsertSource :one
-- Returns no row when a live source has this kind and content already.
INSERT INTO sources (kind, title, content, url, file_path, media_type, size_bytes, sha256, source, source_ref, verified)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (kind, sha256) WHERE deleted_at IS NULL DO NOTHING
RETURNING *;

-- name: GetSourceByHash :one
SELECT * FROM sources
WHERE kind = $1 AND sha256 = $2 AND deleted_at IS NULL;

-- name: GetSource :one
SELECT * FROM sources
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListSources :many
SELECT * FROM sources
WHERE deleted_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT $1;

-- name: SoftDeleteSource :one
UPDATE sources
SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;
