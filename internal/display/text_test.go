package display

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInternationalTextLayout(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	e, err := newTextEngine("", 28)
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	titles := []string{"Björk — Jóga / Sigur Rós", "宇多田ヒカル — First Love", "أم كلثوم — ألف ليلة وليلة", "עידן רייכל — ממעמקים", "लता मंगेशकर — लग जा गले", "Beyoncé 🎵 — Cafe\u0301", strings.Repeat("A very long title / 世界 / ", 30)}
	canvas := image.NewNRGBA(image.Rect(0, 0, 900, len(titles)*65))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.NRGBA{24, 24, 24, 255}), image.Point{}, draw.Src)
	for i, title := range titles {
		p, missing, err := e.raster(title, 860, 1)
		if err != nil {
			t.Fatal(err)
		}
		if p.Bounds().Dx() > 860 || p.Bounds().Dy() > 64 {
			t.Fatalf("unbounded layout: %v", p.Bounds())
		}
		visible := false
		for j := 3; j < len(p.Pix); j += 4 {
			visible = visible || p.Pix[j] > 0
		}
		if !visible {
			t.Fatalf("empty text raster for %q", title)
		}
		if missing > 0 {
			t.Logf("system font coverage: %d missing glyphs in %q", missing, title)
		}
		draw.Draw(canvas, image.Rect(20, i*65+10, 880, (i+1)*65), p, image.Point{}, draw.Over)
	}
	if dir := os.Getenv("ROON_COVER_TEST_ARTIFACTS"); dir != "" {
		f, err := os.Create(filepath.Join(dir, "font-review.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, canvas); err != nil {
			t.Fatal(err)
		}
	}
	base, _, err := e.raster("Scale", 600, 1)
	if err != nil {
		t.Fatal(err)
	}
	large, _, err := e.raster("Scale", 600, 2)
	if err != nil {
		t.Fatal(err)
	}
	if large.Bounds().Dy() < base.Bounds().Dy()*3/2 {
		t.Fatal("DPI scale ignored")
	}
}
func TestInvalidCustomFontFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.ttf")
	_ = os.WriteFile(path, []byte("not a font"), 0600)
	if engine, err := newTextEngine(path, 28); err == nil {
		engine.close()
		t.Fatal("invalid explicit font silently accepted")
	}
}
