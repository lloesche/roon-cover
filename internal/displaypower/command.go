// Package displaypower runs optional, user-configured display power commands.
package displaypower

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type CommandController struct {
	SleepCmd string
	WakeCmd  string
	Timeout  time.Duration
}

func (c CommandController) Sleep(ctx context.Context) error {
	return c.run(ctx, c.SleepCmd)
}

func (c CommandController) Wake(ctx context.Context) error {
	return c.run(ctx, c.WakeCmd)
}

func (c CommandController) run(ctx context.Context, command string) error {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := shellCommand(ctx, command)
	output := &boundedOutput{}
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.WaitDelay = time.Second
	if err := runShell(cmd); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("display power command: %w", ctx.Err())
		}
		return fmt.Errorf("display power command: %w; output: %s", err, output.String())
	}
	return nil
}

type boundedOutput struct {
	mu   sync.Mutex
	data []byte
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	remaining := 4096 - len(b.data)
	if remaining > 0 {
		b.data = append(b.data, p[:min(remaining, n)]...)
	}
	return n, nil
}
func (b *boundedOutput) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(b.data))
}
