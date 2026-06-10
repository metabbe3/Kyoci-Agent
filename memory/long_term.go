package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LongTermMemory is the main memory store that can use SQLite or JSON
type LongTermMemory struct {
	entries     map[string]*memoryEntryMutex
	storagePath string
	sqlite      *SQLiteMemory
	jsonStore   *JSONMemoryStore
	useSQLite   bool
	mu          sync.RWMutex
}

// NewLongTermMemory creates a new long-term memory store
// Tries SQLite first, falls back to JSON if SQLite fails
func NewLongTermMemory(storagePath string) (*LongTermMemory, error) {
	// Ensure directory exists
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	mem := &LongTermMemory{
		entries:     make(map[string]*memoryEntryMutex),
		storagePath: storagePath,
	}

	// Try to initialize SQLite
	dbPath := filepath.Join(storagePath, "memory.db")
	sqliteMem, err := NewSQLiteMemory(dbPath, storagePath)
	if err == nil {
		mem.sqlite = sqliteMem
		mem.useSQLite = true
		
		// Try to migrate from JSON if it exists
		jsonPath := filepath.Join(storagePath, "memory.json")
		if _, statErr := os.Stat(jsonPath); statErr == nil {
			if migrateErr := mem.sqlite.MigrateFromJSON(jsonPath); migrateErr != nil {
				// Log migration error but continue with SQLite
				fmt.Printf("Warning: failed to migrate from JSON: %v\n", migrateErr)
			}
		}
		
		fmt.Printf("Using SQLite memory store at: %s\n", dbPath)
	} else {
		// Fallback to JSON
		jsonStore, jsonErr := NewJSONMemoryStore(storagePath)
		if jsonErr != nil {
			return nil, fmt.Errorf("failed to initialize both SQLite and JSON stores: sqlite: %w, json: %w", err, jsonErr)
		}
		mem.jsonStore = jsonStore
		mem.useSQLite = false
		fmt.Printf("Using JSON memory store at: %s\n", filepath.Join(storagePath, "memory.json"))
	}

	return mem, nil
}

// JSONMemoryStore implements LongTerm memory using JSON file storage
type JSONMemoryStore struct {
	entries     map[string]*memoryEntryMutex
	storagePath string
	mu          sync.RWMutex
}

// NewJSONMemoryStore creates a new JSON-based long-term memory store
func NewJSONMemoryStore(storagePath string) (*JSONMemoryStore, error) {
	// Ensure directory exists
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	store := &JSONMemoryStore{
		entries:     make(map[string]*memoryEntryMutex),
		storagePath: storagePath,
	}

	// Load existing entries
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("failed to load existing memory: %w", err)
	}

	return store, nil
}

// AddEntry adds a new memory entry
func (store *JSONMemoryStore) AddEntry(entry *MemoryEntry) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	if entry.ID == "" {
		entry.ID = generateID()
	}

	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	if entry.Importance < 1 {
		entry.Importance = 1
	}
	if entry.Importance > 10 {
		entry.Importance = 10
	}

	store.entries[entry.ID] = &memoryEntryMutex{
		entry: *entry,
	}

	return store.Save()
}

// GetEntry retrieves an entry by ID
func (store *JSONMemoryStore) GetEntry(id string) (*MemoryEntry, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	wrapped, exists := store.entries[id]
	if !exists {
		return nil, fmt.Errorf("entry not found: %s", id)
	}

	wrapped.mu.RLock()
	defer wrapped.mu.RUnlock()

	// Increment access count
	wrapped.mu.Lock()
	wrapped.entry.AccessCount++
	wrapped.mu.Unlock()

	// Return a copy
	copy := wrapped.entry
	return &copy, nil
}

// GetByCategory retrieves all entries of a specific category
func (store *JSONMemoryStore) GetByCategory(category Category) ([]*MemoryEntry, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	var results []*MemoryEntry

	for _, wrapped := range store.entries {
		wrapped.mu.RLock()
		if wrapped.entry.Category == category {
			copy := wrapped.entry
			results = append(results, &copy)
		}
		wrapped.mu.RUnlock()
	}

	return results, nil
}

// Search searches for entries containing the query string
func (store *JSONMemoryStore) Search(query string) ([]*MemoryEntry, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	if query == "" {
		return store.GetAll()
	}

	var results []*MemoryEntry
	lowerQuery := strings.ToLower(query)

	for _, wrapped := range store.entries {
		wrapped.mu.RLock()
		if strings.Contains(strings.ToLower(wrapped.entry.Content), lowerQuery) {
			copy := wrapped.entry
			results = append(results, &copy)
		}
		wrapped.mu.RUnlock()
	}

	return results, nil
}

