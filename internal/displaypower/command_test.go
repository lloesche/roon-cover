package displaypower

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCommands(t *testing.T) {
	c := CommandController{SleepCmd: "exit 0", WakeCmd: "exit 7"}
	if err := c.Sleep(context.Background()); err != nil {
		t.Fatal(err)
	}
	var exitErr *exec.ExitError
	if err := c.Wake(context.Background()); !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("expected exit code 7, got %v", err)
	}
}

func TestEmptyCommands(t *testing.T) {
	c := CommandController{SleepCmd: " \t"}
	if err := c.Sleep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Wake(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCanceledCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := CommandController{SleepCmd: "exit 0"}
	if err := c.Sleep(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestExpiredCommand(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	c := CommandController{WakeCmd: "exit 0"}
	if err := c.Wake(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestCommandDiagnosticsAndTimeout(t *testing.T) {
	c := CommandController{SleepCmd: "echo power-failed && exit 7"}
	if err := c.Sleep(context.Background()); err == nil || !strings.Contains(err.Error(), "power-failed") {
		t.Fatalf("missing command diagnostics: %v", err)
	}
	command := "sleep 30"
	if runtime.GOOS == "windows" {
		command = "ping -n 30 127.0.0.1 > nul"
	}
	c = CommandController{SleepCmd: command, Timeout: 200 * time.Millisecond}
	start := time.Now()
	if err := c.Sleep(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout, got %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("helper descendants delayed shutdown")
	}
}
