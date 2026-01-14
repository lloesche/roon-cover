package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"roon-cover/internal/display"
	"roon-cover/internal/roon"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newDisplayCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "display",
		Short: "Open a window and display the current cover art for the configured zone",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKiosk(cmd)
		},
	}
}

// runKiosk is the default mode: subscribe to a single configured zone and display its cover art.
func runKiosk(cmd *cobra.Command) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	l := LoggerFromContext(ctx)

	downloadToTemp := viper.GetBool("download.to_temp")

	client := roon.NewClient(roon.Config{DisplayName: "roon-cover"}, roon.WithLogger(l))

	core, err := ensureCoreAndPaired(cmd, client)
	if err != nil {
		return err
	}

	// Resolve initial active zone (by name if provided; otherwise first zone).
	zones, err := client.GetZones(ctx, core)
	if err != nil {
		return err
	}
	if len(zones) == 0 {
		return errors.New("no zones found on this Roon Core")
	}

	zoneName := strings.TrimSpace(viper.GetString("roon.zone"))
	var activeZone roon.Zone
	if zoneName == "" {
		available := make([]string, 0, len(zones))
		for _, z := range zones {
			available = append(available, z.Name)
		}
		activeZone = zones[0]
		l.Warn("No --roon-zone configured; using the first available zone. Set --roon-zone to pick a specific zone.",
			"chosen_zone", activeZone.Name,
			"available_zones", strings.Join(available, ", "),
		)
	} else {
		found := false
		available := make([]string, 0, len(zones))
		for _, z := range zones {
			available = append(available, z.Name)
			if strings.EqualFold(strings.TrimSpace(z.Name), zoneName) {
				activeZone = z
				found = true
			}
		}
		if !found {
			sort.Strings(available)
			return fmt.Errorf("unknown zone %q. Available zones: %s", zoneName, strings.Join(available, ", "))
		}
	}

	updates := make(chan display.Update, 2)
	infoCh := make(chan display.ScreenInfo, 1)
	eventCh := make(chan display.Event, 8)

	disp := &display.SDLDisplay{
		Title: "roon-cover",
	}

	windowed := viper.GetBool("display.window")
	if windowed {
		disp.Width = 800
		disp.Height = 800
		disp.Fullscreen = false
	} else {
		disp.Fullscreen = true
	}
	disp.DisplayIndex = viper.GetInt("display.index")
	disp.InfoCh = infoCh
	disp.EventCh = eventCh
	disp.FadeMS = viper.GetInt("display.fade_ms")
	disp.Ease = viper.GetString("display.ease")
	disp.FontPath = viper.GetString("display.font")
	disp.FontSize = viper.GetInt("display.font_size")
	disp.FontFadeMS = viper.GetInt("display.font_fade_ms")

	showAll := viper.GetBool("display.show_all")
	disp.ShowTitle = showAll || viper.GetBool("display.show_title")
	disp.ShowArtist = showAll || viper.GetBool("display.show_artist")
	disp.ShowAlbum = showAll || viper.GetBool("display.show_album")
	disp.ShowZone = showAll || viper.GetBool("display.show_zone")

	if disp.FadeMS < 0 {
		return fmt.Errorf("--fade-ms must be >= 0 (got %d)", disp.FadeMS)
	}
	if disp.FadeMS > 0 {
		if _, err := display.EasingByName(disp.Ease); err != nil {
			return err
		}
	}

	if disp.FontSize < 6 || disp.FontSize > 256 {
		return fmt.Errorf("--font-size must be in [6,256] (got %d)", disp.FontSize)
	}
	if disp.FontFadeMS < 0 {
		return fmt.Errorf("--font-fade-ms must be >= 0 (got %d)", disp.FontFadeMS)
	}

	errCh := make(chan error, 1)

	// Producer loop: keep a long-lived subscribe_zones stream and react to user zone switches.
	go func() {
		defer close(updates)

		type zoneRef struct {
			ID   roon.ZoneID
			Name string
		}
		buildOrder := func(zs []roon.Zone) []zoneRef {
			out := make([]zoneRef, 0, len(zs))
			for _, z := range zs {
				out = append(out, zoneRef{ID: z.ID, Name: z.Name})
			}
			sort.Slice(out, func(i, j int) bool {
				return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
			})
			return out
		}

		// Wait for the renderer to report the real output size before we start fetching.
		var square SquareSize
		select {
		case <-ctx.Done():
			return
		case info, ok := <-infoCh:
			if !ok {
				return
			}
			square.UpdateFromOutput(l, info.RenderWidth, info.RenderHeight)
		}

		var mu sync.Mutex
		zonesByID := map[roon.ZoneID]roon.Zone{}
		for _, z := range zones {
			zonesByID[z.ID] = z
		}
		order := buildOrder(zones)

		activeID := activeZone.ID

		// Per-zone state tracking (so switching zones doesn't cause constant refetch on every update).
		type zoneState struct {
			lastKey        roon.ImageKey
			lastState      roon.ZoneState
			lastDownloaded roon.ImageKey
			lastFetchedSz  int
			lastFetchAt    time.Time
			lastTitle      string
			lastArtist     string
			lastAlbum      string
		}
		stateByID := map[roon.ZoneID]*zoneState{}
		getState := func(id roon.ZoneID) *zoneState {
			if s := stateByID[id]; s != nil {
				return s
			}
			s := &zoneState{}
			stateByID[id] = s
			return s
		}

		var zlog ZoneStatusLogger

		// Refresh ordering periodically (zones can be added/removed/renamed over time).
		refreshTick := time.NewTicker(60 * time.Second)
		defer refreshTick.Stop()

		zoneUpdatesCh := make(chan []roon.Zone, 2)
		subErrCh := make(chan error, 1)
		go func() {
			err := client.SubscribeZones(ctx, core, func(update roon.ZoneUpdate) error {
				// Keep only the latest update if the consumer is slow.
				select {
				case zoneUpdatesCh <- update.Zones:
				default:
					select {
					case <-zoneUpdatesCh:
					default:
					}
					select {
					case zoneUpdatesCh <- update.Zones:
					default:
					}
				}
				return nil
			})
			select {
			case subErrCh <- err:
			default:
			}
		}()

		chooseNeighbor := func(kind display.EventKind) (roon.ZoneID, string, bool) {
			mu.Lock()
			defer mu.Unlock()

			if len(order) == 0 {
				return "", "", false
			}
			// Find current index; if missing, start at 0.
			idx := 0
			for i := range order {
				if order[i].ID == activeID {
					idx = i
					break
				}
			}
			if kind == display.EventPrevZone {
				idx = (idx - 1 + len(order)) % len(order)
			} else {
				idx = (idx + 1) % len(order)
			}
			return order[idx].ID, order[idx].Name, true
		}

		fetchAndSend := func(z roon.Zone, noFade bool) {
			// Always log zone status transitions in kiosk mode (for the active zone).
			zlog.Observe(l, z)

			np := z.NowPlaying
			if np == nil {
				sendLatest(updates, display.Update{
					Zone:       z.Name,
					State:      z.State,
					NowPlaying: nil,
				})
				return
			}

			key := np.ImageKey
			s := getState(z.ID)
			s.lastState = z.State
			s.lastKey = key
			s.lastTitle, s.lastArtist, s.lastAlbum = np.Title, np.Artist, np.Album

			// If we have an image key, we can fetch/display the cover even when paused/loading.
			if key == "" {
				sendLatest(updates, display.Update{
					Zone:       z.Name,
					State:      z.State,
					NowPlaying: np,
				})
				return
			}

			wantSize := square.Get()
			l.Debug("display: fetching cover (zone switch)", "zone", z.Name, "state", z.State, "image_key", key, "size", wantSize)
			img, mime, err := client.FetchImage(ctx, core, key, roon.ImageFetchOptions{Size: wantSize})
			if err != nil {
				l.Warn("fetch image failed", "err", err, "image_key", key)
				// Still push metadata so overlays update.
				sendLatest(updates, display.Update{
					Zone:       z.Name,
					State:      z.State,
					NowPlaying: np,
				})
				return
			}

			s.lastDownloaded = key
			s.lastFetchedSz = wantSize
			s.lastFetchAt = time.Now()
			sendLatest(updates, display.Update{
				Zone:          z.Name,
				State:         z.State,
				NowPlaying:    np,
				CoverImage:    img,
				CoverMimeType: mime,
				NoFade:        noFade,
			})
			if downloadToTemp {
				writeCoverBytesToTemp(l, z.Name, img, mime)
			}
		}

		// Kick initial render for the chosen zone.
		if z, ok := zonesByID[activeID]; ok {
			fetchAndSend(z, true)
		}

		for {
			select {
			case <-ctx.Done():
				select {
				case errCh <- nil:
				default:
				}
				return

			case info, ok := <-infoCh:
				if !ok {
					return
				}
				square.UpdateFromOutput(l, info.RenderWidth, info.RenderHeight)

			case ev := <-eventCh:
				nextID, nextName, ok := chooseNeighbor(ev.Kind)
				if !ok || nextID == "" {
					continue
				}
				mu.Lock()
				activeID = nextID
				z := zonesByID[activeID]
				mu.Unlock()

				l.Info("switching active zone", "zone", nextName)
				fetchAndSend(z, false)

			case zs := <-zoneUpdatesCh:
				mu.Lock()
				zonesByID = map[roon.ZoneID]roon.Zone{}
				for _, z := range zs {
					zonesByID[z.ID] = z
				}
				// Ensure active zone still exists; if not, fall back to the first in the current order.
				if _, ok := zonesByID[activeID]; !ok {
					if len(order) > 0 {
						activeID = order[0].ID
						l.Warn("active zone disappeared; switching to first zone", "zone", order[0].Name)
					}
				}
				z := zonesByID[activeID]
				mu.Unlock()

				// Only drive rendering off the active zone.
				if z.ID != "" {
					np := z.NowPlaying
					key := roon.ImageKey("")
					if np != nil {
						key = np.ImageKey
					}

					s := getState(z.ID)
					prevState := s.lastState
					prevKey := s.lastKey

					// Update local state after capturing previous values.
					s.lastState = z.State
					s.lastKey = key

					// Always log zone status transitions in kiosk mode.
					zlog.Observe(l, z)

					// If we don't have a now-playing (or image key), just forward metadata.
					if key == "" || np == nil {
						// Still forward metadata so overlays can clear/update on state changes.
						sendLatest(updates, display.Update{
							Zone:       z.Name,
							State:      z.State,
							NowPlaying: np,
						})
						continue
					}

					justStartedPlaying := prevState != roon.ZoneStatePlaying && z.State == roon.ZoneStatePlaying
					keyChanged := key != prevKey
					wantSize := square.Get()

					metaChanged := np.Title != s.lastTitle || np.Artist != s.lastArtist || np.Album != s.lastAlbum
					if metaChanged {
						s.lastTitle, s.lastArtist, s.lastAlbum = np.Title, np.Artist, np.Album
					}

					// If the window/display grew a lot, refetch the current cover even if key unchanged.
					// Debounced to avoid spam while resizing.
					sizeBumped := wantSize > s.lastFetchedSz+64
					canRefetchNow := time.Since(s.lastFetchAt) > 750*time.Millisecond
					refetchForResize := (key == s.lastDownloaded) && sizeBumped && canRefetchNow

					shouldFetch := ((justStartedPlaying || keyChanged) && key != s.lastDownloaded) || refetchForResize
					if !shouldFetch {
						// If metadata changed but the cover key didn't, still forward the update so overlays can refresh.
						if metaChanged {
							sendLatest(updates, display.Update{
								Zone:       z.Name,
								State:      z.State,
								NowPlaying: np,
							})
						}
						l.Debug("display: skip cover fetch", "zone", z.Name, "state", z.State, "image_key", key, "just_started", justStartedPlaying, "key_changed", keyChanged)
						continue
					}

					l.Debug("display: fetching cover", "zone", z.Name, "state", z.State, "image_key", key, "size", wantSize, "just_started", justStartedPlaying, "key_changed", keyChanged, "refetch_resize", refetchForResize)
					img, mime, err := client.FetchImage(ctx, core, key, roon.ImageFetchOptions{Size: wantSize})
					if err != nil {
						l.Warn("fetch image failed", "err", err, "image_key", key)
						continue
					}

					s.lastDownloaded = key
					s.lastFetchedSz = wantSize
					s.lastFetchAt = time.Now()
					sendLatest(updates, display.Update{
						Zone:          z.Name,
						State:         z.State,
						NowPlaying:    np,
						CoverImage:    img,
						CoverMimeType: mime,
						NoFade:        refetchForResize,
					})

					if downloadToTemp {
						// Reuse already fetched bytes (avoid double fetch).
						writeCoverBytesToTemp(l, z.Name, img, mime)
					}
				}

			case <-refreshTick.C:
				// Refresh the zone ordering (names can change, zones can come/go).
				zs, err := client.GetZones(ctx, core)
				if err != nil {
					l.Warn("zone refresh failed", "err", err)
					continue
				}
				mu.Lock()
				order = buildOrder(zs)
				mu.Unlock()

			case err := <-subErrCh:
				// If ctx cancelled, treat as graceful.
				if err == nil || errors.Is(err, context.Canceled) {
					select {
					case errCh <- nil:
					default:
					}
					return
				}
				select {
				case errCh <- err:
				default:
				}
				return
			}
		}

	}()

	// IMPORTANT: SDL/Cocoa must run on the main thread on macOS.
	dispErr := disp.Run(ctx, updates)
	cancel()

	timer := time.NewTimer(300 * time.Millisecond)
	defer timer.Stop()

	select {
	case subErr := <-errCh:
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		if dispErr != nil {
			return dispErr
		}
		return subErr
	case <-timer.C:
		return dispErr
	}
}

func sendLatest(ch chan display.Update, u display.Update) {
	select {
	case ch <- u:
		return
	default:
		// drop one and try again to keep latest semantics
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- u:
		default:
		}
	}
}
