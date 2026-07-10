package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mpesa-gateway/internal/testutil"
	"github.com/stretchr/testify/require"
)

// TestHealthCheck_DatabaseUp uses a real Postgres container (via
// testcontainers) rather than a mock, so this exercises the actual
// pgxpool.Pool.Ping path against real data infrastructure.
func TestHealthCheck_DatabaseUp(t *testing.T) {
	pool := testutil.NewPostgres(t)
	h := &Handler{db: pool}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	h.HealthCheck(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"status":"ok","database":"up"}`, rec.Body.String())
}

func TestHealthCheck_DatabaseDown(t *testing.T) {
	pool := testutil.NewPostgres(t)
	pool.Close() // simulate an unreachable database

	h := &Handler{db: pool}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	h.HealthCheck(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.JSONEq(t, `{"status":"degraded","database":"down"}`, rec.Body.String())
}
