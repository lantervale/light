#!/bin/sh
# Completes the folder $1 (dist by default), which holds every target's
# archive, into what a release publishes: an SBOM of each archive (syft),
# the source archive of HEAD, and the SHA-256 of them all in
# light_<version>-checksums.txt. The version is read off the archives'
# names, so a snapshot's are completed the same way. syft reads no DMG:
# a macOS archive comes with the SBOM bundle.sh made of the app inside.
#
#	tools/release/assemble.sh [dir]
set -eu
dir=${1:-dist}

first=$(find "$dir" -maxdepth 1 -type f \( -name 'light_*.zip' -o -name 'light_*.tar.gz' -o -name 'light_*.dmg' \) | sort | head -1)
if [ -z "$first" ]; then
	echo "assemble: no light_* archive in $dir" >&2
	exit 1
fi
version=$(basename "$first" | sed -E 's/^light_(.*)_[a-z]+_[a-z0-9]+\.(zip|tar\.gz|dmg)$/\1/')

for archive in "$dir"/light_*.dmg; do
	[ -f "$archive" ] || continue
	if [ ! -f "$archive.sbom.json" ]; then
		echo "assemble: $archive has no SBOM beside it; bundle.sh makes it from the app where syft is on PATH" >&2
		exit 1
	fi
done
for archive in "$dir"/light_*.zip "$dir"/light_*.tar.gz; do
	[ -f "$archive" ] || continue
	syft scan "file:$archive" --quiet --output "spdx-json=$archive.sbom.json"
done

git archive --format=tar.gz --prefix="light-$version/" -o "$dir/light-$version.tar.gz" HEAD

sums=light_$version-checksums.txt
rm -f "$dir/$sums"
(
	cd "$dir"
	files=$(find . -maxdepth 1 -type f \( -name 'light_*' -o -name 'light-*' \) | sed 's|^\./||' | sort)
	if command -v sha256sum > /dev/null 2>&1; then
		# shellcheck disable=SC2086 # one name per word, none with a space
		sha256sum $files > "$sums"
	else
		# shellcheck disable=SC2086
		shasum -a 256 $files > "$sums"
	fi
)
echo "assemble: $dir/$sums"
