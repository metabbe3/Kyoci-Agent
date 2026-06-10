package codegraph

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CodeKnowledge provides a unified API for code intelligence
type CodeKnowledge struct {
	graph      *CodeGraph
	search     *HybridSearch
	rootDir    string
	lastIndexed time.Time
	autoReindex bool
	stopChan   chan struct{}
	mu         sync.RWMutex
}

// NewCodeKnowledge creates a new CodeKnowledge instance
func NewCodeKnowledge(rootDir string) *CodeKnowledge {
	graph := NewCodeGraph()
	index := NewIndex()
	search := NewHybridSearch(graph, index)

	return &CodeKnowledge{
		graph:      graph,
		search:     search,
		rootDir:    rootDir,
		autoReindex: true,
		stopChan:   make(chan struct{}),
	}
}

// Initialize parses all code and builds the index
func (ck *CodeKnowledge) Initialize() error {
	ck.mu.Lock()
	defer ck.mu.Unlock()

	// Parse directory into code graph
	if err := ck.graph.ParseDir(ck.rootDir); err != nil {
		return fmt.Errorf("failed to parse code graph: %w", err)
	}

	// Build vector index
	if err := ck.buildIndex(); err != nil {
		return fmt.Errorf("failed to build index: %w", err)
	}

	ck.lastIndexed = time.Now()
	return nil
}

// Query performs a code knowledge query
func (ck *CodeKnowledge) Query(query string) (*SearchContext, error) {
	ck.mu.RLock()
	defer ck.mu.RUnlock()

	if ck.search == nil {
		return nil, fmt.Errorf("search not initialized")
	}

	return ck.search.Search(query, 10), nil
}

// Reindex rebuilds the index from disk
func (ck *CodeKnowledge) Reindex() error {
	ck.mu.Lock()
	defer ck.mu.Unlock()

	// Clear existing graph and index
	ck.graph = NewCodeGraph()
	index := NewIndex()
	ck.search = NewHybridSearch(ck.graph, index)

	// Reparse directory
	if err := ck.graph.ParseDir(ck.rootDir); err != nil {
		return fmt.Errorf("failed to parse code graph: %w", err)
	}

	// Rebuild index
	if err := ck.buildIndex(); err != nil {
		return fmt.Errorf("failed to build index: %w", err)
	}

	ck.lastIndexed = time.Now()
	return nil
}

// WatchChanges monitors the directory for file changes and reindexes automatically
func (ck *CodeKnowledge) WatchChanges(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		lastModTimes := make(map[string]time.Time)

		for {
			select {
			case <-ticker.C:
				if !ck.autoReindex {
					continue
				}

				changed := false

				// Check for new or modified files
				goFiles, err := filepath.Glob(filepath.Join(ck.rootDir, "*.go"))
				if err != nil {
					continue
				}

				for _, file := range goFiles {
					info, err := os.Stat(file)
					if err != nil {
						continue
					}

					lastMod, ok := lastModTimes[file]
					if !ok || info.ModTime().After(lastMod) {
						lastModTimes[file] = info.ModTime()
						changed = true
					}
				}

				// Check for deleted files
				for file := range lastModTimes {
					if _, err := os.Stat(file); os.IsNotExist(err) {
						delete(lastModTimes, file)
						changed = true
					}
				}

				// Reindex if changes detected
				if changed {
					ck.mu.Lock()
					if err := ck.Reindex(); err != nil {
						fmt.Printf("Failed to reindex: %v\n", err)
					} else {
						fmt.Printf("Reindexed codebase at %s\n", time.Now().Format(time.RFC3339))
					}
					ck.mu.Unlock()
				}
			case <-ck.stopChan:
				return
			}
		}
	}()
}

// GetContext returns formatted context for AI consumption
func (ck *CodeKnowledge) GetContext(query string, maxTokens int) (string, error) {
	ck.mu.RLock()
	defer ck.mu.RUnlock()

	if ck.search == nil {
		return "", fmt.Errorf("search not initialized")
	}

	return ck.search.GetContext(query, maxTokens), nil
}

