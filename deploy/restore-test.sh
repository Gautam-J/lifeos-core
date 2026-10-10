#!/bin/bash
# Restore test: load the latest backup into a scratch database and compare
# row counts with production. Exits non-zero on any mismatch.
set -euo pipefail

cd "$(dirname "$0")"
set -a; . ./.env; set +a
export RESTIC_REPOSITORY=/var/backups/lifeos/restic

scratch=lifeos_restore_test
pg() { docker compose exec -T postgres psql -U lifeos -v ON_ERROR_STOP=1 -At "$@"; }

pg -d postgres -c "DROP DATABASE IF EXISTS $scratch" -c "CREATE DATABASE $scratch" > /dev/null
trap 'pg -d postgres -c "DROP DATABASE IF EXISTS $scratch" > /dev/null' EXIT

restic dump latest /var/backups/lifeos/dump/lifeos.dump |
    docker compose exec -T postgres pg_restore -U lifeos -d "$scratch" --exit-on-error

status=0
printf '%-24s %10s %10s\n' table production restored
for table in $(pg -d lifeos -c "SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY 1"); do
    prod=$(pg -d lifeos -c "SELECT count(*) FROM public.\"$table\"")
    restored=$(pg -d "$scratch" -c "SELECT count(*) FROM public.\"$table\"" 2> /dev/null) || restored=missing
    [ "$prod" = "$restored" ] || status=1
    printf '%-24s %10s %10s\n' "$table" "$prod" "$restored"
done
exit $status
