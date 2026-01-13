package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"roon-cover/internal/roon"
)

func validateZoneExists(ctx context.Context, client *roon.Client, core roon.Core, zoneName string) error {
	zoneName = strings.TrimSpace(zoneName)
	if zoneName == "" {
		return errors.New("missing zone: set --roon-zone / ROON_COVER_ROON_ZONE / config roon.zone")
	}

	zones, err := client.GetZones(ctx, core)
	if err != nil {
		return err
	}

	available := make([]string, 0, len(zones))
	for _, z := range zones {
		available = append(available, z.Name)
		if strings.EqualFold(strings.TrimSpace(z.Name), zoneName) {
			return nil
		}
	}

	sort.Strings(available)
	return fmt.Errorf("unknown zone %q. Available zones: %s", zoneName, strings.Join(available, ", "))
}
