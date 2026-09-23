package display

import "math"

// textPhase is the fade phase for a text line.
// It intentionally mirrors the existing 0/1/2 phase encoding used by the SDL renderer.
type textPhase int

const (
	textPhaseNone    textPhase = 0
	textPhaseFadeOut textPhase = 1
	textPhaseFadeIn  textPhase = 2
)

func alphaU8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return uint8(math.Round(v * 255))
}

// coverFadeAlphas returns the (prev, curr) alpha values for a cover crossfade at time t in [0,1].
func coverFadeAlphas(t float64, ease func(float64) float64) (prev, curr uint8, done bool) {
	if t >= 1 {
		return 0, 255, true
	}
	if t <= 0 {
		return 255, 0, false
	}
	e := clamp01(ease(t))
	return 255, alphaU8(e), false
}

// textIntensity returns the visibility (0..1) for the current text texture for a given phase.
func textIntensity(phase textPhase, t float64, ease func(float64) float64) float64 {
	if phase == textPhaseNone {
		return 1
	}
	e := clamp01(ease(t))
	switch phase {
	case textPhaseFadeOut:
		return 1 - e
	case textPhaseFadeIn:
		return e
	default:
		return 1
	}
}

// advanceTextPhase advances a fade-out-in text transition when the phase's t reaches 1.
// swap=true means "fade-out completed; swap pending->current and start fade-in".
func advanceTextPhase(phase textPhase, t float64) (next textPhase, swap bool) {
	if t < 1 {
		return phase, false
	}
	switch phase {
	case textPhaseFadeOut:
		return textPhaseFadeIn, true
	case textPhaseFadeIn:
		return textPhaseNone, false
	default:
		return textPhaseNone, false
	}
}
