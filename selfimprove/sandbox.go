package selfimprove

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Sandbox provides a safe isolated environment for code modification
type Sandbox struct {
	rootDir    string
	sandboxDir string
	gitEnabled bool
}

// NewSandbox creates a new sandbox with a copy of relevant Go files
func NewSandbox(rootDir string) (*Sandbox, error) {
	// Create temp directory for sandbox
	tempDir := os.TempDir()
	sandboxName := fmt.Sprintf("ai-agent-sandbox-%d", os.Getpid())
	sandboxDir := filepath.Join(tempDir, sandboxName)

	if err := os.MkdirAll(sandboxDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sandbox directory: %w", err)
	}

	s := &Sandbox{
		rootDir:    rootDir,
		sandboxDir: sandboxDir,
		gitEnabled: true,
	}

	// Copy relevant files to sandbox
	if err := s.copyProjectFiles(); err != nil {
		s.Cleanup()
		return nil, fmt.Errorf("failed to copy project files: %w", err)
	}

	// Initialize git in sandbox if git is available
	if s.isGitAvailable() {
		s.initGit()
	} else {
		s.gitEnabled = false
	}

	return s, nil
}

// ApplyChange writes file content to a file in the sandbox
func (s *Sandbox) ApplyChange(file, content string) error {
	// Ensure the file path is relative to project root
	if filepath.IsAbs(file) {
		return fmt.Errorf("file path must be relative: %s", file)
	}

	sandboxPath := filepath.Join(s.sandboxDir, file)

	// Create directory if needed
	dir := filepath.Dir(sandboxPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(sandboxPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// ApplyPatch applies a patch to a file in the sandbox
func (s *Sandbox) ApplyPatch(file, oldStr, newStr string) error {
	if filepath.IsAbs(file) {
		return fmt.Errorf("file path must be relative: %s", file)
	}

	sandboxPath := filepath.Join(s.sandboxDir, file)

	// Read existing content
	content, err := os.ReadFile(sandboxPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	contentStr := string(content)

	// Check if old string exists
	if !strings.Contains(contentStr, oldStr) {
		return fmt.Errorf("old string not found in file")
	}

	// Replace
	newContent := strings.Replace(contentStr, oldStr, newStr, 1)

	// Write back
	if err := os.WriteFile(sandboxPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write patched file: %w", err)
	}

	return nil
}

// Validate runs validation in the sandbox
func (s *Sandbox) Validate(ctx context.Context) (*ValidationResult, error) {
	validator := NewValidator(s.sandboxDir)
	return validator.Validate(ctx, nil)
}

// Diff returns git diff of changes in sandbox
func (s *Sandbox) Diff() (string, error) {
	if !s.gitEnabled {
		return "", fmt.Errorf("git not available in sandbox")
	}

	// Get changed files
	changed, err := s.GetChangedFiles()
	if err != nil {
		return "", err
	}

	if len(changed) == 0 {
		return "No changes detected", nil
	}

	// Run git diff for each file
	var diffs []string
	for _, file := range changed {
		sandboxPath := filepath.Join(s.sandboxDir, file)

		gitDiffCmd := exec.Command("git", "diff", "--no-color", sandboxPath)
		gitDiffCmd.Dir = s.sandboxDir

		output, err := gitDiffCmd.CombinedOutput()
		if err != nil {
			continue
		}

		if len(output) > 0 {
			diffs = append(diffs, string(output))
		}
	}

	if len(diffs) == 0 {
		return "No changes detected", nil
	}

	return strings.Join(diffs, "\n\n"), nil
}

// Commit creates a git commit in the sandbox
func (s *Sandbox) Commit(message string) error {
	if !s.gitEnabled {
		return fmt.Errorf("git not available in sandbox")
	}

	// Add all changes
	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = s.sandboxDir
	
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %w, output: %s", err, string(output))
	}

	// Commit
	commitCmd := exec.Command("git", "commit", "-m", message)
	commitCmd.Dir = s.sandboxDir
	
	if output, err := commitCmd.CombinedOutput(); err != nil {
		// Check if it's just "nothing to commit"
		if strings.Contains(string(output), "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit failed: %w, output: %s", err, string(output))
	}

	return nil
}

// Cleanup removes the sandbox directory
func (s *Sandbox) Cleanup() error {
	if s.sandboxDir == "" {
		return nil
	}

	return os.RemoveAll(s.sandboxDir)
}

// GetChangedFiles lists modified files in the sandbox
func (s *Sandbox) GetChangedFiles() ([]string, error) {
	if !s.gitEnabled {
		return nil, fmt.Errorf("git not available in sandbox")
	}

	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = s.sandboxDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}

	var changed []string
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse git status output: " M path" or "M  path"
		if len(line) >= 3 {
			file := strings.TrimSpace(line[2:])
			if file != "" {
				changed = append(changed, file)
			}
		}
	}

	return changed, nil
}

// copyProjectFiles copies relevant Go files to sandbox
func (s *Sandbox) copyProjectFiles() error {
	// Copy go.mod and go.sum
	if err := s.copyFileIfExists("go.mod"); err != nil {
		return err
	}
	if err := s.copyFileIfExists("go.sum"); err != nil {
		return err
	}

	// Copy all Go packages recursively
	return filepath.Walk(s.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(s.rootDir, path)
		if err != nil {
			return err
		}

		// Skip hidden files and directories
		if strings.HasPrefix(relPath, ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip vendor and node_modules
		if info.IsDir() && (info.Name() == "vendor" || info.Name() == "node_modules") {
			return filepath.SkipDir
		}

		// Copy Go source files
		if !info.IsDir() && (strings.HasSuffix(info.Name(), ".go") ||
			info.Name() == "go.mod" ||
			info.Name() == "go.sum") {
			return s.copyFile(path, relPath)
		}

		return nil
	})
}

// copyFile copies a single file to the sandbox
func (s *Sandbox) copyFile(src, relDest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destPath := filepath.Join(s.sandboxDir, relDest)
	destDir := filepath.Dir(destPath)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	destFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	return err
}

// copyFileIfExists copies a file if it exists in root
func (s *Sandbox) copyFileIfExists(filename string) error {
	srcPath := filepath.Join(s.rootDir, filename)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return nil
	}
	return s.copyFile(srcPath, filename)
}

// isGitAvailable checks if git is installed
func (s *Sandbox) isGitAvailable() bool {
	cmd := exec.Command("git", "--version")
	return cmd.Run() == nil
}

// initGit initializes git in the sandbox
func (s *Sandbox) initGit() {
	cmd := exec.Command("git", "init")
	cmd.Dir = s.sandboxDir
	cmd.Run()

	// Configure git to avoid any prompts
	configCmd := exec.Command("git", "config", "user.email", "kyoci-agent@localhost")
	configCmd.Dir = s.sandboxDir
	configCmd.Run()

	nameCmd := exec.Command("git", "config", "user.name", "Kyoci Agent")
	nameCmd.Dir = s.sandboxDir
	nameCmd.Run()

	// Create initial commit
	s.Commit("Initial sandbox state")
}