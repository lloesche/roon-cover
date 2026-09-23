package roon

import (
	"errors"
	"sync"
)

func (s *session) registerPingService() {
	s.conn.RegisterHandler(svcPing, func(req *mooRequest) error {
		switch req.frame.Name {
		case "ping":
			return req.SendComplete("Success", nil)
		default:
			return req.SendComplete("InvalidRequest", map[string]string{"error": "unknown request name: " + req.frame.Name})
		}
	})
}

type pairingState struct {
	mu sync.Mutex

	subs map[string]string // subscription_key as string
}

func (s *session) registerPairingService() {
	ps := &pairingState{subs: map[string]string{}}

	s.conn.RegisterHandler(svcPairing, func(req *mooRequest) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		switch req.frame.Name {
		case "subscribe_pairing":
			var body struct {
				SubscriptionKey any `json:"subscription_key"`
			}
			if err := req.JSON(&body); err != nil {
				return err
			}
			key := parseSubscriptionKey(body.SubscriptionKey)

			ps.mu.Lock()
			ps.subs[key] = req.frame.RequestID
			ps.mu.Unlock()

			return req.SendContinue("Subscribed", map[string]string{"paired_core_id": string(s.creds.PairedCoreID)})

		case "unsubscribe_pairing":
			var body struct {
				SubscriptionKey any `json:"subscription_key"`
			}
			if err := req.JSON(&body); err != nil {
				return err
			}
			key := parseSubscriptionKey(body.SubscriptionKey)

			ps.mu.Lock()
			delete(ps.subs, key)
			ps.mu.Unlock()

			return req.SendComplete("Unsubscribed", nil)

		case "get_pairing":
			return req.SendComplete("Success", map[string]string{"paired_core_id": string(s.creds.PairedCoreID)})

		case "pair":
			// User accepted this extension in Roon.
			if s.core.ID == "" {
				return errors.New("pairing: missing core id (registration incomplete?)")
			}

			s.log.Info("paired by roon core", "core_id", s.core.ID)
			s.creds.PairedCoreID = s.core.ID

			// Notify subscribers (best-effort).
			ps.mu.Lock()
			for _, requestID := range ps.subs {
				_ = s.conn.sendContinue(requestID, "Changed", map[string]string{"paired_core_id": string(s.creds.PairedCoreID)})
			}
			ps.mu.Unlock()

			// Notify the waiter (Pair()) without ever blocking. Closing a channel provides
			// synchronization so the caller will observe the updated creds.PairedCoreID.
			if s.pairedSignal != nil {
				s.pairedOnce.Do(func() { close(s.pairedSignal) })
			}

			return req.SendComplete("Success", nil)

		default:
			return req.SendComplete("InvalidRequest", map[string]string{"error": "unknown request name: " + req.frame.Name})
		}
	})
}
