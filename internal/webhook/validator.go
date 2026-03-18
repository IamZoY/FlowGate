package webhook

import (
	"crypto/hmac"
	"errors"
	"net/http"
	"strings"
)

// Sentinel errors returned by Verify.
var (
	ErrMissingToken = errors.New("missing Authorization header")
	ErrInvalidToken = errors.New("invalid Authorization token")
)

// Verify checks that the request's Authorization header matches the per-app
// webhook secret stored in the database.
// MinIO may send the token as-is or prefixed with "Bearer ".
// Comparison uses hmac.Equal to prevent timing attacks.
func Verify(r *http.Request, storedSecret string) error {
	token := r.Header.Get("Authorization")
	if token == "" {
		return ErrMissingToken
	}
	// Strip "Bearer " prefix if present (MinIO sends it this way).
	token = strings.TrimPrefix(token, "Bearer ")
	if !hmac.Equal([]byte(token), []byte(storedSecret)) {
		return ErrInvalidToken
	}
	return nil
}
