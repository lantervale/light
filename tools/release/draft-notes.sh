#!/bin/sh
# Drafts the notes of the next release in tools/release/notes.md: its title
# becomes "## light <version>" and its "What is new" section the
# conventional commits since the last tag (feat, fix and perf, merges
# left out), the rest of the notes kept as they are. A draft to rewrite by
# hand in its pull request before tagging. Run from light's folder.
#
#	tools/release/draft-notes.sh <version> [since]
#
# since is the last v* tag by default; NOTES names another notes file.
set -eu
version=$1
since=${2:-$(git describe --tags --abbrev=0 --match 'v*')}
notes=${NOTES:-$(cd "$(dirname "$0")" && pwd)/notes.md}
new=$(mktemp)
trap 'rm -f "$new" "$new.notes"' EXIT
git log --no-merges --reverse --format=%s "$since..HEAD" | awk '
	function add(kind, line,   rest, scope) {
		rest = line
		sub(/^[a-z]+(\([^)]*\))?!?: */, "", rest)
		scope = ""
		if (match(line, /^[a-z]+\([^)]*\)/)) {
			scope = substr(line, index(line, "(") + 1)
			scope = substr(scope, 1, index(scope, ")") - 1)
		}
		if (scope != "" && scope != "release") rest = "**" scope "**: " rest
		if (scope == "release") return
		items[kind] = items[kind] "- " rest "\n"
	}
	/^feat(\(|!|:)/ { add("New", $0) }
	/^fix(\(|!|:)/  { add("Fixed", $0) }
	/^perf(\(|!|:)/ { add("Faster", $0) }
	END {
		n = split("New Fixed Faster", kinds, " ")
		for (k = 1; k <= n; k++) {
			if (items[kinds[k]] == "") continue
			printf "%s### %s\n\n%s", (out++ ? "\n" : ""), kinds[k], items[kinds[k]]
		}
		if (!out) print "Nothing a commit since the last release calls a feature or a fix."
	}' > "$new"
awk -v version="$version" -v new="$new" '
	NR == 1 && /^## light / { print "## light " version; next }
	/^## What is new/ {
		print
		print ""
		while ((getline line < new) > 0) print line
		print ""
		skip = 1
		next
	}
	/^## / { skip = 0 }
	!skip { print }
' "$notes" > "$new.notes"
cat "$new.notes" > "$notes"
echo "notes: $notes drafted for $version from the commits since $since"
