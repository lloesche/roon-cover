package cli

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"roon-cover/internal/display"
)

func TestStartupRetriesWithoutInputAndShowsPairing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	scenes := make(chan display.Update, 1)
	done := make(chan error, 1)
	attempts := 0
	go func() {
		done <- runStartup(ctx, scenes, 30*time.Millisecond, func(status func(display.Status)) error {
			attempts++
			if attempts == 1 {
				return errors.New("server unavailable")
			}
			status(display.Status{Title: "Approve in Roon", Detail: "Settings → Extensions"})
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	seenFailure := false
	for {
		select {
		case s := <-scenes:
			if s.Status != nil && s.Status.Settled != nil {
				s.Status.Settled <- struct{}{}
			}
			if s.Status == nil {
				t.Fatal("startup published a blank scene")
			}
			if s.Status.Title == "Couldn't connect to Roon" {
				seenFailure = true
			}
			if s.Status.Title == "Approve in Roon" {
				cancel()
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				if !seenFailure || attempts != 2 {
					t.Fatal("retry did not follow the failure screen")
				}
				return
			}
		case <-ctx.Done():
			t.Fatal("startup did not retry automatically")
		}
	}
}

func TestStartupCancellationDoesNotWaitForRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scenes := make(chan display.Update, 1)
	done := make(chan error, 1)
	go func() {
		done <- runStartup(ctx, scenes, time.Hour, func(func(display.Status)) error { return errors.New("offline") })
	}()
	for {
		select {
		case s := <-scenes:
			if s.Status != nil && s.Status.Settled != nil {
				s.Status.Settled <- struct{}{}
			}
			if s.Status.Title == "Couldn't connect to Roon" {
				cancel()
				select {
				case <-done:
					return
				case <-time.After(time.Second):
					t.Fatal("cancel stuck in retry delay")
				}
			}
		case <-time.After(time.Second):
			t.Fatal("no failure screen")
		}
	}
}

func TestConsolePairCommandRemoved(t *testing.T) {
	for _, cmd := range newRoonCmd().Commands() {
		if cmd.Name() == "pair" {
			t.Fatal("console pairing is still registered")
		}
	}
}

func TestStartupPhasesAreOrderedAndNetworkRunsAhead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		workDone := make(chan struct{})
		want := []string{"Looking for Roon…", "Found blackhole", "Connecting to blackhole…", "Connected", "Choosing listening zone…", "Dialysis", ""}
		at := []time.Duration{0, time.Second, 2500 * time.Millisecond, 3500 * time.Millisecond, 5 * time.Second, 6 * time.Second, 8500 * time.Millisecond}
		index := 0
		err := presentStartupAttempt(context.Background(), func(s display.Status) {
			if index >= len(want) || s.Title != want[index] || time.Since(start) != at[index] {
				t.Fatalf("unexpected phase %d: %q at %v", index, s.Title, time.Since(start))
			}
			if index > 0 {
				select {
				case <-workDone:
				default:
					t.Fatal("presentation delayed network work")
				}
			}
			count := min(index+1, 6)
			if len(s.Lines) != count || s.Lines[0] != want[0] {
				t.Fatal("startup history lost")
			}
			if s.FadeOut != (index == 6) {
				t.Fatal("fade-out must follow the zone hold")
			}
			index++
			go func() { time.Sleep(500 * time.Millisecond); s.Settled <- struct{}{} }()
		}, func(report func(display.Status)) error {
			holds := []time.Duration{time.Second, 500 * time.Millisecond, time.Second, 500 * time.Millisecond, 2 * time.Second}
			for i, title := range want[1:6] {
				report(display.Status{Title: title, Hold: holds[i]})
			}
			close(workDone)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if index != len(want) || time.Since(start) != 9*time.Second {
			t.Fatal("hold times must follow completed fades")
		}
	})
}
func TestStartupPhaseWaitCancelsPromptly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		start := time.Now()
		err := presentStartupAttempt(ctx, func(display.Status) {}, func(func(display.Status)) error {
			<-ctx.Done()
			return ctx.Err()
		})
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != 100*time.Millisecond {
			t.Fatalf("cancellation delayed: %v, %v", err, time.Since(start))
		}
	})
}
