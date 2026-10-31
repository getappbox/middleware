package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestShortenForwardsOnlyResolvedIP(t *testing.T) {
	for _, tc := range []struct {
		name, peer, forwarded, want string
		proxies                     int
		providerS3                  bool
	}{
		{"direct", "192.0.2.10:1234", "", "192.0.2.10", 1, false},
		{"normalized", "192.0.2.10:1234", "198.51.100.7", "198.51.100.7", 1, false},
		{"forged prefix", "192.0.2.10:1234", "203.0.113.99, 198.51.100.7", "198.51.100.7", 1, false},
		{"two proxies", "192.0.2.10:1234", "203.0.113.99,198.51.100.7,192.0.2.20", "198.51.100.7", 2, false},
		{"no proxy trust", "192.0.2.10:1234", "203.0.113.99", "192.0.2.10", 0, false},
		{"untrusted socket", "192.0.2.20:1234", "198.51.100.7", "192.0.2.20", 1, false},
		{"IPv6", "[2001:db8::10]:1234", "2001:0db8:0:0::7", "2001:db8::7", 1, false},
		{"mapped IPv4", "192.0.2.10:1234", "::ffff:198.51.100.7", "198.51.100.7", 1, false},
		{"invalid selected hop", "192.0.2.10:1234", "198.51.100.7,not-an-ip", "192.0.2.10", 1, false},
		{"port in header", "192.0.2.10:1234", "198.51.100.7:80", "192.0.2.10", 1, false},
		{"scoped IPv6", "192.0.2.10:1234", "fe80::7%eth0", "192.0.2.10", 1, false},
		{"missing socket", "", "", "", 1, false},
		{"unknown socket", "unix-peer", "", "", 0, false},
		{"S3", "192.0.2.10:1234", "198.51.100.7", "198.51.100.7", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			s := testServer(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Hostname() == "api.dropboxapi.com" {
					if r.Header.Get("X-Forwarded-For") != "" {
						t.Fatal("client IP leaked to Dropbox verification")
					}
					return reply(200, `{"account_id":"dbid:test"}`)
				}
				calls++
				if got := r.Header.Get("X-Forwarded-For"); got != tc.want {
					t.Errorf("forwarded IP %q, want %q", got, tc.want)
				}
				for _, header := range []string{"CF-Connecting-IP", "X-Real-IP", "Forwarded", "Authorization", "X-AppBox-Client-Token"} {
					if r.Header.Get(header) != "" {
						t.Errorf("unexpected forwarded header %s", header)
					}
				}
				if calls == 1 {
					return reply(503, `{}`)
				}
				return reply(200, `{"shorturl":"https://appbox.me/x1"}`)
			})
			s.config.TrustedProxyCount = tc.proxies
			s.config.TrustedProxyCIDRs = []string{"192.0.2.10/32", "2001:db8::10/128"}
			body := shortenBody()
			if tc.providerS3 {
				body["provider"] = "s3"
				body["url"] = "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fcdn.example.com%2Fappinfo.json"
			}
			data, _ := json.Marshal(body)
			r := httptest.NewRequest(http.MethodPost, "/api/v1/shorten", bytes.NewReader(data))
			r.RemoteAddr = tc.peer
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-AppBox-Client-Token", "test-client-token")
			r.Header.Set("Authorization", "Bearer test-dropbox-token")
			r.Header.Set("X-Forwarded-For", tc.forwarded)
			r.Header.Set("CF-Connecting-IP", "203.0.113.98")
			r.Header.Set("X-Real-IP", "203.0.113.97")
			r.Header.Set("Forwarded", "for=203.0.113.96")
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			assertStatus(t, w, 200, "")
			if calls != 2 {
				t.Fatalf("attempts %d, want 2", calls)
			}
			var got map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got["shortURL"] != "https://appbox.me/x1" || (tc.providerS3 && got["provider"] != "s3") {
				t.Fatalf("shortening response changed: %s", w.Body.String())
			}
		})
	}
}

