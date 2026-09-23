package display

import (
	"context"
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"image/color"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// GPU checks run inside the real engine lifecycle and use actual GPU readback.
// Opt in on a machine with a graphics driver (or an Xvfb/Mesa CI session).
func TestMain(m *testing.M) {
	if os.Getenv("ROON_COVER_STARTUP_SMOKE") == "1" {
		os.Exit(windowStartupSmoke())
	}
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

func windowStartupSmoke() int {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	info := make(chan ScreenInfo, 1)
	updates := make(chan Update, 1)
	var initialized atomic.Bool
	go func() {
		select {
		case size := <-info:
			initialized.Store(size.RenderWidth > 0 && size.RenderHeight > 0)
			cancel()
		case <-ctx.Done():
		}
	}()
	w := Window{Width: 64, Height: 64, Title: "roon-cover startup check", InfoCh: info}
	if err := w.Run(ctx, updates); err != nil || !initialized.Load() {
		return 1
	}
	return 0
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

func TestStartupLineFadePreservesEarlierText(t *testing.T) {
	if os.Getenv("ROON_COVER_GPU_TESTS") != "1" {
		t.Skip("set ROON_COVER_GPU_TESTS=1 for GPU checks")
	}
	text, err := newTextEngine("", 28)
	if err != nil {
		t.Fatal(err)
	}
	g := windowGame{text: text, width: 800, height: 800, scale: 1, cover: coverLayer{duration: time.Second, ease: func(x float64) float64 { return x }}}
	defer g.close()
	out := ebiten.NewImage(800, 800)
	defer out.Deallocate()
	brightness := func(at time.Time) uint64 {
		g.now = at
		g.drawStatus(out)
		pixels := make([]byte, 800*800*4)
		out.ReadPixels(pixels)
		var sum uint64
		for i := 0; i < len(pixels); i += 4 {
			sum += uint64(pixels[i])
		}
		return sum
	}
	now := time.Unix(1, 0)
	g.setStatus(&Status{Lines: []string{"Looking for Roon…"}}, now)
	before := brightness(now.Add(time.Second))
	g.setStatus(&Status{Lines: []string{"Looking for Roon…", "Found blackhole"}}, now.Add(time.Second))
	if got := brightness(now.Add(time.Second)); got != before {
		t.Fatal("adding a line moved, faded, or replaced earlier text")
	}
	mid := brightness(now.Add(1500 * time.Millisecond))
	end := brightness(now.Add(2 * time.Second))
	if !(before < mid && mid < end) {
		t.Fatal("new text did not fade in gradually")
	}
	settled := make(chan struct{}, 1)
	g.setStatus(&Status{Lines: []string{"Looking for Roon…", "Found blackhole"}, FadeOut: true, Settled: settled}, now.Add(2*time.Second))
	startOut := brightness(now.Add(2 * time.Second))
	midOut := brightness(now.Add(2500 * time.Millisecond))
	select {
	case <-settled:
		t.Fatal("fade-out acknowledged before finishing")
	default:
	}
	endOut := brightness(now.Add(3 * time.Second))
	if !(startOut > midOut && midOut > endOut && endOut == 0) {
		t.Fatal("startup did not fade completely to black")
	}
	select {
	case <-settled:
	default:
		t.Fatal("completed fade-out was not acknowledged")
	}
	g.setStatus(nil, now.Add(3*time.Second))
	pixels := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			pixels.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
		}
	}
	g.cover.width, g.cover.height = 800, 800
	g.cover.set(&Artwork{Key: "first", Pixels: pixels}, false, now.Add(3*time.Second))
	coverPixel := func(at time.Time) uint8 {
		out.Fill(color.Black)
		g.cover.draw(out, at)
		p := make([]byte, 800*800*4)
		out.ReadPixels(p)
		return p[(400*800+400)*4]
	}
	if coverPixel(now.Add(3*time.Second)) != 0 {
		t.Fatal("first cover snapped on after startup")
	}
	if p := coverPixel(now.Add(3500 * time.Millisecond)); p < 126 || p > 129 {
		t.Fatalf("first cover did not fade in: %d", p)
	}
	if coverPixel(now.Add(4*time.Second)) != 255 {
		t.Fatal("first cover fade never finished")
	}
}
