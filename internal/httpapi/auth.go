package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jimeh/airplan/internal/serverlog"
)

type authFailureReason string

const (
	authMissing       authFailureReason = "missing"
	authDuplicate     authFailureReason = "duplicate"
	authWrongScheme   authFailureReason = "wrong_scheme"
	authMalformed     authFailureReason = "malformed"
	authTokenMismatch authFailureReason = "token_mismatch"
)

// BearerAuth authenticates one static token using fixed-size digests.
type BearerAuth struct {
	digest [sha256.Size]byte
}

// NewBearerAuth prepares static bearer authentication.
func NewBearerAuth(token string) (*BearerAuth, error) {
	if token == "" {
		return nil, errors.New("airplan HTTP API token is empty")
	}
	return &BearerAuth{digest: sha256.Sum256([]byte(token))}, nil
}

// Wrap rejects missing, duplicate, malformed, and incorrect credentials
// before the downstream handler reads a request body.
func (a *BearerAuth) Wrap(next http.Handler) http.Handler {
	return a.WrapWithLogger(next, nil, "")
}

// WrapWithLogger authenticates like Wrap and logs only a safe rejection reason.
func (a *BearerAuth) WrapWithLogger(
	next http.Handler, logger *slog.Logger, transport string,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reason := a.failureReason(r.Header.Values("Authorization")); reason != "" {
			if logger != nil {
				logger.DebugContext(r.Context(), "authentication rejected",
					"transport", transport,
					"reason", reason,
					"request_id", serverlog.RequestID(r.Context()),
				)
			}
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeProblem(w, r, requestProblem(
				http.StatusUnauthorized,
				"unauthorized",
				"Unauthorized",
				"A valid bearer token is required.",
			))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *BearerAuth) failureReason(values []string) authFailureReason {
	if len(values) == 0 {
		return authMissing
	}
	if len(values) != 1 {
		return authDuplicate
	}
	header := values[0]
	scheme, token, found := strings.Cut(header, " ")
	if scheme != "Bearer" {
		return authWrongScheme
	}
	if !found {
		return authMalformed
	}
	if token == "" || strings.TrimSpace(token) != token ||
		strings.ContainsAny(token, " \t\r\n") {
		return authMalformed
	}
	digest := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(a.digest[:], digest[:]) != 1 {
		return authTokenMismatch
	}
	return ""
}
