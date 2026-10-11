package display

import (
	"fmt"
	"math"
	"testing"
	"time"
)

func TestKeyboardZoneLabelIgnoresOverlayFlagsAndExpires(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, duration := range []time.Duration{0, 500 * time.Millisecond} {
			t.Run(fmt.Sprintf("show-zone=%t/fade=%s", enabled, duration), func(t *testing.T) {
				now := time.Unix(1, 0)
				g := windowGame{now: now, enabled: [4]bool{false, false, false, enabled}}
				ease, _ := FadeEasingByName("in-out-sine")
				g.lines[3].animation = textTransition{duration: duration, hold: 5 * time.Second, ease: ease}
				g.applyScene(Update{Zone: "Living room", NowPlaying: &Metadata{Title: "Hidden title"}})
				if !enabled && g.lines[3].animation.value != "" {
					t.Fatal("ordinary zone snapshot enabled an overlay")
				}
				selection := Update{Zone: "Bedroom", ZoneSelection: 1, NoFade: true}
				g.applyScene(selection)
				zone := &g.lines[3].animation
				if zone.requested != "Bedroom" || g.lines[0].animation.value != "" {
					t.Fatal("keyboard selection must reveal only the zone, even without artwork")
				}
				reveal := min(duration/2, 100*time.Millisecond)
				settled := now.Add(reveal)
				if enabled {
					settled = settled.Add(reveal)
				}
				g.advanceAnimations(settled)
				if zone.value != "Bedroom" || zone.opacity(settled) != 1 {
					t.Fatal("zone did not finish its quick reveal")
				}
				expires := settled.Add(time.Second)
				g.advanceAnimations(now.Add(500 * time.Millisecond))
				g.applyScene(selection)
				if zone.expires != expires {
					t.Fatal("ordinary updates restarted the keyboard notice")
				}
				g.advanceAnimations(expires)
				g.advanceAnimations(expires.Add(duration / 2))
				g.applyScene(selection)
				if zone.value != "" {
					t.Fatal("notice did not hide after one fully visible second and its fade-out")
				}
				selection.ZoneSelection++
				g.applyScene(selection)
				if zone.value != "Bedroom" || zone.expires != g.now.Add(reveal+time.Second) {
					t.Fatal("selecting the same zone again did not restart the notice")
				}
			})
		}
	}
}

func TestKeyboardZoneLabelUsesConfiguredEasing(t *testing.T) {
	now := time.Unix(1, 0)
	ease, _ := FadeEasingByName("in-out-sine")
	zone := textTransition{duration: 500 * time.Millisecond, ease: ease}
	zone.show("Bedroom", now, time.Second)
	if zone.opacity(now) != 0 {
		t.Fatal("notice popped in instead of fading")
	}
	quarter := now.Add(25 * time.Millisecond)
	if math.Abs(zone.opacity(quarter)-ease(.25)) > 1e-9 {
		t.Fatal("quick reveal ignored the configured easing")
	}
	zone.advance(now.Add(100 * time.Millisecond))
	expires := now.Add(1100 * time.Millisecond)
	zone.advance(expires)
	if zone.phaseDuration() != zone.duration/2 {
		t.Fatal("dismissal did not use the standard text fade duration")
	}
	if math.Abs(zone.opacity(expires.Add(zone.duration/8))-(1-ease(.25))) > 1e-9 {
		t.Fatal("dismissal ignored the configured easing")
	}
}

func TestKeyboardZoneLabelRapidRetargeting(t *testing.T) {
	now := time.Unix(1, 0)
	zone := transition()
	zone.show("A", now, time.Second)
	zone.advance(now.Add(100 * time.Millisecond))
	zone.show("B", now.Add(200*time.Millisecond), time.Second)
	start := zone.start
	mid := now.Add(240 * time.Millisecond)
	before := zone.opacity(mid)
	zone.show("C", mid, time.Second)
	if zone.start != start || zone.opacity(mid) != before {
		t.Fatal("rapid selection restarted an unfinished fade or jumped opacity")
	}
	zone.advance(now.Add(300 * time.Millisecond))
	if zone.value != "C" {
		t.Fatal("fade revealed an obsolete selection")
	}
	mid = now.Add(340 * time.Millisecond)
	before = zone.opacity(mid)
	zone.show("D", mid, time.Second)
	if zone.opacity(mid) != before {
		t.Fatal("interrupting fade-in jumped opacity")
	}
	zone.show("E", now.Add(380*time.Millisecond), time.Second)
	zone.advance(now.Add(440 * time.Millisecond))
	zone.advance(now.Add(540 * time.Millisecond))
	if zone.value != "E" || zone.opacity(now.Add(540*time.Millisecond)) != 1 || zone.expires != now.Add(1540*time.Millisecond) {
		t.Fatal("rapid switching failed to settle on the latest zone for a full second")
	}
	// A new key press during dismissal must recover smoothly as well.
	zone.advance(zone.expires)
	mid = now.Add(1590 * time.Millisecond)
	before = zone.opacity(mid)
	zone.show("F", mid, time.Second)
	if zone.opacity(mid) != before {
		t.Fatal("interrupting dismissal jumped opacity")
	}
	zone.advance(now.Add(1790 * time.Millisecond))
	if zone.value != "F" || zone.opacity(now.Add(1790*time.Millisecond)) != 1 {
		t.Fatal("selection during dismissal did not reveal the latest zone")
	}
}
