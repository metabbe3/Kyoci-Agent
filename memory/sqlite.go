package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteMemory implements LongTerm memory using SQLite with FTS5 full-text search
type SQLiteMemory struct {
	db          *sql.DB
	dbPath      string
	jsonPath    string // fallback JSON path
	jsonStore   *JSONMemoryStore
	mu          sync.RWMutex
	initialized bool
}

// SQLiteMemoryEntry represents a memory entry in SQLite storage
type SQLiteMemoryEntry struct {
	ID          int64     `json:"id"`
	Key         string    `json:"key"`
	Content     string    `json:"content"`
	Category    string    `json:"category"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	AccessCount int       `json:"access_count"`
	Importance  int       `json:"importance"`
	Metadata    string    `json:"metadata"` // JSON string
}

// NewSQLiteMemory creates a new SQLite-based long-term memory store
func NewSQLiteMemory(dbPath string, jsonPath string) (*SQLiteMemory, error) {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database connection
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure database for better performance
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	mem := &SQLiteMemory{
		db:        db,
		dbPath:    dbPath,
		jsonPath:  jsonPath,
		initialized: false,
	}

	// Initialize schema
	if err := mem.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database schema: %w", err)
	}

	mem.initialized = true

	// Try to initialize JSON fallback store
	if jsonPath != "" {
		if jsonStore, err := NewJSONMemoryStore(jsonPath); err == nil {
			mem.jsonStore = jsonStore
		}
	}

	return mem, nil
}

// initSchema creates the database schema if it doesn't exist
func (s *SQLiteMemory) initSchema() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create main memories table
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS memories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL UNIQUE,
			content TEXT NOT NULL,
			category TEXT DEFAULT 'general',
			metadata TEXT DEFAULT '{}',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			access_count INTEGER DEFAULT 0,
			importance INTEGER DEFAULT 5
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to create memories table: %w", err)
	}

	// Create indexes
	_, err = s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_memories_category ON memories(category);
		CREATE INDEX IF NOT EXISTS idx_memories_created ON memories(created_at);
		CREATE INDEX IF NOT EXISTS idx_memories_key ON memories(key);
	`)
	if err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	// Create FTS5 virtual table for full-text search
	_, err = s.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
			content,
			category,
			metadata,
			content='memories',
			content_rowid='id'
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to create FTS5 table: %w", err)
	}

	// Create triggers for FTS5 sync
	triggers := []string{
		`DROP TRIGGER IF EXISTS memories_ai;`,
		`DROP TRIGGER IF EXISTS memories_ad;`,
		`DROP TRIGGER IF EXISTS memories_au;`,
	}

	for _, trigger := range triggers {
		if _, err := s.db.Exec(trigger); err != nil {
			return fmt.Errorf("failed to drop trigger: %w", err)
		}
	}

	triggerSQL := `
		CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
			INSERT INTO memories_fts(rowid, content, category, metadata)
			VALUES (new.id, new.content, new.category, new.metadata);
		END;

		CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, content, category, metadata)
			VALUES ('delete', old.id, old.content, old.category, old.metadata);
		END;

		CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, content, category, metadata)
			VALUES ('delete', old.id, old.content, old.category, old.metadata);
			INSERT INTO memories_fts(rowid, content, category, metadata)
			VALUES (new.id, new.content, new.category, new.metadata);
		END;
	`

	if _, err := s.db.Exec(triggerSQL); err != nil {
		return fmt.Errorf("failed to create triggers: %w", err)
	}

	return nil
}

