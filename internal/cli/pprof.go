package cli

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"time"
)

func startPprof(ctx context.Context, addr string, log *slog.Logger) error {
	if addr == "" {
		return nil
	}
	if log == nil {
		log = slog.Default()
	}

	srv := &http.Server{
		Addr: addr,
	}

	go func() {
		log.Info("pprof server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("pprof server error", "err", err)
		}
	}()

	go func() {
		if ctx == nil {
			return
		}
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	return nil
}
