package roon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type transportSubscribeZonesResponse struct {
	Zones            []zoneRaw     `json:"zones"`
	ZonesAdded       []zoneRaw     `json:"zones_added"`
	ZonesChanged     []zoneRaw     `json:"zones_changed"`
	ZonesRemoved     []zoneID      `json:"zones_removed"`
	ZonesSeekChanged []zoneSeekRaw `json:"zones_seek_changed"`
}

type zoneID struct {
	ZoneID string `json:"zone_id"`
}

// Cores send string IDs; accept the older object form too.
func (z *zoneID) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &z.ZoneID)
	}
	type object zoneID
	return json.Unmarshal(data, (*object)(z))
}

type zoneSeekRaw struct {
	ZoneID       string   `json:"zone_id"`
	SeekPosition *float64 `json:"seek_position"`
}

type zoneRaw struct {
	ZoneID      string         `json:"zone_id"`
	DisplayName string         `json:"display_name"`
	State       string         `json:"state"`
	NowPlaying  *nowPlayingRaw `json:"now_playing"`
}

type nowPlayingRaw struct {
	ImageKey     string      `json:"image_key"`
	SeekPosition *float64    `json:"seek_position"`
	Length       *float64    `json:"length"`
	OneLine      *lineBlock1 `json:"one_line"`
	TwoLine      *lineBlock2 `json:"two_line"`
	ThreeLine    *lineBlock3 `json:"three_line"`
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

// zoneSubscription belongs to the serialized connection read loop. Published
// snapshots own their metadata, including after incremental seek updates.
type zoneSubscription struct {
	zonesByID  map[string]Zone
	subscribed bool
}

func (s *session) subscribeZones(ctx context.Context, onUpdate func(ZoneUpdate) error) error {
	state := &zoneSubscription{}
	return s.conn.Subscribe(ctx, svcTransport+"/subscribe_zones", map[string]int{"subscription_key": 1}, func(f *mooFrame) error {
		if f.ResponseName == "" {
			return nil
		}
		zones, err := state.apply(f)
		if err != nil {
			return err
		}
		if onUpdate != nil {
			return onUpdate(ZoneUpdate{Zones: zones})
		}
		return nil
	})
}

func (s *zoneSubscription) apply(f *mooFrame) ([]Zone, error) {
	var payload transportSubscribeZonesResponse
	if len(f.BodyRaw) > 0 {
		if err := json.Unmarshal(f.BodyRaw, &payload); err != nil {
			return nil, err
		}
	}
	switch f.ResponseName {
	case "Subscribed":
		s.subscribed = true
		s.zonesByID = make(map[string]Zone, len(payload.Zones))
		for _, z := range payload.Zones {
			s.zonesByID[z.ZoneID] = mapZone(z)
		}
	case "Changed":
		if !s.subscribed {
			return nil, errors.New("roon: zone change before subscription snapshot")
		}
		for _, z := range payload.ZonesRemoved {
			delete(s.zonesByID, z.ZoneID)
		}
		for _, z := range payload.ZonesAdded {
			s.zonesByID[z.ZoneID] = mapZone(z)
		}
		for _, z := range payload.ZonesChanged {
			s.zonesByID[z.ZoneID] = mapZone(z)
		}
		for _, seek := range payload.ZonesSeekChanged {
			z, ok := s.zonesByID[seek.ZoneID]
			if !ok || z.NowPlaying == nil || seek.SeekPosition == nil {
				continue
			}
			np := *z.NowPlaying
			np.SeekPosition = seek.SeekPosition
			z.NowPlaying = &np
			s.zonesByID[seek.ZoneID] = z
		}
	case "Unsubscribed":
		return nil, errors.New("roon: zone subscription ended")
	default:
		return nil, fmt.Errorf("subscribe_zones: %s", f.ResponseName)
	}
	zones := make([]Zone, 0, len(s.zonesByID))
	for _, z := range s.zonesByID {
		zones = append(zones, z)
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
		ImageKey:     ImageKey(z.NowPlaying.ImageKey),
		SeekPosition: z.NowPlaying.SeekPosition,
		Length:       z.NowPlaying.Length,
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
