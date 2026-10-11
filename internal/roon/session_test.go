package roon

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type memoryCredentials struct {
	mu           sync.Mutex
	value        Credentials
	loads, saves int
}

func (m *memoryCredentials) Load(_ context.Context, core Core) (Credentials, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loads++
	if core.ID != "test-core" {
		return Credentials{}, false, errors.New("credentials loaded before core identity was resolved")
	}
	return m.value, m.value.RegistryToken != "", nil
}
func (m *memoryCredentials) Save(_ context.Context, _ Core, value Credentials) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saves++
	m.value = value
	return nil
}

func sessionServer(t *testing.T, ctx context.Context, approvalDelay time.Duration, revoked <-chan struct{}, connections, registrations *atomic.Int32, onRegister ...func(registryRegisterRequest)) Core {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer ws.CloseNow()
		connections.Add(1)
		registrationID := ""
		for {
			_, data, err := ws.Read(ctx)
			if err != nil {
				return
			}
			frame, err := parseMooFrame(data)
			if err != nil {
				t.Error(err)
				return
			}
			var response []byte
			switch frame.Service + "/" + frame.Name {
			case svcRegistry + "/info":
				response, err = encodeMooComplete(frame.RequestID, "Success", registryInfoResponse{CoreID: "test-core"})
			case svcRegistry + "/register":
				var request registryRegisterRequest
				if err := json.Unmarshal(frame.BodyRaw, &request); err != nil {
					t.Error(err)
					return
				}
				for _, observe := range onRegister {
					observe(request)
				}
				registrations.Add(1)
				registrationID = frame.RequestID
				timer := time.NewTimer(approvalDelay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				// Enable returns Registered, without a separate pairing request.
				response, err = encodeMooContinue(frame.RequestID, "Registered", registryRegistered{CoreID: "test-core", DisplayName: "Test", Token: "token"})
			case svcTransport + "/subscribe_zones":
				response, err = encodeMooContinue(frame.RequestID, "Subscribed", transportSubscribeZonesResponse{Zones: []zoneRaw{{ZoneID: "zone", DisplayName: "Listening", State: "playing"}}})
			default:
				t.Errorf("unexpected request: %s/%s", frame.Service, frame.Name)
				return
			}
			if err != nil {
				t.Error(err)
				return
			}
			if err := ws.Write(ctx, websocket.MessageBinary, response); err != nil {
				return
			}
			if frame.Name == "subscribe_zones" {
				closed := ws.CloseRead(ctx)
				select {
				case <-closed.Done():
					return
				case <-revoked:
					response, _ := encodeMooComplete(registrationID, "Unregistered", nil)
					_ = ws.Write(ctx, websocket.MessageBinary, response)
					<-closed.Done()
					return
				}
			}
		}
	}))
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(u.Host)
	p, _ := strconv.Atoi(port)
	return Core{Host: host, Port: p, Name: "Test"}
}

func TestSessionSlowApprovalUsesOneConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var connections, registrations atomic.Int32
	store := &memoryCredentials{}
	core := sessionServer(t, ctx, 11*time.Second, nil, &connections, &registrations)
	client := NewClient(Config{}, WithCredentialStore(store), WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
	prompted := false
	session, err := client.OpenSession(ctx, core, func() { prompted = true })
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if !prompted || connections.Load() != 1 || registrations.Load() != 1 || len(session.Zones) != 1 {
		t.Fatal("approval or snapshot did not use one session")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.loads != 1 || store.saves != 1 || store.value.PairedCoreID != "test-core" {
		t.Fatal("credentials were not saved once after automatic pairing")
	}
}

func TestSessionRevokedRegistrationClosesConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var connections, registrations atomic.Int32
	revoked := make(chan struct{})
	core := sessionServer(t, ctx, 0, revoked, &connections, &registrations)
	session, err := NewClient(Config{}, WithCredentialStore(&memoryCredentials{})).OpenSession(ctx, core, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	close(revoked)
	select {
	case <-session.Done:
	case <-ctx.Done():
		t.Fatal("revocation left an authorized session alive")
	}
}

func TestSessionAuthorizationCanBeCancelled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var connections, registrations atomic.Int32
	core := sessionServer(t, ctx, 30*time.Second, nil, &connections, &registrations)
	_, err := NewClient(Config{}, WithCredentialStore(&memoryCredentials{})).OpenSession(ctx, core, cancel)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestSessionMigratesOnlyMatchingExplicitAddressCredentials(t *testing.T) {
	for _, paired := range []CoreID{"test-core", "other-core"} {
		t.Run(string(paired), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var connections, registrations atomic.Int32
			var presentedToken atomic.Value
			core := sessionServer(t, ctx, 0, nil, &connections, &registrations, func(request registryRegisterRequest) { presentedToken.Store(request.Token) })
			core.Name = net.JoinHostPort(core.Host, strconv.Itoa(core.Port))
			store := &fileCredentialStore{path: filepath.Join(t.TempDir(), "credentials.json")}
			if err := store.Save(ctx, core, Credentials{RegistryToken: "legacy-token", PairedCoreID: paired}); err != nil {
				t.Fatal(err)
			}
			session, err := NewClient(Config{}, WithCredentialStore(store)).OpenSession(ctx, core, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			expected := ""
			if paired == "test-core" {
				expected = "legacy-token"
			}
			if presentedToken.Load() != expected {
				t.Fatalf("presented token %q, expected %q", presentedToken.Load(), expected)
			}
			value, found, err := store.Load(ctx, session.Core)
			if err != nil || !found || value.CoreKey != "test-core" || value.PairedCoreID != "test-core" {
				t.Fatalf("canonical credentials not saved: %v", err)
			}
		})
	}
}
