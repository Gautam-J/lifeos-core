#!/bin/bash
# Send a message to an ntfy topic. Title, priority and tags default by topic;
# the optional arguments override them. Installed as /usr/local/bin/notify.
# Needs NTFY_TOKEN in deploy/.env.
# Usage: notify.sh <topic> <message> [title] [priority] [tags]
set -euo pipefail

cd "$(dirname "$(readlink -f "$0")")"
. ./.env

topic=$1
case "$topic" in
    pi-alerts) title="Pi alert"; priority=high; tags=warning ;;
    pi-health) title="Pi health check"; priority=low; tags=white_check_mark ;;
    *) title="$topic"; priority=default; tags= ;;
esac

printf '%s' "$2" |
    curl -fsS --max-time 20 -o /dev/null \
        -H "Authorization: Bearer $NTFY_TOKEN" \
        -H "Title: ${3:-$title}" -H "Priority: ${4:-$priority}" -H "Tags: ${5:-$tags}" \
        --data-binary @- "http://127.0.0.1:2586/$topic"
