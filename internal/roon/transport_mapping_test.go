package roon

import "testing"

func TestParseSubscriptionKey(t *testing.T) {
	t.Parallel()

	if got := parseSubscriptionKey(nil); got != "0" {
		t.Fatalf("nil got=%q", got)
	}
	if got := parseSubscriptionKey(float64(7)); got != "7" {
		t.Fatalf("float got=%q", got)
	}
	if got := parseSubscriptionKey(""); got != "0" {
		t.Fatalf("empty string got=%q", got)
	}
	if got := parseSubscriptionKey("abc"); got != "abc" {
		t.Fatalf("string got=%q", got)
	}
	if got := parseSubscriptionKey(12); got != "12" {
		t.Fatalf("int got=%q", got)
	}
}

func TestMapZone_NowPlayingLinePreference(t *testing.T) {
	t.Parallel()

	z := zoneRaw{
		ZoneID:      "z1",
		DisplayName: "Zone",
		State:       "playing",
		NowPlaying: &nowPlayingRaw{
			ImageKey: "img",
			ThreeLine: &lineBlock3{
				Line1: "Title",
				Line2: "Artist",
				Line3: "Album",
			},
		},
	}
	out := mapZone(z)
	if out.ID != ZoneID("z1") || out.Name != "Zone" || out.State != ZoneState("playing") {
		t.Fatalf("zone mismatch: %#v", out)
	}
	if out.NowPlaying == nil {
		t.Fatalf("expected nowplaying")
	}
	if out.NowPlaying.ImageKey != ImageKey("img") {
		t.Fatalf("imagekey got=%q", out.NowPlaying.ImageKey)
	}
	if out.NowPlaying.Title != "Title" || out.NowPlaying.Artist != "Artist" || out.NowPlaying.Album != "Album" {
		t.Fatalf("now playing mismatch: %#v", out.NowPlaying)
	}

	// TwoLine should map Title/Artist.
	z.NowPlaying.ThreeLine = nil
	z.NowPlaying.TwoLine = &lineBlock2{Line1: "T2", Line2: "A2"}
	out = mapZone(z)
	if out.NowPlaying.Title != "T2" || out.NowPlaying.Artist != "A2" || out.NowPlaying.Album != "" {
		t.Fatalf("two-line mismatch: %#v", out.NowPlaying)
	}

	// OneLine should map Title only.
	z.NowPlaying.TwoLine = nil
	z.NowPlaying.OneLine = &lineBlock1{Line1: "T1"}
	out = mapZone(z)
	if out.NowPlaying.Title != "T1" || out.NowPlaying.Artist != "" || out.NowPlaying.Album != "" {
		t.Fatalf("one-line mismatch: %#v", out.NowPlaying)
	}
}
