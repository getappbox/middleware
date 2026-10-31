package middleware

import (
	"log/slog"
	"net/http"
	"strings"
)

func logRecoveredPanic(component string) {
	defer func() { _ = recover() }()
	slog.Error("Recovered middleware panic", "component", component)
}

type recoveryWriter struct {
	http.ResponseWriter
	committed bool
}

func (w *recoveryWriter) WriteHeader(status int) {
	if status >= 200 || status == http.StatusSwitchingProtocols {
		w.committed = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *recoveryWriter) Write(body []byte) (int, error) {
	w.committed = true
	return w.ResponseWriter.Write(body)
}

func (w *recoveryWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tracked := &recoveryWriter{ResponseWriter: w}
	w = tracked
	if r.Method == http.MethodHead {
		w = headWriter{w}
	}
	defer func() {
		if failure := recover(); failure != nil {
			if failure == http.ErrAbortHandler {
				panic(failure)
			}
			logRecoveredPanic("request")
			if tracked.committed {
				panic(http.ErrAbortHandler)
			}
			w.Header().Del("Content-Encoding")
			path := strings.Join(strings.FieldsFunc(r.URL.EscapedPath(), func(c rune) bool { return c == '/' }), "/")
			if strings.HasPrefix(path, "api/") {
				writeError(w, apiError{500, "internal_error", "Internal Server Error"})
			} else {
				writeError(w, legacyError{500, "Internal Server Error"})
			}
		}
	}()
	s.serveHTTP(w, r)
}
