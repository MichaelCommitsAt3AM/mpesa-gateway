package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mpesa-gateway/internal/middleware"
	"github.com/mpesa-gateway/internal/tenant"
	"github.com/mpesa-gateway/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTenantAuth(t *testing.T) {
	pool := testutil.NewPostgres(t)
	store := tenant.NewStore(pool)
	created, apiKey, err := store.Create(context.Background(), "Acme Corp")
	require.NoError(t, err)

	var tenantOnContext *tenant.Tenant
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantOnContext, _ = middleware.TenantFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.TenantAuth(store)(next)

	tests := []struct {
		name       string
		header     string
		wantStatus int
	}{
		{"valid API key", apiKey, http.StatusOK},
		{"wrong API key", "wrong-key", http.StatusUnauthorized},
		{"missing header", "", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenantOnContext = nil
			req := httptest.NewRequest(http.MethodPost, "/initiate", nil)
			if tt.header != "" {
				req.Header.Set("X-API-Key", tt.header)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantStatus == http.StatusOK {
				require.NotNil(t, tenantOnContext)
				assert.Equal(t, created.ID, tenantOnContext.ID)
			} else {
				assert.Nil(t, tenantOnContext)
			}
		})
	}
}
