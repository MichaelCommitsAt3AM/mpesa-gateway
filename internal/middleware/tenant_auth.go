package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/mpesa-gateway/internal/tenant"
)

type contextKey string

const tenantContextKey contextKey = "tenant"

// TenantAuth validates the X-API-Key header against the tenant store and, on
// success, attaches the authenticated tenant to the request context.
func TenantAuth(store *tenant.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			t, err := store.Authenticate(r.Context(), apiKey)
			if err != nil {
				if !errors.Is(err, tenant.ErrTenantNotFound) {
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), tenantContextKey, t)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TenantFromContext returns the tenant attached by TenantAuth, if any.
func TenantFromContext(ctx context.Context) (*tenant.Tenant, bool) {
	t, ok := ctx.Value(tenantContextKey).(*tenant.Tenant)
	return t, ok
}

// WithTenant attaches a tenant to the context, the same way TenantAuth does.
// Exported primarily so tests can exercise handlers downstream of TenantAuth
// without spinning up the full middleware chain.
func WithTenant(ctx context.Context, t *tenant.Tenant) context.Context {
	return context.WithValue(ctx, tenantContextKey, t)
}
