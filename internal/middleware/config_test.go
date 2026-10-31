package middleware

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestEnvironmentConfiguration(t *testing.T) {
	for _, key := range []string{"PORT", "APPBOX_CLIENT_TOKEN", "DROPBOX_APP_KEY", "MAILGUN_API_KEY", "YOURLS_SIGNATURE_SECRET", "MAILGUN_DOMAIN", "MAILGUN_FROM", "YOURLS_API_URL", "SHORTLINK_TARGET_BASE", "CORS_PROXY_ALLOWED_HOSTS", "GITHUB_LATEST_RELEASE_URL", "HOMEBREW_CASK_URL", "NOTIFY_ALLOWED_HOSTS", "PROXY_MAX_BODY_BYTES", "UPDATE_CACHE_TTL_SECONDS", "DROPBOX_TOKEN_CACHE_TTL_SECONDS", "TRUSTED_PROXY_COUNT", "TRUSTED_PROXY_CIDRS"} {
		value, existed := os.LookupEnv(key)
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(key, value)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
	c := ConfigFromEnvironment()
	if c.Port != "8080" || c.ProxyMaxBodyBytes != 4194304 || c.DropboxTokenCacheTTL != 600*time.Second || c.UpdateCacheTTL != time.Hour || c.TrustedProxyCount != 1 || len(c.TrustedProxyCIDRs) != 0 || len(c.MissingAPIConfiguration()) != 4 {
		t.Fatal(c)
	}
	if c.MailgunDomain != "mail.getappbox.com" || c.ShortlinkTargetBase != "https://web.getappbox.com" || c.YOURLSAPIURL != "https://appbox.me/yourls-api.php" {
		t.Fatal(c)
	}
	t.Setenv("DROPBOX_TOKEN_CACHE_TTL_SECONDS", "bad")
	t.Setenv("PROXY_MAX_BODY_BYTES", "2048")
	t.Setenv("TRUSTED_PROXY_COUNT", "-1")
	t.Setenv("CORS_PROXY_ALLOWED_HOSTS", " a.test, ,b.test ")
	t.Setenv("PORT", "2550")
	c = ConfigFromEnvironment()
	if c.Port != "2550" || c.ProxyMaxBodyBytes != 2048 || c.TrustedProxyCount != 0 || c.DropboxTokenCacheTTL != 10*time.Minute || !reflect.DeepEqual(c.CORSProxyAllowedHosts, []string{"a.test", "b.test"}) {
		t.Fatal(c)
	}
	for _, port := range []string{"", "invalid"} {
		t.Setenv("PORT", port)
		if ConfigFromEnvironment().Port != "8080" {
			t.Fatal("invalid PORT did not retain the legacy default")
		}
	}
}

func TestDotEnvDoesNotOverrideProcessEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	t.Setenv("APPBOX_DOTENV_EXISTING", "process")
	keys := []string{"APPBOX_DOTENV_DOUBLE", "APPBOX_DOTENV_SINGLE", "APPBOX_DOTENV_COMMENT", "APPBOX_DOTENV_HASH", "APPBOX_DOTENV_EMPTY"}
	for _, key := range keys {
		t.Setenv(key, "")
		_ = os.Unsetenv(key)
	}
	data := "# comment\nAPPBOX_DOTENV_EXISTING=file\n APPBOX_DOTENV_DOUBLE=\"two words\"\nAPPBOX_DOTENV_SINGLE='single'\nAPPBOX_DOTENV_COMMENT=word # trailing\nAPPBOX_DOTENV_HASH=abc#def\nAPPBOX_DOTENV_EMPTY=\ninvalid line\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"APPBOX_DOTENV_EXISTING": "process", "APPBOX_DOTENV_DOUBLE": "two words", "APPBOX_DOTENV_SINGLE": "single", "APPBOX_DOTENV_COMMENT": "word # trailing", "APPBOX_DOTENV_HASH": "abc#def", "APPBOX_DOTENV_EMPTY": ""} {
		if got := os.Getenv(key); got != want {
			t.Errorf("%s=%q, want %q", key, got, want)
		}
	}
	if err := LoadDotEnv(filepath.Join(dir, "missing")); err != nil {
		t.Fatal(err)
	}
}

func TestDotEnvQuotedHashesAndNewlineEscapes(t *testing.T) {
	keys := []string{"APPBOX_DOTENV_HASH_QUOTED", "APPBOX_DOTENV_NEWLINE", "APPBOX_DOTENV_SINGLE_LITERAL", "APPBOX_DOTENV_RAW"}
	for _, key := range keys {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), ".env")
	contents := "APPBOX_DOTENV_HASH_QUOTED='prefix # suffix'\nAPPBOX_DOTENV_NEWLINE=\"line 1\\nline 2\"\nAPPBOX_DOTENV_SINGLE_LITERAL='line 1\\nline 2'\nAPPBOX_DOTENV_RAW=two words # literal\n"
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"APPBOX_DOTENV_HASH_QUOTED": "prefix # suffix", "APPBOX_DOTENV_NEWLINE": "line 1\nline 2", "APPBOX_DOTENV_SINGLE_LITERAL": `line 1\nline 2`, "APPBOX_DOTENV_RAW": "two words # literal"} {
		if got := os.Getenv(name); got != want {
			t.Errorf("%s=%q, want %q", name, got, want)
		}
	}
}

func TestProjectDotEnvFallbackForSpacedKeys(t *testing.T) {
	for _, key := range []string{"APPBOX_DOTENV_SPACED", "APPBOX_DOTENV_SPACED "} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(" APPBOX_DOTENV_SPACED = value # trailing\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("APPBOX_DOTENV_SPACED") != "value" {
		t.Fatal("project-specific normalized fallback lost")
	}
}
