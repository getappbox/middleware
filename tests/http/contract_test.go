package http_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getappbox/middleware/internal/middleware"
)

type upstreamFunc func(*http.Request) (*http.Response, error)

func (f upstreamFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func upstreamReply(status int, body string) (*http.Response, error) {
	return &http.Response{
		StatusCode:    status,
		Header:        http.Header{"Content-Type": {"application/json"}},
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
	}, nil
}

func configuration() middleware.Config {
	return middleware.Config{
		ClientToken: "integration-client", DropboxAppKey: "integration-app",
		DropboxTokenCacheTTL: time.Minute, MailgunAPIKey: "fake-mailgun-key",
		MailgunDomain: "mail.example.test", MailgunFrom: "AppBox <build@example.test>",
		YOURLSAPIURL: "https://shortener.example.test/api", YOURLSSignatureSecret: "fake-signature",
		ShortlinkTargetBase: "https://web.getappbox.com", ProxyMaxBodyBytes: 4 * 1024 * 1024,
		CORSProxyAllowedHosts:  []string{"dropbox.com", "dropboxusercontent.com"},
		GitHubLatestReleaseURL: "https://release.example.test/latest", HomebrewCaskURL: "https://brew.example.test/latest",
		UpdateCacheTTL: time.Hour, NotifyAllowedHosts: []string{"hooks.slack.com"}, TrustedProxyCount: 1,
	}
}

func successUpstream(r *http.Request) (*http.Response, error) {
	host := strings.ToLower(r.URL.Hostname())
	switch host {
	case "api.dropboxapi.com":
		return upstreamReply(200, `{"account_id":"dbid:integration"}`)
	case "shortener.example.test":
		return upstreamReply(200, `{"shorturl":"https://appbox.me/integration"}`)
	case "release.example.test":
		return upstreamReply(200, `{"tag_name":"4.1.0","html_url":"https://github.com/getappbox/releases/tag/4.1.0"}`)
	case "brew.example.test":
		return upstreamReply(200, `{"version":"4.1.0"}`)
	case "api.mailgun.net":
		return upstreamReply(200, `{"id":"integration-mail"}`)
	case "hooks.slack.com":
		return upstreamReply(200, "")
	case "www.dropbox.com":
		return upstreamReply(200, `{"name":"integration"}`)
	default:
		if strings.HasSuffix(host, ".hooks.slack.com") || host == "dropbox.com" || strings.HasSuffix(host, ".dropbox.com") || host == "dropboxusercontent.com" || strings.HasSuffix(host, ".dropboxusercontent.com") {
			return upstreamReply(200, `{}`)
		}
		return nil, fmt.Errorf("unexpected fake upstream %s", r.URL)
	}
}

func send(handler http.Handler, method, path, token, ip string, body []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	r.Header.Set("X-AppBox-Client-Token", "integration-client")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if ip != "" {
		r.Header.Set("X-Forwarded-For", ip)
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func requireStatus(t *testing.T, response *httptest.ResponseRecorder, status int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status %d, expected %d: %s", response.Code, status, response.Body.String())
	}
}

func shortBody() []byte {
	return []byte(`{"url":"https://www.dropbox.com/s/a/appinfo.json","name":"Demo","version":"1","build":"7","identifier":"com.test.demo"}`)
}

func mailBody() []byte {
	return []byte(`{"name":"Demo","version":"1","build":"7","to":["qa@example.test"],"installURL":"https://appbox.me/integration"}`)
}

func notifyBody() []byte {
	return []byte(`{"service":"slack","webhookURL":"https://hooks.slack.com/services/fake","text":"Build ready"}`)
}

func TestHEADMatchesGETAcrossSuccessfulAndFailedRoutes(t *testing.T) {
	for _, path := range []string{"/", "/api/v1/config", "/api/v1/latest-version", "/appinfo/s/a/appinfo.json", "/appinfo/scl/fi/a/appinfo.json", "/cors?url=https://www.dropbox.com/s/a/appinfo.json", "/install/not-a-share/manifest.plist", "/missing"} {
		t.Run(path, func(t *testing.T) {
			handler := middleware.New(configuration(), upstreamFunc(successUpstream))
			get := send(handler, http.MethodGet, path, "", "", nil)
			head := send(handler, http.MethodHead, path, "", "", nil)
			if head.Code != get.Code || head.Body.Len() != 0 {
				t.Fatalf("HEAD status/body %d/%q differs from GET %d", head.Code, head.Body.String(), get.Code)
			}
			for _, name := range []string{"Content-Type", "Content-Length"} {
				if head.Header().Get(name) != get.Header().Get(name) {
					t.Errorf("HEAD %s=%q, GET %q", name, head.Header().Get(name), get.Header().Get(name))
				}
			}
			length, err := strconv.Atoi(head.Header().Get("Content-Length"))
			if err != nil || length != get.Body.Len() {
				t.Fatalf("HEAD Content-Length %q, GET bytes %d", head.Header().Get("Content-Length"), get.Body.Len())
			}
		})
	}
}

func TestEveryRouteKeepsItsOwnBudget(t *testing.T) {
	for _, route := range []struct {
		method, path string
		limit        int
		body         []byte
	}{
		{"GET", "/api/v1/config", 120, nil}, {"GET", "/api/v1/latest-version", 120, nil},
		{"POST", "/api/v1/shorten", 60, shortBody()}, {"POST", "/api/v1/mail/send", 20, mailBody()},
		{"POST", "/api/v1/notify", 60, notifyBody()},
	} {
		t.Run(route.path, func(t *testing.T) {
			handler := middleware.New(configuration(), upstreamFunc(successUpstream))
			for range route.limit {
				requireStatus(t, send(handler, route.method, route.path, "fake-token", "198.51.100.1", route.body), 200)
			}
			blocked := send(handler, route.method, route.path, "fake-token", "198.51.100.1", route.body)
			requireStatus(t, blocked, 429)
			seconds, err := strconv.Atoi(blocked.Header().Get("Retry-After"))
			if err != nil || seconds < 1 || seconds > 60 {
				t.Fatal(blocked.Header())
			}
			requireStatus(t, send(handler, route.method, route.path, "fake-token", "198.51.100.2", route.body), 200)
			other := "/api/v1/config"
			if route.path == other {
				other = "/api/v1/latest-version"
			}
			requireStatus(t, send(handler, "GET", other, "", "198.51.100.1", nil), 200)
		})
	}
}

func TestPublicProxyRoutesShareOneBudget(t *testing.T) {
	handler := middleware.New(configuration(), upstreamFunc(successUpstream))
	for range 150 {
		requireStatus(t, send(handler, "GET", "/appinfo/s/a/appinfo.json", "", "198.51.100.3", nil), 200)
		requireStatus(t, send(handler, "GET", "/cors?url=https://www.dropbox.com/s/a/appinfo.json", "", "198.51.100.3", nil), 200)
	}
	for _, path := range []string{"/appinfo/scl/fi/a/appinfo.json", "/install/s/a/manifest.plist", "/cors?url=https://www.dropbox.com/s/a/appinfo.json"} {
		requireStatus(t, send(handler, "GET", path, "", "198.51.100.3", nil), 429)
	}
	requireStatus(t, send(handler, "GET", "/api/v1/config", "", "198.51.100.3", nil), 200)
}

func TestAuthenticationRunsBeforeRequestBodyOrUpstream(t *testing.T) {
	var calls atomic.Int32
	handler := middleware.New(configuration(), upstreamFunc(func(r *http.Request) (*http.Response, error) { calls.Add(1); return successUpstream(r) }))
	for _, path := range []string{"/api/v1/shorten", "/api/v1/mail/send", "/api/v1/notify"} {
		request := httptest.NewRequest("POST", path, strings.NewReader(strings.Repeat("x", 20000)))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		requireStatus(t, response, 401)
		request.Header.Set("X-AppBox-Client-Token", "integration-client")
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		requireStatus(t, response, 401)
	}
	if calls.Load() != 0 {
		t.Fatal("unauthenticated request contacted upstream")
	}
}

func TestExactRequestBodyBoundaryAcrossAPIs(t *testing.T) {
	for _, route := range []struct {
		path string
		body []byte
	}{
		{"/api/v1/shorten", shortBody()}, {"/api/v1/mail/send", mailBody()}, {"/api/v1/notify", notifyBody()},
	} {
		t.Run(route.path, func(t *testing.T) {
			prefix := append(append([]byte{}, route.body[:len(route.body)-1]...), []byte(`,"ignoredPadding":"`)...)
			suffix := []byte(`"}`)
			for _, size := range []int{16*1024 - 1, 16 * 1024, 16*1024 + 1} {
				body := append(append(append([]byte{}, prefix...), bytes.Repeat([]byte{'x'}, size-len(prefix)-len(suffix))...), suffix...)
				if len(body) != size {
					t.Fatal("invalid boundary fixture")
				}
				handler := middleware.New(configuration(), upstreamFunc(successUpstream))
				status := 200
				if size > 16*1024 {
					status = 413
				}
				requireStatus(t, send(handler, "POST", route.path, "fake-token", "", body), status)
			}
		})
	}
}

func TestDuplicateProxyURLUsesLastValue(t *testing.T) {
	var urls []string
	handler := middleware.New(configuration(), upstreamFunc(func(r *http.Request) (*http.Response, error) {
		urls = append(urls, r.URL.String())
		return upstreamReply(200, "metadata")
	}))
	allowed := "https://www.dropbox.com/s/a/appinfo.json"
	for _, tc := range []struct {
		first, last string
		status      int
	}{
		{"https://evil.test/private", allowed, 200}, {allowed, "https://evil.test/private", 403},
	} {
		requireStatus(t, send(handler, "GET", "/cors?url="+url.QueryEscape(tc.first)+"&url="+url.QueryEscape(tc.last), "", "", nil), tc.status)
	}
	if len(urls) != 1 || urls[0] != allowed {
		t.Fatal(urls)
	}
}

func TestPublisherCredentialsDoNotReachProxyMailOrNotifyProviders(t *testing.T) {
	var verified atomic.Int32
	handler := middleware.New(configuration(), upstreamFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() == "api.dropboxapi.com" {
			verified.Add(1)
			if r.Header.Get("Authorization") != "Bearer publisher-token" {
				t.Error("missing Dropbox verification token")
			}
		} else {
			if strings.Contains(r.Header.Get("Authorization"), "publisher-token") {
				t.Error("publisher token leaked to provider")
			}
			if r.Header.Get("X-AppBox-Client-Token") != "" || r.Header.Get("Cookie") != "" {
				t.Error("incoming credentials leaked to provider")
			}
		}
		return successUpstream(r)
	}))
	for _, route := range []struct {
		method, path string
		body         []byte
	}{
		{"GET", "/cors?url=https://www.dropbox.com/s/a/appinfo.json", nil},
		{"POST", "/api/v1/mail/send", mailBody()}, {"POST", "/api/v1/notify", notifyBody()},
	} {
		request := httptest.NewRequest(route.method, route.path, bytes.NewReader(route.body))
		request.Header.Set("Authorization", "Bearer publisher-token")
		request.Header.Set("X-AppBox-Client-Token", "integration-client")
		request.Header.Set("Cookie", "private-session=do-not-forward")
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		requireStatus(t, response, 200)
	}
	if verified.Load() != 1 {
		t.Fatal("publisher token was not shared across authenticated routes", verified.Load())
	}
}

