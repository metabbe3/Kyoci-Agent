package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// StdioTransport communicates with MCP server via subprocess stdin/stdout
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr bytes.Buffer
	mu     sync.Mutex
	nextID int
	closed bool
}

// NewStdioTransport creates a new stdio transport for the given command
func NewStdioTransport(command string, args []string, env map[string]string) (*StdioTransport, error) {
	cmd := exec.Command(command, args...)

	// Set environment variables
	if env != nil {
		// Start with parent environment
		cmd.Env = cmd.Environ()
		// Add custom env vars
		for k, v := range env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	// Create pipes for stdin/stdout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Capture stderr
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Start the subprocess
	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	transport := &StdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
		stderr: stderr,
		nextID: 1,
	}

	// Wait for the process in a goroutine to avoid zombie processes
	go func() {
		cmd.Wait()
	}()

	return transport, nil
}

// Send sends a JSON-RPC request and waits for the response
func (t *StdioTransport) Send(request JSONRPCRequest) (*JSONRPCResponse, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, errors.New("transport is closed")
	}

	// Auto-increment request ID if not set
	if request.ID == 0 {
		request.ID = t.nextID
		t.nextID++
	}

	// Set JSON-RPC version if not set
	if request.JSONRPC == "" {
		request.JSONRPC = "2.0"
	}

	// Serialize request
	requestData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Write request to stdin with newline
	requestData = append(requestData, '\n')
	if _, err := t.stdin.Write(requestData); err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Read response from stdout
	responseData, err := t.stdout.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	var response JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Check for JSON-RPC error
	if response.Error != nil {
		return nil, response.Error
	}

	return &response, nil
}

// SendWithContext sends a JSON-RPC request with context support
func (t *StdioTransport) SendWithContext(ctx context.Context, request JSONRPCRequest) (*JSONRPCResponse, error) {
	// Create a channel for the response
	resultChan := make(chan *JSONRPCResponse, 1)
	errChan := make(chan error, 1)

	// Send the request in a goroutine
	go func() {
		resp, err := t.Send(request)
		if err != nil {
			errChan <- err
		} else {
			resultChan <- resp
		}
	}()

	// Wait for response or context cancellation
	select {
	case resp := <-resultChan:
		return resp, nil
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close closes the transport and terminates the subprocess
func (t *StdioTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}

	t.closed = true

	// Close stdin first to signal EOF to the process
	if t.stdin != nil {
		t.stdin.Close()
	}

	// Wait for the process to exit (with timeout)
	done := make(chan error, 1)
	go func() {
		done <- t.cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		// Force kill if it doesn't exit
		if err := t.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
		return errors.New("process killed due to timeout")
	}
}

// IsAlive returns true if the subprocess is still running
func (t *StdioTransport) IsAlive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return false
	}

	// Check if the process is still running
	if t.cmd.Process == nil {
		return false
	}

	// Try to get process state
	if t.cmd.ProcessState != nil && t.cmd.ProcessState.Exited() {
		return false
	}

	return true
}

// Stderr returns the captured stderr output
func (t *StdioTransport) Stderr() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stderr.String()
}