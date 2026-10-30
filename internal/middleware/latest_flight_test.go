package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type joinedContext struct {
	context.Context
	joined chan struct{}
	once   sync.Once
}

func (c *joinedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.joined) })
	return c.Context.Done()
}

func latestRequest(ctx context.Context) (*http.Request, <-chan struct{}) {
	tracked := &joinedContext{Context: ctx, joined: make(chan struct{})}
	return httptest.NewRequest("GET", "/api/v1/latest-version", nil).WithContext(tracked), tracked.joined
}

func awaitLatest(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("latest-version operation did not finish")
	}
}

func successfulFeed(r *http.Request) (*http.Response, error) {
	if r.URL.Hostname() == "api.github.com" {
		return reply(200, `{"tag_name":"4.1","html_url":"https://github.com/getappbox/x/releases/tag/4.1"}`)
	}
	return reply(200, `{"version":"4.0"}`)
}

func TestLatestConcurrentWaitersShareFailedRefresh(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	s := testServer(t, func(r *http.Request) (*http.Response, error) { calls.Add(1); <-release; return reply(503, `{}`) })
	errorsOut := make(chan error, 8)
	for range 8 {
		request, joined := latestRequest(context.Background())
		go func() { errorsOut <- s.latestResponse(httptest.NewRecorder(), request) }()
		awaitLatest(t, joined)
	}
	close(release)
	for range 8 {
		select {
		case err := <-errorsOut:
			var api apiError
			if !errors.As(err, &api) || api.code != "update_check_unavailable" {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("waiter did not receive shared failure")
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("made %d fetches for one concurrent refresh", calls.Load())
	}
}

func TestLatestOwnerCancellationDoesNotCancelOtherWaiters(t *testing.T) {
	release := make(chan struct{})
	s := testServer(t, func(r *http.Request) (*http.Response, error) {
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-release:
			return successfulFeed(r)
		}
	})
	firstCtx, cancel := context.WithCancel(context.Background())
	first, joined := latestRequest(firstCtx)
	firstDone := make(chan error, 1)
	go func() { firstDone <- s.latestResponse(httptest.NewRecorder(), first) }()
	awaitLatest(t, joined)
	second, joined := latestRequest(context.Background())
	secondDone := make(chan error, 1)
	go func() { secondDone <- s.latestResponse(httptest.NewRecorder(), second) }()
	awaitLatest(t, joined)
	cancel()
	select {
	case err := <-firstDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled owner was retained behind the feed")
	}
	close(release)
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("other waiter did not finish")
	}
}

func TestLatestLastCancellationStopsRefreshAndAllowsNewFlight(t *testing.T) {
	var calls atomic.Int32
	stopped := make(chan struct{}, 2)
	s := testServer(t, func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) <= 2 {
			<-r.Context().Done()
			stopped <- struct{}{}
			return nil, r.Context().Err()
		}
		return successfulFeed(r)
	})
	ctx, cancel := context.WithCancel(context.Background())
	request, joined := latestRequest(ctx)
	done := make(chan error, 1)
	go func() { done <- s.latestResponse(httptest.NewRecorder(), request) }()
	awaitLatest(t, joined)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled waiter did not leave")
	}
	awaitLatest(t, stopped)
	awaitLatest(t, stopped)
	request, _ = latestRequest(context.Background())
	if err := s.latestResponse(httptest.NewRecorder(), request); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 4 {
		t.Fatal(calls.Load())
	}
}

func TestLatestAlreadyCanceledRequestDoesNotFetch(t *testing.T) {
	var calls atomic.Int32
	s := testServer(t, func(r *http.Request) (*http.Response, error) { calls.Add(1); return successfulFeed(r) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request, _ := latestRequest(ctx)
	if err := s.latestResponse(httptest.NewRecorder(), request); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal(calls.Load())
	}
}

type blockedLatestWriter struct {
	*httptest.ResponseRecorder
	entered chan struct{}
	release chan struct{}
}

func (w *blockedLatestWriter) Write(body []byte) (int, error) {
	close(w.entered)
	<-w.release
	return w.ResponseRecorder.Write(body)
}

func TestLatestSlowResponseDoesNotLockCachedRequests(t *testing.T) {
	s := testServer(t, nil)
	s.update = &latestVersion{Version: "4.1"}
	s.updateExpires = s.now().Add(time.Hour)
	writer := &blockedLatestWriter{httptest.NewRecorder(), make(chan struct{}), make(chan struct{})}
	firstDone := make(chan struct{})
	go func() {
		request, _ := latestRequest(context.Background())
		_ = s.latestResponse(writer, request)
		close(firstDone)
	}()
	awaitLatest(t, writer.entered)
	secondDone := make(chan struct{})
	go func() {
		request, _ := latestRequest(context.Background())
		_ = s.latestResponse(httptest.NewRecorder(), request)
		close(secondDone)
	}()
	awaitLatest(t, secondDone)
	close(writer.release)
	awaitLatest(t, firstDone)
}
