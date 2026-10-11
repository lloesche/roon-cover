package roon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
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

	log   *slog.Logger
	creds *Credentials
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
		return json.Unmarshal(f.BodyRaw, &info)
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
	lookupCore := core
	core.ID = CoreID(info.CoreID)
	store, err := c.credentialStore()
	if err != nil {
		return nil, err
	}
	creds, found, err := store.Load(ctx, core)
	if err != nil {
		return nil, err
	}
	// Older explicit-address configurations saved under the supplied name.
	// Migrate only credentials that identify this exact Core.
	_, _, explicitErr := net.SplitHostPort(lookupCore.Name)
	if !found && lookupCore.ID == "" && explicitErr == nil {
		legacy, ok, loadErr := store.Load(ctx, lookupCore)
		if loadErr != nil {
			return nil, loadErr
		}
		if ok && legacy.PairedCoreID == core.ID {
			creds = legacy
		}
	}
	s := &session{core: core, conn: conn, log: c.log, creds: &creds}
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
		if err := json.Unmarshal(f.BodyRaw, &reg); err != nil {
			return err
		}
		if reg.CoreID != string(core.ID) || reg.Token == "" {
			return errors.New("roon: invalid registration identity or token")
		}
		s.mu.Lock()
		if reg.DisplayName != "" {
			s.core.Name = reg.DisplayName
		}
		previous := *s.creds
		s.creds.RegistryToken = reg.Token
		s.creds.CoreKey = coreKey(s.core)
		// As in node-roon-api, initial authorization automatically pairs this Core.
		s.creds.PairedCoreID = s.core.ID
		savedCore, savedCreds := s.core, *s.creds
		s.mu.Unlock()
		// Skip unchanged writes to spare SD cards on every reconnect. A failed
		// save must not end an authorized session: kiosks may run read-only.
		if !found || !sameCredentials(previous, savedCreds) {
			if err := store.Save(ctx, savedCore, savedCreds); err != nil {
				c.log.Warn("save Roon credentials failed; authorization will not survive a restart", "error", err)
			}
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
	// Roon sends nothing while approval is pending, so only elapsed time
	// suggests it. A stored token normally registers at once; allow a slow
	// Core (for example, one still starting) longer before asking the user.
	promptDelay := unpairedPromptDelay
	if creds.RegistryToken != "" {
		promptDelay = pairedPromptDelay
	}
	prompt := time.NewTimer(promptDelay)
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

var (
	unpairedPromptDelay = time.Second
	pairedPromptDelay   = 10 * time.Second
)

func sameCredentials(a, b Credentials) bool {
	return a.RegistryToken == b.RegistryToken && a.CoreKey == b.CoreKey && a.PairedCoreID == b.PairedCoreID
}

func (s *session) coreSnapshot() Core { s.mu.Lock(); defer s.mu.Unlock(); return s.core }
