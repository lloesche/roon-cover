# TODO

This is a milestone-oriented plan for building **roon-cover**.

## Milestone 0 — Repo scaffold (today)

- [ ] Add `README.md` (architecture + goals + dev notes)
- [ ] Add this `TODO.md`
- [ ] Decide initial project layout (likely `cmd/` + `internal/`)
- [ ] Create initial `go.mod` and a placeholder `cmd/roon-cover/main.go`

## Milestone 1 — Minimal Roon extension (no rendering yet)

Goal: Connect to Roon Core, pair, subscribe to zones, and fetch cover bytes.

- [ ] Implement **discovery**
  - [ ] Find Roon Core on LAN
  - [ ] Handle multiple Cores (pick one by name/ID or prompt/flag)
- [ ] Implement **pairing**
  - [ ] Persist pairing token/keys locally (file in user config dir)
  - [ ] Reuse token on restart
- [ ] Implement **`transport.subscribe_zones`**
  - [ ] Maintain latest zone state in memory
  - [ ] Detect “now playing” changes (track identity + image key)
  - [ ] Handle zones appearing/disappearing
- [ ] Implement **image service fetch**
  - [ ] Fetch cover art bytes by image key
  - [ ] Choose a preferred image size (configurable)
  - [ ] Add timeouts, retries, and a simple in-memory/disk cache

Deliverable:
- [ ] CLI that prints the selected zone’s current track + writes latest cover to `./cover.jpg` whenever it changes (for easy validation).

## Milestone 2 — SDL renderer MVP (works on macOS)

Goal: Display the current cover art fullscreen on macOS using SDL.

- [ ] Add SDL2 rendering backend
  - [ ] Load image bytes into a texture (decode JPEG/PNG)
  - [ ] Fit strategy: letterbox (preserve aspect) vs crop-to-fill (config)
  - [ ] Fullscreen window (select display index)
- [ ] Wire Roon image updates → renderer updates
  - [ ] Debounce rapid zone updates
  - [ ] Avoid re-decoding the same image key

Deliverable:
- [ ] Run `roon-cover --zone "..."`
  - shows fullscreen cover art that updates on track change

## Milestone 3 — Effects + robustness

- [ ] Fade in/out or crossfade between covers
- [ ] Optional background blur or solid-color backdrop based on cover average color
- [ ] Smooth scaling / v-sync handling
- [ ] Better reconnect behavior
  - [ ] exponential backoff
  - [ ] re-discovery when core disappears
- [ ] Structured logging + metrics (optional)

## Milestone 4 — Raspberry Pi Zero W readiness

Goal: stable kiosk-like behavior on Pi.

- [ ] Confirm SDL2 works on Pi Zero W with kmsdrm or framebuffer
- [ ] Provide cross-compile / build instructions
- [ ] systemd service unit + autostart
- [ ] Resource tuning
  - [ ] cap image size
  - [ ] cap CPU usage in render loop
  - [ ] cache textures and reuse where possible

## Things we might be forgetting (checklist)

- [ ] **Zone identification**: zone names may change; prefer stable IDs if possible.
- [ ] **What counts as “track changed”**: handle same image for different tracks, radio streams, live content.
- [ ] **No cover art**: show placeholder (solid color / icon) and don’t spam fetch.
- [ ] **Rate limiting**: avoid fetching repeatedly during scrubs/seek/rapid updates.
- [ ] **Persistent cache**: optional disk cache to reduce network + speed up startup.
- [ ] **Graceful shutdown**: close SDL, persist state, flush logs.
- [ ] **Headless / debug mode**: print-only mode without SDL for easy SSH debugging on Pi.
- [ ] **Time sync / certificates**: if TLS is involved anywhere, ensure Pi clock is sane (might not apply depending on Roon endpoints).
- [ ] **Security**: store pairing tokens with appropriate permissions.
- [ ] **Licensing**: SDL2 licensing, any image decoding libraries.

