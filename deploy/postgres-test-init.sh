#!/bin/bash
# Runs inside the container postgres-test each time it starts, because its
# data folder is empty then. It makes the read-only user lifeos_query, which
# the migrations give rights to and which the tests of the query tool use.
# psql gets the password from its environment, so the value is in no command
# line.
set -euo pipefail

psql -v ON_ERROR_STOP=1 -q -U "$POSTGRES_USER" -d postgres <<'SQL'
\getenv password LIFEOS_QUERY_PASSWORD
CREATE ROLE lifeos_query LOGIN PASSWORD :'password';
SQL
