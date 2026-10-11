#!/bin/bash
# Makes the read-only Postgres user lifeos_query, or sets its password again.
# The query tool connects as this user, so a query cannot change data. Run
# it one time before the first migration, and again after a change of the
# password. `make query-user` runs it.
# Needs LIFEOS_QUERY_PASSWORD in deploy/.env.
set -euo pipefail

cd "$(dirname "$0")"
set -a; . ./.env; set +a
: "${LIFEOS_QUERY_PASSWORD:?set LIFEOS_QUERY_PASSWORD in deploy/.env}"

# psql gets the password from its environment, so the value is in no command
# line. A user is not a part of one database: migrations give it its rights.
docker compose exec -T -e LIFEOS_QUERY_PASSWORD postgres \
    psql -U lifeos -d postgres -v ON_ERROR_STOP=1 -q <<'SQL'
\getenv password LIFEOS_QUERY_PASSWORD
SELECT 'CREATE ROLE lifeos_query LOGIN'
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'lifeos_query') \gexec
ALTER ROLE lifeos_query PASSWORD :'password';
SQL
echo "the Postgres user lifeos_query is ready"
