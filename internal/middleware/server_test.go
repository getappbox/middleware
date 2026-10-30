package middleware

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"howett.net/plist"
)

//go:embed testdata/storage-links-v1.json
var storageFixture []byte

//go:embed testdata/swift-emails-v1.json
var swiftEmails []byte

//go:embed testdata/swift-http-v1.json
var swiftHTTP []byte

func TestSwiftHTTPGoldenResponses(t *testing.T) {
	var cases []struct {
		Method         string            `json:"method"`
		Path           string            `json:"path"`
		RequestHeaders map[string]string `json:"requestHeaders"`
		Status         int               `json:"status"`
		Body           any               `json:"body"`
		Headers        map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(swiftHTTP, &cases); err != nil {
		t.Fatal(err)
	}
	s := testServer(t, nil)
	for _, tc := range cases {
		t.Run(tc.Method+" "+tc.Path, func(t *testing.T) {
			r := httptest.NewRequest(tc.Method, tc.Path, nil)
			for name, value := range tc.RequestHeaders {
				r.Header.Set(name, value)
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			assertStatus(t, w, tc.Status, "")
			var body any = w.Body.String()
			var decoded any
			if json.Unmarshal(w.Body.Bytes(), &decoded) == nil {
				body = decoded
			}
			if !reflect.DeepEqual(body, tc.Body) {
				t.Errorf("body %#v, Swift %#v", body, tc.Body)
			}
			for name, want := range tc.Headers {
				if got := w.Header().Get(name); got != want {
					t.Errorf("%s=%q, Swift %q", name, got, want)
				}
			}
		})
	}
}

type clientFunc func(*http.Request) (*http.Response, error)

func (f clientFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func reply(status int, body string) (*http.Response, error) {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}, nil
}

func testConfig() Config {
	return Config{
		ClientToken: "test-client-token", DropboxAppKey: "test-app-key", DropboxTokenCacheTTL: 10 * time.Minute,
		MailgunAPIKey: "test-mailgun-key", MailgunDomain: "mail.getappbox.com", MailgunFrom: "AppBox Build <build@getappbox.com>",
		YOURLSAPIURL: "https://appbox.me/yourls-api.php", YOURLSSignatureSecret: "test-yourls-secret", ShortlinkTargetBase: "https://web.getappbox.com",
		CORSProxyAllowedHosts: []string{"dropbox.com", "dropboxusercontent.com", "getappbox.com"}, ProxyMaxBodyBytes: 4 * 1024 * 1024,
		GitHubLatestReleaseURL: "https://api.github.com/releases/latest", HomebrewCaskURL: "https://formulae.brew.sh/api/cask/appbox.json", UpdateCacheTTL: time.Hour,
		NotifyAllowedHosts: []string{"hooks.slack.com", "office.com", "office365.com", "logic.azure.com", "powerplatform.com"}, TrustedProxyCount: 1,
	}
}

func testServer(t *testing.T, upstream clientFunc) *Server {
	t.Helper()
	if upstream == nil {
		upstream = func(r *http.Request) (*http.Response, error) {
			switch r.URL.Hostname() {
			case "api.dropboxapi.com":
				return reply(200, `{"account_id":"dbid:test"}`)
			case "appbox.me":
				return reply(200, `{"shorturl":"https://appbox.me/x1"}`)
			case "api.mailgun.net":
				return reply(200, `{"id":"<msg-id>"}`)
			case "hooks.slack.com", "tenant.webhook.office.com", "tenant.logic.azure.com", "tenant.powerplatform.com":
				return reply(200, "")
			default:
				t.Errorf("unexpected upstream %s", r.URL)
				return nil, errors.New("unexpected upstream")
			}
		}
	}
	return New(testConfig(), upstream)
}

func perform(s *Server, method, path, auth string, body any) *httptest.ResponseRecorder {
	var data []byte
	if raw, ok := body.(string); ok {
		data = []byte(raw)
	} else if body != nil {
		data, _ = json.Marshal(body)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(data))
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if auth == "client" || auth == "full" {
		r.Header.Set("X-AppBox-Client-Token", "test-client-token")
	}
	if auth == "full" {
		r.Header.Set("Authorization", "Bearer test-dropbox-token")
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d, expected %d: %s", w.Code, status, w.Body.String())
	}
	if code != "" {
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Error.Code != code {
			t.Fatalf("error code %q, expected %q", envelope.Error.Code, code)
		}
	}
}

func shortenBody() map[string]any {
	return map[string]any{"url": "https://www.dropbox.com/scl/fi/x/appinfo.json?rlkey=abc", "name": "Demo", "version": "1.0", "build": "7", "identifier": "com.test.demo"}
}

func TestHealthAndRouting(t *testing.T) {
	s := testServer(t, nil)
	w := perform(s, "GET", "/", "", nil)
	assertStatus(t, w, 200, "")
	if w.Body.String() != "AppBox Install Service Helper" {
		t.Fatal(w.Body.String())
	}
	if w.Header().Get("Content-Length") != "29" {
		t.Fatal(w.Header())
	}
	w = perform(s, "HEAD", "/", "", nil)
	if w.Body.Len() != 0 || w.Code != 200 {
		t.Fatal(w)
	}
	for _, route := range []struct{ method, path string }{{"POST", "/"}, {"GET", "/unknown"}, {"GET", "/api/v1/shorten"}, {"POST", "/api/v1/config"}} {
		assertStatus(t, perform(s, route.method, route.path, "", nil), 404, "")
	}
}

func TestCORSPreflightAndErrors(t *testing.T) {
	s := testServer(t, nil)
	for _, method := range []string{"OPTIONS", "GET"} {
		r := httptest.NewRequest(method, "/api/v1/config", nil)
		r.Header.Set("Origin", "https://web.getappbox.com")
		r.Header.Set("Access-Control-Request-Method", "GET")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		want := 401
		if method == "OPTIONS" {
			want = 200
		}
		assertStatus(t, w, want, "")
		if w.Header().Get("Access-Control-Allow-Origin") != "*" || w.Header().Get("Access-Control-Max-Age") != "600" {
			t.Fatal(w.Header())
		}
	}
	if perform(s, "GET", "/", "", nil).Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS headers without Origin")
	}
}

func TestConfigAuthenticationAndMissingKey(t *testing.T) {
	s := testServer(t, nil)
	assertStatus(t, perform(s, "GET", "/api/v1/config", "", nil), 401, "invalid_client_token")
	r := httptest.NewRequest("GET", "/api/v1/config", nil)
	r.Header.Set("X-AppBox-Client-Token", "wrong")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	assertStatus(t, w, 401, "invalid_client_token")
	w = perform(s, "GET", "/api/v1/config/", "client", nil)
	assertStatus(t, w, 200, "")
	if w.Body.String() != `{"dropboxAppKey":"test-app-key"}` {
		t.Fatal(w.Body.String())
	}
	s.config.DropboxAppKey = ""
	assertStatus(t, perform(s, "GET", "/api/v1/config", "client", nil), 500, "")
	s.config.ClientToken = ""
	assertStatus(t, perform(s, "GET", "/api/v1/config", "client", nil), 401, "invalid_client_token")
}

func TestBearerVerificationAndCache(t *testing.T) {
	for _, status := range []int{200, 400, 401, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			s := testServer(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Hostname() == "api.dropboxapi.com" {
					calls++
					if r.Method != "POST" || r.URL.Path != "/2/users/get_current_account" || r.Header.Get("Authorization") != "Bearer test-dropbox-token" {
						t.Fatal(r)
					}
					return reply(status, `{"account_id":"acct"}`)
				}
				return reply(200, `{"shorturl":"https://appbox.me/x1"}`)
			})
			want, code, count := 200, "", 1
			if status == 400 || status == 401 {
				want, code = 401, "invalid_dropbox_token"
			} else if status != 200 {
				want, code, count = 503, "verification_unavailable", 2
			}
			for range 2 {
				assertStatus(t, perform(s, "POST", "/api/v1/shorten", "full", shortenBody()), want, code)
			}
			if calls != count {
				t.Fatalf("verification calls %d, expected %d", calls, count)
			}
		})
	}
	s := testServer(t, func(r *http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	assertStatus(t, perform(s, "POST", "/api/v1/shorten", "full", shortenBody()), 503, "verification_unavailable")
}

func TestAuthenticatedRoutesRequireBothTokens(t *testing.T) {
	for _, path := range []string{"/api/v1/shorten", "/api/v1/mail/send", "/api/v1/notify"} {
		s := testServer(t, nil)
		assertStatus(t, perform(s, "POST", path, "", nil), 401, "invalid_client_token")
		assertStatus(t, perform(s, "POST", path, "client", nil), 401, "missing_dropbox_token")
		for _, auth := range []string{"", "Bearer", "Bearer ", "Basic abc"} {
			r := httptest.NewRequest("POST", path, nil)
			r.Header.Set("X-AppBox-Client-Token", "test-client-token")
			r.Header.Set("Authorization", auth)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			assertStatus(t, w, 401, "missing_dropbox_token")
		}
	}
}

func TestRequestBodyCompatibility(t *testing.T) {
	for _, path := range []string{"/api/v1/shorten", "/api/v1/mail/send", "/api/v1/notify"} {
		s := testServer(t, nil)
		for _, body := range []string{"", "{}", "null", "[]", "{", `{"name":null}`, `{"name":12}`, `{} {}`} {
			assertStatus(t, perform(s, "POST", path, "full", body), 400, "invalid_body")
		}
		assertStatus(t, perform(s, "POST", path, "full", strings.Repeat("a", 16385)), 413, "")
	}
	s := testServer(t, nil)
	body := shortenBody()
	delete(body, "version")
	assertStatus(t, perform(s, "POST", "/api/v1/shorten", "full", body), 400, "invalid_body")
	body = shortenBody()
	body["build"] = 7
	assertStatus(t, perform(s, "POST", "/api/v1/shorten", "full", body), 400, "invalid_body")
	body = shortenBody()
	body["build"] = ""
	body["unknown"] = true
	assertStatus(t, perform(s, "POST", "/api/v1/shorten", "full", body), 200, "")
}

func TestSharedStorageLinkContract(t *testing.T) {
	var fixture struct {
		Links   []map[string]any `json:"links"`
		Invalid []string         `json:"invalidInstallLinks"`
	}
	if err := json.Unmarshal(storageFixture, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, link := range fixture.Links {
		request := shortenRequest{}
		if link["provider"] == "s3" {
			value := "s3"
			request.Provider = &value
			request.URL = link["installURL"].(string)
		} else {
			request.URL = link["sharedURL"].(string)
		}
		got, err := shortlinkTarget(request, "https://web.getappbox.com")
		if err != nil || got != link["shortenerTarget"] {
			t.Fatalf("%s: %s, %v", request.URL, got, err)
		}
	}
	for _, target := range append(fixture.Invalid,
		"https://evil.test/?provider=s3&url=https%3A%2F%2Fcdn.test%2Fappinfo.json",
		"https://web.getappbox.com.evil.test/?provider=s3&url=https%3A%2F%2Fcdn.test%2Fappinfo.json",
		"https://web.getappbox.com:8443/?provider=s3&url=https%3A%2F%2Fcdn.test%2Fappinfo.json",
		"https://web.getappbox.com/other/?provider=s3&url=https%3A%2F%2Fcdn.test%2Fappinfo.json",
		"https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fcdn.test%2Fappinfo.json&extra=1") {
		if validInstallLink(target, "https://web.getappbox.com") {
			t.Fatalf("accepted %s", target)
		}
	}
	if !validInstallLink("https://web2.test/app/?provider=s3&url=https%3A%2F%2Fcdn.test%2Fappinfo.json", "https://web2.test/app/") {
		t.Fatal("custom base")
	}
	for _, provider := range []string{"", "box", "webDAV"} {
		_, err := shortlinkTarget(shortenRequest{URL: "https://www.dropbox.com/s/a/appinfo.json", Provider: &provider}, "https://web.getappbox.com")
		if api, ok := err.(apiError); !ok || api.code != "invalid_provider" {
			t.Fatal(err)
		}
	}
}

func TestS3ShorteningEchoAndValidation(t *testing.T) {
	s := testServer(t, nil)
	body := shortenBody()
	body["provider"] = "s3"
	body["url"] = "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fcdn.test%2Fappinfo.json"
	assertStatus(t, perform(s, "POST", "/api/v1/shorten", "client", body), 401, "missing_dropbox_token")
	w := perform(s, "POST", "/api/v1/shorten", "full", body)
	assertStatus(t, w, 200, "")
	var result map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result["provider"] != "s3" || result["shortURL"] != "https://appbox.me/x1" {
		t.Fatal(result)
	}
	body["url"] = "https://evil.test/?provider=s3&url=https%3A%2F%2Fcdn.test%2Fappinfo.json"
	assertStatus(t, perform(s, "POST", "/api/v1/shorten", "full", body), 400, "invalid_url")
	w = perform(s, "POST", "/api/v1/shorten", "full", shortenBody())
	if strings.Contains(w.Body.String(), "provider") {
		t.Fatal(w.Body.String())
	}
}

func TestYOURLSFormAndRetryCompatibility(t *testing.T) {
	for _, tc := range []struct {
		status         int
		body           string
		attempts, want int
	}{
		{200, `{"shorturl":"https://appbox.me/a"}`, 1, 200}, {400, `{"shorturl":"https://appbox.me/a"}`, 1, 200},
		{400, `{}`, 1, 502}, {500, `{}`, 3, 502}, {200, `{"shorturl":""}`, 3, 502}, {200, `not json`, 3, 502},
	} {
		t.Run(fmt.Sprintf("%d-%s", tc.status, tc.body), func(t *testing.T) {
			calls := 0
			s := testServer(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Hostname() == "api.dropboxapi.com" {
					return reply(200, `{}`)
				}
				calls++
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				for key, expected := range map[string]string{"action": "shorturl", "format": "json", "hash": "sha512", "url": "https://web.getappbox.com?url=/scl/fi/x/appinfo.json?rlkey=abc", "title": "Demo, com.test.demo, 1.0, 7"} {
					if r.Form.Get(key) != expected {
						t.Fatalf("%s: %s", key, r.Form.Get(key))
					}
				}
				if r.Form.Get("signature") != sha512Hex(r.Form.Get("timestamp")+"test-yourls-secret") || len(r.Form.Get("userAuthToken")) != 128 {
					t.Fatal(r.Form)
				}
				return reply(tc.status, tc.body)
			})
			assertStatus(t, perform(s, "POST", "/api/v1/shorten", "full", shortenBody()), tc.want, "")
			if calls != tc.attempts {
				t.Fatalf("calls %d, expected %d", calls, tc.attempts)
			}
		})
	}
	if sha512Hex("abc") != "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f" {
		t.Fatal("SHA-512")
	}
	s := testServer(t, nil)
	s.config.YOURLSSignatureSecret = ""
	assertStatus(t, perform(s, "POST", "/api/v1/shorten", "full", shortenBody()), 500, "misconfigured")
}

func mailBody() mailRequest {
	return mailRequest{Name: "MyApp", Version: "2.0", Build: "42", To: []string{"a@b.com", "c@d.io"}, InstallURL: "https://appbox.me/x1"}
}

func TestMailValidationAndProvider(t *testing.T) {
	for _, recipients := range [][]string{nil, {}, {"not-an-email"}, {"a @b.com"}, strings.Split(strings.Repeat("a@b.com,", 100)+"a@b.com", ",")} {
		body := mailBody()
		body.To = recipients
		status, code := 400, "invalid_recipients"
		if recipients == nil {
			code = "invalid_body"
		}
		assertStatus(t, perform(testServer(t, nil), "POST", "/api/v1/mail/send", "full", body), status, code)
	}
	for _, tc := range []struct {
		status int
		body   string
		want   int
	}{{200, `{"id":"<msg-id>"}`, 200}, {202, `{}`, 200}, {400, `{}`, 502}, {503, `{}`, 502}} {
		s := testServer(t, func(r *http.Request) (*http.Response, error) {
			if r.URL.Hostname() == "api.dropboxapi.com" {
				return reply(200, `{}`)
			}
			username, password, _ := r.BasicAuth()
			if username != "api" || password != "test-mailgun-key" || r.Method != "POST" || r.URL.Path != "/v3/mail.getappbox.com/messages" {
				t.Fatal(r)
			}
			_ = r.ParseForm()
			if r.Form.Get("to") != "a@b.com,c@d.io" || r.Form.Get("subject") != "MyApp 2.0 (42) is ready to test" || !strings.Contains(r.Form.Get("html"), "Install MyApp") || !strings.Contains(r.Form.Get("text"), "https://appbox.me/x1") {
				t.Fatal(r.Form)
			}
			return reply(tc.status, tc.body)
		})
		w := perform(s, "POST", "/api/v1/mail/send", "full", mailBody())
		assertStatus(t, w, tc.want, "")
		if tc.status == 202 && w.Body.String() != "{}" {
			t.Fatal(w.Body.String())
		}
	}
	s := testServer(t, nil)
	s.config.MailgunAPIKey = ""
	assertStatus(t, perform(s, "POST", "/api/v1/mail/send", "full", mailBody()), 500, "misconfigured")
}

func TestSwiftEmailGoldenBytes(t *testing.T) {
	var expected []map[string]string
	if err := json.Unmarshal(swiftEmails, &expected); err != nil {
		t.Fatal(err)
	}
	message := "<b>bold</b>\r\nSecond line"
	samples := []mailRequest{mailBody(), {Name: "<script>alert(1)</script>", Version: "1.0 & 2.0", Build: `"42"`, InstallURL: "https://appbox.me/x?a=1&b=2", PersonalMessage: &message}}
	for i, sample := range samples {
		for name, got := range map[string]string{"subject": emailSubject(sample), "html": emailHTML(sample), "text": emailText(sample)} {
			if got != expected[i][name] {
				t.Errorf("sample %d %s differs from Swift: got %q, expected %q", i, name, got, expected[i][name])
			}
		}
	}
}

func TestNotifyPayloadAndProviderErrors(t *testing.T) {
	for _, service := range []string{"slack", "teams", "TEAMS"} {
		s := testServer(t, func(r *http.Request) (*http.Response, error) {
			if r.URL.Hostname() == "api.dropboxapi.com" {
				return reply(200, `{}`)
			}
			var got map[string]any
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			var expected map[string]any
			if service == "slack" {
				expected = map[string]any{"username": "AppBox", "icon_url": "https://getappbox.com/images/AppBoxIcon.png", "text": "Build is ready"}
			} else {
				data := []byte(`{"type":"message","attachments":[{"contentType":"application/vnd.microsoft.card.adaptive","content":{"$schema":"http://adaptivecards.io/schemas/adaptive-card.json","type":"AdaptiveCard","version":"1.4","body":[{"type":"TextBlock","text":"Build is ready","wrap":true}]}}]}`)
				_ = json.Unmarshal(data, &expected)
			}
			if !reflect.DeepEqual(got, expected) {
				t.Fatal(got)
			}
			if r.Header.Get("Authorization") != "" || r.Header.Get("X-AppBox-Client-Token") != "" {
				t.Fatal("publisher credentials leaked")
			}
			return reply(202, "")
		})
		w := perform(s, "POST", "/api/v1/notify", "full", notifyRequest{service, "https://hooks.slack.com/services/x", "Build is ready"})
		assertStatus(t, w, 200, "")
		if w.Body.Len() != 0 {
			t.Fatal(w.Body.String())
		}
	}
	for _, tc := range []struct {
		service, webhook, code string
		status                 int
	}{
		{"bad", "https://hooks.slack.com/x", "invalid_service", 400}, {"slack", "http://hooks.slack.com/x", "webhook_not_allowed", 403},
		{"slack", "https://evilhooks.slack.com/x", "webhook_not_allowed", 403}, {"teams", "https://tenant.logic.azure.com/x", "", 200},
		{"teams", "https://tenant.powerplatform.com/x", "", 200}, {"teams", "https://tenant.webhook.office.com/x", "", 200},
	} {
		assertStatus(t, perform(testServer(t, nil), "POST", "/api/v1/notify", "full", notifyRequest{tc.service, tc.webhook, "Build"}), tc.status, tc.code)
	}
	for _, status := range []int{400, 429, 500} {
		s := testServer(t, func(r *http.Request) (*http.Response, error) {
			if r.URL.Hostname() == "api.dropboxapi.com" {
				return reply(200, `{}`)
			}
			return reply(status, "")
		})
		assertStatus(t, perform(s, "POST", "/api/v1/notify", "full", notifyRequest{"slack", "https://hooks.slack.com/x", "Build"}), 502, "notify_failed")
	}
}

func TestUpdateMergeCacheAndFailure(t *testing.T) {
	for _, tc := range []struct {
		github, brew int
		want         int
	}{{200, 200, 200}, {200, 500, 200}, {500, 200, 502}} {
		var calls atomic.Int32
		s := testServer(t, func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			if r.Header.Get("User-Agent") != "AppBox-middleware" {
				t.Error("User-Agent")
			}
			if r.URL.Hostname() == "api.github.com" {
				return reply(tc.github, `{"tag_name":"3.8.0","html_url":"https://github.com/getappbox/x/releases/tag/3.8.0"}`)
			}
			return reply(tc.brew, `{"version":"3.7.9"}`)
		})
		assertStatus(t, perform(s, "GET", "/api/v1/latest-version", "", nil), 401, "invalid_client_token")
		for range 2 {
			w := perform(s, "GET", "/api/v1/latest-version", "client", nil)
			assertStatus(t, w, tc.want, "")
			if tc.want == 200 {
				var got map[string]string
				_ = json.Unmarshal(w.Body.Bytes(), &got)
				if got["version"] != "3.8.0" || got["downloadURL"] == "" || (tc.brew == 200 && got["homebrewVersion"] != "3.7.9") {
					t.Fatal(got)
				}
				if tc.brew != 200 {
					if _, exists := got["homebrewVersion"]; exists {
						t.Fatal(got)
					}
				}
			}
		}
		wantCalls := int32(2)
		if tc.want == 502 {
			wantCalls = 4
		}
		if calls.Load() != wantCalls {
			t.Fatal(calls.Load())
		}
	}
}

