package cli

import (
	"log/slog"
	"sync/atomic"
)

// SquareSize tracks the desired square cover size (pixels) based on the current render output.
// It is safe for concurrent use.
type SquareSize struct {
	px atomic.Int64
}

func (s *SquareSize) Get() int {
	v := int(s.px.Load())
	if v <= 0 {
		return 800
	}
	return v
}

// UpdateFromOutput sets the size based on the shortest edge.
func (s *SquareSize) UpdateFromOutput(log *slog.Logger, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	n := w
	if h < n {
		n = h
	}
	// Clamp: avoid tiny or absurdly large requests.
	if n < 200 {
		n = 200
	}
	if n > 4096 {
		n = 4096
	}

	prev := int(s.px.Swap(int64(n)))
	if prev != n && log != nil {
		log.Info("cover fetch size updated", "square_size", n, "prev", prev, "w", w, "h", h)
	}
}