// AddEntry adds a new memory entry
func (s *SQLiteMemory) AddEntry(entry *MemoryEntry) error {
	if !s.initialized {
		return s.fallbackAddEntry(entry)
	}

	ctx := context.Background()

	// Generate key from ID
	key := entry.ID
	if key == "" {
		key = fmt.Sprintf("mem_%d", time.Now().UnixNano())
		entry.ID = key
	}

	// Prepare metadata as JSON
	metadata := map[string]string{}
	if entry.Category != "" {
		metadata["category"] = string(entry.Category)
	}
	metadataJSON, _ := json.Marshal(metadata)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if entry already exists
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM memories WHERE key = ?)", key).Scan(&exists)
	if err != nil {
		return s.fallbackAddEntry(entry)
	}

	if exists {
		// Update existing entry
		_, err = s.db.ExecContext(ctx, `
			UPDATE memories 
			SET content = ?, category = ?, metadata = ?, updated_at = CURRENT_TIMESTAMP,
			    access_count = access_count + 1, importance = ?
			WHERE key = ?
		`, entry.Content, entry.Category, string(metadataJSON), entry.Importance, key)
	} else {
		// Insert new entry
		if entry.CreatedAt.IsZero() {
			entry.CreatedAt = time.Now()
		}
		if entry.Importance < 1 {
			entry.Importance = 1
		}
		if entry.Importance > 10 {
			entry.Importance = 10
		}

		_, err = s.db.ExecContext(ctx, `
			INSERT INTO memories (key, content, category, metadata, created_at, access_count, importance)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, key, entry.Content, entry.Category, string(metadataJSON), entry.CreatedAt, entry.AccessCount, entry.Importance)
	}

	if err != nil {
		return s.fallbackAddEntry(entry)
	}

	return nil
}

// GetEntry retrieves an entry by ID
func (s *SQLiteMemory) GetEntry(id string) (*MemoryEntry, error) {
	if !s.initialized {
		return s.fallbackGetEntry(id)
	}

	ctx := context.Background()

	s.mu.Lock()
	defer s.mu.Unlock()

	var entry SQLiteMemoryEntry
	err := s.db.QueryRowContext(ctx, `
		SELECT id, key, content, category, created_at, updated_at, access_count, importance, metadata
		FROM memories WHERE key = ?
	`, id).Scan(
		&entry.ID, &entry.Key, &entry.Content, &entry.Category,
		&entry.CreatedAt, &entry.UpdatedAt, &entry.AccessCount, &entry.Importance, &entry.Metadata,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return s.fallbackGetEntry(id)
		}
		return s.fallbackGetEntry(id)
	}

	// Increment access count
	s.db.ExecContext(ctx, "UPDATE memories SET access_count = access_count + 1 WHERE key = ?", id)

	// Convert to MemoryEntry
	memEntry := &MemoryEntry{
		ID:          entry.Key,
		Category:    Category(entry.Category),
		Content:     entry.Content,
		CreatedAt:   entry.CreatedAt,
		AccessCount: entry.AccessCount,
		Importance:  entry.Importance,
	}

	return memEntry, nil
}

// GetByCategory retrieves all entries of a specific category
func (s *SQLiteMemory) GetByCategory(category Category) ([]*MemoryEntry, error) {
	if !s.initialized {
		return s.fallbackGetByCategory(category)
	}

	ctx := context.Background()

	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT key, content, category, created_at, access_count, importance
		FROM memories WHERE category = ? ORDER BY created_at DESC
	`, category)
	if err != nil {
		return s.fallbackGetByCategory(category)
	}
	defer rows.Close()

	var results []*MemoryEntry
	for rows.Next() {
		var key, content, cat string
		var created time.Time
		var accessCount, importance int

		if err := rows.Scan(&key, &content, &cat, &created, &accessCount, &importance); err != nil {
			continue
		}

		results = append(results, &MemoryEntry{
			ID:          key,
			Category:    Category(cat),
			Content:     content,
			CreatedAt:   created,
			AccessCount: accessCount,
			Importance:  importance,
		})
	}

	return results, nil
}

