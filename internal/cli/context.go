package cli

import (
	"context"
	"log/slog"
)

type loggerKey struct{}

func withLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

func LoggerFromContext(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return logger()
	}
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return logger()
}
