package mpesa

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// TokenService manages Safaricom OAuth tokens with thread-safe access
type TokenService struct {
	consumerKey    string
	consumerSecret string
	authURL        string
	client         *http.Client

	mu          sync.RWMutex
	token       string
	expiresAt   time.Time
	refreshOnce sync.Once
}

// TokenResponse represents Safaricom OAuth response
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   string `json:"expires_in"` // Duration in seconds as string
}

// NewTokenService creates a new token service with SSL verification enforced
func NewTokenService(consumerKey, consumerSecret, authURL string) *TokenService {
	return &TokenService{
		consumerKey:    consumerKey,
		consumerSecret: consumerSecret,
		authURL:        authURL,
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
					// InsecureSkipVerify: false (default, enforced SSL verification)
				},
			},
		},
	}
}

// GetToken returns a valid access token, refreshing if necessary
func (ts *TokenService) GetToken(ctx context.Context) (string, error) {
	// Fast path: check if current token is valid (read lock)
	ts.mu.RLock()
	if time.Now().Before(ts.expiresAt) && ts.token != "" {
		token := ts.token
		ts.mu.RUnlock()
		return token, nil
	}
	ts.mu.RUnlock()

	// Slow path: token expired or missing, need to refresh
	return ts.refreshTokenSafe(ctx)
}

// refreshTokenSafe ensures only one goroutine refreshes the token at a time
func (ts *TokenService) refreshTokenSafe(ctx context.Context) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have refreshed)
	if time.Now().Before(ts.expiresAt) && ts.token != "" {
		return ts.token, nil
	}

	// Perform actual refresh
	if err := ts.refreshToken(ctx); err != nil {
		return "", err
	}

	return ts.token, nil
}

// refreshToken fetches a new token from Safaricom (caller must hold write lock)
func (ts *TokenService) refreshToken(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.authURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create auth request: %w", err)
	}

	// Set Basic Auth header
	auth := base64.StdEncoding.EncodeToString(
		[]byte(ts.consumerKey + ":" + ts.consumerSecret),
	)
	req.Header.Set("Authorization", "Basic "+auth)

	resp, err := ts.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to request token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return fmt.Errorf("received empty access token")
	}

	// Parse expiry (Safaricom returns seconds as string, typically "3599")
	expiresIn := 3599 * time.Second // Default to ~1 hour
	if tokenResp.ExpiresIn != "" {
		var seconds int
		if _, err := fmt.Sscanf(tokenResp.ExpiresIn, "%d", &seconds); err == nil {
			expiresIn = time.Duration(seconds) * time.Second
		}
	}

	// Store token with buffer time (refresh 5 minutes before actual expiry)
	ts.token = tokenResp.AccessToken
	ts.expiresAt = time.Now().Add(expiresIn - 5*time.Minute)

	return nil
}
