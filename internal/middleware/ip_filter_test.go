package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsIPAllowed(t *testing.T) {
	tests := []struct {
		name       string
		clientIP   string
		allowedIPs []string
		want       bool
	}{
		{"empty allowlist allows everything (dev mode)", "1.2.3.4", nil, true},
		{"exact match", "196.201.214.200", []string{"196.201.214.200"}, true},
		{"no match", "1.2.3.4", []string{"196.201.214.200"}, false},
		{"CIDR match", "196.201.214.5", []string{"196.201.214.0/24"}, true},
		{"CIDR no match", "196.201.215.5", []string{"196.201.214.0/24"}, false},
		{"invalid client IP", "not-an-ip", []string{"196.201.214.200"}, false},
		{"matches second entry in list", "10.0.0.1", []string{"196.201.214.200", "10.0.0.1"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isIPAllowed(tt.clientIP, tt.allowedIPs))
		})
	}
}

func TestIPFilter_BlocksDisallowedIP(t *testing.T) {
	handler := IPFilter([]string{"196.201.214.200"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/callback", nil)
	req.RemoteAddr = "6.6.6.6:12345"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestIPFilter_TrustedHeadersAreSpoofable used to document a real bypass:
// getRealIP trusted client-supplied X-Forwarded-For/X-Real-IP unconditionally,
// with no notion of a trusted upstream proxy, so any direct client could
// impersonate an allowed Safaricom IP. It now exercises the fixed pipeline —
// TrustedRealIP chained in front of IPFilter, matching how server.go wires
// them — and asserts the spoofed request is rejected.
func TestIPFilter_TrustedHeadersAreSpoofable(t *testing.T) {
	allowlist := []string{"196.201.214.200"}
	trustedProxies := []string{"10.0.0.1"}

	handler := TrustedRealIP(trustedProxies)(IPFilter(allowlist)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodPost, "/callback", nil)
	req.RemoteAddr = "6.6.6.6:12345" // real peer is NOT a trusted proxy
	req.Header.Set("X-Forwarded-For", "196.201.214.200")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code,
		"an untrusted peer's forwarded headers must not bypass the Safaricom IP allowlist")
}

// TestIPFilter_LegitimateCallbackThroughTrustedProxyIsAccepted is the
// companion positive case: a real Safaricom callback, forwarded through the
// configured trusted proxy exactly as nginx/ALB would (appending the real
// peer to X-Forwarded-For), must still be accepted.
func TestIPFilter_LegitimateCallbackThroughTrustedProxyIsAccepted(t *testing.T) {
	allowlist := []string{"196.201.214.200"}
	trustedProxies := []string{"10.0.0.1"}

	handler := TrustedRealIP(trustedProxies)(IPFilter(allowlist)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodPost, "/callback", nil)
	req.RemoteAddr = "10.0.0.1:443" // the trusted proxy itself
	req.Header.Set("X-Forwarded-For", "196.201.214.200")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}
