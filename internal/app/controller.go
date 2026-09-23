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
	Source    Source
	Core      roon.Core
	Options   Options
	Log       *slog.Logger
	Power     func(context.Context, bool) error
	zones     []roon.Zone
	active    roon.ZoneID
	size      int
	asset     *display.Artwork
	lastScene display.Update
	sceneZone roon.ZoneID
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
	// Prefer active playback only at startup. Subsequent updates preserve the
	// selected zone, even when another zone starts playing or this one pauses.
	for _, z := range c.zones {
		if z.State == roon.ZoneStatePlaying {
			c.active = z.ID
			break
		}
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

// scene never performs I/O. Loading is a transition within playback, not a
// request to blank: retain the last complete scene until playback resumes.
func (c *Controller) scene() display.Update {
	z := c.selected()
	scene := display.Update{Zone: z.Name}
	if z.State == roon.ZoneStateLoading && z.ID == c.sceneZone {
		scene = c.lastScene
		scene.Zone = z.Name
		return scene
	}
	defer func() { c.lastScene, c.sceneZone = scene, z.ID }()
	if z.State != roon.ZoneStatePlaying || z.NowPlaying == nil {
		return scene
	}
	np := z.NowPlaying
	// Publish artwork and metadata together. Updating the title while the image
	// is still downloading would start its transition before the cover's.
	if np.ImageKey != "" && (c.asset == nil || !strings.HasPrefix(c.asset.Key, string(np.ImageKey)+"/")) {
		scene = c.lastScene
		scene.Zone = z.Name
		return scene
	}
	scene.NowPlaying = &display.Metadata{Title: np.Title, Artist: np.Artist, Album: np.Album}
	// Keep the outgoing cover while its replacement downloads. Publishing nil
	// here would blank the renderer and discard the source of the crossfade.
	// Paused playback and tracks without artwork still blank immediately.
	if np.ImageKey != "" {
		scene.Artwork = c.asset
	}
	return scene
}
func (c *Controller) Run(ctx context.Context, scenes chan display.Update, info <-chan display.ScreenInfo, events <-chan display.Event) error {
	if c.Log == nil {
		c.Log = slog.Default()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	updates := make(chan subscriptionEvent, 1)
	go c.subscribe(ctx, updates)
	power := make(chan bool, 1)
	powerDone := make(chan struct{})
	go func() { defer close(powerDone); c.runPower(ctx, power) }()
	defer func() { cancel(); <-powerDone }()
	jobs := make(chan artworkRequest, 1)
	results := make(chan artworkResult, 1)
	go c.loadArtwork(ctx, jobs, results)
	var idleSince, nextAttempt time.Time
	var requested string
	var fetchCancel context.CancelFunc
	defer func() {
		if fetchCancel != nil {
			fetchCancel()
		}
	}()
	connected := true
	fetching := false
	retry := time.Second
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		dirty := false
		select {
		case <-ctx.Done():
			return nil
		case update := <-updates:
			connected = update.err == nil
			if connected {
				c.replace(update.zones)
			} else {
				c.Log.Warn("zone subscription lost; reconnecting", "error", update.err)
			}
			dirty = true
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			c.cycle(event.Kind)
			dirty = true
		case sz, ok := <-info:
			if !ok {
				info = nil
				continue
			}
			size := coverSize(sz.RenderWidth, sz.RenderHeight)
			if size != c.size {
				c.size = size
				nextAttempt = time.Now().Add(150 * time.Millisecond)
				dirty = true
			}
		case result := <-results:
			if result.key != requested {
				continue
			}
			fetching = false
			if result.err != nil {
				c.Log.Warn("artwork fetch failed; retrying", "error", result.err)
				nextAttempt = time.Now().Add(retry)
				retry = min(30*time.Second, retry*2)
			} else {
				c.asset = result.asset
				retry = time.Second
				dirty = true
			}
		case <-tick.C:
		}
		z := c.selected()
		if connected && (z.State == roon.ZoneStatePlaying || z.State == roon.ZoneStateLoading) {
			idleSince = time.Time{}
			publishPower(power, false)
		} else if idleSince.IsZero() {
			idleSince = time.Now()
		}
		if c.Options.SleepAfter > 0 && !idleSince.IsZero() && time.Since(idleSince) >= c.Options.SleepAfter {
			publishPower(power, true)
		}
		key := ""
		var imageKey roon.ImageKey
		if connected && c.size > 0 && z.State == roon.ZoneStatePlaying && z.NowPlaying != nil && z.NowPlaying.ImageKey != "" {
			imageKey = z.NowPlaying.ImageKey
			key = fmt.Sprintf("%s/%d", imageKey, c.size)
		}
		if key != requested {
			if fetchCancel != nil {
				fetchCancel()
			}
			requested = key
			fetching = false
			retry = time.Second
			if !nextAttempt.After(time.Now()) {
				nextAttempt = time.Now()
			}
		}
		if key != "" && !fetching && (c.asset == nil || c.asset.Key != key) && !time.Now().Before(nextAttempt) {
			fetchCtx, stop := context.WithTimeout(ctx, 15*time.Second)
			fetchCancel = stop
			request := artworkRequest{ctx: fetchCtx, key: key, imageKey: imageKey, size: c.size, zone: z.Name}
			select {
			case <-jobs:
			default:
			}
			jobs <- request
			fetching = true
		}
		if dirty && c.size > 0 {
			scene := c.scene()
			if !connected {
				scene = display.Update{Zone: z.Name, Status: &display.Status{Title: "Connection to Roon lost", Detail: "Waiting for your Roon Server…", Hint: "Reconnecting automatically."}, NoFade: true}
				c.lastScene, c.sceneZone = display.Update{}, ""
			}
			display.Publish(scenes, scene)
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
