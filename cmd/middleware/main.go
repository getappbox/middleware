package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/getappbox/middleware/internal/middleware"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("middleware stopped", "error", err)
		os.Exit(1)
	}
}

type options struct {
	hostname, port, environment, socketPath, logLevel string
	healthcheck, useDefaultPort                       bool
}

func normalizeEnvironment(value string) string {
	switch value {
	case "prod", "production":
		return "production"
	case "dev", "development":
		return "development"
	case "test", "testing":
		return "testing"
	default:
		return value
	}
}

func stripServeCommand(args []string) []string {
	result := make([]string, 0, len(args))
	valueNext, stripped := false, false
	for _, arg := range args {
		if !valueNext && !stripped && arg == "serve" {
			stripped = true
			continue
		}
		result = append(result, arg)
		if valueNext {
			valueNext = false
			continue
		}
		switch arg {
		case "--env", "-env", "-e", "--hostname", "-hostname", "-H", "--port", "-port", "-p", "--bind", "-bind", "-b", "--unix-socket", "-unix-socket", "--log", "-log":
			valueNext = true
		}
	}
	return result
}

func parseOptions(args []string, port string) (options, error) {
	args = stripServeCommand(args)
	flags := flag.NewFlagSet("middleware", flag.ContinueOnError)
	var result options
	environment, exists := os.LookupEnv("VAPOR_ENV")
	if !exists {
		environment = "development"
	}
	var bind string
	flags.StringVar(&result.hostname, "hostname", "127.0.0.1", "HTTP bind address")
	flags.StringVar(&result.hostname, "H", "127.0.0.1", "HTTP bind address")
	flags.StringVar(&result.port, "port", port, "HTTP port")
	flags.StringVar(&result.port, "p", port, "HTTP port")
	flags.StringVar(&result.environment, "env", environment, "deployment environment")
	flags.StringVar(&result.environment, "e", environment, "deployment environment")
	flags.StringVar(&bind, "bind", "", "HTTP hostname and optional port")
	flags.StringVar(&bind, "b", "", "HTTP hostname and optional port")
	flags.StringVar(&result.socketPath, "unix-socket", "", "Unix domain socket path")
	flags.BoolVar(&result.healthcheck, "healthcheck", false, "check the local HTTP server and exit")
	flags.StringVar(&result.logLevel, "log", "", "log level")
	if err := flags.Parse(args); err != nil {
		return result, err
	}
	if flags.NArg() != 0 {
		return result, fmt.Errorf("unexpected argument: %s", flags.Arg(0))
	}
	specified := make(map[string]bool)
	flags.Visit(func(f *flag.Flag) { specified[f.Name] = true })
	if specified["log"] {
		if _, err := parseLogLevel(result.logLevel); err != nil {
			return result, err
		}
	}
	hasHost, hasPort, hasBind, hasSocket := specified["hostname"] || specified["H"], specified["port"] || specified["p"], specified["bind"] || specified["b"], specified["unix-socket"]
	if hasSocket && (hasHost || hasPort || hasBind) || hasBind && (hasHost || hasPort) {
		return result, errors.New("--bind and --unix-socket cannot be combined with hostname or port options")
	}
	if hasSocket && result.socketPath == "" {
		return result, errors.New("--unix-socket requires a nonempty path")
	}
	result.useDefaultPort = !hasPort
	if hasPort {
		numeric, err := strconv.Atoi(result.port)
		if err != nil || numeric < 0 || numeric > 65535 {
			return result, fmt.Errorf("invalid HTTP port: %s", result.port)
		}
		result.port = strconv.Itoa(numeric)
	}
	if hasBind {
		host, bindPort, err := net.SplitHostPort(bind)
		if err == nil {
			if host != "" {
				result.hostname = host
			}
			if numeric, err := strconv.Atoi(bindPort); err == nil {
				if numeric < 0 || numeric > 65535 {
					return result, fmt.Errorf("invalid HTTP port: %s", bindPort)
				}
				result.port, result.useDefaultPort = strconv.Itoa(numeric), false
			}
		} else {
			parts := strings.Split(bind, ":")
			if len(parts) == 1 {
				if bind != "" {
					result.hostname = bind
				}
			} else if len(parts) == 2 {
				if parts[0] != "" {
					result.hostname = parts[0]
				}
				if numeric, err := strconv.Atoi(parts[1]); err == nil {
					if numeric < 0 || numeric > 65535 {
						return result, fmt.Errorf("invalid HTTP port: %s", parts[1])
					}
					result.port, result.useDefaultPort = strconv.Itoa(numeric), false
				}
			} else if net.ParseIP(strings.Trim(bind, "[]")) != nil {
				result.hostname = strings.Trim(bind, "[]")
			} else {
				return result, fmt.Errorf("invalid bind address: %s", bind)
			}
		}
	}
	result.environment = normalizeEnvironment(result.environment)
	return result, nil
}

