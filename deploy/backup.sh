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
# The data of Grafana, Prometheus, Loki and Tempo (docker-compose.yml).
# Prometheus, Loki and Tempo delete no data, so this copy is the only one
# after a loss of the disk. Their blocks do not change after they are
# written; a copy of a running server can cut only the newest data in the
# write-ahead log, which each one repairs at the start.
monitoring_dirs=(/var/lib/lifeos-monitoring/grafana /var/lib/lifeos-monitoring/prometheus
    /var/lib/lifeos-monitoring/loki /var/lib/lifeos-monitoring/tempo)
# Grafana downloads its plugins again at the start (about 450 MB).
skip_plugins=(--exclude /var/lib/lifeos-monitoring/grafana/plugins)
mkdir -p "$dump_dir"
trap 'rm -f "$dump_dir/lifeos.dump" "$dump_dir/canvas.dump" "$dump_dir/globals.sql"' EXIT

# Dump to files first, so a failed pg_dump never becomes a snapshot.
docker compose exec -T postgres pg_dump -U lifeos -Fc lifeos > "$dump_dir/lifeos.dump"
# The database of excalidraw-canvas, in the same container. Until its
# create-db.sh has run, the database is absent: skip it, so that the backup
# of lifeos still runs.
if [ -n "$(docker compose exec -T postgres psql -U lifeos -tAc "SELECT 1 FROM pg_database WHERE datname = 'canvas'")" ]; then
    docker compose exec -T postgres pg_dump -U lifeos -Fc canvas > "$dump_dir/canvas.dump"
else
    echo "canvas: no database, skipped"
fi
# Roles and their passwords live outside the database, so pg_dump skips them.
docker compose exec -T postgres pg_dumpall -U lifeos --globals-only > "$dump_dir/globals.sql"

restic backup --tag postgres "$dump_dir" "$files_dir" "${monitoring_dirs[@]}" "${skip_plugins[@]}"
# Second copy off the Pi, through the S3-compatible API of B2.
restic -r "s3:https://$B2_ENDPOINT/$B2_BUCKET/restic" backup --tag postgres "$dump_dir" "$files_dir" "${monitoring_dirs[@]}" "${skip_plugins[@]}"
