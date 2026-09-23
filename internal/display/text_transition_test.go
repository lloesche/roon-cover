package display

import (
	"math"
	"testing"
	"time"
)

func transition() textTransition {
	return textTransition{duration: 200 * time.Millisecond, ease: func(x float64) float64 { return x }}
}
func TestRepeatedTargetAndReversal(t *testing.T) {
	now := time.Unix(1, 0)
	tr := transition()
	tr.set("A", now, true)
	tr.set("B", now, false)
	mid := now.Add(50 * time.Millisecond)
	before := tr.opacity(mid)
	start := tr.start
	tr.set("B", mid, false)
	if tr.start != start || math.Abs(tr.opacity(mid)-before) > 1e-9 {
		t.Fatal("same target restarted transition")
	}
	tr.set("A", mid, false)
	if math.Abs(tr.opacity(mid)-before) > 1e-9 {
		t.Fatal("reversal changed opacity")
	}
	tr.advance(now.Add(time.Second))
	if tr.value != "A" {
		t.Fatal("obsolete target retained")
	}
}
func TestDurationAndExpiryWithoutAnimation(t *testing.T) {
	now := time.Unix(1, 0)
	tr := transition()
	tr.set("A", now, true)
	tr.set("B", now, false)
	tr.advance(now.Add(200 * time.Millisecond))
	if tr.value != "B" || tr.phase != textPhaseNone {
		t.Fatal("replacement exceeds configured duration")
	}
	tr = textTransition{hold: 5 * time.Second}
	tr.set("Zone", now, false)
	tr.advance(now.Add(5 * time.Second))
	if tr.value != "" {
		t.Fatal("zero fade prevented expiry")
	}
	tr.set("Zone", now.Add(6*time.Second), false)
	if tr.value != "" {
		t.Fatal("unchanged zone redisplayed after expiry")
	}
}

func TestFadeEasingRejectsOvershoot(t *testing.T) {
	for _, name := range []string{"out-elastic", "out-bounce", "in-bounce", "in-out-bounce"} {
		if _, err := FadeEasingByName(name); err == nil {
			t.Fatalf("accepted nonmonotonic opacity curve %s", name)
		}
	}
}

func TestExpiryOnRepeatedUpdateIsReported(t *testing.T) {
	now := time.Unix(1, 0)
	tr := textTransition{hold: time.Second}
	tr.set("Zone", now, false)
	if !tr.set("Zone", now.Add(time.Second), false) || tr.value != "" {
		t.Fatal("expiry must invalidate the scene even with unchanged metadata")
	}
}
