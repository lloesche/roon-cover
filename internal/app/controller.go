// Package app owns zone selection and desired scenes independently of SDL and CLI.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"roon-cover/internal/display"
	"roon-cover/internal/roon"
	"sort"
	"strings"
	"time"
)

type Source interface {
	SubscribeZones(context.Context, roon.Core, func(roon.ZoneUpdate) error) error
	FetchImage(context.Context, roon.Core, roon.ImageKey, roon.ImageFetchOptions) ([]byte, string, error)
}
type Options struct {
	Zone        string
	SleepAfter  time.Duration
	SaveArtwork func(string, []byte, string)
}
type Controller struct {
	Source  Source
	Core    roon.Core
	Options Options
	Log     *slog.Logger
	Power   func(context.Context, bool) error
	zones   []roon.Zone
	active  roon.ZoneID
	size    int
	asset   *display.Artwork
}

func (c *Controller) Initialize(zones []roon.Zone) error {
	c.zones = ordered(zones)
	if len(c.zones) == 0 {
		return fmt.Errorf("no zones found on this Roon Core")
	}
	c.active = c.zones[0].ID
	if name := strings.TrimSpace(c.Options.Zone); name != "" {
		for _, z := range c.zones {
			if strings.EqualFold(z.Name, name) {
				c.active = z.ID
				return nil
			}
		}
		return fmt.Errorf("unknown zone %q", name)
	}
	return nil
}
func ordered(zones []roon.Zone) []roon.Zone {
	out := append([]roon.Zone(nil), zones...)
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if a == b {
			return out[i].ID < out[j].ID
		}
		return a < b
	})
	return out
}
func (c *Controller) selected() roon.Zone {
	for _, z := range c.zones {
		if z.ID == c.active {
			return z
		}
	}
	return roon.Zone{}
}
func (c *Controller) replace(zones []roon.Zone) {
	c.zones = ordered(zones)
	if c.selected().ID != "" {
		return
	}
	c.active = ""
	if len(c.zones) > 0 {
		c.active = c.zones[0].ID
	}
}
func (c *Controller) cycle(kind display.EventKind) {
	if len(c.zones) == 0 {
		return
	}
	i := 0
	for n, z := range c.zones {
		if z.ID == c.active {
			i = n
			break
		}
	}
	delta := 1
	if kind == display.EventPrevZone {
		delta = -1
	}
	c.active = c.zones[(i+delta+len(c.zones))%len(c.zones)].ID
}
func (c *Controller) scene(ctx context.Context) display.Update {
	z := c.selected()
	scene := display.Update{Zone: z.Name}
	if z.State != roon.ZoneStatePlaying || z.NowPlaying == nil {
		return scene
	}
	np := z.NowPlaying
	scene.NowPlaying = &display.Metadata{Title: np.Title, Artist: np.Artist, Album: np.Album}
	if np.ImageKey == "" {
		return scene
	}
	key := fmt.Sprintf("%s/%d", np.ImageKey, c.size)
	if c.asset == nil || c.asset.Key != key {
		data, mime, err := c.Source.FetchImage(ctx, c.Core, np.ImageKey, roon.ImageFetchOptions{Size: c.size})
		if err != nil {
			c.Log.Warn("artwork fetch failed", "err", err)
			return scene
		}
		c.asset = &display.Artwork{Key: key, Data: data}
		if c.Options.SaveArtwork != nil {
			c.Options.SaveArtwork(z.Name, data, mime)
		}
	}
	scene.Artwork = c.asset
	return scene
}
func (c *Controller) Run(ctx context.Context, scenes chan display.Update, info <-chan display.ScreenInfo, events <-chan display.Event) error {
	defer close(scenes)
	if c.Log == nil {
		c.Log = slog.Default()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	updates := make(chan []roon.Zone, 1)
	errors := make(chan error, 1)
	go func() {
		errors <- c.Source.SubscribeZones(ctx, c.Core, func(u roon.ZoneUpdate) error {
			select {
			case updates <- u.Zones:
			default:
				select {
				case <-updates:
				default:
				}
				select {
				case updates <- u.Zones:
				default:
				}
			}
			return nil
		})
	}()
	power := make(chan bool, 1)
	go c.runPower(ctx, power)
	var idleSince time.Time
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errors:
			return err
		case zs := <-updates:
			c.replace(zs)
		case e := <-events:
			c.cycle(e.Kind)
		case sz := <-info:
			c.size = coverSize(sz.RenderWidth, sz.RenderHeight)
		case <-tick.C:
			if c.Options.SleepAfter > 0 && !idleSince.IsZero() && time.Since(idleSince) >= c.Options.SleepAfter {
				publishPower(power, true)
			}
			continue
		}
		if c.selected().State == roon.ZoneStatePlaying {
			idleSince = time.Time{}
			publishPower(power, false)
		} else if idleSince.IsZero() {
			idleSince = time.Now()
		}
		if c.size > 0 {
			display.Publish(scenes, c.scene(ctx))
		}
	}
}
func coverSize(w, h int) int { return max(200, min(min(w, h), 4096)) }
func publishPower(ch chan bool, sleep bool) {
	select {
	case ch <- sleep:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- sleep:
	default:
	}
}
func (c *Controller) runPower(ctx context.Context, desired <-chan bool) {
	current := false
	for {
		select {
		case <-ctx.Done():
			return
		case sleep := <-desired:
			if c.Power == nil || c.Options.SleepAfter <= 0 || sleep == current {
				continue
			}
			if err := c.Power(ctx, sleep); err != nil {
				c.Log.Warn("display power command failed", "err", err)
			} else {
				current = sleep
			}
		}
	}
}
