package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps the connection pool
type DB struct {
	Pool *pgxpool.Pool
}

// NewDatabase creates a new database connection pool
func NewDatabase(ctx context.Context, databaseURL string, minConns, maxConns int) (*DB, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse database URL: %w", err)
	}

	// Configure connection pool
	config.MinConns = int32(minConns)
	config.MaxConns = int32(maxConns)
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute
	config.HealthCheckPeriod = time.Minute

	// Create pool
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	log.Printf("Database connection pool established (min: %d, max: %d)", minConns, maxConns)

	return &DB{Pool: pool}, nil
}

// Close gracefully closes the database connection pool
func (db *DB) Close() {
	if db.Pool != nil {
		log.Println("Closing database connection pool...")
		db.Pool.Close()
	}
}

// Health checks database connectivity
func (db *DB) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return db.Pool.Ping(ctx)
}
