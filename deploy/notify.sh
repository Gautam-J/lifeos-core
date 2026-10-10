#!/bin/bash
# Send the result of a systemd unit to ntfy: quiet on success, urgent with
# the last log lines on failure. Called by lifeos-notify@.service.
# Needs NTFY_TOKEN in deploy/.env.
# Usage: notify.sh <unit>
set -euo pipefail

cd "$(dirname "$0")"
set -a; . ./.env; set +a

unit=$1
result=$(systemctl show "$unit" -p Result --value)
if [ "$result" = success ]; then
    title="$unit succeeded"; priority=low; tags=white_check_mark; lines=5
else
    title="$unit failed ($result)"; priority=high; tags=rotating_light; lines=15
fi

journalctl -u "$unit" -n "$lines" -o cat --no-pager |
    curl -fsS --max-time 20 -o /dev/null \
        -H "Authorization: Bearer $NTFY_TOKEN" \
        -H "Title: $title" -H "Priority: $priority" -H "Tags: $tags" \
        --data-binary @- http://127.0.0.1:2586/lifeos
