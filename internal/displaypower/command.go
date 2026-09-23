// Package displaypower runs optional, user-configured display power commands.
package displaypower

import (
	"context"
	"fmt"
	"strings"
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
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("display power command: %w", ctx.Err())
		}
		return fmt.Errorf("display power command: %w", err)
	}
	return nil
}