// Search searches for entries using FTS5 full-text search
func (s *SQLiteMemory) Search(query string) ([]*MemoryEntry, error) {
	if !s.initialized {
		return s.fallbackSearch(query)
	}

	if query == "" {
		return s.GetAll()
	}

	ctx := context.Background()

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Use FTS5 for full-text search
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.key, m.content, m.category, m.created_at, m.access_count, m.importance
		FROM memories m
		JOIN memories_fts fts ON m.id = fts.rowid
		WHERE memories_fts MATCH ?
		ORDER BY m.created_at DESC
		LIMIT 100
	`, query)
	if err != nil {
		// Fallback to LIKE search if FTS5 fails
		return s.fallbackSearch(query)
	}
	defer rows.Close()

	var results []*MemoryEntry
	for rows.Next() {
		var key, content, cat string
		var created time.Time
		var accessCount, importance int

		if err := rows.Scan(&key, &content, &cat, &created, &accessCount, &importance); err != nil {
			continue
		}

		results = append(results, &MemoryEntry{
			ID:          key,
			Category:    Category(cat),
			Content:     content,
			CreatedAt:   created,
			AccessCount: accessCount,
			Importance:  importance,
		})
	}

	return results, nil
}

// UpdateEntry updates an existing entry
func (s *SQLiteMemory) UpdateEntry(entry *MemoryEntry) error {
	if !s.initialized {
		return s.fallbackUpdateEntry(entry)
	}

	ctx := context.Background()

	// Prepare metadata as JSON
	metadata := map[string]string{}
	if entry.Category != "" {
		metadata["category"] = string(entry.Category)
	}
	metadataJSON, _ := json.Marshal(metadata)

	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx, `
		UPDATE memories 
		SET content = ?, category = ?, metadata = ?, updated_at = CURRENT_TIMESTAMP, importance = ?
		WHERE key = ?
	`, entry.Content, entry.Category, string(metadataJSON), entry.Importance, entry.ID)

	if err != nil {
		return s.fallbackUpdateEntry(entry)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("entry not found: %s", entry.ID)
	}

	return nil
}

// DeleteEntry removes an entry by ID
func (s *SQLiteMemory) DeleteEntry(id string) error {
	if !s.initialized {
		return s.fallbackDeleteEntry(id)
	}

	ctx := context.Background()

	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx, "DELETE FROM memories WHERE key = ?", id)
	if err != nil {
		return s.fallbackDeleteEntry(id)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("entry not found: %s", id)
	}

	return nil
}

// GetAll retrieves all entries
func (s *SQLiteMemory) GetAll() ([]*MemoryEntry, error) {
	if !s.initialized {
		return s.fallbackGetAll()
	}

	ctx := context.Background()

	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT key, content, category, created_at, access_count, importance
		FROM memories ORDER BY created_at DESC
	`)
	if err != nil {
		return s.fallbackGetAll()
	}
	defer rows.Close()

	var results []*MemoryEntry
	for rows.Next() {
		var key, content, cat string
		var created time.Time
		var accessCount, importance int

		if err := rows.Scan(&key, &content, &cat, &created, &accessCount, &importance); err != nil {
			continue
		}

		results = append(results, &MemoryEntry{
			ID:          key,
			Category:    Category(cat),
			Content:     content,
			CreatedAt:   created,
			AccessCount: accessCount,
			Importance:  importance,
		})
	}

	return results, nil
}

// ExtractFromConversation analyzes conversation messages and extracts important facts
func (s *SQLiteMemory) ExtractFromConversation(messages []Message) ([]*MemoryEntry, error) {
	// Delegate to JSON store for extraction, then store in SQLite
	if s.jsonStore != nil {
		extracted, err := s.jsonStore.ExtractFromConversation(messages)
		if err != nil {
			return nil, err
		}
		// Store extracted entries in SQLite
		for _, entry := range extracted {
			if err := s.AddEntry(entry); err != nil {
				return nil, err
			}
		}
		return extracted, nil
	}

	// Fallback: simple pattern extraction
	return extractFromMessages(messages, s)
}

// IncrementAccess increments the access count for an entry
func (s *SQLiteMemory) IncrementAccess(id string) error {
	if !s.initialized {
		return s.fallbackIncrementAccess(id)
	}

	ctx := context.Background()

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, "UPDATE memories SET access_count = access_count + 1 WHERE key = ?", id)
	if err != nil {
		return s.fallbackIncrementAccess(id)
	}

	return nil
}

// Save persists all entries (no-op for SQLite)
func (s *SQLiteMemory) Save() error {
	// SQLite persists automatically
	return nil
}

// Load loads entries from storage (no-op for SQLite)
func (s *SQLiteMemory) Load() error {
	// SQLite loads automatically on connection
	return nil
}

// Close closes the database connection
func (s *SQLiteMemory) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// MigrateFromJSON migrates existing JSON data to SQLite
func (s *SQLiteMemory) MigrateFromJSON(jsonPath string) error {
	if jsonPath == "" {
		return fmt.Errorf("JSON path is required")
	}

	// Read JSON file
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read JSON file: %w", err)
	}

	// Parse JSON entries
	var entries []MemoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Insert entries into SQLite
	for i := range entries {
		if err := s.AddEntry(&entries[i]); err != nil {
			return fmt.Errorf("failed to migrate entry %s: %w", entries[i].ID, err)
		}
	}

	// Backup JSON file
	backupPath := jsonPath + ".bak"
	if err := os.Rename(jsonPath, backupPath); err != nil {
		return fmt.Errorf("failed to backup JSON file: %w", err)
	}

	return nil
}

