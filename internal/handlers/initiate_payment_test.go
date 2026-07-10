package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/mpesa-gateway/internal/middleware"
	"github.com/mpesa-gateway/internal/payment"
	"github.com/mpesa-gateway/internal/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePaymentService is a test double for PaymentInitiator so handler tests
// don't need a real database or Safaricom API.
type fakePaymentService struct {
	resp *payment.InitiatePaymentResponse
	err  error

	lastReq payment.InitiatePaymentRequest
	called  bool
}

func (f *fakePaymentService) InitiatePayment(ctx context.Context, req payment.InitiatePaymentRequest) (*payment.InitiatePaymentResponse, error) {
	f.called = true
	f.lastReq = req
	return f.resp, f.err
}

func newTestHandler(svc PaymentInitiator) *Handler {
	return &Handler{
		paymentService: svc,
		validator:      validator.New(),
	}
}

// testTenant is the tenant TenantAuth would have attached to the request
// context after a successful X-API-Key check.
var testTenant = &tenant.Tenant{ID: uuid.New(), Name: "test-tenant"}

func doInitiateRequest(h *Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/initiate", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(middleware.WithTenant(req.Context(), testTenant))
	rec := httptest.NewRecorder()
	h.InitiatePayment(rec, req)
	return rec
}

func validPayload() map[string]string {
	return map[string]string{
		"amount":          "100",
		"phone":           "254712345678",
		"webhook_url":     "https://tenant.example.com/webhook",
		"idempotency_key": uuid.New().String(),
	}
}

func toJSON(t *testing.T, m map[string]string) string {
	t.Helper()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return string(b)
}

func TestInitiatePayment_Success(t *testing.T) {
	txID := uuid.New()
	fake := &fakePaymentService{resp: &payment.InitiatePaymentResponse{TransactionID: txID, Status: "PENDING"}}
	h := newTestHandler(fake)

	rec := doInitiateRequest(h, toJSON(t, validPayload()))

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.True(t, fake.called)
	assert.Equal(t, testTenant.ID, fake.lastReq.TenantID, "tenant from request context should flow into the payment request")

	var body payment.InitiatePaymentResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, txID, body.TransactionID)
	assert.Equal(t, "PENDING", body.Status)
}

func TestInitiatePayment_MissingTenantContextReturns500(t *testing.T) {
	fake := &fakePaymentService{}
	h := newTestHandler(fake)

	req := httptest.NewRequest(http.MethodPost, "/initiate", bytes.NewBufferString(toJSON(t, validPayload())))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.InitiatePayment(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.False(t, fake.called, "payment service should not be invoked without an authenticated tenant")
}

func TestInitiatePayment_DuplicateIdempotencyKeyReturns409(t *testing.T) {
	fake := &fakePaymentService{err: fmt.Errorf("failed to insert transaction: duplicate idempotency key: %w", errors.New("db error"))}
	h := newTestHandler(fake)

	rec := doInitiateRequest(h, toJSON(t, validPayload()))

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestInitiatePayment_DownstreamErrorReturns500(t *testing.T) {
	fake := &fakePaymentService{err: errors.New("STK Push failed: safaricom timeout")}
	h := newTestHandler(fake)

	rec := doInitiateRequest(h, toJSON(t, validPayload()))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestInitiatePayment_MalformedJSON(t *testing.T) {
	h := newTestHandler(&fakePaymentService{})

	rec := doInitiateRequest(h, `{"amount": "100",`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestInitiatePayment_ValidationFailures(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(m map[string]string)
	}{
		{"missing amount", func(m map[string]string) { delete(m, "amount") }},
		{"non-numeric amount", func(m map[string]string) { m["amount"] = "abc" }},
		{"zero amount", func(m map[string]string) { m["amount"] = "0" }},
		{"negative amount", func(m map[string]string) { m["amount"] = "-5" }},
		{"phone too short", func(m map[string]string) { m["phone"] = "25471234" }},
		{"phone non-numeric", func(m map[string]string) { m["phone"] = "25471234abcd" }},
		{"missing webhook_url", func(m map[string]string) { delete(m, "webhook_url") }},
		{"invalid webhook_url", func(m map[string]string) { m["webhook_url"] = "not-a-url" }},
		{"missing idempotency_key", func(m map[string]string) { delete(m, "idempotency_key") }},
		{"non-uuid4 idempotency_key", func(m map[string]string) { m["idempotency_key"] = "not-a-uuid" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakePaymentService{}
			h := newTestHandler(fake)

			payload := validPayload()
			tt.mutate(payload)

			rec := doInitiateRequest(h, toJSON(t, payload))

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.False(t, fake.called, "payment service should not be invoked when validation fails")
		})
	}
}
