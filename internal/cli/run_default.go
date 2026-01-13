package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"roon-cover/internal/roon"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func runDefault(cmd *cobra.Command) error {
	ctx := cmd.Context()
	l := LoggerFromContext(ctx)

	coreName := strings.TrimSpace(viper.GetString("roon.core"))
	zoneName := strings.TrimSpace(viper.GetString("roon.zone"))
	if zoneName == "" {
		return errors.New("missing zone: set --roon-zone or ROON_COVER_ROON_ZONE (or config roon.zone)")
	}

	client := roon.NewClient(roon.Config{
		DisplayName: "roon-cover",
	}, roon.WithLogger(l))

	core, err := resolveCore(ctx, client, coreName)
	if err != nil {
		return err
	}

	store, err := roon.NewFileCredentialStore("roon-cover")
	if err != nil {
		return err
	}

	if creds, ok, err := store.Load(ctx, core); err != nil {
		return err
	} else if ok {
		l.Info("loaded stored credentials", "core", core.Name)
		// If we have never been paired (or core id changed), re-run pairing flow.
		if creds.PairedCoreID == "" || (core.ID != "" && creds.PairedCoreID != core.ID) {
			l.Info("stored credentials not paired for this core; pairing required", "core", core.Name)
			creds, err := client.Pair(ctx, core)
			if err != nil {
				return err
			}
			if err := store.Save(ctx, core, creds); err != nil {
				return err
			}
			l.Info("paired and saved credentials", "core", core.Name)
		}
	} else {
		l.Info("no stored credentials, pairing required", "core", core.Name)
		creds, err := client.Pair(ctx, core)
		if err != nil {
			return err
		}
		if err := store.Save(ctx, core, creds); err != nil {
			return err
		}
		l.Info("paired and saved credentials", "core", core.Name)
	}

	// Validate zone exists (avoid subscribing to a typo / flag-value like "-h").
	if err := validateZoneExists(ctx, client, core, zoneName); err != nil {
		return err
	}

	l.Info("subscribing to zone", "zone", zoneName)
	downloadToTemp := viper.GetBool("download.to_temp")
	var lastKey roon.ImageKey
	var lastDownloaded roon.ImageKey
	var lastState roon.ZoneState
	var zlog ZoneStatusLogger

	return client.SubscribeZones(ctx, core, func(update roon.ZoneUpdate) error {

		// For now, we just filter by name and print now playing summaries.
		// Later: detect track/image_key changes and trigger image fetch + render.
		for _, z := range update.Zones {
			if !strings.EqualFold(strings.TrimSpace(z.Name), zoneName) {
				continue
			}

			if z.NowPlaying == nil {
				zlog.Observe(l, z)
				return nil
			}

			zlog.Observe(l, z)

			// Gate downloads on playback state:
			// - Only download when state is playing (or when transitioning into playing).
			// - Avoid repeated downloads for the same key.
			if downloadToTemp {
				key := z.NowPlaying.ImageKey
				if key != "" {
					justStartedPlaying := lastState != roon.ZoneStatePlaying && z.State == roon.ZoneStatePlaying
					keyChanged := key != lastKey
					shouldDownload := (z.State == roon.ZoneStatePlaying) && (justStartedPlaying || keyChanged) && key != lastDownloaded
					if shouldDownload {
						lastDownloaded = key
						maybeDownloadCoverToTemp(ctx, l, client, core, z)
					}
					lastKey = key
				}
				lastState = z.State
			}
			return nil
		}
		return nil
	})
}

func resolveCore(ctx context.Context, client *roon.Client, configuredName string) (roon.Core, error) {
	cores, err := client.Discover(ctx)
	if err != nil {
		return roon.Core{}, err
	}

	name := strings.TrimSpace(configuredName)
	if name != "" {
		matches := make([]roon.Core, 0, 2)
		for _, c := range cores {
			if strings.EqualFold(strings.TrimSpace(c.Name), name) {
				matches = append(matches, c)
			}
		}
		switch len(matches) {
		case 0:
			return roon.Core{}, fmt.Errorf("roon core %q not found via discovery", name)
		case 1:
			return matches[0], nil
		default:
			var b strings.Builder
			b.WriteString("multiple discovered roon cores match --roon-core; be more specific:\n")
			for _, c := range matches {
				_, _ = fmt.Fprintf(&b, "- %s (%s:%d) id=%s\n", c.Name, c.Host, c.Port, c.ID)
			}
			return roon.Core{}, errors.New(b.String())
		}
	}

	switch len(cores) {
	case 0:
		return roon.Core{}, errors.New("no roon cores discovered; set --roon-core or ROON_COVER_ROON_CORE (or config roon.core)")
	case 1:
		return cores[0], nil
	default:
		var b strings.Builder
		b.WriteString("multiple roon cores discovered; specify one via --roon-core / ROON_COVER_ROON_CORE (or config roon.core):\n")
		for _, c := range cores {
			_, _ = fmt.Fprintf(&b, "- %s (%s:%d) id=%s\n", c.Name, c.Host, c.Port, c.ID)
		}
		return roon.Core{}, errors.New(b.String())
	}
}
