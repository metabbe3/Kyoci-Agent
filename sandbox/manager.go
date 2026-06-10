package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// SandboxManager manages sandbox execution with multiple backends
type SandboxManager struct {
	docker *DockerSandbox
	host   *HostExecutor
	ssh    *SSHExecutor
	modal  *ModalExecutor
	pool   *ContainerPool
	mode   string // "docker", "host", "auto", "ssh", "modal", "pool"
}

// HostExecutor provides direct host execution (no sandboxing)
type HostExecutor struct {
	workDir string
	timeout time.Duration
}

// NewSandboxManager creates a new sandbox manager
func NewSandboxManager(workDir string, mode string, sshHost string, sshPort int, sshUser, sshKeyPath, sshPassword string, modalToken string) (*SandboxManager, error) {
	// Normalize mode
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "docker" && mode != "host" && mode != "auto" && mode != "ssh" && mode != "modal" && mode != "pool" {
		mode = "auto"
	}

	// Initialize Docker sandbox
	dockerSandbox, err := NewDockerSandbox(workDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize docker sandbox: %w", err)
	}

	// Initialize host executor
	hostExecutor := &HostExecutor{
		workDir: workDir,
		timeout: 30 * time.Second,
	}

	// Initialize SSH executor if SSH configuration is provided
	var sshExecutor *SSHExecutor
	if sshHost != "" && sshUser != "" {
		sshExecutor, err = NewSSHExecutor(sshHost, sshPort, sshUser, sshKeyPath, sshPassword, workDir)
		if err != nil {
			slog.Warn("failed to initialize SSH executor", "error", err)
		}
	}

	// Initialize Modal executor if token is provided
	var modalExecutor *ModalExecutor
	if modalToken != "" {
		modalExecutor = NewModalExecutor(modalToken)
	}

	// Initialize container pool if Docker is available
	var pool *ContainerPool
	if dockerSandbox.IsAvailable() {
		pool, err = NewContainerPool(dockerSandbox, 3, workDir) // Default pool size: 3
		if err != nil {
			slog.Warn("failed to initialize container pool", "error", err)
		}
	}

	return &SandboxManager{
		docker: dockerSandbox,
		host:   hostExecutor,
		ssh:    sshExecutor,
		modal:  modalExecutor,
		pool:   pool,
		mode:   mode,
	}, nil
}

// Exec executes a script with the appropriate executor
func (m *SandboxManager) Exec(ctx context.Context, script string, lang string) (*ExecResult, error) {
	switch m.mode {
	case "docker":
		if !m.docker.IsAvailable() {
			return nil, fmt.Errorf("docker mode requested but docker is not available")
		}
		slog.Debug("executing in Docker", "lang", lang)
		return m.docker.Exec(ctx, script, lang)

	case "host":
		slog.Warn("host execution mode - no resource limits enforced")
		return m.host.Exec(ctx, script, lang)

	case "ssh":
		if m.ssh == nil || !m.ssh.IsAvailable() {
			return nil, fmt.Errorf("ssh mode requested but ssh is not available")
		}
		slog.Debug("executing via SSH", "lang", lang)
		return m.ssh.Exec(ctx, script, lang)

	case "modal":
		if m.modal == nil || !m.modal.IsAvailable() {
			return nil, fmt.Errorf("modal mode requested but modal is not available")
		}
		slog.Debug("executing via Modal", "lang", lang)
		return m.modal.Exec(ctx, script, lang)

	case "pool":
		if m.pool == nil || !m.docker.IsAvailable() {
			return nil, fmt.Errorf("pool mode requested but docker/pool is not available")
		}
		slog.Debug("executing in container pool", "lang", lang)
		// Note: pool execution would need more sophisticated handling
		return m.docker.Exec(ctx, script, lang)

	case "auto":
		// Try backends in order: docker -> ssh -> host
		if m.docker.IsAvailable() {
			slog.Debug("executing in Docker", "lang", lang)
			return m.docker.Exec(ctx, script, lang)
		}
		if m.ssh != nil && m.ssh.IsAvailable() {
			slog.Debug("executing via SSH", "lang", lang)
			return m.ssh.Exec(ctx, script, lang)
		}
		slog.Info("docker and ssh unavailable, falling back to host execution")
		return m.host.Exec(ctx, script, lang)

	default:
		slog.Warn("unknown mode, falling back to host execution", "mode", m.mode)
		return m.host.Exec(ctx, script, lang)
	}
}

