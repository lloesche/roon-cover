package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

	zoneName := strings.TrimSpace(viper.GetString("roon.zone"))
	if zoneName == "" {
		// If no zone is configured, pick the first one and explain what happened.
		zones, err := client.GetZones(ctx, core)
		if err != nil {
			return err
		}
		if len(zones) == 0 {
			return errors.New("no zones found on this Roon Core")
		}

		available := make([]string, 0, len(zones))
		for _, z := range zones {
			available = append(available, z.Name)
		}
		zoneName = zones[0].Name
		l.Warn("No --roon-zone configured; using the first available zone. Set --roon-zone to pick a specific zone.",
			"chosen_zone", zoneName,
			"available_zones", strings.Join(available, ", "),
		)
	} else {
		// Validate zone exists before we open the window and subscribe.
		if err := validateZoneExists(ctx, client, core, zoneName); err != nil {
			return err
		}
	}

	updates := make(chan display.Update, 2)
	infoCh := make(chan display.ScreenInfo, 1)

	disp := &display.SDLDisplay{
		Title: fmt.Sprintf("roon-cover — %s", zoneName),
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
	disp.FadeMS = viper.GetInt("display.fade_ms")
	disp.Ease = viper.GetString("display.ease")
	disp.FontPath = viper.GetString("display.font")
	disp.FontSize = viper.GetInt("display.font_size")
	disp.FontFadeMS = viper.GetInt("display.font_fade_ms")

	showAll := viper.GetBool("display.show_all")
	disp.ShowTitle = showAll || viper.GetBool("display.show_title")
	disp.ShowArtist = showAll || viper.GetBool("display.show_artist")
	disp.ShowAlbum = showAll || viper.GetBool("display.show_album")

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

	// Producer loop: subscribe zones, fetch covers on change, send updates.
	go func() {
		defer close(updates)

		var square SquareSize
		// Wait for the renderer to report the real output size before we start fetching.
		// This avoids an initial "guess" fetch (e.g. 800px) followed by an immediate refetch.
		select {
		case <-ctx.Done():
			return
		case info, ok := <-infoCh:
			if !ok {
				return
			}
			square.UpdateFromOutput(l, info.RenderWidth, info.RenderHeight)
		}

		// Keep SquareSize up to date with SDL output size changes.
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case info, ok := <-infoCh:
					if !ok {
						return
					}
					square.UpdateFromOutput(l, info.RenderWidth, info.RenderHeight)
				}
			}
		}()

		var zlog ZoneStatusLogger
		var lastKey roon.ImageKey
		var lastState roon.ZoneState
		var lastDownloaded roon.ImageKey
		var lastFetchedSize int
		var lastFetchAt time.Time
		var lastTitle, lastArtist, lastAlbum string

		err := client.SubscribeZones(ctx, core, func(update roon.ZoneUpdate) error {
			for _, z := range update.Zones {
				if !strings.EqualFold(strings.TrimSpace(z.Name), zoneName) {
					continue
				}

				np := z.NowPlaying
				key := roon.ImageKey("")
				if np != nil {
					key = np.ImageKey
				}

				prevState := lastState
				prevKey := lastKey

				// Update local state after capturing previous values.
				lastState = z.State
				lastKey = key

				// Always log zone status transitions in kiosk mode.
				zlog.Observe(l, z)

				// Only fetch/display covers when zone is actually playing.
				if z.State != roon.ZoneStatePlaying || key == "" || np == nil {
					return nil
				}

				justStartedPlaying := prevState != roon.ZoneStatePlaying && z.State == roon.ZoneStatePlaying
				keyChanged := key != prevKey
				wantSize := square.Get()

				metaChanged := np.Title != lastTitle || np.Artist != lastArtist || np.Album != lastAlbum
				if metaChanged {
					lastTitle, lastArtist, lastAlbum = np.Title, np.Artist, np.Album
				}

				// If the window/display grew a lot, refetch the current cover even if key unchanged.
				// Debounced to avoid spam while resizing.
				sizeBumped := wantSize > lastFetchedSize+64
				canRefetchNow := time.Since(lastFetchAt) > 750*time.Millisecond
				refetchForResize := (key == lastDownloaded) && sizeBumped && canRefetchNow

				shouldFetch := ((justStartedPlaying || keyChanged) && key != lastDownloaded) || refetchForResize
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
					return nil
				}

				l.Debug("display: fetching cover", "zone", z.Name, "state", z.State, "image_key", key, "size", wantSize, "just_started", justStartedPlaying, "key_changed", keyChanged, "refetch_resize", refetchForResize)
				img, mime, err := client.FetchImage(ctx, core, key, roon.ImageFetchOptions{Size: wantSize})
				if err != nil {
					l.Warn("fetch image failed", "err", err, "image_key", key)
					return nil
				}

				lastDownloaded = key
				lastFetchedSize = wantSize
				lastFetchAt = time.Now()
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

				return nil
			}
			return nil
		})

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
