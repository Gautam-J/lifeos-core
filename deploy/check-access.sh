#!/bin/bash
# Checks that Cloudflare Access guards each public hostname of LifeOS.
# LifeOS has no login of its own: a hostname that answers a request that
# has no login is open to the internet. Run this after each change of the
# tunnel or of an Access application; deploy.sh runs it after each deploy.
#
# Needs LIFEOS_ADMIN_HOST and LIFEOS_API_HOST in deploy/.env. With
# CF_ACCESS_CLIENT_ID and CF_ACCESS_CLIENT_SECRET (an Access service token),
# it also checks that the API answers a request that has the token, and
# that the API hostname gives no admin page.
# Usage: check-access.sh [env-file]   (default: deploy/.env)
set -euo pipefail

env_file=$(realpath "${1:-$(dirname "$0")/.env}")
set -a; . "$env_file"; set +a

if [ -z "${LIFEOS_ADMIN_HOST:-}" ] && [ -z "${LIFEOS_API_HOST:-}" ]; then
    echo "no public hostname is set in $env_file: nothing to check"
    exit 0
fi

failed=0

# guarded <url>: a request that has no login must stop at Cloudflare Access.
# Access answers 302 to its login page, or 401 or 403. LifeOS gives none of
# these to a GET.
guarded() {
    local url=$1 code target
    read -r code target < <(curl -s -o /dev/null --max-time 20 -w '%{http_code} %{redirect_url}\n' "$url" || true)
    case "$code" in
        302)
            if [[ $target == https://*.cloudflareaccess.com/* ]]; then
                echo "ok    $url: Access asks for a login"
                return
            fi ;;
        401|403)
            echo "ok    $url: Access refuses the request ($code)"
            return ;;
        000)
            echo "FAIL  $url: no answer"
            failed=1
            return ;;
    esac
    echo "OPEN  $url: status $code for a request that has no login"
    failed=1
}

# with_token <url> <status>: a request that has the service token must get
# this status from LifeOS.
with_token() {
    local url=$1 want=$2 code
    code=$(curl -s -o /dev/null --max-time 20 -w '%{http_code}' \
        -H "CF-Access-Client-Id: $CF_ACCESS_CLIENT_ID" \
        -H "CF-Access-Client-Secret: $CF_ACCESS_CLIENT_SECRET" "$url" || true)
    if [ "$code" = "$want" ]; then
        echo "ok    $url: status $code with the service token"
    else
        echo "FAIL  $url: status $code with the service token, want $want"
        failed=1
    fi
}

if [ -n "${LIFEOS_ADMIN_HOST:-}" ]; then
    guarded "https://$LIFEOS_ADMIN_HOST/admin/items"
    # The admin hostname gives no API route.
    guarded "https://$LIFEOS_ADMIN_HOST/api/items"
fi
if [ -n "${LIFEOS_API_HOST:-}" ]; then
    guarded "https://$LIFEOS_API_HOST/api/items"
    if [ -n "${CF_ACCESS_CLIENT_ID:-}" ] && [ -n "${CF_ACCESS_CLIENT_SECRET:-}" ]; then
        with_token "https://$LIFEOS_API_HOST/api/items" 200
        # The API hostname gives no admin page.
        with_token "https://$LIFEOS_API_HOST/admin/items" 404
    fi
fi

exit "$failed"
