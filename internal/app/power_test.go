package app

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestPowerRestoredOnShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := make(chan bool, 3)
	c := Controller{Log: slog.Default(), Options: Options{SleepAfter: time.Second}, Power: func(ctx context.Context, sleep bool) error { calls <- sleep; return nil }}
	desired := make(chan bool, 1)
	done := make(chan struct{})
	go func() { defer close(done); c.runPower(ctx, desired) }()
	desired <- true
	select {
	case sleep := <-calls:
		if !sleep {
			t.Fatal("expected sleep")
		}
	case <-time.After(time.Second):
		t.Fatal("sleep not applied")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown blocked")
	}
	select {
	case sleep := <-calls:
		if sleep {
			t.Fatal("expected wake")
		}
	default:
		t.Fatal("display left asleep")
	}
}
