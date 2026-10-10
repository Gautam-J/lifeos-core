#!/bin/bash
# Nightly backup: pg_dump of the lifeos database into the local restic repository.
# Needs RESTIC_PASSWORD in deploy/.env.
set -euo pipefail

cd "$(dirname "$0")"
set -a; . ./.env; set +a
export RESTIC_REPOSITORY=/var/backups/lifeos/restic

dump_dir=/var/backups/lifeos/dump
mkdir -p "$dump_dir"
trap 'rm -f "$dump_dir/lifeos.dump" "$dump_dir/globals.sql"' EXIT

# Dump to files first, so a failed pg_dump never becomes a snapshot.
docker compose exec -T postgres pg_dump -U lifeos -Fc lifeos > "$dump_dir/lifeos.dump"
# Roles and their passwords live outside the database, so pg_dump skips them.
docker compose exec -T postgres pg_dumpall -U lifeos --globals-only > "$dump_dir/globals.sql"

restic backup --tag postgres "$dump_dir"
