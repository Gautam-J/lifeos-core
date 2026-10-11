#!/bin/bash
# Nightly backup: pg_dump of the lifeos database and the file store into two
# restic repositories, one on the Pi and one in Backblaze B2.
# Needs RESTIC_PASSWORD and the B2 settings in deploy/.env.
set -euo pipefail

cd "$(dirname "$0")"
set -a; . ./.env; set +a
export RESTIC_REPOSITORY=/var/backups/lifeos/restic

dump_dir=/var/backups/lifeos/dump
# The captured files: files_dir in config/lifeos.yaml.
files_dir=/var/lib/lifeos/files
mkdir -p "$dump_dir"
trap 'rm -f "$dump_dir/lifeos.dump" "$dump_dir/globals.sql"' EXIT

# Dump to files first, so a failed pg_dump never becomes a snapshot.
docker compose exec -T postgres pg_dump -U lifeos -Fc lifeos > "$dump_dir/lifeos.dump"
# Roles and their passwords live outside the database, so pg_dump skips them.
docker compose exec -T postgres pg_dumpall -U lifeos --globals-only > "$dump_dir/globals.sql"

restic backup --tag postgres "$dump_dir" "$files_dir"
# Second copy off the Pi, through the S3-compatible API of B2.
restic -r "s3:https://$B2_ENDPOINT/$B2_BUCKET/restic" backup --tag postgres "$dump_dir" "$files_dir"
