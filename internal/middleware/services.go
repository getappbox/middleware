package middleware

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

func sha512Hex(value string) string {
	digest := sha512.Sum512([]byte(value))
	return hex.EncodeToString(digest[:])
}

func (s *Server) shorten(w http.ResponseWriter, r *http.Request) error {
	var body shortenRequest
	if err := decodeBody(w, r, &body, []string{"url", "name", "version", "build", "identifier"}, "Expected JSON with url, name, version, build, identifier."); err != nil {
		return err
	}
	target, err := shortlinkTarget(body, s.config.ShortlinkTargetBase)
	if err != nil {
		return err
	}
	if s.config.YOURLSSignatureSecret == "" {
		return apiError{500, "misconfigured", "Shortener is not configured."}
	}
	timestamp := strconv.FormatInt(s.now().Unix(), 10)
	form := url.Values{
		"action": {"shorturl"}, "format": {"json"}, "url": {target},
		"userAuthToken": {sha512Hex(timestamp + s.authToken)}, "hash": {"sha512"},
		"timestamp": {timestamp}, "signature": {sha512Hex(timestamp + s.config.YOURLSSignatureSecret)},
		"title": {fmt.Sprintf("%s, %s, %s, %s", body.Name, body.Identifier, body.Version, body.Build)},
	}
	lastFailure := "Shortener unavailable."
	for attempt := 1; attempt <= 3; attempt++ {
		if r.Context().Err() != nil {
			return apiError{502, "shortener_unavailable", "Shortener request failed."}
		}
		request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, s.config.YOURLSAPIURL, strings.NewReader(form.Encode()))
		if err != nil {
			return apiError{502, "shortener_unavailable", "Shortener request failed."}
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response, data, err := s.fetch(request, 4*1024*1024)
		if err != nil {
			lastFailure = "Shortener request failed."
			continue
		}
		var result struct {
			ShortURL string `json:"shorturl"`
		}
		_ = json.Unmarshal(data, &result)
		if result.ShortURL != "" {
			payload := map[string]string{"shortURL": result.ShortURL}
			if body.Provider != nil && *body.Provider == "s3" {
				payload["provider"] = "s3"
			}
			writeJSON(w, 200, payload)
			return nil
		}
		if response.StatusCode == 400 {
			return apiError{502, "shortener_unavailable", "The shortener rejected the URL."}
		}
		lastFailure = fmt.Sprintf("Shortener responded %d (attempt %d).", response.StatusCode, attempt)
	}
	return apiError{502, "shortener_unavailable", lastFailure}
}

type mailRequest struct {
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	Build           string   `json:"build"`
	To              []string `json:"to"`
	InstallURL      string   `json:"installURL"`
	PersonalMessage *string  `json:"personalMessage,omitempty"`
}

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func validEmail(address string) bool {
	return !strings.ContainsFunc(address, unicode.IsSpace) && emailPattern.MatchString(address)
}

