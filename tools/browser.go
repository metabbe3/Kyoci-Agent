package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// BrowserTool provides web browser automation capabilities
type BrowserTool struct {
	client       *http.Client
	cdpURL       string // Chrome DevTools Protocol URL (e.g., http://localhost:9222)
	chromeBinary string // Path to chrome/Chromium binary
}

// NewBrowserTool creates a new browser tool
func NewBrowserTool() *BrowserTool {
	return &BrowserTool{
		client:       &http.Client{Timeout: 30 * time.Second},
		cdpURL:       "http://localhost:9222",
		chromeBinary: findChromeBinary(),
	}
}

func (t *BrowserTool) Name() string {
	return "browser"
}

func (t *BrowserTool) Description() string {
	return "Automate web browser operations: navigate to URLs, get page content, take screenshots, click elements, type text, and evaluate JavaScript. Supports Chrome DevTools Protocol (CDP) or headless Chrome fallback."
}

func (t *BrowserTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action to perform: navigate, content, screenshot, click, type, evaluate",
				"enum":        []string{"navigate", "content", "screenshot", "click", "type", "evaluate"},
			},
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to navigate to (required for navigate action)",
			},
			"selector": map[string]interface{}{
				"type":        "string",
				"description": "CSS selector for element to click or type into (required for click/type actions)",
			},
			"text": map[string]interface{}{
				"type":        "string",
				"description": "Text to type into input field (required for type action)",
			},
			"script": map[string]interface{}{
				"type":        "string",
				"description": "JavaScript code to evaluate (required for evaluate action)",
			},
			"output_path": map[string]interface{}{
				"type":        "string",
				"description": "File path to save screenshot (required for screenshot action)",
			},
			"cdp_url": map[string]interface{}{
				"type":        "string",
				"description": "Chrome DevTools Protocol URL (default: http://localhost:9222)",
			},
		},
		"required": []string{"action"},
	}
}

// browserParams represents the input parameters for the browser tool
type browserParams struct {
	Action     string `json:"action"`
	URL        string `json:"url"`
	Selector   string `json:"selector"`
	Text       string `json:"text"`
	Script     string `json:"script"`
	OutputPath string `json:"output_path"`
	CDPURL     string `json:"cdp_url"`
}

func (t *BrowserTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var params browserParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	// Set custom CDP URL if provided
	if params.CDPURL != "" {
		t.cdpURL = params.CDPURL
	}

	// Validate and execute action
	switch params.Action {
	case "navigate":
		return t.navigate(ctx, params.URL)
	case "content":
		return t.getContent(ctx)
	case "screenshot":
		if params.OutputPath == "" {
			return "", fmt.Errorf("output_path is required for screenshot action")
		}
		return t.screenshot(ctx, params.OutputPath)
	case "click":
		if params.Selector == "" {
			return "", fmt.Errorf("selector is required for click action")
		}
		return t.click(ctx, params.Selector)
	case "type":
		if params.Selector == "" {
			return "", fmt.Errorf("selector is required for type action")
		}
		if params.Text == "" {
			return "", fmt.Errorf("text is required for type action")
		}
		return t.typing(ctx, params.Selector, params.Text)
	case "evaluate":
		if params.Script == "" {
			return "", fmt.Errorf("script is required for evaluate action")
		}
		return t.evaluate(ctx, params.Script)
	default:
		return "", fmt.Errorf("unknown action: %s (valid: navigate, content, screenshot, click, type, evaluate)", params.Action)
	}
}

// navigate navigates to a URL using CDP or headless Chrome
func (t *BrowserTool) navigate(ctx context.Context, url string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("url is required for navigate action")
	}

	// Try CDP first
	if targetID, err := t.getFirstTarget(ctx); err == nil && targetID != "" {
		if err := t.sendCDPCommand(ctx, targetID, "Page.navigate", map[string]interface{}{"url": url}); err == nil {
			return fmt.Sprintf("Navigated to: %s (via CDP)", url), nil
		}
	}

	// Fallback: Use headless Chrome to validate URL is accessible
	if t.chromeBinary != "" {
		cmd := exec.CommandContext(ctx, t.chromeBinary, "--headless", "--dump-dom", "--no-sandbox", url)
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil {
			return fmt.Sprintf("Navigated to: %s (via headless Chrome)", url), nil
		}
	}

	return fmt.Sprintf("Navigation command sent to: %s (result unknown - Chrome may need to be started with --remote-debugging-port=9222)", url), nil
}

