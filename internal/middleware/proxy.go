package middleware

import (
	"bytes"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"howett.net/plist"
)

type queryItem struct {
	name, value string
	bare        bool
}

func legacyQuery(raw string, ignoreBranch bool) []queryItem {
	var result []queryItem
	for pair := range strings.SplitSeq(raw, "&") {
		var parts []string
		for part := range strings.SplitSeq(pair, "=") {
			if part != "" {
				parts = append(parts, part)
			}
		}
		if len(parts) != 2 {
			continue
		}
		if ignoreBranch && (parts[0] == "$web_only" || parts[0] == "_branch_match_id" || parts[0] == "_branch_referrer") {
			continue
		}
		result = append(result, queryItem{parts[0], parts[1], false})
	}
	return result
}

func directDownload(items []queryItem) []queryItem {
	for i, item := range items {
		if item.name == "dl" {
			items[i].value = "1"
			items[i].bare = false
			return items
		}
	}
	return append(items, queryItem{"dl", "1", false})
}

func queryEscape(value string) string {
	const allowed = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~!$'()*+,;:@/?"
	var result strings.Builder
	for i := 0; i < len(value); i++ {
		if strings.IndexByte(allowed, value[i]) >= 0 {
			result.WriteByte(value[i])
		} else {
			fmt.Fprintf(&result, "%%%02X", value[i])
		}
	}
	return result.String()
}

func encodeQuery(items []queryItem) string {
	pairs := make([]string, len(items))
	for i, item := range items {
		pairs[i] = queryEscape(item.name)
		if !item.bare {
			pairs[i] += "=" + queryEscape(item.value)
		}
	}
	return strings.Join(pairs, "&")
}

func validateDropboxPath(path string) error {
	if utf8.RuneCountInString(path) > 1024 {
		return legacyError{400, "Path too long."}
	}
	if !strings.HasPrefix(path, "/s/") && !strings.HasPrefix(path, "/scl/") {
		return legacyError{400, "Unsupported Dropbox path."}
	}
	for component := range strings.SplitSeq(path, "/") {
		if component == ".." {
			return legacyError{400, "Invalid path."}
		}
	}
	return nil
}

func legacyDropboxURL(r *http.Request, route string) (string, error) {
	path := r.URL.EscapedPath()
	var items []queryItem
	if route == "install" || strings.HasPrefix(route, "install/") {
		items = []queryItem{{"dl", "1", false}}
		path = ""
		for component := range strings.SplitSeq(r.URL.EscapedPath(), "/") {
			switch {
			case component == "" || component == "install":
			case strings.HasPrefix(component, "queryparam-"):
				parts := strings.Split(strings.ReplaceAll(component, "queryparam-", ""), "-value-")
				if len(parts) == 2 {
					items = append(items, queryItem{parts[0], parts[1], false})
				}
			default:
				path += "/" + component
			}
		}
	} else {
		ignoreBranch := strings.HasPrefix(route, "appinfo/s/") || route == "appinfo/s"
		items = legacyQuery(r.URL.RawQuery, ignoreBranch)
		if ignoreBranch && strings.ContainsAny(path, "?&") && len(items) == 0 {
			for _, part := range strings.Split(path, "&") {
				path = strings.ReplaceAll(path, "&"+part, "")
				items = append(items, legacyQuery(part, true)...)
			}
			path = strings.NewReplacer("?", "", "&", "").Replace(path)
		}
		items = directDownload(items)
		path = strings.ReplaceAll(path, "/appinfo/", "/")
	}
	if err := validateDropboxPath(path); err != nil {
		return "", err
	}
	u := url.URL{Scheme: "https", Host: "www.dropbox.com", Path: path, RawQuery: encodeQuery(items)}
	return u.String(), nil
}

