#!/bin/sh
# Pins ember's main in go.mod as it is done by hand, when it moved: go get
# that commit, go mod tidy, then the release preflight, which fails when
# light no longer builds on it. Run from light's folder; it says what it
# did on its last line ("ember: pinned <sha>" or "ember: up to date").
#
#	tools/release/bump-ember.sh [sha]
#
# Without sha, ember's main now (EMBER_REPO, github.com/lantervale/ember by
# default). DRY_RUN=1 says what it would run, changing nothing.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
repo=${EMBER_REPO:-https://github.com/lantervale/ember}
sha=${1:-$(git ls-remote "$repo" refs/heads/main | cut -f1)}
case "$sha" in
	*[!0-9a-f]* | "") echo "ember: no commit of ember's main ($sha)" >&2; exit 1 ;;
esac
pin=$(awk '$1 == "github.com/lantervale/ember" { print $2 }' go.mod)
pinned=${pin##*-}
if [ -n "$pinned" ] && [ "${sha#"$pinned"}" != "$sha" ]; then
	echo "ember: up to date at $pin"
	exit 0
fi
run() {
	if [ -n "${DRY_RUN:-}" ]; then
		echo "ember: would run $*"
	else
		"$@"
	fi
}
run env GOWORK=off GOFLAGS=-mod=mod go get "github.com/lantervale/ember@$sha"
run env GOWORK=off go mod tidy
run env GOWORK=off sh "$here/preflight.sh" false
echo "ember: pinned $sha, was $pin"
