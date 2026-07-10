package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// capturePeerHandler records the resolved r.RemoteAddr (host only) so tests
// can assert what TrustedRealIP decided the client IP was.
func capturePeerHandler(got *string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = getRealIP(r)
		w.WriteHeader(http.StatusOK)
	})
}

func TestTrustedRealIP_UntrustedPeerHeadersIgnored(t *testing.T) {
	var resolved string
	handler := TrustedRealIP([]string{"10.0.0.1"})(capturePeerHandler(&resolved))

	req := httptest.NewRequest(http.MethodPost, "/callback", nil)
	req.RemoteAddr = "6.6.6.6:12345" // not a trusted proxy
	req.Header.Set("X-Forwarded-For", "196.201.214.200")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, "6.6.6.6", resolved, "an untrusted peer's forwarded headers must be ignored")
}

func TestTrustedRealIP_TrustedPeerXRealIPHonored(t *testing.T) {
	var resolved string
	handler := TrustedRealIP([]string{"10.0.0.1"})(capturePeerHandler(&resolved))

	req := httptest.NewRequest(http.MethodPost, "/callback", nil)
	req.RemoteAddr = "10.0.0.1:443" // trusted proxy
	req.Header.Set("X-Real-IP", "196.201.214.200")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, "196.201.214.200", resolved)
}

func TestTrustedRealIP_TrustedPeerXForwardedForUsesRightmostEntry(t *testing.T) {
	var resolved string
	handler := TrustedRealIP([]string{"10.0.0.1"})(capturePeerHandler(&resolved))

	req := httptest.NewRequest(http.MethodPost, "/callback", nil)
	req.RemoteAddr = "10.0.0.1:443" // trusted proxy
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 196.201.214.200")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, "196.201.214.200", resolved, "the rightmost XFF entry is the one the trusted proxy itself appended")
}

// TestTrustedRealIP_ForgedLeadingXFFEntryFromTrustedPeerIsIgnored is the
// specific attack a leftmost-entry implementation would miss: an attacker
// sends a forged X-Forwarded-For claiming to be a Safaricom IP; the trusted
// proxy in front does not replace it, only appends the attacker's real
// observed address. If we trusted the leftmost/first entry, the attacker's
// forged value would be read straight back out and the "trusted proxy"
// check would buy nothing.
func TestTrustedRealIP_ForgedLeadingXFFEntryFromTrustedPeerIsIgnored(t *testing.T) {
	var resolved string
	handler := TrustedRealIP([]string{"10.0.0.1"})(capturePeerHandler(&resolved))

	req := httptest.NewRequest(http.MethodPost, "/callback", nil)
	req.RemoteAddr = "10.0.0.1:443" // trusted proxy
	req.Header.Set("X-Forwarded-For", "196.201.214.200, 6.6.6.6")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, "6.6.6.6", resolved, "must resolve to the attacker's real (rightmost) address, not their forged (leftmost) claim")
}

func TestTrustedRealIP_TrustedProxyCIDRMatch(t *testing.T) {
	var resolved string
	handler := TrustedRealIP([]string{"10.0.0.0/24"})(capturePeerHandler(&resolved))

	req := httptest.NewRequest(http.MethodPost, "/callback", nil)
	req.RemoteAddr = "10.0.0.55:443" // inside the trusted CIDR
	req.Header.Set("X-Real-IP", "196.201.214.200")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, "196.201.214.200", resolved, "a trusted-proxy IP inside a configured CIDR must be recognized, not just an exact match")
}

func TestTrustedRealIP_EmptyTrustedProxiesNeverHonorsHeaders(t *testing.T) {
	var resolved string
	handler := TrustedRealIP(nil)(capturePeerHandler(&resolved))

	req := httptest.NewRequest(http.MethodPost, "/callback", nil)
	req.RemoteAddr = "10.0.0.1:443"
	req.Header.Set("X-Real-IP", "196.201.214.200")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, "10.0.0.1", resolved, "empty trustedProxies must mean trust none, regardless of peer")
}

func TestTrustedRealIP_NoHeadersFallsBackToPeerAddress(t *testing.T) {
	var resolved string
	handler := TrustedRealIP([]string{"10.0.0.1"})(capturePeerHandler(&resolved))

	req := httptest.NewRequest(http.MethodPost, "/callback", nil)
	req.RemoteAddr = "10.0.0.1:443"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, "10.0.0.1", resolved)
}
