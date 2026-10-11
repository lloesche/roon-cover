package app

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"log/slog"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"roon-cover/internal/display"
	"roon-cover/internal/roon"
)

type retryArtworkSource struct {
	source
	attempts atomic.Int32
}

func (s *retryArtworkSource) FetchImage(ctx context.Context, core roon.Core, key roon.ImageKey, options roon.ImageFetchOptions) ([]byte, string, error) {
	if s.attempts.Add(1) == 1 {
		return nil, "", errors.New("temporary artwork failure")
	}
	return s.source.FetchImage(ctx, core, key, options)
}

func TestArtworkRetriesAtDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		s := &retryArtworkSource{}
		c := Controller{Source: s}
		_ = c.Initialize([]roon.Zone{{ID: "zone", State: roon.ZoneStatePlaying, NowPlaying: &roon.NowPlaying{ImageKey: "cover"}}})
		info := make(chan display.ScreenInfo, 1)
		info <- display.ScreenInfo{RenderWidth: 800, RenderHeight: 800}
		done := make(chan error, 1)
		go func() { done <- c.Run(ctx, make(chan display.Update, 1), info, nil) }()
		synctest.Wait()
		time.Sleep(150 * time.Millisecond)
		synctest.Wait()
		if s.attempts.Load() != 1 {
			t.Fatal("resize debounce did not start the first artwork request")
		}
		time.Sleep(time.Second - time.Millisecond)
		synctest.Wait()
		if s.attempts.Load() != 1 {
			t.Fatal("artwork retried before its deadline")
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if s.attempts.Load() != 2 {
			t.Fatal("artwork did not retry at its deadline")
		}
		cancel()
		<-done
	})
}

type movingArtworkSource struct {
	reconnectSource
	started, cancelled chan struct{}
	endpoints          chan roon.Core
}

func (s *movingArtworkSource) OpenSession(ctx context.Context, core roon.Core, prompt func()) (*roon.Session, error) {
	session, err := s.reconnectSource.OpenSession(ctx, core, prompt)
	if session != nil {
		session.Zones = []roon.Zone{{ID: "zone", Name: "Listening", State: roon.ZoneStatePlaying, NowPlaying: &roon.NowPlaying{ImageKey: "cover", Title: "Track"}}}
	}
	return session, err
}
func (s *movingArtworkSource) FetchImage(ctx context.Context, core roon.Core, _ roon.ImageKey, _ roon.ImageFetchOptions) ([]byte, string, error) {
	s.endpoints <- core
	if core.Host == "old" {
		close(s.started)
		<-ctx.Done()
		close(s.cancelled)
		return nil, "", ctx.Err()
	}
	var data bytes.Buffer
	err := png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	return data.Bytes(), "image/png", err
}
func TestReconnectMovesArtworkAndCancelsOldEndpoint(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		old := roon.Core{ID: "core", Name: "Server", Host: "old"}
		current := roon.Core{ID: "core", Name: "Server", Host: "new"}
		s := &movingArtworkSource{reconnectSource: reconnectSource{old: old, cores: []roon.Core{current}, attempts: make(chan roon.Core, 8)}, started: make(chan struct{}), cancelled: make(chan struct{}), endpoints: make(chan roon.Core, 4)}
		lost := make(chan struct{})
		zones := []roon.Zone{{ID: "zone", Name: "Listening", State: roon.ZoneStatePlaying, NowPlaying: &roon.NowPlaying{ImageKey: "cover"}}}
		c := Controller{Source: s, Core: old, InitialSession: &roon.Session{Core: old, Zones: zones, Done: lost}}
		_ = c.Initialize(zones)
		info := make(chan display.ScreenInfo, 1)
		info <- display.ScreenInfo{RenderWidth: 800, RenderHeight: 800}
		scenes := make(chan display.Update, 1)
		done := make(chan error, 1)
		go func() { done <- c.Run(ctx, scenes, info, nil) }()
		synctest.Wait()
		time.Sleep(150 * time.Millisecond)
		synctest.Wait()
		select {
		case <-s.started:
		default:
			t.Fatal("old artwork fetch did not start")
		}
		close(lost)
		synctest.Wait()
		time.Sleep(3 * time.Second)
		synctest.Wait()
		select {
		case <-s.cancelled:
		default:
			t.Fatal("old endpoint fetch was not cancelled")
		}
		if (<-s.endpoints).Host != "old" || (<-s.endpoints).Host != "new" {
			t.Fatal("artwork did not follow the reconnected endpoint")
		}
		if scene := <-scenes; scene.Artwork == nil || scene.NowPlaying.Title != "Track" {
			t.Fatal("reconnected artwork and metadata were not published together")
		}
		cancel()
		<-done
	})
}

