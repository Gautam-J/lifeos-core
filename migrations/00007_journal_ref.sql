-- +goose Up

-- #39: a record that a journal entry reports names the entry. Purge of the
-- entry checks these columns, so each has an index on its set values.
ALTER TABLE habit_logs    ADD COLUMN journal_ref bigint REFERENCES journal_entries (id);
ALTER TABLE todos         ADD COLUMN journal_ref bigint REFERENCES journal_entries (id);
ALTER TABLE catalog_items ADD COLUMN journal_ref bigint REFERENCES journal_entries (id);

CREATE INDEX habit_logs_journal_ref_idx    ON habit_logs (journal_ref)    WHERE journal_ref IS NOT NULL;
CREATE INDEX todos_journal_ref_idx         ON todos (journal_ref)         WHERE journal_ref IS NOT NULL;
CREATE INDEX catalog_items_journal_ref_idx ON catalog_items (journal_ref) WHERE journal_ref IS NOT NULL;

-- +goose Down
ALTER TABLE catalog_items DROP COLUMN journal_ref;
ALTER TABLE todos         DROP COLUMN journal_ref;
ALTER TABLE habit_logs    DROP COLUMN journal_ref;
