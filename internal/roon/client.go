package roon

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

type Client struct {
	store CredentialStore
	log   *slog.Logger
	cfg   Config
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

	if current := s.credentials(); current.PairedCoreID != "" {
		return current, nil
	}
	c.log.Info("waiting for roon pairing approval", "core", core.Name)
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	select {
	case <-waitCtx.Done():
		return Credentials{}, waitCtx.Err()
	case <-s.conn.closed:
		return Credentials{}, errors.New("roon: disconnected while waiting for pairing")
	case <-s.pairedSignal:
		current := s.credentials()
		if current.PairedCoreID == "" {
			return Credentials{}, errors.New("pairing signaled without a core")
		}
		return current, nil
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

func WithCredentialStore(store CredentialStore) Option { return func(c *Client) { c.store = store } }
func (c *Client) credentialStore() (CredentialStore, error) {
	if c.store != nil {
		return c.store, nil
	}
	return NewFileCredentialStore("roon-cover")
}
