package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WebSearchTool searches the web using DuckDuckGo Instant Answer API
type WebSearchTool struct {
	client *http.Client
}

func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *WebSearchTool) Name() string { return "web_search" }

func (t *WebSearchTool) Description() string {
	return "Search the web for information. Returns top results with titles, URLs, and snippets."
}

func (t *WebSearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The search query",
			},
			"num_results": map[string]interface{}{
				"type":        "integer",
				"description": "Number of results to return (default 5)",
				"default":     5,
			},
		},
		"required": []string{"query"},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var params struct {
		Query      string `json:"query"`
		NumResults int    `json:"num_results"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}
	if params.NumResults <= 0 {
		params.NumResults = 5
	}

	// Use DuckDuckGo HTML search (no API key needed)
	reqURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(params.Query))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AIAgent/1.0)")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("search returned status %d", resp.StatusCode)
	}

	// Parse HTML results (simplified - extract from DDG HTML)
	body := make([]byte, 50000)
	n, _ := resp.Body.Read(body)
	content := string(body[:n])

	// Extract result links and snippets from DDG HTML
	var results []string
	// Simple parsing: look for result__a (title) and result__snippet
	titleStart := 0
	for i := 0; i < len(content) && len(results) < params.NumResults; i++ {
		if idx := strings.Index(content[titleStart:], "result__a"); idx != -1 {
			segment := content[titleStart+idx:]
			// Extract title text
			if start := strings.Index(segment, ">"); start != -1 {
				end := strings.Index(segment[start+1:], "<")
				if end != -1 {
					title := strings.TrimSpace(segment[start+1 : start+1+end])
					if title != "" {
						results = append(results, title)
					}
				}
			}
			titleStart += idx + 10
		} else {
			break
		}
	}

	if len(results) == 0 {
		return "No results found.", nil
	}

	output := fmt.Sprintf("Found %d results for '%s':\n\n", len(results), params.Query)
	for i, r := range results {
		output += fmt.Sprintf("%d. %s\n", i+1, r)
	}
	return output, nil
}
