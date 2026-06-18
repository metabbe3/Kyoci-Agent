package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/metabbe3/Kyoci-Agent/pkg"
)

// SearchResult represents a single search result.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"content"`
}

// SearchTool implements the kyoci.Tool interface for web search.
type SearchTool struct {
	client       *http.Client
	logger       *slog.Logger
	searchAPIURL string // Configurable search API endpoint
}

// NewSearchTool creates a new search tool instance.
func NewSearchTool() *SearchTool {
	return &SearchTool{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:       slog.Default(),
		searchAPIURL: "https://searx.be/search", // Default to SearX public instance
	}
}

// Name returns the tool name.
func (s *SearchTool) Name() string {
	return "web_search"
}

// Description returns the tool description.
func (s *SearchTool) Description() string {
	return "Search the web using a configurable search API (SearXNG compatible). Returns formatted results with title, URL, and snippet."
}

// Parameters returns the tool parameter definition.
func (s *SearchTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{
			Name:        "query",
			Type:        "string",
			Description: "The search query string",
			Required:    true,
		},
		{
			Name:        "limit",
			Type:        "integer",
			Description: "Maximum number of results to return (default: 5, max: 20)",
			Required:    false,
			Default:     5,
		},
	}
}

// Execute performs a web search.
//
// Parameters:
//   - ctx: Context for cancellation
//   - params: Map containing "query" (required) and optionally "limit"
//
// Returns:
//   - string: Formatted search results
//   - error: Error if search fails
func (s *SearchTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// Extract query
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("query parameter is required and must be a string")
	}

	// Extract limit (default 5, max 20)
	limit := 5
	if limitVal, ok := params["limit"]; ok {
		switch v := limitVal.(type) {
		case int:
			limit = v
		case float64:
			limit = int(v)
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 20 {
		limit = 20
	}

	s.logger.Info("performing web search", "query", query, "limit", limit)

	// Create request context with timeout
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Build search API URL
	searchURL, err := s.buildSearchURL(query, limit)
	if err != nil {
		return "", fmt.Errorf("failed to build search URL: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(reqCtx, "GET", searchURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Kyoci-Agent/1.0")
	req.Header.Set("Accept", "application/json")

	// Make request
	resp, err := s.client.Do(req)
	if err != nil {
		if reqCtx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("search request timed out")
		}
		return "", fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("search API returned status: %d", resp.StatusCode)
	}

	// Parse response
	var apiResponse struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		// Try parsing as HTML (SearX fallback)
		return s.parseHTMLSearchResults(resp.Body, query, limit)
	}

	// Check for results
	if len(apiResponse.Results) == 0 {
		s.logger.Warn("no search results found", "query", query)
		return fmt.Sprintf("No results found for query: %s", query), nil
	}

	// Limit results
	if len(apiResponse.Results) > limit {
		apiResponse.Results = apiResponse.Results[:limit]
	}

	// Format results
	return s.formatResults(apiResponse.Results), nil
}

// buildSearchURL builds the search API URL with parameters.
func (s *SearchTool) buildSearchURL(query string, limit int) (string, error) {
	baseURL, err := url.Parse(s.searchAPIURL)
	if err != nil {
		return "", fmt.Errorf("invalid search API URL: %w", err)
	}

	params := url.Values{}
	params.Add("q", query)
	params.Add("format", "json")
	params.Add("language", "en")
	params.Add("pageno", "1")
	params.Add("engines", "google,bing,duckduckgo")

	fullURL := baseURL.String() + "?" + params.Encode()
	return fullURL, nil
}

// parseHTMLSearchResults is a fallback for parsing HTML search results.
func (s *SearchTool) parseHTMLSearchResults(bodyReader any, query string, limit int) (string, error) {
	// For simplicity, return an error if HTML parsing is needed
	// In production, you'd use an HTML parser like goquery
	return "", fmt.Errorf("search API returned unexpected response format. Please ensure the search API endpoint returns JSON")
}

// formatResults formats search results for display.
func (s *SearchTool) formatResults(results []struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Found %d result(s):\n\n", len(results)))

	for i, result := range results {
		builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, result.Title))
		builder.WriteString(fmt.Sprintf("   URL: %s\n", result.URL))
		builder.WriteString(fmt.Sprintf("   %s\n\n", result.Content))
	}

	return builder.String()
}

// SetSearchAPIURL sets the search API endpoint.
func (s *SearchTool) SetSearchAPIURL(url string) {
	s.searchAPIURL = url
}

// SetTimeout sets the request timeout.
func (s *SearchTool) SetTimeout(timeout time.Duration) {
	s.client.Timeout = timeout
}
