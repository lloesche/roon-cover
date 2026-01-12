package roon

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type transportSubscribeZonesResponse struct {
	Zones        []zoneRaw `json:"zones"`
	ZonesAdded   []zoneRaw `json:"zones_added"`
	ZonesChanged []zoneRaw `json:"zones_changed"`
	ZonesRemoved []zoneID  `json:"zones_removed"`
}

type zoneID struct {
	ZoneID string `json:"zone_id"`
}

type zoneRaw struct {
	ZoneID      string         `json:"zone_id"`
	DisplayName string         `json:"display_name"`
	State       string         `json:"state"`
	NowPlaying  *nowPlayingRaw `json:"now_playing"`
}

type nowPlayingRaw struct {
	ImageKey  string      `json:"image_key"`
	OneLine   *lineBlock1 `json:"one_line"`
	TwoLine   *lineBlock2 `json:"two_line"`
	ThreeLine *lineBlock3 `json:"three_line"`
}

type lineBlock1 struct {
	Line1 string `json:"line1"`
}
type lineBlock2 struct {
	Line1 string `json:"line1"`
	Line2 string `json:"line2"`
}
type lineBlock3 struct {
	Line1 string `json:"line1"`
	Line2 string `json:"line2"`
	Line3 string `json:"line3"`
}

func (c *Client) SubscribeZones(ctx context.Context, core Core, onUpdate func(ZoneUpdate) error) error {
	// SubscribeZones expects the caller to keep ctx alive.
	// For now, we create a new connection and keep it open until ctx is canceled.
	// In the kiosk app, this will be the main long-lived connection.

	credsStore, _ := NewFileCredentialStore("roon-cover")
	var creds Credentials
	if credsStore != nil {
		if loaded, ok, err := credsStore.Load(ctx, core); err == nil && ok {
			creds = loaded
		}
	}

	s, err := c.connectAndRegister(ctx, core, &creds)
	if err != nil {
		return err
	}
	// Keep connection alive until ctx ends.
	defer s.conn.Close()

	// Persist any updated registry token.
	if credsStore != nil && creds.RegistryToken != "" {
		_ = credsStore.Save(ctx, core, creds)
	}

	// Subscribe.
	subKey := 1
	type subscribeArgs struct {
		SubscriptionKey int `json:"subscription_key"`
	}

	zonesByID := map[string]Zone{}
	var mu sync.Mutex

	err = s.conn.Subscribe(ctx, svcTransport+"/subscribe_zones", subscribeArgs{SubscriptionKey: subKey}, func(f *mooFrame) error {
		// Expect CONTINUE frames with ResponseName Subscribed/Changed/Unsubscribed.
		if f.ResponseName == "" {
			return nil
		}

		var payload transportSubscribeZonesResponse
		if len(f.BodyRaw) > 0 {
			if err := jsonUnmarshal(f.BodyRaw, &payload); err != nil {
				return err
			}
		}

		mu.Lock()
		defer mu.Unlock()

		switch f.ResponseName {
		case "Subscribed":
			zonesByID = map[string]Zone{}
			for _, z := range payload.Zones {
				zonesByID[z.ZoneID] = mapZone(z)
			}
		case "Changed":
			for _, zr := range payload.ZonesRemoved {
				delete(zonesByID, zr.ZoneID)
			}
			for _, z := range payload.ZonesAdded {
				zonesByID[z.ZoneID] = mapZone(z)
			}
			for _, z := range payload.ZonesChanged {
				zonesByID[z.ZoneID] = mapZone(z)
			}
		case "Unsubscribed":
			zonesByID = map[string]Zone{}
		default:
			// Could be an error; bubble it up.
			if f.ResponseName != "Success" {
				return fmt.Errorf("subscribe_zones: %s", f.ResponseName)
			}
		}

		if onUpdate != nil {
			out := make([]Zone, 0, len(zonesByID))
			for _, z := range zonesByID {
				out = append(out, z)
			}
			return onUpdate(ZoneUpdate{Zones: out})
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Block until canceled.
	<-ctx.Done()
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil
	}
	return ctx.Err()
}

func (c *Client) GetZones(ctx context.Context, core Core) ([]Zone, error) {
	credsStore, _ := NewFileCredentialStore("roon-cover")
	var creds Credentials
	if credsStore != nil {
		if loaded, ok, err := credsStore.Load(ctx, core); err == nil && ok {
			creds = loaded
		}
	}

	s, err := c.connectAndRegister(ctx, core, &creds)
	if err != nil {
		return nil, err
	}
	defer s.conn.Close()

	// Persist any updated registry token.
	if credsStore != nil && creds.RegistryToken != "" {
		_ = credsStore.Save(ctx, core, creds)
	}

	type resp struct {
		Zones []zoneRaw `json:"zones"`
	}
	var out resp

	c.log.Debug("calling transport/get_zones")
	err = s.conn.Call(ctx, svcTransport+"/get_zones", nil, func(f *mooFrame) error {
		if f.ResponseName != "Success" {
			return fmt.Errorf("get_zones: %s", f.ResponseName)
		}
		if len(f.BodyRaw) == 0 {
			return nil
		}
		return jsonUnmarshal(f.BodyRaw, &out)
	})
	if err != nil {
		return nil, err
	}

	zones := make([]Zone, 0, len(out.Zones))
	for _, z := range out.Zones {
		zones = append(zones, mapZone(z))
	}
	return zones, nil
}

func mapZone(z zoneRaw) Zone {
	out := Zone{
		ID:    ZoneID(z.ZoneID),
		Name:  z.DisplayName,
		State: ZoneState(z.State),
	}

	if z.NowPlaying == nil {
		return out
	}

	np := &NowPlaying{
		ImageKey: ImageKey(z.NowPlaying.ImageKey),
	}

	// Best-effort mapping. For music tracks, three_line tends to be:
	// line1=Title, line2=Artist, line3=Album.
	if z.NowPlaying.ThreeLine != nil {
		np.Title = z.NowPlaying.ThreeLine.Line1
		np.Artist = z.NowPlaying.ThreeLine.Line2
		np.Album = z.NowPlaying.ThreeLine.Line3
	} else if z.NowPlaying.TwoLine != nil {
		np.Title = z.NowPlaying.TwoLine.Line1
		np.Artist = z.NowPlaying.TwoLine.Line2
	} else if z.NowPlaying.OneLine != nil {
		np.Title = z.NowPlaying.OneLine.Line1
	}

	out.NowPlaying = np
	return out
}
