package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewSQLiteMemory(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create SQLite memory
	sqlite, err := NewSQLiteMemory(dbPath, "")
	if err != nil {
		t.Fatalf("Failed to create SQLite memory: %v", err)
	}
	defer sqlite.Close()

	if !sqlite.initialized {
		t.Error("SQLite memory not initialized")
	}
}

func TestSQLiteAddAndGetEntry(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	sqlite, err := NewSQLiteMemory(dbPath, "")
	if err != nil {
		t.Fatalf("Failed to create SQLite memory: %v", err)
	}
	defer sqlite.Close()

	// Create test entry
	entry := &MemoryEntry{
		ID:          "test1",
		Category:    CategoryFact,
		Content:     "Test content",
		Importance:  5,
		AccessCount: 0,
	}

	// Add entry
	if err := sqlite.AddEntry(entry); err != nil {
		t.Fatalf("Failed to add entry: %v", err)
	}

	// Get entry
	retrieved, err := sqlite.GetEntry("test1")
	if err != nil {
		t.Fatalf("Failed to get entry: %v", err)
	}

	if retrieved.Content != "Test content" {
		t.Errorf("Content mismatch: got %s, want Test content", retrieved.Content)
	}

	if retrieved.Category != CategoryFact {
		t.Errorf("Category mismatch: got %s, want %s", retrieved.Category, CategoryFact)
	}
}

func TestSQLiteSearch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	sqlite, err := NewSQLiteMemory(dbPath, "")
	if err != nil {
		t.Fatalf("Failed to create SQLite memory: %v", err)
	}
	defer sqlite.Close()

	// Add test entries
	entries := []*MemoryEntry{
		{
			ID:          "fact1",
			Category:    CategoryFact,
			Content:     "The user likes Python programming",
			Importance:  7,
		},
		{
			ID:          "pref1",
			Category:    CategoryPreference,
			Content:     "User prefers dark mode",
			Importance:  5,
		},
	}

	for _, entry := range entries {
		if err := sqlite.AddEntry(entry); err != nil {
			t.Fatalf("Failed to add entry: %v", err)
		}
	}

	// Search for "Python"
	results, err := sqlite.Search("Python")
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}

	if len(results) == 0 {
		t.Error("Search returned no results")
	}

	found := false
	for _, r := range results {
		if r.ID == "fact1" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected to find fact1 in search results")
	}
}

func TestNewLongTermMemory(t *testing.T) {
	tmpDir := t.TempDir()

	mem, err := NewLongTermMemory(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create long term memory: %v", err)
	}
	defer mem.Close()

	// Test adding entry
	entry := &MemoryEntry{
		ID:          "test1",
		Category:    CategoryFact,
		Content:     "Test content",
		Importance:  5,
	}

	if err := mem.AddEntry(entry); err != nil {
		t.Fatalf("Failed to add entry: %v", err)
	}

	// Test getting entry
	retrieved, err := mem.GetEntry("test1")
	if err != nil {
		t.Fatalf("Failed to get entry: %v", err)
	}

	if retrieved.Content != "Test content" {
		t.Errorf("Content mismatch: got %s, want Test content", retrieved.Content)
	}

	// Test search
	results, err := mem.Search("Test")
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}

	if len(results) == 0 {
		t.Error("Search returned no results")
	}
}

func TestSQLiteMigrateFromJSON(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	jsonPath := filepath.Join(tmpDir, "memory.json")

	// Create test JSON file
	jsonContent := `[{
		"id": "json1",
		"category": "fact",
		"content": "Migrated from JSON",
		"created_at": "2024-01-01T00:00:00Z",
		"access_count": 0,
		"importance": 5
	}]`

	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("Failed to write JSON: %v", err)
	}

	// Create SQLite memory
	sqlite, err := NewSQLiteMemory(dbPath, "")
	if err != nil {
		t.Fatalf("Failed to create SQLite memory: %v", err)
	}
	defer sqlite.Close()

	// Migrate from JSON
	if err := sqlite.MigrateFromJSON(jsonPath); err != nil {
		t.Fatalf("Failed to migrate: %v", err)
	}

	// Check that entry was migrated
	retrieved, err := sqlite.GetEntry("json1")
	if err != nil {
		t.Fatalf("Failed to get migrated entry: %v", err)
	}

	if retrieved.Content != "Migrated from JSON" {
		t.Errorf("Content mismatch: got %s, want Migrated from JSON", retrieved.Content)
	}

	// Check that JSON was backed up
	backupPath := jsonPath + ".bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("JSON backup file was not created")
	}
}

func TestSQLiteStats(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	sqlite, err := NewSQLiteMemory(dbPath, "")
	if err != nil {
		t.Fatalf("Failed to create SQLite memory: %v", err)
	}
	defer sqlite.Close()

	// Add test entries
	entries := []*MemoryEntry{
		{ID: "test1", Category: CategoryFact, Content: "Test 1", Importance: 5},
		{ID: "test2", Category: CategoryPreference, Content: "Test 2", Importance: 5},
		{ID: "test3", Category: CategoryFact, Content: "Test 3", Importance: 5},
	}

	for _, entry := range entries {
		if err := sqlite.AddEntry(entry); err != nil {
			t.Fatalf("Failed to add entry: %v", err)
		}
	}

	// Get stats
	stats, err := sqlite.Stats(context.Background())
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	if stats.TotalEntries != 3 {
		t.Errorf("Expected 3 entries, got %d", stats.TotalEntries)
	}

	if stats.Categories["fact"] != 2 {
		t.Errorf("Expected 2 fact entries, got %d", stats.Categories["fact"])
	}

	if stats.Categories["preference"] != 1 {
		t.Errorf("Expected 1 preference entry, got %d", stats.Categories["preference"])
	}
}