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

// getRealIP extracts the real client IP from request
func getRealIP(r *http.Request) string {
	// Check X-Real-IP first (set by nginx, etc.)
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	// Check X-Forwarded-For (may contain chain of IPs)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Fall back to RemoteAddr
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
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
