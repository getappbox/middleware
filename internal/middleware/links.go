package middleware

import (
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

type shortenRequest struct {
	URL        string  `json:"url"`
	Name       string  `json:"name"`
	Version    string  `json:"version"`
	Build      string  `json:"build"`
	Identifier string  `json:"identifier"`
	Provider   *string `json:"provider,omitempty"`
}

func shortlinkTarget(request shortenRequest, base string) (string, error) {
	if request.Provider == nil || *request.Provider == "dropbox" {
		if _, path, ok := strings.Cut(request.URL, "dropbox.com"); ok {
			return base + "?url=" + path, nil
		}
		return "", apiError{400, "invalid_url", "Expected a dropbox.com share URL."}
	}
	if *request.Provider != "s3" {
		return "", apiError{400, "invalid_provider", "Unsupported storage provider."}
	}
	if validInstallLink(request.URL, base) {
		return request.URL, nil
	}
	return "", apiError{400, "invalid_url", "Expected an S3 install-page URL with a public HTTPS appinfo.json source."}
}

func validInstallLink(value, base string) bool {
	candidate, ok := publicHTTPSURL(value)
	if !ok {
		return false
	}
	configured, ok := publicHTTPSURL(base)
	if !ok || configured.RawQuery != "" || configured.ForceQuery || !strings.EqualFold(candidate.Hostname(), configured.Hostname()) || effectivePort(candidate) != effectivePort(configured) {
		return false
	}
	path, basePath := candidate.EscapedPath(), configured.EscapedPath()
	if path != basePath && path != strings.TrimRight(basePath, "/")+"/" {
		return false
	}
	parameters := make(map[string]string)
	for pair := range strings.SplitSeq(candidate.RawQuery, "&") {
		key, raw, ok := strings.Cut(pair, "=")
		if !ok {
			return false
		}
		key, keyErr := url.QueryUnescape(key)
		decoded, valueErr := url.QueryUnescape(raw)
		if _, exists := parameters[key]; exists || keyErr != nil || valueErr != nil {
			return false
		}
		parameters[key] = decoded
	}
	if len(parameters) != 2 || parameters["provider"] != "s3" {
		return false
	}
	metadata, ok := publicHTTPSURL(parameters["url"])
	return ok && strings.HasSuffix(metadata.EscapedPath(), "/appinfo.json")
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		if numeric, err := strconv.Atoi(port); err == nil {
			return strconv.Itoa(numeric)
		}
		return port
	}
	return "443"
}

func publicHTTPSURL(value string) (*url.URL, bool) {
	if len(value) > 8192 || strings.ContainsAny(value, "\\#") || strings.ContainsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return nil, false
	}
	if _, err := url.PathUnescape(value); err != nil {
		return nil, false
	}
	u, err := url.Parse(value)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return nil, false
	}
	for pair := range strings.SplitSeq(u.RawQuery, "&") {
		name, _, _ := strings.Cut(pair, "=")
		name, _ = url.QueryUnescape(name)
		switch strings.ToLower(name) {
		case "x-amz-signature", "x-amz-credential", "x-amz-security-token", "awsaccesskeyid", "signature", "key-pair-id":
			return nil, false
		}
	}
	return u, true
}

func allowedHost(host string, suffixes []string) bool {
	host = strings.ToLower(host)
	for _, suffix := range suffixes {
		suffix = strings.ToLower(suffix)
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}
