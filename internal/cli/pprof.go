package cli

import (
	"errors"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
)

func startPprof(addr string, log *slog.Logger) error {
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

	return nil
}
