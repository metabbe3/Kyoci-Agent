package selfimprove

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// PRCreator handles automated PR creation
type PRCreator struct {
	repoDir    string
	remote     string
	baseBranch string
}

// NewPRCreator creates a new PRCreator instance
func NewPRCreator(repoDir string) *PRCreator {
	return &PRCreator{
		repoDir:    repoDir,
		remote:     "origin",
		baseBranch: "main",
	}
}

// CreateBranch creates and switches to a new git branch
func (p *PRCreator) CreateBranch(ctx context.Context, name string) error {
	// First, check current branch
	currentBranch, err := p.getCurrentBranch(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// If already on the target branch, just return
	if currentBranch == name {
		return nil
	}

	// Fetch and checkout base branch to ensure we're up to date
	fetchCmd := exec.CommandContext(ctx, "git", "fetch", p.remote, p.baseBranch)
	fetchCmd.Dir = p.repoDir
	if output, err := fetchCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch failed: %w, output: %s", err, string(output))
	}

	checkoutCmd := exec.CommandContext(ctx, "git", "checkout", p.baseBranch)
	checkoutCmd.Dir = p.repoDir
	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("git checkout failed: %w", err)
	}

	// Pull latest changes
	pullCmd := exec.CommandContext(ctx, "git", "pull")
	pullCmd.Dir = p.repoDir
	if _, err := pullCmd.CombinedOutput(); err != nil {
		// Pull might fail if there's nothing to pull, that's okay
		fmt.Printf("Warning: git pull failed: %v\n", err)
	}

	// Create and checkout new branch
	branchCmd := exec.CommandContext(ctx, "git", "checkout", "-b", name)
	branchCmd.Dir = p.repoDir
	
	if output, err := branchCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout -b failed: %w, output: %s", err, string(output))
	}

	return nil
}

// Commit stages and commits the specified files
func (p *PRCreator) Commit(ctx context.Context, files []string, message string) error {
	if len(files) == 0 {
		return nil
	}

	// Add files
	args := append([]string{"add"}, files...)
	gitAddCmd := exec.CommandContext(ctx, "git", args...)
	gitAddCmd.Dir = p.repoDir

	if output, err := gitAddCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %w, output: %s", err, string(output))
	}

	// Commit
	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	commitCmd.Dir = p.repoDir
	
	if output, err := commitCmd.CombinedOutput(); err != nil {
		// Check if nothing to commit
		outputStr := string(output)
		if strings.Contains(outputStr, "nothing to commit") || 
		   strings.Contains(outputStr, "no changes added") {
			return nil
		}
		return fmt.Errorf("git commit failed: %w, output: %s", err, outputStr)
	}
	return nil
}

// Push pushes the branch to remote
func (p *PRCreator) Push(ctx context.Context, branch string) error {
	pushCmd := exec.CommandContext(ctx, "git", "push", "-u", p.remote, branch)
	pushCmd.Dir = p.repoDir
	
	if output, err := pushCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push failed: %w, output: %s", err, string(output))
	}

	return nil
}

// CreatePR creates a pull request using gh CLI
func (p *PRCreator) CreatePR(ctx context.Context, title, body, branch string) (string, error) {
	if !p.IsGhAvailable() {
		return "", fmt.Errorf("gh CLI not available")
	}

	// Build gh pr create command
	args := []string{
		"pr", "create",
		"--title", title,
		"--body", body,
		"--base", p.baseBranch,
		"--head", branch,
	}

	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = p.repoDir

	// Set environment to avoid prompts
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh pr create failed: %w, output: %s", err, string(output))
	}

	// Parse PR URL from output
	outputStr := string(output)
	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "http") {
			return line, nil
		}
	}

	// Try to extract from URL patterns
	if strings.Contains(outputStr, "pull/") {
		// Extract the URL
		parts := strings.Split(outputStr, " ")
		for _, part := range parts {
			if strings.Contains(part, "pull/") {
				return strings.TrimSpace(part), nil
			}
		}
	}

	return "", fmt.Errorf("could not parse PR URL from output: %s", outputStr)
}

// AddLabels adds labels to an existing PR
func (p *PRCreator) AddLabels(prURL string, labels []string) error {
	if !p.IsGhAvailable() {
		return nil // Skip gracefully
	}

	if len(labels) == 0 {
		return nil
	}

	// Extract PR number from URL
	var prNum string
	parts := strings.Split(prURL, "/")
	for i, part := range parts {
		if part == "pull" && i+1 < len(parts) {
			prNum = strings.Split(parts[i+1], "/")[0]
			break
		}
	}

	if prNum == "" {
		return fmt.Errorf("could not extract PR number from URL: %s", prURL)
	}

	// Add labels using gh
	args := append([]string{"pr", "edit", prNum, "--add-label"}, labels...)
	args[len(args)-1] = strings.Join(labels, ",")
	
	cmd := exec.Command("gh", args...)
	cmd.Dir = p.repoDir

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gh pr edit failed: %w, output: %s", err, string(output))
	}

	return nil
}

// IsGhAvailable checks if gh CLI is installed
func (p *PRCreator) IsGhAvailable() bool {
	cmd := exec.Command("gh", "--version")
	return cmd.Run() == nil
}

// getCurrentBranch returns the current git branch
func (p *PRCreator) getCurrentBranch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = p.repoDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}