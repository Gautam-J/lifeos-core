#!/bin/bash
# Copies the commits of the live wiki to GitHub. The server makes one commit
# for each write of a page. It does not push: it has no key.
#
# The server can write the live repository, so a setting or a hook there
# can be the work of a request from the internet. This script runs no git
# command inside the live repository. It fetches from there into the
# mirror, a bare repository that the server cannot write, and it pushes
# from the mirror. For a fetch, git starts only `git upload-pack` in the
# source, and that program runs no hook and no program that a setting of
# the source names. deploy/test.sh holds a case for that.
#
# The script never forces. When the live wiki or GitHub has a history that
# does not continue the history of the mirror, it fails, and the unit sends
# the failure to ntfy. wiki-push-repair.sh is the fix for a commit that only
# GitHub has.
#
# One time, make the mirror:
#   git init --bare /var/backups/lifeos/wiki.git
#   git -C /var/backups/lifeos/wiki.git remote add origin <URL of the wiki repository>
#   git -C /var/backups/lifeos/wiki.git fetch origin
# Usage: wiki-push.sh [live-repository [mirror]]
set -euo pipefail

live=${1:-/var/lib/lifeos/lifeos-wiki}
mirror=${2:-/var/backups/lifeos/wiki.git}

# The one known way that a fetch runs a program of its source is a lazy
# fetch: the source is a partial clone, and git gets an absent object with
# the program that a setting of the source names. git has this off by
# default; this line keeps it off when the environment says something else.
export GIT_NO_LAZY_FETCH=1

# git gives no text when it refuses a fetch that is not a fast-forward.
if ! git --git-dir="$mirror" fetch --quiet --no-tags --no-recurse-submodules "$live" main:main; then
    echo "wiki-push: the fetch from $live failed. With no reason from git above, the live wiki does not continue the history of the mirror: a commit that the mirror has is not in the live wiki." >&2
    exit 1
fi
# The mirror knows what GitHub has from its last push. No change: no
# connection to GitHub.
if [ "$(git --git-dir="$mirror" rev-parse main)" = "$(git --git-dir="$mirror" rev-parse --quiet --verify refs/remotes/origin/main)" ]; then
    echo "GitHub has each commit of the wiki"
    exit 0
fi
if ! git --git-dir="$mirror" push --quiet origin main; then
    echo "wiki-push: GitHub refused the push. When GitHub has a commit that the wiki does not have, run deploy/wiki-push-repair.sh: it keeps that commit on a branch and puts the history of the wiki on main." >&2
    exit 1
fi
echo "pushed $(git --git-dir="$mirror" rev-parse --short main)"
