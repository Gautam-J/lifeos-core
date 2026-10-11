-- Each search query gets the words of the search as patterns: one word
-- between % signs for each pattern. A row matches when its text matches
-- every pattern. ILIKE does not look at upper and lower case.

-- name: SearchCatalogItems :many
SELECT * FROM catalog_items
WHERE deleted_at IS NULL
  AND (kind || ' ' || title || ' ' || attrs::text) ILIKE ALL (sqlc.arg(patterns)::text[])
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(max_rows);

-- name: SearchJournalEntries :many
SELECT * FROM journal_entries
WHERE deleted_at IS NULL
  AND (body || ' ' || entry_date::text) ILIKE ALL (sqlc.arg(patterns)::text[])
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(max_rows);

-- name: SearchTodos :many
SELECT * FROM todos
WHERE deleted_at IS NULL
  AND (title || ' ' || coalesce(project, '')) ILIKE ALL (sqlc.arg(patterns)::text[])
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(max_rows);

-- name: SearchSources :many
SELECT * FROM sources
WHERE deleted_at IS NULL
  AND (coalesce(title, '') || ' ' || coalesce(content, '') || ' ' || coalesce(url, '')) ILIKE ALL (sqlc.arg(patterns)::text[])
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(max_rows);
