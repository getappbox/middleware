package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type panicBody struct{ closeOnly bool }

func (b panicBody) Read([]byte) (int, error) {
	if !b.closeOnly {
		panic("body read detail")
	}
	return 0, io.EOF
}
func (b panicBody) Close() error { panic("body close detail") }

func TestFetchContainsProviderPanics(t *testing.T) {
	for _, scenario := range []string{"client", "nil response", "read", "close"} {
		t.Run(scenario, func(t *testing.T) {
			fail := true
			s := testServer(t, func(r *http.Request) (*http.Response, error) {
				if !fail {
					return reply(200, `{}`)
				}
				switch scenario {
				case "client":
					panic("provider detail")
				case "nil response":
					return nil, nil
				default:
					return &http.Response{StatusCode: 200, ContentLength: -1, Body: panicBody{closeOnly: scenario == "close"}}, nil
				}
			})
			r := httptest.NewRequest("GET", "https://provider.example.test/", nil)
			response, body, err := s.fetch(r, 1024)
			if err == nil || response != nil || body != nil {
				t.Fatal("provider panic reported as success")
			}
			fail = false
			if _, _, err := s.fetch(r, 1024); err != nil {
				t.Fatal("later upstream request did not recover", err)
			}
		})
	}
}

func TestRequestPanicReturnsSafeErrorAndKeepsServing(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		t.Run(method, func(t *testing.T) {
			s := testServer(t, nil)
			var fail atomic.Bool
			fail.Store(true)
			s.now = func() time.Time {
				if fail.Swap(false) {
					panic("request-private-detail")
				}
				return time.Now()
			}
			server := httptest.NewServer(s)
			defer server.Close()
			client := &http.Client{Timeout: 2 * time.Second}
			r, _ := http.NewRequest(method, server.URL+"/api/v1/config", nil)
			r.Header.Set("X-AppBox-Client-Token", "test-client-token")
			response, err := client.Do(r)
			if err != nil {
				t.Fatal("request panic disconnected client", err)
			}
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != 500 || bytes.Contains(body, []byte("private")) {
				t.Fatalf("unexpected recovery response %d %s", response.StatusCode, body)
			}
			if method == "HEAD" && len(body) != 0 {
				t.Fatal("HEAD recovery emitted a body")
			}
			assertStatus(t, perform(s, "GET", "/api/v1/config", "client", nil), 200, "")
			assertStatus(t, perform(s, "GET", "/", "", nil), 200, "")
		})
	}
}

func TestVerificationPanicReleasesWaitersAndAllowsRetry(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	s := testServer(t, func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
			panic("verification detail")
		}
		return reply(200, `{"account_id":"dbid:recovered"}`)
	})
	digest := sha256.Sum256([]byte("recovery-token"))
	done := make(chan error, 4)
	for range 4 {
		tracked := &joinedContext{Context: context.Background(), joined: make(chan struct{})}
		go func() { _, err := s.verifiedToken(tracked, "recovery-token", digest); done <- err }()
		awaitLatest(t, tracked.joined)
	}
	awaitLatest(t, started)
	close(release)
	for range 4 {
		select {
		case err := <-done:
			var api apiError
			if !errors.As(err, &api) || api.code != "verification_unavailable" {
				t.Fatal("verification panic did not fail closed", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("verification panic stranded a waiter")
		}
	}
	if verdict, cached := s.tokens.get(digest, s.now()); cached || verdict.verified {
		t.Fatal("failed verification cached authorization")
	}
	verdict, err := s.verifiedToken(context.Background(), "recovery-token", digest)
	if err != nil || !verdict.verified || calls.Load() != 2 {
		t.Fatal("same-token retry did not recover", err)
	}
}

func TestVerificationPanicOutsideFetchDoesNotPoisonLocks(t *testing.T) {
	for _, failureCall := range []int32{1, 2, 3} {
		t.Run(string(rune('0'+failureCall)), func(t *testing.T) {
			s := testServer(t, nil)
			var calls atomic.Int32
			s.now = func() time.Time {
				if calls.Add(1) == failureCall {
					panic("cache clock detail")
				}
				return time.Now()
			}
			server := httptest.NewServer(s)
			defer server.Close()
			client := &http.Client{Timeout: 2 * time.Second}
			for attempt := range 2 {
				r, _ := http.NewRequest("POST", server.URL+"/api/v1/shorten", strings.NewReader(`{"url":"https://www.dropbox.com/s/test/appinfo.json","name":"Test","version":"1","build":"1","identifier":"test.example"}`))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("X-AppBox-Client-Token", "test-client-token")
				r.Header.Set("Authorization", "Bearer test-dropbox-token")
				response, err := client.Do(r)
				if err != nil {
					t.Fatal("request or shared lock did not recover", err)
				}
				io.Copy(io.Discard, response.Body)
				response.Body.Close()
				if attempt == 0 && response.StatusCode < 500 || attempt == 1 && response.StatusCode != 200 {
					t.Fatal("unexpected recovery status", attempt, response.StatusCode)
				}
			}
		})
	}
}

func TestLatestProviderPanicIsIsolated(t *testing.T) {
	for _, host := range []string{"api.github.com", "formulae.brew.sh"} {
		t.Run(host, func(t *testing.T) {
			var fail atomic.Bool
			fail.Store(true)
			s := testServer(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Hostname() == host && fail.Swap(false) {
					panic("feed detail")
				}
				return successfulFeed(r)
			})
			response := perform(s, "GET", "/api/v1/latest-version", "client", nil)
			want := 502
			if host == "formulae.brew.sh" {
				want = 200
				var result map[string]any
				json.Unmarshal(response.Body.Bytes(), &result)
				if _, exists := result["homebrewVersion"]; exists {
					t.Fatal("failed optional feed was reported as available")
				}
			}
			assertStatus(t, response, want, "")
			assertStatus(t, perform(s, "GET", "/api/v1/latest-version", "client", nil), 200, "")
		})
	}
}

