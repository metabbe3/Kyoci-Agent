package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Process represents a running background process
type Process struct {
	Cmd       *exec.Cmd
	StartedAt time.Time
	Output    strings.Builder
	mu        sync.Mutex
}

// TerminalTool provides shell command execution capabilities
type TerminalTool struct {
	backgroundProcesses sync.Map // map[string]*Process
	sessionCounter     int
	sessionCounterMux  sync.Mutex
}

// NewTerminalTool creates a new TerminalTool instance
func NewTerminalTool() *TerminalTool {
	return &TerminalTool{}
}

// Name returns the tool identifier
func (t *TerminalTool) Name() string {
	return "terminal"
}

// Description explains what the tool does
func (t *TerminalTool) Description() string {
	return "Execute shell commands with timeout support. Run commands in foreground (wait for result) or background (return session ID). List, kill, and read output from background processes."
}

// Parameters returns the JSON Schema for the tool's input
func (t *TerminalTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"run", "list", "kill", "read"},
				"description": "Action to perform: 'run' to execute a command, 'list' to show background processes, 'kill' to terminate a process, 'read' to get process output",
			},
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Shell command to execute (required for 'run' action)",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"default":     30,
				"description": "Timeout in seconds (default: 30, for 'run' action only)",
			},
			"background": map[string]interface{}{
				"type":        "boolean",
				"default":     false,
				"description": "Run command in background and return session ID (for 'run' action only)",
			},
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "Session ID of background process (required for 'kill' and 'read' actions)",
			},
			"workdir": map[string]interface{}{
				"type":        "string",
				"description": "Working directory for command execution (optional)",
			},
		},
		"required": []string{"action"},
	}
}

// Execute runs the tool with given parameters
func (t *TerminalTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var params struct {
		Action    string `json:"action"`
		Command   string `json:"command"`
		Timeout   int    `json:"timeout"`
		Background bool  `json:"background"`
		SessionID string `json:"session_id"`
		Workdir   string `json:"workdir"`
	}

	if err := json.Unmarshal(raw, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	// Set default timeout
	if params.Timeout == 0 {
		params.Timeout = 30
	}

	switch params.Action {
	case "run":
		return t.runCommand(ctx, params.Command, params.Timeout, params.Background, params.Workdir)
	case "list":
		return t.listProcesses()
	case "kill":
		return t.killProcess(params.SessionID)
	case "read":
		return t.readProcessOutput(params.SessionID)
	default:
		return "", fmt.Errorf("invalid action: %s (must be 'run', 'list', 'kill', or 'read')", params.Action)
	}
}

// runCommand executes a shell command
func (t *TerminalTool) runCommand(ctx context.Context, command string, timeout int, background bool, workdir string) (string, error) {
	if command == "" {
		return "", fmt.Errorf("command is required for 'run' action")
	}

	// Create command with shell
	// Use sh on Unix-like systems
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if workdir != "" {
		cmd.Dir = workdir
	}

	if background {
		return t.runInBackground(cmd, command)
	}

	return t.runInForeground(ctx, cmd, command, timeout)
}

// runInBackground starts a command and returns a session ID
func (t *TerminalTool) runInBackground(cmd *exec.Cmd, command string) (string, error) {
	// Create pipes for capturing output
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Generate session ID
	t.sessionCounterMux.Lock()
	t.sessionCounter++
	sessionID := fmt.Sprintf("session-%d-%d", time.Now().Unix(), t.sessionCounter)
	t.sessionCounterMux.Unlock()

	// Create process struct
	process := &Process{
		Cmd:       cmd,
		StartedAt: time.Now(),
	}

	// Start capturing output in a goroutine
	go t.captureOutput(process, stdoutPipe, stderrPipe)

	// Start the command
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start command: %w", err)
	}

	// Wait for command to complete in background
	go func() {
		cmd.Wait()
		t.backgroundProcesses.Delete(sessionID)
	}()

	// Store the process
	t.backgroundProcesses.Store(sessionID, process)

	return fmt.Sprintf("Background process started\nSession ID: %s\nCommand: %s\nStarted at: %s", sessionID, command, process.StartedAt.Format(time.RFC3339)), nil
}

