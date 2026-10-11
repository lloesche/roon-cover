package roon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	svcRegistry  = "com.roonlabs.registry:1"
	svcPing      = "com.roonlabs.ping:1"
	svcPairing   = "com.roonlabs.pairing:1"
	svcTransport = "com.roonlabs.transport:1"
	svcImage     = "com.roonlabs.image:1"
)

type registryInfoResponse struct {
	CoreID string `json:"core_id"`
}

type registryRegisterRequest struct {
	ExtensionID    string `json:"extension_id"`
	DisplayName    string `json:"display_name"`
	DisplayVersion string `json:"display_version"`
	Publisher      string `json:"publisher"`
	Email          string `json:"email"`
	Website        string `json:"website,omitempty"`

	RequiredServices []string `json:"required_services"`
	OptionalServices []string `json:"optional_services"`
	ProvidedServices []string `json:"provided_services"`

	// Token is optional; when present it allows re-registering with the same identity.
	Token string `json:"token,omitempty"`
}

type registryRegistered struct {
	CoreID         string `json:"core_id"`
	DisplayName    string `json:"display_name"`
	DisplayVersion string `json:"display_version"`

	ProvidedServices []string `json:"provided_services"`

	Token string `json:"token"`
}

type session struct {
	mu   sync.Mutex
	core Core
	conn *mooConn

	log          *slog.Logger
	pairedSignal chan struct{}
	pairedOnce   sync.Once
	creds        *Credentials
}

func (c *Client) connectAndRegister(ctx context.Context, core Core, onAuthorization func()) (*session, error) {
	if core.Host == "" || core.Port == 0 {
		return nil, errors.New("roon: core missing host/port")
	}
	conn, err := dialMoo(ctx, c.log, core.Host, core.Port)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			conn.Close()
		}
	}()
	var info registryInfoResponse
	err = conn.Call(ctx, svcRegistry+"/info", nil, func(f *mooFrame) error {
		if f.ResponseName != "Success" {
			return fmt.Errorf("registry info: %s", f.ResponseName)
		}
		return jsonUnmarshal(f.BodyRaw, &info)
	})
	if err != nil {
		return nil, err
	}
	if info.CoreID == "" {
		return nil, errors.New("roon: registry info missing core id")
	}
	if core.ID != "" && core.ID != CoreID(info.CoreID) {
		return nil, errors.New("roon: endpoint belongs to a different core")
	}
	core.ID = CoreID(info.CoreID)
	store, err := c.credentialStore()
	if err != nil {
		return nil, err
	}
	creds, _, err := store.Load(ctx, core)
	if err != nil {
		return nil, err
	}
	s := &session{core: core, conn: conn, log: c.log, pairedSignal: make(chan struct{}), creds: &creds}
	s.registerPingService()
	s.registerPairingService()
	request := registryRegisterRequest{
		ExtensionID: c.cfg.ExtensionID, DisplayName: c.cfg.DisplayName, DisplayVersion: c.cfg.DisplayVersion,
		Publisher: c.cfg.Publisher, Email: c.cfg.Email, Website: c.cfg.Website,
		RequiredServices: []string{svcTransport, svcImage}, OptionalServices: []string{},
		ProvidedServices: []string{svcPairing, svcPing}, Token: creds.RegistryToken,
	}
	registered := make(chan struct{}, 1)
	// Registration is a long-lived CONTINUE stream: COMPLETE Unregistered must
	// invalidate the session even after initial authorization succeeds.
	err = conn.Subscribe(ctx, svcRegistry+"/register", request, func(f *mooFrame) error {
		if f.ResponseName != "Registered" {
			return fmt.Errorf("roon: registration ended: %s", f.ResponseName)
		}
		var reg registryRegistered
		if err := jsonUnmarshal(f.BodyRaw, &reg); err != nil {
			return err
		}
		if reg.CoreID != string(core.ID) || reg.Token == "" {
			return errors.New("roon: invalid registration identity or token")
		}
		s.mu.Lock()
		if reg.DisplayName != "" {
			s.core.Name = reg.DisplayName
		}
		s.creds.RegistryToken = reg.Token
		s.creds.CoreKey = coreKey(s.core)
		// As in node-roon-api, initial authorization automatically pairs this Core.
		s.creds.PairedCoreID = s.core.ID
		savedCore, savedCreds := s.core, *s.creds
		s.mu.Unlock()
		if err := store.Save(ctx, savedCore, savedCreds); err != nil {
			return err
		}
		select {
		case registered <- struct{}{}:
		default:
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	prompt := time.NewTimer(time.Second)
	defer prompt.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-conn.closed:
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, errors.New("roon: disconnected during registration")
		case <-prompt.C:
			if onAuthorization != nil {
				onAuthorization()
			}
		case <-registered:
			success = true
			return s, nil
		}
	}
}

func (s *session) credentials() Credentials { s.mu.Lock(); defer s.mu.Unlock(); return *s.creds }
func (s *session) coreSnapshot() Core       { s.mu.Lock(); defer s.mu.Unlock(); return s.core }
