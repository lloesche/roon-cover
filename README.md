# roon-cover

Show the **Roon cover art** of the currently playing track (per zone) on an attached **HDMI display**.

Primary target: **Raspberry Pi Zero W**  
Current dev environment: **macOS**

## Goals (initial scope)

Build two main components:

- **Roon extension (Go)** that supports only:
  - discovery
  - pairing
  - `transport.subscribe_zones`
  - image service fetch (fetch cover art bytes given an image key)
- **Renderer (Go)** that displays the current cover art on a screen.

## High-level architecture

1. **Roon Core** announces itself on the network (discovery).
2. Our **extension** connects, handles **pairing**, then subscribes to zone updates via `transport.subscribe_zones`.
3. For the active zone, we detect the currently playing track and its **image key**.
4. We fetch the cover art bytes via the **Roon Image service**.
5. The **renderer** displays the image fullscreen, optionally with effects (fade in/out, crossfade, etc.).

Proposed process split (can start in one process and split later if needed):

- `cmd/roon-cover` (single binary) with internal packages:
  - `internal/roon/` (discovery, pairing, subscribe_zones, image fetch)
  - `internal/state/` (zone selection, current image tracking, debouncing)
  - `internal/render/` (SDL2 renderer; later optional DRM/KMS backend)

## Renderer choice: SDL vs DRM/KMS

We’ll start with **SDL2**:

- portable across macOS/Linux
- easiest path to effects/animations
- lets us iterate without Pi-specific graphics complexity

Later we can consider a DRM/KMS backend for minimal dependencies and faster boot, but that’s best after the Roon side is stable.

## Key product behaviors (MVP)

- Select a target **zone** by name (config flag/env var).
- When the zone is playing and cover art exists:
  - fetch image
  - display fullscreen
- When the track changes:
  - fetch new image
  - fade/crossfade to new cover (optional; can be phase 2)
- Handle reconnects:
  - Roon Core restarts
  - Wi‑Fi drops on Pi
  - pairing token refresh

## Configuration (planned)

- **Zone selection**: zone name (exact match) or a stable zone identifier if available.
- **Display**: fullscreen mode, target display index, background color.
- **Image**: preferred size (e.g. 600px/800px), cache size, fetch timeout.
- **Logging**: verbose/debug mode.

We’ll likely support:
- CLI flags (e.g. `--zone "Living Room"`)
- environment variables (for service deployment on Pi)
- optionally a small config file (TOML/YAML) later

## Development notes

### Roon extension protocol

We will implement a minimal subset of the Roon extension API:

- discovery (find Core on LAN)
- pairing (authorize our extension)
- `transport.subscribe_zones` (stream zone state changes)
- image fetch (retrieve cover art bytes)

### Testing strategy (planned)

- unit tests for:
  - zone selection logic
  - state transitions (track change → fetch → render)
  - retry/backoff logic
- integration testing:
  - run against a local Roon Core on the same network
  - add a “mock mode” that replays captured `subscribe_zones` events for renderer iteration

## Raspberry Pi Zero W considerations (planned)

- ARMv6 (Pi Zero W) constraints: CPU/memory are tight; avoid heavy deps.
- Wi‑Fi reliability: robust reconnect and backoff.
- Startup: systemd service, autostart on boot.
- SDL2 on Pi: ensure we can use the right video driver (kmsdrm or framebuffer) and disable unnecessary compositing.
- Image decoding: prefer efficient decoders and cache scaled textures.

## Roadmap

See `TODO.md` for a milestone-based plan.

