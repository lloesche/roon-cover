package app

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"log/slog"
	"roon-cover/internal/display"
	"roon-cover/internal/roon"
	"testing"
)

type source struct{ calls int }

func (s *source) SubscribeZones(context.Context, roon.Core, func(roon.ZoneUpdate) error) error {
	return nil
}
func (s *source) FetchImage(context.Context, roon.Core, roon.ImageKey, roon.ImageFetchOptions) ([]byte, string, error) {
	s.calls++
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	return b.Bytes(), "image/png", nil
}
func TestPauseResumeRestoresCompleteScene(t *testing.T) {
	s := &source{}
	c := Controller{Source: s, Log: slog.Default(), size: 800}
	z := roon.Zone{ID: "a", Name: "A", State: roon.ZoneStatePlaying, NowPlaying: &roon.NowPlaying{ImageKey: "cover", Title: "title"}}
	if err := c.Initialize([]roon.Zone{z}); err != nil {
		t.Fatal(err)
	}
	c.asset = &display.Artwork{Key: "cover/800"}
	first := c.scene()
	z.State = roon.ZoneStatePaused
	c.replace([]roon.Zone{z})
	if c.scene().Artwork != nil {
		t.Fatal("paused scene must blank")
	}
	z.State = roon.ZoneStatePlaying
	c.replace([]roon.Zone{z})
	resumed := c.scene()
	if resumed.Artwork != first.Artwork || resumed.NowPlaying.Title != "title" || s.calls != 0 {
		t.Fatal("resume must restore retained artwork and metadata")
	}
}
func TestZoneRemovalAndOrdering(t *testing.T) {
	c := Controller{}
	_ = c.Initialize([]roon.Zone{{ID: "b", Name: "B"}, {ID: "a", Name: "A"}})
	c.cycle(display.EventNextZone)
	if c.active != "b" {
		t.Fatal(c.active)
	}
	c.replace([]roon.Zone{{ID: "c", Name: "C"}})
	if c.active != "c" {
		t.Fatal(c.active)
	}
	c.replace(nil)
	if c.selected().ID != "" {
		t.Fatal("removed zone retained")
	}
}

func TestCoverReplacementKeepsFadeSourceWhileLoading(t *testing.T) {
	c := Controller{}
	z := roon.Zone{ID: "a", Name: "A", State: roon.ZoneStatePlaying, NowPlaying: &roon.NowPlaying{ImageKey: "old", Title: "Old title"}}
	if err := c.Initialize([]roon.Zone{z}); err != nil {
		t.Fatal(err)
	}
	old := &display.Artwork{Key: "old/800"}
	c.asset = old
	z.NowPlaying = &roon.NowPlaying{ImageKey: "new", Title: "New title"}
	c.replace([]roon.Zone{z})
	pending := c.scene()
	if pending.Artwork != old || pending.NoFade || pending.NowPlaying.Title != "New title" {
		t.Fatal("pending download must preserve outgoing cover and allow metadata to fade")
	}
	next := &display.Artwork{Key: "new/800"}
	c.asset = next
	if ready := c.scene(); ready.Artwork != next || ready.NoFade {
		t.Fatal("downloaded cover must replace the outgoing cover with a fade")
	}
	z.State = roon.ZoneStatePaused
	c.replace([]roon.Zone{z})
	if paused := c.scene(); paused.Artwork != nil || paused.NowPlaying != nil {
		t.Fatal("pause must still blank immediately")
	}
	z.State = roon.ZoneStatePlaying
	z.NowPlaying = &roon.NowPlaying{Title: "No cover"}
	c.replace([]roon.Zone{z})
	if c.scene().Artwork != nil {
		t.Fatal("track without artwork must not retain the previous cover")
	}
}