// Stats returns memory statistics
func (s *SQLiteMemory) Stats(ctx context.Context) (*MemoryStats, error) {
	if !s.initialized {
		return &MemoryStats{}, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &MemoryStats{}

	// Count total entries
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM memories").Scan(&stats.TotalEntries)
	if err != nil {
		return nil, err
	}

	// Count entries by category
	stats.Categories = make(map[string]int64)
	rows, err := s.db.QueryContext(ctx, `
		SELECT category, COUNT(*) as count
		FROM memories GROUP BY category
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var cat string
		var count int64
		if err := rows.Scan(&cat, &count); err != nil {
			continue
		}
		stats.Categories[cat] = count
	}

	// Get database file size
	if info, err := os.Stat(s.dbPath); err == nil {
		stats.DBSize = info.Size()
	}

	return stats, nil
}

// Fallback methods that delegate to JSON store

func (s *SQLiteMemory) fallbackAddEntry(entry *MemoryEntry) error {
	if s.jsonStore != nil {
		return s.jsonStore.AddEntry(entry)
	}
	return fmt.Errorf("SQLite not initialized and no JSON fallback available")
}

func (s *SQLiteMemory) fallbackGetEntry(id string) (*MemoryEntry, error) {
	if s.jsonStore != nil {
		return s.jsonStore.GetEntry(id)
	}
	return nil, fmt.Errorf("SQLite not initialized and no JSON fallback available")
}

func (s *SQLiteMemory) fallbackGetByCategory(category Category) ([]*MemoryEntry, error) {
	if s.jsonStore != nil {
		return s.jsonStore.GetByCategory(category)
	}
	return nil, fmt.Errorf("SQLite not initialized and no JSON fallback available")
}

func (s *SQLiteMemory) fallbackSearch(query string) ([]*MemoryEntry, error) {
	if s.jsonStore != nil {
		return s.jsonStore.Search(query)
	}
	return nil, fmt.Errorf("SQLite not initialized and no JSON fallback available")
}

func (s *SQLiteMemory) fallbackUpdateEntry(entry *MemoryEntry) error {
	if s.jsonStore != nil {
		return s.jsonStore.UpdateEntry(entry)
	}
	return fmt.Errorf("SQLite not initialized and no JSON fallback available")
}

func (s *SQLiteMemory) fallbackDeleteEntry(id string) error {
	if s.jsonStore != nil {
		return s.jsonStore.DeleteEntry(id)
	}
	return fmt.Errorf("SQLite not initialized and no JSON fallback available")
}

func (s *SQLiteMemory) fallbackGetAll() ([]*MemoryEntry, error) {
	if s.jsonStore != nil {
		return s.jsonStore.GetAll()
	}
	return nil, fmt.Errorf("SQLite not initialized and no JSON fallback available")
}

func (s *SQLiteMemory) fallbackIncrementAccess(id string) error {
	if s.jsonStore != nil {
		return s.jsonStore.IncrementAccess(id)
	}
	return fmt.Errorf("SQLite not initialized and no JSON fallback available")
}

// extractFromMessages is a helper function for extracting memories from messages
func extractFromMessages(messages []Message, store LongTerm) ([]*MemoryEntry, error) {
	var extracted []*MemoryEntry

	for _, msg := range messages {
		if msg.Role == "user" || msg.Role == "assistant" {
			// Look for preference patterns
			contentLower := msg.Content
			if len(contentLower) > 100 {
				// Truncate very long messages for pattern matching
				contentLower = contentLower[:100]
			}

			// Look for preference patterns
			if containsAny(contentLower, []string{"i prefer", "i like", "i want"}) {
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
			if containsAny(contentLower, []string{"is a ", "has a "}) {
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

// containsAny checks if text contains any of the substrings (case-insensitive)
func containsAny(text string, substrings []string) bool {
	textLower := text
	for _, substr := range substrings {
		if len(textLower) >= len(substr) {
			// Simple case-insensitive check
			for i := 0; i <= len(textLower)-len(substr); i++ {
				match := true
				for j := 0; j < len(substr); j++ {
					c1 := textLower[i+j]
					c2 := substr[j]
					if c1 >= 'A' && c1 <= 'Z' {
						c1 = c1 + 32
					}
					if c2 >= 'A' && c2 <= 'Z' {
						c2 = c2 + 32
					}
					if c1 != c2 {
						match = false
						break
					}
				}
				if match {
					return true
				}
			}
		}
	}
	return false
}