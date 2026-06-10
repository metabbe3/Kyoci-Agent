package codegraph

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strings"
)

// HybridSearch combines code graph analysis with vector search
type HybridSearch struct {
	graph *CodeGraph
	index *Index
}

// SearchContext provides context for a search query
type SearchContext struct {
	Query          string
	Results        []SearchResult
	Dependencies   []CallEdge
	RelatedStructs []StructInfo
	ImpactFiles    []string
}

// NewHybridSearch creates a new hybrid search instance
func NewHybridSearch(graph *CodeGraph, index *Index) *HybridSearch {
	return &HybridSearch{
		graph: graph,
		index: index,
	}
}

// Search performs a hybrid search combining vector search and code graph analysis
func (h *HybridSearch) Search(query string, limit int) *SearchContext {
	ctx := &SearchContext{
		Query: query,
	}

	// Perform vector search
	ctx.Results = h.index.Search(query, limit)

	// Extract dependencies from search results
	ctx.Dependencies = h.extractDependencies(query)

	// Find related structs based on query terms
	ctx.RelatedStructs = h.findRelatedStructs(query)

	// Compute impact files
	ctx.ImpactFiles = h.computeImpactFiles(ctx.Results, ctx.Dependencies)

	return ctx
}

// GetContext returns formatted context for AI consumption
func (h *HybridSearch) GetContext(query string, maxTokens int) string {
	ctx := h.Search(query, 10)

	context := fmt.Sprintf("Query: %s\n\n", query)

	// Add search results
	if len(ctx.Results) > 0 {
		context += "Relevant Files:\n"
		for i, result := range ctx.Results {
			context += fmt.Sprintf("%d. %s (score: %.3f)\n", i+1, result.Path, result.Score)
			if result.Snippet != "" {
				context += fmt.Sprintf("   %s\n", result.Snippet)
			}
		}
		context += "\n"
	}

	// Add dependencies
	if len(ctx.Dependencies) > 0 {
		context += "Function Dependencies:\n"
		for _, dep := range ctx.Dependencies {
			context += fmt.Sprintf("  %s calls %s (%s)\n", dep.Caller, dep.Callee, dep.File)
		}
		context += "\n"
	}

	// Add related structs
	if len(ctx.RelatedStructs) > 0 {
		context += "Related Structs:\n"
		for _, st := range ctx.RelatedStructs {
			context += fmt.Sprintf("  - %s (%s:%d)\n", st.Name, st.File, st.Line)
			if len(st.Fields) > 0 {
				context += "    Fields:\n"
				for _, f := range st.Fields {
					if f.Name != "" {
						context += fmt.Sprintf("      %s %s", f.Name, f.Type)
						if f.Tag != "" {
							context += fmt.Sprintf(" `%s`", f.Tag)
						}
						context += "\n"
					}
				}
			}
		}
		context += "\n"
	}

	// Add impact files
	if len(ctx.ImpactFiles) > 0 {
		context += "Impact Files:\n"
		for _, file := range ctx.ImpactFiles {
			context += fmt.Sprintf("  - %s\n", file)
		}
	}

	// Token limit check (rough approximation)
	if maxTokens > 0 && len(context) > maxTokens*4 {
		context = context[:maxTokens*4] + "\n... (truncated)"
	}

	return context
}

// extractDependencies extracts dependencies based on query
func (h *HybridSearch) extractDependencies(query string) []CallEdge {
	deps := make([]CallEdge, 0)

	// Find functions matching query
	funcs := h.graph.FindFunction(query)
	for _, fn := range funcs {
		// Find callees
		callees := h.graph.FindCallees(fn.Name)
		deps = append(deps, callees...)

		// Find callers
		callers := h.graph.FindCallers(fn.Name)
		deps = append(deps, callers...)
	}

	// Find structs matching query
	structs := h.graph.FindStruct(query)
	for _, st := range structs {
		// For each method in the struct, find dependencies
		for _, methodName := range st.Methods {
			callees := h.graph.FindCallees(methodName)
			deps = append(deps, callees...)
		}
	}

	// Remove duplicates
	uniqueDeps := make(map[string]CallEdge)
	for _, dep := range deps {
		key := fmt.Sprintf("%s:%s:%s", dep.Caller, dep.Callee, dep.File)
		uniqueDeps[key] = dep
	}

	result := make([]CallEdge, 0, len(uniqueDeps))
	for _, dep := range uniqueDeps {
		result = append(result, dep)
	}

	return result
}

// findRelatedStructs finds structs related to the query
func (h *HybridSearch) findRelatedStructs(query string) []StructInfo {
	// Direct struct match
	structs := h.graph.FindStruct(query)

	// Find structs in files matching query
	results := h.index.Search(query, 5)
	for _, result := range results {
		for _, pkg := range h.graph.Packages {
			for _, st := range pkg.Structs {
				if strings.HasSuffix(st.File, result.Path) {
					// Check for duplicates
					found := false
					for _, existing := range structs {
						if existing.Name == st.Name && existing.File == st.File {
							found = true
							break
						}
					}
					if !found {
						structs = append(structs, st)
					}
				}
			}
		}
	}

	return structs
}

// computeImpactFiles computes files that would be impacted by changes
func (h *HybridSearch) computeImpactFiles(results []SearchResult, deps []CallEdge) []string {
	impactFiles := make(map[string]bool)

	// Add files from search results
	for _, result := range results {
		impactFiles[result.Path] = true
	}

	// Add files from dependencies
	for _, dep := range deps {
		impactFiles[dep.File] = true

		// Find the file containing the callee
		if funcInfos := h.graph.FindFunction(dep.Callee); len(funcInfos) > 0 {
			impactFiles[funcInfos[0].File] = true
		}

		// Find the file containing the caller
		if funcInfos := h.graph.FindFunction(dep.Caller); len(funcInfos) > 0 {
			impactFiles[funcInfos[0].File] = true
		}
	}

	// Sort files alphabetically
	sortedFiles := make([]string, 0, len(impactFiles))
	for file := range impactFiles {
		sortedFiles = append(sortedFiles, file)
	}
	sort.Strings(sortedFiles)

	return sortedFiles
}

// indexDirectory indexes all Go files in a directory into the vector index
func (h *HybridSearch) indexDirectory(dir string) error {
	goFiles, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return fmt.Errorf("failed to find .go files: %w", err)
	}

	for _, file := range goFiles {
		relPath := file
		if absPath, err := filepath.Abs(file); err == nil {
			if baseDir, err := filepath.Abs(dir); err == nil {
				if rel, err := filepath.Rel(baseDir, absPath); err == nil {
					relPath = rel
				}
			}
		}

		content, err := ioutil.ReadFile(file)
		if err != nil {
			continue
		}

		h.index.AddDoc(relPath, file, string(content))
	}

	return nil
}