func TestHostAllowlist(t *testing.T) {
	for host, want := range map[string]bool{"dropbox.com": true, "www.dropbox.com": true, "WEB.GETAPPBOX.COM": true, "evildropbox.com": false, "dropbox.com.evil.io": false, "": false} {
		if allowedHost(host, []string{"dropbox.com", "getappbox.com"}) != want {
			t.Error(host)
		}
	}
}

func TestCORSProxyValidationAndBoundedBody(t *testing.T) {
	s := testServer(t, nil)
	assertStatus(t, perform(s, "GET", "/cors", "", nil), 400, "")
	for _, target := range []string{"https://169.254.169.254/latest/meta-data", "https://evil.example.com/x", "http://www.dropbox.com/scl/x", "https://notdropbox.com.evil.com/x"} {
		assertStatus(t, perform(s, "GET", "/cors?url="+url.QueryEscape(target), "", nil), 403, "")
	}
	for _, declared := range []int64{-1, 100} {
		s = testServer(t, func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("Authorization") != "" {
				t.Fatal("unexpected credentials")
			}
			response, _ := reply(200, strings.Repeat("x", 100))
			response.ContentLength = declared
			return response, nil
		})
		s.config.ProxyMaxBodyBytes = 20
		assertStatus(t, perform(s, "GET", "/cors?url=https://www.dropbox.com/s/a/info.json", "", nil), 502, "")
	}
}

