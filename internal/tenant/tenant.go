// Package tenant manages registered API consumers of the gateway: their
// API key (used to authenticate /initiate) and their webhook signing
// secret (used to HMAC-sign outgoing webhooks).
package tenant

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrTenantNotFound is returned when no tenant matches the given API key or ID.
var ErrTenantNotFound = errors.New("tenant not found")

// Tenant is a registered API consumer of the gateway.
type Tenant struct {
	ID                   uuid.UUID
	Name                 string
	WebhookSigningSecret string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Store provides tenant persistence and lookup.
type Store struct {
	db *pgxpool.Pool
}

// NewStore creates a new tenant Store.
func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// Create provisions a new tenant with a freshly generated API key and
// webhook signing secret. The raw API key is only ever available here, at
// creation time; only its hash is persisted.
func (s *Store) Create(ctx context.Context, name string) (t *Tenant, rawAPIKey string, err error) {
	rawAPIKey, err = generateSecret("mpg_")
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate API key: %w", err)
	}

	webhookSigningSecret, err := generateSecret("")
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate webhook signing secret: %w", err)
	}

	apiKeyHash := hashAPIKey(rawAPIKey)

	const insertSQL = `
		INSERT INTO tenants (name, api_key_hash, webhook_signing_secret)
		VALUES ($1, $2, $3)
		RETURNING id, name, webhook_signing_secret, created_at, updated_at
	`

	var tenant Tenant
	err = s.db.QueryRow(ctx, insertSQL, name, apiKeyHash, webhookSigningSecret).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.WebhookSigningSecret,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to insert tenant: %w", err)
	}

	return &tenant, rawAPIKey, nil
}

// Authenticate looks up the tenant owning the given raw API key.
func (s *Store) Authenticate(ctx context.Context, rawAPIKey string) (*Tenant, error) {
	const query = `
		SELECT id, name, webhook_signing_secret, created_at, updated_at
		FROM tenants
		WHERE api_key_hash = $1
	`

	var tenant Tenant
	err := s.db.QueryRow(ctx, query, hashAPIKey(rawAPIKey)).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.WebhookSigningSecret,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTenantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate tenant: %w", err)
	}

	return &tenant, nil
}

// GetByID fetches a tenant by ID.
func (s *Store) GetByID(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	const query = `
		SELECT id, name, webhook_signing_secret, created_at, updated_at
		FROM tenants
		WHERE id = $1
	`

	var tenant Tenant
	err := s.db.QueryRow(ctx, query, id).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.WebhookSigningSecret,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTenantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tenant: %w", err)
	}

	return &tenant, nil
}

// generateSecret returns a random 32-byte value, hex-encoded and prefixed.
func generateSecret(prefix string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(buf), nil
}

// hashAPIKey returns the SHA-256 hex digest of an API key. API keys are
// high-entropy random tokens, not passwords, so a fast one-way hash is
// sufficient (the same approach GitHub and Stripe use for API tokens).
func hashAPIKey(rawAPIKey string) string {
	sum := sha256.Sum256([]byte(rawAPIKey))
	return hex.EncodeToString(sum[:])
}
