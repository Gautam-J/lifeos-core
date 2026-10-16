-- +goose Up

-- #40: a record that an agent writes is a record at once. Provenance is the
-- source and the two timestamps; the events keep each old value.
ALTER TABLE catalog_items   DROP COLUMN verified;
ALTER TABLE journal_entries DROP COLUMN verified;
ALTER TABLE todos           DROP COLUMN verified;
ALTER TABLE habits          DROP COLUMN verified;
ALTER TABLE habit_logs      DROP COLUMN verified;
ALTER TABLE sources         DROP COLUMN verified;

-- +goose Down
-- Each row that stays was a live record, so it comes back as verified.
-- A new row gets false again, as before.
ALTER TABLE sources         ADD COLUMN verified boolean NOT NULL DEFAULT true;
ALTER TABLE habit_logs      ADD COLUMN verified boolean NOT NULL DEFAULT true;
ALTER TABLE habits          ADD COLUMN verified boolean NOT NULL DEFAULT true;
ALTER TABLE todos           ADD COLUMN verified boolean NOT NULL DEFAULT true;
ALTER TABLE journal_entries ADD COLUMN verified boolean NOT NULL DEFAULT true;
ALTER TABLE catalog_items   ADD COLUMN verified boolean NOT NULL DEFAULT true;
ALTER TABLE sources         ALTER COLUMN verified SET DEFAULT false;
ALTER TABLE habit_logs      ALTER COLUMN verified SET DEFAULT false;
ALTER TABLE habits          ALTER COLUMN verified SET DEFAULT false;
ALTER TABLE todos           ALTER COLUMN verified SET DEFAULT false;
ALTER TABLE journal_entries ALTER COLUMN verified SET DEFAULT false;
ALTER TABLE catalog_items   ALTER COLUMN verified SET DEFAULT false;
