package middleware

import (
	"context"
	"errors"
	"net/http"
)

type redirectHostsKey struct{}

func allowRedirectHosts(request *http.Request, hosts []string) *http.Request {
	return request.WithContext(context.WithValue(request.Context(), redirectHostsKey{}, hosts))
}

func checkRedirect(request *http.Request, via []*http.Request) error {
	if len(via) > 5 {
		return errors.New("redirect limit exceeded")
	}
	if request.URL.Scheme != "https" || request.URL.User != nil {
		return errors.New("redirect requires credential-free HTTPS")
	}
	for _, previous := range via {
		if previous.URL.String() == request.URL.String() {
			return errors.New("redirect cycle")
		}
	}
	if hosts, scoped := request.Context().Value(redirectHostsKey{}).([]string); scoped && !allowedHost(request.URL.Hostname(), hosts) {
		return errors.New("redirect host is not allowed")
	}
	return nil
}
