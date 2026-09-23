package roon

import (
	"context"
	"github.com/coder/websocket"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuditCloseDuringResponse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer ws.CloseNow()
		<-ws.CloseRead(ctx).Done()
	}))
	defer server.Close()
	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	started, release := make(chan struct{}), make(chan struct{})
	c := &mooConn{ws: ws, log: slog.New(slog.NewTextHandler(io.Discard, nil)), closed: make(chan struct{}), pending: map[string]*pendingRequest{}}
	c.pending["1"] = &pendingRequest{done: make(chan error, 1), onFrame: func(*mooFrame) error { close(started); <-release; return nil }}
	finished := make(chan any, 1)
	go func() {
		defer func() { finished <- recover() }()
		c.handleResponse(&mooFrame{RequestID: "1", Verb: mooVerbComplete})
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	_ = c.Close()
	close(release)
	select {
	case p := <-finished:
		if p != nil {
			t.Fatalf("closing during response panics: %v", p)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestContinueCallRemainsRegisteredUntilComplete(t *testing.T) {
	count := 0
	c := &mooConn{log: slog.Default(), closed: make(chan struct{}), pending: map[string]*pendingRequest{}}
	p := &pendingRequest{done: make(chan error, 1), completeOnFirstResponse: true, onFrame: func(*mooFrame) error { count++; return nil }}
	c.pending["1"] = p
	c.handleResponse(&mooFrame{RequestID: "1", Verb: mooVerbContinue})
	if len(c.pending) != 1 {
		t.Fatal("CONTINUE incorrectly removed registration")
	}
	c.handleResponse(&mooFrame{RequestID: "1", Verb: mooVerbComplete})
	if count != 1 || len(c.pending) != 0 {
		t.Fatal("completion duplicated the callback or leaked request")
	}
	c.handleResponse(&mooFrame{RequestID: "cancelled", Verb: mooVerbComplete})
	select {
	case <-c.closed:
		t.Fatal("late reply closed connection")
	default:
	}
}
func TestMooRejectsHugeDeclaredBody(t *testing.T) {
	_, err := parseMooFrame([]byte("MOO/1 COMPLETE Success\nRequest-Id: 1\nContent-Type: application/json\nContent-Length: 2147483647\n\n"))
	if err == nil {
		t.Fatal("accepted huge body length")
	}
}
func FuzzMooFrame(f *testing.F) {
	f.Add([]byte("MOO/1 COMPLETE Success\nRequest-Id: 1\n\n"))
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = parseMooFrame(data) })
}
