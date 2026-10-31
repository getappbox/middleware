package middleware

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Server struct {
	config        Config
	client        HTTPClient
	now           func() time.Time
	authToken     string
	authMu        sync.Mutex
	authFlights   map[[32]byte]*verificationFlight
	tokens        *tokenStore
	rates         rateStore
	updateMu      sync.Mutex
	update        *latestVersion
	updateExpires time.Time
	updateFlight  *latestFlight
}

func New(config Config, client HTTPClient) *Server {
	if client == nil {
		transport := &http.Transport{
			Proxy:        http.ProxyFromEnvironment,
			DialContext:  (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			MaxIdleConns: 64, MaxIdleConnsPerHost: 4, MaxConnsPerHost: 8,
			IdleConnTimeout: 60 * time.Second, TLSHandshakeTimeout: 10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second, DisableCompression: true,
		}
		client = &http.Client{Transport: transport, Timeout: 40 * time.Second, CheckRedirect: checkRedirect}
	}
	return &Server{config: config, client: client, now: time.Now, authToken: rand.Text(), tokens: newTokenStore(), authFlights: make(map[[32]byte]*verificationFlight)}
}

type apiError struct {
	status  int
	code    string
	message string
}

func (e apiError) Error() string { return e.message }

type legacyError struct {
	status int
	reason string
}

func (e legacyError) Error() string { return e.reason }

func writeJSON(w http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		writeError(w, legacyError{500, "Internal Server Error"})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func writeError(w http.ResponseWriter, err error) {
	var api apiError
	var legacy legacyError
	switch {
	case errors.As(err, &api):
		writeJSON(w, api.status, map[string]any{"error": map[string]string{"code": api.code, "message": api.message}})
	case errors.As(err, &legacy):
		writeJSON(w, legacy.status, map[string]any{"error": true, "reason": legacy.reason})
	default:
		writeJSON(w, 500, map[string]any{"error": true, "reason": "Internal Server Error"})
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodHead {
		w = headWriter{w}
	}
	if r.Header.Get("Origin") != "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "accept, authorization, content-type, origin, x-requested-with, user-agent, access-control-allow-origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS, DELETE, PATCH")
		w.Header().Set("Access-Control-Max-Age", "600")
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.WriteHeader(http.StatusOK)
			return
		}
	}
	path := strings.Join(strings.FieldsFunc(r.URL.EscapedPath(), func(r rune) bool { return r == '/' }), "/")
	isGet := r.Method == http.MethodGet || r.Method == http.MethodHead
	if isGet && path == "" {
		const greeting = "AppBox Install Service Helper"
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(greeting)))
		_, _ = io.WriteString(w, greeting)
		return
	}
	if isGet && (path == "cors" || path == "install" || strings.HasPrefix(path, "install/") || path == "appinfo/s" || path == "appinfo/scl" || strings.HasPrefix(path, "appinfo/s/") || strings.HasPrefix(path, "appinfo/scl/")) {
		if s.rateLimit(w, r, "legacy", 300) {
			return
		}
		if err := s.legacy(w, r, path); err != nil {
			writeError(w, err)
		}
		return
	}
	var handler func(http.ResponseWriter, *http.Request) error
	bucket, limit, dropbox := "", 0, false
	switch {
	case isGet && path == "api/v1/config":
		handler, bucket, limit = s.configResponse, "config", 120
	case isGet && path == "api/v1/latest-version":
		handler, bucket, limit = s.latestResponse, "latest-version", 120
	case r.Method == http.MethodPost && path == "api/v1/shorten":
		handler, bucket, limit, dropbox = s.shorten, "shorten", 60, true
	case r.Method == http.MethodPost && path == "api/v1/mail/send":
		handler, bucket, limit, dropbox = s.mail, "mail", 20, true
	case r.Method == http.MethodPost && path == "api/v1/notify":
		handler, bucket, limit, dropbox = s.notify, "notify", 60, true
	}
	if handler == nil {
		writeError(w, legacyError{404, "Not Found"})
		return
	}
	if s.config.ClientToken == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-AppBox-Client-Token")), []byte(s.config.ClientToken)) != 1 {
		writeError(w, apiError{401, "invalid_client_token", "Missing or invalid client token."})
		return
	}
	if dropbox {
		if err := s.authenticate(r); err != nil {
			writeError(w, err)
			return
		}
	}
	if s.rateLimit(w, r, bucket, limit) {
		return
	}
	if err := handler(w, r); err != nil {
		writeError(w, err)
	}
}

type headWriter struct{ http.ResponseWriter }

func (w headWriter) Write(p []byte) (int, error) { return len(p), nil }

func (s *Server) rateLimit(w http.ResponseWriter, r *http.Request, bucket string, limit int) bool {
	if retry := s.rates.register(bucket+":"+clientIP(r, s.config.TrustedProxyCount, s.config.TrustedProxyCIDRs...), limit, s.now()); retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(retry))
		writeError(w, apiError{429, "rate_limited", "Too many requests. Try again later."})
		return true
	}
	return false
}

func (s *Server) authenticate(r *http.Request) error {
	parts := strings.FieldsFunc(r.Header.Get("Authorization"), func(r rune) bool { return r == ' ' })
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return apiError{401, "missing_dropbox_token", "A Dropbox access token is required."}
	}
	token := parts[1]
	digest := sha256.Sum256([]byte(token))
	verdict, err := s.verifiedToken(r.Context(), token, digest)
	if err != nil {
		return err
	}
	if !verdict.verified {
		return apiError{401, "invalid_dropbox_token", "The Dropbox token was rejected."}
	}
	return nil
}

func (s *Server) fetch(request *http.Request, maxBytes int64) (*http.Response, []byte, error) {
	response, err := s.client.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	maxBytes = max(0, maxBytes)
	if response.ContentLength > maxBytes {
		return nil, nil, legacyError{502, fmt.Sprintf("Upstream response exceeds the %d byte proxy limit.", maxBytes)}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, nil, legacyError{502, fmt.Sprintf("Upstream response exceeds the %d byte proxy limit.", maxBytes)}
	}
	return response, body, nil
}

func (s *Server) configResponse(w http.ResponseWriter, r *http.Request) error {
	if s.config.DropboxAppKey == "" {
		return legacyError{500, "DROPBOX_APP_KEY is not configured."}
	}
	writeJSON(w, 200, map[string]string{"dropboxAppKey": s.config.DropboxAppKey})
	return nil
}
