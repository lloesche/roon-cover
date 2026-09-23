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
	for {
		started := time.Now()
		err := c.Source.SubscribeZones(ctx, c.Core, func(u roon.ZoneUpdate) error {
			publishSubscription(updates, subscriptionEvent{zones: u.Zones})
			return nil
		})
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
