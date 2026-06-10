package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ModalExecutor provides sandboxed code execution via Modal API
type ModalExecutor struct {
	token      string
	baseURL    string
	enabled    bool
	httpClient *http.Client
	timeout    time.Duration
}

// ModalExecRequest represents a request to Modal API
type ModalExecRequest struct {
	Script string `json:"script"`
	Lang   string `json:"lang"`
}

// ModalExecResponse represents a response from Modal API
type ModalExecResponse struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
}

// NewModalExecutor creates a new Modal executor instance
func NewModalExecutor(token string) *ModalExecutor {
	if token == "" {
		return &ModalExecutor{
			token:   "",
			baseURL: "https://api.modal.com",
			enabled: false,
			httpClient: &http.Client{
				Timeout: 120 * time.Second,
			},
			timeout: 30 * time.Second,
		}
	}

	return &ModalExecutor{
		token:   token,
		baseURL: "https://api.modal.com",
		enabled: true,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		timeout: 30 * time.Second,
	}
}

// IsAvailable checks if Modal API is available
func (m *ModalExecutor) IsAvailable() bool {
	return m.enabled && m.token != ""
}

// Exec executes a script via Modal API
func (m *ModalExecutor) Exec(ctx context.Context, script string, lang string) (*ExecResult, error) {
	if !m.enabled {
		return nil, fmt.Errorf("modal executor is not enabled (no token provided)")
	}

	startTime := time.Now()

	// Add timeout to context if not already set
	execCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, m.timeout)
		defer cancel()
	}

	// Prepare request
	reqBody := ModalExecRequest{
		Script: script,
		Lang:   lang,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/v1/sandbox/exec", m.baseURL)
	req, err := http.NewRequestWithContext(execCtx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.token)

	// Execute request
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("modal API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var result ModalExecResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		// If parsing fails, try to interpret as plain text
		return &ExecResult{
			ExitCode: 0,
			Stdout:   string(respBody),
			Stderr:   "",
			Duration: time.Since(startTime),
			TimedOut: execCtx.Err() == context.DeadlineExceeded,
		}, nil
	}

	return &ExecResult{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		Duration: time.Since(startTime),
		TimedOut: execCtx.Err() == context.DeadlineExceeded,
	}, nil
}

// ExecFile executes a file via Modal API
func (m *ModalExecutor) ExecFile(ctx context.Context, filePath string, lang string) (*ExecResult, error) {
	if !m.enabled {
		return nil, fmt.Errorf("modal executor is not enabled (no token provided)")
	}

	// Read file content
	// Note: This is a simplified implementation
	// In production, you'd read the file and pass its content to Exec
	return nil, fmt.Errorf("ExecFile not implemented for Modal executor")
}

// PollForCompletion polls for execution completion (for async execution)
func (m *ModalExecutor) PollForCompletion(ctx context.Context, execID string, pollInterval time.Duration) (*ExecResult, error) {
	if !m.enabled {
		return nil, fmt.Errorf("modal executor is not enabled")
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return &ExecResult{
				ExitCode: -1,
				Stdout:   "",
				Stderr:   "execution cancelled",
				Duration: 0,
				TimedOut: ctx.Err() == context.DeadlineExceeded,
			}, ctx.Err()

		case <-ticker.C:
			// Check execution status
			url := fmt.Sprintf("%s/v1/sandbox/exec/%s", m.baseURL, execID)
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}

			req.Header.Set("Authorization", "Bearer "+m.token)

			resp, err := m.httpClient.Do(req)
			if err != nil {
				return nil, fmt.Errorf("failed to check status: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusNotFound {
				return nil, fmt.Errorf("execution ID not found")
			}

			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to read response: %w", err)
			}

			var result ModalExecResponse
			if err := json.Unmarshal(respBody, &result); err != nil {
				continue // Retry on parse errors
			}

			// Check if execution is complete
			if result.Status == "completed" || result.Status == "failed" {
				return &ExecResult{
					ExitCode: result.ExitCode,
					Stdout:   result.Stdout,
					Stderr:   result.Stderr,
					Duration: 0, // Would need to track start time separately
					TimedOut: false,
				}, nil
			}
		}
	}
}

// SetTimeout sets the execution timeout
func (m *ModalExecutor) SetTimeout(timeout time.Duration) {
	m.timeout = timeout
}

// GetTimeout returns the execution timeout
func (m *ModalExecutor) GetTimeout() time.Duration {
	return m.timeout
}

// SetBaseURL sets the API base URL
func (m *ModalExecutor) SetBaseURL(url string) {
	m.baseURL = url
}

// GetBaseURL returns the API base URL
func (m *ModalExecutor) GetBaseURL() string {
	return m.baseURL
}

// IsEnabled returns whether the executor is enabled
func (m *ModalExecutor) IsEnabled() bool {
	return m.enabled
}

// GetToken returns the API token (for testing purposes)
func (m *ModalExecutor) GetToken() string {
	return m.token
}