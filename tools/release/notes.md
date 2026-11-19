## Install

| System | Download | JavaScript |
|---|---|---|
| macOS 13 or later, Apple silicon | `light_<version>_darwin_arm64.zip` | yes (V8) |
| macOS 13 or later, Intel | `light_<version>_darwin_amd64.zip` | yes (V8) |
| Linux, x86-64 | `light_<version>_linux_amd64.tar.gz` | yes (V8) |
| Linux, arm64 | `light_<version>_linux_arm64.tar.gz` | yes (V8) |
| Windows 10 or later, x86-64 | `light_<version>_windows_amd64.zip` | **no**: pages show without their scripts |

- **macOS**: unzip, move `light.app` to Applications. The app is signed ad hoc and not notarized by Apple, so the first
  launch is refused: open System Settings → Privacy & Security and choose **Open Anyway**, or run
  `xattr -dr com.apple.quarantine /Applications/light.app` once.
- **Linux**: unpack and run `./light` from its folder; it needs the Vulkan loader, X11 and ALSA
  (`sudo apt install libvulkan1 libx11-6 libxrandr2 libxinerama1 libxcursor1 libxi6 libxxf86vm1 libasound2`).
- **Windows**: unzip and run `light.exe`, which must stay beside `wgpu_native.dll`.

Each archive holds light, the wgpu-native library it draws with, and their licences. `light_<version>-checksums.txt`
lists the SHA-256 of every file and is signed with cosign (`.sigstore.json`); each archive has its SBOM beside it. The
[README](https://github.com/lantervale/light#install) says more.
