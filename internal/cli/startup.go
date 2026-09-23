package cli

import (
	"context"
	"errors"
	"log/slog"
	"roon-cover/internal/display"
	"time"
)

// One worker owns startup and playback; retry never creates overlapping sessions.
// Kiosks have no input devices, so retries and progression are automatic.
func runStartup(ctx context.Context, scenes chan display.Update, retryDelay time.Duration, attempt func(func(display.Status)) error) error {
	status := func(s display.Status) { display.Publish(scenes, display.Update{Status: &s, NoFade: true}) }
	for ctx.Err() == nil {
		status(display.Status{Title: "Looking for Roon…", Detail: "Make sure your Roon Server is running."})
		err := attempt(status)
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			err = errors.New("the Roon session ended")
		}
		slog.Error("Unable to connect to Roon", "error", err)
		status(display.Status{Title: "Couldn't connect to Roon", Detail: err.Error(), Hint: "Trying again automatically…"})
		timer := time.NewTimer(retryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	return nil
}
