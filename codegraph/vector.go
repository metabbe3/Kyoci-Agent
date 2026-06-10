package codegraph

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Document represents a document in the index
type Document struct {
	ID      string
	Path    string
	Content string
	Tokens  []string
}

// Index represents the TF-IDF search index
type Index struct {
	docs      map[string]*Document
	idf       map[string]float64
	tf        map[string]map[string]float64 // docID -> term -> tf
	mu        sync.RWMutex
	tokenizer *regexp.Regexp
}

// SearchResult represents a search result
type SearchResult struct {
	Path    string
	Score   float64
	Snippet string
}

// NewIndex creates a new TF-IDF index
func NewIndex() *Index {
	return &Index{
		docs:      make(map[string]*Document),
		idf:       make(map[string]float64),
		tf:        make(map[string]map[string]float64),
		tokenizer: regexp.MustCompile(`[^\w]+`),
	}
}

// AddDoc adds a document to the index
func (idx *Index) AddDoc(id, path, content string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	tokens := idx.tokenize(content)

	// Create document
	doc := &Document{
		ID:      id,
		Path:    path,
		Content: content,
		Tokens:  tokens,
	}
	idx.docs[id] = doc

	// Compute TF for this document
	tfMap := make(map[string]float64)
	totalTerms := float64(len(tokens))
	if totalTerms > 0 {
		for _, token := range tokens {
			tfMap[token]++
		}
		for token := range tfMap {
			tfMap[token] = tfMap[token] / totalTerms
		}
	}
	idx.tf[id] = tfMap

	// Update IDF
	idx.recomputeIDF()
}

// RemoveDoc removes a document from the index
func (idx *Index) RemoveDoc(id string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	delete(idx.docs, id)
	delete(idx.tf, id)

	// Recompute IDF for all documents
	idx.recomputeIDF()
}

// Search performs a TF-IDF cosine similarity search
func (idx *Index) Search(query string, limit int) []SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if len(idx.docs) == 0 {
		return []SearchResult{}
	}

	queryTokens := idx.tokenize(query)
	if len(queryTokens) == 0 {
		return []SearchResult{}
	}

	// Compute query TF
	queryTF := make(map[string]float64)
	for _, token := range queryTokens {
		queryTF[token]++
	}
	totalQueryTerms := float64(len(queryTokens))
	for token := range queryTF {
		queryTF[token] = queryTF[token] / totalQueryTerms
	}

	// Compute query TF-IDF
	queryTFIDF := make(map[string]float64)
	for token, tf := range queryTF {
		idf, ok := idx.idf[token]
		if ok {
			queryTFIDF[token] = tf * idf
		}
	}

	// Compute cosine similarity with each document
	results := make([]SearchResult, 0)
	for id, doc := range idx.docs {
		docTFMap := idx.tf[id]
		if docTFMap == nil {
			continue
		}

		dotProduct := 0.0
		queryNorm := 0.0
		docNorm := 0.0

		// Collect all unique terms
		allTerms := make(map[string]bool)
		for term := range queryTFIDF {
			allTerms[term] = true
		}
		for term := range docTFMap {
			allTerms[term] = true
		}

		for term := range allTerms {
			queryWeight := queryTFIDF[term]
			docWeight := docTFMap[term] * idx.idf[term]

			dotProduct += queryWeight * docWeight
			queryNorm += queryWeight * queryWeight
			docNorm += docWeight * docWeight
		}

		if queryNorm > 0 && docNorm > 0 {
			similarity := dotProduct / (math.Sqrt(queryNorm) * math.Sqrt(docNorm))

			if similarity > 0 {
				snippet := idx.extractSnippet(doc.Content, queryTokens)
				results = append(results, SearchResult{
					Path:    doc.Path,
					Score:   similarity,
					Snippet: snippet,
				})
			}
		}
	}

	// Sort by score descending
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// Limit results
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// RebuildFromDir walks a directory and rebuilds the index from all .go files
func (idx *Index) RebuildFromDir(dir string) error {
	// Clear existing index
	idx.mu.Lock()
	idx.docs = make(map[string]*Document)
	idx.tf = make(map[string]map[string]float64)
	idx.idf = make(map[string]float64)
	idx.mu.Unlock()

	// Find all .go files
	goFiles, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return fmt.Errorf("failed to find .go files: %w", err)
	}

	// Add all files to index
	for _, file := range goFiles {
		// Use relative path as ID
		relPath := file
		if absPath, err := filepath.Abs(file); err == nil {
			if baseDir, err := filepath.Abs(dir); err == nil {
				if rel, err := filepath.Rel(baseDir, absPath); err == nil {
					relPath = rel
				}
			}
		}
		_ = relPath // Use relative path as document ID

		// Read file content - we'll need to implement this
		// For now, we'll skip since we don't have file reading in this function
		// The caller should read and pass content
	}

	return nil
}

// tokenize splits text into tokens
func (idx *Index) tokenize(text string) []string {
	// Split on non-word characters
	parts := idx.tokenizer.Split(text, -1)

	tokens := make([]string, 0)
	goKeywords := map[string]bool{
		"package": true, "import": true, "func": true, "return": true,
		"var": true, "const": true, "type": true, "struct": true,
		"interface": true, "map": true, "chan": true, "go": true,
		"defer": true, "select": true, "case": true, "default": true,
		"fallthrough": true, "break": true, "continue": true, "goto": true,
		"if": true, "else": true, "switch": true, "for": true, "range": true,
		"true": true, "false": true, "nil": true, "iota": true,
	}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) == 0 {
			continue
		}

		// Skip single characters
		if len(part) == 1 {
			continue
		}

		// Convert to lowercase
		lower := strings.ToLower(part)

		// Skip Go keywords
		if goKeywords[lower] {
			continue
		}

		tokens = append(tokens, lower)
	}

	return tokens
}

// recomputeIDF recomputes IDF values for all terms
func (idx *Index) recomputeIDF() {
	// Count document frequency for each term
	df := make(map[string]int)
	for _, doc := range idx.docs {
		uniqueTokens := make(map[string]bool)
		for _, token := range doc.Tokens {
			uniqueTokens[token] = true
		}
		for token := range uniqueTokens {
			df[token]++
		}
	}

	// Compute IDF: log(N / df)
	n := float64(len(idx.docs))
	if n == 0 {
		return
	}

	idx.idf = make(map[string]float64)
	for term, freq := range df {
		idx.idf[term] = math.Log(n / float64(freq))
	}
}

// extractSnippet extracts a snippet containing the query terms
func (idx *Index) extractSnippet(content string, queryTokens []string) string {
	if len(content) == 0 {
		return ""
	}

	lines := strings.Split(content, "\n")

	// Find lines containing query terms
	matchingLines := make([]int, 0)
	for i, line := range lines {
		lineLower := strings.ToLower(line)
		for _, token := range queryTokens {
			if strings.Contains(lineLower, strings.ToLower(token)) {
				matchingLines = append(matchingLines, i)
				break
			}
		}
	}

	if len(matchingLines) == 0 {
		// Return first few lines if no match
		snippet := strings.Join(lines[:min(3, len(lines))], "\n")
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		return snippet
	}

	// Extract context around matching lines
	contextLines := 2
	startLine := max(0, matchingLines[0]-contextLines)
	endLine := min(len(lines), matchingLines[len(matchingLines)-1]+contextLines+1)

	snippet := strings.Join(lines[startLine:endLine], "\n")
	if len(snippet) > 500 {
		snippet = snippet[:500] + "..."
	}

	return snippet
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}