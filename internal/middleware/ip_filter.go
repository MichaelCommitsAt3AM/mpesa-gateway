package middleware

import (
	"net"
	"net/http"
	"strings"
)

// IPFilter creates a middleware that validates source IP against an allowlist
func IPFilter(allowedIPs []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract real IP from headers or remote address
			clientIP := getRealIP(r)

			// Check if IP is in allowlist
			if !isIPAllowed(clientIP, allowedIPs) {
				http.Error(w, "Forbidden: Source IP not allowed", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// getRealIP extracts the client IP from the request. r.RemoteAddr is the
// single source of truth here: TrustedRealIP has already resolved it to the
// real client address when (and only when) the immediate peer was a
// configured trusted proxy, so no header parsing happens in this package.
func getRealIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	// No port present, e.g. a header-derived bare IP set by TrustedRealIP.
	return r.RemoteAddr
}

// isIPAllowed checks if client IP is in the allowlist
func isIPAllowed(clientIP string, allowedIPs []string) bool {
	// Empty allowlist = allow all (for development)
	if len(allowedIPs) == 0 {
		return true
	}

	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}

	for _, allowed := range allowedIPs {
		// Check if allowed is a CIDR range
		if strings.Contains(allowed, "/") {
			_, ipNet, err := net.ParseCIDR(allowed)
			if err == nil && ipNet.Contains(ip) {
				return true
			}
		} else {
			// Direct IP comparison
			allowedIP := net.ParseIP(allowed)
			if allowedIP != nil && ip.Equal(allowedIP) {
				return true
			}
		}
	}

	return false
}
