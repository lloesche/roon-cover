package display

import (
	"math"
	"testing"
)

func TestNormalizeEaseName(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"":            "",
		" linear ":    "linear",
		"NONE":        "linear",
		"in_out_sine": "in-out-sine",
		"in out sine": "in-out-sine",
		"inout-sine":  "in-out-sine",
		"inout-quad":  "in-out-quad",
	}
	for in, want := range cases {
		if got := normalizeEaseName(in); got != want {
			t.Fatalf("normalizeEaseName(%q) got=%q want=%q", in, got, want)
		}
	}
}

func TestEasingByName_DefaultAndUnknown(t *testing.T) {
	t.Parallel()

	// Empty name should fall back to default.
	f, err := EasingByName("")
	if err != nil {
		t.Fatalf("EasingByName(\"\") err=%v", err)
	}
	if f == nil {
		t.Fatalf("EasingByName(\"\") returned nil func")
	}

	if _, err := EasingByName("definitely-not-a-real-ease"); err == nil {
		t.Fatalf("expected error for unknown easing")
	}
}

func TestEasing_FunctionsReasonable(t *testing.T) {
	t.Parallel()

	// Sanity checks:
	// - all easings should map 0->0 and 1->1
	// - most should stay within [0,1], except elastic which intentionally overshoots a bit
	for _, name := range SupportedEasingNames() {
		f, err := EasingByName(name)
		if err != nil {
			t.Fatalf("EasingByName(%q) err=%v", name, err)
		}
		if got := f(0); math.Abs(got-0) > 1e-9 {
			t.Fatalf("%s: f(0)=%v want 0", name, got)
		}
		if got := f(1); math.Abs(got-1) > 1e-9 {
			t.Fatalf("%s: f(1)=%v want 1", name, got)
		}

		lo, hi := 0.0, 1.0
		if name == "out-elastic" {
			// Elastic overshoots by design.
			lo, hi = -0.25, 1.25
		}
		for _, t0 := range []float64{-1, 0, 0.1, 0.5, 0.9, 1, 2} {
			v := f(t0)
			if v < lo || v > hi || math.IsNaN(v) {
				t.Fatalf("%s: f(%v)=%v outside [%v,%v]", name, t0, v, lo, hi)
			}
		}
	}
}
