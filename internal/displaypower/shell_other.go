//go:build !windows

package displaypower

import (
	"context"
	"os/exec"
)

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "/bin/sh", "-c", command)
}
