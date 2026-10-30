package process_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

var executable string

func TestMain(m *testing.M) {
	working, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	root := filepath.Clean(filepath.Join(working, "../.."))
	dir, err := os.MkdirTemp("", "middleware-process-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	executable = filepath.Join(dir, "middleware")
	goBin := os.Getenv("GO_BIN")
	if goBin == "" {
		goBin = filepath.Join(runtime.GOROOT(), "bin", "go")
	}
	command := exec.Command(goBin, "build", "-race", "-mod=readonly", "-trimpath", "-o", executable, "./cmd/middleware")
	command.Dir = root
	command.Env = append(os.Environ(), "GOPROXY=off", "GOTOOLCHAIN=local")
	if output, err := command.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build test executable: %v\n%s", err, output)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func processEnv(extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "APPBOX_CLIENT_TOKEN=test-client", "DROPBOX_APP_KEY=test-key", "LOG_LEVEL=info"}, extra...)
}

func runCommand(t *testing.T, dir string, env []string, args ...string) ([]byte, error) {
	t.Helper()
	command := exec.Command(executable, args...)
	command.Dir = dir
	command.Env = env
	return command.CombinedOutput()
}

type runningServer struct {
	command *exec.Cmd
	log     *os.File
	done    chan error
	stopped bool
}

func startServer(t *testing.T, dir string, env []string, args ...string) (*runningServer, string) {
	t.Helper()
	log, err := os.CreateTemp(t.TempDir(), "server-log-")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, args...)
	command.Dir = dir
	command.Env = env
	command.Stdout = log
	command.Stderr = log
	if err := command.Start(); err != nil {
		log.Close()
		t.Fatal(err)
	}
	server := &runningServer{command: command, log: log, done: make(chan error, 1)}
	go func() { server.done <- command.Wait() }()
	t.Cleanup(func() {
		if !server.stopped {
			command.Process.Kill()
			<-server.done
		}
		log.Close()
	})
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-server.done:
			server.stopped = true
			data, _ := os.ReadFile(log.Name())
			t.Fatalf("server exited: %v\n%s", err, data)
		default:
		}
		data, _ := os.ReadFile(log.Name())
		for _, line := range bytes.Split(data, []byte("\n")) {
			var entry struct {
				Message string `json:"msg"`
				Address string `json:"address"`
			}
			if json.Unmarshal(line, &entry) == nil && entry.Message == "AppBox middleware listening" {
				return server, entry.Address
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	data, _ := os.ReadFile(log.Name())
	t.Fatalf("server did not start: %s", data)
	return nil, ""
}

func (s *runningServer) stop(t *testing.T) {
	t.Helper()
	if err := s.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-s.done:
		s.stopped = true
		if err != nil {
			data, _ := os.ReadFile(s.log.Name())
			t.Fatalf("shutdown: %v\n%s", err, data)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("shutdown did not finish")
	}
}

func get(t *testing.T, address, path string) *http.Response {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}}
	t.Cleanup(client.CloseIdleConnections)
	request, err := http.NewRequest("GET", "http://"+address+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-AppBox-Client-Token", "test-client")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	return response
}

func TestNativeLoopbackHealthAndSIGTERM(t *testing.T) {
	dir := t.TempDir()
	server, address := startServer(t, dir, processEnv(), "serve", "--port", "0")
	host, port, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		t.Fatalf("default listener %q: %v", address, err)
	}
	response := get(t, address, "/")
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != 200 || string(body) != "AppBox Install Service Helper" || response.ContentLength != int64(len(body)) {
		t.Fatalf("health body %q, status %d, length %d, error %v", body, response.StatusCode, response.ContentLength, err)
	}
	if output, err := runCommand(t, dir, processEnv(), "--healthcheck", "--port", port); err != nil {
		t.Fatalf("health command: %v: %s", err, output)
	}
	server.stop(t)
	if output, err := runCommand(t, dir, processEnv(), "--healthcheck", "--port", port); err == nil {
		t.Fatalf("health succeeds after shutdown: %s", output)
	}
}

func TestNativeEnvironmentFilePrecedence(t *testing.T) {
	dir := t.TempDir()
	for name, contents := range map[string]string{".env": "DROPBOX_APP_KEY=generic\nPORT=not-a-port\n", ".env.production": "DROPBOX_APP_KEY='production # key'\nAPPBOX_CLIENT_TOKEN=file-token\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name  string
		extra []string
		want  string
	}{
		{"environment-file", []string{"PATH=" + os.Getenv("PATH"), "APPBOX_CLIENT_TOKEN=test-client", "LOG_LEVEL=info"}, "production # key"},
		{"process-override", processEnv(), "test-key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, address := startServer(t, dir, tc.extra, "--env", "prod", "serve", "--port", "0")
			response := get(t, address, "/api/v1/config")
			var config struct {
				Key string `json:"dropboxAppKey"`
			}
			if err := json.NewDecoder(response.Body).Decode(&config); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != 200 || config.Key != tc.want {
				t.Fatal(response.StatusCode, config.Key)
			}
			server.stop(t)
		})
	}
}

func TestNativeUnixSocketAndOccupiedAddress(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "abx-socket-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "middleware.sock")
	server, address := startServer(t, dir, processEnv(), "serve", "--unix-socket", socket)
	if address != socket {
		t.Fatal(address)
	}
	if output, err := runCommand(t, dir, processEnv(), "--healthcheck", "--unix-socket", socket); err != nil {
		t.Fatalf("socket health: %v: %s", err, output)
	}
	if output, err := runCommand(t, dir, processEnv(), "serve", "--unix-socket", socket); err == nil {
		t.Fatalf("replaced occupied socket: %s", output)
	}
	if _, err := os.Stat(socket); err != nil {
		t.Fatal("occupied socket removed", err)
	}
	server.stop(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if output, err := runCommand(t, dir, processEnv(), "serve", "--bind", listener.Addr().String()); err == nil {
		t.Fatalf("occupied TCP address accepted: %s", output)
	}
}

func TestHealthCommandRejectsBadResponses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"error-status", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }},
		{"redirect", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/other", 302) }},
		{"truncated-body", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			io.WriteString(w, "short")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(tc.handler)
			defer server.Close()
			if output, err := runCommand(t, t.TempDir(), processEnv(), "--healthcheck", "--bind", strings.TrimPrefix(server.URL, "http://")); err == nil {
				t.Fatalf("health accepted bad response: %s", output)
			}
		})
	}
}

func TestInvalidStartupNeverBinds(t *testing.T) {
	for _, args := range [][]string{{"serve", "--port", "invalid"}, {"serve", "--bind", "127.0.0.1:0", "--port", "0"}, {"serve", "--unix-socket", ""}, {"serve", "--log", "invalid"}, {"unknown"}} {
		if output, err := runCommand(t, t.TempDir(), processEnv(), args...); err == nil {
			t.Fatalf("accepted %q: %s", args, output)
		}
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".env"), 0700); err != nil {
		t.Fatal(err)
	}
	if output, err := runCommand(t, dir, processEnv(), "--help"); err != nil || !strings.Contains(string(output), "-healthcheck") {
		t.Fatalf("help: %v: %s", err, output)
	}
}
