package builtin

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/platform"
	"github.com/metabbe3/Kyoci-Agent/pkg"
)

// BrowserTool implements browser operations: open URLs, fetch web content.
type BrowserTool struct {
	logger *slog.Logger
	client *http.Client
}

// NewBrowserTool creates a new browser tool instance.
func NewBrowserTool() *BrowserTool {
	return &BrowserTool{
		logger: slog.Default(),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the tool name.
func (b *BrowserTool) Name() string {
	return "browser"
}

// Description returns the tool description.
func (b *BrowserTool) Description() string {
	return "Open URLs in the default browser or fetch web page content as clean text. Actions: 'open' (open URL in browser), 'fetch' (get page content as text), 'title' (get page title)."
}

// Parameters returns the tool parameter definition.
func (b *BrowserTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{
			Name:        "action",
			Type:        "string",
			Description: "Action: 'open' (open in browser), 'fetch' (get page text), 'title' (get page title only)",
			Required:    true,
			EnumValues:  []string{"open", "fetch", "title"},
		},
		{
			Name:        "url",
			Type:        "string",
			Description: "The URL to open or fetch",
			Required:    true,
		},
	}
}

// Execute performs the browser action.
func (b *BrowserTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	action, ok := params["action"].(string)
	if !ok || action == "" {
		return "", fmt.Errorf("action parameter is required (open, fetch, or title)")
	}

	url, ok := params["url"].(string)
	if !ok || url == "" {
		return "", fmt.Errorf("url parameter is required")
	}

	// Add https:// if no scheme
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	switch action {
	case "open":
		return b.openURL(ctx, url)
	case "fetch":
		return b.fetchContent(ctx, url, false)
	case "title":
		return b.fetchContent(ctx, url, true)
	default:
		return "", fmt.Errorf("unknown action: %s (use open, fetch, or title)", action)
	}
}

// openURL opens a URL in the default system browser.
func (b *BrowserTool) openURL(ctx context.Context, url string) (string, error) {
	cmdStr := platform.OpenBrowserCommand(url)
	b.logger.Info("opening browser", "url", url, "command", cmdStr)

	shell := platform.ShellPath()
	args := append(platform.ShellArgs(), cmdStr)

	cmd := exec.CommandContext(ctx, shell, args...)
	if err := cmd.Run(); err != nil {
		b.logger.Warn("browser open failed", "url", url, "error", err)
		return fmt.Sprintf("Failed to open %s in browser: %v", url, err), nil
	}

	b.logger.Info("browser opened successfully", "url", url)
	return fmt.Sprintf("Opened %s in browser.", url), nil
}

// fetchContent fetches a web page and extracts clean text.
func (b *BrowserTool) fetchContent(ctx context.Context, url string, titleOnly bool) (string, error) {
	b.logger.Info("fetching web page", "url", url, "title_only", titleOnly)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set a realistic User-Agent to avoid blocks
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := b.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read up to 2MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	htmlStr := string(body)

	// Extract title
	title := extractTitle(htmlStr)

	if titleOnly {
		if title == "" {
			return fmt.Sprintf("No title found for %s", url), nil
		}
		return title, nil
	}

	// Extract clean text
	text := htmlToText(htmlStr)

	// Build result
	var result strings.Builder
	result.WriteString(fmt.Sprintf("URL: %s\n", url))
	if title != "" {
		result.WriteString(fmt.Sprintf("Title: %s\n", title))
	}
	result.WriteString(fmt.Sprintf("Status: %d\n\n", resp.StatusCode))
	result.WriteString(text)

	// Truncate to ~8000 chars for 8B model context window
	resultStr := result.String()
	if len(resultStr) > 8000 {
		resultStr = resultStr[:8000] + "\n\n[Content truncated — page was too long]"
	}

	b.logger.Info("web page fetched", "url", url, "title", title, "content_len", len(resultStr))
	return resultStr, nil
}

// extractTitle extracts the <title> from HTML.
var titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

func extractTitle(html string) string {
	matches := titleRe.FindStringSubmatch(html)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(decodeEntities(matches[1]))
}

// htmlToText strips HTML tags and returns clean text.
func htmlToText(html string) string {
	// Remove script and style blocks entirely
	html = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?is)<nav[^>]*>.*?</nav>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?is)<footer[^>]*>.*?</footer>`).ReplaceAllString(html, "")

	// Convert common block tags to newlines
	html = regexp.MustCompile(`(?i)<br\s*/?>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`(?i)</(p|div|h[1-6]|li|tr|blockquote)>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`(?i)<(p|div|h[1-6]|li|tr|blockquote)[^>]*>`).ReplaceAllString(html, "\n")

	// Remove all remaining HTML tags
	html = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, "")

	// Decode HTML entities
	html = decodeEntities(html)

	// Clean up whitespace
	html = strings.ReplaceAll(html, "\r", "\n")
	html = regexp.MustCompile(`\n{3,}`).ReplaceAllString(html, "\n\n")
	html = regexp.MustCompile(`[ \t]{2,}`).ReplaceAllString(html, " ")

	// Trim each line
	lines := strings.Split(html, "\n")
	var cleanLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleanLines = append(cleanLines, trimmed)
		}
	}

	return strings.Join(cleanLines, "\n")
}

// decodeEntities decodes common HTML entities.
func decodeEntities(s string) string {
	entities := map[string]string{
		"&amp;":  "&",
		"&lt;":   "<",
		"&gt;":   ">",
		"&quot;": "\"",
		"&#39;":  "'",
		"&apos;": "'",
		"&nbsp;": " ",
		"&ndash;": "–",
		"&mdash;": "—",
		"&hellip;": "…",
		"&laquo;": "«",
		"&raquo;": "»",
		"&copy;": "©",
		"&reg;":  "®",
		"&trade;": "™",
		"&euro;": "€",
		"&pound;": "£",
		"&yen;":  "¥",
		"&deg;":  "°",
		"&plusmn;": "±",
		"&times;": "×",
		"&divide;": "÷",
	}
	for entity, char := range entities {
		s = strings.ReplaceAll(s, entity, char)
	}
	// Decode numeric entities &#NNN;
	s = regexp.MustCompile(`&#(\d+);`).ReplaceAllStringFunc(s, func(m string) string {
		var num int
		fmt.Sscanf(m, "&#%d;", &num)
		if num > 0 && num < 0x10000 {
			return string(rune(num))
		}
		return m
	})
	// Decode hex entities &#xNN;
	s = regexp.MustCompile(`(?i)&#x([0-9a-f]+);`).ReplaceAllStringFunc(s, func(m string) string {
		var num int
		fmt.Sscanf(m, "&#x%x;", &num)
		if num > 0 && num < 0x10000 {
			return string(rune(num))
		}
		return m
	})
	return s
}