func TestProxyResponseHeadersAndBody(t *testing.T) {
	s := testServer(t, func(r *http.Request) (*http.Response, error) {
		response, _ := reply(404, "body")
		response.Header = http.Header{
			"Content-Type": {"application/xml"}, "Content-Encoding": {"gzip"}, "Content-Disposition": {`attachment; filename="x"`}, "Cache-Control": {"no-cache", "no-store"},
			"Content-Length": {"99999", "123"}, "Set-Cookie": {"secret=token"}, "Content-Security-Policy": {strings.Repeat("a", 2670)}, "X-Dropbox-Request-Id": {"id"}, "X-Content-Type-Options": {"nosniff"},
		}
		return response, nil
	})
	w := perform(s, "GET", "/cors?url=https://www.dropbox.com/s/a/info.json", "", nil)
	assertStatus(t, w, 404, "")
	if w.Body.String() != "body" || w.Header().Get("Content-Length") != "4" || len(w.Header().Values("Content-Length")) != 1 {
		t.Fatal(w)
	}
	for _, name := range []string{"Content-Type", "Content-Encoding", "Content-Disposition", "Cache-Control"} {
		if w.Header().Get(name) == "" {
			t.Error(name)
		}
	}
	for _, name := range []string{"Set-Cookie", "Content-Security-Policy", "X-Dropbox-Request-Id", "X-Content-Type-Options"} {
		if w.Header().Get(name) != "" {
			t.Error(name)
		}
	}
}

