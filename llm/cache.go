package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CacheStats holds statistics about cache performance
type CacheStats struct {
	Hits     int `json:"hits"`
	Misses   int `json:"misses"`
	Evictions int `json:"evictions"`
	Size     int `json:"size"`
}

// ResponseCache provides content-addressable caching for LLM responses
type ResponseCache struct {
	mu      sync.RWMutex
	dir     string
	items   map[string]*cacheEntry
	stats   CacheStats
}

type cacheEntry struct {
	Key       string        `json:"key"`
	Hash      string        `json:"hash"`
	Response  *Response     `json:"response"`
	Model     string        `json:"model"`
	Tokens    int           `json:"tokens"`
	CreatedAt time.Time     `json:"created_at"`
	TTL       time.Duration `json:"ttl"`
}

// NewResponseCache creates a new response cache with the specified directory
func NewResponseCache(dir string) (*ResponseCache, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	c := &ResponseCache{
		dir:   dir,
		items: make(map[string]*cacheEntry),
	}

	// Load existing cache entries from disk
	if err := c.loadFromDisk(); err != nil {
		return nil, fmt.Errorf("failed to load cache: %w", err)
	}

	return c, nil
}

// Get retrieves a cached response for the given system prompt and messages
func (c *ResponseCache) Get(systemPrompt string, messages []Message) (*Response, bool) {
	hash := c.computeHash(systemPrompt, messages)

	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.items[hash]
	if !ok {
		c.stats.Misses++
		return nil, false
	}

	// Check if entry has expired
	if time.Since(entry.CreatedAt) > entry.TTL {
		// Entry expired, delete it asynchronously
		go c.Delete(hash)
		c.stats.Misses++
		return nil, false
	}

	c.stats.Hits++
	return entry.Response, true
}

// Set stores a response in the cache with the specified TTL
func (c *ResponseCache) Set(systemPrompt string, messages []Message, resp *Response, ttl time.Duration) {
	hash := c.computeHash(systemPrompt, messages)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Calculate total tokens
	tokens := resp.Usage.InputTokens + resp.Usage.OutputTokens

	entry := &cacheEntry{
		Key:       hash,
		Hash:      hash,
		Response:  resp,
		Model:     resp.Model,
		Tokens:    tokens,
		CreatedAt: time.Now(),
		TTL:       ttl,
	}

	c.items[hash] = entry
	c.stats.Size++

	// Persist to disk
	go c.saveToDisk(hash, entry)
}

// Invalidate removes cache entries whose hash matches the given prefix
// This is useful when codegraph reindexes and cached AST context becomes stale
func (c *ResponseCache) Invalidate(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var toDelete []string
	for hash := range c.items {
		if len(hash) >= len(prefix) && hash[:len(prefix)] == prefix {
			toDelete = append(toDelete, hash)
		}
	}

	for _, hash := range toDelete {
		delete(c.items, hash)
		c.stats.Evictions++
		c.stats.Size--

		// Delete from disk
		os.Remove(filepath.Join(c.dir, hash+".json"))
	}
}

// Clear removes all entries from the cache
func (c *ResponseCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clear in-memory map
	c.items = make(map[string]*cacheEntry)
	c.stats = CacheStats{}

	// Clear disk
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range entries {
		if err := os.Remove(filepath.Join(c.dir, entry.Name())); err != nil {
			// Log error but continue
			fmt.Printf("failed to delete cache entry %s: %v\n", entry.Name(), err)
		}
	}

	return nil
}

// Delete removes a specific entry from the cache by its hash
func (c *ResponseCache) Delete(hash string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.items[hash]; ok {
		delete(c.items, hash)
		c.stats.Evictions++
		c.stats.Size--
		os.Remove(filepath.Join(c.dir, hash+".json"))
	}
}

// Stats returns the current cache statistics
func (c *ResponseCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.stats
}

// computeHash creates a SHA-256 hash of the system prompt and messages
func (c *ResponseCache) computeHash(systemPrompt string, messages []Message) string {
	h := sha256.New()

	// Hash system prompt
	h.Write([]byte(systemPrompt))
	h.Write([]byte("|||"))

	// Hash messages in a deterministic order
	for _, msg := range messages {
		// Serialize message in a consistent way
		msgJSON, _ := json.Marshal(struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			Role:    msg.Role,
			Content: msg.Content,
		})
		h.Write(msgJSON)
		h.Write([]byte("|||"))
	}

	return hex.EncodeToString(h.Sum(nil))
}

// loadFromDisk loads all cache entries from disk into memory
func (c *ResponseCache) loadFromDisk() error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) < 5 || name[len(name)-5:] != ".json" {
			continue
		}

		path := filepath.Join(c.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue // Skip files that can't be read
		}

		var cacheEntry cacheEntry
		if err := json.Unmarshal(data, &cacheEntry); err != nil {
			continue // Skip malformed files
		}

		// Check if expired
		if time.Since(cacheEntry.CreatedAt) > cacheEntry.TTL {
			os.Remove(path)
			continue
		}

		c.items[cacheEntry.Hash] = &cacheEntry
		c.stats.Size++
	}

	return nil
}

// saveToDisk saves a single cache entry to disk
func (c *ResponseCache) saveToDisk(hash string, entry *cacheEntry) error {
	path := filepath.Join(c.dir, hash+".json")

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache entry: %w", err)
	}

	return nil
}

// Cleanup removes expired entries from the cache
func (c *ResponseCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var toDelete []string

	for hash, entry := range c.items {
		if now.Sub(entry.CreatedAt) > entry.TTL {
			toDelete = append(toDelete, hash)
		}
	}

	for _, hash := range toDelete {
		delete(c.items, hash)
		c.stats.Evictions++
		c.stats.Size--
		os.Remove(filepath.Join(c.dir, hash+".json"))
	}
}