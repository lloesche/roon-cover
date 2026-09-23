# roon-cover

A Roon now-playing display with fullscreen artwork, smooth crossfades, optional multilingual metadata, and keyboard zone selection.

The renderer uses **Ebitengine 2.10** and **go-text**. Desktop builds need Go 1.25 or newer and a working graphics driver. SDL, Pango, Cairo, a C compiler, and the old `-tags sdl` option are no longer required. Dependencies are pinned in `go.mod` and `go.sum`.

## Run from source

From the repository root on Windows, macOS, or Linux:

```sh
go run ./cmd/roon-cover --window --show-all
```

The first zone in name order is selected when `--roon-zone` is omitted. Left/Right cycle zones; Q or Escape quits. Selection survives updates, and removing the selected zone selects the next available default. Pausing blanks artwork and metadata immediately. `--show-zone` displays the zone label briefly at the top.

To watch a particular zone:

```sh
go run ./cmd/roon-cover --window --show-all --roon-zone "Dialysis"
```

Omit `--window` for fullscreen. On a graphical desktop, choose a monitor with `--display 1` (zero-based). Invalid monitor indices return an error.

On Windows, if MSYS2's `go` shadows the official installation or reports a missing GOROOT, use:

```powershell
.\scripts\run.ps1 --window --show-all
```

The helper prefers `C:\Program Files\Go\bin\go.exe`. Do not set GOROOT to the repository.

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

Or use `go build -o build/ ./cmd/roon-cover` directly. The executable does not need the old SDL/Pango DLL bundle. `roon-cover version` reports the commit, dirty status, Go version, and renderer version.

Cross-compilation works with `CGO_ENABLED=0`, for example:

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/roon-cover-linux-arm64 ./cmd/roon-cover
```

Linux still needs graphics runtime libraries. For an existing Ubuntu/Debian desktop, typical packages are `libx11-6 libgl1 libglx-mesa0 libxcursor1 libxi6 libxinerama1 libxrandr2 libxrender1 libxext6`. Follow [Ebitengine's platform instructions](https://ebitengine.org/en/documents/install.html) for your distribution and driver.

### Raspberry Pi without a desktop

**The former SDL console/second-HDMI deployment is not a verified drop-in migration.** Ebitengine 2.10.3's framebuffer backend opens `/dev/fb0`, requires a compatible EGL driver, and exposes one monitor. A framebuffer device is not necessarily one physical HDMI connector; `--display 1` cannot select a second connector through this backend.

Linux ARM64 compilation is checked, but that does not validate the Pi graphics driver, direct HDMI selection, or power controls. Keep the previous working executable for that deployment until the actual Pi is tested. A graphical X11/XWayland session offers the normal desktop monitor path; direct DRM/KMS connector selection needs additional backend work. No desktop or boot configuration is changed by this project.

## Roon connection

Authorize the extension in Roon on first use. Credentials are stored under the OS user configuration directory in `roon-cover/credentials.json`; do not commit that file.

```sh
go run ./cmd/roon-cover roon discover
go run ./cmd/roon-cover roon pair --roon-core "My Core"
go run ./cmd/roon-cover roon zones
go run ./cmd/roon-cover --roon-core 192.168.1.10:9330 --window
```

`--roon-core` accepts a discovered name or an explicit `host:port` to bypass discovery. Use your Core's actual HTTP/API port; the example port is not a universal default. IPv6 addresses use `[address]:port`.

Dropped zone subscriptions blank the display and retry with a bounded delay. Artwork downloads run separately from zone selection, cancel obsolete requests, and retry failures. The app retains one decoded cover, with encoded data and decoded pixel limits.

## Typography and transitions

`--show-title`, `--show-artist`, `--show-album`, `--show-zone`, or `--show-all` enable overlays. `--font` selects a TTF/OTF/TTC file; installed fonts supply fallback. A small bundled font covers basic text if system font discovery fails. For broad coverage on Linux, install Noto fonts, including CJK and emoji packages.

Go-text handles script shaping, bidirectional layout, and cluster-aware ellipsis. Available fonts still determine glyph coverage. Bitmap/SVG and COLRv0 color glyphs are supported; COLRv1 currently falls back to a monochrome outline when available. This is not a guarantee of identical typography or complete font coverage across operating systems.

```sh
go run ./cmd/roon-cover --show-all --font-size 28 --fade-ms 500 --ease in-out-sine
```

Font size is in logical pixels and scales with display DPI. `--fade-ms` controls both cover and text transitions, defaulting to 500 ms. They start together when the new artwork is ready and finish together; the outgoing cover and text remain visible while it downloads. Zero disables both animations. The separate `--font-fade-ms` option has been removed. Opacity fades accept monotonic easing curves; elastic and bounce curves are rejected. Interrupted cover fades preserve the currently visible blend. Static scenes do not continuously redraw.

## Configuration

Precedence: explicit flags, environment variables, configuration file, defaults. Configuration is owned by each command instance. Use `--config path/to/config.yaml`:

```yaml
roon:
  zone: Dialysis
display:
  window: true
  show_all: true
  font_size: 28
  fade_ms: 500
log:
  level: info
  format: text
```

A font path supplied in the file is relative to that file; a command-line or environment font path is relative to the working directory. Environment examples: `ROON_COVER_ROON_ZONE`, `ROON_COVER_DISPLAY_SHOW_ALL`, `ROON_COVER_LOG_LEVEL`, `ROON_COVER_DEBUG_PPROF_ADDR`. `--help` lists all flags.

## Optional display power control

`--display-sleep-idle-sec 60` runs `--display-sleep-cmd` after a minute without playback; playback runs `--display-wake-cmd`. Commands are explicit shell commands supplied by you for the target system. No HDMI utility is assumed or installed.

Commands have a ten-second timeout, bounded diagnostic output, and process-tree cleanup. Failures retry with backoff. If this process may have put the display to sleep, shutdown attempts to wake it. Blanking on pause happens immediately and independently of this power-saving delay.

## Development checks

```sh
go test ./...
go vet ./...
```

For actual GPU pixel checks, set `ROON_COVER_GPU_TESTS=1` and run `go test ./internal/display -count=1`. A graphics session is required. Linux CI uses Xvfb/Mesa; local Windows verification uses the actual renderer. Set `ROON_COVER_TEST_ARTIFACTS` to an existing directory to save the multilingual sample image.

`go test -race ./internal/app ./internal/roon ./internal/displaypower` exercises concurrency. Go's race detector itself needs Cgo/a C compiler; the application build does not. CI checks Windows, macOS, Linux AMD64, and Linux ARM64. Hardware-specific Pi validation remains separate.