func TestDropboxPathValidation(t *testing.T) {
	for _, path := range []string{"/scl/fi/abc/manifest.plist", "/s/abc/appinfo.json"} {
		if err := validateDropboxPath(path); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"/other/thing", "/scl/../../etc/passwd", "/scl/" + strings.Repeat("a", 1025)} {
		if validateDropboxPath(path) == nil {
			t.Fatal(path)
		}
	}
	for _, path := range []string{"/install/notascl/manifest.plist", "/install/scl/../../etc/passwd", "/appinfo/scl/../../etc/passwd"} {
		assertStatus(t, perform(testServer(t, nil), "GET", path, "", nil), 400, "")
	}
}

func TestDropboxURLLegacyEncoding(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"/install/scl/fi/abc/queryparam-rlkey-value-xyz/manifest.plist", "https://www.dropbox.com/scl/fi/abc/manifest.plist?dl=1&rlkey=xyz"},
		{"/appinfo/scl/fi/abc/appinfo.json?rlkey=xyz&dl=0", "https://www.dropbox.com/scl/fi/abc/appinfo.json?rlkey=xyz&dl=1"},
		{"/appinfo/s/abc/appinfo.json?$web_only=true&_branch_match_id=ignore&rlkey=xyz", "https://www.dropbox.com/s/abc/appinfo.json?rlkey=xyz&dl=1"},
		{"/appinfo/s/abc/appinfo.json&rlkey=xyz", "https://www.dropbox.com/s/abc/appinfo.json?rlkey=xyz&dl=1"},
		{"/appinfo/scl/fi/abc/QA%20build/appinfo.json?st=a%2Bb%3D&rlkey=xyz", "https://www.dropbox.com/scl/fi/abc/QA%2520build/appinfo.json?st=a%252Bb%253D&rlkey=xyz&dl=1"},
	} {
		r := httptest.NewRequest("GET", tc.path, nil)
		got, err := legacyDropboxURL(r, strings.Trim(r.URL.EscapedPath(), "/"))
		if err != nil || got != tc.want {
			t.Errorf("%s: got %s, want %s, %v", tc.path, got, tc.want, err)
		}
	}
}

