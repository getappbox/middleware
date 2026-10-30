package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf16"

	"howett.net/plist"
)

func TestRateSweepRetainsLiveClientsAndRemovesUnreadExpiredClients(t *testing.T) {
	now := time.Unix(1_000, 0)
	store := rateStore{}
	if retry := store.register("expired", 1, now); retry != 0 {
		t.Fatal(retry)
	}
	if retry := store.register("live", 1, now.Add(50*time.Second)); retry != 0 {
		t.Fatal(retry)
	}
	if retry := store.register("new", 1, now.Add(61*time.Second)); retry != 0 {
		t.Fatal(retry)
	}
	if _, exists := store.windows["expired"]; exists {
		t.Fatal("unread expired window survived sweep")
	}
	if retry := store.register("live", 1, now.Add(61*time.Second)); retry != 49 {
		t.Fatalf("live client's bucket reset during sweep: %d", retry)
	}
	if retry := store.register("expired", 1, now.Add(61*time.Second)); retry != 0 {
		t.Fatal("expired client did not receive a fresh window")
	}
}

func TestRateRetryRoundingAndExactWindowBoundary(t *testing.T) {
	now := time.Unix(1_000, 0)
	store := rateStore{}
	store.register("client", 1, now)
	if retry := store.register("client", 1, now.Add(59*time.Second+999*time.Millisecond)); retry != 1 {
		t.Fatal(retry)
	}
	if retry := store.register("client", 1, now.Add(time.Minute)); retry != 0 {
		t.Fatal(retry)
	}
}

func TestClientIPFallbacksPreserveIPv6AndMissingSocket(t *testing.T) {
	for _, tc := range []struct {
		socket, forwarded string
		proxies           int
		want              string
	}{
		{"[2001:db8::1]:1234", "", 1, "2001:db8::1"},
		{"2001:db8::1", "", 1, "2001:db8::1"},
		{"", " , , ", 2, "unknown"},
		{"raw-peer", "spoofed", 0, "raw-peer"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.socket
		r.Header.Set("X-Forwarded-For", tc.forwarded)
		if got := clientIP(r, tc.proxies); got != tc.want {
			t.Fatalf("%+v: %q", tc, got)
		}
	}
}

func TestVerificationOwnerCancellationKeepsOtherWaiterAlive(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	s := testServer(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		close(started)
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-release:
			return reply(200, `{"account_id":"dbid:shared"}`)
		}
	})
	digest := sha256.Sum256([]byte("shared"))
	ownerCtx, cancelOwner := context.WithCancel(context.Background())
	defer cancelOwner()
	owner := &joinedContext{Context: ownerCtx, joined: make(chan struct{})}
	ownerDone := make(chan error, 1)
	go func() { _, err := s.verifiedToken(owner, "shared", digest); ownerDone <- err }()
	awaitLatest(t, started)
	awaitLatest(t, owner.joined)
	peer := &joinedContext{Context: context.Background(), joined: make(chan struct{})}
	peerDone := make(chan error, 1)
	go func() {
		verdict, err := s.verifiedToken(peer, "shared", digest)
		if err == nil && (!verdict.verified || verdict.accountID != "dbid:shared") {
			err = errors.New("peer lost verified account")
		}
		peerDone <- err
	}()
	awaitLatest(t, peer.joined)
	cancelOwner()
	select {
	case err := <-ownerDone:
		if err == nil {
			t.Fatal("canceled caller succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled owner did not leave promptly")
	}
	close(release)
	select {
	case err := <-peerDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("peer did not receive verification")
	}
	if calls.Load() != 1 {
		t.Fatalf("same token caused %d verifications", calls.Load())
	}
	if verdict, cached := s.tokens.get(digest, s.now()); !cached || !verdict.verified {
		t.Fatal("shared successful verification not cached")
	}
}

func TestCanceledVerificationNeverContactsProvider(t *testing.T) {
	var calls atomic.Int32
	s := testServer(t, func(r *http.Request) (*http.Response, error) { calls.Add(1); return reply(200, `{}`) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.verifiedToken(ctx, "token", sha256.Sum256([]byte("token")))
	if err == nil || calls.Load() != 0 {
		t.Fatal(err, calls.Load())
	}
}

func TestVerificationPositiveAndNegativeTTLBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		ttl    time.Duration
	}{
		{"positive", 200, 3 * time.Second}, {"negative", 401, time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			var clock atomic.Int64
			clock.Store(time.Unix(1_000, 0).UnixNano())
			s := testServer(t, func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				return reply(tc.status, `{"account_id":"dbid:ttl"}`)
			})
			s.config.DropboxTokenCacheTTL = 3 * time.Second
			s.now = func() time.Time { return time.Unix(0, clock.Load()) }
			digest := sha256.Sum256([]byte("ttl-token"))
			for _, advance := range []time.Duration{0, tc.ttl - time.Nanosecond, tc.ttl} {
				clock.Store(time.Unix(1_000, 0).Add(advance).UnixNano())
				verdict, err := s.verifiedToken(context.Background(), "ttl-token", digest)
				if err != nil || verdict.verified != (tc.status == 200) {
					t.Fatal(verdict, err)
				}
			}
			if calls.Load() != 2 {
				t.Fatalf("expected verification only initially and at exact expiration, got %d", calls.Load())
			}
		})
	}
}

type failingReadBody struct{ closed bool }