func loadConfiguration(selected *options) middleware.Config {
	for _, path := range []string{".env." + selected.environment, ".env"} {
		if err := middleware.LoadDotEnv(path); err != nil {
			slog.Warn("optional environment file unavailable", "path", path, "error", err)
		}
	}
	config := middleware.ConfigFromEnvironment()
	if selected.useDefaultPort {
		selected.port = config.Port
	}
	return config
}

func checkHealth(selected options) error {
	transport := &http.Transport{Proxy: nil}
	hostname := selected.hostname
	switch hostname {
	case "", "0.0.0.0":
		hostname = "127.0.0.1"
	case "::":
		hostname = "::1"
	}
	if selected.socketPath != "" {
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 4 * time.Second}).DialContext(ctx, "unix", selected.socketPath)
		}
		hostname = "localhost"
	}
	client := &http.Client{Timeout: 4 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	response, err := client.Get("http://" + net.JoinHostPort(hostname, selected.port) + "/")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if _, err := io.Copy(io.Discard, io.LimitReader(response.Body, 1024)); err != nil {
		return err
	}
	if response.StatusCode != 200 {
		return fmt.Errorf("health check returned %d", response.StatusCode)
	}
	return nil
}

func run(args []string) error {
	config := middleware.ConfigFromEnvironment()
	selected, err := parseOptions(args, config.Port)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	config = loadConfiguration(&selected)
	if selected.healthcheck {
		return checkHealth(selected)
	}
	level := selectedLogLevel(selected)
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	if missing := config.MissingAPIConfiguration(); len(missing) != 0 {
		slog.Warn("AppBox /api/v1 secrets missing", "variables", missing)
	}
	server := &http.Server{
		Addr:              net.JoinHostPort(selected.hostname, selected.port),
		Handler:           middleware.New(config, nil),
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 210 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 1024 * 1024,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	network, address := "tcp", server.Addr
	if selected.socketPath != "" {
		network, address = "unix", selected.socketPath
	}
	listener, err := net.Listen(network, address)
	if err != nil {
		return err
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() { done <- serveHTTP(server, listener) }()
	slog.Info("AppBox middleware listening", "network", network, "address", listener.Addr().String())
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

func serveHTTP(server *http.Server, listener net.Listener) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("HTTP listener failed unexpectedly")
		}
	}()
	return server.Serve(listener)
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(value) {
	case "trace":
		return slog.Level(-8), nil
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "notice":
		return slog.LevelInfo + 2, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	case "critical":
		return slog.LevelError + 4, nil
	default:
		return slog.LevelInfo, fmt.Errorf("invalid log level: %s", value)
	}
}

func selectedLogLevel(selected options) slog.Level {
	value := selected.logLevel
	if value == "" {
		value = os.Getenv("LOG_LEVEL")
	}
	if level, err := parseLogLevel(value); err == nil {
		return level
	}
	if selected.environment == "production" {
		return slog.LevelInfo + 2
	}
	return slog.LevelInfo
}