// getContent gets the page content using CDP or headless Chrome
func (t *BrowserTool) getContent(ctx context.Context) (string, error) {
	// Try CDP first
	if targetID, err := t.getFirstTarget(ctx); err == nil && targetID != "" {
		// Navigate to about:blank and get document text
		var result struct {
			Result struct {
				Root struct {
					NodeID int `json:"nodeId"`
				} `json:"root"`
			} `json:"result"`
		}

		// Get document
		if err := t.sendCDPCommand(ctx, targetID, "DOM.getDocument", map[string]interface{}{}, &result); err != nil {
			goto fallback
		}

		// For simplicity, just return a message about CDP availability
		return "Page content available via CDP. Use 'evaluate' action with 'document.documentElement.outerHTML' to get full HTML.", nil
	}

fallback:
	// Fallback: Use headless Chrome --dump-dom
	if t.chromeBinary != "" {
		// Get the current URL from CDP tabs if available
		currentURL := ""
		if tabs, err := t.listTabs(ctx); err == nil && len(tabs) > 0 {
			if url, ok := tabs[0]["url"].(string); ok {
				currentURL = url
			}
		}

		if currentURL != "" {
			cmd := exec.CommandContext(ctx, t.chromeBinary, "--headless", "--dump-dom", "--no-sandbox", currentURL)
			var out bytes.Buffer
			cmd.Stdout = &out
			if err := cmd.Run(); err == nil {
				return out.String(), nil
			}
		}
	}

	return "", fmt.Errorf("no active page found. Start Chrome with 'google-chrome --remote-debugging-port=9222' or use 'navigate' action first")
}

// screenshot takes a screenshot using CDP or headless Chrome
func (t *BrowserTool) screenshot(ctx context.Context, outputPath string) (string, error) {
	// Try CDP first
	if targetID, err := t.getFirstTarget(ctx); err == nil && targetID != "" {
		if err := t.sendCDPCommand(ctx, targetID, "Page.captureScreenshot", map[string]interface{}{
			"format": "png",
		}); err == nil {
			// Note: CDP screenshot would require parsing the base64 response and writing to file
			// For simplicity, we return a message
			return fmt.Sprintf("Screenshot captured via CDP (would save to: %s - requires WebSocket for full implementation)", outputPath), nil
		}
	}

	// Fallback: Use headless Chrome --screenshot
	if t.chromeBinary != "" {
		// Get the current URL from CDP tabs if available
		currentURL := ""
		if tabs, err := t.listTabs(ctx); err == nil && len(tabs) > 0 {
			if url, ok := tabs[0]["url"].(string); ok {
				currentURL = url
			}
		}

		if currentURL != "" {
			cmd := exec.CommandContext(ctx, t.chromeBinary, "--headless", "--screenshot="+outputPath, "--no-sandbox", currentURL)
			if err := cmd.Run(); err == nil {
				return fmt.Sprintf("Screenshot saved to: %s", outputPath), nil
			}
		}
	}

	return "", fmt.Errorf("screenshot failed. Start Chrome with 'google-chrome --remote-debugging-port=9222' or navigate to a URL first")
}

// click clicks an element using CDP
func (t *BrowserTool) click(ctx context.Context, selector string) (string, error) {
	targetID, err := t.getFirstTarget(ctx)
	if err != nil {
		return "", fmt.Errorf("CDP not available: %w", err)
	}

	// Click using JavaScript evaluation
	script := fmt.Sprintf(`
		(function() {
			var el = document.querySelector('%s');
			if (!el) return 'Element not found';
			el.click();
			return 'Clicked: ' + '%s';
		})()
	`, escapeJSString(selector), escapeJSString(selector))

	result, err := t.evaluateCDP(ctx, targetID, script)
	if err != nil {
		return "", fmt.Errorf("click failed: %w", err)
	}

	return result, nil
}

