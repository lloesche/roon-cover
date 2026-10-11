package roon

import (
	"context"
	"errors"
	"fmt"
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

func (s *session) subscribeZones(ctx context.Context, onUpdate func(ZoneUpdate) error) error {
	zonesByID := map[string]Zone{}
	subscribed := false

	return s.conn.Subscribe(ctx, svcTransport+"/subscribe_zones", map[string]int{"subscription_key": 1}, func(f *mooFrame) error {
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

		switch f.ResponseName {
		case "Subscribed":
			subscribed = true
			zonesByID = map[string]Zone{}
			for _, z := range payload.Zones {
				zonesByID[z.ZoneID] = mapZone(z)
			}
		case "Changed":
			if !subscribed {
				return errors.New("roon: zone change before subscription snapshot")
			}
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
			return errors.New("roon: zone subscription ended")
		default:
			// Could be an error; bubble it up.
			return fmt.Errorf("subscribe_zones: %s", f.ResponseName)
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
}

// SubscribeZones is a convenience for diagnostics. The kiosk owns a Session
// directly so startup and playback share the same connection.
func (c *Client) SubscribeZones(ctx context.Context, core Core, onUpdate func(ZoneUpdate) error) error {
	s, err := c.OpenSession(ctx, core, nil)
	if err != nil {
		return err
	}
	defer s.Close()
	if onUpdate != nil {
		if err := onUpdate(ZoneUpdate{Zones: s.Zones}); err != nil {
			return err
		}
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.Done:
			return errors.New("roon: zone subscription disconnected")
		case update := <-s.Updates:
			if onUpdate != nil {
				if err := onUpdate(update); err != nil {
					return err
				}
			}
		}
	}
}

func (c *Client) GetZones(ctx context.Context, core Core) ([]Zone, error) {
	s, err := c.OpenSession(ctx, core, nil)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	return s.Zones, nil
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
