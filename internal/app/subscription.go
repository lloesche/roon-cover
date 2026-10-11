package app

import (
	"context"
	"errors"
	"roon-cover/internal/roon"
	"time"
)

type subscriptionEvent struct {
	zones []roon.Zone
	err   error
}

// A single ordered mailbox prevents an old snapshot from reviving a lost session.
func publishSubscription(ch chan subscriptionEvent, event subscriptionEvent) {
	select {
	case ch <- event:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- event:
	default:
	}
}
func (c *Controller) subscribe(ctx context.Context, updates chan subscriptionEvent) {
	delay := time.Second
	session := c.InitialSession
	for {
		started := time.Now()
		var err error
		if session == nil {
			session, err = c.Source.OpenSession(ctx, c.Core, nil)
			if err == nil {
				publishSubscription(updates, subscriptionEvent{zones: session.Zones})
			}
		}
		if err == nil {
			err = consumeSession(ctx, session, updates)
			session.Close()
			session = nil
		}
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = errors.New("roon: subscription ended")
		}
		publishSubscription(updates, subscriptionEvent{err: err})
		if time.Since(started) > 30*time.Second {
			delay = time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay = min(30*time.Second, delay*2)
	}
}
func consumeSession(ctx context.Context, session *roon.Session, updates chan subscriptionEvent) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-session.Done:
			return errors.New("roon: zone subscription disconnected")
		case update := <-session.Updates:
			select {
			case <-session.Done:
				return errors.New("roon: zone subscription disconnected")
			default:
			}
			publishSubscription(updates, subscriptionEvent{zones: update.Zones})
		}
	}
}
