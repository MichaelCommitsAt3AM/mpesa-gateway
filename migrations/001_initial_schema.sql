-- M-Pesa Payment Gateway - Database Schema
-- PostgreSQL 13+

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Transactions table: Core payment transaction records
CREATE TABLE transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    internal_transaction_id UUID UNIQUE NOT NULL,
    idempotency_key UUID UNIQUE NOT NULL,
    
    -- Safaricom identifiers
    checkout_request_id VARCHAR(100),
    merchant_request_id VARCHAR(100),
    
    -- Transaction details
    amount DECIMAL(20,2) NOT NULL CHECK (amount > 0),
    phone VARCHAR(12) NOT NULL CHECK (phone ~ '^254[0-9]{9}$'),
    
    -- Status management
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' 
        CHECK (status IN ('PENDING', 'COMPLETED', 'FAILED')),
    
    -- Metadata from M-Pesa callback
    mpesa_metadata JSONB,
    
    -- Tenant webhook configuration
    tenant_webhook_url TEXT NOT NULL,
    
    -- Error tracking
    error_message TEXT,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

-- Webhook delivery attempts audit trail
CREATE TABLE webhook_attempts (
    id BIGSERIAL PRIMARY KEY,
    transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    
    -- Attempt metadata
    attempt_number INTEGER NOT NULL CHECK (attempt_number > 0),
    webhook_url TEXT NOT NULL,
    
    -- Request/Response details
    request_payload JSONB NOT NULL,
    response_status_code INTEGER,
    response_body TEXT,
    response_time_ms INTEGER,
    
    -- Result
    success BOOLEAN NOT NULL DEFAULT FALSE,
    error_message TEXT,
    
    -- Timestamp
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Performance Indexes
CREATE INDEX idx_transactions_checkout_request 
    ON transactions(checkout_request_id) 
    WHERE checkout_request_id IS NOT NULL;

CREATE INDEX idx_transactions_status 
    ON transactions(status) 
    WHERE status = 'PENDING';

CREATE INDEX idx_transactions_created_at 
    ON transactions(created_at DESC);

CREATE INDEX idx_webhook_attempts_transaction 
    ON webhook_attempts(transaction_id, attempted_at DESC);

-- Updated timestamp trigger
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_transactions_updated_at 
    BEFORE UPDATE ON transactions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Comments for documentation
COMMENT ON TABLE transactions IS 'Core M-Pesa payment transaction records';
COMMENT ON COLUMN transactions.internal_transaction_id IS 'Our internal UUID for transaction tracking';
COMMENT ON COLUMN transactions.idempotency_key IS 'Client-provided idempotency key to prevent duplicate requests';
COMMENT ON COLUMN transactions.checkout_request_id IS 'Safaricom STK Push checkout request identifier';
COMMENT ON COLUMN transactions.amount IS 'Transaction amount in KES (Kenyan Shillings)';
COMMENT ON COLUMN transactions.phone IS 'Customer phone number in format 254XXXXXXXXX';
COMMENT ON COLUMN transactions.status IS 'Transaction state: PENDING, COMPLETED, or FAILED';
COMMENT ON COLUMN transactions.mpesa_metadata IS 'Parsed M-Pesa callback metadata as JSON';
COMMENT ON COLUMN transactions.tenant_webhook_url IS 'URL to POST transaction result to';

COMMENT ON TABLE webhook_attempts IS 'Audit trail for webhook delivery attempts';
COMMENT ON COLUMN webhook_attempts.attempt_number IS 'Sequential attempt number (1-based)';
COMMENT ON COLUMN webhook_attempts.response_time_ms IS 'Webhook response time in milliseconds';
