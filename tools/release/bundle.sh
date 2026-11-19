#!/bin/sh
# Packs one built light into the archive a release ships, in dist:
#
#	macOS    light_<version>_darwin_<arch>.zip holding light.app: light in
#	         Contents/MacOS, the wgpu-native library in Contents/Frameworks,
#	         light-model in Contents/Helpers, the icon, the licences and the
#	         README in Contents/Resources, signed ad hoc
#	Linux    light_<version>_linux_<arch>.tar.gz holding one folder: light,
#	         light-model, libwgpu_native.so, light.desktop, light.png, the
#	         licences and the README side by side
#	Windows  light_<version>_windows_<arch>.zip, one folder too: light.exe,
#	         light-model.exe, wgpu_native.dll, the licences and the README
#
# light-model, the helper that runs a language model for light, is built
# here from helper/model, a module of its own and pure Go, at the same
# version: a released light refuses a helper of another one. It goes where
# pkg/model.FindHelper looks: beside light, or in the app's Helpers.
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

# helper builds light-model for the target as the file $1.
helper() {
	flags="-s -w -X main.version=$version"
	# As light there: started by a windowed program, it opens no console.
	[ "$goos" != windows ] || flags="$flags -H=windowsgui"
	to=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
	(cd helper/model && GOWORK=off GOFLAGS='' CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
		go build -trimpath -ldflags "$flags" -o "$to" .)
}

# licences copies what the archive owes its parts into the folder $1.
licences() {
	cp LICENSE README.md "$wgpu"/LICENSE-wgpu-native.* "$1/"
	# What light-model is built on, each module's licence under its last name.
	(cd helper/model && GOWORK=off GOFLAGS='' GOOS="$goos" GOARCH="$goarch" go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}} {{.Dir}}{{end}}{{end}}' .) |
		sort -u | while read -r path dir; do
		dir=$(slashes "$dir")
		for f in "$dir"/LICENSE "$dir"/LICENSE.txt "$dir"/LICENSE.md; do
			[ -f "$f" ] || continue
			cp "$f" "$1/LICENSE-$(basename "$path")"
			chmod u+w "$1/LICENSE-$(basename "$path")"
			break
		done
	done
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

# icon makes light.icns in the folder $1 with what macOS ships (sips,
# iconutil): from logo.svg where this macOS's sips draws an SVG, else from
# icon.png beside this script, which is logo.svg drawn at 1024 pixels
# (sips -s format png -z 1024 1024 logo.svg --out tools/release/icon.png).
icon() {
	set_=$wgpu/light.iconset
	master=$wgpu/icon-1024.png
	rm -rf "$set_" "$master"
	mkdir -p "$set_"
	if ! sips -s format png -z 1024 1024 logo.svg --out "$master" > /dev/null 2>&1 ||
		[ "$(sips -g pixelWidth "$master" 2> /dev/null | awk '/pixelWidth/ { print $2 }')" != 1024 ]; then
		echo "bundle: sips does not draw logo.svg here; the icon is made from tools/release/icon.png" >&2
		cp "$(dirname "$0")/icon.png" "$master"
	fi
	for s in 16 32 128 256 512; do
		sips -z "$s" "$s" "$master" --out "$set_/icon_${s}x${s}.png" > /dev/null
		d=$((s * 2))
		sips -z "$d" "$d" "$master" --out "$set_/icon_${s}x${s}@2x.png" > /dev/null
	done
	iconutil -c icns -o "$1/light.icns" "$set_"
}

stage=$dist/stage/$name
rm -rf "$stage"

if [ "$goos" = darwin ]; then
	app=$stage/light.app
	mkdir -p "$app/Contents/MacOS" "$app/Contents/Frameworks" "$app/Contents/Helpers" "$app/Contents/Resources"
	cp "$bin" "$app/Contents/MacOS/light"
	cp "$src" "$app/Contents/Frameworks/$lib"
	helper "$app/Contents/Helpers/light-model"
	licences "$app/Contents/Resources"
	for f in "$app/Contents/MacOS/light" "$app/Contents/Frameworks/$lib" "$app/Contents/Helpers/light-model"; do
		v=$(minos "$f")
		if [ -z "$v" ] || later "$v" "$macos_min"; then
			echo "bundle: $f needs macOS ${v:-unknown}, later than $macos_min" >&2
			exit 1
		fi
	done
	icon "$app/Contents/Resources"
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
	<key>CFBundleIconFile</key><string>light</string>
	<key>CFBundleInfoDictionaryVersion</key><string>6.0</string>
	<key>LSApplicationCategoryType</key><string>public.app-category.productivity</string>
	<key>LSMinimumSystemVersion</key><string>$macos_min</string>
	<key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
EOF
	plutil -lint "$app/Contents/Info.plist" > /dev/null
	# Ad hoc: there is no Apple Developer identity, so nothing is notarized.
	# --deep signs the library and the helper inside before the app over them.
	codesign --force --deep --sign - "$app"
	codesign --verify --deep --strict "$app"
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
	helper "$stage/light-model.exe"
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
	helper "$stage/light-model"
	cp "$(dirname "$0")/light.desktop" "$(dirname "$0")/icon.png" "$stage/"
	mv "$stage/icon.png" "$stage/light.png"
	out=$name.tar.gz
	tar -czf "$dist/$out" -C "$dist/stage" "$name"
fi
echo "bundle: $dist/$out"
