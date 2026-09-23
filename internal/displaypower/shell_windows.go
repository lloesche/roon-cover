package displaypower

import (
	"context"
	"os/exec"
	"syscall"
)

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "cmd.exe", "/d", "/s", "/c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}
