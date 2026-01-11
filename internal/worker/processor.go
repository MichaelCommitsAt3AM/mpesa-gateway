package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	"mpesa-gateway/internal/models"
	"mpesa-gateway/internal/mpesa"
)

const (
	TypeProcessCallback = "callback:process"
)

// Processor handles background job processing
type Processor struct {
	db     *pgxpool.Pool
	client *http.Client
}

// NewProcessor creates a new worker processor
func NewProcessor(db *pgxpool.Pool) *Processor {
	return &Processor{
		db: db,
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
		},
	}
}

// NewProcessCallbackTask creates a new callback processing task
func NewProcessCallbackTask(payload []byte) (*asynq.Task, error) {
	return asynq.NewTask(TypeProcessCallback, payload), nil
}

// ProcessCallback processes M-Pesa callback
func (p *Processor) ProcessCallback(ctx context.Context, t *asynq.Task) error {
	var callback CallbackPayload
	if err := json.Unmarshal(t.Payload(), &callback); err != nil {
		return fmt.Errorf("failed to unmarshal callback: %w", err)
	}

	log.Printf("Processing callback for CheckoutRequestID: %s", callback.Body.StkCallback.CheckoutRequestID)

	// Extract checkout request ID
	checkoutRequestID := callback.Body.StkCallback.CheckoutRequestID
	if checkoutRequestID == "" {
		return fmt.Errorf("missing CheckoutRequestID in callback")
	}

	// Find transaction in database
	tx, err := p.getTransactionByCheckoutID(ctx, checkoutRequestID)
	if err != nil {
		return fmt.Errorf("failed to find transaction: %w", err)
	}

	// Validate state transition
	currentStatus := models.TransactionStatus(tx.Status)
	if currentStatus != models.StatusPending {
		log.Printf("Transaction %s is already in terminal state: %s", tx.InternalTransactionID, currentStatus)
		return nil // Skip processing
	}

	// Parse result
	resultCode := callback.Body.StkCallback.ResultCode
	var newStatus models.TransactionStatus
	var errorMsg *string

	if resultCode == 0 {
		newStatus = models.StatusCompleted
	} else {
		newStatus = models.StatusFailed
		msg := callback.Body.StkCallback.ResultDesc
		errorMsg = &msg
	}

	// Validate transition
	if !models.IsValidTransition(currentStatus, newStatus) {
		return fmt.Errorf("invalid state transition from %s to %s", currentStatus, newStatus)
	}

	// Parse metadata
	metadata := mpesa.ParseMpesaMetadata(callback.Body.StkCallback.CallbackMetadata.Item)
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Update transaction
	updateSQL := `
		UPDATE transactions 
		SET status = $1, 
		    mpesa_metadata = $2, 
		    error_message = $3,
		    completed_at = CASE WHEN $1 IN ('COMPLETED', 'FAILED') THEN NOW() ELSE completed_at END
		WHERE checkout_request_id = $4 AND status = 'PENDING'
	`

	result, err := p.db.Exec(ctx, updateSQL, string(newStatus), metadataJSON, errorMsg, checkoutRequestID)
	if err != nil {
		return fmt.Errorf("failed to update transaction: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		log.Printf("No rows updated for CheckoutRequestID: %s (may have been processed already)", checkoutRequestID)
		return nil
	}

	log.Printf("Transaction %s updated to status: %s", tx.InternalTransactionID, newStatus)

	// Send webhook to tenant
	if err := p.sendWebhook(ctx, tx, newStatus, metadata); err != nil {
		log.Printf("Webhook delivery failed for %s: %v", tx.InternalTransactionID, err)
		// Don't fail the task, webhook failures are logged separately
	}

	return nil
}

// getTransactionByCheckoutID fetches transaction from database
func (p *Processor) getTransactionByCheckoutID(ctx context.Context, checkoutRequestID string) (*models.Transaction, error) {
	query := `
		SELECT id, internal_transaction_id, idempotency_key, checkout_request_id, 
		       amount, phone, status, tenant_webhook_url, created_at, updated_at
		FROM transactions 
		WHERE checkout_request_id = $1
	`

	var tx models.Transaction
	err := p.db.QueryRow(ctx, query, checkoutRequestID).Scan(
		&tx.ID,
		&tx.InternalTransactionID,
		&tx.IdempotencyKey,
		&tx.CheckoutRequestID,
		&tx.Amount,
		&tx.Phone,
		&tx.Status,
		&tx.TenantWebhookURL,
		&tx.CreatedAt,
		&tx.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &tx, nil
}

// sendWebhook delivers the result to tenant's webhook URL
func (p *Processor) sendWebhook(ctx context.Context, tx *models.Transaction, status models.TransactionStatus, metadata map[string]interface{}) error {
	webhookPayload := map[string]interface{}{
		"transaction_id": tx.InternalTransactionID,
		"status":         string(status),
		"amount":         tx.Amount,
		"phone":          tx.Phone,
		"metadata":       metadata,
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	}

	payloadBytes, err := json.Marshal(webhookPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	// Create signature (HMAC-SHA256)
	signature := generateSignature(payloadBytes, []byte(tx.InternalTransactionID.String()))

	// Send webhook with retries
	attemptNumber := 1
	maxRetries := 4
	backoff := []time.Duration{0, 1 * time.Minute, 5 * time.Minute, 15 * time.Minute}

	for attemptNumber <= maxRetries {
		if attemptNumber > 1 {
			log.Printf("Webhook retry %d/%d for %s", attemptNumber, maxRetries, tx.InternalTransactionID)
			time.Sleep(backoff[attemptNumber-1])
		}

		success, statusCode, responseBody, responseTime := p.deliverWebhook(ctx, tx.TenantWebhookURL, payloadBytes, signature)

		// Record attempt
		p.recordWebhookAttempt(ctx, tx.ID, attemptNumber, tx.TenantWebhookURL, webhookPayload, success, statusCode, responseBody, responseTime)

		if success {
			log.Printf("Webhook delivered successfully to %s", tx.TenantWebhookURL)
			return nil
		}

		attemptNumber++
	}

	return fmt.Errorf("webhook delivery failed after %d attempts", maxRetries)
}

// deliverWebhook performs the actual HTTP POST
func (p *Processor) deliverWebhook(ctx context.Context, url string, payload []byte, signature string) (bool, int, string, int64) {
	startTime := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return false, 0, err.Error(), 0
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", signature)

	resp, err := p.client.Do(req)
	responseTime := time.Since(startTime).Milliseconds()

	if err != nil {
		return false, 0, err.Error(), responseTime
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	success := resp.StatusCode >= 200 && resp.StatusCode < 300

	return success, resp.StatusCode, string(body), responseTime
}

// recordWebhookAttempt logs webhook delivery attempt
func (p *Processor) recordWebhookAttempt(ctx context.Context, txID interface{}, attemptNum int, url string, payload map[string]interface{}, success bool, statusCode int, responseBody string, responseTime int64) {
	insertSQL := `
		INSERT INTO webhook_attempts (
			transaction_id, attempt_number, webhook_url, 
			request_payload, response_status_code, response_body, 
			response_time_ms, success, error_message
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	payloadJSON, _ := json.Marshal(payload)

	var errMsg *string
	if !success {
		msg := responseBody
		errMsg = &msg
	}

	_, err := p.db.Exec(ctx, insertSQL,
		txID, attemptNum, url, payloadJSON,
		statusCode, responseBody, responseTime, success, errMsg,
	)

	if err != nil {
		log.Printf("Failed to record webhook attempt: %v", err)
	}
}

// generateSignature creates HMAC-SHA256 signature
func generateSignature(payload, secret []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// CallbackPayload represents the M-Pesa callback structure
type CallbackPayload struct {
	Body struct {
		StkCallback struct {
			MerchantRequestID string `json:"MerchantRequestID"`
			CheckoutRequestID string `json:"CheckoutRequestID"`
			ResultCode        int    `json:"ResultCode"`
			ResultDesc        string `json:"ResultDesc"`
			CallbackMetadata  struct {
				Item []mpesa.Item `json:"Item"`
			} `json:"CallbackMetadata"`
		} `json:"stkCallback"`
	} `json:"Body"`
}
