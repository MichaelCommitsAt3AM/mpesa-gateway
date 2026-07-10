package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"

	"github.com/mpesa-gateway/internal/config"
	"github.com/mpesa-gateway/internal/database"
	"github.com/mpesa-gateway/internal/mpesa"
	"github.com/mpesa-gateway/internal/payment"
	"github.com/mpesa-gateway/internal/queue"
	"github.com/mpesa-gateway/internal/server"
	"github.com/mpesa-gateway/internal/handlers"
	"github.com/mpesa-gateway/internal/tenant"
	"github.com/mpesa-gateway/internal/worker"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("M-Pesa Payment Gateway starting...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	cfg.LogSafeConfig()

	// Create context
	ctx := context.Background()

	// Initialize database
	db, err := database.NewDatabase(ctx, cfg.DatabaseURL, cfg.DBMinConns, cfg.DBMaxConns)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Initialize queue
	q, err := queue.NewQueue(cfg.RedisURL, cfg.WorkerConcurrency)
	if err != nil {
		log.Fatalf("Failed to initialize queue: %v", err)
	}
	defer q.Close()

	// Initialize token service
	tokenService := mpesa.NewTokenService(
		cfg.SafaricomConsumerKey,
		cfg.SafaricomConsumerSecret,
		cfg.SafaricomAuthURL,
	)

	// Initialize payment service
	paymentService := payment.NewService(
		db.Pool,
		tokenService,
		payment.PaymentConfig{
			ShortCode:   cfg.SafaricomShortCode,
			Passkey:     cfg.SafaricomPasskey,
			STKPushURL:  cfg.SafaricomSTKPushURL,
			CallbackURL: cfg.SafaricomCallbackURL,
		},
	)

	// Initialize tenant store
	tenantStore := tenant.NewStore(db.Pool)

	// Initialize HTTP handlers
	httpHandlers := handlers.NewHandler(db.Pool, paymentService, q.Client)

	// Initialize worker processor
	processor := worker.NewProcessor(db.Pool, tenantStore)

	// Register worker handlers
	q.Server.HandleFunc(worker.TypeProcessCallback, processor.ProcessCallback)

	// Start Asynq worker in background
	redisOpt, serverConfig, err := q.GetServerConfig(cfg.RedisURL, cfg.WorkerConcurrency)
	if err != nil {
		log.Fatalf("Failed to create worker config: %v", err)
	}

	asynqServer := asynq.NewServer(
		redisOpt,
		*serverConfig,
	)

	go func() {
		log.Println("Starting Asynq worker...")
		if err := asynqServer.Run(q.Server); err != nil {
			log.Fatalf("Asynq worker failed: %v", err)
		}
	}()

	// Initialize HTTP server
	httpServer := server.NewServer(cfg, httpHandlers, tenantStore)

	// Start HTTP server in background
	go func() {
		if err := httpServer.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down gracefully...")

	// Stop accepting new HTTP connections and drain in-flight requests
	// before shutting down the worker that processes their side effects.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server did not shut down cleanly: %v", err)
	}

	// Shutdown Asynq worker
	asynqServer.Shutdown()

	log.Println("Shutdown complete")
}
