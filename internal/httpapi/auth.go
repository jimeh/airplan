package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := r.Header.Values("Authorization")
		if len(values) != 1 || !a.valid(values[0]) {
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

func (a *BearerAuth) valid(header string) bool {
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	token := strings.TrimPrefix(header, "Bearer ")
	if token == "" || strings.TrimSpace(token) != token ||
		strings.ContainsAny(token, " \t\r\n") {
		return false
	}
	digest := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(a.digest[:], digest[:]) == 1
}
