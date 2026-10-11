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
LIFEOS_MCP_HOST=mcp.test
CF_ACCESS_CLIENT_ID=id
CF_ACCESS_CLIENT_SECRET=secret
ENV

# check <name> <want exit> <want text> <answers>: runs check-access.sh
# against the table of answers, with the env file $hosts.
hosts=$tmp/hosts.env
check() {
    local name=$1 want_exit=$2 want_text=$3 output status=0
    printf '%s\n' "$4" > "$tmp/answers"
    output=$(ANSWERS="$tmp/answers" PATH="$tmp/bin:$PATH" ./check-access.sh "$hosts" 2>&1) || status=$?
    if [ "$status" != "$want_exit" ] || [[ $output != *"$want_text"* ]]; then
        echo "FAIL  check-access: $name: exit $status, want $want_exit with the text '$want_text'"
        echo "$output" | sed 's/^/      /'
        failed=1
    fi
}

good="https://admin.test/admin/items 302 $login
https://admin.test/api/items 302 $login
https://api.test/api/items 401
https://mcp.test/mcp 401
https://mcp.test/ 401
token:https://api.test/api/items 200
token:https://api.test/admin/items 404
token:https://mcp.test/mcp 200"

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

# The MCP hostname. The MCP server checks the token of Access itself and
# answers 403 to a request that has no valid token. So a 403 here shows
# that a request came past Access: the hostname is not guarded.
check "the MCP hostname is guarded" 0 "ok    https://mcp.test/mcp: Access refuses the request (401)" "$good"
check "the MCP server answers the service token" 0 "ok    https://mcp.test/mcp: status 200 with the service token" "$good"
check "the MCP hostname answers" 1 "OPEN  https://mcp.test/mcp" "${good/mcp.test\/mcp 401/mcp.test\/mcp 200}"
check "only the MCP server refuses" 1 "OPEN  https://mcp.test/mcp: status 403" "${good/mcp.test\/mcp 401/mcp.test\/mcp 403}"
check "a different path of the MCP hostname answers" 1 "OPEN  https://mcp.test/: status 404" "${good/mcp.test\/ 401/mcp.test\/ 404}"
check "the MCP server refuses the service token" 1 "FAIL  https://mcp.test/mcp: status 403 with the service token" "${good/token:https:\/\/mcp.test\/mcp 200/token:https:\/\/mcp.test\/mcp 403}"
# The MCP hostname alone is a hostname to check.
printf 'LIFEOS_MCP_HOST=mcp.test\n' > "$tmp/mcp.env"
hosts=$tmp/mcp.env
check "only the MCP hostname is set" 1 "OPEN  https://mcp.test/mcp" "https://mcp.test/mcp 200"
hosts=$tmp/hosts.env

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

# wiki-push.sh copies the commits of the live wiki to the remote of the
# mirror. Here the remote, the live repository and the mirror are in the
# temporary folder, and no command uses the network.
w=$tmp/wiki
wgit() { git -c user.name=test -c user.email=test@example.com -c init.defaultBranch=main "$@"; }
# wiki_commit <text>: one more commit in the live repository, as the server
# makes one for each write of a page.
wiki_commit() {
    echo "$1" >> "$w/live/page.md"
    wgit -C "$w/live" add page.md
    wgit -C "$w/live" commit -q -m "$1"
}
# wiki_repos: the remote with one commit, the live repository with that
# commit, and the mirror as the header of wiki-push.sh says to make it.
wiki_repos() {
    rm -rf "$w"
    mkdir -p "$w"
    wgit init -q --bare "$w/remote.git"
    wgit init -q "$w/live"
    wiki_commit one
    git -C "$w/live" push -q "$w/remote.git" main
    wgit init -q --bare "$w/mirror.git"
    git -C "$w/mirror.git" remote add origin "$w/remote.git"
    git -C "$w/mirror.git" fetch -q origin
}
# push_check <name> <want exit> <want text>: runs wiki-push.sh.
push_check() {
    local name=$1 want_exit=$2 want_text=$3 output status=0
    output=$(./wiki-push.sh "$w/live" "$w/mirror.git" 2>&1) || status=$?
    if [ "$status" != "$want_exit" ] || [[ $output != *"$want_text"* ]]; then
        echo "FAIL  wiki-push: $name: exit $status, want $want_exit with the text '$want_text'"
        echo "$output" | sed 's/^/      /'
        failed=1
    fi
}
# remote_is <name> <commit>: the remote must be at this commit.
remote_is() {
    if [ "$(git -C "$w/remote.git" rev-parse main)" != "$2" ]; then
        echo "FAIL  wiki-push: $1: the remote is not at the commit $2"
        failed=1
    fi
}

wiki_repos
push_check "nothing new" 0 "GitHub has each commit of the wiki"
wiki_commit two
push_check "a new commit" 0 "pushed"
remote_is "a new commit" "$(git -C "$w/live" rev-parse HEAD)"
push_check "the same commit a second time" 0 "GitHub has each commit of the wiki"

