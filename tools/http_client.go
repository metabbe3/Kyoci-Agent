package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPClientTool performs HTTP requests with authentication support
type HTTPClientTool struct {
	client *http.Client
}

// NewHTTPClientTool creates a new HTTP client tool
func NewHTTPClientTool() *HTTPClientTool {
	return &HTTPClientTool{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *HTTPClientTool) Name() string {
	return "http_client"
}

func (t *HTTPClientTool) Description() string {
	return "Execute HTTP requests (GET, POST, PUT, DELETE, PATCH) with custom headers, body, and authentication (none, bearer, basic). Returns status code, headers, and response body (truncated to 50KB)."
}

func (t *HTTPClientTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"method": map[string]interface{}{
				"type":        "string",
				"description": "HTTP method to use",
				"enum":        []string{"GET", "POST", "PUT", "DELETE", "PATCH"},
			},
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to send the request to",
			},
			"headers": map[string]interface{}{
				"type":        "object",
				"description": "HTTP headers as key-value pairs",
			},
			"body": map[string]interface{}{
				"type":        "string",
				"description": "Request body (for POST, PUT, PATCH methods)",
			},
			"auth_type": map[string]interface{}{
				"type":        "string",
				"description": "Authentication type",
				"enum":        []string{"none", "bearer", "basic"},
			},
			"auth_token": map[string]interface{}{
				"type":        "string",
				"description": "Authentication token (for bearer or basic auth)",
			},
		},
		"required": []string{"method", "url"},
	}
}

type httpParams struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body"`
	AuthType  string            `json:"auth_type"`
	AuthToken string            `json:"auth_token"`
}

func (t *HTTPClientTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var params httpParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	// Validate required parameters
	if params.Method == "" {
		return "", fmt.Errorf("method is required")
	}
	if params.URL == "" {
		return "", fmt.Errorf("url is required")
	}

	// Validate method
	validMethods := map[string]bool{"GET": true, "POST": true, "PUT": true, "DELETE": true, "PATCH": true}
	if !validMethods[params.Method] {
		return "", fmt.Errorf("invalid method: %s (valid: GET, POST, PUT, DELETE, PATCH)", params.Method)
	}

	// Validate auth type
	if params.AuthType != "" && params.AuthType != "none" && params.AuthType != "bearer" && params.AuthType != "basic" {
		return "", fmt.Errorf("invalid auth_type: %s (valid: none, bearer, basic)", params.AuthType)
	}

	// Validate body for non-GET/DELETE methods
	if (params.Method == "POST" || params.Method == "PUT" || params.Method == "PATCH") && params.Body == "" {
		params.Body = ""
	}

	// Create request
	var bodyReader io.Reader
	if params.Body != "" {
		bodyReader = strings.NewReader(params.Body)
	} else {
		bodyReader = nil
	}

	req, err := http.NewRequestWithContext(ctx, params.Method, params.URL, bodyReader)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set custom headers
	for key, value := range params.Headers {
		req.Header.Set(key, value)
	}

	// Set authentication
	switch params.AuthType {
	case "bearer":
		if params.AuthToken == "" {
			return "", fmt.Errorf("auth_token is required for bearer auth")
		}
		req.Header.Set("Authorization", "Bearer "+params.AuthToken)
	case "basic":
		if params.AuthToken == "" {
			return "", fmt.Errorf("auth_token is required for basic auth")
		}
		// Assume auth_token is "username:password"
		encoded := base64.StdEncoding.EncodeToString([]byte(params.AuthToken))
		req.Header.Set("Authorization", "Basic "+encoded)
	}

	// Set default Content-Type for methods with body
	if params.Body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Execute request
	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Truncate body to 50KB
	maxBodySize := 50 * 1024
	bodyStr := string(respBody)
	if len(bodyStr) > maxBodySize {
		bodyStr = bodyStr[:maxBodySize] + "... (truncated to 50KB)"
	}

	// Build headers string
	var headers []string
	for key, values := range resp.Header {
		for _, value := range values {
			headers = append(headers, fmt.Sprintf("%s: %s", key, value))
		}
	}

	// Format response
	result := fmt.Sprintf("Status: %d %s\n\nHeaders:\n%s\n\nBody:\n%s",
		resp.StatusCode, resp.Status,
		strings.Join(headers, "\n"),
		bodyStr,
	)

	return result, nil
}