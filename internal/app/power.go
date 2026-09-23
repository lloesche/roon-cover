package app

import (
	"context"
	"time"
)

func (c *Controller) runPower(ctx context.Context, desired <-chan bool) {
	current, target, possiblyAsleep := false, false, false
	var retry <-chan time.Time
	var timer *time.Timer
	delay := time.Second
	defer func() {
		if timer != nil {
			timer.Stop()
		}
		if possiblyAsleep && c.Power != nil {
			wakeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := c.Power(wakeCtx, false); err != nil {
				c.Log.Warn("restore display power failed", "error", err)
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case next, ok := <-desired:
			if !ok {
				return
			}
			changed := next != target
			target = next
			if retry != nil && !changed {
				continue
			}
			if changed {
				if timer != nil {
					timer.Stop()
				}
				retry = nil
				delay = time.Second
			}
		case <-retry:
			retry = nil
		}
		if c.Power == nil || c.Options.SleepAfter <= 0 || target == current {
			continue
		}
		if target {
			possiblyAsleep = true
		}
		commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := c.Power(commandCtx, target)
		cancel()
		if err != nil {
			c.Log.Warn("display power command failed; retrying", "error", err)
			timer = time.NewTimer(delay)
			retry = timer.C
			delay = min(30*time.Second, delay*2)
		} else {
			current = target
			possiblyAsleep = target
			delay = time.Second
		}
	}
}
