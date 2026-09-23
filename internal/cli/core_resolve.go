package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"roon-cover/internal/roon"
)

func resolveCore(ctx context.Context, client *roon.Client, configuredName string) (roon.Core, error) {
	if host, portText, err := net.SplitHostPort(strings.TrimSpace(configuredName)); err == nil {
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 || host == "" {
			return roon.Core{}, errors.New("invalid Roon Core host:port")
		}
		return roon.Core{Name: configuredName, Host: host, Port: port}, nil
	}
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
		return roon.Core{}, errors.New("no roon cores discovered; check the network or use --roon-core host:port to bypass discovery")
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
