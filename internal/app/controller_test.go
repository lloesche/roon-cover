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
	first := c.scene(context.Background())
	z.State = roon.ZoneStatePaused
	c.replace([]roon.Zone{z})
	if c.scene(context.Background()).Artwork != nil {
		t.Fatal("paused scene must blank")
	}
	z.State = roon.ZoneStatePlaying
	c.replace([]roon.Zone{z})
	resumed := c.scene(context.Background())
	if resumed.Artwork != first.Artwork || resumed.NowPlaying.Title != "title" || s.calls != 1 {
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
