package display

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPairingStatusRaster(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	engine, err := newTextEngine("", 28)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.close()
	pixels, err := renderStatus(engine, Status{Lines: []string{
		"Looking for Roon…", "found blackhole", "Connecting to blackhole…",
		"connected", "Choosing listening zone…", "Dialysis",
	}, Joins: []bool{false, true, false, true, false, true}}, 800, 800, 1)
	if err != nil {
		t.Fatal(err)
	}
	visible := 0
	for i := 0; i < len(pixels.Pix); i += 4 {
		if pixels.Pix[i] > 150 {
			visible++
		}
	}
	if visible < 100 {
		t.Fatal("instructions did not render")
	}
	if dir := os.Getenv("ROON_COVER_TEST_ARTIFACTS"); dir != "" {
		f, err := os.Create(filepath.Join(dir, "pairing.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, pixels); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAppendingStatusDoesNotRestartEarlierLines(t *testing.T) {
	var g windowGame
	now := time.Unix(1, 0)
	g.setStatus(&Status{Lines: []string{"Looking for Roon…"}}, now)
	g.setStatus(&Status{Lines: []string{"Looking for Roon…", "found blackhole", "Connecting…"}, Joins: []bool{false, true, false}}, now.Add(time.Second))
	if len(g.statusRows) != 3 || g.statusRows[0].start != now || g.statusRows[1].start != now.Add(time.Second) {
		t.Fatal("only appended lines should begin fading")
	}
	if statusLineCount(g.status) != 2 || !g.statusRows[1].inline {
		t.Fatal("result must share the prompt's line")
	}
	g.clearStatusImage()
	if g.statusRows[0].start != now {
		t.Fatal("resize restarted a completed fade")
	}
	g.setStatus(nil, now)
	if len(g.statusRows) != 0 {
		t.Fatal("playback retained startup text")
	}
}

// Static rendering also supports visual checks without a graphics session.
func renderStatus(text *textEngine, status Status, width, height int, scale float64) (*image.NRGBA, error) {
	canvas := image.NewNRGBA(image.Rect(0, 0, max(1, width), max(1, height)))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.NRGBA{R: 18, G: 22, B: 30, A: 255}), image.Point{}, draw.Src)
	lines := statusLines(&status)
	x, y, lineWidth, lineHeight, textScale := statusLayout(text, width, height, scale, statusLineCount(&status))
	cursorX, cursorY := x, y
	for i, value := range lines {
		if i > 0 && !statusInline(&status, i) {
			cursorX = x
			cursorY += lineHeight
		}
		line, _, err := text.raster(value, max(1, lineWidth-(cursorX-x)), textScale)
		if err != nil {
			return nil, err
		}
		draw.Draw(canvas, line.Bounds().Add(image.Pt(cursorX, cursorY)), line, line.Bounds().Min, draw.Over)
		cursorX += line.Bounds().Dx() + int(math.Round(text.size*textScale*.2))
	}
	return canvas, nil
}
