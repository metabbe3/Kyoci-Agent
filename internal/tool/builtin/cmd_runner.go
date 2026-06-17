package builtin

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// =====================================================================================
// Shared command-runner helpers for tools that wrap external binaries.
//
// Tools like `git`, `grep` (ripgrep), `format` (gofmt/prettier), and `lsp`
// (gopls) all follow the same pattern: look up the binary in $PATH, run it
// with a timeout, and return combined stdout+stderr. These helpers keep that
// pattern in one place so individual tool files stay focused on their domain.
// =====================================================================================

// lookPath returns the absolute path to name if it's resolvable in $PATH, else "".
// Tools use this to detect whether the external binary is available and
// gracefully fall back (e.g. grep tool falls back to Go regex when rg is missing).
func lookPath(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// runCmd executes a command with arguments and a deadline. Returns combined
// stdout+stderr and any execution error. The caller decides how to interpret
// non-zero exit codes — some tools (git diff returning non-zero on dirty tree)
// treat that as success-with-content.
//
// If the binary is not found, returns a clear error rather than spawning a shell.
func runCmd(ctx context.Context, binPath string, args []string, workdir string, timeout time.Duration) (string, error) {
	if binPath == "" {
		return "", fmt.Errorf("binary not found in PATH")
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(rctx, binPath, args...)
	if workdir != "" {
		cmd.Dir = workdir
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// runCmdInShell runs `bash -c <cmd>` (or sh on systems without bash). Used by
// tools that need shell features (pipes, redirection, glob expansion) rather
// than direct exec.
func runCmdInShell(ctx context.Context, cmdText string, workdir string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	shell := "/bin/bash"
	if lookPath("bash") == "" {
		shell = "/bin/sh"
	}

	cmd := exec.CommandContext(rctx, shell, "-c", cmdText)
	if workdir != "" {
		cmd.Dir = workdir
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}
