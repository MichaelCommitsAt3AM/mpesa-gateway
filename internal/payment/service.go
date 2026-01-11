package payment

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mpesa-gateway/internal/models"
	"github.com/mpesa-gateway/internal/mpesa"
	"github.com/shopspring/decimal"
)

// Service handles payment operations
type Service struct {
	db           *pgxpool.Pool
	tokenService *mpesa.TokenService
	cfg          PaymentConfig
	client       *http.Client
}

// PaymentConfig holds Safaricom API configuration
type PaymentConfig struct {
	ShortCode   string
	Passkey     string
	STKPushURL  string
	CallbackURL string
}

// NewService creates a new payment service
func NewService(db *pgxpool.Pool, tokenService *mpesa.TokenService, cfg PaymentConfig) *Service {
	return &Service{
		db:           db,
		tokenService: tokenService,
		cfg:          cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
		},
	}
}

// InitiatePaymentRequest represents the payment initiation request
type InitiatePaymentRequest struct {
	Amount         decimal.Decimal `validate:"required"`
	Phone          string          `validate:"required,len=12,numeric"`
	WebhookURL     string          `validate:"required,url"`
	IdempotencyKey uuid.UUID       `validate:"required"`
}

// InitiatePaymentResponse represents the payment initiation response
type InitiatePaymentResponse struct {
	TransactionID uuid.UUID `json:"transaction_id"`
	Status        string    `json:"status"`
}

// STKPushRequest represents Safaricom STK Push API request
type STKPushRequest struct {
	BusinessShortCode string `json:"BusinessShortCode"`
	Password          string `json:"Password"`
	Timestamp         string `json:"Timestamp"`
	TransactionType   string `json:"TransactionType"`
	Amount            string `json:"Amount"`
	PartyA            string `json:"PartyA"`
	PartyB            string `json:"PartyB"`
	PhoneNumber       string `json:"PhoneNumber"`
	CallBackURL       string `json:"CallBackURL"`
	AccountReference  string `json:"AccountReference"`
	TransactionDesc   string `json:"TransactionDesc"`
}

// STKPushResponse represents Safaricom STK Push API response
type STKPushResponse struct {
	MerchantRequestID   string `json:"MerchantRequestID"`
	CheckoutRequestID   string `json:"CheckoutRequestID"`
	ResponseCode        string `json:"ResponseCode"`
	ResponseDescription string `json:"ResponseDescription"`
	CustomerMessage     string `json:"CustomerMessage"`
}

// InitiatePayment initiates an STK Push payment
func (s *Service) InitiatePayment(ctx context.Context, req InitiatePaymentRequest) (*InitiatePaymentResponse, error) {
	// Generate internal transaction ID
	internalTxID := uuid.New()

	// Start database transaction
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Insert initial transaction record
	insertSQL := `
		INSERT INTO transactions (
			internal_transaction_id, 
			idempotency_key, 
			amount, 
			phone, 
			status, 
			tenant_webhook_url
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	var txID uuid.UUID
	err = tx.QueryRow(ctx, insertSQL,
		internalTxID,
		req.IdempotencyKey,
		req.Amount,
		req.Phone,
		models.StatusPending,
		req.WebhookURL,
	).Scan(&txID)

	if err != nil {
		// Check for unique constraint violation (idempotency)
		// In pgx v5, errors are in pgconn package
		if err.Error() == "ERROR: duplicate key value violates unique constraint \"transactions_idempotency_key_key\" (SQLSTATE 23505)" ||
			strings.Contains(err.Error(), "23505") {
			return nil, fmt.Errorf("duplicate idempotency key: %w", err)
		}
		return nil, fmt.Errorf("failed to insert transaction: %w", err)
	}

	// Call Safaricom STK Push API
	checkoutRequestID, merchantRequestID, err := s.callSTKPush(ctx, req.Phone, req.Amount, internalTxID.String())
	if err != nil {
		// Update transaction with error
		updateErrSQL := `UPDATE transactions SET error_message = $1 WHERE id = $2`
		tx.Exec(ctx, updateErrSQL, err.Error(), txID)
		tx.Commit(ctx)
		return nil, fmt.Errorf("STK Push failed: %w", err)
	}

	// Update transaction with Safaricom IDs
	updateSQL := `
		UPDATE transactions 
		SET checkout_request_id = $1, merchant_request_id = $2 
		WHERE id = $3
	`
	_, err = tx.Exec(ctx, updateSQL, checkoutRequestID, merchantRequestID, txID)
	if err != nil {
		return nil, fmt.Errorf("failed to update transaction with checkout ID: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &InitiatePaymentResponse{
		TransactionID: internalTxID,
		Status:        string(models.StatusPending),
	}, nil
}

// callSTKPush calls Safaricom's STK Push API
func (s *Service) callSTKPush(ctx context.Context, phone string, amount decimal.Decimal, reference string) (string, string, error) {
	// Get access token
	token, err := s.tokenService.GetToken(ctx)
	if err != nil {
		return "", "", fmt.Errorf("failed to get access token: %w", err)
	}

	// Generate timestamp and password
	timestamp := time.Now().Format("20060102150405")
	password := base64.StdEncoding.EncodeToString(
		[]byte(s.cfg.ShortCode + s.cfg.Passkey + timestamp),
	)

	// Build request
	stkReq := STKPushRequest{
		BusinessShortCode: s.cfg.ShortCode,
		Password:          password,
		Timestamp:         timestamp,
		TransactionType:   "CustomerPayBillOnline",
		Amount:            amount.StringFixed(0), // No decimals for Safaricom
		PartyA:            phone,
		PartyB:            s.cfg.ShortCode,
		PhoneNumber:       phone,
		CallBackURL:       s.cfg.CallbackURL,
		AccountReference:  reference,
		TransactionDesc:   "Payment",
	}

	body, err := json.Marshal(stkReq)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal STK request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.STKPushURL, bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to send STK Push: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("STK Push failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var stkResp STKPushResponse
	if err := json.Unmarshal(respBody, &stkResp); err != nil {
		return "", "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if stkResp.ResponseCode != "0" {
		return "", "", fmt.Errorf("STK Push error: %s", stkResp.ResponseDescription)
	}

	return stkResp.CheckoutRequestID, stkResp.MerchantRequestID, nil
}
