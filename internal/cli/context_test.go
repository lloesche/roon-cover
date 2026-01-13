package cli

import (
	"context"
	"log/slog"
	"testing"
)

func TestLoggerFromContext_Default(t *testing.T) {
	t.Parallel()

	l := LoggerFromContext(context.Background())
	if l == nil {
		t.Fatalf("expected non-nil logger")
	}
}

func TestLoggerFromContext_Stored(t *testing.T) {
	t.Parallel()

	want := slog.New(slog.NewTextHandler(nil, nil))
	ctx := withLogger(context.Background(), want)
	got := LoggerFromContext(ctx)
	if got != want {
		t.Fatalf("expected same logger instance")
	}
}
