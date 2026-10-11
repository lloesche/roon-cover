package cli

import (
	"fmt"
	"strings"

	"roon-cover/internal/roon"

	"github.com/spf13/cobra"
)

func newRoonZonesCmd() *cobra.Command {
	var watch bool

	cmd := &cobra.Command{
		Use:   "zones",
		Short: "List zones (or watch zones with --watch)",
		RunE: func(cmd *cobra.Command, args []string) error {
			l := LoggerFromContext(cmd.Context())
			client := roon.NewClient(roon.Config{
				DisplayName: "roon-cover",
			}, roon.WithLogger(l))

			// Resolve core (configured vs discovery) and ensure we're paired.
			session, err := openCoreSession(cmd.Context(), cmd, client, nil)
			if err != nil {
				return err
			}

			defer session.Close()
			core := session.Core
			zoneFilter := strings.TrimSpace(configFor(cmd).GetString("roon.zone"))
			downloadToTemp := configFor(cmd).GetBool("download.to_temp")
			var lastKey roon.ImageKey
			var lastDownloaded roon.ImageKey
			var lastState roon.ZoneState

			if !watch {
				for _, z := range session.Zones {
					if zoneFilter != "" && !strings.EqualFold(strings.TrimSpace(z.Name), zoneFilter) {
						continue
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s (id=%s)\n", z.Name, z.ID)
				}
				return nil
			}

			observe := func(update roon.ZoneUpdate) error {
				for _, z := range update.Zones {
					if zoneFilter != "" && !strings.EqualFold(strings.TrimSpace(z.Name), zoneFilter) {
						continue
					}
					if z.NowPlaying != nil {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s [%s]: %s — %s (image_key=%s)\n", z.Name, z.State, z.NowPlaying.Artist, z.NowPlaying.Title, z.NowPlaying.ImageKey)

						if downloadToTemp {
							key := z.NowPlaying.ImageKey
							if key != "" {
								justStartedPlaying := lastState != roon.ZoneStatePlaying && z.State == roon.ZoneStatePlaying
								keyChanged := key != lastKey
								shouldDownload := (z.State == roon.ZoneStatePlaying) && (justStartedPlaying || keyChanged) && key != lastDownloaded
								if shouldDownload {
									lastDownloaded = key
									maybeDownloadCoverToTemp(cmd.Context(), l, client, core, z)
								}
								lastKey = key
							}
							lastState = z.State
						}
					} else {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s [%s]: (no now playing)\n", z.Name, z.State)
						lastState = z.State
					}
				}
				return nil
			}
			if err := observe(roon.ZoneUpdate{Zones: session.Zones}); err != nil {
				return err
			}
			for {
				select {
				case <-cmd.Context().Done():
					return nil
				case <-session.Done:
					return fmt.Errorf("roon: zone subscription disconnected")
				case update := <-session.Updates:
					if err := observe(update); err != nil {
						return err
					}
				}
			}
		},
	}
	cmd.Flags().BoolVar(&watch, "watch", false, "watch for live zone updates (subscribe_zones)")
	return cmd
}