# The server can write the live repository. A setting or a hook there must
# not make this script run a program. The trap is a program that leaves a
# file; each setting and each hook below names it.
wiki_commit three
printf '#!/bin/sh\ntouch %s\n' "$w/ran" > "$w/trap"
chmod +x "$w/trap"
mkdir "$w/hooks"
for hook in reference-transaction pre-auto-gc post-update pre-receive update post-receive; do
    cp "$w/trap" "$w/hooks/$hook"
done
for setting in uploadpack.packObjectsHook core.fsmonitor core.sshCommand core.pager core.alternateRefsCommand; do
    git -C "$w/live" config "$setting" "$w/trap"
done
git -C "$w/live" config core.hooksPath "$w/hooks"
push_check "settings and hooks in the live repository" 0 "pushed"
remote_is "settings and hooks in the live repository" "$(git --git-dir="$w/live/.git" rev-parse main)"
if [ -e "$w/ran" ]; then
    echo "FAIL  wiki-push: a setting or a hook of the live repository made the script run a program"
    failed=1
fi
# The control: a git command inside the live repository does run the trap.
# Without this, the case above proves nothing.
git -C "$w/live" status > /dev/null 2>&1 || true
if [ ! -e "$w/ran" ]; then
    echo "FAIL  wiki-push: the trap of this test does not work: git status inside the live repository did not run it"
    failed=1
fi

# The script never forces. A history that does not continue the last one
# stops it, and the remote keeps what it has.
wiki_repos
wiki_commit two
push_check "a new commit" 0 "pushed"
before=$(git -C "$w/remote.git" rev-parse main)
wgit -C "$w/live" commit -q --amend -m "two, changed"
push_check "the live wiki lost a commit" 1 "does not continue the history of the mirror"
remote_is "the live wiki lost a commit" "$before"
# The mirror keeps the commit that the live wiki lost.
if [ "$(git --git-dir="$w/mirror.git" rev-parse main)" != "$before" ]; then
    echo "FAIL  wiki-push: the live wiki lost a commit: the mirror did not keep the commit $before"
    failed=1
fi

wiki_repos
wgit clone -q "$w/remote.git" "$w/other"
echo other > "$w/other/other.md"
wgit -C "$w/other" add other.md
wgit -C "$w/other" commit -q -m other
git -C "$w/other" push -q origin main
before=$(git -C "$w/remote.git" rev-parse main)
wiki_commit two
push_check "the remote has a commit that the wiki does not have" 1 "deploy/wiki-push-repair.sh"
remote_is "the remote has a commit that the wiki does not have" "$before"

# wiki-push-repair.sh is the fix for that state. It keeps the commit of the
# remote on a branch there, and it puts the history of the wiki on main.
# repair_check <name> <want text>: runs wiki-push-repair.sh.
repair_check() {
    local name=$1 want_text=$2 output status=0
    output=$(./wiki-push-repair.sh "$w/mirror.git" 2>&1) || status=$?
    if [ "$status" != 0 ] || [[ $output != *"$want_text"* ]]; then
        echo "FAIL  wiki-push-repair: $name: exit $status, want 0 with the text '$want_text'"
        echo "$output" | sed 's/^/      /'
        failed=1
    fi
}
repair_check "the remote has a commit that the wiki does not have" "are on the branch from-github-"
remote_is "after the repair" "$(git -C "$w/live" rev-parse HEAD)"
kept=$(git -C "$w/remote.git" for-each-ref --format='%(objectname)' 'refs/heads/from-github-*')
if [ "$kept" != "$before" ]; then
    echo "FAIL  wiki-push-repair: the remote does not keep its commit $before on a branch from-github-*"
    failed=1
fi
push_check "after the repair" 0 "GitHub has each commit of the wiki"
wiki_commit three
push_check "a new commit after the repair" 0 "pushed"
# A second run finds nothing to do and makes no second branch.
repair_check "nothing to repair" "nothing to repair"
if [ "$(git -C "$w/remote.git" for-each-ref 'refs/heads/from-github-*' | wc -l)" != 1 ]; then
    echo "FAIL  wiki-push-repair: a run with nothing to repair made a branch"
    failed=1
fi

# The default paths of wiki-push.sh. The live repository holds wiki_dir.
# The mirror is in a folder that the server cannot write: the script runs
# git inside the mirror, with the SSH key of the user.
push_live=$(sed -n 's/^live=${1:-\(.*\)}$/\1/p' wiki-push.sh)
push_mirror=$(sed -n 's/^mirror=${2:-\(.*\)}$/\1/p' wiki-push.sh)
case $wiki_dir/ in
    "$push_live"/*) ;;
    *)
        echo "FAIL  wiki-push.sh: wiki_dir ($wiki_dir) is not in its live repository ($push_live)"
        failed=1
        ;;
esac
if [ -z "$push_mirror" ]; then
    echo "FAIL  wiki-push.sh: no default path of the mirror"
    failed=1
fi
for path in $(sed -n 's/^ReadWritePaths=//p' systemd/lifeos.service); do
    case $push_mirror/ in
        "$path"/*)
            echo "FAIL  wiki-push.sh: the server can write the mirror ($push_mirror)"
            failed=1
            ;;
    esac
done

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
