-- +goose Up

-- Verbatim captures: a text, a link or a file. The service never changes a
-- capture. A record that an agent takes from a capture names the row in
-- its source_ref.
CREATE TABLE sources (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind         text NOT NULL CHECK (kind IN ('text', 'link', 'file')),
    title        text,                -- optional label; the file name of a file
    content      text,                -- text: the text exactly as it came
    url          text,                -- link: the URL as it came
    file_path    text,                -- file: the path below the file store, made from the SHA-256
    media_type   text,                -- file: the type that the service found in the bytes
    size_bytes   bigint,              -- file: the number of bytes
    sha256       text NOT NULL,       -- hex SHA-256 of the text, the URL or the file bytes
    processed_at timestamptz,         -- null until the inbox job reads the capture

    source       text NOT NULL CHECK (source IN ('manual', 'import', 'connector', 'agent')),
    source_ref   bigint,
    verified     boolean NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz,

    -- A later plan can fill content for a link or a file, so only a text
    -- must have it.
    CONSTRAINT sources_kind_fields CHECK (
        (kind = 'text' AND content IS NOT NULL AND url IS NULL AND file_path IS NULL) OR
        (kind = 'link' AND url IS NOT NULL AND file_path IS NULL) OR
        (kind = 'file' AND file_path IS NOT NULL AND url IS NULL)
    )
);

CREATE TRIGGER sources_set_updated_at
    BEFORE UPDATE ON sources
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- One live capture for each kind and content. The same thing captured two
-- times gives the row that exists.
CREATE UNIQUE INDEX sources_kind_sha256_idx ON sources (kind, sha256) WHERE deleted_at IS NULL;

-- source_ref was a plain number until now, because sources did not exist.
ALTER TABLE catalog_items   ADD CONSTRAINT catalog_items_source_ref_fkey   FOREIGN KEY (source_ref) REFERENCES sources (id);
ALTER TABLE journal_entries ADD CONSTRAINT journal_entries_source_ref_fkey FOREIGN KEY (source_ref) REFERENCES sources (id);
ALTER TABLE todos           ADD CONSTRAINT todos_source_ref_fkey           FOREIGN KEY (source_ref) REFERENCES sources (id);
ALTER TABLE habits          ADD CONSTRAINT habits_source_ref_fkey          FOREIGN KEY (source_ref) REFERENCES sources (id);
ALTER TABLE habit_logs      ADD CONSTRAINT habit_logs_source_ref_fkey      FOREIGN KEY (source_ref) REFERENCES sources (id);
ALTER TABLE sources         ADD CONSTRAINT sources_source_ref_fkey         FOREIGN KEY (source_ref) REFERENCES sources (id);

-- +goose Down
ALTER TABLE habit_logs      DROP CONSTRAINT habit_logs_source_ref_fkey;
ALTER TABLE habits          DROP CONSTRAINT habits_source_ref_fkey;
ALTER TABLE todos           DROP CONSTRAINT todos_source_ref_fkey;
ALTER TABLE journal_entries DROP CONSTRAINT journal_entries_source_ref_fkey;
ALTER TABLE catalog_items   DROP CONSTRAINT catalog_items_source_ref_fkey;
DROP TABLE sources;
