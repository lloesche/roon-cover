package roon

import (
	"log/slog"
	"net/http"
	"time"
)

type Client struct {
	imageClient *http.Client
	store       CredentialStore
	log         *slog.Logger
	cfg         Config
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
		imageClient: &http.Client{Timeout: 10 * time.Second},
		log:         slog.Default(),
		cfg:         cfg,
	}
	c.withDefaultLogger()
	c.cfg = applyConfigDefaults(c.cfg)
	for _, opt := range opts {
		opt(c)
	}
	return c
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
