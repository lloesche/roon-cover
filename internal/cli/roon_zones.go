package cli

import (
	"fmt"
	"strings"

	"roon-cover/internal/roon"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
			core, err := ensureCoreAndPaired(cmd, client)
			if err != nil {
				return err
			}

			zoneFilter := strings.TrimSpace(viper.GetString("roon.zone"))

			if !watch {
				zones, err := client.GetZones(cmd.Context(), core)
				if err != nil {
					return err
				}
				for _, z := range zones {
					if zoneFilter != "" && !strings.EqualFold(strings.TrimSpace(z.Name), zoneFilter) {
						continue
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s (id=%s)\n", z.Name, z.ID)
				}
				return nil
			}

			return client.SubscribeZones(cmd.Context(), core, func(update roon.ZoneUpdate) error {
				for _, z := range update.Zones {
					if zoneFilter != "" && !strings.EqualFold(strings.TrimSpace(z.Name), zoneFilter) {
						continue
					}
					if z.NowPlaying != nil {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s — %s (image_key=%s)\n", z.Name, z.NowPlaying.Artist, z.NowPlaying.Title, z.NowPlaying.ImageKey)
					} else {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: (no now playing)\n", z.Name)
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&watch, "watch", false, "watch for live zone updates (subscribe_zones)")
	return cmd
}
