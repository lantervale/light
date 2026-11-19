## light v0.2.0

light is a small web browser written from scratch in Go on the [ember](https://github.com/lantervale/ember) engine.
It fetches a page, builds it, lays it out and draws it with its own HTML and CSS code: there is no WebKit, Blink or
Gecko in it. Scripts run in V8 on macOS and Linux; the Windows build runs no script. It is a demo of how far
ember's UI goes on real pages, not a browser to live in.

## What is new

- **An optional model on your device**, experimental and off by default, turned on in the settings: questions
  about the page from the address bar, translation of a page or of a text, a card on a selection made with the
  right button (Explain, Summarise, Translate), find by meaning, bookmark suggestions, page summaries and page
  categories of your own.
- **light follows your system**: dark mode and languages, which the settings override; a click on the start
  page's logo switches the dark mode. The wheel has its direction and its speed in the settings.
- **A warning at the first start of each release** that light is an early preview.
- A new start page, a list under the address field, ICO and BMP icons, `min()`, `max()`, `clamp()` and the units
  of the font and the viewport in CSS, and fixes (a page that crashed light on an empty CSS variable, the zoom
  that never came back to 100 %).

> [!WARNING]
> A very early preview, not even an alpha. Its security is not hardened yet: **do not open sensitive pages with
> it and do not type a password, a card number or any other secret anywhere in it.** light says so itself at its
> first start.

## What to download

| System | Archive | Scripts |
|---|---|---|
| macOS 13 or later, Apple silicon | `light_<version>_darwin_arm64.zip` | yes (V8) |
| macOS 13 or later, Intel | `light_<version>_darwin_amd64.zip` | yes (V8) |
| Linux, x86-64 (glibc 2.35 or later) | `light_<version>_linux_amd64.tar.gz` | yes (V8) |
| Linux, arm64 (glibc 2.35 or later) | `light_<version>_linux_arm64.tar.gz` | yes (V8) |
| Windows 10 or later, x86-64 | `light_<version>_windows_amd64.zip` | **no**: pages show without their scripts |

## What is in an archive

- **macOS**: `light.app`. Inside it: `Contents/MacOS/light` (the browser), `Contents/Frameworks/libwgpu_native.dylib`
  (the wgpu-native library it draws with), `Contents/Helpers/light-model` (see the model below), and the icon, the
  licences and the README in `Contents/Resources`.
- **Linux**: one folder holding `light`, `libwgpu_native.so`, `light-model`, `light.desktop`, `light.png`, the
  licences and the README.
- **Windows**: one folder holding `light.exe`, `wgpu_native.dll`, `light-model.exe`, the licences and the README.

## How to run it

- **macOS**: unzip and move `light.app` to Applications. **The app is not notarized by Apple**: there is no Apple
  Developer identity behind it, and it is only signed ad hoc. macOS therefore refuses to open it the first time.
  Either right-click (or Control-click) `light.app` and choose **Open**, and where macOS 15 or later still refuses,
  open System Settings → Privacy & Security and choose **Open Anyway**; or run this once in a terminal:
  `xattr -dr com.apple.quarantine /Applications/light.app`.
  From a terminal, light is `/Applications/light.app/Contents/MacOS/light` (`--help` lists what it takes).
- **Linux**: unpack and run `./light` from its folder; `libwgpu_native.so` and `light-model` must stay beside it. It
  needs a Vulkan driver with the Vulkan loader, X11 (or XWayland) and ALSA; on Debian and Ubuntu:
  `sudo apt install libvulkan1 libx11-6 libxrandr2 libxinerama1 libxcursor1 libxi6 libxxf86vm1 libasound2`.
  `light.desktop` and `light.png` are there for a menu entry: they name `light`, so they work once `light` is on
  your `PATH` and you copy them to `~/.local/share/applications/` and `~/.local/share/icons/`.
- **Windows**: unzip and run `light.exe`; `wgpu_native.dll` and `light-model.exe` must stay beside it. It opens no
  console, so `light --version` prints nothing there.

## The model on this device

light can run a small language model on your machine, through `light-model`, a separate program in the archive. It
is **off by default**. Until you turn it on in the settings, under Experimental, and accept a model, nothing of it is downloaded, started
or written. Once you do, light downloads the llama.cpp libraries from `github.com/ggml-org/llama.cpp` and the model
from `huggingface.co` (1.1 or 2.5 GB), checks both against the sizes and SHA-256 this version was built with, and
runs them on your machine only.
[A model on this device](https://github.com/lantervale/light/blob/main/docs/model.md) says the rest.

## Known limits

- Windows runs no script, so pages that build themselves in JavaScript show little there.
- No notarization on macOS, no installer or package on any system, no automatic update.
- No cache, no sign-in prompt (401, 407), no file upload or `multipart/form-data` form, no AVIF, and
  `window.open` opens nothing.
- It draws what ember's UI draws: many pages look right, many do not.
- Only the macOS app (Apple silicon) was started by hand before this release; the other archives were built and
  packed by the release workflow and not run.

The reference lists [what it does not do](https://github.com/lantervale/light/blob/main/docs/reference.md#what-it-does-not-do) in full.

## Checking a download

`light_<version>-checksums.txt` lists the SHA-256 of every file of the release and is signed with cosign
(`.sigstore.json` beside it); each archive has its SBOM (`.sbom.json`).
