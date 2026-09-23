package display

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestPairingStatusRaster(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	engine, err := newTextEngine("", 28)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.close()
	pixels, err := renderStatus(engine, Status{Title: "Connect to blackhole", Detail: "In Roon, open Settings → Extensions", Hint: "Enable roon-cover to continue."}, 800, 800, 1)
	if err != nil {
		t.Fatal(err)
	}
	visible := 0
	for i := 3; i < len(pixels.Pix); i += 4 {
		if pixels.Pix[i] > 0 {
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
