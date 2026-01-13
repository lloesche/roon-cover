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
		Short: "Open an 800x800 window and display the current cover art for the configured zone",
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

	zoneName := strings.TrimSpace(viper.GetString("roon.zone"))
	if zoneName == "" {
		return errors.New("missing zone: set --roon-zone / ROON_COVER_ROON_ZONE / config roon.zone")
	}

	downloadToTemp := viper.GetBool("download.to_temp")

	client := roon.NewClient(roon.Config{DisplayName: "roon-cover"}, roon.WithLogger(l))

	core, err := ensureCoreAndPaired(cmd, client)
	if err != nil {
		return err
	}

	// Validate zone exists before we open the window and subscribe.
	if err := validateZoneExists(ctx, client, core, zoneName); err != nil {
		return err
	}

	updates := make(chan display.Update, 2)

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

	errCh := make(chan error, 1)

	// Producer loop: subscribe zones, fetch covers on change, send updates.
	go func() {
		defer close(updates)

		var zlog ZoneStatusLogger
		var lastKey roon.ImageKey
		var lastState roon.ZoneState
		var lastDownloaded roon.ImageKey

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
				shouldFetch := (justStartedPlaying || keyChanged) && key != lastDownloaded
				if !shouldFetch {
					l.Debug("display: skip cover fetch", "zone", z.Name, "state", z.State, "image_key", key, "just_started", justStartedPlaying, "key_changed", keyChanged)
					return nil
				}

				l.Debug("display: fetching cover", "zone", z.Name, "state", z.State, "image_key", key, "just_started", justStartedPlaying, "key_changed", keyChanged)
				img, mime, err := client.FetchImage(ctx, core, key, roon.ImageFetchOptions{Size: 800})
				if err != nil {
					l.Warn("fetch image failed", "err", err, "image_key", key)
					return nil
				}

				lastDownloaded = key
				sendLatest(updates, display.Update{
					Zone:          z.Name,
					State:         z.State,
					NowPlaying:    np,
					CoverImage:    img,
					CoverMimeType: mime,
				})

				if downloadToTemp {
					// For now, reuse the existing helper (it re-fetches); we'll optimize to avoid double fetch later.
					maybeDownloadCoverToTemp(ctx, l, client, core, z)
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

	select {
	case subErr := <-errCh:
		if dispErr != nil {
			return dispErr
		}
		return subErr
	case <-time.After(300 * time.Millisecond):
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
