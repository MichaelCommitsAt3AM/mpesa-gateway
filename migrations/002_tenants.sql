-- M-Pesa Payment Gateway - Tenant identity and per-tenant webhook secrets
-- PostgreSQL 13+

-- Tenants table: registered API consumers of the gateway
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL UNIQUE,
    api_key_hash TEXT NOT NULL UNIQUE,
    webhook_signing_secret TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE transactions ADD COLUMN tenant_id UUID NOT NULL REFERENCES tenants(id);
CREATE INDEX idx_transactions_tenant_id ON transactions(tenant_id);

-- Updated timestamp trigger (reuses the function defined in 001_initial_schema.sql)
CREATE TRIGGER update_tenants_updated_at
    BEFORE UPDATE ON tenants
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Comments for documentation
COMMENT ON TABLE tenants IS 'Registered API consumers of the gateway';
COMMENT ON COLUMN tenants.api_key_hash IS 'SHA-256 hex digest of the tenant API key; the raw key is only ever shown once, at creation';
COMMENT ON COLUMN tenants.webhook_signing_secret IS 'Per-tenant secret used to HMAC-sign outgoing webhooks; provisioned out-of-band, never returned by any API response';
COMMENT ON COLUMN transactions.tenant_id IS 'Tenant this transaction belongs to; determines which webhook_signing_secret is used to sign the outgoing webhook';
