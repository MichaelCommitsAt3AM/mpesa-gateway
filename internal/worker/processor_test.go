package worker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mpesa-gateway/internal/models"
	"github.com/mpesa-gateway/internal/tenant"
	"github.com/mpesa-gateway/internal/testutil"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedTransaction inserts a minimal transactions row for the given tenant,
// webhook URL, and (optional) checkout request ID, and returns the
// resulting *models.Transaction the way getTransactionByCheckoutID would
// produce it.
func seedTransaction(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID, webhookURL, checkoutRequestID string) *models.Transaction {
	t.Helper()

	tx := &models.Transaction{
		InternalTransactionID: uuid.New(),
		TenantID:              tenantID,
		IdempotencyKey:        uuid.New(),
		Amount:                decimal.NewFromInt(100),
		Phone:                 "254712345678",
		Status:                string(models.StatusPending),
		TenantWebhookURL:      webhookURL,
	}

	const insertSQL = `
		INSERT INTO transactions (
			internal_transaction_id, tenant_id, idempotency_key,
			amount, phone, status, tenant_webhook_url, checkout_request_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`
	err := pool.QueryRow(context.Background(), insertSQL,
		tx.InternalTransactionID, tx.TenantID, tx.IdempotencyKey,
		tx.Amount, tx.Phone, tx.Status, tx.TenantWebhookURL, checkoutRequestID,
	).Scan(&tx.ID)
	require.NoError(t, err)

	return tx
}

func TestSendWebhook_SignsWithTenantSecret_NotInternalTransactionID(t *testing.T) {
	pool := testutil.NewPostgres(t)
	tenantStore := tenant.NewStore(pool)
	createdTenant, _, err := tenantStore.Create(context.Background(), "Acme Corp")
	require.NoError(t, err)

	var receivedSignature string
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-Signature")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tx := seedTransaction(t, pool, createdTenant.ID, server.URL, "")

	p := NewProcessor(pool, tenantStore)

	err = p.sendWebhook(context.Background(), tx, models.StatusCompleted, map[string]interface{}{})
	require.NoError(t, err)

	require.NotEmpty(t, receivedSignature)

	expected := generateSignature(receivedBody, []byte(createdTenant.WebhookSigningSecret))
	assert.Equal(t, expected, receivedSignature, "signature must verify against the tenant's webhook signing secret")

	forgeable := generateSignature(receivedBody, []byte(tx.InternalTransactionID.String()))
	assert.NotEqual(t, forgeable, receivedSignature, "signature must not be derivable from data returned to the caller")
}

func TestProcessCallback_UpdatesStatusAndDeliversWebhook(t *testing.T) {
	pool := testutil.NewPostgres(t)
	tenantStore := tenant.NewStore(pool)
	createdTenant, _, err := tenantStore.Create(context.Background(), "Acme Corp")
	require.NoError(t, err)

	webhookDelivered := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookDelivered <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	const checkoutRequestID = "ws_CO_11012024135500"
	tx := seedTransaction(t, pool, createdTenant.ID, server.URL, checkoutRequestID)

	callbackPayload := map[string]interface{}{
		"Body": map[string]interface{}{
			"stkCallback": map[string]interface{}{
				"MerchantRequestID": "29115-34620561-1",
				"CheckoutRequestID": checkoutRequestID,
				"ResultCode":        0,
				"ResultDesc":        "The service request is processed successfully.",
				"CallbackMetadata": map[string]interface{}{
					"Item": []map[string]interface{}{
						{"Name": "Amount", "Value": 100},
						{"Name": "MpesaReceiptNumber", "Value": "OEI2AK3ZQO"},
						{"Name": "TransactionDate", "Value": 20240111135500},
						{"Name": "PhoneNumber", "Value": 254712345678},
					},
				},
			},
		},
	}
	payloadBytes, err := json.Marshal(callbackPayload)
	require.NoError(t, err)

	p := NewProcessor(pool, tenantStore)
	task := asynq.NewTask(TypeProcessCallback, payloadBytes)

	err = p.ProcessCallback(context.Background(), task)
	require.NoError(t, err)

	select {
	case <-webhookDelivered:
	case <-time.After(2 * time.Second):
		t.Fatal("webhook was not delivered")
	}

	var status string
	var completedAt *time.Time
	err = pool.QueryRow(context.Background(),
		"SELECT status, completed_at FROM transactions WHERE id = $1", tx.ID,
	).Scan(&status, &completedAt)
	require.NoError(t, err)

	assert.Equal(t, string(models.StatusCompleted), status)
	require.NotNil(t, completedAt, "completed_at should be set once a transaction reaches a terminal state")
}
