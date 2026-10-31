package middleware

import (
	"container/heap"
	"math"
	"net"
	"net/http"
	"net/netip"
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

type rateExpiry struct {
	key string
	at  time.Time
}

type rateExpiries []rateExpiry

func (h rateExpiries) Len() int           { return len(h) }
func (h rateExpiries) Less(i, j int) bool { return h[i].at.Before(h[j].at) }
func (h rateExpiries) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *rateExpiries) Push(value any)    { *h = append(*h, value.(rateExpiry)) }
func (h *rateExpiries) Pop() any {
	last := len(*h) - 1
	value := (*h)[last]
	(*h)[last] = rateExpiry{}
	*h = (*h)[:last]
	return value
}

type rateStore struct {
	mu         sync.Mutex
	windows    map[string]rateWindow
	expiries   rateExpiries
	maxEntries int
}

func (s *rateStore) register(key string, limit int, now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.windows == nil {
		s.windows = make(map[string]rateWindow)
	}
	if s.maxEntries <= 0 {
		s.maxEntries = 100_000
	}
	for len(s.expiries) > 0 && !s.expiries[0].at.After(now) {
		delete(s.windows, heap.Pop(&s.expiries).(rateExpiry).key)
	}
	entry, exists := s.windows[key]
	if !exists {
		if len(s.windows) >= s.maxEntries {
			return max(1, int(math.Ceil(s.expiries[0].at.Sub(now).Seconds())))
		}
		s.windows[key] = rateWindow{now, 1}
		heap.Push(&s.expiries, rateExpiry{key, now.Add(time.Minute)})
		return 0
	}
	if entry.count >= limit {
		return max(1, int(math.Ceil((time.Minute - now.Sub(entry.start)).Seconds())))
	}
	entry.count++
	s.windows[key] = entry
	return 0
}

func clientIP(r *http.Request, trustedProxyCount int, trustedProxyCIDRs ...string) string {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	if trustedProxyCount > 0 && (len(trustedProxyCIDRs) == 0 || trustedProxyPeer(peer, trustedProxyCIDRs)) {
		var hops []string
		for _, header := range r.Header.Values("X-Forwarded-For") {
			for hop := range strings.SplitSeq(header, ",") {
				if hop = strings.TrimSpace(hop); hop != "" {
					hops = append(hops, hop)
				}
			}
		}
		if len(hops) > 0 {
			if ip, err := netip.ParseAddr(hops[max(0, len(hops)-trustedProxyCount)]); err == nil && ip.Zone() == "" {
				return ip.Unmap().String()
			}
		}
	}
	if peer != "" {
		return peer
	}
	return "unknown"
}

func trustedProxyPeer(peer string, ranges []string) bool {
	ip, err := netip.ParseAddr(peer)
	if err != nil || ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	for _, entry := range ranges {
		entry = strings.TrimSpace(entry)
		if prefix, err := netip.ParsePrefix(entry); err == nil && prefix.Contains(ip) {
			return true
		}
		if address, err := netip.ParseAddr(entry); err == nil && address.Zone() == "" && address.Unmap() == ip {
			return true
		}
	}
	return false
}
