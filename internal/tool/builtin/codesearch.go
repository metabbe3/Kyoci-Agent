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

// CodeSearchTool is "grep + context expansion": it returns the function or
// type declaration CONTAINING each match, not just the matched line. Saves
// the worker a follow-up `file op=read` after every grep hit — a common
// pattern that doubles the worker's tool-call budget.
type CodeSearchTool struct{}

func NewCodeSearchTool() *CodeSearchTool { return &CodeSearchTool{} }

func (c *CodeSearchTool) Name() string { return "codesearch" }

func (c *CodeSearchTool) Description() string {
	return "Search code and return enclosing function/type for each match. " +
		`codesearch pattern="func.*Handler" path="./src" lang=go max_results=10. ` +
		"Returns the full function body, not just the matched line."
}

func (c *CodeSearchTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{Name: "pattern", Type: "string", Required: true,
			Description: "Regex to search for"},
		{Name: "path", Type: "string", Required: false, Default: ".",
			Description: "Directory to search"},
		{Name: "lang", Type: "string", Required: false, Default: "go",
			EnumValues:  []string{"go", "ts", "js", "py", "rs", "java"},
			Description: "Language (determines function-boundary detection)"},
		{Name: "max_results", Type: "integer", Required: false, Default: 10,
			Description: "Maximum matches to return"},
	}
}

func (c *CodeSearchTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	pattern, _ := params["pattern"].(string)
	if pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	root, _ := params["path"].(string)
	if root == "" {
		root = "."
	}
	root = expandHome(root)
	lang, _ := params["lang"].(string)
	if lang == "" {
		lang = "go"
	}
	maxResults := 10
	if v, ok := params["max_results"].(int); ok && v > 0 {
		maxResults = v
	}
	if v, ok := params["max_results"].(float64); ok && v > 0 {
		maxResults = int(v)
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex: %w", err)
	}

	exts := langExts(lang)
	type hit struct {
		file     string
		startLine int
		fn       string
		body     string
	}
	var hits []hit

	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
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
		ext := strings.ToLower(filepath.Ext(p))
		if _, ok := exts[ext]; !ok {
			return nil
		}
		if isLikelyBinary(p) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if !re.MatchString(line) {
				continue
			}
			start, fn := findEnclosingFunction(lines, i, lang)
			if start < 0 {
				start = i
				fn = "(unknown scope)"
			}
			// Extract up to ~40 lines from start, or to the next function boundary.
			end := detectFunctionEnd(lines, start, lang, 40)
			body := strings.Join(lines[start:end], "\n")
			hits = append(hits, hit{p, start + 1, fn, body})
			if len(hits) >= maxResults {
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk: %w", err)
	}

	if len(hits) == 0 {
		return "(no matches)", nil
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].file != hits[j].file {
			return hits[i].file < hits[j].file
		}
		return hits[i].startLine < hits[j].startLine
	})
	var b strings.Builder
	fmt.Fprintf(&b, "%d match(es) with enclosing scope:\n\n", len(hits))
	for _, h := range hits {
		fmt.Fprintf(&b, "─── %s:%d (%s) ───\n%s\n\n", h.file, h.startLine, h.fn, strings.TrimRight(h.body, "\n"))
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// langExts returns the set of file extensions associated with a language.
func langExts(lang string) map[string]bool {
	switch lang {
	case "go":
		return map[string]bool{".go": true}
	case "ts":
		return map[string]bool{".ts": true, ".tsx": true}
	case "js":
		return map[string]bool{".js": true, ".jsx": true, ".mjs": true, ".cjs": true}
	case "py":
		return map[string]bool{".py": true}
	case "rs":
		return map[string]bool{".rs": true}
	case "java":
		return map[string]bool{".java": true}
	}
	return map[string]bool{".go": true}
}

// funcDeclPattern matches the start of a function/method/type declaration
// in the given language. Used by findEnclosingFunction.
func funcDeclPattern(lang string) *regexp.Regexp {
	switch lang {
	case "go":
		// `func (...) Name(` or `func Name(` or `type Name`
		return regexp.MustCompile(`^\s*(func\s+(\([^)]+\)\s+)?[A-Z]\w+|type\s+[A-Z]\w+)`)
	case "ts", "js":
		// `function name(`, `const name = (`, `class Name`, `export function`
		return regexp.MustCompile(`^\s*(export\s+)?(async\s+)?(function\s+\w+|const\s+\w+\s*=\s*(\([^)]*\)|\w+)\s*=>|class\s+\w+)`)
	case "py":
		return regexp.MustCompile(`^\s*def\s+\w+`)
	case "rs":
		return regexp.MustCompile(`^\s*(pub\s+)?(fn|impl|struct|enum|trait)\s+\w+`)
	case "java":
		return regexp.MustCompile(`^\s*(public|private|protected)?\s*(static\s+)?[\w<>\[\]]+\s+\w+\s*\(`)
	}
	return regexp.MustCompile(`^\s*(func|function|def|fn|class)\s+\w+`)
}

// findEnclosingFunction walks UP from lineIdx looking for the most recent
// function-declaration line. Returns (startLineIdx, functionName).
// startLineIdx is -1 if no enclosing function is found.
func findEnclosingFunction(lines []string, lineIdx int, lang string) (int, string) {
	re := funcDeclPattern(lang)
	for i := lineIdx; i >= 0; i-- {
		if re.MatchString(lines[i]) {
			return i, extractFnName(lines[i], lang)
		}
	}
	return -1, ""
}

// extractFnName pulls the function/type identifier from a declaration line.
func extractFnName(line, lang string) string {
	// Generic word-boundary capture; works for most languages.
	re := regexp.MustCompile(`(func|function|def|fn|class|type|struct|enum|trait|impl)\s+(\w+)`)
	m := re.FindStringSubmatch(line)
	if m != nil {
		return m[1] + " " + m[2]
	}
	return strings.TrimSpace(line)
}

// detectFunctionEnd returns the line index (exclusive) at which the function
// starting at startIdx ends. Uses indentation heuristics that work across
// curly-brace languages; for Python, returns startIdx + maxLines.
func detectFunctionEnd(lines []string, start int, lang string, maxLines int) int {
	if lang == "py" {
		end := start + maxLines
		if end > len(lines) {
			end = len(lines)
		}
		return end
	}
	// Curly-brace languages: count braces.
	depth := 0
	entered := false
	for i := start; i < len(lines) && i < start+maxLines*2; i++ {
		for _, r := range lines[i] {
			if r == '{' {
				depth++
				entered = true
			} else if r == '}' {
				depth--
				if entered && depth == 0 {
					return i + 1
				}
			}
		}
		if entered && depth == 0 && i > start {
			return i + 1
		}
	}
	end := start + maxLines
	if end > len(lines) {
		end = len(lines)
	}
	return end
}

var _ kyoci.Tool = (*CodeSearchTool)(nil)

// guard against accidental time import removal
var _ = time.Now
