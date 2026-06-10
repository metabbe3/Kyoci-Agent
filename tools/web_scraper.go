package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// WebScraperTool extracts data from HTML pages using simple string matching
type WebScraperTool struct {
	client *http.Client
}

// NewWebScraperTool creates a new web scraper tool
func NewWebScraperTool() *WebScraperTool {
	return &WebScraperTool{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *WebScraperTool) Name() string {
	return "web_scraper"
}

func (t *WebScraperTool) Description() string {
	return "Extract data from HTML pages. Supports simple CSS selector-based extraction (class, id, tag) with text, HTML content, or attribute values. Uses simple string matching - no external HTML parser."
}

func (t *WebScraperTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL of the HTML page to scrape",
			},
			"css_selector": map[string]interface{}{
				"type":        "string",
				"description": "CSS selector (supports .class, #id, tagname combinations like div.content, .item.title)",
			},
			"extract": map[string]interface{}{
				"type":        "string",
				"description": "What to extract: text (inner text), html (inner HTML), or attr (attribute value)",
				"enum":        []string{"text", "html", "attr"},
			},
			"attribute": map[string]interface{}{
				"type":        "string",
				"description": "Attribute name to extract (required when extract=attr, e.g., href, src, class)",
			},
		},
		"required": []string{"url", "css_selector"},
	}
}

type scraperParams struct {
	URL         string `json:"url"`
	CSSSelector string `json:"css_selector"`
	Extract     string `json:"extract"`
	Attribute   string `json:"attribute"`
}

func (t *WebScraperTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var params scraperParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	// Validate required parameters
	if params.URL == "" {
		return "", fmt.Errorf("url is required")
	}
	if params.CSSSelector == "" {
		return "", fmt.Errorf("css_selector is required")
	}

	// Set defaults
	if params.Extract == "" {
		params.Extract = "text"
	}

	// Validate extract mode
	if params.Extract != "text" && params.Extract != "html" && params.Extract != "attr" {
		return "", fmt.Errorf("invalid extract mode: %s (valid: text, html, attr)", params.Extract)
	}

	// Validate attribute for attr mode
	if params.Extract == "attr" && params.Attribute == "" {
		return "", fmt.Errorf("attribute is required when extract=attr")
	}

	// Fetch the HTML
	req, err := http.NewRequestWithContext(ctx, "GET", params.URL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AI-Agent/1.0)")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: failed to fetch URL", resp.StatusCode)
	}

	html, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	htmlStr := string(html)

	// Parse CSS selector and extract data
	results, err := t.extractBySelector(htmlStr, params.CSSSelector, params.Extract, params.Attribute)
	if err != nil {
		return "", fmt.Errorf("extraction failed: %w", err)
	}

	if len(results) == 0 {
		return fmt.Sprintf("No elements found matching selector: %s", params.CSSSelector), nil
	}

	// Format results
	var output []string
	for i, result := range results {
		if len(result) > 1000 {
			result = result[:1000] + "... (truncated)"
		}
		output = append(output, fmt.Sprintf("[%d] %s", i+1, result))
	}

	return strings.Join(output, "\n\n"), nil
}

// extractBySelector parses a simple CSS selector and extracts matching content
func (t *WebScraperTool) extractBySelector(html, selector, extractMode, attribute string) ([]string, error) {
	// Parse the CSS selector into components
	// Supported: tag, .class, #id, and simple combinations like div.content
	parts := t.parseCSSSelector(selector)
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid CSS selector: %s", selector)
	}

	// Find all elements matching the outermost selector
	matches := t.findElements(html, parts)

	// Extract content from matches
	var results []string
	for _, match := range matches {
		var result string
		switch extractMode {
		case "text":
			result = t.extractText(match)
		case "html":
			result = t.extractInnerHTML(match)
		case "attr":
			result = t.extractAttribute(match, attribute)
		}
		if result != "" {
			results = append(results, result)
		}
	}

	return results, nil
}

// parseCSSSelector parses a CSS selector into tag, id, and class components
func (t *WebScraperTool) parseCSSSelector(selector string) []string {
	// Simple parser for: tag, .class, #id, tag.class, tag#id, .class1.class2
	var parts []string

	// Extract tag (first part without # or .)
	tagEnd := strings.IndexAny(selector, "#.")
	if tagEnd == 0 {
		// No tag specified, will match any tag
		parts = append(parts, "*")
	} else if tagEnd > 0 {
		parts = append(parts, selector[:tagEnd])
		selector = selector[tagEnd:]
	} else {
		// Just a tag name
		parts = append(parts, selector)
		return parts
	}

	// Extract IDs (#id)
	for {
		idx := strings.Index(selector, "#")
		if idx == -1 {
			break
		}
		selector = selector[idx+1:]
		idEnd := strings.IndexAny(selector, "#.")
		if idEnd == -1 {
			parts = append(parts, "#"+selector)
			break
		}
		parts = append(parts, "#"+selector[:idEnd])
		selector = selector[idEnd:]
	}

	// Extract classes (.class)
	for {
		idx := strings.Index(selector, ".")
		if idx == -1 {
			break
		}
		selector = selector[idx+1:]
		classEnd := strings.IndexAny(selector, ".#")
		if classEnd == -1 {
			parts = append(parts, "."+selector)
			break
		}
		parts = append(parts, "."+selector[:classEnd])
		selector = selector[classEnd:]
	}

	return parts
}