func (s *Server) legacy(w http.ResponseWriter, r *http.Request, route string) error {
	var upstream string
	if route == "cors" {
		values, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || !values.Has("url") {
			return legacyError{400, "No value found (expected type 'String') at path 'url'. No String was found at 'url'."}
		}
		urls := values["url"]
		upstream = urls[len(urls)-1]
		u, err := url.Parse(upstream)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || !allowedHost(u.Hostname(), s.config.CORSProxyAllowedHosts) {
			return legacyError{403, "URL not allowed."}
		}
	} else {
		var err error
		upstream, err = legacyDropboxURL(r, route)
		if err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstream, nil)
	if err != nil {
		return legacyError{400, "Invalid upstream URL."}
	}
	hosts := s.config.CORSProxyAllowedHosts
	if route != "cors" {
		hosts = []string{"dropbox.com", "dropboxusercontent.com"}
	}
	request = allowRedirectHosts(request, hosts)
	response, body, err := s.fetch(request, s.config.ProxyMaxBodyBytes)
	if err != nil {
		return err
	}
	headers := proxyHeaders(response.Header, len(body))
	if route != "cors" {
		subject := "App info"
		install := route == "install" || strings.HasPrefix(route, "install/")
		if install {
			subject = "Install manifest"
		}
		isHTML, err := validateDropboxResponse(headers, body, subject)
		if err != nil {
			return err
		}
		if install && isHTML {
			status := response.StatusCode
			if status < 400 {
				status = 502
			}
			return legacyError{status, "Dropbox did not return an install manifest for this link."}
		}
		if install && response.StatusCode == 200 {
			if updated, ok := updatedManifest(body); ok {
				body = updated
				headers.Del("Content-Encoding")
				headers.Set("Content-Length", strconv.Itoa(len(body)))
				headers.Set("Content-Type", "application/xml")
			}
		}
	}
	for name, values := range headers {
		w.Header()[name] = values
	}
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(body)
	return nil
}

func proxyHeaders(upstream http.Header, count int) http.Header {
	result := make(http.Header)
	for name, values := range upstream {
		switch strings.ToLower(name) {
		case "content-type", "content-encoding", "content-disposition", "cache-control":
			for _, value := range values {
				result.Add(name, value)
			}
		}
	}
	result.Set("Content-Length", strconv.Itoa(count))
	return result
}

func validateDropboxResponse(headers http.Header, body []byte, subject string) (bool, error) {
	mediaType, _, _ := mime.ParseMediaType(headers.Get("Content-Type"))
	if mediaType != "text/html" || len(body) == 0 || !utf8.Valid(body) {
		return false, nil
	}
	root, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return true, nil
	}
	var findTitle func(*html.Node) string
	findTitle = func(n *html.Node) string {
		if n.Type == html.ElementNode && n.Data == "title" {
			var text strings.Builder
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				text.WriteString(child.Data)
			}
			return strings.ToLower(strings.Join(strings.Fields(text.String()), " "))
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if result := findTitle(child); result != "" {
				return result
			}
		}
		return ""
	}
	title := findTitle(root)
	switch {
	case strings.Contains(title, "deleted"):
		return true, legacyError{404, subject + " not found on Dropbox."}
	case strings.Contains(title, "invalid link"):
		return true, legacyError{404, subject + " link is not valid on Dropbox."}
	case strings.Contains(title, "link temporarily disabled"):
		return true, legacyError{423, "Share link is temporarily disabled by Dropbox."}
	}
	return true, nil
}

type ipaManifest struct {
	Items []ipaItem `plist:"items"`
}

type ipaItem struct {
	Assets   []ipaAsset   `plist:"assets"`
	Metadata *ipaMetadata `plist:"metadata,omitempty"`
}

type ipaAsset struct {
	Kind *string `plist:"kind"`
	URL  *string `plist:"url"`
}

type ipaMetadata struct {
	BundleIdentifier *string `plist:"bundle-identifier,omitempty"`
	BundleVersion    *string `plist:"bundle-version,omitempty"`
	Kind             *string `plist:"kind,omitempty"`
	Title            *string `plist:"title,omitempty"`
}

func updatedManifest(body []byte) ([]byte, bool) {
	var manifest ipaManifest
	if !decodeManifest(body, &manifest) {
		return nil, false
	}
	if len(manifest.Items) == 0 || len(manifest.Items[0].Assets) == 0 {
		return nil, false
	}
	for _, item := range manifest.Items {
		if item.Assets == nil {
			return nil, false
		}
		for _, asset := range item.Assets {
			if asset.Kind == nil || asset.URL == nil {
				return nil, false
			}
		}
	}
	u, err := url.Parse(*manifest.Items[0].Assets[0].URL)
	if err != nil {
		return nil, false
	}
	if u.Hostname() == "dl.dropboxusercontent.com" {
		port := u.Port()
		u.Host = "www.dropbox.com"
		if port != "" {
			u.Host += ":" + port
		}
	}
	var query []queryItem
	for _, pair := range manifestQueryParts(u.RawQuery) {
		name, value, hasValue := strings.Cut(pair, "=")
		name, err = url.PathUnescape(name)
		if err != nil {
			return nil, false
		}
		value, err = url.PathUnescape(value)
		if err != nil {
			return nil, false
		}
		query = append(query, queryItem{name, value, !hasValue})
	}
	u.RawQuery = encodeQuery(directDownload(query))
	updatedURL := u.String()
	manifest.Items[0].Assets[0].URL = &updatedURL
	result, err := plist.MarshalIndent(manifest, plist.XMLFormat, "\t")
	return result, err == nil
}

func manifestQueryParts(raw string) []string {
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "&")
}
