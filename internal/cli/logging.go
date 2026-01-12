package cli

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
)

var globalLogger atomic.Pointer[slog.Logger]

func logger() *slog.Logger {
	if l := globalLogger.Load(); l != nil {
		return l
	}
	// Safe fallback; should be replaced during PersistentPreRunE.
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func initLogging(format string, level string) error {
	lvl, err := parseLevel(level)
	if err != nil {
		return err
	}

	var w io.Writer = os.Stderr
	var handler slog.Handler

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "text", "":
		handler = slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl})
	case "json":
		handler = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lvl})
	default:
		return fmt.Errorf("log-format: unsupported value %q (use text|json)", format)
	}

	l := slog.New(handler)
	globalLogger.Store(l)
	slog.SetDefault(l)
	return nil
}

func parseLevel(s string) (slog.Leveler, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return nil, errors.New("log-level: invalid value (use debug|info|warn|error)")
	}
}