func TestTrustedProxyPeerResolution(t *testing.T) {
	for _, tc := range []struct {
		name, peer, want string
		trusted          []string
	}{
		{"legacy count-only", "192.0.2.10:1234", "198.51.100.7", nil},
		{"exact IPv4", "192.0.2.10:1234", "198.51.100.7", []string{"192.0.2.10"}},
		{"IPv4 range", "192.0.2.10:1234", "198.51.100.7", []string{"192.0.2.0/24"}},
		{"untrusted peer", "192.0.2.20:1234", "192.0.2.20", []string{"192.0.2.10"}},
		{"exact IPv6", "[2001:db8::10]:1234", "198.51.100.7", []string{"2001:0db8::10"}},
		{"IPv6 range", "[2001:db8::10]:1234", "198.51.100.7", []string{"2001:db8::/64"}},
		{"mapped peer", "[::ffff:192.0.2.10]:1234", "198.51.100.7", []string{"192.0.2.10/32"}},
		{"different family", "[2001:db8::10]:1234", "2001:db8::10", []string{"192.0.2.0/24"}},
		{"invalid ranges fail closed", "192.0.2.10:1234", "192.0.2.10", []string{"invalid", "192.0.2.0/33"}},
		{"empty entries fail closed", "192.0.2.10:1234", "192.0.2.10", []string{"", " "}},
		{"mixed ranges", "192.0.2.10:1234", "198.51.100.7", []string{"invalid", " 192.0.2.10 "}},
		{"missing peer", "", "unknown", []string{"192.0.2.10"}},
		{"unknown peer", "unix-peer", "unix-peer", []string{"192.0.2.10"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.peer
			r.Header.Add("X-Forwarded-For", "203.0.113.99")
			r.Header.Add("X-Forwarded-For", "198.51.100.7")
			if got := clientIP(r, 1, tc.trusted...); got != tc.want {
				t.Fatalf("resolved IP %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUntrustedPeerCannotChangeRateLimitIP(t *testing.T) {
	s := testServer(t, nil)
	s.config.TrustedProxyCIDRs = []string{"192.0.2.10/32"}
	for i := range 121 {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
		r.RemoteAddr = "192.0.2.20:1234"
		r.Header.Set("X-AppBox-Client-Token", "test-client-token")
		r.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(i+1))
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		want := 200
		if i == 120 {
			want = 429
		}
		assertStatus(t, w, want, "")
	}
}

func TestTrustedProxyCIDRsEnvironment(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", " 192.0.2.10/32, ,2001:db8::10,invalid ")
	want := []string{"192.0.2.10/32", "", "2001:db8::10", "invalid"}
	if got := ConfigFromEnvironment().TrustedProxyCIDRs; !reflect.DeepEqual(got, want) {
		t.Fatalf("proxy ranges %v, want %v", got, want)
	}
	t.Setenv("TRUSTED_PROXY_CIDRS", "")
	if got := ConfigFromEnvironment().TrustedProxyCIDRs; len(got) != 0 {
		t.Fatalf("empty proxy ranges %v", got)
	}
	for _, malformed := range []string{" ", ",", " , , "} {
		t.Setenv("TRUSTED_PROXY_CIDRS", malformed)
		if got := ConfigFromEnvironment().TrustedProxyCIDRs; len(got) == 0 || trustedProxyPeer("192.0.2.10", got) {
			t.Fatalf("malformed proxy ranges did not fail closed: %q", malformed)
		}
	}
}

func TestOtherProvidersDoNotReceiveClientIP(t *testing.T) {
	for _, tc := range []struct {
		path string
		body any
	}{
		{"/api/v1/mail/send", mailBody()},
		{"/api/v1/notify", notifyRequest{Service: "slack", WebhookURL: "https://hooks.slack.com/test", Text: "test"}},
		{"/cors?url=https://www.dropbox.com/s/test/appinfo.json", nil},
	} {
		t.Run(tc.path, func(t *testing.T) {
			calls := 0
			s := testServer(t, func(r *http.Request) (*http.Response, error) {
				calls++
				for _, header := range []string{"X-Forwarded-For", "CF-Connecting-IP", "X-Real-IP", "Forwarded"} {
					if r.Header.Get(header) != "" {
						t.Errorf("client header %s reached %s", header, r.URL.Hostname())
					}
				}
				return reply(200, `{}`)
			})
			data, _ := json.Marshal(tc.body)
			method := http.MethodPost
			if tc.body == nil {
				method = http.MethodGet
			}
			r := httptest.NewRequest(method, tc.path, bytes.NewReader(data))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-AppBox-Client-Token", "test-client-token")
			r.Header.Set("Authorization", "Bearer test-dropbox-token")
			for _, header := range []string{"X-Forwarded-For", "CF-Connecting-IP", "X-Real-IP", "Forwarded"} {
				r.Header.Set(header, "198.51.100.7")
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			assertStatus(t, w, 200, "")
			if calls == 0 {
				t.Fatal("no provider request was exercised")
			}
		})
	}
}

func TestShortenToInternalHTTPYOURLS(t *testing.T) {
	calls := 0
	yourls := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/yourls-api.php" {
			t.Errorf("unexpected internal request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Forwarded-For"); got != "198.51.100.7" || len(r.Header.Values("X-Forwarded-For")) != 1 {
			t.Errorf("internal HTTP forwarded IP %q", got)
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("action") != "shorturl" {
			t.Error("internal HTTP form changed", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"shorturl":"https://appbox.me/internal"}`))
	}))
	defer yourls.Close()
	s := testServer(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() == "api.dropboxapi.com" {
			return reply(200, `{"account_id":"dbid:test"}`)
		}
		return yourls.Client().Do(r)
	})
	s.config.YOURLSAPIURL = yourls.URL + "/yourls-api.php"
	s.config.TrustedProxyCIDRs = []string{"192.0.2.10/32"}
	data, _ := json.Marshal(shortenBody())
	r := httptest.NewRequest(http.MethodPost, "/api/v1/shorten", bytes.NewReader(data))
	r.RemoteAddr = "192.0.2.10:1234"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-AppBox-Client-Token", "test-client-token")
	r.Header.Set("Authorization", "Bearer test-dropbox-token")
	r.Header.Set("X-Forwarded-For", "203.0.113.99, 198.51.100.7")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	assertStatus(t, w, 200, "")
	if calls != 1 {
		t.Fatalf("internal HTTP calls %d, want 1", calls)
	}
}

func TestClientIPDoesNotLeakAcrossRedirectHosts(t *testing.T) {
	for _, tc := range []struct{ name, target, want string }{
		{"same host", "https://short.example.com/next", "198.51.100.7"},
		{"different host", "https://other.example.com/next", ""},
		{"subdomain", "https://sub.short.example.com/next", ""},
		{"different port", "https://short.example.com:8443/next", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := &http.Client{CheckRedirect: checkRedirect, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					response, _ := reply(307, "")
					response.Header.Set("Location", tc.target)
					return response, nil
				}
				if got := r.Header.Get("X-Forwarded-For"); got != tc.want {
					t.Errorf("redirect forwarded IP %q, want %q", got, tc.want)
				}
				return reply(200, `{}`)
			})}
			r, _ := http.NewRequest(http.MethodPost, "https://short.example.com/start", strings.NewReader("action=shorturl"))
			r.Header.Set("X-Forwarded-For", "198.51.100.7")
			response, err := client.Do(r)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if calls != 2 {
				t.Fatalf("requests %d, want 2", calls)
			}
		})
	}
}
