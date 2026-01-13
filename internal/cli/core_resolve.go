package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"roon-cover/internal/roon"
)

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
