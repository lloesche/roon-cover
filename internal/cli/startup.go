package cli

import (
	"context"
	"log/slog"
	"roon-cover/internal/display"
	"time"
)

// One worker owns startup and playback; retry never creates overlapping sessions.
// Kiosks have no input devices, so retries and progression are automatic.
func runStartup(ctx context.Context, scenes chan display.Update, retryDelay time.Duration, attempt func(func(display.Status)) error) error {
	status := func(s display.Status) { display.Publish(scenes, display.Update{Status: &s, NoFade: true}) }
	for ctx.Err() == nil {
		err := presentStartupAttempt(ctx, status, attempt)
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			return nil
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

// Network work runs ahead of presentation. Only the startup handoff waits for
// the ordered screens; a fast connection cannot overwrite unread phases.
func presentStartupAttempt(ctx context.Context, show func(display.Status), attempt func(func(display.Status)) error) error {
	phases := make(chan display.Status, 8)
	done := make(chan error, 1)
	var transcript []string
	go func() {
		report := func(s display.Status) {
			select {
			case phases <- s:
			case <-ctx.Done():
			}
		}
		report(display.Status{Title: "Looking for Roon…", Hold: 500 * time.Millisecond})
		err := attempt(report)
		close(phases)
		done <- err
	}()
	for {
		select {
		case <-ctx.Done():
			<-done
			return ctx.Err()
		case phase, ok := <-phases:
			if !ok {
				err := <-done
				if err == nil {
					showStartupPhase(ctx, show, display.Status{Lines: transcript, FadeOut: true})
				}
				return err
			}
			for _, line := range []string{phase.Title, phase.Detail, phase.Hint} {
				if line != "" {
					transcript = append(transcript, line)
				}
			}
			// Complete immutable snapshots survive coalescing in the renderer.
			phase.Lines = append([]string(nil), transcript...)
			showStartupPhase(ctx, show, phase)
		}
	}
}

func showStartupPhase(ctx context.Context, show func(display.Status), phase display.Status) {
	settled := make(chan struct{}, 1)
	phase.Settled = settled
	show(phase)
	select {
	case <-ctx.Done():
		return
	case <-settled:
	}
	waitStartup(ctx, phase.Hold)
}

func waitStartup(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
