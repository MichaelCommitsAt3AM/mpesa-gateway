package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"

	"mpesa-gateway/internal/config"
	"mpesa-gateway/internal/database"
	"mpesa-gateway/internal/queue"
	"mpesa-gateway/internal/worker"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("M-Pesa Payment Gateway Worker starting...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

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

	// Initialize worker processor
	processor := worker.NewProcessor(db.Pool)

	// Register worker handlers
	q.Server.HandleFunc(worker.TypeProcessCallback, processor.ProcessCallback)

	// Start Asynq worker
	serverConfig, err := q.GetServerConfig(cfg.RedisURL, cfg.WorkerConcurrency)
	if err != nil {
		log.Fatalf("Failed to create worker config: %v", err)
	}

	asynqServer := asynq.NewServer(
		serverConfig.RedisConnOpt,
		asynq.Config{
			Concurrency: serverConfig.Concurrency,
			Queues:      serverConfig.Queues,
		},
	)

	// Handle shutdown signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("Shutting down worker...")
		asynqServer.Shutdown()
	}()

	log.Println("Worker started, processing tasks...")
	if err := asynqServer.Run(q.Server); err != nil {
		log.Fatalf("Worker failed: %v", err)
	}

	log.Println("Worker shutdown complete")
}