// captureOutput continuously reads from stdout and stderr
func (t *TerminalTool) captureOutput(process *Process, stdout, stderr io.ReadCloser) {
	// Use a scanner to read line by line
	scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))
	for scanner.Scan() {
		process.mu.Lock()
		process.Output.WriteString(scanner.Text())
		process.Output.WriteString("\n")
		process.mu.Unlock()
	}
}

// runInForeground runs a command and waits for completion with timeout
func (t *TerminalTool) runInForeground(ctx context.Context, cmd *exec.Cmd, command string, timeout int) (string, error) {
	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// Run with timeout and capture combined output
	output, err := exec.CommandContext(timeoutCtx, "sh", "-c", command).CombinedOutput()

	if timeoutCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out after %d seconds", timeout)
	}

	if err != nil {
		return fmt.Sprintf("Command failed: %v\nOutput: %s", err, string(output)), nil
	}

	return string(output), nil
}

// listProcesses returns all running background processes
func (t *TerminalTool) listProcesses() (string, error) {
	var processes []string

	t.backgroundProcesses.Range(func(key, value interface{}) bool {
		sessionID := key.(string)
		process := value.(*Process)
		process.mu.Lock()
		outputSize := process.Output.Len()
		process.mu.Unlock()

		status := "running"
		if process.Cmd.ProcessState != nil {
			if process.Cmd.ProcessState.Exited() {
				if process.Cmd.ProcessState.Success() {
					status = "completed"
				} else {
					status = "failed"
				}
			}
		}

		pid := "N/A"
		if process.Cmd.Process != nil {
			pid = fmt.Sprintf("%d", process.Cmd.Process.Pid)
		}

		info := fmt.Sprintf("Session ID: %s\n  PID: %s\n  Status: %s\n  Started: %s\n  Output size: %d bytes\n",
			sessionID, pid, status, process.StartedAt.Format(time.RFC3339), outputSize)
		processes = append(processes, info)
		return true
	})

	if len(processes) == 0 {
		return "No background processes running.", nil
	}

	return fmt.Sprintf("Background processes (%d total):\n\n%s", len(processes), strings.Join(processes, "")), nil
}

// killProcess terminates a background process
func (t *TerminalTool) killProcess(sessionID string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("session_id is required for 'kill' action")
	}

	value, ok := t.backgroundProcesses.Load(sessionID)
	if !ok {
		return "", fmt.Errorf("process not found: %s", sessionID)
	}

	process := value.(*Process)

	if process.Cmd.Process == nil {
		t.backgroundProcesses.Delete(sessionID)
		return fmt.Sprintf("Process %s has no active process to kill", sessionID), nil
	}

	if err := process.Cmd.Process.Kill(); err != nil {
		return "", fmt.Errorf("failed to kill process %s: %w", sessionID, err)
	}

	t.backgroundProcesses.Delete(sessionID)
	return fmt.Sprintf("Process %s killed successfully", sessionID), nil
}

// readProcessOutput returns the captured output of a background process
func (t *TerminalTool) readProcessOutput(sessionID string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("session_id is required for 'read' action")
	}

	value, ok := t.backgroundProcesses.Load(sessionID)
	if !ok {
		return "", fmt.Errorf("process not found: %s", sessionID)
	}

	process := value.(*Process)
	process.mu.Lock()
	defer process.mu.Unlock()

	output := process.Output.String()

	// Get process status
	status := "running"
	if process.Cmd.ProcessState != nil {
		if process.Cmd.ProcessState.Exited() {
			if process.Cmd.ProcessState.Success() {
				status = "completed"
			} else {
				status = "failed"
			}
		}
	}

	return fmt.Sprintf("Session ID: %s\nStatus: %s\nStarted: %s\n\n--- Output ---\n%s",
		sessionID, status, process.StartedAt.Format(time.RFC3339), output), nil
}