package display

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// EasingByName returns an easing function that maps t in [0,1] to [0,1].
// Names are case-insensitive and may use '-' or '_' separators (e.g. "in-out-sine", "in_out_sine").
func EasingByName(name string) (func(t float64) float64, error) {
	n := normalizeEaseName(name)
	if n == "" {
		n = "in-out-sine"
	}
	if f, ok := easingByName[n]; ok {
		return f, nil
	}
	return nil, fmt.Errorf("unknown easing %q (supported: %s)", name, strings.Join(SupportedEasingNames(), ", "))
}

func SupportedEasingNames() []string {
	out := make([]string, 0, len(easingByName))
	for k := range easingByName {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func normalizeEaseName(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "--", "-")
	s = strings.Trim(s, "-")
	// Common aliases.
	switch s {
	case "linear", "none":
		return "linear"
	case "inout-sine":
		return "in-out-sine"
	case "inout-quad":
		return "in-out-quad"
	case "inout-cubic":
		return "in-out-cubic"
	case "inout-expo":
		return "in-out-expo"
	case "inout-circ":
		return "in-out-circ"
	case "inout-bounce":
		return "in-out-bounce"
	}
	return s
}

func clamp01(t float64) float64 {
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t
}

var easingByName = map[string]func(t float64) float64{
	"linear": func(t float64) float64 { return clamp01(t) },

	// Sine
	"in-sine": func(t float64) float64 {
		t = clamp01(t)
		return 1 - math.Cos((t*math.Pi)/2)
	},
	"out-sine": func(t float64) float64 {
		t = clamp01(t)
		return math.Sin((t * math.Pi) / 2)
	},
	"in-out-sine": func(t float64) float64 {
		t = clamp01(t)
		return -(math.Cos(math.Pi*t) - 1) / 2
	},

	// Quad
	"in-quad": func(t float64) float64 {
		t = clamp01(t)
		return t * t
	},
	"out-quad": func(t float64) float64 {
		t = clamp01(t)
		return 1 - (1-t)*(1-t)
	},
	"in-out-quad": func(t float64) float64 {
		t = clamp01(t)
		if t < 0.5 {
			return 2 * t * t
		}
		return 1 - math.Pow(-2*t+2, 2)/2
	},

	// Cubic
	"in-cubic": func(t float64) float64 {
		t = clamp01(t)
		return t * t * t
	},
	"out-cubic": func(t float64) float64 {
		t = clamp01(t)
		return 1 - math.Pow(1-t, 3)
	},
	"in-out-cubic": func(t float64) float64 {
		t = clamp01(t)
		if t < 0.5 {
			return 4 * t * t * t
		}
		return 1 - math.Pow(-2*t+2, 3)/2
	},

	// Expo
	"in-expo": func(t float64) float64 {
		t = clamp01(t)
		if t == 0 {
			return 0
		}
		return math.Pow(2, 10*t-10)
	},
	"out-expo": func(t float64) float64 {
		t = clamp01(t)
		if t == 1 {
			return 1
		}
		return 1 - math.Pow(2, -10*t)
	},
	"in-out-expo": func(t float64) float64 {
		t = clamp01(t)
		if t == 0 {
			return 0
		}
		if t == 1 {
			return 1
		}
		if t < 0.5 {
			return math.Pow(2, 20*t-10) / 2
		}
		return (2 - math.Pow(2, -20*t+10)) / 2
	},

	// Circ
	"in-circ": func(t float64) float64 {
		t = clamp01(t)
		return 1 - math.Sqrt(1-math.Pow(t, 2))
	},
	"out-circ": func(t float64) float64 {
		t = clamp01(t)
		return math.Sqrt(1 - math.Pow(t-1, 2))
	},
	"in-out-circ": func(t float64) float64 {
		t = clamp01(t)
		if t < 0.5 {
			return (1 - math.Sqrt(1-math.Pow(2*t, 2))) / 2
		}
		return (math.Sqrt(1-math.Pow(-2*t+2, 2)) + 1) / 2
	},

	// Elastic (out only for now; useful for playful transitions)
	"out-elastic": func(t float64) float64 {
		t = clamp01(t)
		if t == 0 {
			return 0
		}
		if t == 1 {
			return 1
		}
		c4 := (2 * math.Pi) / 3
		return math.Pow(2, -10*t)*math.Sin((t*10-0.75)*c4) + 1
	},

	// Bounce
	"out-bounce":    easeOutBounce,
	"in-bounce":     easeInBounce,
	"in-out-bounce": easeInOutBounce,
}

func easeOutBounce(t float64) float64 {
	t = clamp01(t)
	n1 := 7.5625
	d1 := 2.75
	if t < 1/d1 {
		return n1 * t * t
	}
	if t < 2/d1 {
		t -= 1.5 / d1
		return n1*t*t + 0.75
	}
	if t < 2.5/d1 {
		t -= 2.25 / d1
		return n1*t*t + 0.9375
	}
	t -= 2.625 / d1
	return n1*t*t + 0.984375
}

func easeInBounce(t float64) float64 {
	t = clamp01(t)
	return 1 - easeOutBounce(1-t)
}

func easeInOutBounce(t float64) float64 {
	t = clamp01(t)
	if t < 0.5 {
		return (1 - easeOutBounce(1-2*t)) / 2
	}
	return (1 + easeOutBounce(2*t-1)) / 2
}
