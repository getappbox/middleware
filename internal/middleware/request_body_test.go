package middleware

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestLegacyContentTypes(t *testing.T) {
	for _, tc := range []struct{ contentType, body string }{
		{"application/vnd.api+json", `{"url":"https://www.dropbox.com/s/a/appinfo.json","name":"Demo","version":"1","build":"7","identifier":"com.test.demo"}`},
		{"application/x-www-form-urlencoded", "url=https%3A%2F%2Fwww.dropbox.com%2Fs%2Fa%2Fappinfo.json&name=Demo&version=1&build=7&identifier=com.test.demo"},
	} {
		r := httptest.NewRequest("POST", "/api/v1/shorten", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", tc.contentType)
		r.Header.Set("X-AppBox-Client-Token", "test-client-token")
		r.Header.Set("Authorization", "Bearer fake")
		w := httptest.NewRecorder()
		testServer(t, nil).ServeHTTP(w, r)
		assertStatus(t, w, 200, "")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range map[string]string{"url": "https://www.dropbox.com/s/a/appinfo.json", "name": "Demo", "version": "1", "build": "7", "identifier": "com.test.demo"} {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/v1/shorten", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("X-AppBox-Client-Token", "test-client-token")
	r.Header.Set("Authorization", "Bearer fake")
	w := httptest.NewRecorder()
	testServer(t, nil).ServeHTTP(w, r)
	assertStatus(t, w, 200, "")
}

func TestFormRecipientRepresentations(t *testing.T) {
	for _, encoded := range []string{"to=a%40b.com&to=c%40d.io", "to[]=a%40b.com&to[]=c%40d.io", "to[1]=c%40d.io&to[0]=a%40b.com", "to=a%40b.com,c%40d.io", "to=a%40b.com|c%40d.io"} {
		values := make(map[string][]string)
		for pair := range strings.SplitSeq(encoded, "&") {
			key, value, _ := strings.Cut(pair, "=")
			values[key] = append(values[key], value)
		}
		got, err := formRecipients(values)
		if err != nil || !reflect.DeepEqual(got, []string{"a@b.com", "c@d.io"}) {
			t.Fatal(encoded, got, err)
		}
	}
	if got, err := formRecipients(map[string][]string{"to": {"a%40b.com%2Cc%40d.io"}}); err != nil || len(got) != 1 || got[0] != "a@b.com,c@d.io" {
		t.Fatal(got, err)
	}
	for _, values := range []map[string][]string{{"to[1]": {"a"}}, {"to[x]": {"a"}}, {"to": {"%XX"}}} {
		if _, err := formRecipients(values); err == nil {
			t.Fatal(values)
		}
	}
	form := url.Values{"name": {"Demo"}, "version": {"1"}, "build": {"7"}, "installURL": {"https://appbox.me/x"}}
	r := httptest.NewRequest("POST", "/api/v1/mail/send", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("X-AppBox-Client-Token", "test-client-token")
	r.Header.Set("Authorization", "Bearer fake")
	w := httptest.NewRecorder()
	testServer(t, nil).ServeHTTP(w, r)
	assertStatus(t, w, 400, "invalid_recipients")
}

func TestNullRecipientIsInvalidBody(t *testing.T) {
	assertStatus(t, perform(testServer(t, nil), "POST", "/api/v1/mail/send", "full", `{"name":"Demo","version":"1","build":"7","to":[null],"installURL":"https://appbox.me/x"}`), 400, "invalid_body")
}
