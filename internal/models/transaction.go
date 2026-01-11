package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Transaction represents a payment transaction record
type Transaction struct {
	ID                    uuid.UUID       `db:"id"`
	InternalTransactionID uuid.UUID       `db:"internal_transaction_id"`
	IdempotencyKey        uuid.UUID       `db:"idempotency_key"`
	CheckoutRequestID     *string         `db:"checkout_request_id"`
	MerchantRequestID     *string         `db:"merchant_request_id"`
	Amount                decimal.Decimal `db:"amount"`
	Phone                 string          `db:"phone"`
	Status                string          `db:"status"`
	MpesaMetadata         []byte          `db:"mpesa_metadata"` // JSONB
	TenantWebhookURL      string          `db:"tenant_webhook_url"`
	ErrorMessage          *string         `db:"error_message"`
	CreatedAt             time.Time       `db:"created_at"`
	UpdatedAt             time.Time       `db:"updated_at"`
	CompletedAt           *time.Time      `db:"completed_at"`
}

// TransactionStatus represents valid transaction states
type TransactionStatus string

const (
	StatusPending   TransactionStatus = "PENDING"
	StatusCompleted TransactionStatus = "COMPLETED"
	StatusFailed    TransactionStatus = "FAILED"
)

// IsValidTransition checks if a status transition is allowed
func IsValidTransition(from, to TransactionStatus) bool {
	validTransitions := map[TransactionStatus][]TransactionStatus{
		StatusPending: {StatusCompleted, StatusFailed},
		// No transitions allowed from terminal states
		StatusCompleted: {},
		StatusFailed:    {},
	}

	allowed, exists := validTransitions[from]
	if !exists {
		return false
	}

	for _, validTo := range allowed {
		if validTo == to {
			return true
		}
	}

	return false
}
