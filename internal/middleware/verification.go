package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

const maxTokenVerifications = 8

type verificationFlight struct {
	done    chan struct{}
	verdict tokenVerdict
	err     error
	cancel  context.CancelFunc
	waiters int
}

func verificationUnavailable() error {
	return apiError{503, "verification_unavailable", "Could not verify the Dropbox token. Try again shortly."}
}

func (s *Server) verifiedToken(ctx context.Context, token string, digest [32]byte) (tokenVerdict, error) {
	if ctx.Err() != nil {
		return tokenVerdict{}, verificationUnavailable()
	}
	if verdict, cached := s.tokens.get(digest, s.now()); cached {
		return verdict, nil
	}
	flight, verdict, cached, err := s.joinVerification(ctx, token, digest)
	if cached || err != nil {
		return verdict, err
	}
	select {
	case <-ctx.Done():
		s.authMu.Lock()
		flight.waiters--
		if flight.waiters == 0 && s.authFlights[digest] == flight {
			delete(s.authFlights, digest)
			flight.cancel()
		}
		s.authMu.Unlock()
		return tokenVerdict{}, verificationUnavailable()
	case <-flight.done:
		if ctx.Err() != nil {
			return tokenVerdict{}, verificationUnavailable()
		}
		return flight.verdict, flight.err
	}
}

func (s *Server) joinVerification(ctx context.Context, token string, digest [32]byte) (*verificationFlight, tokenVerdict, bool, error) {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	if verdict, cached := s.tokens.get(digest, s.now()); cached {
		return nil, verdict, true, nil
	}
	flight := s.authFlights[digest]
	if flight == nil {
		if len(s.authFlights) >= maxTokenVerifications {
			return nil, tokenVerdict{}, false, verificationUnavailable()
		}
		verifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 40*time.Second)
		flight = &verificationFlight{done: make(chan struct{}), cancel: cancel}
		s.authFlights[digest] = flight
		go s.verifyToken(verifyCtx, token, digest, flight)
	}
	flight.waiters++
	return flight, tokenVerdict{}, false, nil
}

func (s *Server) verifyToken(ctx context.Context, token string, digest [32]byte, flight *verificationFlight) {
	defer flight.cancel()
	var verdict tokenVerdict
	var err error
	defer func() {
		if recover() != nil {
			logRecoveredPanic("verification")
			verdict, err = tokenVerdict{}, verificationUnavailable()
		}
		s.authMu.Lock()
		defer s.authMu.Unlock()
		if s.authFlights[digest] == flight {
			delete(s.authFlights, digest)
		}
		flight.verdict, flight.err = verdict, err
		close(flight.done)
	}()
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.dropboxapi.com/2/users/get_current_account", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	var response *http.Response
	var body []byte
	response, body, err = s.fetch(request, 4*1024*1024)
	if err != nil || ctx.Err() != nil || (response.StatusCode != 200 && response.StatusCode != 400 && response.StatusCode != 401) {
		err = verificationUnavailable()
	} else {
		verdict.verified = response.StatusCode == 200
		ttl := time.Minute
		if verdict.verified {
			var account struct {
				ID string `json:"account_id"`
			}
			_ = json.Unmarshal(body, &account)
			verdict.accountID = account.ID
			if verdict.accountID == "" {
				verdict.accountID = "unknown"
			}
			ttl = s.config.DropboxTokenCacheTTL
		}
		s.tokens.put(digest, verdict, ttl, s.now())
	}
}
