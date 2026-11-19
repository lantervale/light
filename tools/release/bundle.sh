#!/bin/sh
# Packs one built light into the archive a release ships, in dist:
#
#	macOS    light_<version>_darwin_<arch>.zip holding light.app: light in
#	         Contents/MacOS, the wgpu-native library in Contents/Frameworks,
#	         the licences and the README in Contents/Resources, signed ad hoc
#	Linux    light_<version>_linux_<arch>.tar.gz holding one folder: light,
#	         libwgpu_native.so, the licences and the README side by side
#	Windows  light_<version>_windows_<arch>.zip, the same with light.exe and
#	         wgpu_native.dll
#
# It is GoReleaser's post-build hook (.goreleaser.yaml), which says what
# was built through the environment:
#
#	LIGHT_OS, LIGHT_ARCH  the target
#	LIGHT_BIN             the binary
#	LIGHT_VERSION         the version, for the archive's name and the app
#
# The library is fetched by ember's own fetch.sh, at the version the
# ember light is built on vendors the header of: the script and that
# version are copied out of ember (the module cache is read-only) and run
# from dist. The layout follows ember's package.sh, which cannot be used
# as it is: it builds the program itself, with its own flags.
set -eu
goos=${LIGHT_OS:?}
goarch=${LIGHT_ARCH:?}
# slashes turns a Windows path into one every tool of a POSIX shell there takes.
slashes() { printf %s "$1" | tr '\\' /; }
bin=$(slashes "${LIGHT_BIN:?}")
version=${LIGHT_VERSION:?}
dist=${LIGHT_DIST:-dist}
macos_min=${MACOS_MIN:-13.0}
name=light_${version}_${goos}_${goarch}

case "$goos" in
	windows) lib=wgpu_native.dll ;;
	darwin) lib=libwgpu_native.dylib ;;
	*) lib=libwgpu_native.so ;;
esac

ember=$(slashes "$(go list -m -f '{{.Dir}}' github.com/lantervale/ember)")
wgpu=$dist/wgpu-$goos-$goarch
rm -rf "$wgpu"
mkdir -p "$wgpu/include"
cp -R "$ember/internal/wgpu/scripts" "$wgpu/scripts"
cp "$ember/internal/wgpu/include/VERSION" "$wgpu/include/VERSION"
cp "$ember"/internal/wgpu/include/webgpu/LICENSE-wgpu-native.* "$wgpu/"
chmod -R u+w "$wgpu"
sh "$wgpu/scripts/fetch.sh" "$goos" "$goarch"
src=$wgpu/lib/$goos-$goarch/$lib

# fetch.sh checks nothing of what it downloads: the library must be the
# one wgpu-native.sha256 lists for this version and target.
wgpu_version=$(tr -d '[:space:]' < "$wgpu/include/VERSION")
want=$(awk -v v="$wgpu_version" -v t="$goos-$goarch" '$1 == v && $2 == t { print $3 }' "$(dirname "$0")/wgpu-native.sha256")
if [ -z "$want" ]; then
	echo "bundle: no checksum for wgpu-native $wgpu_version $goos-$goarch in tools/release/wgpu-native.sha256; add it once the library is checked" >&2
	exit 1
fi
if command -v sha256sum > /dev/null 2>&1; then
	got=$(sha256sum "$src" | cut -d' ' -f1)
else
	got=$(shasum -a 256 "$src" | cut -d' ' -f1)