func TestDropboxNoticePages(t *testing.T) {
	for _, tc := range []struct {
		title string
		want  int
	}{
		{"Dropbox - File Deleted - Simplify your life", 404}, {"Dropbox - Invalid Link - Simplify your life", 404}, {"Dropbox - Link temporarily disabled", 423}, {"Dropbox - Verify it's you", 502},
	} {
		for _, route := range []string{"/install/scl/fi/a/manifest.plist", "/appinfo/scl/fi/a/appinfo.json"} {
			s := testServer(t, func(r *http.Request) (*http.Response, error) {
				res, _ := reply(200, "<html><head><title>"+tc.title+"</title></head></html>")
				res.Header.Set("Content-Type", "text/html; charset=utf-8")
				return res, nil
			})
			want := tc.want
			if want == 502 && strings.HasPrefix(route, "/appinfo") {
				want = 200
			}
			assertStatus(t, perform(s, "GET", route, "", nil), want, "")
		}
	}
	for _, status := range []int{401, 404, 500} {
		s := testServer(t, func(r *http.Request) (*http.Response, error) {
			res, _ := reply(status, "<title>Login required</title>")
			res.Header.Set("Content-Type", "text/html")
			return res, nil
		})
		assertStatus(t, perform(s, "GET", "/install/s/a/manifest.plist", "", nil), status, "")
	}
}

