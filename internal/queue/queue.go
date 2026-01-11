package queue

import (
	"log"

	"github.com/hibiken/asynq"
)

// Queue wraps Asynq client and server
type Queue struct {
	Client *asynq.Client
	Server *asynq.ServeMux
}

// NewQueue creates a new queue client and server
func NewQueue(redisURL string, concurrency int) (*Queue, error) {
	redisOpt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, err
	}

	// Create client for enqueueing tasks
	client := asynq.NewClient(redisOpt)

	// Create server mux for registering handlers
	serverMux := asynq.NewServeMux()

	log.Printf("Queue client and server initialized (concurrency: %d)", concurrency)

	return &Queue{
		Client: client,
		Server: serverMux,
	}, nil
}

// GetServerConfig returns server configuration for worker
func (q *Queue) GetServerConfig(redisURL string, concurrency int) (*asynq.Config, error) {
	redisOpt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, err
	}

	return &asynq.Config{
		Concurrency: concurrency,
		Queues: map[string]int{
			"critical": 6,
			"default":  3,
			"low":      1,
		},
		RedisConnOpt: redisOpt,
	}, nil
}

// Close gracefully closes the queue client
func (q *Queue) Close() error {
	if q.Client != nil {
		log.Println("Closing queue client...")
		return q.Client.Close()
	}
	return nil
}