// UpdateEntry updates an existing entry
func (store *JSONMemoryStore) UpdateEntry(entry *MemoryEntry) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	wrapped, exists := store.entries[entry.ID]
	if !exists {
		return fmt.Errorf("entry not found: %s", entry.ID)
	}

	wrapped.mu.Lock()
	wrapped.entry = *entry
	wrapped.mu.Unlock()

	return store.Save()
}

// DeleteEntry removes an entry by ID
func (store *JSONMemoryStore) DeleteEntry(id string) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	if _, exists := store.entries[id]; !exists {
		return fmt.Errorf("entry not found: %s", id)
	}

	delete(store.entries, id)
	return store.Save()
}

// GetAll retrieves all entries
func (store *JSONMemoryStore) GetAll() ([]*MemoryEntry, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	results := make([]*MemoryEntry, 0, len(store.entries))

	for _, wrapped := range store.entries {
		wrapped.mu.RLock()
		copy := wrapped.entry
		results = append(results, &copy)
		wrapped.mu.RUnlock()
	}

	return results, nil
}

// ExtractFromConversation analyzes conversation messages and extracts important facts
// This is a placeholder implementation - in a real system, this would use an LLM
func (store *JSONMemoryStore) ExtractFromConversation(messages []Message) ([]*MemoryEntry, error) {
	// Placeholder: Extract simple patterns from messages
	var extracted []*MemoryEntry

	for _, msg := range messages {
		if msg.Role == "user" || msg.Role == "assistant" {
			// Look for preference patterns
			if strings.Contains(strings.ToLower(msg.Content), "i prefer") ||
				strings.Contains(strings.ToLower(msg.Content), "i like") ||
				strings.Contains(strings.ToLower(msg.Content), "i want") {
				entry := &MemoryEntry{
					ID:          generateID(),
					Category:    CategoryPreference,
					Content:     msg.Content,
					CreatedAt:   time.Now(),
					AccessCount: 0,
					Importance:  5,
				}
				extracted = append(extracted, entry)
			}

			// Look for factual statements
			if strings.Contains(strings.ToLower(msg.Content), "is a ") ||
				strings.Contains(strings.ToLower(msg.Content), "has a ") {
				entry := &MemoryEntry{
					ID:          generateID(),
					Category:    CategoryFact,
					Content:     msg.Content,
					CreatedAt:   time.Now(),
					AccessCount: 0,
					Importance:  4,
				}
				extracted = append(extracted, entry)
			}
		}
	}

	// Add extracted entries to store
	for _, entry := range extracted {
		if err := store.AddEntry(entry); err != nil {
			return nil, fmt.Errorf("failed to add extracted entry: %w", err)
		}
	}

	return extracted, nil
}

// IncrementAccess increments the access count for an entry
func (store *JSONMemoryStore) IncrementAccess(id string) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	wrapped, exists := store.entries[id]
	if !exists {
		return fmt.Errorf("entry not found: %s", id)
	}

	wrapped.mu.Lock()
	wrapped.entry.AccessCount++
	wrapped.mu.Unlock()

	return nil
}

// Save persists all entries to storage
func (store *JSONMemoryStore) Save() error {
	store.mu.RLock()
	defer store.mu.RUnlock()

	// Collect all entries
	entries := make([]MemoryEntry, 0, len(store.entries))
	for _, wrapped := range store.entries {
		wrapped.mu.RLock()
		entries = append(entries, wrapped.entry)
		wrapped.mu.RUnlock()
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal memory: %w", err)
	}

	// Write to file
	filePath := filepath.Join(store.storagePath, "memory.json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write memory file: %w", err)
	}

	return nil
}

// Load loads entries from storage
func (store *JSONMemoryStore) Load() error {
	store.mu.Lock()
	defer store.mu.Unlock()

	filePath := filepath.Join(store.storagePath, "memory.json")

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// No existing memory, start fresh
		return nil
	}

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read memory file: %w", err)
	}

	// Unmarshal JSON
	var entries []MemoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("failed to unmarshal memory: %w", err)
	}

	// Populate entries map
	store.entries = make(map[string]*memoryEntryMutex)
	for _, entry := range entries {
		store.entries[entry.ID] = &memoryEntryMutex{
			entry: entry,
		}
	}

	return nil
}

