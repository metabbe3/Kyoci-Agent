package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// WAL is a Write-Ahead Log for durable DAG execution
type WAL struct {
	mu     sync.Mutex
	dir    string                       // directory for WAL files
	active map[string]*os.File          // open WAL files per DAG
}

// WALEntry represents a single log entry for a DAG step
type WALEntry struct {
	DAGID    string    `json:"dag_id"`
	StepID   string    `json:"step_id"`
	Status   string    `json:"status"` // RUNNING, COMPLETED, FAILED
	Result   string    `json:"result,omitempty"`
	Error    string    `json:"error,omitempty"`
	TS       time.Time `json:"ts"`
}

// RecoveryTask represents an incomplete DAG that can be recovered
// Note: The Plan field should be populated by the caller from external storage
// as the WAL only tracks execution state, not the full DAG plan
type RecoveryTask struct {
	DAGID          string
	Plan           interface{} // Should be *DAGPlan when populated
	CompletedSteps map[string]string // stepID -> result
}

// NewWAL creates a new WAL instance with the specified directory
func NewWAL(dir string) (*WAL, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create WAL directory: %w", err)
	}

	return &WAL{
		dir:    dir,
		active: make(map[string]*os.File),
	}, nil
}

// Checkpoint writes a WAL entry for a DAG step
func (w *WAL) Checkpoint(dagID, stepID, status string, result string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Get or create WAL file for this DAG
	f, err := w.getOrCreateFile(dagID)
	if err != nil {
		return fmt.Errorf("failed to get WAL file: %w", err)
	}

	// Create entry
	entry := WALEntry{
		DAGID:  dagID,
		StepID: stepID,
		Status: status,
		Result: result,
		TS:     time.Now(),
	}

	// Marshal to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal WAL entry: %w", err)
	}

	// Append to file with newline
	data = append(data, '\n')
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write WAL entry: %w", err)
	}

	// Sync to disk for durability
	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to sync WAL file: %w", err)
	}

	return nil
}

// CheckpointWithError writes a WAL entry with an error message
func (w *WAL) CheckpointWithError(dagID, stepID, status string, errMsg string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Get or create WAL file for this DAG
	f, err := w.getOrCreateFile(dagID)
	if err != nil {
		return fmt.Errorf("failed to get WAL file: %w", err)
	}

	// Create entry
	entry := WALEntry{
		DAGID:  dagID,
		StepID: stepID,
		Status: status,
		Error:  errMsg,
		TS:     time.Now(),
	}

	// Marshal to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal WAL entry: %w", err)
	}

	// Append to file with newline
	data = append(data, '\n')
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write WAL entry: %w", err)
	}

	// Sync to disk for durability
	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to sync WAL file: %w", err)
	}

	return nil
}

// Recover reads all incomplete WAL files and returns recovery tasks
func (w *WAL) Recover() ([]RecoveryTask, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// List all WAL files (non-completed)
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read WAL directory: %w", err)
	}

	var recoveryTasks []RecoveryTask

	for _, entry := range entries {
		name := entry.Name()
		// Skip .completed files and directories
		if entry.IsDir() || strings.HasSuffix(name, ".completed") {
			continue
		}
		// Only process wal-*.jsonl files
		if !strings.HasPrefix(name, "wal-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}

		// Extract DAG ID from filename
		dagID := strings.TrimPrefix(name, "wal-")
		dagID = strings.TrimSuffix(dagID, ".jsonl")

		// Read and parse WAL file
		task, err := w.parseWALFile(dagID, filepath.Join(w.dir, name))
		if err != nil {
			// Log error but continue with other files
			fmt.Printf("Failed to parse WAL file %s: %v\n", name, err)
			continue
		}

		if task != nil {
			recoveryTasks = append(recoveryTasks, *task)
		}
	}

	return recoveryTasks, nil
}

// Complete marks a DAG as done by renaming its WAL file to .completed
func (w *WAL) Complete(dagID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Close the file if open
	if f, ok := w.active[dagID]; ok {
		f.Close()
		delete(w.active, dagID)
	}

	// Rename to .completed
	oldPath := filepath.Join(w.dir, fmt.Sprintf("wal-%s.jsonl", dagID))
	newPath := filepath.Join(w.dir, fmt.Sprintf("wal-%s.jsonl.completed", dagID))

	if err := os.Rename(oldPath, newPath); err != nil {
		// File might not exist if DAG never had checkpoints
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to rename WAL file: %w", err)
		}
	}

	return nil
}

// Compact removes completed WAL files older than 24 hours
func (w *WAL) Compact() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return fmt.Errorf("failed to read WAL directory: %w", err)
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	var removed int

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process .completed files
		if !strings.HasSuffix(entry.Name(), ".completed") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Remove if older than cutoff
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(w.dir, entry.Name())
			if err := os.Remove(path); err == nil {
				removed++
			}
		}
	}

	return nil
}

// Close closes all open WAL files
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var lastErr error
	for dagID, f := range w.active {
		if err := f.Close(); err != nil {
			lastErr = err
		}
		delete(w.active, dagID)
	}

	return lastErr
}

// getOrCreateFile gets an existing open file or creates a new one
func (w *WAL) getOrCreateFile(dagID string) (*os.File, error) {
	// Check if already open
	if f, ok := w.active[dagID]; ok {
		return f, nil
	}

	// Open file in append mode
	path := filepath.Join(w.dir, fmt.Sprintf("wal-%s.jsonl", dagID))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	w.active[dagID] = f
	return f, nil
}

// parseWALFile reads a WAL file and returns a RecoveryTask
func (w *WAL) parseWALFile(dagID string, path string) (*RecoveryTask, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read WAL file: %w", err)
	}

	task := &RecoveryTask{
		DAGID:          dagID,
		CompletedSteps: make(map[string]string),
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry WALEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Skip malformed lines
			continue
		}

		// Track completed steps
		if entry.Status == "COMPLETED" {
			task.CompletedSteps[entry.StepID] = entry.Result
		}
	}

	// If no completed steps, this might be an empty/corrupt WAL
	if len(task.CompletedSteps) == 0 {
		return nil, nil
	}

	return task, nil
}