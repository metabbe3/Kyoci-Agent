package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
	_ "modernc.org/sqlite"
)

// ==============================================================================
// Long-Term Memory
// ==============================================================================

// LongTermMemory manages persistent storage using SQLite with FTS5 full-text search.
// It is thread-safe (sql.DB handles this internally).
type LongTermMemory struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewLongTermMemory creates a new long-term memory instance with SQLite backend.
func NewLongTermMemory(dbPath string, logger *slog.Logger) (*LongTermMemory, error) {
	// Open database connection
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	ltm := &LongTermMemory{
		db:     db,
		logger: logger,
	}

	// Create schema
	if err := ltm.createSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	logger.Info("long-term memory initialized", "db_path", dbPath)
	return ltm, nil
}

// createSchema creates the database tables and FTS5 virtual table.
func (ltm *LongTermMemory) createSchema() error {
	schema := `
		-- Main memories table
		CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			type INTEGER NOT NULL,
			metadata_json TEXT,
			created_at TEXT NOT NULL,
			relevance_score REAL DEFAULT 0.0
		);

		-- FTS5 virtual table for full-text search
		CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
			content,
			content=memories,
			content_rowid=rowid
		);

		-- Triggers to keep FTS table in sync
		CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
			INSERT INTO memories_fts(rowid, content) VALUES (new.rowid, new.content);
		END;

		CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, content) VALUES('delete', old.rowid, old.content);
		END;

		CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, content) VALUES('delete', old.rowid, old.content);
			INSERT INTO memories_fts(rowid, content) VALUES (new.rowid, new.content);
		END;

		-- Indexes for common queries
		CREATE INDEX IF NOT EXISTS idx_memories_type ON memories(type);
		CREATE INDEX IF NOT EXISTS idx_memories_created_at ON memories(created_at);
	`

	_, err := ltm.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// Store stores content in long-term memory and returns the ID.
func (ltm *LongTermMemory) Store(content string, memType kyoci.MemoryType, metadata map[string]string) (string, error) {
	id := generateID()
	metadataJSON, _ := json.Marshal(metadata)
	createdAt := time.Now()

	query := `
		INSERT INTO memories (id, content, type, metadata_json, created_at, relevance_score)
		VALUES (?, ?, ?, ?, ?, 0.0)
	`

	_, err := ltm.db.Exec(query, id, content, int(memType), string(metadataJSON), createdAt.String())
	if err != nil {
		ltm.logger.Error("failed to store memory entry", "id", id, "error", err)
		return "", fmt.Errorf("failed to store memory entry: %w", err)
	}

	ltm.logger.Debug("memory stored in long-term", "id", id, "type", memType.String(), "size", len(content))
	return id, nil
}

// Recall retrieves memory entries relevant to a query using FTS5 with BM25 ranking.
func (ltm *LongTermMemory) Recall(query string, limit int, memType kyoci.MemoryType) ([]kyoci.MemoryEntry, error) {
	// Build the query based on whether we need type filtering
	var rows *sql.Rows
	var err error

	typeFilter := ""
	args := []interface{}{}

	if memType != 0 {
		typeFilter = "AND type = ?"
		args = append(args, int(memType))
	}

	// Empty query: skip FTS5 (it errors on empty MATCH), just return recent entries
	query = strings.TrimSpace(query)
	if query == "" {
		queryBuilder := fmt.Sprintf(`
			SELECT id, content, type, metadata_json, created_at, relevance_score
			FROM memories
			WHERE 1=1 %s
			ORDER BY created_at DESC
		`, typeFilter)
		if limit > 0 {
			queryBuilder += " LIMIT ?"
			args = append(args, limit)
		}
		rows, err = ltm.db.Query(queryBuilder, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to query memories: %w", err)
		}
		defer rows.Close()
	} else {
		// FTS5 full-text search
		queryBuilder := fmt.Sprintf(`
			SELECT m.id, m.content, m.type, m.metadata_json, m.created_at, m.relevance_score
			FROM memories m
			WHERE m.rowid IN (
				SELECT rowid FROM memories_fts
				WHERE memories_fts MATCH ?
				ORDER BY bm25(memories_fts)
			)
			%s
			ORDER BY m.created_at DESC
		`, typeFilter)
		if limit > 0 {
			queryBuilder += " LIMIT ?"
			args = append(args, limit)
		}
		ftsQuery := escapeFTSQuery(query)
		args = append([]interface{}{ftsQuery}, args...)
		rows, err = ltm.db.Query(queryBuilder, args...)
		if err != nil {
			ltm.logger.Warn("FTS5 search failed, falling back to keyword search", "error", err)
			return ltm.fallbackSearch(query, limit, memType)
		}
		defer rows.Close()
	}

	entries, err := ltm.scanEntries(rows)
	if err != nil {
		return nil, fmt.Errorf("failed to scan entries: %w", err)
	}

	ltm.logger.Debug("long-term memory recalled", "query", query, "results", len(entries), "limit", limit)
	return entries, nil
}

// Delete removes a memory entry by ID.
func (ltm *LongTermMemory) Delete(id string) error {
	query := `DELETE FROM memories WHERE id = ?`
	result, err := ltm.db.Exec(query, id)
	if err != nil {
		ltm.logger.Error("failed to delete memory entry", "id", id, "error", err)
		return fmt.Errorf("failed to delete memory entry: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return kyoci.ErrMemoryNotFound
	}

	ltm.logger.Debug("memory deleted from long-term", "id", id)
	return nil
}

// Search performs a hybrid search: FTS5 + keyword fallback.
func (ltm *LongTermMemory) Search(query string, limit int) []kyoci.MemoryEntry {
	entries, err := ltm.Recall(query, limit, 0)
	if err != nil {
		ltm.logger.Error("search failed", "query", query, "error", err)
		return []kyoci.MemoryEntry{}
	}
	return entries
}

// fallbackSearch performs a simple keyword search when FTS5 fails.
func (ltm *LongTermMemory) fallbackSearch(query string, limit int, memType kyoci.MemoryType) ([]kyoci.MemoryEntry, error) {
	queryBuilder := `
		SELECT id, content, type, metadata_json, created_at, relevance_score
		FROM memories
		WHERE content LIKE ?
	`
	args := []interface{}{"%" + escapeLikeQuery(query) + "%"}

	if memType != 0 {
		queryBuilder += " AND type = ?"
		args = append(args, int(memType))
	}

	queryBuilder += " ORDER BY created_at DESC"
	if limit > 0 {
		queryBuilder += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := ltm.db.Query(queryBuilder, args...)
	if err != nil {
		return nil, fmt.Errorf("fallback search failed: %w", err)
	}
	defer rows.Close()

	entries, err := ltm.scanEntries(rows)
	if err != nil {
		return nil, fmt.Errorf("failed to scan entries from fallback: %w", err)
	}

	return entries, nil
}

// scanEntries scans rows into a slice of MemoryEntry.
func (ltm *LongTermMemory) scanEntries(rows *sql.Rows) ([]kyoci.MemoryEntry, error) {
	entries := make([]kyoci.MemoryEntry, 0)
	for rows.Next() {
		var id, content, metadataJSON, createdAtStr string
		var memType int
		var relevanceScore float64

		if err := rows.Scan(&id, &content, &memType, &metadataJSON, &createdAtStr, &relevanceScore); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Parse metadata
		var metadata map[string]string
		if metadataJSON != "" {
			if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
				metadata = make(map[string]string)
			}
		}

		// Parse created_at
		createdAt, err := time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			// Fallback to Unix timestamp parsing
			t, err2 := time.Parse("2006-01-02 15:04:05.999999999-07:00", createdAtStr)
			if err2 != nil {
				createdAt = time.Now()
			} else {
				createdAt = t
			}
		}

		entries = append(entries, kyoci.MemoryEntry{
			ID:             id,
			Content:        content,
			Type:           kyoci.MemoryType(memType),
			Metadata:       metadata,
			CreatedAt:      createdAt,
			RelevanceScore: relevanceScore,
			Tags:           []string{},
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return entries, nil
}

// escapeFTSQuery escapes special characters for FTS5 search.
func escapeFTSQuery(query string) string {
	// Simple escaping: wrap terms in double quotes
	terms := strings.Fields(query)
	for i, term := range terms {
		if !strings.HasPrefix(term, "\"") {
			terms[i] = "\"" + strings.ReplaceAll(term, "\"", "\"\"") + "\""
		}
	}
	return strings.Join(terms, " ")
}

// escapeLikeQuery escapes special characters for SQL LIKE queries.
func escapeLikeQuery(query string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(query, "%", "\\%"), "_", "\\_"), "\\", "\\\\")
}

// Close closes the database connection.
func (ltm *LongTermMemory) Close() error {
	if ltm.db != nil {
		if err := ltm.db.Close(); err != nil {
			return fmt.Errorf("failed to close database: %w", err)
		}
		ltm.logger.Info("long-term memory closed")
	}
	return nil
}

// Stats returns statistics about the long-term memory.
func (ltm *LongTermMemory) Stats() MemoryStats {
	stats := MemoryStats{}

	// Get total count
	var totalCount, shortTermCount, longTermCount, skillCount, totalTokens sql.NullInt64

	ltm.db.QueryRow("SELECT COUNT(*) FROM memories").Scan(&totalCount)
	ltm.db.QueryRow("SELECT COUNT(*) FROM memories WHERE type = 0").Scan(&shortTermCount)
	ltm.db.QueryRow("SELECT COUNT(*) FROM memories WHERE type = 1").Scan(&longTermCount)
	ltm.db.QueryRow("SELECT COUNT(*) FROM memories WHERE type = 2").Scan(&skillCount)

	// Estimate tokens (rough: length/4 per entry)
	ltm.db.QueryRow("SELECT SUM(LENGTH(content) / 4) FROM memories").Scan(&totalTokens)

	if totalCount.Valid {
		stats.TotalEntries = int(totalCount.Int64)
	}
	if shortTermCount.Valid {
		stats.ShortTermEntries = int(shortTermCount.Int64)
	}
	if longTermCount.Valid {
		stats.LongTermEntries = int(longTermCount.Int64)
	}
	if skillCount.Valid {
		stats.SkillEntries = int(skillCount.Int64)
	}
	if totalTokens.Valid {
		stats.EstimatedTokens = int(totalTokens.Int64)
	}

	return stats
}