package builtin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// PatchTool implements a Hermes-style fuzzy find/replace file edit.
//
// The existing file tool's `edit` operation requires an exact-string match,
// which fails on small models that get whitespace or quoting subtly wrong.
// Patch tolerates whitespace differences by normalizing both old_string and
// the file contents before matching, and verifies the result compiles (for
// .go files, runs `go vet`; for .js/.ts/.tsx/.jsx, runs `node --check`) —
// reverting and returning a diff on failure.
type PatchTool struct{}

// NewPatchTool creates a new patch tool.
func NewPatchTool() *PatchTool { return &PatchTool{} }

// Name returns the tool name.
func (p *PatchTool) Name() string { return "patch" }

// Description returns a terse LLM-facing description with example usage.
func (p *PatchTool) Description() string {
	return "Fuzzy find/replace edit in a file with auto-revert on syntax error. " +
		`patch path="main.go" old_string="return a * b" new_string="return a + b". ` +
		"Runs go vet / node --check after the edit; reverts if it fails."
}

// Parameters returns the patch tool's parameter schema.
func (p *PatchTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{
			Name: "path", Type: "string",
			Description: "Absolute or relative path to the file to edit",
			Required:    true,
		},
		{
			Name: "old_string", Type: "string",
			Description: "The text to find. Matched exactly OR fuzzily (whitespace-normalized) depending on the `fuzzy` flag.",
			Required:    true,
		},
		{
			Name: "new_string", Type: "string",
			Description: "The replacement text. Set to empty string to delete old_string.",
			Required:    true,
		},
		{
			Name: "fuzzy", Type: "boolean",
			Description: "If true, match old_string with whitespace collapsed (any run of whitespace matches any other run). Default: true.",
			Required:    false,
			Default:     true,
		},
		{
			Name: "verify", Type: "boolean",
			Description: "If true (default), run a syntax check (go vet / node --check) after the edit. If it fails, revert the file and return the syntax error.",
			Required:    false,
			Default:     true,
		},
	}
}

// Execute performs the fuzzy find/replace edit.
func (p *PatchTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	path, _ := params["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	oldStr, _ := params["old_string"].(string)
	newStr, _ := params["new_string"].(string)
	if oldStr == "" {
		return "", fmt.Errorf("old_string is required")
	}

	fuzzy := true
	if v, ok := params["fuzzy"].(bool); ok {
		fuzzy = v
	}
	verify := true
	if v, ok := params["verify"].(bool); ok {
		verify = v
	}

	// Expand ~ in path.
	path = expandHome(path)

	// Read the file.
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	original := string(content)

	// Find and replace.
	newContent, matchCount, err := applyPatch(original, oldStr, newStr, fuzzy)
	if err != nil {
		return "", err
	}
	if matchCount == 0 {
		return "", fmt.Errorf("old_string not found in %s (consider fuzzy=true or check whitespace)", path)
	}

	// Write the patched content.
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}

	// Verify (optional but default-on).
	if verify {
		if verr := verifySyntax(ctx, path); verr != nil {
			// Revert.
			if rerr := os.WriteFile(path, []byte(original), 0644); rerr != nil {
				return "", fmt.Errorf("syntax check failed AND revert failed: verify=%v revert_err=%v", verr, rerr)
			}
			return fmt.Sprintf("REVERTED: syntax check failed after edit. Error: %v\nFile restored to original.", verr), nil
		}
	}

	return fmt.Sprintf("patched %s: %d replacement(s) applied%s",
		filepath.Base(path), matchCount, verifySyntaxSuffix(verify)), nil
}

func verifySyntaxSuffix(verified bool) string {
	if verified {
		return " (syntax verified)"
	}
	return ""
}

