#!/bin/sh
# Makes LLVM's MSVC-target clang, linking with LLD, the C and C++ compiler of
# the GitHub Actions job it runs in, on Windows: the V8 the binding ships for
# Windows is built for the MSVC ABI, so light is not built with mingw-w64
# there. The runner's Visual Studio Build Tools give the Microsoft C runtime
# and the Windows SDK. CLANG_VERSION is the oldest major version taken (what
# V8's libc++ headers ask for): the runner's LLVM when it is that recent,
# else LLVM's own installer of the release below, checked, in a folder of its
# own.
set -eu
want=${CLANG_VERSION:?}
release=21.1.8
sha256=7a5386c26497db1691f320121e5b113364dd0274b98e55f15f4dbc00c0450113

llvm='C:\Program Files\LLVM'
have=$("$(cygpath -u "$llvm")/bin/clang" -dumpversion 2> /dev/null | cut -d. -f1 || true)
if [ -z "$have" ] || [ "$have" -lt "$want" ]; then
	[ "${release%%.*}" -ge "$want" ] || { echo "llvm: $release is older than $want: update this script" >&2; exit 1; }
	echo "llvm: clang ${have:-none} in $llvm, installing LLVM $release"
	exe=$RUNNER_TEMP/LLVM-$release-win64.exe
	curl -fsSL --retry 5 --retry-all-errors -o "$exe" \
		"https://github.com/llvm/llvm-project/releases/download/llvmorg-$release/LLVM-$release-win64.exe"
	echo "$sha256  $exe" | sha256sum -c -
	llvm='C:\LLVM'
	# NSIS: silent, and /D last and unquoted. MSYS_NO_PATHCONV keeps Git
	# Bash from taking /S for a path.
	MSYS_NO_PATHCONV=1 timeout 600 "$exe" /S "/D=$llvm"
fi
"$(cygpath -u "$llvm")/bin/clang" --version
# Go splits CC on spaces, so clang is named, not given by its path: its
# folder comes first on PATH.
echo "$llvm\\bin" >> "$GITHUB_PATH"
# GLFW's cgo flags put its copy of mingw-w64's DirectInput and XInput
# headers before the Windows SDK's, and its xinput.h names mingw's WINBOOL;
# the Microsoft C runtime's warnings on strcpy and the like are its own. The
# system libraries mingw-w64 links by default and GLFW therefore does not
# name are named here.
{
	echo "CC=clang -fuse-ld=lld"
	echo "CXX=clang++ -fuse-ld=lld"
	echo "CGO_CFLAGS=-O2 -g -DWINBOOL=BOOL -D_CRT_SECURE_NO_WARNINGS"
	echo "CGO_LDFLAGS=-O2 -g -luser32 -lshell32 -ladvapi32"
} >> "$GITHUB_ENV"
