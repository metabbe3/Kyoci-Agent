package builtin

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/taskctx"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// procEntry represents a background process entry.
type procEntry struct {
	PID          int
	Command      string
	StartedAt    time.Time
	Cmd          *exec.Cmd
	OutputBuffer *bytes.Buffer
}

// Package-level process store with synchronization.
var procStore = struct {
	sync.RWMutex
	procs map[int]*procEntry
}{
	procs: make(map[int]*procEntry),
}

// ProcessTool implements the kyoci.Tool interface for managing background processes.
type ProcessTool struct {
	logger *slog.Logger
}

// NewProcessTool creates a new process tool instance.
func NewProcessTool() *ProcessTool {
	return &ProcessTool{
		logger: slog.Default(),
	}
}

// Name returns the tool name.
func (p *ProcessTool) Name() string {
	return "process"
}

// Description returns the tool description.
func (p *ProcessTool) Description() string {
	return "Manage background processes: start, list, kill. Run long tasks without blocking."
}

// Parameters returns the tool parameter definition.
func (p *ProcessTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{
			Name:        "action",
			Type:        "string",
			Description: "Action to perform: start, list, kill, output",
			Required:    true,
			EnumValues:  []string{"start", "list", "kill", "output"},
		},
		{
			Name:        "command",
			Type:        "string",
			Description: "The shell command to run in background (required for start action)",
			Required:    false,
		},
		{
			Name:        "pid",
			Type:        "string",
			Description: "Process ID to target (required for kill and output actions)",
			Required:    false,
		},
	}
}

// Execute performs process operations.
//
// Parameters:
//   - ctx: Context for cancellation
//   - params: Map containing "action", "command", and optionally "pid"
//
// Returns:
//   - string: Result of the operation
//   - error: Error if operation fails
func (p *ProcessTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// Extract action
	action, ok := params["action"].(string)
	if !ok || action == "" {
		return "", fmt.Errorf("action parameter is required and must be a string")
	}

	p.logger.Info("executing process action", "action", action)

	// Execute based on action type
	switch action {
	case "start":
		return p.startProcess(ctx, params)
	case "list":
		return p.listProcesses()
	case "kill":
		return p.killProcess(params)
	case "output":
		return p.getProcessOutput(params)
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

// startProcess starts a command in the background.
func (p *ProcessTool) startProcess(ctx context.Context, params map[string]interface{}) (string, error) {
	// Extract command
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("command parameter is required for start action")
	}

	p.logger.Info("starting background process", "command", command)

	// Determine the shell command based on OS
	shell, args := p.getShellCommand(command)

	// Create command
	cmd := exec.Command(shell, args...)
	// Default to the per-task workspace so a background server runs where the
	// agent's files live (tasks/<id>/deliverable/). Mirrors the terminal tool.
	if ws := taskctx.WorkspaceFromCtx(ctx); ws != "" {
		cmd.Dir = ws
	}

	// Create output buffer
	var outputBuffer bytes.Buffer

	// Redirect stdout and stderr to the buffer
	cmd.Stdout = &outputBuffer
	cmd.Stderr = &outputBuffer

	// Start the command
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start process: %w", err)
	}

	pid := cmd.Process.Pid
	startedAt := time.Now()

	p.logger.Info("process started", "pid", pid, "command", command)

	// Store process entry
	procStore.Lock()
	procStore.procs[pid] = &procEntry{
		PID:          pid,
		Command:      command,
		StartedAt:    startedAt,
		Cmd:          cmd,
		OutputBuffer: &outputBuffer,
	}
	procStore.Unlock()

	// Start a goroutine to wait for the process and clean up
	go func() {
		err := cmd.Wait()
		p.logger.Info("process exited", "pid", pid, "error", err)

		// Keep the entry for a while after exit to allow reading output
		time.Sleep(30 * time.Second)

		procStore.Lock()
		delete(procStore.procs, pid)
		procStore.Unlock()
	}()

	return fmt.Sprintf("Background process started successfully. PID: %d", pid), nil
}

// getShellCommand returns the shell and arguments to run a command.
func (p *ProcessTool) getShellCommand(command string) (string, []string) {
	// Use cmd on Windows, sh on Unix systems
	if strings.Contains(strings.ToLower(command), "windows") {
		return "cmd", []string{"/c", command}
	}
	return "sh", []string{"-c", command}
}

// listProcesses lists all running background processes.
func (p *ProcessTool) listProcesses() (string, error) {
	p.logger.Info("listing background processes")

	procStore.RLock()
	defer procStore.RUnlock()

	if len(procStore.procs) == 0 {
		return "No background processes running.", nil
	}

	// Format result
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Found %d background process(es):\n", len(procStore.procs)))

	for pid, entry := range procStore.procs {
		duration := time.Since(entry.StartedAt).Round(time.Second)
		result.WriteString(fmt.Sprintf("  PID: %d\n", pid))
		result.WriteString(fmt.Sprintf("    Command: %s\n", entry.Command))
		result.WriteString(fmt.Sprintf("    Started: %s ago\n", duration))
		result.WriteString(fmt.Sprintf("    Status: running\n"))
	}

	return result.String(), nil
}

// killProcess kills a background process by PID.
func (p *ProcessTool) killProcess(params map[string]interface{}) (string, error) {
	// Extract pid
	pidStr, ok := params["pid"].(string)
	if !ok || pidStr == "" {
		return "", fmt.Errorf("pid parameter is required for kill action")
	}

	// Parse PID
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return "", fmt.Errorf("invalid pid: must be an integer")
	}

	p.logger.Info("killing process", "pid", pid)

	// Find and remove the process entry
	procStore.Lock()
	entry, exists := procStore.procs[pid]
	if !exists {
		procStore.Unlock()
		return "", fmt.Errorf("process not found: PID %d", pid)
	}
	delete(procStore.procs, pid)
	procStore.Unlock()

	// Send SIGTERM to the process
	if entry.Cmd != nil && entry.Cmd.Process != nil {
		if err := entry.Cmd.Process.Signal(syscall.SIGTERM); err != nil {
			return "", fmt.Errorf("failed to send SIGTERM to process %d: %w", pid, err)
		}
		p.logger.Info("SIGTERM sent to process", "pid", pid)
		return fmt.Sprintf("Process %d terminated successfully", pid), nil
	}

	return fmt.Sprintf("Process %d entry removed", pid), nil
}

// getProcessOutput retrieves the output of a background process.
func (p *ProcessTool) getProcessOutput(params map[string]interface{}) (string, error) {
	// Extract pid
	pidStr, ok := params["pid"].(string)
	if !ok || pidStr == "" {
		return "", fmt.Errorf("pid parameter is required for output action")
	}

	// Parse PID
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return "", fmt.Errorf("invalid pid: must be an integer")
	}

	p.logger.Info("getting process output", "pid", pid)

	// Find the process entry
	procStore.RLock()
	entry, exists := procStore.procs[pid]
	procStore.RUnlock()

	if !exists {
		return "", fmt.Errorf("process not found: PID %d", pid)
	}

	// Get the output from the buffer
	output := entry.OutputBuffer.String()

	if output == "" {
		return fmt.Sprintf("No output available for process %d", pid), nil
	}

	// Format result
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Output for process %d:\n", pid))
	result.WriteString(strings.Repeat("-", 40))
	result.WriteString("\n")
	result.WriteString(output)
	result.WriteString(strings.Repeat("-", 40))
	result.WriteString("\n")

	return result.String(), nil
}
