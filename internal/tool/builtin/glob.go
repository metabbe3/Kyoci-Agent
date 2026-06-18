package builtin

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// GlobTool matches files by pattern, supporting `**` recursive globs. Faster
// than repeated `file op=list` calls for discovery.
type GlobTool struct{}

func NewGlobTool() *GlobTool { return &GlobTool{} }

func (g *GlobTool) Name() string { return "glob" }

func (g *GlobTool) Description() string {
	return "Find files matching a pattern. Supports ** for recursive match. " +
		`glob pattern="**/*.go" path="." max_results=200 returns all .go files.`
}

func (g *GlobTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{Name: "pattern", Type: "string", Required: true,
			Description: "Glob pattern. '*' matches within a path segment, '**' matches recursively across segments."},
		{Name: "path", Type: "string", Required: false, Default: ".",
			Description: "Root directory for the search"},
		{Name: "max_results", Type: "integer", Required: false, Default: 200,
			Description: "Maximum files to return"},
	}
}

func (g *GlobTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	pattern, _ := params["pattern"].(string)
	if pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	root, _ := params["path"].(string)
	if root == "" {
		root = "."
	}
	root = expandHome(root)
	maxResults := 200
	if v, ok := params["max_results"].(int); ok && v > 0 {
		maxResults = v
	}
	if v, ok := params["max_results"].(float64); ok && v > 0 {
		maxResults = int(v)
	}

	// Compile the pattern to a regex matcher that supports **.
	matcher := compileGlobstar(pattern)

	var matches []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == "vendor" || base == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		// Make path relative to root for matching against pattern.
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			rel = p
		}
		rel = filepath.ToSlash(rel)
		if matcher.Match(rel) {
			matches = append(matches, p)
			if len(matches) >= maxResults {
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk: %w", err)
	}

	if len(matches) == 0 {
		return fmt.Sprintf("(no files matching %q under %s)", pattern, root), nil
	}
	sort.Strings(matches)
	var b strings.Builder
	fmt.Fprintf(&b, "%d file(s) matching %q:\n", len(matches), pattern)
	for _, m := range matches {
		// Annotate directories vs files clearly.
		if info, err := os.Stat(m); err == nil {
			if info.IsDir() {
				fmt.Fprintf(&b, "[DIR]  %s\n", m)
				continue
			}
		}
		fmt.Fprintf(&b, "       %s\n", m)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// globstarMatcher wraps a regex compiled from a globstar pattern.
type globstarMatcher struct {
	re *globstarRegex
}

// Match reports whether rel (slash-separated, relative) matches the pattern.
func (m globstarMatcher) Match(rel string) bool {
	if m.re == nil {
		return false
	}
	return m.re.MatchString(rel)
}

// compileGlobstar turns a glob pattern (with ** support) into a regex.
//
// Pattern syntax:
//
//	**  → matches any number of path segments (including zero)
//	*   → matches any run of non-/ characters
//	?   → matches a single non-/ character
//	.   → literal dot
//	other regex metachars → escaped
func compileGlobstar(pattern string) globstarMatcher {
	var b strings.Builder
	b.WriteString("^")
	i := 0
	for i < len(pattern) {
		// Check for ** (possibly followed by /).
		if i+1 < len(pattern) && pattern[i] == '*' && pattern[i+1] == '*' {
			// Consume trailing slash so "**/foo" → ".*/foo" not ".*/foo" with extra slash.
			if i+2 < len(pattern) && pattern[i+2] == '/' {
				b.WriteString(`(?:.*/)?`) // optional dir prefix
				i += 3
				continue
			}
			b.WriteString(`.*`)
			i += 2
			continue
		}
		c := pattern[i]
		switch c {
		case '*':
			b.WriteString(`[^/]*`)
		case '?':
			b.WriteString(`[^/]`)
		case '.', '+', '(', ')', '[', ']', '{', '}', '^', '$', '|', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
		i++
	}
	b.WriteString("$")
	re, err := compileRegex(b.String())
	if err != nil {
		return globstarMatcher{}
	}
	return globstarMatcher{re: re}
}

// globstarRegex is a thin alias to *regexp.Regexp via a helper, kept here so
// the file's only stdlib imports are clear. The actual compile happens in
// the regexp stdlib.
type globstarRegex = regexpAdapter

// Compile-time interface check.
var _ kyoci.Tool = (*GlobTool)(nil)
