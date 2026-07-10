package middleware

import (
	"net"
	"net/http"
	"strings"
)

// TrustedRealIP replaces chi's middleware.RealIP, which rewrites
// r.RemoteAddr from X-Real-IP/X-Forwarded-For unconditionally — safe only if
// every request is guaranteed to come through a trusted reverse proxy, which
// is not true here without this check (see its doc comment).
//
// It only honors forwarded headers when the immediate TCP peer is itself in
// trustedProxies; otherwise the raw peer address is used untouched. An empty
// trustedProxies list means trust none (headers are always ignored) — the
// opposite of isIPAllowed's "empty allowlist = allow all" dev-mode default,
// since this is a new security boundary that must fail closed.
func TrustedRealIP(trustedProxies []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peerIP, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				peerIP = r.RemoteAddr
			}

			if len(trustedProxies) > 0 && isIPAllowed(peerIP, trustedProxies) {
				if resolved := resolveForwardedIP(r); resolved != "" {
					r.RemoteAddr = resolved
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// resolveForwardedIP extracts the client IP a trusted proxy vouches for.
//
// X-Real-IP is set (overwritten), not appended, by proxies like nginx, so a
// single value there is trustworthy as-is.
//
// X-Forwarded-For is appended to, not replaced, by each hop
// (nginx's $proxy_add_x_forwarded_for, ALB, Cloudflare, etc.), so a request
// arriving with a client-forged "X-Forwarded-For: <forged-ip>" gets the
// proxy's own observed peer address appended: "<forged-ip>, <real-ip>". Only
// the rightmost entry is the one the trusted proxy itself added; everything
// left of it is client-supplied and must not be trusted. This assumes the
// trusted proxy actually appends rather than blindly forwarding the client's
// header verbatim — true for nginx/ALB/Cloudflare defaults, but worth
// confirming for whatever proxy is actually deployed in front.
func resolveForwardedIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}

	return ""
}