func manifestValue() map[string]any {
	return map[string]any{"items": []any{map[string]any{
		"assets":   []any{map[string]any{"kind": "software-package", "url": "https://dl.dropboxusercontent.com/scl/fi/a/app.ipa?rlkey=x&dl=0&st=a%2Bb%3D"}, map[string]any{"kind": "display-image", "url": "https://cdn.test/icon.png"}},
		"metadata": map[string]any{"bundle-identifier": "com.test.demo", "bundle-version": "7", "kind": "software", "title": "Demo & QA"},
	}}}
}

func TestInstallManifestXMLAndBinary(t *testing.T) {
	for _, format := range []int{plist.XMLFormat, plist.BinaryFormat} {
		data, err := plist.Marshal(manifestValue(), format)
		if err != nil {
			t.Fatal(err)
		}
		s := testServer(t, func(r *http.Request) (*http.Response, error) {
			res, _ := reply(200, string(data))
			res.Header.Set("Content-Type", "application/octet-stream")
			return res, nil
		})
		w := perform(s, "GET", "/install/scl/fi/a/queryparam-rlkey-value-x/manifest.plist", "", nil)
		assertStatus(t, w, 200, "")
		if w.Header().Get("Content-Type") != "application/xml" || w.Header().Get("Content-Encoding") != "" || w.Header().Get("Content-Length") != fmt.Sprint(w.Body.Len()) {
			t.Fatal(w.Header())
		}
		var got map[string]any
		if _, err := plist.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		expected := manifestValue()
		expected["items"].([]any)[0].(map[string]any)["assets"].([]any)[0].(map[string]any)["url"] = "https://www.dropbox.com/scl/fi/a/app.ipa?rlkey=x&dl=1&st=a+b%3D"
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("manifest differs: %#v", got)
		}
	}
	for _, data := range []string{"not a plist", `<?xml version="1.0"?><plist version="1.0"><dict><key>items</key><array/></dict></plist>`} {
		s := testServer(t, func(r *http.Request) (*http.Response, error) { return reply(200, data) })
		w := perform(s, "GET", "/install/s/a/manifest.plist", "", nil)
		if w.Body.String() != data {
			t.Fatal("malformed/empty manifest changed")
		}
	}
}

