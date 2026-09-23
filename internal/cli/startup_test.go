package cli

import (
	"context"
	"errors"
	"testing"
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