func TestLatestWorkerPanicCompletesAndRetries(t *testing.T) {
	s := testServer(t, successfulFeed)
	var calls atomic.Int32
	s.now = func() time.Time {
		if calls.Add(1) == 2 {
			panic("update clock detail")
		}
		return time.Now()
	}
	assertStatus(t, perform(s, "GET", "/api/v1/latest-version", "client", nil), 502, "update_check_unavailable")
	assertStatus(t, perform(s, "GET", "/api/v1/latest-version", "client", nil), 200, "")
}

func TestLatestCachePanicDoesNotPoisonLock(t *testing.T) {
	s := testServer(t, successfulFeed)
	s.update = &latestVersion{Version: "4.1"}
	s.updateExpires = time.Now().Add(time.Minute)
	var calls atomic.Int32
	s.now = func() time.Time {
		if calls.Add(1) == 2 {
			panic("cached clock detail")
		}
		return time.Now()
	}
	server := httptest.NewServer(s)
	defer server.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	for attempt := range 2 {
		r, _ := http.NewRequest("GET", server.URL+"/api/v1/latest-version", nil)
		r.Header.Set("X-AppBox-Client-Token", "test-client-token")
		response, err := client.Do(r)
		if err != nil {
			t.Fatal("update cache lock was not released", err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		want := 200
		if attempt == 0 {
			want = 500
		}
		if response.StatusCode != want {
			t.Fatal("unexpected cache recovery response", response.StatusCode)
		}
	}
}

type partialPanicWriter struct{ http.ResponseWriter }

func (w partialPanicWriter) Write(body []byte) (int, error) {
	w.ResponseWriter.Write(body[:min(4, len(body))])
	w.ResponseWriter.(http.Flusher).Flush()
	panic("partial response detail")
}

func TestPartialResponsePanicAbortsOnlyFailedRequest(t *testing.T) {
	s := testServer(t, nil)
	var fail atomic.Bool
	fail.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Swap(false) {
			s.ServeHTTP(partialPanicWriter{w}, r)
		} else {
			s.ServeHTTP(w, r)
		}
	}))
	defer server.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(server.URL)
	if err == nil {
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr == nil || strings.Contains(string(body), "Internal Server Error") {
			t.Fatal("partial response was completed or corrupted with an error body")
		}
	}
	response, err = client.Get(server.URL)
	if err != nil {
		t.Fatal("server did not survive the failed response", err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("later health check failed")
	}
}

func TestRecoveredPanicDetailsAreNotLogged(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previous)
	s := testServer(t, func(*http.Request) (*http.Response, error) { panic("sensitive-panic-marker") })
	r := httptest.NewRequest("GET", "https://provider.example.test/?private-query", nil)
	_, _, err := s.fetch(r, 1024)
	if err == nil || strings.Contains(output.String(), "sensitive-panic-marker") || strings.Contains(output.String(), "private-query") || output.Len() == 0 {
		t.Fatal("recovery logs expose panic data or provide no operational signal")
	}
}