// GetByImportance retrieves entries sorted by importance
func (store *JSONMemoryStore) GetByImportance(minImportance int) ([]*MemoryEntry, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	var results []*MemoryEntry

	for _, wrapped := range store.entries {
		wrapped.mu.RLock()
		if wrapped.entry.Importance >= minImportance {
			copy := wrapped.entry
			results = append(results, &copy)
		}
		wrapped.mu.RUnlock()
	}

	// Sort by importance (highest first)
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Importance < results[j].Importance {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results, nil
}

// GetByAccessCount retrieves entries sorted by access count
func (store *JSONMemoryStore) GetByAccessCount(minAccessCount int) ([]*MemoryEntry, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	var results []*MemoryEntry

	for _, wrapped := range store.entries {
		wrapped.mu.RLock()
		if wrapped.entry.AccessCount >= minAccessCount {
			copy := wrapped.entry
			results = append(results, &copy)
		}
		wrapped.mu.RUnlock()
	}

	// Sort by access count (highest first)
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].AccessCount < results[j].AccessCount {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results, nil
}

// GetEntryCount returns the total number of entries
func (store *JSONMemoryStore) GetEntryCount() int {
	store.mu.RLock()
	defer store.mu.RUnlock()

	return len(store.entries)
}

// Clear removes all entries
func (store *JSONMemoryStore) Clear() error {
	store.mu.Lock()
	defer store.mu.Unlock()

	store.entries = make(map[string]*memoryEntryMutex)
	return store.Save()
}

// generateID generates a unique ID for memory entries
func generateID() string {
	return fmt.Sprintf("mem_%d_%d", time.Now().UnixNano(), time.Now().Nanosecond())
}

// LongTermMemory implements LongTerm interface methods

// AddEntry adds a new memory entry
func (m *LongTermMemory) AddEntry(entry *MemoryEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.useSQLite {
		return m.sqlite.AddEntry(entry)
	}
	return m.jsonStore.AddEntry(entry)
}

// GetEntry retrieves an entry by ID
func (m *LongTermMemory) GetEntry(id string) (*MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.useSQLite {
		return m.sqlite.GetEntry(id)
	}
	return m.jsonStore.GetEntry(id)
}

// GetByCategory retrieves all entries of a specific category
func (m *LongTermMemory) GetByCategory(category Category) ([]*MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.useSQLite {
		return m.sqlite.GetByCategory(category)
	}
	return m.jsonStore.GetByCategory(category)
}

// Search searches for entries containing the query string
func (m *LongTermMemory) Search(query string) ([]*MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.useSQLite {
		return m.sqlite.Search(query)
	}
	return m.jsonStore.Search(query)
}

// UpdateEntry updates an existing entry
func (m *LongTermMemory) UpdateEntry(entry *MemoryEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.useSQLite {
		return m.sqlite.UpdateEntry(entry)
	}
	return m.jsonStore.UpdateEntry(entry)
}

// DeleteEntry removes an entry by ID
func (m *LongTermMemory) DeleteEntry(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.useSQLite {
		return m.sqlite.DeleteEntry(id)
	}
	return m.jsonStore.DeleteEntry(id)
}

// GetAll retrieves all entries
func (m *LongTermMemory) GetAll() ([]*MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.useSQLite {
		return m.sqlite.GetAll()
	}
	return m.jsonStore.GetAll()
}

// ExtractFromConversation analyzes conversation messages and extracts important facts
func (m *LongTermMemory) ExtractFromConversation(messages []Message) ([]*MemoryEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.useSQLite {
		return m.sqlite.ExtractFromConversation(messages)
	}
	return m.jsonStore.ExtractFromConversation(messages)
}

// IncrementAccess increments the access count for an entry
func (m *LongTermMemory) IncrementAccess(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.useSQLite {
		return m.sqlite.IncrementAccess(id)
	}
	return m.jsonStore.IncrementAccess(id)
}

// Save persists all entries to storage
func (m *LongTermMemory) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.useSQLite {
		return m.sqlite.Save()
	}
	return m.jsonStore.Save()
}

// Load loads entries from storage
func (m *LongTermMemory) Load() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.useSQLite {
		return m.sqlite.Load()
	}
	return m.jsonStore.Load()
}

// Close closes the memory store
func (m *LongTermMemory) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.useSQLite && m.sqlite != nil {
		return m.sqlite.Close()
	}
	return nil
}

// GetStats returns memory statistics
func (m *LongTermMemory) GetStats() (*MemoryStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.useSQLite && m.sqlite != nil {
		return m.sqlite.Stats(context.Background())
	}
	return &MemoryStats{}, nil
}