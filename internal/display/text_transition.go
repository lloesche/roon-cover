package display

import "time"

// textTransition is independent of textures and samples only its supplied clock.
// Duration is the total fade-out/fade-in duration of a replacement.
type textTransition struct {
	value, target, requested string
	phase                    textPhase
	start                    time.Time
	from                     float64
	duration, hold           time.Duration
	expires                  time.Time
	ease                     func(float64) float64
}

func (t *textTransition) opacity(now time.Time) float64 {
	if t.value == "" {
		return 0
	}
	if t.phase == textPhaseNone {
		return 1
	}
	half := t.duration / 2
	if half <= 0 {
		return 1
	}
	x := clamp01(float64(now.Sub(t.start)) / float64(half))
	e := t.ease(x)
	if t.phase == textPhaseFadeOut {
		return t.from * (1 - e)
	}
	return t.from + (1-t.from)*e
}
func (t *textTransition) set(value string, now time.Time, immediate bool) bool {
	changed := t.advance(now)
	if value == t.requested {
		return changed
	}
	t.requested = value
	t.target = value
	t.expires = time.Time{}
	if value == "" || immediate || t.duration <= 0 || t.value == "" {
		t.value = value
		t.phase = textPhaseNone
		t.scheduleExpiry(now)
		return true
	}
	t.from = t.opacity(now)
	t.start = now
	t.phase = textPhaseFadeOut
	if value == t.value {
		t.phase = textPhaseFadeIn
		t.scheduleExpiry(now)
	}
	return true
}
func (t *textTransition) scheduleExpiry(now time.Time) {
	if t.hold > 0 && t.value != "" {
		t.expires = now.Add(t.hold)
	}
}
func (t *textTransition) advance(now time.Time) bool {
	changed := t.phase != textPhaseNone
	if t.phase != textPhaseNone && now.Sub(t.start) >= t.duration/2 {
		if t.phase == textPhaseFadeOut {
			t.value = t.target
			t.start = t.start.Add(t.duration / 2)
			t.from = 0
			t.phase = textPhaseFadeIn
			t.scheduleExpiry(t.start)
		}
		if t.value == "" || now.Sub(t.start) >= t.duration/2 {
			t.phase = textPhaseNone
		}
	}
	if !t.expires.IsZero() && !now.Before(t.expires) {
		t.expires = time.Time{}
		t.target = ""
		changed = true
		if t.duration <= 0 {
			t.value = ""
			t.phase = textPhaseNone
		} else {
			t.from = t.opacity(now)
			t.start = now
			t.phase = textPhaseFadeOut
		}
	}
	return changed
}