func (s *Server) mail(w http.ResponseWriter, r *http.Request) error {
	var body mailRequest
	if err := decodeBody(w, r, &body, []string{"name", "version", "build", "to", "installURL"}, "Expected JSON with name, version, build, to, installURL."); err != nil {
		return err
	}
	valid := len(body.To) >= 1 && len(body.To) <= 100
	for _, address := range body.To {
		valid = valid && validEmail(address)
	}
	if !valid {
		return apiError{400, "invalid_recipients", "Provide 1–100 valid email addresses."}
	}
	if s.config.MailgunAPIKey == "" {
		return apiError{500, "misconfigured", "Mail is not configured."}
	}
	form := url.Values{
		"from": {s.config.MailgunFrom}, "to": {strings.Join(body.To, ",")},
		"subject": {emailSubject(body)}, "html": {emailHTML(body)}, "text": {emailText(body)},
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://api.mailgun.net/v3/"+s.config.MailgunDomain+"/messages", strings.NewReader(form.Encode()))
	if err != nil {
		return apiError{502, "mail_provider_error", "Mail provider unreachable."}
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth("api", s.config.MailgunAPIKey)
	response, data, err := s.fetch(request, 4*1024*1024)
	if err != nil {
		return apiError{502, "mail_provider_error", "Mail provider unreachable."}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return apiError{502, "mail_provider_error", fmt.Sprintf("Mail provider responded %d.", response.StatusCode)}
	}
	var result struct {
		ID *string `json:"id"`
	}
	_ = json.Unmarshal(data, &result)
	payload := map[string]string{}
	if result.ID != nil {
		payload["id"] = *result.ID
	}
	writeJSON(w, 200, payload)
	return nil
}

type notifyRequest struct {
	Service    string `json:"service"`
	WebhookURL string `json:"webhookURL"`
	Text       string `json:"text"`
}

func teamsPayload(text string) map[string]any {
	return map[string]any{
		"type": "message",
		"attachments": []any{map[string]any{
			"contentType": "application/vnd.microsoft.card.adaptive",
			"content": map[string]any{
				"$schema": "http://adaptivecards.io/schemas/adaptive-card.json", "type": "AdaptiveCard", "version": "1.4",
				"body": []any{map[string]any{"type": "TextBlock", "text": text, "wrap": true}},
			},
		}},
	}
}

func (s *Server) notify(w http.ResponseWriter, r *http.Request) error {
	var body notifyRequest
	if err := decodeBody(w, r, &body, []string{"service", "webhookURL", "text"}, "Expected JSON with service, webhookURL, text."); err != nil {
		return err
	}
	u, err := url.Parse(body.WebhookURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || !allowedHost(u.Hostname(), s.config.NotifyAllowedHosts) {
		return apiError{403, "webhook_not_allowed", "The webhook URL host is not permitted."}
	}
	var payload any
	switch strings.ToLower(body.Service) {
	case "slack":
		payload = map[string]string{"username": "AppBox", "icon_url": "https://getappbox.com/images/AppBoxIcon.png", "text": body.Text}
	case "teams":
		payload = teamsPayload(body.Text)
	default:
		return apiError{400, "invalid_service", "service must be \"slack\" or \"teams\"."}
	}
	data, _ := json.Marshal(payload)
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, body.WebhookURL, bytes.NewReader(data))
	if err != nil {
		return apiError{502, "notify_failed", "Webhook request failed."}
	}
	request = allowRedirectHosts(request, s.config.NotifyAllowedHosts)
	request.Header.Set("Content-Type", "application/json")
	response, _, err := s.fetch(request, 4*1024*1024)
	if err != nil {
		return apiError{502, "notify_failed", "Webhook request failed."}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return apiError{502, "notify_failed", fmt.Sprintf("Webhook responded %d.", response.StatusCode)}
	}
	w.WriteHeader(200)
	return nil
}

type latestVersion struct {
	Version         string  `json:"version"`
	DownloadURL     *string `json:"downloadURL,omitempty"`
	HomebrewVersion *string `json:"homebrewVersion,omitempty"`
}

type latestFlight struct {
	done    chan struct{}
	result  *latestVersion
	err     error
	cancel  context.CancelFunc
	waiters int
}

func (s *Server) latestResponse(w http.ResponseWriter, r *http.Request) error {
	if err := r.Context().Err(); err != nil {
		return err
	}
	s.updateMu.Lock()
	if s.update != nil && s.updateExpires.After(s.now()) {
		result := s.update
		s.updateMu.Unlock()
		writeJSON(w, 200, result)
		return nil
	}
	flight := s.updateFlight
	if flight == nil {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 40*time.Second)
		flight = &latestFlight{done: make(chan struct{}), cancel: cancel}
		s.updateFlight = flight
		go s.refreshLatest(ctx, flight)
	}
	flight.waiters++
	s.updateMu.Unlock()
	select {
	case <-r.Context().Done():
		s.updateMu.Lock()
		flight.waiters--
		if flight.waiters == 0 && s.updateFlight == flight {
			s.updateFlight = nil
			flight.cancel()
		}
		s.updateMu.Unlock()
		return r.Context().Err()
	case <-flight.done:
		if err := r.Context().Err(); err != nil {
			return err
		}
		if flight.err != nil {
			return flight.err
		}
		writeJSON(w, 200, flight.result)
		return nil
	}
}

func (s *Server) refreshLatest(ctx context.Context, flight *latestFlight) {
	defer flight.cancel()
	result, err := s.fetchLatest(ctx)
	if ctx.Err() != nil {
		result, err = nil, apiError{502, "update_check_unavailable", "Could not reach the release feed."}
	}
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	flight.result, flight.err = result, err
	if s.updateFlight == flight {
		s.updateFlight = nil
		if err == nil {
			s.update = result
			s.updateExpires = s.now().Add(s.config.UpdateCacheTTL)
		}
	}
	close(flight.done)
}

func (s *Server) fetchLatest(ctx context.Context) (*latestVersion, error) {
	type release struct {
		Tag *string `json:"tag_name"`
		URL *string `json:"html_url"`
	}
	github := make(chan *release, 1)
	brew := make(chan *string, 1)
	get := func(endpoint string, target any) bool {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return false
		}
		request.Header.Set("User-Agent", "AppBox-middleware")
		response, body, err := s.fetch(request, 4*1024*1024)
		return err == nil && response.StatusCode == 200 && json.Unmarshal(body, target) == nil
	}
	go func() {
		var result release
		if get(s.config.GitHubLatestReleaseURL, &result) && result.Tag != nil && result.URL != nil {
			github <- &result
		} else {
			github <- nil
		}
	}()
	go func() {
		var result struct {
			Version *string `json:"version"`
		}
		if get(s.config.HomebrewCaskURL, &result) {
			brew <- result.Version
		} else {
			brew <- nil
		}
	}()
	result, homebrew := <-github, <-brew
	if result == nil {
		return nil, apiError{502, "update_check_unavailable", "Could not reach the release feed."}
	}
	return &latestVersion{*result.Tag, result.URL, homebrew}, nil
}
