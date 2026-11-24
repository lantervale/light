# light: building, testing and releasing

## Build from source

You need Go 1.27, [Task](https://taskfile.dev) (`brew install go-task`), a C and C++ compiler, and a
[WebGPU-capable GPU](https://github.com/lantervale/ember/blob/main/docs/01-getting-started/README.md).

light is worked on beside ember, through a Go workspace. `go.mod` pins a commit of ember; inside the workspace
the ember beside light is used instead.

```sh
mkdir lantervale && cd lantervale
git clone https://github.com/lantervale/ember.git ember
git clone https://github.com/lantervale/light.git light
go work init ./ember ./light
(cd ember && task wgpu)        # ember's WebGPU library, one time only
cd light
task run                       # light with its JavaScript engine (the v8 build tag)
task run -- https://csszengarden.com
task run:plain                 # light without a JavaScript engine: go run ./cmd/light
task app                       # macOS: dist/light.app, as a release ships it, then `open dist/light.app`
```

`task run` is `CGO_CXXFLAGS=-nostdinc++ go run -tags v8 ./cmd/light`: V8's headers need their own libc++. Without
the tag light builds on every system and runs no script; `light://config` then says no engine is built in.

### On Windows

The V8 light links on Windows (x86-64) is built for the MSVC ABI, so light is compiled there with LLVM's clang,
which targets MSVC, linking with LLD, and not with mingw-w64. You need:

- [LLVM](https://github.com/llvm/llvm-project/releases) 21 or later, from its Windows installer
  (`LLVM-<version>-win64.exe`), its `bin` folder on `PATH`: Go splits `CC` on spaces, so clang is named, not given
  by its path;
- the Microsoft C runtime and the Windows SDK, which the Visual Studio Build Tools install (the "Desktop
  development with C++" workload);
- Git Bash, or another POSIX shell, for Task and the release scripts.

```sh
export CGO_ENABLED=1 CC="clang -fuse-ld=lld" CXX="clang++ -fuse-ld=lld" CGO_CXXFLAGS=-nostdinc++
export CGO_CFLAGS="-O2 -g -DWINBOOL=BOOL -D_CRT_SECURE_NO_WARNINGS" CGO_LDFLAGS="-O2 -g -luser32 -lshell32 -ladvapi32"
go run -tags v8 ./cmd/light
```

`-fuse-ld=lld` must be in `CC`, for Go to pass the linker flags LLD takes. GLFW is written for mingw-w64: its own
copy of mingw's XInput header names `WINBOOL`, and it leaves out the system libraries mingw links by default, hence
`CGO_CFLAGS` and `CGO_LDFLAGS`. `tools/release/llvm-windows.sh` sets the same for the `windows` and `release`
workflows. On Windows on arm64 V8 is not built in: light builds there without the tag, and runs no script.

## Driving it

`light drive` serves [W3C WebDriver](https://www.w3.org/TR/webdriver2/), the protocol the web-platform-tests
runner speaks. A light started any other way serves nothing.

```sh
task run -- drive --address 127.0.0.1:4444
EMBER_HIDDEN=1 go run -tags v8 ./cmd/light drive --address 127.0.0.1:4444   # without a window on screen
```

It listens on this machine only (`127.0.0.1`, `::1` or `localhost`; any other address is refused at launch),
answers 403 to a request whose `Host` is not this machine or that carries an `Origin`, and keeps its settings,
history and cookies in memory unless `--dir` names a folder. [WebDriver](reference.md#webdriver) lists the commands.

## Development

The tests need no GPU and no network: pages are served by a local test server, and clicks and keys are simulated.
They write nothing in your config or Downloads folders, and open no file.

```sh
task test             # go vet ./... and go test ./..., without and with the v8 tag
task lint             # golangci-lint, without and with the v8 tag (after task golangci:boilerplate, once)
task model:build      # light-model, the helper that runs the model, into dist/ (its own Go module)
task model:test       # its vet and tests
task wpt              # the web-platform-tests of tools/wpt/subset.txt (see "Web platform tests")
```

`.github/workflows/windows.yml` builds light with the `v8` tag on Windows and runs the tests of `pkg/script` and
`internal/dom` there, on every pull request to `main`.

- With the `v8` tag, every `go` command needs `CGO_CXXFLAGS=-nostdinc++`; the Taskfile sets it. By hand:
  `CGO_CXXFLAGS=-nostdinc++ go test -tags v8 ./...`.
- `go test -tags "v8 gpu" ./cmd/light` also drives the real program, hidden, over WebDriver and compares its
  screenshots; it needs ember's WebGPU library.
- A screenshot without opening a window:
  `EMBER_HIDDEN=1 EMBER_CAPTURE=shot.png EMBER_CAPTURE_FRAME=30 go run ./cmd/light`.
- The packages and how a page goes through them are in [Under the hood](reference.md#under-the-hood). Dependencies go one
  way: `cmd` → `internal` → `pkg`.

## Releasing

A tag `v*` pushed to GitHub runs `.github/workflows/release.yml`: each target is built on its own system (light is
cgo, so nothing cross compiles between systems) by `goreleaser build`, whose post hook (`tools/release/bundle.sh`)
packs the binary with `light-model` and the wgpu-native library, checked against
`tools/release/wgpu-native.sha256`, into a `.dmg` holding `light.app` on macOS (its SBOM made there, from the app:
syft reads no DMG), a `.tar.gz` on Linux and a `.zip` on Windows; the next job adds the other archives' SBOMs, the
source archive and the checksums (`tools/release/assemble.sh`), and the last signs them and publishes the GitHub
release, once every target built.
Run by hand from the Actions tab, the workflow does the same as a snapshot and publishes nothing.

It needs [GoReleaser](https://goreleaser.com) v2 (the open-source one), and syft for the SBOMs:
`brew install goreleaser syft`.

```sh
task release:snapshot   # this system's archive in dist/, as the workflow builds it (a .dmg on a Mac)
git tag v0.1.0 && git push origin v0.1.0
```

The Homebrew cask, `brew install --cask lantervale/tap/light`, lives in
[lantervale/homebrew-tap](https://github.com/lantervale/homebrew-tap): a workflow there reads light's latest release
every hour and points the cask at its DMGs and their checksums, so a release needs nothing more.

A release builds from `go.mod` and `go.sum` alone, without the workspace: `tools/release/preflight.sh` stops it
early, saying so, while light needs an ember later than the one pinned.
