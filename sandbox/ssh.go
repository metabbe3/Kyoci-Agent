package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SSHExecutor provides sandboxed code execution via SSH on remote hosts
type SSHExecutor struct {
	host     string
	port     int
	user     string
	keyPath  string
	password string
	timeout  time.Duration
	workDir  string
}

// NewSSHExecutor creates a new SSH executor instance
func NewSSHExecutor(host string, port int, user, keyPath, password string, workDir string) (*SSHExecutor, error) {
	if host == "" {
		return nil, fmt.Errorf("SSH host is required")
	}
	if user == "" {
		return nil, fmt.Errorf("SSH user is required")
	}
	if keyPath == "" && password == "" {
		return nil, fmt.Errorf("SSH requires either keyPath or password")
	}

	return &SSHExecutor{
		host:     host,
		port:     port,
		user:     user,
		keyPath:  keyPath,
		password: password,
		timeout:  30 * time.Second,
		workDir:  workDir,
	}, nil
}

// IsAvailable checks if SSH connection is available
func (s *SSHExecutor) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try SSH connection with timeout
	args := s.buildSSHArgs("exit 0")
	cmd := exec.CommandContext(ctx, "ssh", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	// Check if output is clean (no connection errors)
	if strings.Contains(string(output), "Connection refused") ||
		strings.Contains(string(output), "Could not resolve hostname") ||
		strings.Contains(string(output), "Permission denied") {
		return false
	}

	return true
}

// Exec executes a script on the remote host via SSH
func (s *SSHExecutor) Exec(ctx context.Context, script string, lang string) (*ExecResult, error) {
	// Create temp file for the script locally
	ext := GetFileExtension(lang)
	if ext == "" {
		ext = ".sh"
	}

	tempFile := filepath.Join(s.workDir, ".ssh_temp_"+fmt.Sprintf("%d", time.Now().UnixNano())+ext)
	if err := writeFile(tempFile, script); err != nil {
		return nil, fmt.Errorf("failed to write script: %w", err)
	}
	defer osRemove(tempFile)

	return s.ExecFile(ctx, tempFile, lang)
}

// ExecFile executes a file on the remote host via SSH
func (s *SSHExecutor) ExecFile(ctx context.Context, localPath string, lang string) (*ExecResult, error) {
	startTime := time.Now()

	// Determine remote temp file path
	remoteFileName := filepath.Base(localPath)
	remotePath := "/tmp/" + remoteFileName

	// Copy file to remote host
	if err := s.copyFileToRemote(ctx, localPath, remotePath); err != nil {
		return nil, fmt.Errorf("failed to copy file to remote: %w", err)
	}
	defer s.cleanupRemoteFile(ctx, remotePath)

	// Build execution command
	cmdParts := GetCommandForLang(lang, remotePath)
	if len(cmdParts) == 0 {
		// Default: execute as shell script
		cmdParts = []string{"sh", remotePath}
	}

	remoteCmd := strings.Join(cmdParts, " ")

	// Add timeout to context if not already set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	// Execute on remote host
	args := s.buildSSHArgs(remoteCmd)
	cmd := exec.CommandContext(ctx, "ssh", args...)

	// Capture stdout and stderr separately
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(startTime)

	result := &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
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

// buildSSHArgs builds SSH command arguments
func (s *SSHExecutor) buildSSHArgs(command string) []string {
	args := []string{
		"-o", "ConnectTimeout=5",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
	}

	// Add key-based auth if keyPath is provided
	if s.keyPath != "" {
		args = append(args, "-i", s.keyPath)
	}

	// Add host and port
	host := fmt.Sprintf("%s@%s", s.user, s.host)
	if s.port > 0 && s.port != 22 {
		host = fmt.Sprintf("-p %d %s@%s", s.port, s.user, s.host)
		args = append([]string{"-p", fmt.Sprintf("%d", s.port)}, args...)
	}

	args = append(args, host, command)

	// For password auth, we need to use sshpass
	if s.password != "" {
		return []string{"sshpass", "-p", s.password, "ssh", "-o", "StrictHostKeyChecking=no", host, command}
	}

	return args
}

// copyFileToRemote copies a file to the remote host via SCP
func (s *SSHExecutor) copyFileToRemote(ctx context.Context, localPath, remotePath string) error {
	// For password auth, use sshpass
	var cmd *exec.Cmd

	if s.password != "" {
		host := fmt.Sprintf("%s@%s:%s", s.user, s.host, remotePath)
		cmd = exec.CommandContext(ctx, "sshpass", "-p", s.password, "scp", localPath, host)
	} else {
		args := []string{}
		if s.port > 0 && s.port != 22 {
			args = append(args, "-P", fmt.Sprintf("%d", s.port))
		}
		if s.keyPath != "" {
			args = append(args, "-i", s.keyPath)
		}
		args = append(args,
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			localPath,
			fmt.Sprintf("%s@%s:%s", s.user, s.host, remotePath),
		)
		cmd = exec.CommandContext(ctx, "scp", args...)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("scp failed: %s: %w", string(output), err)
	}

	return nil
}

// cleanupRemoteFile removes a remote file
func (s *SSHExecutor) cleanupRemoteFile(ctx context.Context, remotePath string) {
	args := s.buildSSHArgs(fmt.Sprintf("rm -f %s", remotePath))
	cmd := exec.CommandContext(ctx, "ssh", args...)
	_ = cmd.Run() // Ignore errors in cleanup
}

// checkDependency checks if a command is available
func (s *SSHExecutor) checkDependency(cmdName string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "which", cmdName)
	err := cmd.Run()
	return err == nil
}

// HasSSHpass checks if sshpass is available for password auth
func (s *SSHExecutor) HasSSHpass() bool {
	return s.checkDependency("sshpass")
}

// GetWorkDir returns the working directory
func (s *SSHExecutor) GetWorkDir() string {
	return s.workDir
}