func (b *failingReadBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (b *failingReadBody) Close() error             { b.closed = true; return nil }

func TestUpstreamReadFailureClosesBodyAndReturnsNoPartialResponse(t *testing.T) {
	body := &failingReadBody{}
	s := testServer(t, func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: body, ContentLength: -1}, nil
	})
	r, _ := http.NewRequest("GET", "https://www.dropbox.com/s/a/file", nil)
	response, data, err := s.fetch(r, 100)
	if !errors.Is(err, io.ErrUnexpectedEOF) || !body.closed || response != nil || data != nil {
		t.Fatal(response, data, err, body.closed)
	}
}

func TestProviderTransportFailuresUseStableErrorsWithoutRetryingMailOrNotify(t *testing.T) {
	for _, tc := range []struct {
		path string
		body any
		code string
	}{
		{"/api/v1/mail/send", mailBody(), "mail_provider_error"},
		{"/api/v1/notify", notifyRequest{"slack", "https://hooks.slack.com/services/x", "Build"}, "notify_failed"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			calls := 0
			s := testServer(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Hostname() == "api.dropboxapi.com" {
					return reply(200, `{}`)
				}
				calls++
				return nil, errors.New("transport diagnostic with private-token")
			})
			response := perform(s, "POST", tc.path, "full", tc.body)
			assertStatus(t, response, 502, tc.code)
			if strings.Contains(response.Body.String(), "private-token") {
				t.Fatal("upstream diagnostic leaked to caller")
			}
			if calls != 1 {
				t.Fatal("non-idempotent delivery retried", calls)
			}
		})
	}
}

func TestMalformedProviderConfigurationFailsBeforeOutboundDelivery(t *testing.T) {
	for _, tc := range []struct {
		name, path, code string
		body             any
		configure        func(*Config)
	}{
		{"shortener", "/api/v1/shorten", "shortener_unavailable", shortenBody(), func(c *Config) { c.YOURLSAPIURL = "https://short.test/%GG" }},
		{"mail", "/api/v1/mail/send", "mail_provider_error", mailBody(), func(c *Config) { c.MailgunDomain = "%GG" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			s := testServer(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Hostname() == "api.dropboxapi.com" {
					return reply(200, `{}`)
				}
				calls++
				return reply(200, `{}`)
			})
			tc.configure(&s.config)
			assertStatus(t, perform(s, "POST", tc.path, "full", tc.body), 502, tc.code)
			if calls != 0 {
				t.Fatal("malformed URL contacted delivery provider")
			}
		})
	}
}

func TestUnreadableReleaseFeedAndMalformedFeedConfigurationFailCleanly(t *testing.T) {
	for _, endpoint := range []string{"https://api.github.com/releases/latest", "https://api.github.com/%GG"} {
		s := testServer(t, func(r *http.Request) (*http.Response, error) {
			if r.URL.Hostname() == "api.github.com" {
				return nil, errors.New("offline")
			}
			return reply(200, `{"version":"4"}`)
		})
		s.config.GitHubLatestReleaseURL = endpoint
		assertStatus(t, perform(s, "GET", "/api/v1/latest-version", "client", nil), 502, "update_check_unavailable")
	}
}

func TestUTF16BigEndianManifestWithAndWithoutBOM(t *testing.T) {
	xml, _ := plist.Marshal(manifestValue(), plist.XMLFormat)
	text := strings.Replace(string(xml), `encoding="UTF-8"`, `encoding="UTF-16BE"`, 1)
	units := utf16.Encode([]rune(text))
	encoded := make([]byte, 2*len(units))
	for i, unit := range units {
		binary.BigEndian.PutUint16(encoded[2*i:], unit)
	}
	for _, input := range [][]byte{encoded, append([]byte{0xfe, 0xff}, encoded...)} {
		rewritten, ok := updatedManifest(input)
		if !ok {
			t.Fatal("valid big-endian manifest was not rewritten")
		}
		var manifest ipaManifest
		if _, err := plist.Unmarshal(rewritten, &manifest); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(*manifest.Items[0].Assets[0].URL, "https://www.dropbox.com/") {
			t.Fatal(string(rewritten))
		}
	}
}

func TestMalformedManifestShapePassesThroughUnchanged(t *testing.T) {
	for _, input := range []string{
		`<plist><dict><key>items</key><array><dict><key>assets</key><array><dict><key>url</key><string>https://cdn.test/a.ipa</string></dict></array></dict></array></dict></plist>`,
		`<plist><dict><key>items</key><array><dict><key>assets</key><array><dict><key>kind</key><string>software-package</string></dict></array></dict></array></dict></plist>`,
		`<plist><dict><key>items</key><array><dict><key>assets</key><array/></dict><dict/></array></dict></plist>`,
		`<plist><dict><key>items</key><array><dict><key>assets</key><array><dict><key>kind</key><string>software-package</string><key>url</key><string>https://cdn.test/app.ipa?%GG=x</string></dict></array></dict></array></dict></plist>`,
	} {
		s := testServer(t, func(r *http.Request) (*http.Response, error) {
			response, _ := reply(200, input)
			response.Header.Set("Content-Type", "application/octet-stream")
			return response, nil
		})
		response := perform(s, "GET", "/install/s/a/manifest.plist", "", nil)
		assertStatus(t, response, 200, "")
		if !bytes.Equal(response.Body.Bytes(), []byte(input)) || response.Header().Get("Content-Type") != "application/octet-stream" {
			t.Fatal("malformed manifest was partially rewritten")
		}
	}
}
