package httpapi

import (
	"fmt"
	"net/http"
	"net/url"
)

// OriginGuard validates present browser origins against an exact allowlist.
// Requests without Origin are accepted for non-browser agent clients.
type OriginGuard struct {
	allowed map[string]struct{}
}

// NewOriginGuard validates and stores an explicit origin allowlist.
func NewOriginGuard(origins []string) (*OriginGuard, error) {
	guard := &OriginGuard{allowed: make(map[string]struct{}, len(origins))}
	for _, origin := range origins {
		canonical, err := canonicalOrigin(origin)
		if err != nil {
			return nil, fmt.Errorf("invalid allowed origin %q: %w", origin, err)
		}
		guard.allowed[canonical] = struct{}{}
	}
	return guard, nil
}

// Wrap enforces explicit Origin verification. It is intended to wrap the
// Streamable HTTP MCP endpoint as well as any future browser-capable route.
func (g *OriginGuard) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origins := r.Header.Values("Origin")
		if len(origins) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		allowed := false
		if len(origins) == 1 {
			origin, err := canonicalOrigin(origins[0])
			if err == nil {
				_, allowed = g.allowed[origin]
			}
		}
		if !allowed {
			writeProblem(w, r, requestProblem(
				http.StatusForbidden,
				"origin_forbidden",
				"Origin forbidden",
				"The request Origin is not allowed.",
			))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func canonicalOrigin(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil || parsed.Path != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("must be an HTTP(S) origin without a path")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}
