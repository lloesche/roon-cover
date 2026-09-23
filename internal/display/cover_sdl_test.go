//go:build sdl

package display

import (
	"github.com/veandco/go-sdl2/sdl"
	"image"
	"image/color"
	"testing"
	"time"
)

func TestCoverCompositionAndInterruption(t *testing.T) {
	s, err := sdl.CreateRGBSurfaceWithFormat(0, 4, 4, 32, sdl.PIXELFORMAT_ABGR8888)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Free()
	r, err := sdl.CreateSoftwareRenderer(s)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Destroy()
	c := coverLayer{ren: r, duration: time.Second, ease: func(x float64) float64 { return x }}
	defer c.close()
	asset := func(key string, value uint8) *Artwork {
		p := image.NewNRGBA(image.Rect(0, 0, 4, 4))
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				p.SetNRGBA(x, y, color.NRGBA{value, value, value, 255})
			}
		}
		return &Artwork{Key: key, Pixels: p}
	}
	now := time.Now()
	if err = c.set(asset("a", 255), true, now); err != nil {
		t.Fatal(err)
	}
	if err = c.set(asset("b", 255), false, now); err != nil {
		t.Fatal(err)
	}
	draw := func(at time.Time) uint8 {
		t.Helper()
		_ = r.SetDrawColor(0, 0, 0, 255)
		_ = r.Clear()
		if err := c.draw(at); err != nil {
			t.Fatal(err)
		}
		r.Present()
		return s.Pixels()[0]
	}
	if got := draw(now.Add(time.Second / 2)); got < 254 {
		t.Fatalf("identical covers darken: %d", got)
	}
	if err = c.set(asset("c", 0), false, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	mid := now.Add(1500 * time.Millisecond)
	before := draw(mid)
	if err = c.set(asset("d", 255), false, mid); err != nil {
		t.Fatal(err)
	}
	after := draw(mid)
	if delta := int(after) - int(before); delta < -1 || delta > 1 {
		t.Fatalf("interrupted transition jumped: %d -> %d", before, after)
	}
}
