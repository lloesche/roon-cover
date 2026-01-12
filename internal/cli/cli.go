package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
)

func Execute() int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
