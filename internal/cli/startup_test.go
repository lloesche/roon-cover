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
		done <- runStartup(ctx, scenes, 30*time.Millisecond, 0, func(status func(display.Status)) error {
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
		done <- runStartup(ctx, scenes, time.Hour, 0, func(func(display.Status)) error { return errors.New("offline") })
	}()
	for {
		select {
		case s := <-scenes:
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
		var titles []string
		err := presentStartupAttempt(context.Background(), func(s display.Status) {
			if got := time.Since(start); got != time.Duration(len(titles))*time.Second {
				t.Fatalf("phase %q shown at %v", s.Title, got)
			}
			if len(titles) > 0 {
				select {
				case <-workDone:
				default:
					t.Fatal("presentation delayed connection work")
				}
			}
			if len(s.Lines) < len(titles)+1 || s.Lines[0] != "Looking for Roon…" {
				t.Fatal("startup history was replaced instead of appended")
			}
			titles = append(titles, s.Title)
		}, time.Second, func(report func(display.Status)) error {
			for _, title := range []string{"Found", "Connected", "Using Dialysis"} {
				report(display.Status{Title: title})
			}
			close(workDone)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"Looking for Roon…", "Found", "Connected", "Using Dialysis"}
		if len(titles) != len(want) {
			t.Fatal(titles)
		}
		for i := range want {
			if titles[i] != want[i] {
				t.Fatal(titles)
			}
		}
		if time.Since(start) != 5*time.Second {
			t.Fatal("final zone announcement must last two seconds")
		}
	})
}

func TestStartupPhaseWaitCancelsPromptly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		start := time.Now()
		err := presentStartupAttempt(ctx, func(display.Status) {}, time.Second, func(func(display.Status)) error {
			<-ctx.Done()
			return ctx.Err()
		})
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != 100*time.Millisecond {
			t.Fatalf("cancellation delayed: %v, %v", err, time.Since(start))
		}
	})
}
