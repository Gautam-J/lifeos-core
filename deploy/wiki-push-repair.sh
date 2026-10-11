#!/bin/bash
# Repairs the push of the wiki after GitHub got a commit that the wiki on
# the Pi does not have: a change on github.com, or a push from a different
# clone. wiki-push.sh never forces, so it fails each hour in that state.
#
# The wiki on the Pi wins, because the server is the one writer of the wiki.
# This script loses nothing: it puts the commits that only GitHub has on a
# branch from-github-<time> on GitHub, and then it puts the history of the
# wiki on main. It changes main only when GitHub still has the commit that
# it put on the branch.
#
# After the repair, read the change and give it to the agent, which writes
# the page through the server. The script prints the command.
# Usage: wiki-push-repair.sh [mirror]
set -euo pipefail

mirror=${1:-/var/backups/lifeos/wiki.git}
g() { git --git-dir="$mirror" "$@"; }

g fetch --quiet --no-tags origin main
theirs=$(g rev-parse refs/remotes/origin/main)
if g merge-base --is-ancestor "$theirs" main; then
    echo "GitHub has no commit that the wiki does not have: nothing to repair"
    exit 0
fi

branch=from-github-$(date +%Y%m%d-%H%M%S)
g push --quiet origin "$theirs:refs/heads/$branch"
g push --quiet --force-with-lease="main:$theirs" origin main
echo "main on GitHub has the history of the wiki again."
echo "The commits that only GitHub had are on the branch $branch. To read them:"
echo "  git --git-dir=$mirror log -p main..$theirs"
