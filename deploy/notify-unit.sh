#!/bin/bash
# Send the result of a systemd unit to ntfy: quiet on success, urgent with
# the last log lines on failure. Called by lifeos-notify@.service.
# Usage: notify-unit.sh <unit>
set -euo pipefail

unit=$1
result=$(systemctl show "$unit" -p Result --value)
if [ "$result" = success ]; then
    title="$unit succeeded"; priority=low; tags=white_check_mark; lines=5
else
    title="$unit failed ($result)"; priority=high; tags=rotating_light; lines=15
fi

"$(dirname "$0")/notify.sh" lifeos \
    "$(journalctl -u "$unit" -n "$lines" -o cat --no-pager)" \
    "$title" "$priority" "$tags"
