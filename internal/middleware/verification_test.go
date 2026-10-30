package middleware

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestVerificationSharedAndBounded(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	s := testServer(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		select {
		case <-release:
			return reply(200, `{"account_id":"a"}`)
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	})
	done := make(chan error, 2*maxTokenVerifications)
	for index := range maxTokenVerifications {
		ctx := &joinedContext{Context: context.Background(), joined: make(chan struct{})}
		token := fmt.Sprint(index)
		go func() { _, err := s.verifiedToken(ctx, token, sha256.Sum256([]byte(token))); done <- err }()
		awaitLatest(t, ctx.joined)
	}
	if _, err := s.verifiedToken(context.Background(), "overflow", sha256.Sum256([]byte("overflow"))); err == nil {
		t.Fatal("unbounded verifier queue")
	}
	for index := range maxTokenVerifications {
		ctx := &joinedContext{Context: context.Background(), joined: make(chan struct{})}
		token := fmt.Sprint(index)
		go func() { _, err := s.verifiedToken(ctx, token, sha256.Sum256([]byte(token))); done <- err }()
		awaitLatest(t, ctx.joined)
	}
	close(release)
	for range 2 * maxTokenVerifications {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("verification did not finish")
		}
	}
	if calls.Load() != maxTokenVerifications {
		t.Fatal(calls.Load())
	}
}

func TestVerificationLastCancellationAbortsUpstream(t *testing.T) {
	stopped := make(chan struct{})
	s := testServer(t, func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		close(stopped)
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	tracked := &joinedContext{Context: ctx, joined: make(chan struct{})}
	done := make(chan struct{})
	go func() { _, _ = s.verifiedToken(tracked, "token", sha256.Sum256([]byte("token"))); close(done) }()
	awaitLatest(t, tracked.joined)
	cancel()
	awaitLatest(t, done)
	awaitLatest(t, stopped)
	if len(s.tokens.entries) != 0 {
		t.Fatal("canceled verification cached")
	}
}
