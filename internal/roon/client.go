package roon

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

type Client struct {
	log *slog.Logger
	cfg Config
}

type Option func(*Client)

func WithLogger(l *slog.Logger) Option {
	return func(c *Client) {
		if l != nil {
			c.log = l
		}
	}
}

func NewClient(cfg Config, opts ...Option) *Client {
	c := &Client{
		log: slog.Default(),
		cfg: cfg,
	}
	c.withDefaultLogger()
	c.cfg = applyConfigDefaults(c.cfg)
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) Pair(ctx context.Context, core Core) (Credentials, error) {
	// Pair is invoked when we don't have credentials yet; it will:
	// - connect + registry/register (obtains registry token)
	// - wait until user accepts the extension (pairing:1/pair)
	creds := Credentials{CoreKey: coreKey(core)}

	s, err := c.connectAndRegister(ctx, core, &creds)
	if err != nil {
		return Credentials{}, err
	}
	defer s.conn.Close()

	// If already paired (from stored creds), nothing to do.
	if creds.PairedCoreID != "" {
		return creds, nil
	}

	c.log.Info("waiting for roon pairing approval", "core", core.Name)

	// Wait for the Core to call com.roonlabs.pairing:1/pair after user approval.
	waitCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
	}

	for {
		// Fast path if the pairing request was handled before we started waiting.
		if creds.PairedCoreID != "" {
			c.log.Info("pairing approved", "core_id", creds.PairedCoreID)
			return creds, nil
		}

		select {
		case <-waitCtx.Done():
			return Credentials{}, waitCtx.Err()
		case <-s.pairedSignal:
			if creds.PairedCoreID == "" {
				return Credentials{}, errors.New("pairing signaled but paired_core_id is empty")
			}
			c.log.Info("pairing approved", "core_id", creds.PairedCoreID)
			return creds, nil
		}
	}
}

type ImageFetchOptions struct {
	// Size is the target square dimension in pixels (e.g. 600, 800).
	//
	// Implemented in `internal/roon/image.go` using Roon's HTTP endpoint:
	//   /api/image/<image_key>?scale=fit&width=<Size>&height=<Size>&format=image/jpeg
	//
	// If Size is 0, the original size is requested (Roon may return a large image).
	Size int
}

type ZoneUpdate struct {
	Zones []Zone
}
