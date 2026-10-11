# roon-cover

A fullscreen Roon now-playing display with smooth cover crossfades and optional song, album, artist, and zone labels. Built with Ebitengine and go-text for Windows, macOS, and Linux.

## Run

Download a package for your OS and CPU from [Releases](https://github.com/lloesche/roon-cover/releases), or run from source with Go 1.27.1 or newer:

```sh
go run ./cmd/roon-cover --window --show-all
go run ./cmd/roon-cover --show-all --roon-zone "Bedroom"
```

Omit `--window` for fullscreen. `--display 1` chooses the second desktop monitor (indices start at zero). Left/Right cycle zones and briefly show the selected zone for one second, even with text overlays disabled; Q or Escape quits. Without `--roon-zone`, startup chooses the first playing zone in name order, or the first available zone if none is playing. Later playback changes do not switch zones. Pause and stop blank the display immediately.

On first use, enable **roon-cover** in **Roon → Settings → Extensions** from your phone, tablet, or computer. The display shows discovery, connection, and zone selection as they happen, then fades into the cover. Pairing needs no input on the display itself. Credentials live in the OS user configuration directory under `roon-cover/credentials.json`.

Connection failures retry automatically. Repeated failures trigger rediscovery of the selected server. `--roon-core` accepts a discovered server name or an explicit `host:port`; an explicit address stays fixed. Use your server's actual API port; IPv6 uses `[address]:port`.

```sh
go run ./cmd/roon-cover roon discover
go run ./cmd/roon-cover roon zones
```

The zones diagnostic requires prior authorization. On Windows, `scripts/run.ps1` selects the official Go installation if another `go` executable shadows it.

## Build

```powershell
# Windows
.\scripts\build.ps1
.\build\roon-cover.exe --window --show-all
```

```sh
# macOS / Linux
sh scripts/build.sh
./build/roon-cover --show-all
```

Alternatively, use `go build -o build/ ./cmd/roon-cover`. Builds use `CGO_ENABLED=0` and require no C compiler. A working graphics driver is required at runtime. Linux desktops also need graphics libraries; see [Ebitengine's platform instructions](https://ebitengine.org/en/documents/install.html).

### Raspberry Pi without a desktop

The console framebuffer backend uses `/dev/fb0`, needs a compatible EGL driver, and exposes one monitor. A framebuffer device does not necessarily map to one HDMI connector, so `--display 1` cannot select the second HDMI output in this mode. Desktop X11/XWayland sessions support normal monitor selection.

Linux ARM64 builds are checked in CI, but console rendering, second-HDMI selection, and display power control still need testing on the Pi. Keep the existing working Pi executable until that deployment is verified; direct DRM/KMS connector selection needs additional backend work.

## Appearance and configuration

`--show-all` enables every label; `--show-title`, `--show-album`, `--show-artist`, and `--show-zone` enable individual labels. The title appears above “On [album]” and “Performed by [artist]” at the bottom left. The zone label appears briefly at the top. `--fade-ms` sets both cover and metadata transition time (default 500 ms); zero disables fades. Both wait for the new artwork before transitioning. `--ease` selects a monotonic easing curve.

Inter Regular is embedded under the [SIL Open Font License](internal/display/fonts/LICENSE.txt). `--font` accepts a custom TTF/OTF/TTC file; `--font-size` uses logical pixels and scales with display DPI. System fonts supply fallback for scripts and emoji. Install Noto fonts, including CJK and emoji, for broad Linux coverage. Go-text provides shaping and bidirectional layout; available fonts determine glyph coverage. `roon-cover licenses` shows the embedded font license.

Settings resolve in this order: flags, environment variables, configuration file, defaults. Pass `--config path/to/config.yaml`:

```yaml
roon:
  zone: Bedroom
display:
  window: true
  show_all: true
  font_size: 28
  fade_ms: 500
log:
  level: info
  format: text
```

Examples: `ROON_COVER_ROON_ZONE`, `ROON_COVER_DISPLAY_SHOW_ALL`, `ROON_COVER_LOG_LEVEL`. Font paths in a config file are relative to that file; flag/environment paths are relative to the working directory. `--help` lists all options.

`--display-sleep-idle-sec 60` runs `--display-sleep-cmd` after a minute without playback; playback runs `--display-wake-cmd`. Supply commands appropriate for your system. Commands have a ten-second timeout, bounded output, and retry backoff. Shutdown attempts to wake a display this process may have put to sleep. Pausing still blanks immediately.

## Builds and releases

GitHub Actions tests and packages Windows, macOS, and Linux for x86-64 (`amd64`) and ARM64 (`arm64`). Packages include the executable, this README, and the Inter license.

- **Pull requests:** tests and builds, without uploaded artifacts.
- **Main:** downloadable packages and checksums in **Actions → workflow run → Artifacts**, retained for 90 days.
- **Version tags:** pushing `v0.0.1` publishes a [GitHub Release](https://github.com/lloesche/roon-cover/releases) with six packages, `SHA256SUMS`, and release notes. Tags such as `v0.0.1-rc.1` produce prereleases.

Packages are unsigned; macOS notarization is not configured. `roon-cover version` reports the build version, commit, Go version, and renderer.

## Development

Run `go test ./...` and `go vet ./...`. CI also runs Staticcheck. For graphics tests, set `ROON_COVER_GPU_TESTS=1` and run `go test ./internal/display -count=1` in a graphics session. Linux CI uses Xvfb/Mesa.

Run `go test -race ./internal/app ./internal/roon ./internal/displaypower` for concurrency checks. The race detector requires Cgo and a C compiler; application builds do not.

