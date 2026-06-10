package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// CodeExecTool executes code in a sandbox environment
type CodeExecTool struct{}

// NewCodeExecTool creates a new CodeExec tool
func NewCodeExecTool() *CodeExecTool {
	return &CodeExecTool{}
}

func (t *CodeExecTool) Name() string {
	return "code_exec"
}

func (t *CodeExecTool) Description() string {
	return "Execute code in a sandbox environment. Supports Python, JavaScript, and Go. Writes code to a temp file, executes it, captures stdout/stderr, and returns the output."
}

func (t *CodeExecTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"language": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"python", "javascript", "go"},
				"description": "Programming language to execute",
			},
			"code": map[string]interface{}{
				"type":        "string",
				"description": "Code to execute",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "Timeout in seconds (default: 10)",
				"default":     10,
			},
		},
		"required": []string{"language", "code"},
	}
}

func (t *CodeExecTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var params struct {
		Language string `json:"language"`
		Code     string `json:"code"`
		Timeout  int    `json:"timeout"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	// Set default timeout
	if params.Timeout <= 0 {
		params.Timeout = 10
	}

	// Validate language
	if params.Language != "python" && params.Language != "javascript" && params.Language != "go" {
		return "", fmt.Errorf("unsupported language: %s. Must be one of: python, javascript, go", params.Language)
	}

	// Validate code
	if params.Code == "" {
		return "", fmt.Errorf("code is required")
	}

	// Determine file extension and command
	var ext string
	var cmdName string
	var cmdArgs []string

	switch params.Language {
	case "python":
		ext = ".py"
		cmdName = "python3"
		cmdArgs = []string{}
	case "javascript":
		ext = ".js"
		cmdName = "node"
		cmdArgs = []string{}
	case "go":
		ext = ".go"
		cmdName = "go"
		cmdArgs = []string{"run"}
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("code-exec-*%s", ext))
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Write code to temp file
	if _, err := tmpFile.WriteString(params.Code); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to write code to temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}

	// Ensure temp file is cleaned up
	defer os.Remove(tmpPath)

	// Build command arguments
	fullArgs := append(cmdArgs, tmpPath)

	// Create command with context and timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(params.Timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, cmdName, fullArgs...)

	// Capture stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start command: %w", err)
	}

	// Read output
	output, err := io.ReadAll(stdout)
	if err != nil {
		return "", fmt.Errorf("failed to read stdout: %w", err)
	}
	errOutput, err := io.ReadAll(stderr)
	if err != nil {
		return "", fmt.Errorf("failed to read stderr: %w", err)
	}

	// Wait for command to complete
	if err := cmd.Wait(); err != nil {
		// Return both stdout and stderr even on error
		result := string(output)
		if len(errOutput) > 0 {
			if result != "" {
				result += "\n"
			}
			result += string(errOutput)
		}
		return result, fmt.Errorf("execution failed: %w", err)
	}

	// Combine stdout and stderr
	result := string(output)
	if len(errOutput) > 0 {
		if result != "" {
			result += "\n"
		}
		result += string(errOutput)
	}

	return result, nil
}