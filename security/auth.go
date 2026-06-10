package security

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// KeyInfo holds information about an API key
type KeyInfo struct {
	Key        string
	Owner      string
	Roles      []string
	RateLimit  int
	CreatedAt  time.Time
}

// APIKeyAuth manages API key authentication using SHA256 hashing
type APIKeyAuth struct {
	keys map[string]KeyInfo // map[hash]KeyInfo
	mu   sync.RWMutex
}

// NewAPIKeyAuth creates a new APIKeyAuth instance and loads keys from env
func NewAPIKeyAuth() *APIKeyAuth {
	a := &APIKeyAuth{
		keys: make(map[string]KeyInfo),
	}
	a.loadFromEnv()
	return a
}

// loadFromEnv loads keys from AGENT_API_KEYS environment variable
// Format: key1:owner1:role1,role2;key2:owner2:admin
func (a *APIKeyAuth) loadFromEnv() {
	envKeys := os.Getenv("AGENT_API_KEYS")
	if envKeys == "" {
		return
	}

	entries := strings.Split(envKeys, ";")
	for _, entry := range entries {
		parts := strings.Split(entry, ":")
		if len(parts) < 2 {
			continue
		}
		key := parts[0]
		owner := parts[1]
		
		var roles []string
		var rateLimit int = 60 // default
		
		if len(parts) > 2 && parts[2] != "" {
			roles = strings.Split(parts[2], ",")
		}
		
		// Optional rate limit as 4th part
		if len(parts) > 3 && parts[3] != "" {
			var err error
			rateLimit, err = parseRateLimit(parts[3])
			if err != nil {
				rateLimit = 60
			}
		}
		
		a.AddKey(key, owner, roles, rateLimit)
	}
}

// parseRateLimit parses rate limit string to int
func parseRateLimit(s string) (int, error) {
	var limit int
	_, err := fmt.Sscanf(s, "%d", &limit)
	return limit, err
}

// AddKey adds a new API key (stores SHA256 hash, not plaintext)
func (a *APIKeyAuth) AddKey(key, owner string, roles []string, rateLimit int) {
	hash := sha256Hash(key)
	prefix := HashForPrefixMatch(hash)
	
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Store with prefix as key for fast lookups
	a.keys[prefix] = KeyInfo{
		Key:       "", // Never store plaintext key
		Owner:     owner,
		Roles:     roles,
		RateLimit: rateLimit,
		CreatedAt: time.Now(),
	}
}

// Validate checks if a key is valid and returns KeyInfo
// Uses SHA256 prefix match (first 8 chars) for fast lookups
func (a *APIKeyAuth) Validate(key string) (KeyInfo, bool) {
	if key == "" {
		return KeyInfo{}, false
	}
	
	hash := sha256Hash(key)
	prefix := HashForPrefixMatch(hash)
	
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	info, exists := a.keys[prefix]
	return info, exists
}

// Revoke removes an API key
func (a *APIKeyAuth) Revoke(key string) {
	hash := sha256Hash(key)
	prefix := HashForPrefixMatch(hash)
	
	a.mu.Lock()
	defer a.mu.Unlock()
	
	delete(a.keys, prefix)
}

// ListKeys returns all registered key information (without actual keys)
func (a *APIKeyAuth) ListKeys() []KeyInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	result := make([]KeyInfo, 0, len(a.keys))
	for _, info := range a.keys {
		result = append(result, info)
	}
	return result
}

// sha256Hash returns SHA256 hash of input string
func sha256Hash(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}

// HashForPrefixMatch returns first 8 chars of hash for fast prefix matching
func HashForPrefixMatch(hash string) string {
	if len(hash) < 8 {
		return hash
	}
	return hash[:8]
}