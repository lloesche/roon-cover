package roon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

type mooConn struct {
	log *slog.Logger
	ws  *websocket.Conn

	nextReqID atomic.Int64

	mu       sync.Mutex
	pending  map[string]*pendingRequest
	handlers map[string]serviceHandler

	closeOnce sync.Once
	closed    chan struct{}
}

type pendingRequest struct {
	mu         sync.Mutex
	finishOnce sync.Once

	onFrame func(*mooFrame) error
	done    chan error // closed on COMPLETE (or connection close)
	// Some Roon Core calls reply with CONTINUE only (no COMPLETE).
	// For those, we treat the first response frame as completion.
	completeOnFirstResponse bool
}

type serviceHandler func(req *mooRequest) error

type mooRequest struct {
	c     *mooConn
	frame *mooFrame
	body  []byte
}

func (r *mooRequest) JSON(v any) error {
	if len(r.body) == 0 {
		return nil
	}
	return jsonUnmarshal(r.body, v)
}

func (r *mooRequest) SendContinue(name string, body any) error {
	return r.c.sendContinue(r.frame.RequestID, name, body)
}

func (r *mooRequest) SendComplete(name string, body any) error {
	return r.c.sendComplete(r.frame.RequestID, name, body)
}

func dialMoo(ctx context.Context, log *slog.Logger, host string, port int) (*mooConn, error) {
	if log == nil {
		log = slog.Default()
	}
	u := url.URL{Scheme: "ws", Host: net.JoinHostPort(host, fmt.Sprint(port)), Path: "/api"}

	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dialCancel()
	ws, _, err := websocket.Dial(dialCtx, u.String(), nil)
	if err != nil {
		return nil, err
	}

	ws.SetReadLimit(maxMooFrameBytes)
	c := &mooConn{
		log:      log,
		ws:       ws,
		pending:  map[string]*pendingRequest{},
		handlers: map[string]serviceHandler{},
		closed:   make(chan struct{}),
	}

	go c.readLoop()
	go c.heartbeatLoop()
	return c, nil
}

func (p *pendingRequest) finish(err error) {
	p.finishOnce.Do(func() { p.done <- err })
}
func (c *mooConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
		if c.ws != nil {
			_ = c.ws.CloseNow()
		}
		c.mu.Lock()
		pending := c.pending
		c.pending = map[string]*pendingRequest{}
		c.mu.Unlock()
		for _, p := range pending {
			p.finish(errors.New("moo: connection closed"))
		}
	})
	return nil
}
func (c *mooConn) RegisterHandler(service string, h serviceHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[service] = h
}

func (c *mooConn) Call(ctx context.Context, fullName string, reqBody any, onComplete func(*mooFrame) error) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	reqID := c.nextReqID.Add(1) - 1
	buf, err := encodeMooRequest(reqID, fullName, reqBody)
	if err != nil {
		return err
	}

	p := &pendingRequest{
		onFrame: func(f *mooFrame) error {
			// For Call(), treat any response frame as eligible to satisfy the call.
			// The caller can inspect f.ResponseName.
			if onComplete != nil {
				return onComplete(f)
			}
			return nil
		},
		done:                    make(chan error, 1),
		completeOnFirstResponse: true,
	}

	idStr := fmt.Sprintf("%d", reqID)
	c.mu.Lock()
	select {
	case <-c.closed:
		c.mu.Unlock()
		return errors.New("moo: connection closed")
	default:
	}
	c.pending[idStr] = p
	c.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.onFrame = nil
		p.mu.Unlock()
		if ctx.Err() != nil {
			c.mu.Lock()
			delete(c.pending, idStr)
			c.mu.Unlock()
		}
	}()
	c.log.Debug("moo -> request", "id", idStr, "name", fullName)
	if err := c.ws.Write(ctx, websocket.MessageBinary, buf); err != nil {
		c.mu.Lock()
		delete(c.pending, idStr)
		c.mu.Unlock()
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-p.done:
		return err
	}
}

