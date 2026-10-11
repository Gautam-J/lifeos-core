-- name: CanChangeATable :one
-- Reports whether the user of the connection can change a row of a table
-- in the schema public. The pool of the query tool must give false.
SELECT EXISTS (
    SELECT 1 FROM pg_catalog.pg_tables
    WHERE schemaname = 'public'
      AND has_table_privilege(current_user, format('%I.%I', schemaname, tablename), 'INSERT, UPDATE, DELETE, TRUNCATE')
)::boolean AS can_change;
