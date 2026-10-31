package middleware

import (
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestRateStoreCapacityPreservesActiveBudgets(t *testing.T) {
	now := time.Unix(1000, 0)
	store := rateStore{maxEntries: 2}
	if store.register("first", 1, now) != 0 || store.register("second", 2, now.Add(10*time.Second)) != 0 {
		t.Fatal("existing capacity rejected")
	}
	for i := range 50 {
		if got := store.register(fmt.Sprint("other-", i), 1, now.Add(20*time.Second)); got != 40 {
			t.Fatalf("full-store retry %d, want 40", got)
		}
	}
	if got := store.register("second", 2, now.Add(20*time.Second)); got != 0 {
		t.Fatalf("existing client lost its remaining budget: %d", got)
	}
	if got := store.register("first", 1, now.Add(20*time.Second)); got != 40 {
		t.Fatalf("active limit was evicted: %d", got)
	}
	if len(store.windows) != 2 || len(store.expiries) != 2 {
		t.Fatalf("unbounded state: windows=%d expiries=%d", len(store.windows), len(store.expiries))
	}
	if got := store.register("fresh", 1, now.Add(time.Minute)); got != 0 {
		t.Fatalf("exact expiry did not release capacity: %d", got)
	}
	if got := store.register("second", 2, now.Add(time.Minute)); got != 10 {
		t.Fatalf("unexpired budget changed: %d", got)
	}
}

func TestRateStoreOutOfOrderArrivalAndExpiry(t *testing.T) {
	now := time.Unix(1000, 0)
	store := rateStore{maxEntries: 2}
	store.register("later", 1, now.Add(20*time.Second))
	store.register("earlier", 1, now)
	if got := store.register("blocked", 1, now.Add(30*time.Second)); got != 30 {
		t.Fatalf("wrong earliest expiry: %d", got)
	}
	if got := store.register("new", 1, now.Add(time.Minute)); got != 0 {
		t.Fatalf("earliest entry retained: %d", got)
	}
	if _, exists := store.windows["later"]; !exists {
		t.Fatal("live budget was discarded")
	}
	for i := range 50 {
		store.register("new", 1, now.Add(time.Duration(i+2)*time.Minute))
		if len(store.windows) != 1 || len(store.expiries) != 1 {
			t.Fatal("expired state accumulated")
		}
	}
}

func TestRateStoreCapacityUnderConcurrentClients(t *testing.T) {
	store := rateStore{maxEntries: 32}
	now := time.Unix(1000, 0)
	var group sync.WaitGroup
	for i := range 200 {
		group.Go(func() {
			store.register(fmt.Sprint("client-", i), 1, now)
		})
	}
	group.Wait()
	if len(store.windows) != 32 || len(store.expiries) != 32 {
		t.Fatalf("concurrent capacity exceeded: windows=%d expiries=%d", len(store.windows), len(store.expiries))
	}
}

func TestRateStoreDefaultCapacity(t *testing.T) {
	var store rateStore
	now := time.Unix(1000, 0)
	for i := range 100_001 {
		retry := store.register(fmt.Sprint("client-", i), 1, now)
		if i < 100_000 && retry != 0 {
			t.Fatal("default capacity rejected too early")
		}
		if i == 100_000 && retry != 60 {
			t.Fatalf("default capacity did not reject overflow: %d", retry)
		}
	}
	if len(store.windows) != 100_000 || len(store.expiries) != 100_000 {
		t.Fatal("default state exceeds its bound")
	}
}

func TestRateStoreCapacityHTTPResponseAndRecovery(t *testing.T) {
	now := time.Unix(1000, 0)
	s := testServer(t, nil)
	s.now = func() time.Time { return now }
	s.rates.maxEntries = 2
	request := func(ip string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/v1/config", nil)
		r.Header.Set("X-AppBox-Client-Token", "test-client-token")
		r.Header.Set("X-Forwarded-For", ip)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		assertStatus(t, w, want, "")
		return w
	}
	request("198.51.100.1", 200)
	request("198.51.100.2", 200)
	blocked := request("198.51.100.3", 429)
	assertStatus(t, blocked, 429, "rate_limited")
	if blocked.Header().Get("Retry-After") != "60" {
		t.Fatal("capacity response lacks a bounded retry")
	}
	request("198.51.100.1", 200)
	now = now.Add(time.Minute)
	request("198.51.100.3", 200)
}
