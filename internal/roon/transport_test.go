package roon

import "testing"

func TestZoneSubscriptionKeepsSeekSnapshotsImmutable(t *testing.T) {
	state := &zoneSubscription{}
	apply := func(name, body string) []Zone {
		t.Helper()
		zones, err := state.apply(&mooFrame{ResponseName: name, BodyRaw: []byte(body)})
		if err != nil {
			t.Fatal(err)
		}
		return zones
	}
	initial := apply("Subscribed", `{"zones":[{"zone_id":"bedroom","display_name":"Bedroom","state":"playing","now_playing":{"image_key":"cover","seek_position":0,"length":240,"three_line":{"line1":"Song","line2":"Artist","line3":"Album"}}}]}`)
	if len(initial) != 1 || initial[0].NowPlaying.Title != "Song" || *initial[0].NowPlaying.SeekPosition != 0 || *initial[0].NowPlaying.Length != 240 {
		t.Fatal("initial metadata or zero seek position was lost")
	}
	updated := apply("Changed", `{"zones_seek_changed":[{"zone_id":"bedroom","seek_position":15.5},{"zone_id":"unknown","seek_position":9}]}`)
	if len(updated) != 1 || *updated[0].NowPlaying.SeekPosition != 15.5 || *initial[0].NowPlaying.SeekPosition != 0 || updated[0].NowPlaying.Album != "Album" {
		t.Fatal("seek delta lost metadata or mutated an earlier snapshot")
	}
	unchanged := apply("Changed", `{"zones_seek_changed":[{"zone_id":"bedroom"}]}`)
	if *unchanged[0].NowPlaying.SeekPosition != 15.5 {
		t.Fatal("missing seek position overwrote a known position")
	}
	removed := apply("Changed", `{"zones_removed":["bedroom"]}`)
	if len(removed) != 0 {
		t.Fatal("string zone ID was not removed")
	}
	apply("Changed", `{"zones_added":[{"zone_id":"radio","now_playing":{"one_line":{"line1":"Live"}}}]}`)
	if len(state.zonesByID) != 1 || state.zonesByID["radio"].NowPlaying.Length != nil || state.zonesByID["radio"].NowPlaying.SeekPosition != nil {
		t.Fatal("unavailable radio timing was treated as zero")
	}
	removed = apply("Changed", `{"zones_removed":[{"zone_id":"radio"}]}`)
	if len(removed) != 0 {
		t.Fatal("legacy object zone ID was not removed")
	}
}

func TestZoneSubscriptionRejectsOutOfOrderAndMalformedFrames(t *testing.T) {
	for _, f := range []*mooFrame{
		{ResponseName: "Changed", BodyRaw: []byte(`{}`)},
		{ResponseName: "Subscribed", BodyRaw: []byte(`{"zones_removed":[1]}`)},
		{ResponseName: "Unsubscribed"},
		{ResponseName: "InvalidRequest"},
	} {
		if _, err := (&zoneSubscription{}).apply(f); err == nil {
			t.Fatalf("accepted invalid %s frame", f.ResponseName)
		}
	}
}
