package middleware

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port                   string
	ClientToken            string
	DropboxAppKey          string
	DropboxTokenCacheTTL   time.Duration
	MailgunAPIKey          string
	MailgunDomain          string
	MailgunFrom            string
	YOURLSAPIURL           string
	YOURLSSignatureSecret  string
	ShortlinkTargetBase    string
	CORSProxyAllowedHosts  []string
	ProxyMaxBodyBytes      int64
	GitHubLatestReleaseURL string
	HomebrewCaskURL        string
	UpdateCacheTTL         time.Duration
	NotifyAllowedHosts     []string
	TrustedProxyCount      int
	TrustedProxyCIDRs      []string
}

func ConfigFromEnvironment() Config {
	get := func(key, fallback string) string {
		if value, ok := os.LookupEnv(key); ok {
			return value
		}
		return fallback
	}
	integer := func(key string, fallback int) int {
		if value, err := strconv.Atoi(os.Getenv(key)); err == nil {
			return value
		}
		return fallback
	}
	hosts := func(key, fallback string) []string {
		var result []string
		for host := range strings.SplitSeq(get(key, fallback), ",") {
			if host = strings.TrimSpace(host); host != "" {
				result = append(result, host)
			}
		}
		return result
	}
	var trustedProxyCIDRs []string
	if raw := os.Getenv("TRUSTED_PROXY_CIDRS"); raw != "" {
		for entry := range strings.SplitSeq(raw, ",") {
			trustedProxyCIDRs = append(trustedProxyCIDRs, strings.TrimSpace(entry))
		}
	}
	return Config{
		Port:                   strconv.Itoa(integer("PORT", 8080)),
		ClientToken:            os.Getenv("APPBOX_CLIENT_TOKEN"),
		DropboxAppKey:          os.Getenv("DROPBOX_APP_KEY"),
		DropboxTokenCacheTTL:   time.Duration(integer("DROPBOX_TOKEN_CACHE_TTL_SECONDS", 600)) * time.Second,
		MailgunAPIKey:          os.Getenv("MAILGUN_API_KEY"),
		MailgunDomain:          get("MAILGUN_DOMAIN", "mail.getappbox.com"),
		MailgunFrom:            get("MAILGUN_FROM", "AppBox Build <build@getappbox.com>"),
		YOURLSAPIURL:           get("YOURLS_API_URL", "https://appbox.me/yourls-api.php"),
		YOURLSSignatureSecret:  os.Getenv("YOURLS_SIGNATURE_SECRET"),
		ShortlinkTargetBase:    get("SHORTLINK_TARGET_BASE", "https://web.getappbox.com"),
		CORSProxyAllowedHosts:  hosts("CORS_PROXY_ALLOWED_HOSTS", "dropbox.com,dropboxusercontent.com,getappbox.com"),
		ProxyMaxBodyBytes:      int64(integer("PROXY_MAX_BODY_BYTES", 4*1024*1024)),
		GitHubLatestReleaseURL: get("GITHUB_LATEST_RELEASE_URL", "https://api.github.com/repos/getappbox/AppBox-iOSAppsWirelessInstallation/releases/latest"),
		HomebrewCaskURL:        get("HOMEBREW_CASK_URL", "https://formulae.brew.sh/api/cask/appbox.json"),
		UpdateCacheTTL:         time.Duration(integer("UPDATE_CACHE_TTL_SECONDS", 3600)) * time.Second,
		NotifyAllowedHosts:     hosts("NOTIFY_ALLOWED_HOSTS", "hooks.slack.com,office.com,office365.com,logic.azure.com,powerplatform.com"),
		TrustedProxyCount:      max(0, integer("TRUSTED_PROXY_COUNT", 1)),
		TrustedProxyCIDRs:      trustedProxyCIDRs,
	}
}

func (c Config) MissingAPIConfiguration() []string {
	var missing []string
	for _, entry := range []struct{ name, value string }{
		{"APPBOX_CLIENT_TOKEN", c.ClientToken}, {"DROPBOX_APP_KEY", c.DropboxAppKey},
		{"MAILGUN_API_KEY", c.MailgunAPIKey}, {"YOURLS_SIGNATURE_SECRET", c.YOURLSSignatureSecret},
	} {
		if entry.value == "" {
			missing = append(missing, entry.name)
		}
	}
	return missing
}

func LoadDotEnv(path string) error {
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimLeft(line, " ")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			continue
		}
		if len(value) >= 2 {
			switch {
			case value[0] == '"' && value[len(value)-1] == '"':
				value = strings.ReplaceAll(value[1:len(value)-1], `\n`, "\n")
			case value[0] == '\'' && value[len(value)-1] == '\'':
				value = value[1 : len(value)-1]
			}
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	if filepath.Base(path) == ".env" {
		for _, line := range strings.Split(string(contents), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if index := strings.IndexByte(line, '#'); index > 0 && (line[index-1] == ' ' || line[index-1] == '\t') {
				line = strings.TrimSpace(line[:index])
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
				value = value[1 : len(value)-1]
			}
			if _, exists := os.LookupEnv(key); !exists && key != "" {
				_ = os.Setenv(key, value)
			}
		}
	}

	return nil
}