// FindFunction searches for a function by name
func (ck *CodeKnowledge) FindFunction(name string) []FuncInfo {
	ck.mu.RLock()
	defer ck.mu.RUnlock()
	return ck.graph.FindFunction(name)
}

// FindStruct searches for a struct by name
func (ck *CodeKnowledge) FindStruct(name string) []StructInfo {
	ck.mu.RLock()
	defer ck.mu.RUnlock()
	return ck.graph.FindStruct(name)
}

// FindCallers finds functions that call the given function
func (ck *CodeKnowledge) FindCallers(funcName string) []CallEdge {
	ck.mu.RLock()
	defer ck.mu.RUnlock()
	return ck.graph.FindCallers(funcName)
}

// FindCallees finds functions called by the given function
func (ck *CodeKnowledge) FindCallees(funcName string) []CallEdge {
	ck.mu.RLock()
	defer ck.mu.RUnlock()
	return ck.graph.FindCallees(funcName)
}

// GetDependencies returns dependencies for a file
func (ck *CodeKnowledge) GetDependencies(file string) []string {
	ck.mu.RLock()
	defer ck.mu.RUnlock()
	return ck.graph.GetDependencies(file)
}

// Stats returns a summary of the indexed codebase
func (ck *CodeKnowledge) Stats() string {
	ck.mu.RLock()
	defer ck.mu.RUnlock()

	stats := ck.graph.Stats()
	return fmt.Sprintf("Code Knowledge Statistics:\n"+
		"  Packages: %d\n"+
		"  Functions: %d\n"+
		"  Structs: %d\n"+
		"  Interfaces: %d\n"+
		"  Call Edges: %d\n"+
		"  Root Directory: %s\n"+
		"  Last Indexed: %s\n",
		stats.Packages,
		stats.Functions,
		stats.Structs,
		stats.Interfaces,
		stats.CallEdges,
		ck.rootDir,
		ck.lastIndexed.Format(time.RFC3339))
}

// buildIndex builds the vector index from all Go files
func (ck *CodeKnowledge) buildIndex() error {
	goFiles, err := filepath.Glob(filepath.Join(ck.rootDir, "*.go"))
	if err != nil {
		return fmt.Errorf("failed to find .go files: %w", err)
	}

	for _, file := range goFiles {
		relPath := file
		if absPath, err := filepath.Abs(file); err == nil {
			if baseDir, err := filepath.Abs(ck.rootDir); err == nil {
				if rel, err := filepath.Rel(baseDir, absPath); err == nil {
					relPath = rel
				}
			}
		}

		content, err := ioutil.ReadFile(file)
		if err != nil {
			continue
		}

		ck.search.index.AddDoc(relPath, file, string(content))
	}

	return nil
}

// AddFile manually adds a file to the code knowledge
func (ck *CodeKnowledge) AddFile(path string) error {
	ck.mu.Lock()
	defer ck.mu.Unlock()

	// Parse file into graph
	if err := ck.graph.ParseFile(path); err != nil {
		return fmt.Errorf("failed to parse file: %w", err)
	}

	// Add to index
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	relPath := path
	if absPath, err := filepath.Abs(path); err == nil {
		if baseDir, err := filepath.Abs(ck.rootDir); err == nil {
			if rel, err := filepath.Rel(baseDir, absPath); err == nil {
				relPath = rel
			}
		}
	}

	ck.search.index.AddDoc(relPath, path, string(content))
	ck.lastIndexed = time.Now()

	return nil
}

// SetAutoReindex enables or disables automatic reindexing
func (ck *CodeKnowledge) SetAutoReindex(enabled bool) {
	ck.mu.Lock()
	defer ck.mu.Unlock()
	ck.autoReindex = enabled
}

// GetLastIndexed returns the last indexed time
func (ck *CodeKnowledge) GetLastIndexed() time.Time {
	ck.mu.RLock()
	defer ck.mu.RUnlock()
	return ck.lastIndexed
}

// GetRootDir returns the root directory
func (ck *CodeKnowledge) GetRootDir() string {
	ck.mu.RLock()
	defer ck.mu.RUnlock()
	return ck.rootDir
}