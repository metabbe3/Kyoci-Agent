package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DockerSandbox provides sandboxed code execution using Docker containers
type DockerSandbox struct {
	enabled     bool          // auto-detect Docker availability
	image       string        // container image (default: alpine:latest)
	memLimit    string        // "128m"
	cpuLimit    string        // "0.5"
	pidsLimit   int64         // 64
	timeout     time.Duration // 30s
	networkMode string        // "none"
	workDir     string        // mounted volume
}

// ExecResult contains the result of a sandboxed execution
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
	TimedOut bool
}

// NewDockerSandbox creates a new Docker sandbox instance
func NewDockerSandbox(workDir string) (*DockerSandbox, error) {
	s := &DockerSandbox{
		image:       "alpine:latest",
		memLimit:    "128m",
		cpuLimit:    "0.5",
		pidsLimit:   64,
		timeout:     30 * time.Second,
		networkMode: "none",
		workDir:     workDir,
	}

	s.enabled = s.IsAvailable()

	return s, nil
}

// IsAvailable checks if Docker is available on the system
func (s *DockerSandbox) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "info")
	err := cmd.Run()
	return err == nil
}

// Exec executes a script in the Docker sandbox
func (s *DockerSandbox) Exec(ctx context.Context, script string, lang string) (*ExecResult, error) {
	if !s.enabled {
		return nil, fmt.Errorf("docker is not available")
	}

	// Create temp file for the script
	ext := GetFileExtension(lang)
	if ext == "" {
		ext = ".sh"
	}

	tempFile := filepath.Join(s.workDir, ".sandbox_temp_"+fmt.Sprintf("%d", time.Now().UnixNano())+ext)
	if err := s.writeScript(tempFile, script); err != nil {
		return nil, fmt.Errorf("failed to write script: %w", err)
	}
	defer osRemove(tempFile)

	return s.ExecFile(ctx, tempFile, lang)
}

// ExecFile executes a file in the Docker sandbox
func (s *DockerSandbox) ExecFile(ctx context.Context, filePath string, lang string) (*ExecResult, error) {
	if !s.enabled {
		return nil, fmt.Errorf("docker is not available")
	}

	image := GetImageForLang(lang)
	if image == "" {
		image = s.image
	}

	// Pull image if not present
	if err := s.PullImage(image); err != nil {
		return nil, fmt.Errorf("failed to pull image %s: %w", image, err)
	}

	startTime := time.Now()

	// Build docker run command
	cmd := s.buildDockerCommand(image, filePath, lang)

	// Add timeout to context if not already set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
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

// PullImage pulls a Docker image if not already present
func (s *DockerSandbox) PullImage(image string) error {
	// Check if image exists locally
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	if cmd.Run() == nil {
		return nil // Image already exists
	}

	// Pull the image
	pullCtx, pullCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer pullCancel()

	pullCmd := exec.CommandContext(pullCtx, "docker", "pull", image)
	output, err := pullCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %s: %w", image, string(output), err)
	}

	return nil
}

// writeScript writes a script to a file
func (s *DockerSandbox) writeScript(path, content string) error {
	return writeFile(path, content)
}

// buildDockerCommand builds the docker run command for executing a script
func (s *DockerSandbox) buildDockerCommand(image, filePath, lang string) *exec.Cmd {
	cmdArgs := []string{
		"run",
		"--rm",
		"--network=" + s.networkMode,
		"--memory=" + s.memLimit,
		"--cpus=" + s.cpuLimit,
		"--pids-limit=" + fmt.Sprintf("%d", s.pidsLimit),
		"--read-only",
		"--tmpfs", "/tmp:size=10m",
		"--user=nobody",
		"--security-opt=no-new-privileges",
		"-v", s.workDir + ":/workspace:ro",
		"--entrypoint", "/bin/sh",
		image,
		"-c",
		s.buildExecutionCommand(filePath, lang),
	}

	return exec.Command("docker", cmdArgs...)
}

// buildExecutionCommand builds the shell command to execute inside the container
func (s *DockerSandbox) buildExecutionCommand(filePath, lang string) string {
	relPath := filepath.Base(filePath)
	cmdParts := GetCommandForLang(lang, relPath)

	if len(cmdParts) == 0 {
		// Default: execute as shell script
		return fmt.Sprintf("cd /workspace && sh %s", relPath)
	}

	cmdStr := strings.Join(cmdParts, " ")
	return fmt.Sprintf("cd /workspace && %s", cmdStr)
}

// Helper functions for file operations (stdlib only)

func writeFile(path, content string) error {
	// Simple implementation using exec echo
	cmd := exec.Command("sh", "-c", fmt.Sprintf("cat > %s", path))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	if _, err := stdin.Write([]byte(content)); err != nil {
		return err
	}
	stdin.Close()

	return cmd.Wait()
}

func osRemove(path string) error {
	return exec.Command("rm", "-f", path).Run()
}