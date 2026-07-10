package tenant_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/mpesa-gateway/internal/tenant"
	"github.com/mpesa-gateway/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_CreateAndAuthenticate(t *testing.T) {
	pool := testutil.NewPostgres(t)
	store := tenant.NewStore(pool)
	ctx := context.Background()

	created, apiKey, err := store.Create(ctx, "Acme Corp")
	require.NoError(t, err)
	require.NotEmpty(t, apiKey)
	require.NotEmpty(t, created.WebhookSigningSecret)

	authenticated, err := store.Authenticate(ctx, apiKey)
	require.NoError(t, err)
	assert.Equal(t, created.ID, authenticated.ID)
	assert.Equal(t, created.WebhookSigningSecret, authenticated.WebhookSigningSecret)
}

func TestStore_Authenticate_WrongKeyFails(t *testing.T) {
	pool := testutil.NewPostgres(t)
	store := tenant.NewStore(pool)
	ctx := context.Background()

	_, _, err := store.Create(ctx, "Acme Corp")
	require.NoError(t, err)

	_, err = store.Authenticate(ctx, "not-the-right-key")
	assert.ErrorIs(t, err, tenant.ErrTenantNotFound)
}

func TestStore_Authenticate_EmptyKeyFails(t *testing.T) {
	pool := testutil.NewPostgres(t)
	store := tenant.NewStore(pool)
	ctx := context.Background()

	_, err := store.Authenticate(ctx, "")
	assert.ErrorIs(t, err, tenant.ErrTenantNotFound)
}

func TestStore_Create_DoesNotPersistRawAPIKey(t *testing.T) {
	pool := testutil.NewPostgres(t)
	store := tenant.NewStore(pool)
	ctx := context.Background()

	_, apiKey, err := store.Create(ctx, "Acme Corp")
	require.NoError(t, err)

	var storedHash string
	err = pool.QueryRow(ctx, "SELECT api_key_hash FROM tenants WHERE name = $1", "Acme Corp").Scan(&storedHash)
	require.NoError(t, err)

	assert.NotEqual(t, apiKey, storedHash, "the raw API key must never be stored directly")
	assert.NotContains(t, storedHash, apiKey)
}

func TestStore_GetByID(t *testing.T) {
	pool := testutil.NewPostgres(t)
	store := tenant.NewStore(pool)
	ctx := context.Background()

	created, _, err := store.Create(ctx, "Acme Corp")
	require.NoError(t, err)

	fetched, err := store.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.WebhookSigningSecret, fetched.WebhookSigningSecret)
}

func TestStore_GetByID_NotFound(t *testing.T) {
	pool := testutil.NewPostgres(t)
	store := tenant.NewStore(pool)
	ctx := context.Background()

	_, err := store.GetByID(ctx, uuid.Nil) // zero UUID, never assigned by uuid_generate_v4
	assert.ErrorIs(t, err, tenant.ErrTenantNotFound)
}
