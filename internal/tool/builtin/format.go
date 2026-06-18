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

// FormatTool runs language-appropriate formatters on a file or string. Wraps
// gofmt (Go), prettier (JS/TS/CSS/MD), and rustfmt (Rust) — saves the worker
// from constructing shell commands for routine formatting.
type FormatTool struct{}

func NewFormatTool() *FormatTool { return &FormatTool{} }

func (f *FormatTool) Name() string { return "format" }

func (f *FormatTool) Description() string {
	return "Format code in-place. `format language=go path=main.go` runs gofmt -w. " +
		"Supports go, js/ts/md/css via prettier, rust via rustfmt. Returns diff summary."
}

func (f *FormatTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{Name: "language", Type: "string", Required: true,
			EnumValues:  []string{"go", "js", "ts", "tsx", "jsx", "css", "markdown", "rust", "python"},
			Description: "Language (selects the formatter)"},
		{Name: "path", Type: "string", Required: true,
			Description: "File to format in-place"},
	}
}

func (f *FormatTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	lang, _ := params["language"].(string)
	path, _ := params["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	path = expandHome(path)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}

	// Snapshot before for diff.
	before, _ := os.ReadFile(path)
	beforeLines := strings.Count(string(before), "\n") + 1

	var (
		cmd     string
		args    []string
		workDir string
	)
	switch lang {
	case "go":
		if lookPath("gofmt") == "" {
			return "", fmt.Errorf("gofmt not in PATH (it ships with Go)")
		}
		cmd = "gofmt"
		args = []string{"-w", path}
		workDir = filepath.Dir(path)
	case "js", "ts", "tsx", "jsx", "css", "markdown":
		if lookPath("prettier") == "" {
			return "", fmt.Errorf("prettier not in PATH (install: npm i -g prettier)")
		}
		cmd = "prettier"
		args = []string{"--write", path}
		workDir = filepath.Dir(path)
	case "rust":
		if lookPath("rustfmt") == "" {
			return "", fmt.Errorf("rustfmt not in PATH (install: rustup component add rustfmt)")
		}
		cmd = "rustfmt"
		args = []string{"--edition", "2021", path}
		workDir = filepath.Dir(path)
	case "python":
		if lookPath("black") == "" && lookPath("autopep8") == "" {
			return "", fmt.Errorf("neither black nor autopep8 in PATH")
		}
		if lookPath("black") != "" {
			cmd = "black"
			args = []string{path}
		} else {
			cmd = "autopep8"
			args = []string{"--in-place", path}
		}
		workDir = filepath.Dir(path)
	default:
		return "", fmt.Errorf("unsupported language: %s", lang)
	}

	out, err := runCmd(ctx, cmd, args, workDir, 30*time.Second)
	if err != nil {
		return out, fmt.Errorf("formatter failed: %w", err)
	}

	after, _ := os.ReadFile(path)
	afterLines := strings.Count(string(after), "\n") + 1

	if string(before) == string(after) {
		return fmt.Sprintf("already formatted (%d line(s), no change)", beforeLines), nil
	}
	return fmt.Sprintf("formatted %s: %d → %d line(s)", filepath.Base(path), beforeLines, afterLines), nil
}

var _ kyoci.Tool = (*FormatTool)(nil)