// findElements finds HTML elements matching the selector parts
func (t *WebScraperTool) findElements(html string, parts []string) []string {
	var matches []string

	var tagPattern string
	var idPattern string
	var classPatterns []string

	for _, part := range parts {
		if strings.HasPrefix(part, "#") {
			idPattern = part[1:]
		} else if strings.HasPrefix(part, ".") {
			classPatterns = append(classPatterns, part[1:])
		} else if part != "*" {
			tagPattern = part
		}
	}

	// Build opening tag regex
	if tagPattern == "" {
		tagPattern = "[a-zA-Z][a-zA-Z0-9]*"
	}
	openTagPattern := `<\s*` + tagPattern

	// Add id requirement
	if idPattern != "" {
		openTagPattern += fmt.Sprintf(`[^>]*\bid\s*=\s*["']%s["']`, regexp.QuoteMeta(idPattern))
	}

	// Add class requirements
	for _, class := range classPatterns {
		openTagPattern += fmt.Sprintf(`[^>]*\bclass\s*=\s*["'][^"']*\b%s\b[^"']*["']`, regexp.QuoteMeta(class))
	}

	openTagPattern += `[^>]*>`

	// Find all matching opening tags
	openTagRegex := regexp.MustCompile(`(?i)` + openTagPattern)
	openTagMatches := openTagRegex.FindAllStringIndex(html, -1)

	for _, match := range openTagMatches {
		startPos := match[0]
		endPos := t.findMatchingClosingTag(html, startPos, match[1])
		if endPos > startPos {
			matches = append(matches, html[startPos:endPos])
		}
	}

	return matches
}

// findMatchingClosingTag finds the matching closing tag for an opening tag
func (t *WebScraperTool) findMatchingClosingTag(html string, tagStart, tagEnd int) int {
	// Extract tag name
	tagNameRegex := regexp.MustCompile(`<\s*([a-zA-Z][a-zA-Z0-9]*)`)
	tagNameMatch := tagNameRegex.FindStringSubmatch(html[tagStart:tagEnd])
	if len(tagNameMatch) < 2 {
		return -1
	}
	tagName := tagNameMatch[1]
	if tagName == "" {
		return -1
	}

	// Self-closing tags don't have closing tags
	selfClosing := map[string]bool{"img": true, "br": true, "hr": true, "input": true, "meta": true, "link": true, "base": true, "col": true, "area": true, "param": true, "wbr": true, "embed": true, "source": true, "track": true}
	if selfClosing[strings.ToLower(tagName)] {
		return tagEnd
	}

	// Check if tag is self-closing (ends with />)
	if strings.HasSuffix(strings.TrimSpace(html[tagStart:tagEnd]), "/>") {
		return tagEnd
	}

	// Find matching closing tag
	openPattern := `<\s*` + tagName + `\b[^>]*>`
	closePattern := `</\s*` + tagName + `\s*>`

	openRegex := regexp.MustCompile(`(?i)` + openPattern)
	closeRegex := regexp.MustCompile(`(?i)` + closePattern)

	// Search from the opening tag
	searchStart := tagEnd
	depth := 1

	for depth > 0 && searchStart < len(html) {
		// Find next opening or closing tag
		nextOpen := openRegex.FindStringIndex(html[searchStart:])
		nextClose := closeRegex.FindStringIndex(html[searchStart:])

		if nextClose == nil {
			return -1 // No closing tag found
		}

		// Adjust positions to be absolute
		if nextOpen != nil {
			nextOpen[0] += searchStart
			nextOpen[1] += searchStart
		}
		nextClose[0] += searchStart
		nextClose[1] += searchStart

		if nextOpen != nil && nextOpen[0] < nextClose[0] {
			depth++
			searchStart = nextOpen[1]
		} else {
			depth--
			if depth == 0 {
				return nextClose[1]
			}
			searchStart = nextClose[1]
		}
	}

	return -1
}

// extractText extracts text content from HTML, removing tags
func (t *WebScraperTool) extractText(html string) string {
	// Remove script and style content
	scriptRegex := regexp.MustCompile(`(?i)<\s*script[^>]*>.*?<\s*/\s*script\s*>`)
	html = scriptRegex.ReplaceAllString(html, "")

	styleRegex := regexp.MustCompile(`(?i)<\s*style[^>]*>.*?<\s*/\s*style\s*>`)
	html = styleRegex.ReplaceAllString(html, "")

	// Remove all HTML tags
	tagRegex := regexp.MustCompile(`<[^>]+>`)
	text := tagRegex.ReplaceAllString(html, " ")

	// Decode common HTML entities
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&apos;", "'")
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&ndash;", "-")
	text = strings.ReplaceAll(text, "&mdash;", "—")
	text = strings.ReplaceAll(text, "&rsquo;", "'")
	text = strings.ReplaceAll(text, "&lsquo;", "'")
	text = strings.ReplaceAll(text, "&rdquo;", "\"")
	text = strings.ReplaceAll(text, "&ldquo;", "\"")

	// Normalize whitespace
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	return text
}

// extractInnerHTML extracts the inner HTML (content between opening and closing tags)
func (t *WebScraperTool) extractInnerHTML(html string) string {
	// Find the first > (end of opening tag)
	tagEnd := strings.Index(html, ">")
	if tagEnd == -1 {
		return ""
	}

	// Find the last < (start of closing tag)
	closeTagStart := strings.LastIndex(html, "<")
	if closeTagStart <= tagEnd {
		// Self-closing tag or no closing tag
		return ""
	}

	return html[tagEnd+1 : closeTagStart]
}

// extractAttribute extracts an attribute value from an HTML element
func (t *WebScraperTool) extractAttribute(html, attrName string) string {
	// Pattern to match attribute="value" or attribute='value'
	attrPattern := regexp.MustCompile(`(?i)\b`+regexp.QuoteMeta(attrName)+`\s*=\s*["']([^"']*)["']`)
	matches := attrPattern.FindStringSubmatch(html)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}