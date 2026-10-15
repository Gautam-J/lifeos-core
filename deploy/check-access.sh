#!/bin/bash
# Checks that Cloudflare Access guards each public hostname of LifeOS.
# LifeOS has no login of its own: a hostname that answers a request that
# has no login is open to the internet. Run this after each change of the
# tunnel or of an Access application; deploy.sh runs it after each deploy.
#
# Needs LIFEOS_ADMIN_HOST, LIFEOS_API_HOST, LIFEOS_MCP_HOST and
# LIFEOS_DASH_HOST in deploy/.env. With CF_ACCESS_CLIENT_ID and
# CF_ACCESS_CLIENT_SECRET (an Access service token), it also checks that
# the API and the MCP server answer a request that has the token, and that
# the API hostname gives no admin page.
# Usage: check-access.sh [env-file]   (default: deploy/.env)
set -euo pipefail

env_file=$(realpath "${1:-$(dirname "$0")/.env}")
set -a; . "$env_file"; set +a

if [ -z "${LIFEOS_ADMIN_HOST:-}" ] && [ -z "${LIFEOS_API_HOST:-}" ] && [ -z "${LIFEOS_MCP_HOST:-}" ] && [ -z "${LIFEOS_DASH_HOST:-}" ]; then
    echo "no public hostname is set in $env_file: nothing to check"
    exit 0
fi

failed=0

# guarded <kind> <url>: a request that has no login must stop at Cloudflare
# Access. Only an answer that Access alone gives is proof:
#   login  the application asks a person to log in: 302 to the login page
#          of Access.
#   token  the application takes a service token: 401, or the same 302.
# A 403 is no proof. A firewall rule of Cloudflare can give it to curl and
# let a browser through.
guarded() {
    local kind=$1 url=$2 code target
    read -r code target < <(curl -s -o /dev/null --max-time 20 -w '%{http_code} %{redirect_url}\n' "$url" || true)
    if [ "$code" = 302 ] && [[ $target =~ ^https://[a-z0-9-]+\.cloudflareaccess\.com/ ]]; then
        echo "ok    $url: Access asks for a login"
        return
    fi
    if [ "$code" = 401 ] && [ "$kind" = token ]; then
        echo "ok    $url: Access refuses the request (401)"
        return
    fi
    failed=1
    if [ "$code" = 000 ]; then
        echo "FAIL  $url: no answer"
        return
    fi
    echo "OPEN  $url: status $code for a request that has no login"
}

# with_token <url> <status> [curl option...]: a request that has the
# service token must get this status from LifeOS.
with_token() {
    local url=$1 want=$2 code
    shift 2
    code=$(curl -s -o /dev/null --max-time 20 -w '%{http_code}' \
        -H "CF-Access-Client-Id: $CF_ACCESS_CLIENT_ID" \
        -H "CF-Access-Client-Secret: $CF_ACCESS_CLIENT_SECRET" "$@" "$url" || true)
    if [ "$code" = "$want" ]; then
        echo "ok    $url: status $code with the service token"
    else
        echo "FAIL  $url: status $code with the service token, want $want"
        failed=1
    fi
}

if [ -n "${LIFEOS_ADMIN_HOST:-}" ]; then
    guarded login "https://$LIFEOS_ADMIN_HOST/admin/items"
    # Access guards each path of the hostname, not only the admin pages.
    guarded login "https://$LIFEOS_ADMIN_HOST/api/items"
fi
if [ -n "${LIFEOS_API_HOST:-}" ]; then
    guarded token "https://$LIFEOS_API_HOST/api/items"
    if [ -n "${CF_ACCESS_CLIENT_ID:-}" ] && [ -n "${CF_ACCESS_CLIENT_SECRET:-}" ]; then
        with_token "https://$LIFEOS_API_HOST/api/items" 200
        # The API hostname gives no admin page.
        with_token "https://$LIFEOS_API_HOST/admin/items" 404
    fi
fi
if [ -n "${LIFEOS_MCP_HOST:-}" ]; then
    # The MCP server checks the token of Access itself and answers 403 to a
    # request that has no valid token. A 403 here is thus no proof, as for
    # each hostname: it shows a request that came past Access.
    guarded token "https://$LIFEOS_MCP_HOST/mcp"
    # The tunnel sends only the path /mcp to the server. Access guards each
    # path of the hostname.
    guarded token "https://$LIFEOS_MCP_HOST/"
    if [ -n "${CF_ACCESS_CLIENT_ID:-}" ] && [ -n "${CF_ACCESS_CLIENT_SECRET:-}" ]; then
        # The first request of an MCP client. 200 shows the full way: the
        # tunnel, the token check of the server and the MCP server.
        with_token "https://$LIFEOS_MCP_HOST/mcp" 200 \
            -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
            -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"check-access","version":"1"}}}'
    fi
fi
if [ -n "${LIFEOS_DASH_HOST:-}" ]; then
    # Grafana has no login of its own: Access is the one lock.
    guarded login "https://$LIFEOS_DASH_HOST/"
fi

exit "$failed"
