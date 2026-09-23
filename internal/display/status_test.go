package display

import (
	"image/png"
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
		"Looking for Roon…", "Searching for your Roon Server.",
		"Found blackhole", "Connecting to Roon…",
		"Connect to blackhole", "In Roon, open Settings → Extensions", "Enable roon-cover to continue.",
		"Connected to blackhole", "Choosing your listening zone…",
		"Using Dialysis", "No zone specified. Using a playing zone.", "Displaying now playing…",
	}}, 800, 800, 1)
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
	g.setStatus(&Status{Lines: []string{"Looking for Roon…", "Found blackhole", "Connecting…"}}, now.Add(time.Second))
	if len(g.statusRows) != 3 || g.statusRows[0].start != now || g.statusRows[1].start != now.Add(time.Second) {
		t.Fatal("only appended lines should begin fading")
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
