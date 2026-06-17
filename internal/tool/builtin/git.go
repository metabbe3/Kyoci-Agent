package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// GitTool wraps common read-only git operations so the agent doesn't have to
// construct shell commands for every status / log / diff lookup.
//
// Read-only by design: write operations (commit, push, merge) still require
// the `terminal` tool, which keeps a dangerous-command blocklist.
type GitTool struct{}

func NewGitTool() *GitTool { return &GitTool{} }

func (g *GitTool) Name() string { return "git" }

func (g *GitTool) Description() string {
	return "Read-only git operations: status, diff, log, branch, show, blame. " +
		`git operation=status path="."; git operation=log limit=10. ` +
		"Wraps the git CLI; returns combined stdout+stderr."
}

func (g *GitTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{Name: "operation", Type: "string", Required: true,
			EnumValues:  []string{"status", "diff", "log", "branch", "show", "blame", "remote"},
			Description: "Git operation to perform (all read-only)"},
		{Name: "path", Type: "string", Required: false, Default: ".",
			Description: "Path to the git repo (default: current directory)"},
		{Name: "ref", Type: "string", Required: false,
			Description: "Ref (branch, tag, commit hash) — used by diff/show/blame"},
		{Name: "file", Type: "string", Required: false,
			Description: "File path (relative to repo root) — used by diff/blame"},
		{Name: "limit", Type: "integer", Required: false, Default: 20,
			Description: "Max log entries to return (log operation)"},
	}
}

func (g *GitTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	op, _ := params["operation"].(string)
	path, _ := params["path"].(string)
	if path == "" {
		path = "."
	}
	path = expandHome(path)
	// Find the repo root.
	repoRoot := findRepoRoot(path)
	if repoRoot == "" {
		return "", fmt.Errorf("not inside a git repository (looked upward from %s)", path)
	}

	ref, _ := params["ref"].(string)
	file, _ := params["file"].(string)
	limit := 20
	if v, ok := params["limit"].(int); ok && v > 0 {
		limit = v
	}
	if v, ok := params["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	var args []string
	switch op {
	case "status":
		args = []string{"status", "--short", "--branch"}
	case "diff":
		args = []string{"diff", "--stat"}
		if ref != "" {
			args = append(args, ref)
		}
		if file != "" {
			args = append(args, "--", file)
		}
	case "log":
		args = []string{"log", "--oneline", fmt.Sprintf("-%d", limit)}
		if ref != "" {
			args = append(args, ref)
		}
	case "branch":
		args = []string{"branch", "-a", "--list"}
	case "show":
		if ref == "" {
			return "", fmt.Errorf("show requires a 'ref' (commit hash, branch, tag)")
		}
		args = []string{"show", "--stat", ref}
	case "blame":
		if file == "" {
			return "", fmt.Errorf("blame requires a 'file'")
		}
		args = []string{"blame", "-w", file}
	case "remote":
		args = []string{"remote", "-v"}
	default:
		return "", fmt.Errorf("unsupported operation: %s", op)
	}

	if lookPath("git") == "" {
		return "", fmt.Errorf("git binary not in PATH")
	}
	out, err := runCmd(ctx, "git", args, repoRoot, 30*time.Second)
	if err != nil {
		return out, fmt.Errorf("git %s: %w", op, err)
	}
	if strings.TrimSpace(out) == "" {
		return fmt.Sprintf("(no output from git %s)", op), nil
	}
	return strings.TrimRight(out, "\n"), nil
}

// findRepoRoot walks upward from start looking for a .git entry. Returns ""
// if no repo is found.
func findRepoRoot(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	dir := abs
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

var _ kyoci.Tool = (*GitTool)(nil)
