package display

import (
	"fmt"
	"testing"
	"time"
)

func TestKeyboardZoneLabelIgnoresOverlayFlagsAndExpires(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, duration := range []time.Duration{0, 500 * time.Millisecond} {
			t.Run(fmt.Sprintf("show-zone=%t/fade=%s", enabled, duration), func(t *testing.T) {
				now := time.Unix(1, 0)
				g := windowGame{now: now, enabled: [4]bool{false, false, false, enabled}}
				g.lines[3].animation = textTransition{duration: duration, hold: 5 * time.Second, ease: func(x float64) float64 { return x }}
				g.applyScene(Update{Zone: "Living room", NowPlaying: &Metadata{Title: "Hidden title"}})
				if !enabled && g.lines[3].animation.value != "" {
					t.Fatal("ordinary zone snapshot enabled an overlay")
				}
				selection := Update{Zone: "Bedroom", ZoneSelection: 1}
				g.applyScene(selection)
				zone := &g.lines[3].animation
				if zone.value != "Bedroom" || zone.opacity(now) != 1 || g.lines[0].animation.value != "" {
					t.Fatal("keyboard selection must reveal only the zone immediately, even without artwork")
				}
				expires := now.Add(time.Second)
				g.advanceAnimations(now.Add(500 * time.Millisecond))
				g.applyScene(selection)
				if zone.expires != expires {
					t.Fatal("ordinary updates restarted the keyboard notice")
				}
				g.advanceAnimations(expires)
				g.advanceAnimations(expires.Add(duration / 2))
				g.applyScene(selection)
				if zone.value != "" {
					t.Fatal("notice did not hide after one second and its fade-out")
				}
				selection.ZoneSelection++
				g.applyScene(selection)
				if zone.value != "Bedroom" || zone.expires != g.now.Add(time.Second) {
					t.Fatal("selecting the same zone again did not restart the notice")
				}
			})
		}
	}
}
