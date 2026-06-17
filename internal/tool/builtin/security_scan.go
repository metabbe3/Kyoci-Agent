package builtin

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/metabbe3/Kyoci-Agent/pkg"
)

// SecurityScanTool scans code files for OWASP Top 10 vulnerabilities.
type SecurityScanTool struct {
	logger *slog.Logger
}

func NewSecurityScanTool() *SecurityScanTool {
	return &SecurityScanTool{logger: slog.Default()}
}

func (s *SecurityScanTool) Name() string {
	return "security_scan"
}

func (s *SecurityScanTool) Description() string {
	return "Scan code files for OWASP Top 10 security vulnerabilities. Checks for SQL injection, XSS, hardcoded secrets, IDOR, weak crypto, command injection, and CORS issues. Pass a file path or directory."
}

func (s *SecurityScanTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{
			Name:        "path",
			Type:        "string",
			Description: "File path or directory to scan for security vulnerabilities",
			Required:    true,
		},
	}
}

type Vulnerability struct {
	Severity    string
	Category    string
	File        string
	Line        int
	Description string
	BadCode     string
	Fix         string
}

type scanRule struct {
	Pattern  *regexp.Regexp
	Severity string
	Category string
	Desc     string
	Fix      string
}

// All OWASP detection rules — compiled once at init.
var compiledRules = func() []scanRule {
	return []scanRule{
		// === SQL INJECTION ===
		{
			Pattern:  regexp.MustCompile(`(?i)(?:SELECT|INSERT|UPDATE|DELETE|DROP)\s+.*\+\s*\w+`),
			Severity: "CRITICAL", Category: "A03:2021 - Injection (SQL)",
			Desc: "Potential SQL injection: string concatenation in SQL query",
			Fix:  "Use parameterized queries: db.query(\"SELECT ... WHERE id = $1\", [id])",
		},
		{
			Pattern:  regexp.MustCompile(`(?i)execute\s*\(\s*["'` + "`" + `]\s*(?:SELECT|INSERT|UPDATE|DELETE).*\+`),
			Severity: "CRITICAL", Category: "A03:2021 - Injection (SQL)",
			Desc: "Dynamic SQL execution with concatenation",
			Fix:  "Use prepared statements with bound parameters",
		},

		// === HARDCODED SECRETS ===
		{
			Pattern:  regexp.MustCompile(`(?i)(?:api[_-]?key|apikey|secret|password|passwd|token|auth)\s*[:=]\s*["'][^"']{8,}["']`),
			Severity: "CRITICAL", Category: "A02:2021 - Cryptographic Failures",
			Desc: "Hardcoded secret/credential in source code",
			Fix:  "Move to environment variable: process.env.API_KEY",
		},
		{
			Pattern:  regexp.MustCompile(`(?i)(?:AWS_SECRET_ACCESS_KEY|STRIPE_SECRET|PRIVATE_KEY)\s*[:=]\s*["'][^"']+["']`),
			Severity: "CRITICAL", Category: "A02:2021 - Cryptographic Failures",
			Desc: "Cloud service secret key hardcoded",
			Fix:  "Use secret manager or environment variable. NEVER commit secrets to git.",
		},
		{
			Pattern:  regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA )?PRIVATE KEY-----`),
			Severity: "CRITICAL", Category: "A02:2021 - Cryptographic Failures",
			Desc: "Private key embedded in source code",
			Fix:  "Load private key from secure file or environment variable",
		},
		{
			Pattern:  regexp.MustCompile(`(?i)(?:mongodb|postgres|mysql|redis)://[^:\s]+:[^@\s]+@`),
			Severity: "HIGH", Category: "A02:2021 - Cryptographic Failures",
			Desc: "Database connection string with embedded credentials",
			Fix:  "Build connection string from environment variables at runtime",
		},

		// === XSS ===
		{
			Pattern:  regexp.MustCompile(`dangerouslySetInnerHTML`),
			Severity: "HIGH", Category: "A03:2021 - Injection (XSS)",
			Desc: "dangerouslySetInnerHTML used — potential XSS vector",
			Fix:  "Sanitize with DOMPurify before rendering",
		},
		{
			Pattern:  regexp.MustCompile(`(?i)innerHTML\s*=\s*[^;]*\+`),
			Severity: "HIGH", Category: "A03:2021 - Injection (XSS)",
			Desc: "innerHTML assigned with concatenation — XSS risk",
			Fix:  "Use textContent or sanitize HTML before assignment",
		},

		// === CODE INJECTION ===
		{
			Pattern:  regexp.MustCompile(`(?i)\beval\s*\(`),
			Severity: "HIGH", Category: "A03:2021 - Injection",
			Desc: "eval() used — code injection risk",
			Fix:  "Never use eval(). Use JSON.parse() for data parsing.",
		},

		// === IDOR / ACCESS CONTROL ===
		{
			Pattern:  regexp.MustCompile(`(?i)req\.(?:params|query|body)\.(?:id|userId|ownerId|resourceId)\b`),
			Severity: "MEDIUM", Category: "A01:2021 - Broken Access Control",
			Desc: "Resource ID taken from request — verify ownership before use",
			Fix:  "Check ownership: if (resource.ownerId !== currentUser.id) return 403",
		},

		// === WEAK CRYPTO ===
		{
			Pattern:  regexp.MustCompile(`(?i)\b(?:md5|sha1)\s*\(`),
			Severity: "MEDIUM", Category: "A02:2021 - Cryptographic Failures",
			Desc: "Weak hash algorithm (MD5/SHA1) used",
			Fix:  "Use SHA-256 or bcrypt for passwords",
		},

		// === DISABLED SECURITY ===
		{
			Pattern:  regexp.MustCompile(`(?i)rejectUnauthorized\s*:\s*false`),
			Severity: "HIGH", Category: "A05:2021 - Security Misconfiguration",
			Desc: "TLS certificate validation disabled",
			Fix:  "Never disable cert validation in production",
		},
		{
			Pattern:  regexp.MustCompile(`(?i)(?:cors|Access-Control-Allow-Origin).*\*`),
			Severity: "MEDIUM", Category: "A05:2021 - Security Misconfiguration",
			Desc: "CORS set to allow all origins (*)",
			Fix:  "Specify allowed origins explicitly",
		},

		// === COMMAND INJECTION ===
		{
			Pattern:  regexp.MustCompile(`(?i)(?:exec|execSync|spawn)\s*\(\s*[^"'` + "`" + `\s]+(?:\+|` + "`" + `.*\$\{)`),
			Severity: "CRITICAL", Category: "A03:2021 - Injection (Command)",
			Desc: "Command execution with dynamic input — command injection risk",
			Fix:  "Use execFile/spawn with argument array. NEVER concatenate user input.",
		},

		// === NO ERROR HANDLING ===
		{
			Pattern:  regexp.MustCompile(`(?i)await\s+fetch\s*\([^)]+\)\s*$`),
			Severity: "LOW", Category: "Code Quality",
			Desc: "fetch() without error handling",
			Fix:  "Wrap in try-catch: try { const res = await fetch(url) } catch(e) { ... }",
		},
	}
}()

