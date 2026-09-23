package roon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Credentials struct {
	// CoreKey is the key we used when saving credentials (CoreID if available, else Core name).
	CoreKey string `json:"core_key"`

	// RegistryToken is returned by com.roonlabs.registry:1/register and allows
	// reconnecting without having to re-register as a "new" extension.
	RegistryToken string `json:"registry_token"`

	// PairedCoreID is set once the user accepts/pairs the extension in Roon.
	PairedCoreID CoreID `json:"paired_core_id"`

	CreatedAt time.Time `json:"created_at"`
}

type CredentialStore interface {
	Load(ctx context.Context, core Core) (creds Credentials, ok bool, err error)
	Save(ctx context.Context, core Core, creds Credentials) error
}

var credentialFileMu sync.Mutex

type fileCredentialStore struct {
	path string
}

// Path returns the full path to the credentials file on disk.
func (s *fileCredentialStore) Path() string { return s.path }

func NewFileCredentialStore(appName string) (CredentialStore, error) {
	if appName == "" {
		return nil, errors.New("roon: credential store: missing appName")
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("roon: credential store: user config dir: %w", err)
	}

	cfgDir := filepath.Join(dir, appName)
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		return nil, fmt.Errorf("roon: credential store: mkdir: %w", err)
	}

	return &fileCredentialStore{path: filepath.Join(cfgDir, "credentials.json")}, nil
}

func (s *fileCredentialStore) Load(ctx context.Context, core Core) (Credentials, bool, error) {
	if err := ctx.Err(); err != nil {
		return Credentials{}, false, err
	}
	credentialFileMu.Lock()
	defer credentialFileMu.Unlock()

	key := coreKey(core)
	if key == "" {
		return Credentials{}, false, errors.New("roon: credential store: core has no ID or name")
	}

	m, err := s.readAll()
	if err != nil {
		return Credentials{}, false, err
	}

	creds, ok := m[key]
	return creds, ok, nil
}

func (s *fileCredentialStore) Save(ctx context.Context, core Core, creds Credentials) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	credentialFileMu.Lock()
	defer credentialFileMu.Unlock()

	key := coreKey(core)
	if key == "" {
		return errors.New("roon: credential store: core has no ID or name")
	}

	m, err := s.readAll()
	if err != nil {
		return err
	}

	if creds.CoreKey == "" {
		creds.CoreKey = key
	}
	if creds.CreatedAt.IsZero() {
		creds.CreatedAt = time.Now().UTC()
	}

	m[key] = creds
	return s.writeAll(m)
}

func coreKey(core Core) string {
	if core.ID != "" {
		return string(core.ID)
	}
	if core.Name != "" {
		return core.Name
	}
	return ""
}

func (s *fileCredentialStore) readAll() (map[string]Credentials, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]Credentials{}, nil
		}
		return nil, fmt.Errorf("roon: credential store: read: %w", err)
	}

	var m map[string]Credentials
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("roon: credential store: decode: %w", err)
	}
	if m == nil {
		m = map[string]Credentials{}
	}
	return m, nil
}

func (s *fileCredentialStore) writeAll(m map[string]Credentials) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("roon: credential store: encode: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".credentials-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}
