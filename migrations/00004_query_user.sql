-- +goose Up

-- The read-only user of the query tool can read the tables that exist now,
-- and can change none. `make query-user` makes the user; a user is not a
-- part of one database, so this migration cannot make it with its password.
--
-- A later table gets no right from this migration. The migration that adds
-- a table gives the right with its own GRANT, and a table that holds a
-- secret, such as a token, gets none.
GRANT SELECT ON ALL TABLES IN SCHEMA public TO lifeos_query;

-- +goose Down
REVOKE SELECT ON ALL TABLES IN SCHEMA public FROM lifeos_query;
