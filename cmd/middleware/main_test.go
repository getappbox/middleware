package main

import (
	"log/slog"
	"os"
	"testing"
)

func TestLegacyServeArguments(t *testing.T) {
	for _, args := range [][]string{{}, {"serve"}, {"serve", "--env", "production", "--hostname", "127.0.0.1", "--port", "2550"}, {"--port=2550", "--hostname=127.0.0.1"}, {"serve", "-e", "production", "-H", "127.0.0.1", "-p", "2550"}} {
		got, err := parseOptions(args, "8080")
		if err != nil {
			t.Fatal(err)
		}
		if len(args) > 1 && (got.hostname != "127.0.0.1" || got.port != "2550") {
			t.Fatal(got)
		}
		if len(args) <= 1 && got.port != "8080" {
			t.Fatal(got)
		}
	}
	if got, err := parseOptions([]string{"--healthcheck"}, "2550"); err != nil || !got.healthcheck || got.port != "2550" {
		t.Fatal(got, err)
	}
	if _, err := parseOptions([]string{"unknown"}, "8080"); err == nil {
		t.Fatal("unknown command accepted")
	}
}

func TestLegacyDefaultsAndEnvironmentSelection(t *testing.T) {
	t.Setenv("VAPOR_ENV", "prod")
	for _, args := range [][]string{{}, {"serve"}, {"--env", "development", "serve"}, {"-e", "dev", "serve"}} {
		got, err := parseOptions(args, "2550")
		if err != nil || got.hostname != "127.0.0.1" || got.port != "2550" {
			t.Fatal(got, err)
		}
		want := "production"
		if len(args) > 1 {
			want = "development"
		}
		if got.environment != want {
			t.Fatal(got.environment, want)
		}
	}
	for input, want := range map[string]string{"prod": "production", "production": "production", "dev": "development", "development": "development", "test": "testing", "testing": "testing", "staging": "staging"} {
		got, err := parseOptions([]string{"serve", "--env", input}, "8080")
		if err != nil || got.environment != want {
			t.Fatal(input, got, err)
		}
	}
}

func TestLegacyBindAndSocketOptions(t *testing.T) {
	for _, tc := range []struct {
		args               []string
		host, port, socket string
	}{
		{[]string{"serve", "--bind", "localhost:2550"}, "localhost", "2550", ""},
		{[]string{"serve", "-b", "localhost"}, "localhost", "8080", ""},
		{[]string{"--bind", "[::1]:2550"}, "::1", "2550", ""},
		{[]string{"--bind", "localhost:invalid"}, "localhost", "8080", ""},
		{[]string{"serve", "--unix-socket", "/tmp/appbox-test.sock"}, "127.0.0.1", "8080", "/tmp/appbox-test.sock"},
	} {
		got, err := parseOptions(tc.args, "8080")
		if err != nil || got.hostname != tc.host || got.port != tc.port || got.socketPath != tc.socket {
			t.Fatal(tc, got, err)
		}
	}
	for _, args := range [][]string{
		{"--bind", "localhost:8080", "--hostname", "localhost"},
		{"--bind", "localhost:8080", "-p", "8080"},
		{"--unix-socket", "/tmp/test.sock", "--bind", "localhost:8080"},
		{"--unix-socket", "/tmp/test.sock", "-H", "localhost"},
		{"--unix-socket", "/tmp/test.sock", "--port", "8080"},
		{"--unix-socket", ""}, {"--port", "http"}, {"--port", "65536"},
	} {
		if _, err := parseOptions(args, "8080"); err == nil {
			t.Fatal("invalid combination accepted", args)
		}
	}
}

func TestEnvironmentFilesAndPortPrecedence(t *testing.T) {
	withWorkingDirectory(t)
	for _, name := range []string{"PORT", "APPBOX_CLIENT_TOKEN", "DROPBOX_APP_KEY", "VAPOR_ENV"} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(".env.production", []byte("PORT=2550\nDROPBOX_APP_KEY=production-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".env", []byte("PORT=8081\nDROPBOX_APP_KEY=generic-key\nAPPBOX_CLIENT_TOKEN='prefix # suffix'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	selected, err := parseOptions([]string{"serve", "--env", "prod"}, "8080")
	if err != nil {
		t.Fatal(err)
	}
	config := loadConfiguration(&selected)
	if config.DropboxAppKey != "production-key" || config.ClientToken != "prefix # suffix" || selected.port != "2550" {
		t.Fatal(config.DropboxAppKey, config.ClientToken, selected)
	}
	t.Setenv("DROPBOX_APP_KEY", "injected-key")
	selected, err = parseOptions([]string{"serve", "--env", "production", "-p", "9000"}, "8080")
	if err != nil {
		t.Fatal(err)
	}
	config = loadConfiguration(&selected)
	if config.DropboxAppKey != "injected-key" || selected.port != "9000" {
		t.Fatal(config.DropboxAppKey, selected)
	}
}

func TestOptionalEnvironmentReadFailuresDoNotBlockHelpOrInjectedConfiguration(t *testing.T) {
	withWorkingDirectory(t)
	if err := os.Mkdir(".env", 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPBOX_CLIENT_TOKEN", "process-client")
	t.Setenv("DROPBOX_APP_KEY", "process-key")
	selected, err := parseOptions(nil, "8080")
	if err != nil {
		t.Fatal(err)
	}
	config := loadConfiguration(&selected)
	if config.ClientToken != "process-client" || config.DropboxAppKey != "process-key" {
		t.Fatal("injected configuration changed")
	}
	if err := run([]string{"--help"}); err != nil {
		t.Fatal(err)
	}
}

func withWorkingDirectory(t *testing.T) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Error(err)
		}
	})
}

func TestLegacyLogLevelFlagsAndEnvironment(t *testing.T) {
	t.Setenv("LOG_LEVEL", "trace")
	for name, want := range map[string]slog.Level{"trace": -8, "debug": slog.LevelDebug, "info": slog.LevelInfo, "notice": slog.LevelInfo + 2, "warning": slog.LevelWarn, "warn": slog.LevelWarn, "error": slog.LevelError, "critical": slog.LevelError + 4, "WARNING": slog.LevelWarn} {
		selected, err := parseOptions([]string{"--log", name, "serve"}, "8080")
		if err != nil || selectedLogLevel(selected) != want {
			t.Fatal(name, selected, err, selectedLogLevel(selected))
		}
	}
	for _, name := range []string{"", "invalid", " warning"} {
		if _, err := parseOptions([]string{"serve", "--log", name}, "8080"); err == nil {
			t.Fatal("invalid explicit log level accepted", name)
		}
	}
	selected, err := parseOptions([]string{"serve", "--env", "production"}, "8080")
	if err != nil {
		t.Fatal(err)
	}
	if selectedLogLevel(selected) != slog.Level(-8) {
		t.Fatal("process LOG_LEVEL ignored")
	}
	t.Setenv("LOG_LEVEL", "warning")
	if selectedLogLevel(selected) != slog.LevelWarn {
		t.Fatal("legacy warning env level ignored")
	}
	t.Setenv("LOG_LEVEL", "invalid")
	if selectedLogLevel(selected) != slog.LevelInfo+2 {
		t.Fatal("invalid environment level lost production default")
	}
	t.Setenv("LOG_LEVEL", "")
	if selectedLogLevel(options{environment: "development"}) != slog.LevelInfo || selectedLogLevel(selected) != slog.LevelInfo+2 {
		t.Fatal("legacy default levels lost")
	}
}
