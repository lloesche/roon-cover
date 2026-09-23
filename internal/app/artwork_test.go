package app

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"roon-cover/internal/display"
	"roon-cover/internal/roon"
	"testing"
	"time"
)

type blockedArtworkSource struct{ started, canceled chan struct{} }

func (s *blockedArtworkSource) SubscribeZones(ctx context.Context, _ roon.Core, _ func(roon.ZoneUpdate) error) error {
	<-ctx.Done()
	return nil
}
func (s *blockedArtworkSource) FetchImage(ctx context.Context, _ roon.Core, key roon.ImageKey, _ roon.ImageFetchOptions) ([]byte, string, error) {
	if key == "slow" {
		close(s.started)
		<-ctx.Done()
		close(s.canceled)
		return nil, "", ctx.Err()
	}
	var b bytes.Buffer
	err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	return b.Bytes(), "image/png", err
}
func TestZoneChangeCancelsBlockedArtwork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	source := &blockedArtworkSource{started: make(chan struct{}), canceled: make(chan struct{})}
	c := Controller{Source: source}
	if err := c.Initialize([]roon.Zone{{ID: "a", Name: "A", State: roon.ZoneStatePlaying, NowPlaying: &roon.NowPlaying{ImageKey: "slow"}}, {ID: "b", Name: "B", State: roon.ZoneStatePlaying, NowPlaying: &roon.NowPlaying{ImageKey: "fast"}}}); err != nil {
		t.Fatal(err)
	}
	scenes := make(chan display.Update, 1)
	info := make(chan display.ScreenInfo, 1)
	events := make(chan display.Event, 1)
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, scenes, info, events) }()
	defer func() { cancel(); <-done }()
	info <- display.ScreenInfo{RenderWidth: 800, RenderHeight: 800}
	select {
	case <-source.started:
	case <-ctx.Done():
		t.Fatal("fetch did not start")
	}
	events <- display.Event{Kind: display.EventNextZone}
	for {
		select {
		case scene := <-scenes:
			if scene.Zone == "B" && scene.Artwork != nil {
				select {
				case <-source.canceled:
					return
				default:
					t.Fatal("obsolete fetch was not canceled")
				}
			}
		case <-ctx.Done():
			t.Fatal("zone change blocked behind artwork download")
		}
	}
}