type reconnectSource struct {
	old, current roon.Core
	cores        []roon.Core
	attempts     chan roon.Core
	discoveries  atomic.Int32
}

func (s *reconnectSource) OpenSession(ctx context.Context, core roon.Core, _ func()) (*roon.Session, error) {
	s.attempts <- core
	if core.Host == s.old.Host {
		return nil, errors.New("old address unavailable")
	}
	return &roon.Session{Core: core, Zones: []roon.Zone{{ID: "zone", Name: "Listening"}}, Done: ctx.Done()}, nil
}
func (s *reconnectSource) Discover(context.Context) ([]roon.Core, error) {
	s.discoveries.Add(1)
	return s.cores, nil
}
func (s *reconnectSource) FetchImage(context.Context, roon.Core, roon.ImageKey, roon.ImageFetchOptions) ([]byte, string, error) {
	return nil, "", errors.New("unused")
}

func TestRediscoveryPreservesIdentityAndRequiresUniqueReplacement(t *testing.T) {
	old := roon.Core{ID: "core", Name: "Server", Host: "old"}
	updated := roon.Core{ID: "core", Name: "Renamed", Host: "new"}
	s := &reconnectSource{cores: []roon.Core{{ID: "other", Name: "Server"}, updated}}
	found, err := rediscoverCore(context.Background(), s, old, "")
	if err != nil || found != updated {
		t.Fatal("did not prefer the stable core identity")
	}
	s.cores = []roon.Core{{ID: "replacement", Name: "Server", Host: "new"}}
	if found, err := rediscoverCore(context.Background(), s, old, ""); err != nil || found.ID != "replacement" {
		t.Fatal("unique reinstalled Core was not found")
	}
	s.cores = append(s.cores, roon.Core{ID: "unrelated", Name: "Server"})
	if _, err := rediscoverCore(context.Background(), s, old, ""); err == nil {
		t.Fatal("ambiguous discovery silently selected a Core")
	}
}

func TestSubscriptionRediscoversAfterRepeatedFailures(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		old := roon.Core{ID: "core", Name: "Server", Host: "old"}
		current := roon.Core{ID: "core", Name: "Server", Host: "new"}
		s := &reconnectSource{old: old, current: current, cores: []roon.Core{current}, attempts: make(chan roon.Core, 8)}
		closed := make(chan struct{})
		close(closed)
		c := Controller{Source: s, Core: old, InitialSession: &roon.Session{Core: old, Done: closed}, Log: slog.Default()}
		updates := make(chan subscriptionEvent, 1)
		done := make(chan struct{})
		go func() { defer close(done); c.subscribe(ctx, updates) }()
		synctest.Wait()
		time.Sleep(3 * time.Second)
		synctest.Wait()
		if s.discoveries.Load() != 1 {
			t.Fatal("missing rediscovery after two failures")
		}
		first, second := <-s.attempts, <-s.attempts
		if first.Host != "old" || second.Host != "new" {
			t.Fatal("reconnect did not use the discovered address")
		}
		if event := <-updates; event.err != nil || event.core.Host != "new" {
			t.Fatal("new endpoint was not delivered to the controller")
		}
		cancel()
		<-done
	})
}

func TestExplicitCoreDoesNotRediscover(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		old := roon.Core{Host: "old", Port: 9330}
		s := &reconnectSource{old: old, attempts: make(chan roon.Core, 8)}
		c := Controller{Source: s, Core: old, Log: slog.Default(), Options: Options{CoreSelector: "old:9330"}}
		done := make(chan struct{})
		go func() { defer close(done); c.subscribe(ctx, make(chan subscriptionEvent, 1)) }()
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if s.discoveries.Load() != 0 || len(s.attempts) < 3 {
			t.Fatal("explicit address was not retained")
		}
		cancel()
		<-done
	})
}

func TestIdlePowerDeadlineWithoutPolling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		power := make(chan bool, 2)
		c := Controller{Source: &source{}, Options: Options{SleepAfter: time.Minute}, Power: func(_ context.Context, sleep bool) error { power <- sleep; return nil }}
		if err := c.Initialize([]roon.Zone{{ID: "zone", State: roon.ZoneStatePaused}}); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- c.Run(ctx, make(chan display.Update, 1), nil, nil) }()
		synctest.Wait()
		time.Sleep(time.Minute - time.Millisecond)
		synctest.Wait()
		if len(power) != 0 {
			t.Fatal("display slept before the deadline")
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if len(power) != 1 || !<-power {
			t.Fatal("display did not sleep at its exact deadline")
		}
		cancel()
		<-done
		if <-power {
			t.Fatal("shutdown did not restore display power")
		}
	})
}
