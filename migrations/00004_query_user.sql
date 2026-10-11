-- +goose Up

-- The read-only user of the query tool can read every table and change
-- none. `make query-user` makes the user; a user is not a part of one
-- database, so this migration cannot make it with its password.
GRANT SELECT ON ALL TABLES IN SCHEMA public TO lifeos_query;

-- A table that a later migration makes gets the same right.
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO lifeos_query;

-- +goose Down
ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE SELECT ON TABLES FROM lifeos_query;
REVOKE SELECT ON ALL TABLES IN SCHEMA public FROM lifeos_query;
