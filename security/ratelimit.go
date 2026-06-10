package security

import (
	"sync"
	"time"
)

// visitorWindow tracks request timestamps for a single client
type visitorWindow struct {
	mu       sync.Mutex
	requests []time.Time
	limit    int
}

// RateLimiter implements sliding window rate limiting
type RateLimiter struct {
	windows     sync.Map // map[key]*visitorWindow
	defaultLimit int
	windowSize   time.Duration
}

// NewRateLimiter creates a new rate limiter with default limit per minute
func NewRateLimiter(limitPerMinute int) *RateLimiter {
	rl := &RateLimiter{
		defaultLimit: limitPerMinute,
		windowSize:   time.Minute,
	}
	
	// Start background cleanup goroutine
	go rl.cleanupLoop()
	
	return rl
}

// Allow checks if a request from key is allowed (zero-allocation fast path)
func (r *RateLimiter) Allow(key string) bool {
	if key == "" {
		return false
	}
	
	now := time.Now()
	
	// Get or create window
	value, _ := r.windows.LoadOrStore(key, &visitorWindow{
		requests: make([]time.Time, 0, 10),
		limit:    r.defaultLimit,
	})
	
	w := value.(*visitorWindow)
	
	w.mu.Lock()
	defer w.mu.Unlock()
	
	// Remove expired requests (older than windowSize)
	cutoff := now.Add(-r.windowSize)
	
	// Fast path: just check count
	// Note: requests slice only grows, so len() is O(1)
	// We'll use position-based filtering for zero allocation
	validCount := 0
	for i, reqTime := range w.requests {
		if reqTime.After(cutoff) {
			if i != validCount {
				w.requests[validCount] = w.requests[i]
			}
			validCount++
		}
	}
	
	w.requests = w.requests[:validCount]
	
	// Check if under limit
	if validCount >= w.limit {
		return false
	}
	
	// Append current request (single allocation only when needed)
	w.requests = append(w.requests, now)
	return true
}

// SetLimit sets a custom rate limit for a specific key
func (r *RateLimiter) SetLimit(key string, limit int) {
	if key == "" {
		return
	}
	
	value, _ := r.windows.LoadOrStore(key, &visitorWindow{
		requests: make([]time.Time, 0, 10),
		limit:    limit,
	})
	
	w := value.(*visitorWindow)
	w.mu.Lock()
	w.limit = limit
	w.mu.Unlock()
}

// Cleanup removes windows older than 2x windowSize
// Called by background goroutine every 5 minutes
func (r *RateLimiter) Cleanup() {
	cutoff := time.Now().Add(-2 * r.windowSize)
	
	r.windows.Range(func(key, value interface{}) bool {
		w := value.(*visitorWindow)
		
		w.mu.Lock()
		defer w.mu.Unlock()
		
		// Check if window is completely expired
		if len(w.requests) == 0 {
			return true
		}
		
		// If oldest request is beyond cutoff, delete the window
		if w.requests[0].Before(cutoff) {
			r.windows.Delete(key)
		}
		
		return true
	})
}

// cleanupLoop runs cleanup every 5 minutes
func (r *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		r.Cleanup()
	}
}

// GetLimit returns the current limit for a key (or default)
func (r *RateLimiter) GetLimit(key string) int {
	if value, ok := r.windows.Load(key); ok {
		w := value.(*visitorWindow)
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.limit
	}
	return r.defaultLimit
}