func TestRateLimitsAndTrustedProxies(t *testing.T) {
	s := testServer(t, nil)
	for i := 0; i < 120; i++ {
		assertStatus(t, perform(s, "GET", "/api/v1/config", "client", nil), 200, "")
	}
	w := perform(s, "GET", "/api/v1/config", "client", nil)
	assertStatus(t, w, 429, "rate_limited")
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	for _, tc := range []struct {
		trusted         int
		forwarded, want string
	}{
		{1, "1.1.1.1, 9.9.9.9", "9.9.9.9"}, {2, "1.1.1.1,8.8.8.8,9.9.9.9", "8.8.8.8"}, {2, "7.7.7.7", "7.7.7.7"}, {1, "1.1.1.1, , 9.9.9.9", "9.9.9.9"}, {0, "1.1.1.1,9.9.9.9", "127.0.0.1"}, {1, "", "127.0.0.1"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("X-Forwarded-For", tc.forwarded)
		if got := clientIP(r, tc.trusted); got != tc.want {
			t.Errorf("%+v: %s", tc, got)
		}
	}
	var store rateStore
	now := time.Now()
	if store.register("b:9.9.9.9", 1, now) != 0 || store.register("b:9.9.9.9", 1, now) != 60 || store.register("b:8.8.8.8", 1, now) != 0 || store.register("other:9.9.9.9", 1, now) != 0 || store.register("b:9.9.9.9", 1, now.Add(time.Minute)) != 0 {
		t.Fatal(store.windows)
	}
}

func TestTokenStoreExpirySweepAndCapacity(t *testing.T) {
	now := time.Now()
	store := newTokenStore()
	store.maxEntries = 3
	key := func(value string) [32]byte { return sha256.Sum256([]byte(value)) }
	store.put(key("a"), tokenVerdict{true, "a"}, time.Second, now)
	store.put(key("b"), tokenVerdict{true, "b"}, time.Hour, now)
	store.put(key("c"), tokenVerdict{false, ""}, time.Hour, now)
	store.put(key("d"), tokenVerdict{true, "d"}, time.Hour, now)
	if len(store.entries) != 3 {
		t.Fatal(len(store.entries))
	}
	if _, ok := store.get(key("a"), now); ok {
		t.Fatal("earliest entry retained")
	}
	store.put(key("gone"), tokenVerdict{}, -time.Second, now)
	if _, ok := store.get(key("gone"), now); ok {
		t.Fatal("expired entry returned")
	}
	store.put(key("expired-unread"), tokenVerdict{}, -time.Second, now)
	store.put(key("live"), tokenVerdict{true, "live"}, time.Hour, now.Add(time.Minute))
	if _, ok := store.entries[key("expired-unread")]; ok {
		t.Fatal("expired unread entry retained")
	}
}

func TestConcurrentState(t *testing.T) {
	s := testServer(t, nil)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Go(func() {
			assertStatus(t, perform(s, "POST", "/api/v1/shorten", "full", shortenBody()), 200, "")
		})
	}
	wg.Wait()
	if len(s.tokens.entries) != 1 {
		t.Fatal("unexpected token cache size")
	}
}
