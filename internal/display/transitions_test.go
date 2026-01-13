package display

import "testing"

func TestAlphaU8(t *testing.T) {
	t.Parallel()

	if got := alphaU8(-1); got != 0 {
		t.Fatalf("alphaU8(-1)=%d", got)
	}
	if got := alphaU8(0); got != 0 {
		t.Fatalf("alphaU8(0)=%d", got)
	}
	if got := alphaU8(0.5); got != 128 { // round(127.5) = 128
		t.Fatalf("alphaU8(0.5)=%d", got)
	}
	if got := alphaU8(1); got != 255 {
		t.Fatalf("alphaU8(1)=%d", got)
	}
	if got := alphaU8(2); got != 255 {
		t.Fatalf("alphaU8(2)=%d", got)
	}
}

func TestCoverFadeAlphas_Linear(t *testing.T) {
	t.Parallel()

	linear := func(x float64) float64 { return x }

	p, c, done := coverFadeAlphas(0, linear)
	if done || p != 255 || c != 0 {
		t.Fatalf("t=0 got prev=%d curr=%d done=%v", p, c, done)
	}

	p, c, done = coverFadeAlphas(0.25, linear)
	if done || p != 191 || c != 64 {
		t.Fatalf("t=0.25 got prev=%d curr=%d done=%v", p, c, done)
	}

	p, c, done = coverFadeAlphas(1, linear)
	if !done || p != 0 || c != 255 {
		t.Fatalf("t=1 got prev=%d curr=%d done=%v", p, c, done)
	}
}

func TestTextIntensityAndAdvancePhase(t *testing.T) {
	t.Parallel()

	linear := func(x float64) float64 { return x }

	if got := textIntensity(textPhaseNone, 0.5, linear); got != 1 {
		t.Fatalf("none intensity=%v", got)
	}
	if got := textIntensity(textPhaseFadeOut, 0, linear); got != 1 {
		t.Fatalf("fadeout t=0 intensity=%v", got)
	}
	if got := textIntensity(textPhaseFadeOut, 0.5, linear); got != 0.5 {
		t.Fatalf("fadeout t=0.5 intensity=%v", got)
	}
	if got := textIntensity(textPhaseFadeIn, 0.5, linear); got != 0.5 {
		t.Fatalf("fadein t=0.5 intensity=%v", got)
	}

	next, swap := advanceTextPhase(textPhaseFadeOut, 0.9)
	if next != textPhaseFadeOut || swap {
		t.Fatalf("fadeout t<1 next=%v swap=%v", next, swap)
	}
	next, swap = advanceTextPhase(textPhaseFadeOut, 1.0)
	if next != textPhaseFadeIn || !swap {
		t.Fatalf("fadeout t>=1 next=%v swap=%v", next, swap)
	}
	next, swap = advanceTextPhase(textPhaseFadeIn, 1.0)
	if next != textPhaseNone || swap {
		t.Fatalf("fadein t>=1 next=%v swap=%v", next, swap)
	}
}
