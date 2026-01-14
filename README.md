# roon-cover 🎛️🖼️

Turn any screen into a **“Now Playing” cover art display** for your **Roon** zone.

- 🖥️ **Fullscreen cover art** (perfect for an HDMI display / wall screen)
- ✨ **Smooth transitions** (crossfade + easing)
- 📝 Optional **title / artist / album** overlay with a soft shadow
- 🪄 Works great as a simple “kiosk mode” companion for a listening room

> Tip: if you just want to try it out, start in windowed mode first.

---

## What it does ✅

`roon-cover` connects to your Roon Core, watches a selected zone, and shows the current track’s cover art (plus optional text) in a clean, distraction-free window.

On first run, Roon will ask you to **authorize / pair** the extension.

---

## Quick start 🚀

### 1) Install

If you have Go installed:

```bash
go install ./cmd/roon-cover
```

Or from the repo root:

```bash
go run ./cmd/roon-cover --help
```

### 2) Run it (windowed)

```bash
roon-cover --roon-zone "Living Room" --window
```

### 3) Run it (fullscreen)

```bash
roon-cover --roon-zone "Living Room"
```

---

## Usage examples ✨

### Pick a specific Roon Core

```bash
roon-cover --roon-core "My Roon Core" --roon-zone "Living Room"
```

### Choose a display (fullscreen)

```bash
roon-cover --roon-zone "Living Room" --display 1
```

### Cover crossfade + easing

```bash
roon-cover --roon-zone "Living Room" --fade-ms 500 --ease in-out-sine
```

### Show text overlays

```bash
roon-cover --roon-zone "Living Room" --show-all
```

### Customize font + faster text transitions

```bash
roon-cover --roon-zone "Living Room" --show-all --font "/path/to/font.ttf" --font-size 28 --font-fade-ms 200
```

---

## Common options 🎚️

Run `roon-cover --help` for the full list. The most useful ones:

- **Zone / core**
  - `--roon-zone`: the zone to follow (optional; if omitted, the first available zone is used)
  - `--roon-core`: optional, helps if you have multiple cores
- **Display**
  - `--window`: run windowed (handy for testing)
  - `--display`: choose which display to use (0-based)
- **Transitions**
  - `--fade-ms`: cover crossfade duration (0 disables)
  - `--ease`: easing function name (e.g. `in-sine`, `out-quad`, `in-out-sine`, `out-elastic`, `out-bounce`)
  - `--font-fade-ms`: text transition duration (0 disables; independent from cover fade)
- **Text overlay**
  - `--show-title`, `--show-artist`, `--show-album`, `--show-all`
  - `--font`, `--font-size`

---

## Troubleshooting 🧰

### “I don’t see anything / it closes immediately”

- Make sure the zone name is correct: `--roon-zone "…"`
- Try windowed mode first: `--window`
- Run with logs: `--log-level debug`

### “It can’t find my Core”

- Put the machine running `roon-cover` on the **same network** as your Roon Core.
- If you have multiple Cores, specify one with `--roon-core`.

---

## License 📄

MIT (see `LICENSE` if present).