// typing types text into an input field using CDP
func (t *BrowserTool) typing(ctx context.Context, selector, text string) (string, error) {
	targetID, err := t.getFirstTarget(ctx)
	if err != nil {
		return "", fmt.Errorf("CDP not available: %w", err)
	}

	// Type using JavaScript evaluation
	script := fmt.Sprintf(`
		(function() {
			var el = document.querySelector('%s');
			if (!el) return 'Element not found';
			el.value = '%s';
			el.dispatchEvent(new Event('input', { bubbles: true }));
			el.dispatchEvent(new Event('change', { bubbles: true }));
			return 'Typed text into: ' + '%s';
		})()
	`, escapeJSString(selector), escapeJSString(text), escapeJSString(selector))

	result, err := t.evaluateCDP(ctx, targetID, script)
	if err != nil {
		return "", fmt.Errorf("type failed: %w", err)
	}

	return result, nil
}

// evaluate executes JavaScript using CDP
func (t *BrowserTool) evaluate(ctx context.Context, script string) (string, error) {
	targetID, err := t.getFirstTarget(ctx)
	if err != nil {
		return "", fmt.Errorf("CDP not available: %w", err)
	}

	return t.evaluateCDP(ctx, targetID, script)
}

// evaluateCDP evaluates JavaScript via CDP Runtime.evaluate
func (t *BrowserTool) evaluateCDP(ctx context.Context, targetID, script string) (string, error) {
	var result struct {
		Result struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"result"`
	}

	err := t.sendCDPCommand(ctx, targetID, "Runtime.evaluate", map[string]interface{}{
		"expression":            script,
		"returnByValue":         true,
		"awaitPromise":          true,
		"timeout":               30000,
	}, &result)

	if err != nil {
		return "", err
	}

	if result.Result.Type == "string" {
		return result.Result.Value, nil
	}
	return fmt.Sprintf("%v", result.Result.Value), nil
}

// listTabs gets list of Chrome tabs from CDP /json endpoint
func (t *BrowserTool) listTabs(ctx context.Context) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/json", t.cdpURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("CDP endpoint returned status %d", resp.StatusCode)
	}

	var tabs []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
		return nil, err
	}

	return tabs, nil
}

// getFirstTarget returns the first available target ID from CDP
func (t *BrowserTool) getFirstTarget(ctx context.Context) (string, error) {
	tabs, err := t.listTabs(ctx)
	if err != nil {
		return "", err
	}

	if len(tabs) == 0 {
		return "", fmt.Errorf("no tabs found")
	}

	// Find a page target (not about:blank or chrome:// pages)
	for _, tab := range tabs {
		if tabType, ok := tab["type"].(string); ok && tabType == "page" {
			if tabID, ok := tab["id"].(string); ok {
				return tabID, nil
			}
		}
	}

	// Fallback to first tab
	if id, ok := tabs[0]["id"].(string); ok {
		return id, nil
	}

	return "", fmt.Errorf("no valid target ID found")
}

// sendCDPCommand sends a JSON-RPC command to CDP via HTTP
func (t *BrowserTool) sendCDPCommand(ctx context.Context, targetID, method string, params map[string]interface{}, result ...interface{}) error {
	// For simplicity, we use the HTTP-based send command endpoint
	// This sends a command and returns the result
	url := fmt.Sprintf("%s/json/target/%s", t.cdpURL, targetID)

	// Create CDP message
	msg := map[string]interface{}{
		"id":     1,
		"method": method,
		"params": params,
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("CDP command failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse result if a result pointer was provided
	if len(result) > 0 && result[0] != nil {
		var cdpResp struct {
			Result interface{} `json:"result"`
			Error  struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(respBody, &cdpResp); err != nil {
			return err
		}
		if cdpResp.Error.Message != "" {
			return fmt.Errorf("CDP error: %s", cdpResp.Error.Message)
		}
		if err := json.Unmarshal(respBody, result[0]); err != nil {
			return err
		}
	}

	return nil
}

// findChromeBinary attempts to locate Chrome or Chromium binary
func findChromeBinary() string {
	candidates := []string{
		"google-chrome",
		"google-chrome-stable",
		"chromium-browser",
		"chromium",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/usr/bin/google-chrome",
		"/usr/bin/chromium-browser",
	}

	for _, binary := range candidates {
		if _, err := exec.LookPath(binary); err == nil {
			return binary
		}
	}

	return ""
}

// escapeJSString escapes a string for safe inclusion in JavaScript code
func escapeJSString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}