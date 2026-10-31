package main

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
)

type failingListener struct{}

func (failingListener) Accept() (net.Conn, error) { panic("listener-private-marker") }
func (failingListener) Close() error              { return nil }
func (failingListener) Addr() net.Addr            { return &net.TCPAddr{} }

func TestListenerPanicReturnsFailureForSupervisor(t *testing.T) {
	err := serveHTTP(&http.Server{}, failingListener{})
	if err == nil || strings.Contains(err.Error(), "listener-private-marker") {
		t.Fatal("listener failure escaped or leaked its panic value", err)
	}
}

func TestListenerNormalShutdownRemainsSuccessful(t *testing.T) {
	server := &http.Server{}
	server.Close()
	if err := serveHTTP(server, failingListener{}); !errors.Is(err, http.ErrServerClosed) {
		t.Fatal("normal shutdown was changed", err)
	}
}
