#!/bin/bash
# Nightly backup: pg_dump of the lifeos database, the file store and the
# data of Grafana and Prometheus into two
# restic repositories, one on the Pi and one in Backblaze B2.
# Needs RESTIC_PASSWORD and the B2 settings in deploy/.env.
set -euo pipefail

cd "$(dirname "$0")"
set -a; . ./.env; set +a
export RESTIC_REPOSITORY=/var/backups/lifeos/restic

dump_dir=/var/backups/lifeos/dump
# The captured files: files_dir in config/lifeos.yaml.
files_dir=/var/lib/lifeos/files
# The data of Grafana and Prometheus (docker-compose.yml). Prometheus
# deletes no data, so this copy is the only one after a loss of the disk.
# Its blocks do not change after they are written; a copy of the running
# server can cut only the write-ahead log, which Prometheus repairs at the
# start.
monitoring_dirs=(/var/lib/lifeos-monitoring/grafana /var/lib/lifeos-monitoring/prometheus)
# Grafana downloads its plugins again at the start (about 450 MB).
skip_plugins=(--exclude /var/lib/lifeos-monitoring/grafana/plugins)
mkdir -p "$dump_dir"
trap 'rm -f "$dump_dir/lifeos.dump" "$dump_dir/globals.sql"' EXIT

# Dump to files first, so a failed pg_dump never becomes a snapshot.
docker compose exec -T postgres pg_dump -U lifeos -Fc lifeos > "$dump_dir/lifeos.dump"
# Roles and their passwords live outside the database, so pg_dump skips them.
docker compose exec -T postgres pg_dumpall -U lifeos --globals-only > "$dump_dir/globals.sql"

restic backup --tag postgres "$dump_dir" "$files_dir" "${monitoring_dirs[@]}" "${skip_plugins[@]}"
# Second copy off the Pi, through the S3-compatible API of B2.
restic -r "s3:https://$B2_ENDPOINT/$B2_BUCKET/restic" backup --tag postgres "$dump_dir" "$files_dir" "${monitoring_dirs[@]}" "${skip_plugins[@]}"