// ExecFile executes a file with the appropriate executor
func (m *SandboxManager) ExecFile(ctx context.Context, filePath string, lang string) (*ExecResult, error) {
	switch m.mode {
	case "docker":
		if !m.docker.IsAvailable() {
			return nil, fmt.Errorf("docker mode requested but docker is not available")
		}
		slog.Debug("executing file in Docker", "path", filePath, "lang", lang)
		return m.docker.ExecFile(ctx, filePath, lang)

	case "host":
		slog.Debug("executing file on host", "path", filePath, "lang", lang)
		return m.host.ExecFile(ctx, filePath, lang)

	case "ssh":
		if m.ssh == nil || !m.ssh.IsAvailable() {
			return nil, fmt.Errorf("ssh mode requested but ssh is not available")
		}
		slog.Debug("executing file via SSH", "path", filePath, "lang", lang)
		return m.ssh.ExecFile(ctx, filePath, lang)

	case "modal":
		if m.modal == nil || !m.modal.IsAvailable() {
			return nil, fmt.Errorf("modal mode requested but modal is not available")
		}
		slog.Debug("executing file via Modal", "path", filePath, "lang", lang)
		return m.modal.ExecFile(ctx, filePath, lang)

	case "pool":
		if m.pool == nil || !m.docker.IsAvailable() {
			return nil, fmt.Errorf("pool mode requested but docker/pool is not available")
		}
		slog.Debug("executing file in container pool", "path", filePath, "lang", lang)
		return m.docker.ExecFile(ctx, filePath, lang)

	case "auto":
		// Try backends in order: docker -> ssh -> host
		if m.docker.IsAvailable() {
			slog.Debug("executing file in Docker", "path", filePath, "lang", lang)
			return m.docker.ExecFile(ctx, filePath, lang)
		}
		if m.ssh != nil && m.ssh.IsAvailable() {
			slog.Debug("executing file via SSH", "path", filePath, "lang", lang)
			return m.ssh.ExecFile(ctx, filePath, lang)
		}
		slog.Info("docker and ssh unavailable, falling back to host execution")
		return m.host.ExecFile(ctx, filePath, lang)

	default:
		slog.Warn("unknown mode, falling back to host execution", "mode", m.mode)
		return m.host.ExecFile(ctx, filePath, lang)
	}
}

// IsDockerAvailable returns whether Docker is available
func (m *SandboxManager) IsDockerAvailable() bool {
	return m.docker.IsAvailable()
}

// Mode returns the current execution mode
func (m *SandboxManager) Mode() string {
	return m.mode
}

// IsSSHAvailable returns whether SSH executor is available
func (m *SandboxManager) IsSSHAvailable() bool {
	return m.ssh != nil && m.ssh.IsAvailable()
}

// IsModalAvailable returns whether Modal executor is available
func (m *SandboxManager) IsModalAvailable() bool {
	return m.modal != nil && m.modal.IsAvailable()
}

// IsPoolAvailable returns whether container pool is available
func (m *SandboxManager) IsPoolAvailable() bool {
	return m.pool != nil
}

// GetPoolStats returns container pool statistics
func (m *SandboxManager) GetPoolStats() (PoolStats, error) {
	if m.pool == nil {
		return PoolStats{}, fmt.Errorf("pool not initialized")
	}
	return m.pool.Stats(), nil
}

// Exec executes a script directly on the host
func (h *HostExecutor) Exec(ctx context.Context, script string, lang string) (*ExecResult, error) {
	// Create temp file
	ext := GetFileExtension(lang)
	if ext == "" {
		ext = ".sh"
	}

	tempFile := h.tempFilePath(ext)
	if err := writeFile(tempFile, script); err != nil {
		return nil, fmt.Errorf("failed to write script: %w", err)
	}
	defer osRemove(tempFile)

	return h.ExecFile(ctx, tempFile, lang)
}

// ExecFile executes a file directly on the host
func (h *HostExecutor) ExecFile(ctx context.Context, filePath string, lang string) (*ExecResult, error) {
	startTime := time.Now()

	// Build command
	cmdParts := GetCommandForLang(lang, filePath)
	if len(cmdParts) == 0 {
		// Default: execute as shell script
		cmdParts = []string{"sh", filePath}
	}

	// Create command with context
	cmd := exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)
	cmd.Dir = h.workDir

	// Set timeout if not already set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, h.timeout)
		defer cancel()
	}

	// Execute
	output, err := cmd.CombinedOutput()
	duration := time.Since(startTime)

	result := &ExecResult{
		Stdout:   string(output),
		Stderr:   "",
		Duration: duration,
		TimedOut: ctx.Err() == context.DeadlineExceeded,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}

	return result, nil
}

// tempFilePath generates a temporary file path
func (h *HostExecutor) tempFilePath(ext string) string {
	return h.workDir + "/.sandbox_host_temp_" + fmt.Sprint(time.Now().UnixNano()) + ext
}