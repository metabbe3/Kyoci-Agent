package builtin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// WebFetchTool fetches a URL and returns the body as cleaned-up markdown.
// Pairs with the existing `http_client` (which returns raw HTML) by stripping
// scripts, nav, and styling, then converting basic HTML to markdown — making
// small-model extraction dramatically more reliable.
type WebFetchTool struct{}

func NewWebFetchTool() *WebFetchTool { return &WebFetchTool{} }

func (w *WebFetchTool) Name() string { return "web_fetch" }

func (w *WebFetchTool) Description() string {
	return "Fetch a URL and return cleaned-up markdown (strips <script>/<style>/<nav>). " +
		`web_fetch url="https://example.com" max_chars=4000. ` +
		"Returns plain readable text — much smaller models handle it better than raw HTML."
}

func (w *WebFetchTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{Name: "url", Type: "string", Required: true,
			Description: "HTTP(S) URL to fetch"},
		{Name: "max_chars", Type: "integer", Required: false, Default: 8000,
			Description: "Maximum chars to return (truncates longer pages)"},
		{Name: "timeout", Type: "integer", Required: false, Default: 30,
			Description: "Request timeout in seconds"},
	}
}

func (w *WebFetchTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	url, _ := params["url"].(string)
	if url == "" {
		return "", fmt.Errorf("url is required")
	}
	maxChars := 8000
	if v, ok := params["max_chars"].(int); ok && v > 0 {
		maxChars = v
	}
	if v, ok := params["max_chars"].(float64); ok && v > 0 {
		maxChars = int(v)
	}
	timeoutSecs := 30
	if v, ok := params["timeout"].(int); ok && v > 0 {
		timeoutSecs = v
	}
	if v, ok := params["timeout"].(float64); ok && v > 0 {
		timeoutSecs = int(v)
	}

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	}
	req.Header.Set("User-Agent", "Kyoci-Agent/5.0 (+local-model-friendly fetch)")
	req.Header.Set("Accept", "text/html, text/plain, */*")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024)) // 2MB cap
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	ct := resp.Header.Get("Content-Type")
	content := string(body)

	// If the response is already plain text or markdown, return as-is.
	if strings.Contains(ct, "text/plain") || strings.Contains(ct, "text/markdown") || strings.Contains(ct, "application/json") {
		return truncateForFetch(content, maxChars), nil
	}

	// Otherwise: strip noise tags and convert basic HTML to text.
	cleaned := htmlToReadableMarkdown(content)
	return truncateForFetch(cleaned, maxChars), nil
}

// htmlToReadableMarkdown strips <script>, <style>, <nav>, <footer>, <header> blocks and
// then collapses remaining HTML tags. Headings (h1-h6) become markdown-style
// "# " prefixes; paragraphs and list items get newlines. Links keep their
// text. It's a lossy best-effort cleaner, not a full HTML→MD converter.
func htmlToReadableMarkdown(s string) string {
	// Strip script/style/nav blocks.
	for _, tag := range []string{"script", "style", "nav", "footer", "header", "noscript", "svg"} {
		re := regexp.MustCompile(`(?is)<` + tag + `[^>]*>.*?</` + tag + `>`)
		s = re.ReplaceAllString(s, "")
	}
	// HTML comments.
	s = regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(s, "")
	// Headings → markdown.
	for i := 6; i >= 1; i-- {
		re := regexp.MustCompile(`(?is)<h` + fmt.Sprintf("%d", i) + `[^>]*>(.*?)</h` + fmt.Sprintf("%d", i) + `>`)
		prefix := strings.Repeat("#", i) + " "
		s = re.ReplaceAllString(s, prefix+`${1}`+"\n\n")
	}
	// Paragraphs, list items, table cells → newlines.
	for _, tag := range []string{"p", "li", "tr", "br", "div"} {
		re := regexp.MustCompile(`(?is)<` + tag + `[^>]*>`)
		s = re.ReplaceAllString(s, "\n")
		reClose := regexp.MustCompile(`(?is)</` + tag + `>`)
		s = reClose.ReplaceAllString(s, "\n")
	}
	// Strip all remaining tags.
	s = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(s, "")
	// Decode common entities.
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	// Collapse whitespace.
	s = regexp.MustCompile(`[ \t]+`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func truncateForFetch(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n\n...(truncated)"
}

var _ kyoci.Tool = (*WebFetchTool)(nil)
