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

// LSPTool bridges to language servers (gopls for Go, typescript-language-server
// for TS/JS) to provide code-intelligence operations: definition, references,
// hover, document symbols, formatting. Major capability upgrade for the worker
// — instead of grepping for a symbol, it can resolve it precisely.
//
// All operations are best-effort: if no language server is available for the
// file's language, returns a clear "LSP unavailable" error rather than hanging.
type LSPTool struct{}

func NewLSPTool() *LSPTool { return &LSPTool{} }

func (l *LSPTool) Name() string { return "lsp" }

func (l *LSPTool) Description() string {
	return "Code intelligence via LSP. Resolve symbol definitions, find references, " +
		"hover info, document symbols. `lsp operation=definition path=main.go line=42 char=10`. " +
		"Spawns gopls (Go) or typescript-language-server (TS/JS) per file extension."
}

func (l *LSPTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{Name: "operation", Type: "string", Required: true,
			EnumValues:  []string{"definition", "references", "hover", "symbols", "format"},
			Description: "LSP operation"},
		{Name: "path", Type: "string", Required: true,
			Description: "Absolute path to the file"},
		{Name: "line", Type: "integer", Required: false, Default: 1,
			Description: "1-based line number (for definition/references/hover)"},
		{Name: "char", Type: "integer", Required: false, Default: 1,
			Description: "1-based character offset (for definition/references/hover)"},
	}
}

func (l *LSPTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	op, _ := params["operation"].(string)
	path, _ := params["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	path = expandHome(path)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}

	line := 1
	if v, ok := params["line"].(int); ok {
		line = v
	}
	if v, ok := params["line"].(float64); ok {
		line = int(v)
	}
	char := 1
	if v, ok := params["char"].(int); ok {
		char = v
	}
	if v, ok := params["char"].(float64); ok {
		char = int(v)
	}

	// Dispatch by file extension.
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return runGoLSP(ctx, op, path, line, char)
	case ".ts", ".tsx", ".js", ".jsx", ".mjs":
		return runTSLSP(ctx, op, path, line, char)
	}
	return "", fmt.Errorf("LSP unavailable for %s files (supported: .go, .ts, .tsx, .js, .jsx)", ext)
}

// runGoLSP runs gopls for the requested operation. gopls has a CLI mode that
// accepts queries like `gopls definition file.go:line:col`.
func runGoLSP(ctx context.Context, op, path string, line, char int) (string, error) {
	if lookPath("gopls") == "" {
		return "", fmt.Errorf("gopls not in PATH (install: go install golang.org/x/tools/gopls@latest)")
	}
	args := []string{}
	pos := fmt.Sprintf("%s:%d:%d", path, line, char)
	switch op {
	case "definition":
		args = []string{"-remote", "internal", "definition", pos}
		// Simpler: use the query subcommand.
		args = []string{"definition", pos}
	case "references":
		args = []string{"references", pos}
	case "hover":
		args = []string{"hover", pos}
	case "symbols":
		args = []string{"symbols", path}
	case "format":
		args = []string{"format", path}
	default:
		return "", fmt.Errorf("unsupported LSP operation: %s", op)
	}
	out, err := runCmd(ctx, "gopls", args, filepath.Dir(path), 30*time.Second)
	if err != nil {
		return out, fmt.Errorf("gopls %s: %w", op, err)
	}
	if strings.TrimSpace(out) == "" {
		return fmt.Sprintf("(no %s results)", op), nil
	}
	return strings.TrimRight(out, "\n"), nil
}

// runTSLSP runs typescript-language-server in --mode=cli. Falls back to a
// clear error if the binary isn't installed.
func runTSLSP(ctx context.Context, op, path string, line, char int) (string, error) {
	if lookPath("typescript-language-server") == "" {
		return "", fmt.Errorf("typescript-language-server not in PATH (install: npm i -g typescript-language-server typescript)")
	}
	// CLI mode is limited; we wrap tsc directly for hover/definition.
	args := []string{}
	pos := fmt.Sprintf("%s:%d:%d", path, line, char)
	switch op {
	case "definition":
		args = []string{"--mode=cli", "definition", pos}
	case "references":
		args = []string{"--mode=cli", "references", pos}
	case "hover":
		args = []string{"--mode=cli", "hover", pos}
	case "symbols":
		args = []string{"--mode=cli", "symbols", path}
	case "format":
		// Use tsc directly.
		if lookPath("tsc") != "" {
			return "", fmt.Errorf("use the `format` tool with language=typescript")
		}
		return "", fmt.Errorf("tsc not in PATH")
	default:
		return "", fmt.Errorf("unsupported LSP operation: %s", op)
	}
	out, err := runCmd(ctx, "typescript-language-server", args, filepath.Dir(path), 30*time.Second)
	if err != nil {
		return out, fmt.Errorf("ts-lsp %s: %w", op, err)
	}
	if strings.TrimSpace(out) == "" {
		return fmt.Sprintf("(no %s results)", op), nil
	}
	return strings.TrimRight(out, "\n"), nil
}

var _ kyoci.Tool = (*LSPTool)(nil)