func TestProviderNetworkFailuresDoNotBecomeSuccessOrRetryNotifications(t *testing.T) {
	for _, route := range []struct {
		path, code string
		body       []byte
	}{
		{"/api/v1/mail/send", "mail_provider_error", mailBody()}, {"/api/v1/notify", "notify_failed", notifyBody()},
	} {
		t.Run(route.path, func(t *testing.T) {
			calls := 0
			handler := middleware.New(configuration(), upstreamFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Hostname() == "api.dropboxapi.com" {
					return successUpstream(r)
				}
				calls++
				return nil, errors.New("fake network failure")
			}))
			response := send(handler, "POST", route.path, "publisher", "", route.body)
			requireStatus(t, response, 502)
			var got struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil || got.Error.Code != route.code {
				t.Fatal(response.Body.String(), err)
			}
			if calls != 1 {
				t.Fatalf("provider request was retried %d times", calls)
			}
		})
	}
}

func TestConcurrentDifferentAPIsSharePublisherVerification(t *testing.T) {
	var verificationCalls atomic.Int32
	handler := middleware.New(configuration(), upstreamFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() == "api.dropboxapi.com" {
			verificationCalls.Add(1)
		}
		return successUpstream(r)
	}))
	var group sync.WaitGroup
	for range 6 {
		for _, route := range []struct {
			path string
			body []byte
		}{{"/api/v1/shorten", shortBody()}, {"/api/v1/mail/send", mailBody()}, {"/api/v1/notify", notifyBody()}} {
			group.Go(func() { requireStatus(t, send(handler, "POST", route.path, "shared-publisher", "", route.body), 200) })
		}
	}
	group.Wait()
	if verificationCalls.Load() != 1 {
		t.Fatal("verification not shared across API handlers", verificationCalls.Load())
	}
}

