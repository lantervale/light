#!/bin/sh
# Fails a release early, saying why, when light does not build from its
# go.mod alone: a release is built from a clean checkout, where no Go
# workspace stands in for the ember that go.mod pins.
#
#	tools/release/preflight.sh <snapshot: true|false>
#
# A snapshot made inside a workspace is let through with a warning: it
# is built on the ember beside light, which is how light is worked on.
set -eu
snapshot=${1:-false}
work=$(go env GOWORK)

if [ -n "$work" ] && [ "$work" != off ]; then
	if [ "$snapshot" = true ]; then
		echo "release: snapshot built through the workspace $work: on the ember beside light, not the one go.mod pins" >&2
		exit 0
	fi
	echo "release: the workspace $work is in effect; a release is built with GOWORK=off, from go.mod alone" >&2
	exit 1
fi

case "$(go env GOOS)" in
	darwin) tags=v8 ;;
	linux) tags=v8,vulkan ;;
	*) tags= ;;
esac
log=$(mktemp)
trap 'rm -f "$log"' EXIT
if ! GOWORK=off CGO_ENABLED=1 CGO_CXXFLAGS="${CGO_CXXFLAGS:--nostdinc++}" go build -trimpath -tags "$tags" ./cmd/light/... > "$log" 2>&1; then
	pin=$(GOWORK=off go list -m -f '{{.Version}}' github.com/lantervale/ember 2> /dev/null || echo unknown)
	{
		echo "release: light does not build from go.mod and go.sum alone (ember pinned at $pin)."
		echo "release: when its code needs a later ember, merge that work in ember, then bump the pin (go get github.com/lantervale/ember@main, go mod tidy) before tagging."
		echo "release: the first errors were:"
		# Without the compiler's warnings on V8's headers, which come first.
		grep -v -e 'warning:' -e 'note:' -e '^In file included' "$log" | head -20
	} >&2
	exit 1
fi
