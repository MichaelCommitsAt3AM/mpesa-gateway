// Package testutil provides shared test infrastructure for spinning up
// real dependencies (Postgres) in integration tests via testcontainers.
package testutil

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// migrationsDir resolves the migrations/ directory relative to this source
// file, so it works regardless of which package invokes the helper.
func migrationsDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
}

// migrationFiles returns every *.sql file in migrations/, sorted so they're
// applied in the same order Postgres's docker-entrypoint-initdb.d would use.
func migrationFiles(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(migrationsDir())
	if err != nil {
		t.Fatalf("failed to read migrations directory: %v", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" {
			files = append(files, filepath.Join(migrationsDir(), e.Name()))
		}
	}
	sort.Strings(files)

	return files
}

// NewPostgres starts a throwaway Postgres container with the project schema
// applied and returns a connected pool. The container and pool are torn down
// automatically via t.Cleanup.
func NewPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping testcontainers-based test in -short mode")
	}

	migrations := migrationFiles(t)

	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:15-alpine",
		tcpostgres.WithDatabase("mpesa_gateway_test"),
		tcpostgres.WithUsername("mpesa"),
		tcpostgres.WithPassword("mpesa_test_password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("failed to terminate postgres container: %v", err)
		}
	})

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("failed to connect to test postgres: %v", err)
	}
	t.Cleanup(pool.Close)

	for _, path := range migrations {
		schema, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read migration file %s: %v", path, err)
		}
		if _, err := pool.Exec(ctx, string(schema)); err != nil {
			t.Fatalf("failed to apply migration %s: %v", path, err)
		}
	}

	return pool
}
