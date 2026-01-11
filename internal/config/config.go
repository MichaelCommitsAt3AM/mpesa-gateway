package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration
type Config struct {
	// Server settings
	ServerPort string

	// Database configuration
	DatabaseURL string
	DBMaxConns  int
	DBMinConns  int

	// Redis configuration
	RedisURL string

	// Safaricom API credentials
	SafaricomConsumerKey    string
	SafaricomConsumerSecret string
	SafaricomPasskey        string
	SafaricomShortCode      string
	SafaricomAuthURL        string
	SafaricomSTKPushURL     string
	SafaricomCallbackURL    string

	// Security settings
	InternalSecret string
	SafaricomIPs   []string

	// Request limits
	MaxRequestSize int64

	// Worker settings
	WorkerConcurrency int
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		// Server
		ServerPort: getEnv("MPESA_SERVER_PORT", "8080"),

		// Database
		DatabaseURL: getEnv("MPESA_DATABASE_URL", ""),
		DBMaxConns:  getEnvInt("MPESA_DB_MAX_CONNS", 25),
		DBMinConns:  getEnvInt("MPESA_DB_MIN_CONNS", 5),

		// Redis
		RedisURL: getEnv("MPESA_REDIS_URL", ""),

		// Safaricom
		SafaricomConsumerKey:    getEnv("MPESA_SAFARICOM_CONSUMER_KEY", ""),
		SafaricomConsumerSecret: getEnv("MPESA_SAFARICOM_CONSUMER_SECRET", ""),
		SafaricomPasskey:        getEnv("MPESA_SAFARICOM_PASSKEY", ""),
		SafaricomShortCode:      getEnv("MPESA_SAFARICOM_SHORT_CODE", ""),
		SafaricomAuthURL:        getEnv("MPESA_SAFARICOM_AUTH_URL", "https://sandbox.safaricom.co.ke/oauth/v1/generate?grant_type=client_credentials"),
		SafaricomSTKPushURL:     getEnv("MPESA_SAFARICOM_STK_PUSH_URL", "https://sandbox.safaricom.co.ke/mpesa/stkpush/v1/processrequest"),
		SafaricomCallbackURL:    getEnv("MPESA_SAFARICOM_CALLBACK_URL", ""),

		// Security
		InternalSecret: getEnv("MPESA_INTERNAL_SECRET", ""),
		MaxRequestSize: getEnvInt64("MPESA_MAX_REQUEST_SIZE", 1<<20), // 1MB

		// Worker
		WorkerConcurrency: getEnvInt("MPESA_WORKER_CONCURRENCY", 10),
	}

	// Parse IP allowlist
	ipList := getEnv("MPESA_SAFARICOM_IPS", "")
	if ipList != "" {
		cfg.SafaricomIPs = strings.Split(ipList, ",")
		for i := range cfg.SafaricomIPs {
			cfg.SafaricomIPs[i] = strings.TrimSpace(cfg.SafaricomIPs[i])
		}
	}

	// Validation
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate ensures all required configuration is present
func (c *Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("MPESA_DATABASE_URL is required")
	}
	if c.RedisURL == "" {
		return fmt.Errorf("MPESA_REDIS_URL is required")
	}
	if c.InternalSecret == "" {
		return fmt.Errorf("MPESA_INTERNAL_SECRET is required")
	}
	if c.SafaricomConsumerKey == "" {
		return fmt.Errorf("MPESA_SAFARICOM_CONSUMER_KEY is required")
	}
	if c.SafaricomConsumerSecret == "" {
		return fmt.Errorf("MPESA_SAFARICOM_CONSUMER_SECRET is required")
	}
	if c.SafaricomPasskey == "" {
		return fmt.Errorf("MPESA_SAFARICOM_PASSKEY is required")
	}
	if c.SafaricomShortCode == "" {
		return fmt.Errorf("MPESA_SAFARICOM_SHORT_CODE is required")
	}
	if c.SafaricomCallbackURL == "" {
		return fmt.Errorf("MPESA_SAFARICOM_CALLBACK_URL is required (public URL for callbacks)")
	}

	return nil
}

// LogSafeConfig logs configuration without secrets
func (c *Config) LogSafeConfig() {
	fmt.Printf("Configuration loaded:\n")
	fmt.Printf("  Server Port: %s\n", c.ServerPort)
	fmt.Printf("  Database URL: %s\n", maskConnectionString(c.DatabaseURL))
	fmt.Printf("  Redis URL: %s\n", maskConnectionString(c.RedisURL))
	fmt.Printf("  DB Pool: %d min, %d max\n", c.DBMinConns, c.DBMaxConns)
	fmt.Printf("  Worker Concurrency: %d\n", c.WorkerConcurrency)
	fmt.Printf("  Safaricom Short Code: %s\n", c.SafaricomShortCode)
	fmt.Printf("  Safaricom IP Allowlist: %v\n", c.SafaricomIPs)
	fmt.Printf("  Max Request Size: %d bytes\n", c.MaxRequestSize)
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func maskConnectionString(connStr string) string {
	if strings.Contains(connStr, "@") {
		parts := strings.Split(connStr, "@")
		if len(parts) == 2 {
			return "***@" + parts[1]
		}
	}
	return "***"
}
