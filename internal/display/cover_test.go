package display

import (
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"image/color"
	"os"
	"testing"
	"time"
)

// GPU checks run inside the real engine lifecycle and use actual GPU readback.
// Opt in on a machine with a graphics driver (or an Xvfb/Mesa CI session).
func TestMain(m *testing.M) {
	if os.Getenv("ROON_COVER_GPU_TESTS") != "1" {
		os.Exit(m.Run())
	}
	ebiten.SetWindowVisible(false)
	g := &testGame{run: m.Run}
	if err := ebiten.RunGame(g); err != nil {
		panic(err)
	}
	os.Exit(g.code)
}

type testGame struct {
	run  func() int
	code int
}

func (g *testGame) Update() error              { g.code = g.run(); return ebiten.Termination }
func (g *testGame) Draw(*ebiten.Image)         {}
func (g *testGame) Layout(int, int) (int, int) { return 32, 32 }
func TestCoverCompositionAndInterruption(t *testing.T) {
	if os.Getenv("ROON_COVER_GPU_TESTS") != "1" {
		t.Skip("set ROON_COVER_GPU_TESTS=1 for GPU checks")
	}
	c := coverLayer{width: 4, height: 4, duration: time.Second, ease: func(x float64) float64 { return x }}
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
	out := ebiten.NewImage(4, 4)
	defer out.Deallocate()
	draw := func(at time.Time) uint8 {
		out.Fill(color.Black)
		c.draw(out, at)
		p := make([]byte, 64)
		out.ReadPixels(p)
		return p[0]
	}
	now := time.Now()
	c.set(asset("a", 255), true, now)
	c.set(asset("b", 255), false, now)
	if got := draw(now.Add(time.Second / 2)); got < 254 {
		t.Fatalf("identical covers darken: %d", got)
	}
	c.set(asset("c", 0), false, now.Add(time.Second))
	mid := now.Add(1500 * time.Millisecond)
	before := draw(mid)
	c.set(asset("d", 255), false, mid)
	after := draw(mid)
	if delta := int(after) - int(before); delta < -1 || delta > 1 {
		t.Fatalf("interrupted fade jumped: %d -> %d", before, after)
	}
	c.set(nil, false, mid)
	if draw(mid) != 0 {
		t.Fatal("pause did not blank immediately")
	}
}
