package middleware

import (
	"crypto/subtle"
	"net/http"
)

// EnsureInternalAuth validates the X-Internal-Secret header
func EnsureInternalAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			providedSecret := r.Header.Get("X-Internal-Secret")

			// Constant-time comparison to prevent timing attacks
			if subtle.ConstantTimeCompare([]byte(providedSecret), []byte(secret)) != 1 {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
