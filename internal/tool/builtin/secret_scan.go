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

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// SecretScanTool scans a directory tree for likely API keys, tokens, and
// private keys. PicoClaw-inspired safety feature — surfaces secrets before
// they leak into commits, pastebins, or PR descriptions.
type SecretScanTool struct{}

func NewSecretScanTool() *SecretScanTool { return &SecretScanTool{} }

func (s *SecretScanTool) Name() string { return "secret_scan" }

func (s *SecretScanTool) Description() string {
	return "Scan a directory for likely secrets (AWS, GitHub, Stripe, Google, JWT, private keys). " +
		`secret_scan path="." max_findings=50. Returns file:line:type for each match.`
}

func (s *SecretScanTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{Name: "path", Type: "string", Required: true,
			Description: "Directory to scan recursively"},
		{Name: "max_findings", Type: "integer", Required: false, Default: 50,
			Description: "Maximum findings to return"},
	}
}

// scanPatterns mirrors the security skill's secretPatterns list, but here as
// a slice of (name, *regexp.Regexp) for efficient reuse across many files.
var scanPatterns = func() []struct {
	name string
	re   *regexp.Regexp
} {
	pairs := []struct {
		name    string
		pattern string
	}{
		{"AWS Access Key", `AKIA[0-9A-Z]{16}`},
		{"AWS Secret Key", `(?m)aws_secret_access_key\s*=\s*[A-Za-z0-9/+=]{40}`},
		{"GitHub Token (ghp_)", `ghp_[A-Za-z0-9]{36}`},
		{"GitHub Token (github_pat_)", `github_pat_[A-Za-z0-9_]{82}`},
		{"Google API Key", `AIza[0-9A-Za-z\-_]{35}`},
		{"Slack Token", `xox[baprs]-[A-Za-z0-9-]{10,}`},
		{"Stripe (sk_live_)", `sk_live_[A-Za-z0-9]{24,}`},
		{"Stripe (rk_live_)", `rk_live_[A-Za-z0-9]{24,}`},
		{"JWT", `eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`},
		{"Generic Bearer Token", `(?i)bearer\s+[A-Za-z0-9_\-\.=]{20,}`},
		{"Private Key Block", `-----BEGIN [A-Z ]*PRIVATE KEY-----`},
	}
	out := make([]struct {
		name string
		re   *regexp.Regexp
	}, len(pairs))
	for i, p := range pairs {
		out[i] = struct {
			name string
			re   *regexp.Regexp
		}{p.name, regexp.MustCompile(p.pattern)}
	}
	return out
}()

func (s *SecretScanTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	root, _ := params["path"].(string)
	if root == "" {
		return "", fmt.Errorf("path is required")
	}
	root = expandHome(root)
	maxFindings := 50
	if v, ok := params["max_findings"].(int); ok && v > 0 {
		maxFindings = v
	}
	if v, ok := params["max_findings"].(float64); ok && v > 0 {
		maxFindings = int(v)
	}

	type finding struct {
		file  string
		line  int
		kind  string
		match string
	}
	var findings []finding

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
		if isLikelyBinary(p) {
			return nil
		}
		// Skip the secret-scan skill file itself, which contains all these patterns.
		if strings.HasSuffix(p, "internal/skill/builtin/security.go") ||
			strings.HasSuffix(p, "internal/tool/builtin/secret_scan.go") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			for _, pat := range scanPatterns {
				if pat.re.MatchString(line) {
					findings = append(findings, finding{
						file:  p,
						line:  i + 1,
						kind:  pat.name,
						match: strings.TrimSpace(line),
					})
					if len(findings) >= maxFindings {
						return filepath.SkipDir
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk: %w", err)
	}
	if len(findings) == 0 {
		return "no secrets found", nil
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].file != findings[j].file {
			return findings[i].file < findings[j].file
		}
		return findings[i].line < findings[j].line
	})
	var b strings.Builder
	fmt.Fprintf(&b, "found %d potential secret(s):\n\n", len(findings))
	for _, f := range findings {
		// Mask the middle of the match to avoid leaking it in tool output.
		masked := maskSecret(f.match)
		fmt.Fprintf(&b, "%s:%d  [%s]\n  %s\n\n", f.file, f.line, f.kind, masked)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// maskSecret redacts the middle of a likely secret so tool output doesn't
// re-expose the very thing it found.
func maskSecret(s string) string {
	if len(s) < 12 {
		return strings.Repeat("*", len(s))
	}
	return s[:6] + strings.Repeat("*", len(s)-12) + s[len(s)-6:]
}

var _ kyoci.Tool = (*SecretScanTool)(nil)
