package roon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
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
	core Core
	conn *mooConn

	log *slog.Logger

	pairedCh     chan CoreID // deprecated; replaced by pairedSignal (kept temporarily)
	pairedSignal chan struct{}
	pairedOnce   sync.Once
	creds        *Credentials
}

func (c *Client) connectAndRegister(ctx context.Context, core Core, creds *Credentials) (*session, error) {
	if core.Host == "" || core.Port == 0 {
		return nil, errors.New("roon: core missing host/port (did discovery run?)")
	}

	log := c.log
	if log == nil {
		log = slog.Default()
	}

	log.Debug("dialing core websocket", "host", core.Host, "port", core.Port)
	conn, err := dialMoo(ctx, log, core.Host, core.Port)
	if err != nil {
		return nil, err
	}

	s := &session{
		core:         core,
		conn:         conn,
		log:          log,
		pairedCh:     make(chan CoreID, 1),
		pairedSignal: make(chan struct{}),
		creds:        creds,
	}

	// Provided services so the Core can ping us and pair us.
	s.registerPingService()
	s.registerPairingService()

	// 1) registry info
	var info registryInfoResponse
	log.Debug("calling registry/info")
	err = conn.Call(ctx, svcRegistry+"/info", nil, func(f *mooFrame) error {
		if f.ResponseName != "Success" {
			return fmt.Errorf("registry info failed: %s", f.ResponseName)
		}
		if len(f.BodyRaw) == 0 {
			return errors.New("registry info: empty body")
		}
		return jsonUnmarshal(f.BodyRaw, &info)
	})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if info.CoreID != "" && core.ID == "" {
		core.ID = CoreID(info.CoreID)
		s.core.ID = core.ID
	}

	// 2) registry register (with optional token)
	log.Debug("calling registry/register", "has_token", creds.RegistryToken != "")
	regReq := registryRegisterRequest{
		ExtensionID:    c.cfg.ExtensionID,
		DisplayName:    c.cfg.DisplayName,
		DisplayVersion: c.cfg.DisplayVersion,
		Publisher:      c.cfg.Publisher,
		Email:          c.cfg.Email,
		Website:        c.cfg.Website,

		RequiredServices: []string{
			svcTransport,
			svcImage,
		},
		OptionalServices: []string{},
		ProvidedServices: []string{
			svcPairing,
			svcPing,
		},
		Token: creds.RegistryToken,
	}

	var reg registryRegistered
	err = conn.Call(ctx, svcRegistry+"/register", regReq, func(f *mooFrame) error {
		if f.ResponseName != "Registered" {
			return fmt.Errorf("registry register failed: %s", f.ResponseName)
		}
		if len(f.BodyRaw) == 0 {
			return errors.New("registry register: empty body")
		}
		return jsonUnmarshal(f.BodyRaw, &reg)
	})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if reg.Token != "" {
		creds.RegistryToken = reg.Token
	}
	if reg.CoreID != "" {
		s.core.ID = CoreID(reg.CoreID)
	}
	creds.CoreKey = coreKey(s.core)
	log.Debug("registered", "core_id", s.core.ID, "has_token", creds.RegistryToken != "")

	return s, nil
}