func (s *SecurityScanTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("path parameter is required")
	}

	s.logger.Info("scanning for vulnerabilities", "path", path)

	var files []string
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("path not found: %s", path)
	}

	if info.IsDir() {
		filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				// Skip known irrelevant directories entirely
				if fi != nil && fi.IsDir() {
					base := strings.ToLower(fi.Name())
					if base == "node_modules" || base == ".git" ||
						base == "vendor" || base == "dist" ||
						base == "build" || base == "__pycache__" ||
						strings.HasPrefix(base, "_archive") {
						return filepath.SkipDir
					}
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(p))
			switch ext {
			case ".js", ".ts", ".jsx", ".tsx", ".py", ".go", ".java", ".rb",
				".php", ".cs", ".rs", ".swift", ".kt", ".sql", ".sh", ".html":
				files = append(files, p)
			}
			return nil
		})
	} else {
		files = []string{path}
	}

	if len(files) == 0 {
		return "No source code files found to scan.", nil
	}

	var vulns []Vulnerability
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		lines := strings.Split(string(content), "\n")

		for lineNum, line := range lines {
			trimmed := strings.TrimSpace(line)
			// Skip comments
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") ||
				strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
				continue
			}
			for _, rule := range compiledRules {
				if rule.Pattern.MatchString(line) {
					vulns = append(vulns, Vulnerability{
						Severity:    rule.Severity,
						Category:    rule.Category,
						File:        file,
						Line:        lineNum + 1,
						Description: rule.Desc,
						BadCode:     trimmed,
						Fix:         rule.Fix,
					})
				}
			}
		}
	}

	return s.formatReport(files, vulns), nil
}

func (s *SecurityScanTool) formatReport(files []string, vulns []Vulnerability) string {
	var sb strings.Builder

	sb.WriteString("SECURITY SCAN REPORT\n")
	sb.WriteString(fmt.Sprintf("Files scanned: %d\n", len(files)))
	sb.WriteString(fmt.Sprintf("Vulnerabilities found: %d\n\n", len(vulns)))

	if len(vulns) == 0 {
		sb.WriteString("RESULT: CLEAN — No vulnerabilities detected.\n")
		sb.WriteString("Note: Automated scan. Manual security review still recommended.\n")
		return sb.String()
	}

	critical, high, medium, low := 0, 0, 0, 0
	for _, v := range vulns {
		switch v.Severity {
		case "CRITICAL":
			critical++
		case "HIGH":
			high++
		case "MEDIUM":
			medium++
		case "LOW":
			low++
		}
	}

	sb.WriteString(fmt.Sprintf("  CRITICAL: %d\n", critical))
	sb.WriteString(fmt.Sprintf("  HIGH:     %d\n", high))
	sb.WriteString(fmt.Sprintf("  MEDIUM:   %d\n", medium))
	sb.WriteString(fmt.Sprintf("  LOW:      %d\n\n", low))

	if critical > 0 {
		sb.WriteString("RESULT: FAIL — CRITICAL vulnerabilities MUST be fixed before deployment.\n\n")
	} else if high > 0 {
		sb.WriteString("RESULT: WARN — HIGH severity issues should be fixed.\n\n")
	} else {
		sb.WriteString("RESULT: REVIEW — Minor issues found.\n\n")
	}

	for i, v := range vulns {
		sb.WriteString(fmt.Sprintf("--- Finding #%d ---\n", i+1))
		sb.WriteString(fmt.Sprintf("Severity: %s\n", v.Severity))
		sb.WriteString(fmt.Sprintf("Category: %s\n", v.Category))
		sb.WriteString(fmt.Sprintf("File: %s:%d\n", v.File, v.Line))
		sb.WriteString(fmt.Sprintf("Issue: %s\n", v.Description))
		sb.WriteString(fmt.Sprintf("Code: %s\n", v.BadCode))
		sb.WriteString(fmt.Sprintf("Fix: %s\n\n", v.Fix))
	}

	return sb.String()
}
