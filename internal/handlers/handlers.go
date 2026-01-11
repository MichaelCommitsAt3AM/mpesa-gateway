package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mpesa-gateway/internal/payment"
	"github.com/mpesa-gateway/internal/worker"
	"github.com/shopspring/decimal"
)

// Handler holds dependencies for HTTP handlers
type Handler struct {
	db             *pgxpool.Pool
	paymentService *payment.Service
	queueClient    *asynq.Client
	validator      *validator.Validate
}

// NewHandler creates a new handler instance
func NewHandler(db *pgxpool.Pool, paymentService *payment.Service, queueClient *asynq.Client) *Handler {
	return &Handler{
		db:             db,
		paymentService: paymentService,
		queueClient:    queueClient,
		validator:      validator.New(),
	}
}

// InitiatePaymentRequest represents the /initiate request
type InitiatePaymentRequest struct {
	Amount         string `json:"amount" validate:"required,numeric"`
	Phone          string `json:"phone" validate:"required,len=12,numeric"`
	WebhookURL     string `json:"webhook_url" validate:"required,url"`
	IdempotencyKey string `json:"idempotency_key" validate:"required,uuid4"`
}

// InitiatePayment handles POST /initiate
func (h *Handler) InitiatePayment(w http.ResponseWriter, r *http.Request) {
	var req InitiatePaymentRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	// Validate request
	if err := h.validator.Struct(req); err != nil {
		respondError(w, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	// Parse amount
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid amount format")
		return
	}

	if amount.LessThanOrEqual(decimal.Zero) {
		respondError(w, http.StatusBadRequest, "Amount must be greater than zero")
		return
	}

	// Parse idempotency key
	idempotencyKey, err := uuid.Parse(req.IdempotencyKey)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid idempotency key")
		return
	}

	// Call payment service
	paymentReq := payment.InitiatePaymentRequest{
		Amount:         amount,
		Phone:          req.Phone,
		WebhookURL:     req.WebhookURL,
		IdempotencyKey: idempotencyKey,
	}

	resp, err := h.paymentService.InitiatePayment(r.Context(), paymentReq)
	if err != nil {
		log.Printf("Payment initiation failed: %v", err)

		// Check for idempotency conflict
		if contains(err.Error(), "duplicate idempotency key") {
			respondError(w, http.StatusConflict, "Duplicate request")
			return
		}

		respondError(w, http.StatusInternalServerError, "Failed to initiate payment")
		return
	}

	respondJSON(w, http.StatusCreated, resp)
}

// MPesaCallback handles POST /callback (non-blocking)
func (h *Handler) MPesaCallback(w http.ResponseWriter, r *http.Request) {
	// Read raw body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read callback body: %v", err)
		respondError(w, http.StatusBadRequest, "Failed to read request")
		return
	}

	// Minimal validation: ensure it's valid JSON
	var rawPayload map[string]interface{}
	if err := json.Unmarshal(body, &rawPayload); err != nil {
		log.Printf("Invalid JSON in callback: %v", err)
		respondError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Enqueue task for background processing
	task, err := worker.NewProcessCallbackTask(body)
	if err != nil {
		log.Printf("Failed to create task: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to queue callback")
		return
	}

	info, err := h.queueClient.Enqueue(task, asynq.Queue("default"), asynq.MaxRetry(3))
	if err != nil {
		log.Printf("Failed to enqueue task: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to queue callback")
		return
	}

	log.Printf("Callback queued: task_id=%s", info.ID)

	// Immediately return 200 OK to Safaricom
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"received"}`))
}

// HealthCheck handles GET /health
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	health := map[string]string{
		"status": "ok",
	}

	// Check database
	if err := h.db.Ping(ctx); err != nil {
		health["database"] = "down"
		health["status"] = "degraded"
	} else {
		health["database"] = "up"
	}

	// Note: Queue health check would require Redis ping, omitted for simplicity

	status := http.StatusOK
	if health["status"] != "ok" {
		status = http.StatusServiceUnavailable
	}

	respondJSON(w, status, health)
}

// respondJSON writes a JSON response
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError writes an error response
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
