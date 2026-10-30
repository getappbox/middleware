package middleware

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"howett.net/plist"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBearerWhitespaceCompatibility(t *testing.T) {
	for _, header := range []string{"Bearer token", "Bearer  token", "  Bearer token  ", "bearer token"} {
		s := testServer(t, func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("Authorization") != "Bearer token" {
				t.Errorf("forwarded %q", r.Header.Get("Authorization"))
			}
			return reply(200, `{"account_id":"dbid:test"}`)
		})
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", header)
		if err := s.authenticate(r); err != nil {
			t.Fatal(header, err)
		}
	}
	for _, header := range []string{"Bearer token extra", "Bearer\ttoken", "Bearer"} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", header)
		if err := testServer(t, nil).authenticate(r); err == nil {
			t.Fatal(header)
		}
	}
}

func TestUnicodeEmailWhitespace(t *testing.T) {
	for _, space := range []string{"\u00a0", "\v", "\u2028", "\u3000", "\t", "\n", " "} {
		if validEmail("qa" + space + "@example.com") {
			t.Fatalf("accepted whitespace %q", space)
		}
	}
	for _, email := range []string{"a@b.com", "qa+build@example.com", "a@b..", "a@.b.c"} {
		if !validEmail(email) {
			t.Fatal(email)
		}
	}
}

func TestRedirectBoundariesWithRealClient(t *testing.T) {
	for _, tc := range []struct {
		target  string
		allowed bool
	}{
		{"https://dl.dropboxusercontent.com/s/a/file", true},
		{"https://internal.test/private", false}, {"http://www.dropbox.com/s/a/file", false},
		{"https://user:secret@www.dropbox.com/s/a/file", false},
		{"https://www.dropbox.com/start", false},
	} {
		t.Run(tc.target, func(t *testing.T) {
			calls := 0
			client := &http.Client{CheckRedirect: checkRedirect, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					response, _ := reply(302, "")
					response.Header.Set("Location", tc.target)
					return response, nil
				}
				if !tc.allowed {
					t.Error("contacted forbidden redirect destination")
				}
				return reply(200, "public file")
			})}
			r, _ := http.NewRequest("GET", "https://www.dropbox.com/start", nil)
			r = allowRedirectHosts(r, []string{"dropbox.com", "dropboxusercontent.com"})
			response, err := client.Do(r)
			if tc.allowed {
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
			} else if err == nil {
				response.Body.Close()
				t.Fatal("forbidden redirect succeeded")
			}
		})
	}
}

func TestRedirectLimitAndWebhookScope(t *testing.T) {
	count := 0
	client := &http.Client{CheckRedirect: checkRedirect, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		count++
		response, _ := reply(302, "")
		response.Header.Set("Location", fmt.Sprintf("https://www.dropbox.com/%d", count))
		return response, nil
	})}
	r, _ := http.NewRequest("GET", "https://www.dropbox.com/start", nil)
	if _, err := client.Do(r); err == nil || count != 6 {
		t.Fatal(count, err)
	}
	first, _ := http.NewRequest("POST", "https://hooks.slack.com/start", strings.NewReader("message"))
	first = allowRedirectHosts(first, []string{"hooks.slack.com"})
	next, _ := http.NewRequestWithContext(first.Context(), "POST", "https://evil.test/x", nil)
	if checkRedirect(next, []*http.Request{first}) == nil {
		t.Fatal("webhook redirect escaped allowlist")
	}
}

func TestManifestURLAuthorityAndQueryFlags(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{"https://dl.dropboxusercontent.com:443/s/a/app.ipa?flag&dl=0", "https://www.dropbox.com:443/s/a/app.ipa?flag&dl=1"},
		{"https://dl.dropboxusercontent.com/s/a/app.ipa?x=1&&dl=0", "https://www.dropbox.com/s/a/app.ipa?x=1&&dl=1"},
	} {
		manifest := manifestValue()
		manifest["items"].([]any)[0].(map[string]any)["assets"].([]any)[0].(map[string]any)["url"] = tc.source
		body, _ := plist.Marshal(manifest, plist.XMLFormat)
		updated, ok := updatedManifest(body)
		if !ok {
			t.Fatal("not rewritten")
		}
		var got ipaManifest
		_, err := plist.Unmarshal(updated, &got)
		if err != nil || *got.Items[0].Assets[0].URL != tc.want {
			t.Fatal(string(updated), err)
		}
	}
}

func TestRepeatedSlashRoutes(t *testing.T) {
	s := testServer(t, nil)
	assertStatus(t, perform(s, "GET", "/api//v1/config", "client", nil), 200, "")
}

func TestProxyRedirectPolicyIndependentOfCustomCORSList(t *testing.T) {
	s := testServer(t, func(r *http.Request) (*http.Response, error) {
		hosts, ok := r.Context().Value(redirectHostsKey{}).([]string)
		if !ok || !allowedHost("dl.dropboxusercontent.com", hosts) {
			t.Error("Dropbox routes lost Dropbox redirect permission")
		}
		return reply(200, "{}")
	})
	s.config.CORSProxyAllowedHosts = []string{"getappbox.com"}
	assertStatus(t, perform(s, "GET", "/appinfo/s/a/appinfo.json", "", nil), 200, "")
}

func TestFetchAlwaysClosesOversizedBody(t *testing.T) {
	body := &closingBody{Reader: strings.NewReader("oversized")}
	s := testServer(t, func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: -1, Body: body}, nil
	})
	r, _ := http.NewRequest("GET", "https://www.dropbox.com/x", nil)
	_, _, err := s.fetch(r, 3)
	if err == nil || !body.closed {
		t.Fatal(err, body.closed)
	}
}

type closingBody struct {
	io.Reader
	closed bool
}

func (b *closingBody) Close() error { b.closed = true; return nil }

func TestCanceledShortenDoesNotRetry(t *testing.T) {
	calls := 0
	ctx, cancel := context.WithCancel(context.Background())
	s := testServer(t, func(r *http.Request) (*http.Response, error) { calls++; cancel(); return nil, context.Canceled })
	body := `{"url":"https://www.dropbox.com/s/a/appinfo.json","name":"A","version":"1","build":"7","identifier":"com.test.a"}`
	r := httptest.NewRequest("POST", "/api/v1/shorten", strings.NewReader(body)).WithContext(ctx)
	r.Header.Set("Content-Type", "application/json")
	if err := s.shorten(httptest.NewRecorder(), r); err == nil || calls != 1 {
		t.Fatal(err, calls)
	}
}
