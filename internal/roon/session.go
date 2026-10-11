package roon

import (
	"context"
	"errors"
	"time"
)

// Session owns one registered connection and its initial and subsequent zone snapshots.
// Snapshots are immutable and coalesced when a consumer falls behind.
type Session struct {
	Core       Core
	Zones      []Zone
	Updates    <-chan ZoneUpdate
	Done       <-chan struct{}
	connection *mooConn
}

func (s *Session) Close() error {
	if s.connection == nil {
		return nil
	}
	return s.connection.Close()
}

// OpenSession waits for authorization and the initial subscription snapshot.
// Authorization has no human-response deadline; cancellation and connection loss
// still end the wait. onAuthorization is called if registration remains pending.
func (c *Client) OpenSession(ctx context.Context, core Core, onAuthorization func()) (*Session, error) {
	s, err := c.connectAndRegister(ctx, core, onAuthorization)
	if err != nil {
		return nil, err
	}
	updates := make(chan ZoneUpdate, 1)
	first := make(chan []Zone, 1)
	initialized := false
	err = s.subscribeZones(ctx, func(update ZoneUpdate) error {
		if !initialized {
			initialized = true
			first <- update.Zones
			return nil
		}
		select {
		case updates <- update:
			return nil
		default:
		}
		select {
		case <-updates:
		default:
		}
		select {
		case updates <- update:
		default:
		}
		return nil
	})
	if err != nil {
		s.conn.Close()
		return nil, err
	}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case zones := <-first:
		return &Session{Core: s.coreSnapshot(), Zones: zones, Updates: updates, Done: s.conn.closed, connection: s.conn}, nil
	case <-ctx.Done():
		err = ctx.Err()
	case <-s.conn.closed:
		err = errors.New("roon: disconnected before initial zone snapshot")
	case <-timer.C:
		err = errors.New("roon: initial zone snapshot timed out")
	}
	s.conn.Close()
	return nil, err
}