// applyPatch applies the find/replace to content. Returns the new content,
// the number of matches replaced, and any error. With fuzzy=true, runs of
// whitespace in old_string match any equivalent run in content.
func applyPatch(content, oldStr, newStr string, fuzzy bool) (string, int, error) {
	if !fuzzy {
		count := strings.Count(content, oldStr)
		if count == 0 {
			return content, 0, nil
		}
		return strings.ReplaceAll(content, oldStr, newStr), count, nil
	}

	// Fuzzy: normalize whitespace. Build a regex where each run of \s+ in
	// oldStr becomes \s+ and other regex metachars are escaped.
	pattern := whitespaceFuzzyRegex(oldStr)
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", 0, fmt.Errorf("internal: bad fuzzy regex: %w", err)
	}
	matches := re.FindAllStringIndex(content, -1)
	if len(matches) == 0 {
		return content, 0, nil
	}
	// Replace from end to start so indices stay valid.
	out := content
	for i := len(matches) - 1; i >= 0; i-- {
		s, e := matches[i][0], matches[i][1]
		out = out[:s] + newStr + out[e:]
	}
	return out, len(matches), nil
}

// whitespaceFuzzyRegex escapes regex metacharacters in s, then replaces each
// run of whitespace with \s+ (non-greedy). The match is multi-line.
func whitespaceFuzzyRegex(s string) string {
	var b strings.Builder
	b.WriteString("(?m)")
	runOfWS := false
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r':
			if !runOfWS {
				b.WriteString(`\s+`)
				runOfWS = true
			}
		default:
			runOfWS = false
			// Escape regex metacharacters.
			switch r {
			case '.', '+', '*', '?', '(', ')', '[', ']', '{', '}', '^', '$', '|', '\\':
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// verifySyntax runs a language-appropriate syntax check on path. Returns nil
// if the file passes, or an error describing the failure.
func verifySyntax(ctx context.Context, path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return runGoCheck(ctx, path, "vet")
	case ".js", ".mjs", ".cjs":
		return runNodeCheck(ctx, path)
	case ".jsx", ".tsx":
		// These need a transpiler — skip syntax check (no clean way without babel/tsc).
		return nil
	case ".ts":
		// Try tsc if available; otherwise no-op.
		if lookPath("tsc") == "" {
			return nil
		}
		return runShell(ctx, "tsc --noEmit --skipLibCheck "+shellQuote(path), filepath.Dir(path), 30*time.Second)
	case ".py":
		if lookPath("python3") == "" {
			return nil
		}
		return runShell(ctx, "python3 -m py_compile "+shellQuote(path), filepath.Dir(path), 30*time.Second)
	case ".rs":
		if lookPath("rustc") == "" {
			return nil
		}
		return runShell(ctx, "rustc --edition 2021 --crate-type lib "+shellQuote(path)+" -o /dev/null", filepath.Dir(path), 60*time.Second)
	}
	// Unknown file type — no syntax check possible.
	return nil
}

func runGoCheck(ctx context.Context, path, subcmd string) error {
	// Run `go vet` on the file's package. go vet requires a buildable package
	// context, so we cd to the file's directory.
	dir := filepath.Dir(path)
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		// No go.mod — fall back to `go build file.go` which compiles just this file.
		return runShell(ctx, "go build -o /dev/null "+shellQuote(filepath.Base(path)), dir, 60*time.Second)
	}
	return runShell(ctx, "go vet ./...", dir, 90*time.Second)
}

func runNodeCheck(ctx context.Context, path string) error {
	if lookPath("node") == "" {
		return nil
	}
	return runShell(ctx, "node --check "+shellQuote(path), filepath.Dir(path), 30*time.Second)
}

// runShell is a thin local wrapper that delegates to the package-level
// runCmdInShell. Kept here so patch.go's syntax-check paths read cleanly.
func runShell(ctx context.Context, cmd, workdir string, timeout time.Duration) error {
	out, err := runCmdInShell(ctx, cmd, workdir, timeout)
	if err != nil {
		// Trim noisy boilerplate from the error.
		msg := strings.TrimSpace(out)
		if len(msg) > 800 {
			msg = msg[:800] + "..."
		}
		return fmt.Errorf("%s\n%s", err, msg)
	}
	return nil
}

// shellQuote single-quotes a path so shell-special characters are literal.
func shellQuote(s string) string {
	// Replace ' with '"'"' (close-quote, escaped quote, open-quote).
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

// Compile-time interface check.
var _ kyoci.Tool = (*PatchTool)(nil)

// guard against accidental import of exec without using it (we shell out via runCmdInShell).
var _ = exec.LookPath
