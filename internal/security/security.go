package security

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// SecurityManager handles authentication, rate limiting, and input sanitization.
type SecurityManager struct {
	config          SecurityConfig
	apiKeys         map[string]bool
	rateLimiter     sync.Map
	rateWindow      time.Duration
	rateLimitMax    int
}

// SecurityConfig holds security configuration.
type SecurityConfig struct {
	Enabled       bool
	APIKeyHashes  []string // SHA256 hashes of valid API keys
	RateLimitMax  int      // Max requests per window
	RateWindowSec int      // Rate limit window in seconds
	MaxInputSize  int      // Maximum input length
}

// New creates a new SecurityManager with the given configuration.
func New(cfg SecurityConfig) *SecurityManager {
	if cfg.RateLimitMax <= 0 {
		cfg.RateLimitMax = 100
	}
	if cfg.RateWindowSec <= 0 {
		cfg.RateWindowSec = 60
	}
	if cfg.MaxInputSize <= 0 {
		cfg.RateWindowSec = 10000
	}

	apiKeys := make(map[string]bool)
	for _, hash := range cfg.APIKeyHashes {
		apiKeys[strings.ToLower(hash)] = true
	}

	return &SecurityManager{
		config:       cfg,
		apiKeys:      apiKeys,
		rateWindow:   time.Duration(cfg.RateWindowSec) * time.Second,
		rateLimitMax: cfg.RateLimitMax,
	}
}

// Authenticate validates an API key by comparing its SHA256 hash with stored hashes.
// If security is disabled, always returns true.
func (sm *SecurityManager) Authenticate(apiKey string) bool {
	if !sm.config.Enabled {
		return true
	}

	if apiKey == "" {
		return false
	}

	// Compute SHA256 hash of the provided API key
	hash := sha256.Sum256([]byte(apiKey))
	hashStr := strings.ToLower(hex.EncodeToString(hash[:]))

	// Check if hash matches any stored key
	return sm.apiKeys[hashStr]
}

// RateLimit checks if a given key has exceeded its rate limit.
// Returns true if the request should be allowed, false if rate limited.
func (sm *SecurityManager) RateLimit(key string) bool {
	if !sm.config.Enabled {
		return true
	}

	if key == "" {
		key = "anonymous"
	}

	now := time.Now()
	windowStart := now.Add(-sm.rateWindow)

	// Get or create rate limit entry for this key
	value, _ := sm.rateLimiter.LoadOrStore(key, &rateLimitEntry{})
	entry := value.(*rateLimitEntry)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	// Clean up old entries outside the window
	var recent []time.Time
	for _, timestamp := range entry.timestamps {
		if timestamp.After(windowStart) {
			recent = append(recent, timestamp)
		}
	}
	entry.timestamps = recent

	// Check if under limit
	if len(entry.timestamps) >= sm.rateLimitMax {
		return false
	}

	// Add current request
	entry.timestamps = append(entry.timestamps, now)
	return true
}

// rateLimitEntry tracks request timestamps for a single key.
type rateLimitEntry struct {
	timestamps []time.Time
	mu         sync.Mutex
}

// Sanitize performs basic input sanitization:
// - Removes control characters except newline, tab, carriage return
// - Truncates to maximum input length
func (sm *SecurityManager) Sanitize(input string) string {
	maxLen := sm.config.MaxInputSize
	if maxLen <= 0 {
		maxLen = 10000
	}

	// Remove control characters but preserve whitespace
	var sanitized strings.Builder
	for _, r := range input {
		// Allow newline, tab, carriage return, and printable characters
		if r == '\n' || r == '\t' || r == '\r' || (r >= 32 && r <= 126) || (r >= 128) {
			sanitized.WriteRune(r)
		}
	}

	result := sanitized.String()

	// Truncate to max length
	if len(result) > maxLen {
		result = result[:maxLen]
	}

	return result
}

// ValidateRequest validates incoming request fields.
// Returns an error if validation fails, nil otherwise.
func (sm *SecurityManager) ValidateRequest(req interface{}) error {
	// Type-based validation would be implemented here
	// For now, we'll do basic checks on common request types

	switch v := req.(type) {
	case *ExecuteRequest:
		if v.Task == "" {
			return fmt.Errorf("task cannot be empty")
		}
		if len(v.Task) > sm.config.MaxInputSize {
			return fmt.Errorf("task exceeds maximum size of %d", sm.config.MaxInputSize)
		}
	case *StreamRequest:
		if v.Task == "" {
			return fmt.Errorf("task cannot be empty")
		}
		if len(v.Task) > sm.config.MaxInputSize {
			return fmt.Errorf("task exceeds maximum size of %d", sm.config.MaxInputSize)
		}
	}

	return nil
}

// ExecuteRequest represents a task execution request for validation.
type ExecuteRequest struct {
	Task      string
	Role      string
	SessionID string
}

// StreamRequest represents a streaming task request for validation.
type StreamRequest struct {
	Task      string
	Role      string
	SessionID string
}