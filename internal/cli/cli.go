package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

func Execute() int {
	// Explorer launches are valid: the default command opens the cover display.
	cobra.MousetrapHelpText = ""
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd := newRootCmd(ctx)
	if err := cmd.Execute(); err != nil {
		// Cobra already prints usage for many errors; keep this minimal.
		var exitErr interface{ ExitCode() int }
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}

		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	return 0
}
