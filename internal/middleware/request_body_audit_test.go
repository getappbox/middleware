package middleware

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func auditedShortenRequest(t *testing.T, contentType, encoding string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest("POST", "/api/v1/shorten", bytes.NewReader(body))
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Content-Encoding", encoding)
	request.Header.Set("X-AppBox-Client-Token", "test-client-token")
	request.Header.Set("Authorization", "Bearer fake")
	response := httptest.NewRecorder()
	testServer(t, nil).ServeHTTP(response, request)
	return response
}

func compressRequest(t *testing.T, body []byte, algorithm string) []byte {
	t.Helper()
	var compressed bytes.Buffer
	var writer io.WriteCloser
	if algorithm == "gzip" {
		writer, _ = gzip.NewWriterLevel(&compressed, gzip.BestCompression)
	} else {
		writer, _ = zlib.NewWriterLevel(&compressed, zlib.BestCompression)
	}
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return compressed.Bytes()
}

func TestFirstDuplicateJSONMemberMatchesSwift(t *testing.T) {
	body := []byte(`{"url":"https://www.dropbox.com/s/a/appinfo.json","name":"first","name":"second","version":"1","build":"7","identifier":"com.test.demo","provider":null,"provider":"s3"}`)
	var decoded shortenRequest
	request := httptest.NewRequest("POST", "/api/v1/shorten", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if err := decodeBody(httptest.NewRecorder(), request, &decoded, []string{"url", "name", "version", "build", "identifier"}, "invalid"); err != nil {
		t.Fatal(err)
	}
	if decoded.Name != "first" || decoded.Provider != nil {
		t.Fatalf("duplicate fields chose %+v", decoded)
	}
	fields, err := firstJSONMembers([]byte(`{"na\u006de":"first","name":"last","provider":"s3","provider":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(fields["name"]) != `"first"` || string(fields["provider"]) != `"s3"` {
		t.Fatal(fields)
	}
}

func TestRejectInvalidUnicodeWithoutReplacingFields(t *testing.T) {
	prefix := []byte(`{"url":"https://www.dropbox.com/s/a/appinfo.json","name":"`)
	suffix := []byte(`","version":"1","build":"7","identifier":"com.test.demo"}`)
	for _, invalid := range [][]byte{{0xff}, {0xed, 0xa0, 0x80}, []byte(`\uD800`), []byte(`\uDC00`), []byte(`\uD800\u0041`)} {
		body := append(append(append([]byte{}, prefix...), invalid...), suffix...)
		assertStatus(t, auditedShortenRequest(t, "application/json", "", body), 400, "invalid_body")
	}
	for _, valid := range []string{`\uD83D\uDE00`, `\\uD800`, `Demo 😀`, `\"quoted\"`} {
		body := append(append(append([]byte{}, prefix...), []byte(valid)...), suffix...)
		assertStatus(t, auditedShortenRequest(t, "application/json", "", body), 200, "")
	}
	for _, field := range []string{"name=%FF", "name=%ED%A0%80", "provider=%FF"} {
		body := []byte("url=https%3A%2F%2Fwww.dropbox.com%2Fs%2Fa%2Fappinfo.json&name=Demo&version=1&build=7&identifier=com.test.demo&" + field)
		assertStatus(t, auditedShortenRequest(t, "application/x-www-form-urlencoded", "", body), 400, "invalid_body")
	}
	if _, err := formRecipients(map[string][]string{"to": {"a%FF%40example.com"}}); err == nil {
		t.Fatal("invalid recipient bytes were replaced")
	}
}

func TestNestedScalarFormsAreRejected(t *testing.T) {
	for _, extra := range []string{"provider[x]=s3", "name[x]=nested", "version[]=2", "build[0]=7", "identifier[x]=com.evil"} {
		body := []byte("url=https%3A%2F%2Fwww.dropbox.com%2Fs%2Fa%2Fappinfo.json&name=Demo&version=1&build=7&identifier=com.test.demo&" + extra)
		assertStatus(t, auditedShortenRequest(t, "application/x-www-form-urlencoded", "", body), 400, "invalid_body")
	}
	for _, contentType := range []string{"application/x-www-form-urlencoded", "multipart/form-data"} {
		values := url.Values{"name": {"Demo"}, "version": {"1"}, "build": {"7"}, "to[]": {"qa@example.com"}, "installURL": {"https://appbox.me/x"}, "personalMessage[]": {"hello"}}
		var body []byte
		if contentType == "multipart/form-data" {
			var buffer bytes.Buffer
			writer := multipart.NewWriter(&buffer)
			for name, entries := range values {
				for _, value := range entries {
					if err := writer.WriteField(name, value); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			body = buffer.Bytes()
			contentType = writer.FormDataContentType()
		} else {
			body = []byte(values.Encode())
		}
		request := httptest.NewRequest("POST", "/api/v1/mail/send", bytes.NewReader(body))
		request.Header.Set("Content-Type", contentType)
		request.Header.Set("X-AppBox-Client-Token", "test-client-token")
		request.Header.Set("Authorization", "Bearer fake")
		response := httptest.NewRecorder()
		testServer(t, nil).ServeHTTP(response, request)
		assertStatus(t, response, 400, "invalid_body")
	}
}

func TestCompressedRequestBodies(t *testing.T) {
	jsonBody, _ := json.Marshal(shortenBody())
	formBody := []byte("url=https%3A%2F%2Fwww.dropbox.com%2Fs%2Fa%2Fappinfo.json&name=Demo&version=1&build=7&identifier=com.test.demo")
	for _, algorithm := range []string{"gzip", "deflate"} {
		for _, body := range []struct {
			contentType string
			data        []byte
		}{{"application/json", jsonBody}, {"application/vnd.api+json", jsonBody}, {"application/x-www-form-urlencoded", formBody}} {
			t.Run(algorithm+"/"+body.contentType, func(t *testing.T) {
				compressed := compressRequest(t, body.data, algorithm)
				assertStatus(t, auditedShortenRequest(t, body.contentType, algorithm, compressed), 200, "")
			})
		}
	}
	assertStatus(t, auditedShortenRequest(t, "application/json", "deflate", compressRequest(t, jsonBody, "gzip")), 200, "")
	assertStatus(t, auditedShortenRequest(t, "application/json", "GZIP", compressRequest(t, jsonBody, "gzip")), 200, "")
}

func TestCompressedBodyBoundsAndIntegrity(t *testing.T) {
	jsonBody, _ := json.Marshal(shortenBody())
	for _, algorithm := range []string{"gzip", "deflate"} {
		valid := compressRequest(t, jsonBody, algorithm)
		for _, broken := range [][]byte{[]byte("garbage"), valid[:len(valid)-1], append(append([]byte{}, valid...), 1), append(append([]byte{}, valid...), valid...)} {
			assertStatus(t, auditedShortenRequest(t, "application/json", algorithm, broken), 400, "invalid_body")
		}
		repetitive := compressRequest(t, []byte(strings.Repeat("a", 8000)), algorithm)
		if len(repetitive)*25 >= 8000 {
			t.Fatal("ratio fixture is insufficiently compressed")
		}
		assertStatus(t, auditedShortenRequest(t, "application/json", algorithm, repetitive), 413, "")
		random := rand.New(rand.NewSource(42))
		noise := make([]byte, 13000)
		_, _ = random.Read(noise)
		oversized := []byte(base64.StdEncoding.EncodeToString(noise))
		compressed := compressRequest(t, oversized, algorithm)
		if len(compressed) > requestBodyLimit || len(oversized) <= requestBodyLimit {
			t.Fatalf("decoded-limit fixture size is invalid: %s encoded=%d decoded=%d", algorithm, len(compressed), len(oversized))
		}
		assertStatus(t, auditedShortenRequest(t, "application/json", algorithm, compressed), 413, "")
		assertStatus(t, auditedShortenRequest(t, "application/json", algorithm, bytes.Repeat([]byte{'x'}, requestBodyLimit+1)), 413, "")
	}
}
