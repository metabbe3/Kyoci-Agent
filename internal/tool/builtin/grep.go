package builtin

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// GrepTool searches file contents using ripgrep when available, with a
// pure-Go regex walker as the fallback. Unlike the `file` tool's built-in
// `search` operation (which walks the tree and loads every file into memory),
// grep streams results, respects .gitignore, and returns ranked output.
type GrepTool struct{}

func NewGrepTool() *GrepTool { return &GrepTool{} }

func (g *GrepTool) Name() string { return "grep" }

func (g *GrepTool) Description() string {
	return "Search file contents by regex. Returns file:line:match triples, up to max_results. " +
		`grep pattern="TODO|FIXME" path="./src" glob="*.go" max_results=50. ` +
		"Uses ripgrep if installed; falls back to Go regex."
}

func (g *GrepTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{Name: "pattern", Type: "string", Required: true,
			Description: "Regular expression to search for"},
		{Name: "path", Type: "string", Required: false, Default: ".",
			Description: "Directory or file to search (default: current directory)"},
		{Name: "glob", Type: "string", Required: false,
			Description: "Optional file glob filter (e.g. '*.go', '*.ts')"},
		{Name: "ignore_case", Type: "boolean", Required: false, Default: false,
			Description: "Case-insensitive match"},
		{Name: "max_results", Type: "integer", Required: false, Default: 100,
			Description: "Maximum number of matches to return"},
	}
}

func (g *GrepTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	pattern, _ := params["pattern"].(string)
	if pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	path, _ := params["path"].(string)
	if path == "" {
		path = "."
	}
	path = expandHome(path)
	glob, _ := params["glob"].(string)
	ignoreCase := false
	if v, ok := params["ignore_case"].(bool); ok {
		ignoreCase = v
	}
	maxResults := 100
	if v, ok := params["max_results"].(int); ok && v > 0 {
		maxResults = v
	}
	if v, ok := params["max_results"].(float64); ok && v > 0 {
		maxResults = int(v)
	}

	// Prefer ripgrep when available — it's 10-100× faster and respects .gitignore.
	if rgPath := lookPath("rg"); rgPath != "" {
		out, err := runRipgrep(ctx, rgPath, pattern, path, glob, ignoreCase, maxResults)
		if err == nil {
			return out, nil
		}
		// Fall through to Go regex on ripgrep failure.
	}

	// Fallback: pure-Go regex walker.
	return grepGoFallback(ctx, pattern, path, glob, ignoreCase, maxResults)
}

// runRipgrep shells out to rg with sensible defaults: --line-number,
// --no-heading, --color=never, and -i if requested.
func runRipgrep(ctx context.Context, rgPath, pattern, path, glob string, ignoreCase bool, max int) (string, error) {
	args := []string{"--line-number", "--no-heading", "--color=never", "--max-count", fmt.Sprintf("%d", max)}
	if ignoreCase {
		args = append(args, "-i")
	}
	if glob != "" {
		args = append(args, "-g", glob)
	}
	args = append(args, pattern, path)
	out, err := runCmd(ctx, rgPath, args, "", 30*time.Second)
	if err != nil {
		// rg returns exit 1 when no matches — that's not an error for us.
		if strings.Contains(out, "") && strings.TrimSpace(out) == "" {
			return "(no matches)", nil
		}
		return "", err
	}
	if strings.TrimSpace(out) == "" {
		return "(no matches)", nil
	}
	return strings.TrimRight(out, "\n"), nil
}

// grepGoFallback walks the tree and matches the regex line-by-line.
func grepGoFallback(_ context.Context, pattern, root, glob string, ignoreCase bool, max int) (string, error) {
	prefix := ""
	if ignoreCase {
		prefix = "(?i)"
	}
	re, err := regexp.Compile(prefix + pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex: %w", err)
	}
	globRe := compileGlob(glob)

	type hit struct {
		file string
		line int
		text string
	}
	var hits []hit
	visited := 0
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == "vendor" || base == "dist" || base == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		if globRe != nil && !globRe.MatchString(d.Name()) {
			return nil
		}
		// Skip binary-ish files.
		if isLikelyBinary(p) {
			return nil
		}
		visited++
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if re.MatchString(line) {
				hits = append(hits, hit{p, i + 1, strings.TrimSpace(line)})
				if len(hits) >= max {
					return filepath.SkipDir // best-effort stop
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk: %w", err)
	}

	if len(hits) == 0 {
		return fmt.Sprintf("(no matches in %d files)", visited), nil
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].file != hits[j].file {
			return hits[i].file < hits[j].file
		}
		return hits[i].line < hits[j].line
	})
	var b strings.Builder
	fmt.Fprintf(&b, "%d match(es) in %d file(s) walked:\n", len(hits), visited)
	for _, h := range hits {
		fmt.Fprintf(&b, "%s:%d: %s\n", h.file, h.line, h.text)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// compileGlob converts a shell glob (e.g. "*.go") into a Go regexp.
// Returns nil if glob is empty.
func compileGlob(glob string) *regexp.Regexp {
	if glob == "" {
		return nil
	}
	pattern := "^"
	for _, r := range glob {
		switch r {
		case '*':
			pattern += "[^/]*"
		case '?':
			pattern += "[^/]"
		case '.', '+', '(', ')', '[', ']', '{', '}', '^', '$', '|', '\\':
			pattern += "\\" + string(r)
		default:
			pattern += string(r)
		}
	}
	pattern += "$"
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return re
}

// isLikelyBinary returns true for files we shouldn't grep (.png, .jpg, .exe, etc.).
func isLikelyBinary(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".ico", ".webp",
		".pdf", ".doc", ".docx", ".xls", ".xlsx",
		".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz",
		".exe", ".dll", ".so", ".dylib", ".a", ".o",
		".mp3", ".mp4", ".mov", ".avi", ".mkv", ".wav",
		".db", ".sqlite", ".sqlite3",
		".class", ".jar", ".war":
		return true
	}
	return false
}

// Compile-time interface check.
var _ kyoci.Tool = (*GrepTool)(nil)
