#!/bin/bash
# Tests of the files in deploy/ that need no network and no service.
# `make test` runs this file.
set -euo pipefail

cd "$(dirname "$0")"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
failed=0

# A curl that answers from a table. Each line of $ANSWERS is
# "<url> <status> [<redirect target>]"; a request that has the header of
# an Access service token looks for "token:<url>".
mkdir "$tmp/bin"
cat > "$tmp/bin/curl" <<'STUB'
#!/bin/bash
url=${!#}
key=$url
format=code
for arg in "$@"; do
    case $arg in
        CF-Access-Client-Id:*) key="token:$url" ;;
        *redirect_url*) format=both ;;
    esac
done
read -r _ code target < <(grep -F -- "$key " "$ANSWERS" | head -1) || true
if [ "$format" = both ]; then
    echo "${code:-000} ${target:-}"
else
    printf '%s' "${code:-000}"
fi
STUB
chmod +x "$tmp/bin/curl"

login=https://team.cloudflareaccess.com/cdn-cgi/access/login/x
cat > "$tmp/hosts.env" <<'ENV'
LIFEOS_ADMIN_HOST=admin.test
LIFEOS_API_HOST=api.test
CF_ACCESS_CLIENT_ID=id
CF_ACCESS_CLIENT_SECRET=secret
ENV

# check <name> <want exit> <want text> <answers>: runs check-access.sh
# against the table of answers.
check() {
    local name=$1 want_exit=$2 want_text=$3 output status=0
    printf '%s\n' "$4" > "$tmp/answers"
    output=$(ANSWERS="$tmp/answers" PATH="$tmp/bin:$PATH" ./check-access.sh "$tmp/hosts.env" 2>&1) || status=$?
    if [ "$status" != "$want_exit" ] || [[ $output != *"$want_text"* ]]; then
        echo "FAIL  check-access: $name: exit $status, want $want_exit with the text '$want_text'"
        echo "$output" | sed 's/^/      /'
        failed=1
    fi
}

good="https://admin.test/admin/items 302 $login
https://admin.test/api/items 302 $login
https://api.test/api/items 401
token:https://api.test/api/items 200
token:https://api.test/admin/items 404"

check "each hostname is guarded" 0 "status 200 with the service token" "$good"
check "the admin hostname answers" 1 "OPEN  https://admin.test/admin/items" "${good/admin.test\/admin\/items 302 $login/admin.test\/admin\/items 200}"
# 403 can come from a part of Cloudflare that is not Access, such as a
# firewall rule that stops curl and not a browser.
check "the admin hostname gives 403" 1 "OPEN  https://admin.test/admin/items" "${good/admin.test\/admin\/items 302 $login/admin.test\/admin\/items 403}"
check "the admin hostname gives 401" 1 "OPEN  https://admin.test/admin/items" "${good/admin.test\/admin\/items 302 $login/admin.test\/admin\/items 401}"
check "a redirect to a look-alike address" 1 "OPEN  https://admin.test/admin/items" "${good/admin.test\/admin\/items 302 $login/admin.test\/admin\/items 302 https:\/\/evil.example\/team.cloudflareaccess.com\/}"
check "the API hostname gives 403" 1 "OPEN  https://api.test/api/items" "${good/api.test\/api\/items 401/api.test\/api\/items 403}"
check "the API hostname answers" 1 "OPEN  https://api.test/api/items" "${good/api.test\/api\/items 401/api.test\/api\/items 200}"
check "the API hostname asks for a login" 0 "ok    https://api.test/api/items: Access asks for a login" "${good/api.test\/api\/items 401/api.test\/api\/items 302 $login}"
check "the service token does not get through" 1 "FAIL  https://api.test/api/items: status 403 with the service token" "${good/token:https:\/\/api.test\/api\/items 200/token:https:\/\/api.test\/api\/items 403}"
check "the API hostname gives an admin page" 1 "FAIL  https://api.test/admin/items: status 200 with the service token" "${good/token:https:\/\/api.test\/admin\/items 404/token:https:\/\/api.test\/admin\/items 200}"
check "no answer" 1 "FAIL  https://admin.test/admin/items: no answer" "nothing 000"

# The server gets only the two database passwords and the two values of
# Cloudflare Access for the token check of the MCP server. Each other key
# of the env file must be in UnsetEnvironment of the unit.
unset_keys=" $(sed -n 's/^UnsetEnvironment=//p' systemd/lifeos.service | tr '\n' ' ') "
for key in $(sed -n 's/^\([A-Z][A-Z0-9_]*\)=.*/\1/p' .env.example); do
    case $key in POSTGRES_PASSWORD | LIFEOS_QUERY_PASSWORD | CF_ACCESS_TEAM | LIFEOS_MCP_AUD) continue ;; esac
    if [[ $unset_keys != *" $key "* ]]; then
        echo "FAIL  lifeos.service: UnsetEnvironment does not name $key"
        failed=1
    fi
done

# The server writes the wiki pages and makes a commit for each write. The
# unit lets it write only below the paths of ReadWritePaths.
wiki_dir=$(sed -n 's/^wiki_dir: *//p' ../config/lifeos.yaml)
writable=0
for path in $(sed -n 's/^ReadWritePaths=//p' systemd/lifeos.service); do
    case $wiki_dir/ in "$path"/*) writable=1 ;; esac
done
if [ "$writable" = 0 ]; then
    echo "FAIL  lifeos.service: wiki_dir ($wiki_dir) is not below a path of ReadWritePaths"
    failed=1
fi

# The tests use the Postgres server postgres-test, whose data is in memory.
# One run of the tests writes about 2.5 GB, and that load must not go to the
# SD card or to the server that holds the real data.
test_port=$(sed -n 's/^export LIFEOS_TEST_DATABASE_URL.*@127\.0\.0\.1:\([0-9]*\)\/.*/\1/p' ../Makefile)
test_service=$(sed -n '/^  postgres-test:$/,/^$/p' docker-compose.yml)
if [ -z "$test_port" ] || [[ $test_service != *"\"127.0.0.1:$test_port:5432\""* ]]; then
    echo "FAIL  Makefile: the tests do not use the port of postgres-test (LIFEOS_TEST_DATABASE_URL has the port '$test_port')"
    failed=1
fi
if [[ $test_service != *"tmpfs:"* ]] || [[ $test_service == *"pgdata"* ]]; then
    echo "FAIL  docker-compose.yml: the data of postgres-test is not in memory"
    failed=1
fi

[ "$failed" = 0 ] && echo "ok    deploy/test.sh"
exit "$failed"
