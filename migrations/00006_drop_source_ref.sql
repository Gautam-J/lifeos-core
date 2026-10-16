-- +goose Up

-- #39: no record comes from a capture. A source holds what came in; its
-- ideas go into the wiki and cite (src:N).
ALTER TABLE catalog_items   DROP COLUMN source_ref;
ALTER TABLE journal_entries DROP COLUMN source_ref;
ALTER TABLE todos           DROP COLUMN source_ref;
ALTER TABLE habits          DROP COLUMN source_ref;
ALTER TABLE habit_logs      DROP COLUMN source_ref;
ALTER TABLE sources         DROP COLUMN source_ref;

-- +goose Down
ALTER TABLE catalog_items   ADD COLUMN source_ref bigint REFERENCES sources (id);
ALTER TABLE journal_entries ADD COLUMN source_ref bigint REFERENCES sources (id);
ALTER TABLE todos           ADD COLUMN source_ref bigint REFERENCES sources (id);
ALTER TABLE habits          ADD COLUMN source_ref bigint REFERENCES sources (id);
ALTER TABLE habit_logs      ADD COLUMN source_ref bigint REFERENCES sources (id);
ALTER TABLE sources         ADD COLUMN source_ref bigint REFERENCES sources (id);
