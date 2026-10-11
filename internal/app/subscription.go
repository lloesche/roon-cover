package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"roon-cover/internal/display"
	"roon-cover/internal/roon"
	"strings"
	"time"
)

type subscriptionEvent struct {
	core   roon.Core
	status *display.Status
	zones  []roon.Zone
	err    error
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
	core := c.Core
	failures := 0
	for {
		started := time.Now()
		var err error
		if session == nil {
			if failures >= 2 {
				// An explicit host:port is the user's chosen endpoint; never replace it.
				if !isHostPort(c.Options.CoreSelector) {
					found, discoveryErr := rediscoverCore(ctx, c.Source, core, c.Options.CoreSelector)
					if discoveryErr == nil {
						core = found
					} else {
						c.Log.Warn("Roon rediscovery failed; retrying", "error", discoveryErr)
					}
				}
			}
			session, err = c.Source.OpenSession(ctx, core, func() {
				publishSubscription(updates, subscriptionEvent{err: errors.New("roon: authorization required"), status: &display.Status{Title: "Connect to " + core.Name, Detail: "In Roon, open Settings → Extensions", Hint: "Enable roon-cover to continue."}})
			})
			if err == nil {
				core = session.Core
				failures = 0
				publishSubscription(updates, subscriptionEvent{core: core, zones: session.Zones})
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
		failures++
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
func isHostPort(selector string) bool {
	_, _, err := net.SplitHostPort(strings.TrimSpace(selector))
	return err == nil
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
			publishSubscription(updates, subscriptionEvent{core: session.Core, zones: update.Zones})
		}
	}
}

// Prefer the stable identity, then a unique same-name replacement after a Core
// reinstall. Never silently select a different server from a discovery list.
func rediscoverCore(ctx context.Context, source Source, previous roon.Core, selector string) (roon.Core, error) {
	cores, err := source.Discover(ctx)
	if err != nil {
		return roon.Core{}, err
	}
	for _, core := range cores {
		if previous.ID != "" && core.ID == previous.ID {
			return core, nil
		}
	}
	name := strings.TrimSpace(selector)
	if name == "" {
		name = previous.Name
	}
	var matches []roon.Core
	for _, core := range cores {
		if strings.EqualFold(strings.TrimSpace(core.Name), name) {
			matches = append(matches, core)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return roon.Core{}, fmt.Errorf("could not uniquely rediscover Roon Core %q", name)
}
