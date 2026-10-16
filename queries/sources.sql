-- name: InsertSource :one
-- Returns no row when a live source has this kind and content already.
INSERT INTO sources (kind, title, content, url, file_path, media_type, size_bytes, sha256, source)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
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

-- name: PurgeSource :one
-- Removes a row for good. Only a row that is deleted already can go.
DELETE FROM sources
WHERE id = $1 AND deleted_at IS NOT NULL
RETURNING *;

-- name: CountSourcesOfFile :one
-- Counts the rows, live and deleted, that use one file of the file store.
SELECT count(*) FROM sources
WHERE file_path = $1;

-- name: ListDeletedSources :many
SELECT * FROM sources
WHERE deleted_at IS NOT NULL
ORDER BY deleted_at DESC, id DESC
LIMIT $1;

-- name: GetSourceForUpdate :one
SELECT * FROM sources
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- name: MarkSourceProcessed :one
-- Sets the time at which the source was read. It changes no other column.
UPDATE sources
SET processed_at = now()
WHERE id = $1
RETURNING *;
