package builtin

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/metabbe3/Kyoci-Agent/pkg"
)

// HTTPTool implements the kyoci.Tool interface for HTTP requests.
type HTTPTool struct {
	client          *http.Client
	logger          *slog.Logger
	maxResponseSize int64 // Maximum response size in bytes (default 1MB)
}

// NewHTTPTool creates a new HTTP tool instance.
func NewHTTPTool() *HTTPTool {
	return &HTTPTool{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:          slog.Default(),
		maxResponseSize: 1024 * 1024, // 1MB default
	}
}

// Name returns the tool name.
func (h *HTTPTool) Name() string {
	return "http_client"
}

// Description returns the tool description.
func (h *HTTPTool) Description() string {
	return "Make HTTP requests with configurable method, headers, body, and timeout. Returns status code and response body with size limits."
}

// Parameters returns the tool parameter definition.
func (h *HTTPTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{
			Name:        "url",
			Type:        "string",
			Description: "The URL to make the request to",
			Required:    true,
		},
		{
			Name:        "method",
			Type:        "string",
			Description: "HTTP method: GET, POST, PUT, DELETE (default: GET)",
			Required:    false,
			Default:     "GET",
			EnumValues:  []string{"GET", "POST", "PUT", "DELETE"},
		},
		{
			Name:        "headers",
			Type:        "object",
			Description: "HTTP headers as a map (optional)",
			Required:    false,
		},
		{
			Name:        "body",
			Type:        "string",
			Description: "Request body for POST/PUT requests (optional)",
			Required:    false,
		},
		{
			Name:        "timeout",
			Type:        "integer",
			Description: "Request timeout in seconds (default: 30)",
			Required:    false,
			Default:     30,
		},
	}
}

// Execute makes an HTTP request.
//
// Parameters:
//   - ctx: Context for cancellation
//   - params: Map containing "url" (required), and optionally "method", "headers", "body", "timeout"
//
// Returns:
//   - string: Response body with status code
//   - error: Error if request fails
func (h *HTTPTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// Extract URL
	url, ok := params["url"].(string)
	if !ok || url == "" {
		return "", fmt.Errorf("url parameter is required and must be a string")
	}

	// Validate URL
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("invalid URL: must start with http:// or https://")
	}

	// Extract method (default GET)
	method := "GET"
	if methodVal, ok := params["method"].(string); ok && methodVal != "" {
		method = strings.ToUpper(methodVal)
		// Validate method
		validMethods := map[string]bool{"GET": true, "POST": true, "PUT": true, "DELETE": true}
		if !validMethods[method] {
			return "", fmt.Errorf("invalid method: %s (must be GET, POST, PUT, or DELETE)", method)
		}
	}

	// Extract timeout (default 30 seconds)
	timeoutSeconds := 30
	if timeoutVal, ok := params["timeout"]; ok {
		switch v := timeoutVal.(type) {
		case int:
			timeoutSeconds = v
		case float64:
			timeoutSeconds = int(v)
		}
	}

	// Create request context with timeout
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	// Extract headers (optional)
	headers := make(map[string]string)
	if headersVal, ok := params["headers"].(map[string]interface{}); ok {
		for k, v := range headersVal {
			if strVal, ok := v.(string); ok {
				headers[k] = strVal
			}
		}
	}

	// Extract body (optional)
	var body io.Reader
	if bodyVal, ok := params["body"].(string); ok && method != "GET" && method != "DELETE" {
		body = strings.NewReader(bodyVal)
	}

	h.logger.Info("making HTTP request", "method", method, "url", url, "timeout", timeoutSeconds)

	// Create request
	req, err := http.NewRequestWithContext(reqCtx, method, url, body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Set Content-Type for POST/PUT if body provided and not already set
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Make request with timeout
	client := &http.Client{
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		if reqCtx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("request timed out after %d seconds", timeoutSeconds)
		}
		if strings.Contains(err.Error(), "no such host") || strings.Contains(err.Error(), "lookup") {
			return "", fmt.Errorf("DNS failure: %w", err)
		}
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response with size limit
	limitedReader := io.LimitReader(resp.Body, h.maxResponseSize)
	respBody, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check if response was truncated
	if int64(len(respBody)) == h.maxResponseSize {
		h.logger.Warn("response truncated due to size limit", "url", url, "size", h.maxResponseSize)
		respBody = append(respBody, []byte("\n[Response truncated due to size limit]")...)
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		h.logger.Warn("HTTP request returned non-2xx status", "status", resp.StatusCode, "url", url)
		return fmt.Sprintf("Status: %d %s\n\n%s", resp.StatusCode, resp.Status, string(respBody)), nil
	}

	h.logger.Info("HTTP request successful", "status", resp.StatusCode, "url", url)
	return fmt.Sprintf("Status: %d %s\n\n%s", resp.StatusCode, resp.Status, string(respBody)), nil
}

// SetMaxResponseSize sets the maximum response size.
func (h *HTTPTool) SetMaxResponseSize(size int64) {
	h.maxResponseSize = size
}

// SetTimeout sets the default request timeout.
func (h *HTTPTool) SetTimeout(timeout time.Duration) {
	h.client.Timeout = timeout
}
