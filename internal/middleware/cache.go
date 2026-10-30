package middleware

import (
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type tokenVerdict struct {
	verified  bool
	accountID string
}

type tokenEntry struct {
	verdict tokenVerdict
	expires time.Time
}

type tokenStore struct {
	mu            sync.Mutex
	entries       map[[32]byte]tokenEntry
	maxEntries    int
	lastSweep     time.Time
	sweepInterval time.Duration
}

func newTokenStore() *tokenStore {
	return &tokenStore{entries: make(map[[32]byte]tokenEntry), maxEntries: 10_000, sweepInterval: time.Minute}
}

func (s *tokenStore) get(key [32]byte, now time.Time) (tokenVerdict, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[key]
	if exists && !entry.expires.After(now) {
		delete(s.entries, key)
		return tokenVerdict{}, false
	}
	return entry.verdict, exists
}

func (s *tokenStore) put(key [32]byte, verdict tokenVerdict, ttl time.Duration, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.Sub(s.lastSweep) >= s.sweepInterval {
		for key, entry := range s.entries {
			if !entry.expires.After(now) {
				delete(s.entries, key)
			}
		}
		s.lastSweep = now
	}
	if _, exists := s.entries[key]; !exists && len(s.entries) >= s.maxEntries {
		var victim [32]byte
		var earliest time.Time
		for key, entry := range s.entries {
			if earliest.IsZero() || entry.expires.Before(earliest) {
				victim, earliest = key, entry.expires
			}
		}
		delete(s.entries, victim)
	}
	s.entries[key] = tokenEntry{verdict, now.Add(ttl)}
}

type rateWindow struct {
	start time.Time
	count int
}

type rateStore struct {
	mu        sync.Mutex
	windows   map[string]rateWindow
	lastSweep time.Time
}

func (s *rateStore) register(key string, limit int, now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.windows == nil {
		s.windows = make(map[string]rateWindow)
	}
	if now.Sub(s.lastSweep) > time.Minute {
		for key, window := range s.windows {
			if now.Sub(window.start) >= time.Minute {
				delete(s.windows, key)
			}
		}
		s.lastSweep = now
	}
	entry, exists := s.windows[key]
	if !exists || now.Sub(entry.start) >= time.Minute {
		s.windows[key] = rateWindow{now, 1}
		return 0
	}
	if entry.count >= limit {
		return max(1, int(math.Ceil((time.Minute - now.Sub(entry.start)).Seconds())))
	}
	entry.count++
	s.windows[key] = entry
	return 0
}

func clientIP(r *http.Request, trustedProxyCount int) string {
	if trustedProxyCount > 0 {
		var hops []string
		for hop := range strings.SplitSeq(r.Header.Get("X-Forwarded-For"), ",") {
			if hop = strings.TrimSpace(hop); hop != "" {
				hops = append(hops, hop)
			}
		}
		if len(hops) > 0 {
			return hops[max(0, len(hops)-trustedProxyCount)]
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}
