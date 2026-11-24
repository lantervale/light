#!/bin/sh
# Makes LLVM's MSVC-target clang, linking with LLD, the C and C++ compiler of
# the GitHub Actions job it runs in, on Windows: the V8 the binding ships for
# Windows is built for the MSVC ABI, so light is not built with mingw-w64
# there. The runner's Visual Studio Build Tools give the Microsoft C runtime
# and the Windows SDK. CLANG_VERSION is the oldest major version taken (what
# V8's libc++ headers ask for); an older LLVM is replaced with Chocolatey's.
set -eu
want=${CLANG_VERSION:?}
llvm='C:\Program Files\LLVM\bin'
bin=$(cygpath -u "$llvm")
have=$("$bin/clang" -dumpversion 2> /dev/null | cut -d. -f1 || true)
if [ -z "$have" ] || [ "$have" -lt "$want" ]; then
	echo "llvm: clang ${have:-none} at $llvm, installing LLVM $want"
	v=$(choco search llvm --exact --all-versions --limit-output | awk -F'|' -v w="$want" 'index($2, w ".") == 1 { print $2 }' | sort -t. -k1,1n -k2,2n -k3,3n | tail -n 1)
	choco install llvm --version "${v:?no LLVM $want on Chocolatey}" --allow-downgrade --force -y --no-progress
fi
"$bin/clang" --version
# Go splits CC on spaces, so clang is named, not given by its path: LLVM's
# folder comes first on PATH.
echo "$llvm" >> "$GITHUB_PATH"
# GLFW's cgo flags put its copy of mingw-w64's DirectInput and XInput
# headers before the Windows SDK's, and its xinput.h names mingw's WINBOOL.
{
	echo "CC=clang -fuse-ld=lld"
	echo "CXX=clang++ -fuse-ld=lld"
	echo "CGO_CFLAGS=-O2 -g -DWINBOOL=BOOL"
} >> "$GITHUB_ENV"
