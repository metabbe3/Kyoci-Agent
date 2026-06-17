package builtin

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/metabbe3/Kyoci-Agent/pkg"
)

// TerminalTool implements the kyoci.Tool interface for shell command execution.
type TerminalTool struct {
	logger *slog.Logger
}

// NewTerminalTool creates a new terminal tool instance.
func NewTerminalTool() *TerminalTool {
	return &TerminalTool{
		logger: slog.Default(),
	}
}

// Name returns the tool name.
func (t *TerminalTool) Name() string {
	return "terminal"
}

// Description returns the tool description.
func (t *TerminalTool) Description() string {
	return "Execute shell commands with timeout support. Runs commands in a safe environment with dangerous command detection."
}

// Parameters returns the tool parameter definition.
func (t *TerminalTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{
			Name:        "command",
			Type:        "string",
			Description: "The shell command to execute",
			Required:    true,
		},
		{
			Name:        "timeout",
			Type:        "integer",
			Description: "Command timeout in seconds (default: 30)",
			Required:    false,
			Default:     30,
		},
		{
			Name:        "workdir",
			Type:        "string",
			Description: "Working directory for command execution (default: current directory)",
			Required:    false,
		},
	}
}

// Execute runs a shell command with timeout and context cancellation.
//
// Parameters:
//   - ctx: Context for cancellation
//   - params: Map containing "command" (required), "timeout" (optional), "workdir" (optional)
//
// Returns:
//   - string: Combined stdout and stderr
//   - error: Error if command fails, times out, or is blocked
func (t *TerminalTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// Extract command
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("command parameter is required and must be a string")
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

	// Extract workdir (optional)
	workdir := ""
	if workdirVal, ok := params["workdir"].(string); ok {
		workdir = workdirVal
	}

	// Check for dangerous commands
	if t.isDangerousCommand(command) {
		t.logger.Warn("dangerous command blocked", "command", command)
		return "", fmt.Errorf("command blocked for safety: potentially dangerous command")
	}

	t.logger.Info("executing terminal command", "command", command, "timeout", timeoutSeconds, "workdir", workdir)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	// Determine which shell to use (cross-platform)
	shell := "/bin/bash"
	if _, err := os.Stat(shell); os.IsNotExist(err) {
		shell = "/bin/sh"
	}
	// Windows fallback
	if runtime.GOOS == "windows" {
		shell = "cmd.exe"
		if p := os.Getenv("SystemRoot"); p != "" {
			ps := p + "\\System32\\WindowsPowerShell\\v1.0\\powershell.exe"
			if _, err := os.Stat(ps); err == nil {
				shell = ps
			}
		}
	}

	// Create command
	cmd := exec.CommandContext(ctx, shell, "-c", command)

	// Set working directory if provided
	if workdir != "" {
		cmd.Dir = workdir
	}

	// Ensure PATH includes common system directories so standard commands
	// (uname, df, system_profiler, etc.) are always findable, even when the
	// Go process inherits a minimal environment (e.g., launched from a daemon
	// or launchd plist).
	path := os.Getenv("PATH")
	for _, dir := range []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin", "/usr/local/bin", "/opt/homebrew/bin"} {
		if !strings.Contains(path, dir) {
			path = path + ":" + dir
		}
	}
	cmd.Env = append(os.Environ(), "PATH="+path)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Check for context timeout
	if ctx.Err() == context.DeadlineExceeded {
		return outputStr, fmt.Errorf("command timed out after %d seconds", timeoutSeconds)
	}

	// "command not found" is NOT a fatal error — return the output so the
	// agent can adapt (try an alternative command, report to user, etc.)
	// Only return a hard error if we got zero output at all.
	if err != nil && strings.Contains(outputStr, "command not found") {
		t.logger.Warn("command not found", "command", command, "output", outputStr)
		return outputStr, nil // return output, no error — let agent handle it
	}

	// Check for execution error (exit code != 0 but output was produced)
	if err != nil {
		t.logger.Warn("command exited non-zero", "command", command, "error", err, "output_len", len(outputStr))
		// Still return the output — the agent can read it to understand the failure
		return outputStr, nil
	}

	t.logger.Info("command executed successfully", "command", command)
	return outputStr, nil
}

// isDangerousCommand checks if a command is potentially dangerous.
func (t *TerminalTool) isDangerousCommand(command string) bool {
	dangerousPatterns := []string{
		"rm -rf /",
		"rm -rf /*",
		":(){ :|:& };:",
		"> /dev/sda",
		"mkfs",
		"dd if=/dev/zero",
		"chmod -R 777 /",
		"chown -R root",
		"curl http|sh",
		"curl https|sh",
		"wget -q|sh",
		"eval $(",
		"exec $(",
	}

	cmdLower := strings.ToLower(command)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(cmdLower, pattern) {
			return true
		}
	}

	return false
}