func FuzzAuthenticatedJSONHandlers(f *testing.F) {
	f.Add(byte(0), shortBody())
	f.Add(byte(1), mailBody())
	f.Add(byte(2), notifyBody())
	f.Add(byte(2), []byte(`{"service":"slACk","webhookURL":"https://hooks.slACk.Com#","text":""}`))
	f.Add(byte(2), []byte(`{"service":"slack","webhookURL":"https://sub.hooks.slack.com/services/fake","text":"ready"}`))
	f.Add(byte(0), []byte(`{"provider":null,"provider":"s3"}`))
	f.Fuzz(func(t *testing.T, route byte, body []byte) {
		if len(body) > 20000 {
			return
		}
		paths := []string{"/api/v1/shorten", "/api/v1/mail/send", "/api/v1/notify"}
		handler := middleware.New(configuration(), upstreamFunc(successUpstream))
		response := send(handler, "POST", paths[int(route)%len(paths)], "fake-token", "", body)
		if response.Code != 200 && response.Code != 400 && response.Code != 403 && response.Code != 413 {
			t.Fatalf("unexpected response %d: %s", response.Code, response.Body.String())
		}
		if response.Code != 200 || response.Body.Len() > 0 {
			var envelope any
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal("non-JSON handler response", err)
			}
		}
	})
}
