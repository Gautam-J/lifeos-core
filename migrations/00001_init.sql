-- +goose Up

-- Keeps the updated_at provenance column current on every record table.
-- +goose StatementBegin
CREATE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Append-only log of every write. The service layer inserts one row per
-- write; rows are never updated or deleted.
CREATE TABLE events (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    action      text NOT NULL,       -- create, update, delete, verify
    record_type text NOT NULL,       -- table name, or 'wiki' for a wiki page
    record_id   bigint,              -- null when the write has no row (wiki pages)
    source      text NOT NULL CHECK (source IN ('manual', 'import', 'connector', 'agent')),
    old_value   jsonb,               -- the record before an update or delete
    new_value   jsonb
);

CREATE INDEX events_record_idx ON events (record_type, record_id);

-- Lists: movies, books, places, games.
CREATE TABLE catalog_items (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind        text NOT NULL,       -- movie, book, place, game
    title       text NOT NULL,
    rating      numeric(2, 1) CHECK (rating BETWEEN 0 AND 5),
    occurred_on date,                -- day watched, read, visited or played
    attrs       jsonb NOT NULL DEFAULT '{}',

    -- Provenance: every record table carries these same columns.
    source      text NOT NULL CHECK (source IN ('manual', 'import', 'connector', 'agent')),
    source_ref  bigint,              -- sources.id; the foreign key arrives with the sources table
    verified    boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz          -- soft delete
);

CREATE TRIGGER catalog_items_set_updated_at
    BEFORE UPDATE ON catalog_items
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE catalog_items;
DROP TABLE events;
DROP FUNCTION set_updated_at();