fi
# Git Bash's sha256sum marks a path it had to escape with a leading backslash.
got=${got#\\}
if [ "$got" != "$want" ]; then
	echo "bundle: $src is not the wgpu-native $wgpu_version of tools/release/wgpu-native.sha256: sha256 $got, expected $want" >&2
	exit 1
fi

# licences copies what the archive owes its parts into the folder $1.
licences() {
	cp LICENSE README.md "$wgpu"/LICENSE-wgpu-native.* "$1/"
	# V8 is linked in where light runs scripts: macOS and Linux.
	if [ "$goos" != windows ]; then
		v8=$(go list -m -f '{{.Dir}}' "github.com/tommie/v8go/deps/${goos}_${goarch}")
		cp "$v8/LICENSE" "$1/LICENSE-v8"
		chmod u+w "$1/LICENSE-v8"
	fi
}

# minos is the macOS version the Mach-O file $1 says it needs.
minos() {
	otool -l "$1" | awk '/LC_BUILD_VERSION|LC_VERSION_MIN_MACOSX/ { found = 1 }
		found && ($1 == "minos" || $1 == "version") { print $2; exit }'
}

# later reports whether version $1 is later than $2, both major.minor[.patch].
later() {
	awk -v a="$1" -v b="$2" 'BEGIN {
		n = split(a, x, "."); m = split(b, y, ".")
		for (i = 1; i <= 3; i++) { if (x[i] + 0 != y[i] + 0) exit !(x[i] + 0 > y[i] + 0) }
		exit 1
	}'
}

# icon makes light.icns from logo.svg in the folder $1, with what macOS
# ships; an app without it still runs, so a failure only warns.
icon() {
	set_=$wgpu/light.iconset
	mkdir -p "$set_"
	for s in 16 32 128 256 512; do
		sips -s format png -z "$s" "$s" logo.svg --out "$set_/icon_${s}x${s}.png" > /dev/null 2>&1 || return 1
		d=$((s * 2))
		sips -s format png -z "$d" "$d" logo.svg --out "$set_/icon_${s}x${s}@2x.png" > /dev/null 2>&1 || return 1
	done
	iconutil -c icns -o "$1/light.icns" "$set_"
}

stage=$dist/stage/$name
rm -rf "$stage"

if [ "$goos" = darwin ]; then
	app=$stage/light.app
	mkdir -p "$app/Contents/MacOS" "$app/Contents/Frameworks" "$app/Contents/Resources"
	cp "$bin" "$app/Contents/MacOS/light"
	cp "$src" "$app/Contents/Frameworks/$lib"
	licences "$app/Contents/Resources"
	for f in "$app/Contents/MacOS/light" "$app/Contents/Frameworks/$lib"; do
		v=$(minos "$f")
		if [ -z "$v" ] || later "$v" "$macos_min"; then
			echo "bundle: $f needs macOS ${v:-unknown}, later than $macos_min" >&2
			exit 1
		fi
	done
	iconkey=
	if icon "$app/Contents/Resources"; then
		iconkey='<key>CFBundleIconFile</key><string>light</string>'
	else
		echo "bundle: no icon made from logo.svg; the app keeps the system's" >&2
	fi
	cat > "$app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key><string>light</string>
	<key>CFBundleIdentifier</key><string>io.github.lantervale.light</string>
	<key>CFBundleName</key><string>light</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleShortVersionString</key><string>${version%%-*}</string>
	<key>CFBundleVersion</key><string>$version</string>
	$iconkey
	<key>LSApplicationCategoryType</key><string>public.app-category.productivity</string>
	<key>LSMinimumSystemVersion</key><string>$macos_min</string>
	<key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
EOF
	codesign --force --sign - "$app/Contents/Frameworks/$lib"
	codesign --force --sign - "$app"
	codesign --verify --strict "$app"
	out=$dist/$name.zip
	rm -f "$out"
	ditto -c -k --norsrc --keepParent "$app" "$out"
	echo "bundle: $out"
	exit 0
fi

mkdir -p "$stage"
licences "$stage"
cp "$src" "$stage/$lib"
if [ "$goos" = windows ]; then
	cp "$bin" "$stage/light.exe"
	out=$name.zip
	rm -f "$dist/$out"
	if command -v zip > /dev/null 2>&1; then
		(cd "$dist/stage" && zip -q -r "../$out" "$name")
	elif command -v 7z > /dev/null 2>&1; then
		(cd "$dist/stage" && 7z a -tzip -bso0 "../$out" "$name")
	else
		echo "bundle: neither zip nor 7z to make $out" >&2
		exit 1
	fi
else
	cp "$bin" "$stage/light"
	out=$name.tar.gz
	tar -czf "$dist/$out" -C "$dist/stage" "$name"
fi
echo "bundle: $dist/$out"
