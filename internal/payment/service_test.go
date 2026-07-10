package payment_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mpesa-gateway/internal/mpesa"
	"github.com/mpesa-gateway/internal/payment"
	"github.com/mpesa-gateway/internal/tenant"
	"github.com/mpesa-gateway/internal/testutil"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func newTestService(t *testing.T, pool *pgxpool.Pool) *payment.Service {
	t.Helper()

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "test-token", "expires_in": "3599"})
	}))
	t.Cleanup(authServer.Close)

	stkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"MerchantRequestID":   "29115-34620561-1",
			"CheckoutRequestID":   "ws_CO_11012024135500",
			"ResponseCode":        "0",
			"ResponseDescription": "Success",
		})
	}))
	t.Cleanup(stkServer.Close)

	tokenService := mpesa.NewTokenService("key", "secret", authServer.URL)

	return payment.NewService(pool, tokenService, payment.PaymentConfig{
		ShortCode:   "174379",
		Passkey:     "passkey",
		STKPushURL:  stkServer.URL,
		CallbackURL: "https://example.com/callback",
	})
}

func TestInitiatePayment_PersistsTenantAndOmitsSecrets(t *testing.T) {
	pool := testutil.NewPostgres(t)
	tenantStore := tenant.NewStore(pool)
	ctx := context.Background()

	createdTenant, _, err := tenantStore.Create(ctx, "Acme Corp")
	require.NoError(t, err)

	svc := newTestService(t, pool)

	req := payment.InitiatePaymentRequest{
		TenantID:       createdTenant.ID,
		Amount:         decimal.NewFromInt(100),
		Phone:          "254712345678",
		WebhookURL:     "https://tenant.example.com/webhook",
		IdempotencyKey: uuid.New(),
	}

	resp, err := svc.InitiatePayment(ctx, req)
	require.NoError(t, err)

	// The response must never carry any secret - only what the struct
	// declares (transaction_id, status).
	respJSON, err := json.Marshal(resp)
	require.NoError(t, err)
	var respMap map[string]interface{}
	require.NoError(t, json.Unmarshal(respJSON, &respMap))
	require.ElementsMatch(t, []string{"transaction_id", "status"}, mapKeys(respMap))

	var storedTenantID uuid.UUID
	err = pool.QueryRow(ctx, "SELECT tenant_id FROM transactions WHERE internal_transaction_id = $1", resp.TransactionID).Scan(&storedTenantID)
	require.NoError(t, err)
	require.Equal(t, createdTenant.ID, storedTenantID)
}

func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
