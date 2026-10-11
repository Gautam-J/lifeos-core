#!/bin/bash
# Ships the code of this repository on the Pi: pull, build, dump the
# database, apply the migrations, start the service again, then check that
# Cloudflare Access guards the public hostnames.
# Run it on the Pi as the user of the service: deploy/deploy.sh
set -euo pipefail

cd "$(dirname "$0")/.."
set -a; . deploy/.env; set +a

git pull --ff-only
# The service runs the old binary until the restart.
make build

# A migration can drop or rename a column, so each deploy dumps the
# database before it applies one. The last five dumps stay.
dump_dir=/var/backups/lifeos/deploy
mkdir -p "$dump_dir"
dump="$dump_dir/before-$(date +%Y%m%d-%H%M%S).dump"
docker compose -f deploy/docker-compose.yml exec -T postgres pg_dump -U lifeos -Fc lifeos > "$dump.part"
mv "$dump.part" "$dump"
ls -1t "$dump_dir"/before-*.dump | tail -n +6 | xargs -r rm --

bin/lifeos migrate
# The unit file is a link into this folder: a pull can change it.
sudo systemctl daemon-reload
sudo systemctl restart lifeos.service

# The server listens after it has a connection to Postgres.
addr=$(sed -n 's/^http_addr: *//p' config/lifeos.yaml)
for _ in $(seq 30); do
    if curl -fs -o /dev/null "http://$addr/admin/items"; then
        echo "lifeos answers on $addr"
        deploy/check-access.sh
        exit 0
    fi
    sleep 1
done
echo "lifeos does not answer on $addr: see journalctl -u lifeos" >&2
exit 1