func (c *mooConn) Subscribe(ctx context.Context, fullName string, reqBody any, onEvent func(*mooFrame) error) error {
	reqID := c.nextReqID.Add(1) - 1
	buf, err := encodeMooRequest(reqID, fullName, reqBody)
	if err != nil {
		return err
	}

	idStr := fmt.Sprintf("%d", reqID)
	p := &pendingRequest{
		onFrame: func(f *mooFrame) error {
			if onEvent != nil {
				return onEvent(f)
			}
			return nil
		},
		done:                    make(chan error, 1),
		completeOnFirstResponse: false,
	}

	c.mu.Lock()
	select {
	case <-c.closed:
		c.mu.Unlock()
		return errors.New("moo: connection closed")
	default:
	}
	c.pending[idStr] = p
	c.mu.Unlock()

	c.log.Debug("moo -> subscribe request", "id", idStr, "name", fullName)
	writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
	defer writeCancel()
	if err := c.ws.Write(writeCtx, websocket.MessageBinary, buf); err != nil {
		c.mu.Lock()
		delete(c.pending, idStr)
		c.mu.Unlock()
		return err
	}

	// Caller is expected to keep ctx alive while subscribed; when ctx is canceled, the caller
	// should close the session.
	return nil
}

func (c *mooConn) sendContinue(requestID string, name string, body any) error {
	buf, err := encodeMooContinue(requestID, name, body)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.ws.Write(ctx, websocket.MessageBinary, buf)
}

func (c *mooConn) sendComplete(requestID string, name string, body any) error {
	buf, err := encodeMooComplete(requestID, name, body)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.ws.Write(ctx, websocket.MessageBinary, buf)
}

func (c *mooConn) readLoop() {
	defer func() { _ = c.Close() }()

	for {
		_, data, err := c.ws.Read(context.Background())
		if err != nil {
			return
		}

		f, err := parseMooFrame(data)
		if err != nil {
			c.log.Warn("moo parse failed", "err", err)
			return
		}

		switch f.Verb {
		case mooVerbRequest:
			c.handleIncomingRequest(f)
		case mooVerbContinue, mooVerbComplete:
			c.handleResponse(f)
		default:
			c.log.Warn("moo unknown verb", "verb", f.Verb)
		}
	}
}

func (c *mooConn) handleIncomingRequest(f *mooFrame) {
	c.mu.Lock()
	h := c.handlers[f.Service]
	c.mu.Unlock()

	req := &mooRequest{c: c, frame: f, body: f.BodyRaw}
	c.log.Debug("moo <- request", "id", f.RequestID, "service", f.Service, "name", f.Name)
	if h == nil {
		_ = req.SendComplete("InvalidRequest", map[string]string{"error": "unknown service: " + f.Service})
		return
	}
	if err := h(req); err != nil {
		_ = req.SendComplete("Error", map[string]string{"error": err.Error()})
		return
	}
}

func (c *mooConn) handleResponse(f *mooFrame) {
	c.mu.Lock()
	p := c.pending[f.RequestID]
	if f.Verb == mooVerbComplete {
		delete(c.pending, f.RequestID)
	}
	c.mu.Unlock()
	// Late replies to cancelled calls are normal, and must not kill other requests.
	if p == nil {
		return
	}
	p.mu.Lock()
	var err error
	if p.onFrame != nil {
		err = p.onFrame(f)
	}
	if p.completeOnFirstResponse {
		p.onFrame = nil
	}
	if err != nil || f.Verb == mooVerbComplete || p.completeOnFirstResponse {
		p.finish(err)
	}
	p.mu.Unlock()
	if err != nil && !p.completeOnFirstResponse {
		_ = c.Close()
	}
}
func (c *mooConn) heartbeatLoop() {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-c.closed:
			return
		case <-t.C:
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := c.ws.Ping(ctx)
			cancel()
			if err != nil {
				_ = c.Close()
				return
			}
		}
	}
}
