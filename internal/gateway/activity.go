package gateway

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// Activity Result — rich telemetry from each task execution
// =============================================================================

// ActivityResult holds full telemetry from a task execution.
type ActivityResult struct {
	Content    string                  // Final response text
	Role       string                  // Role that handled the task (auto-detected or explicit)
	ToolCalls  int                     // Total tool calls made
	ToolLog    []kyoci.ToolCallEntry   // Per-tool-call details
	Iterations int                     // ReAct iterations
	TokensUsed int                     // Total tokens consumed
}

// FormatFooter returns a compact multi-line activity summary for Telegram.
// Uses Telegram HTML for rich formatting.
func (a *ActivityResult) FormatFooter(duration time.Duration) string {
	if a == nil {
		return ""
	}

	// Role emoji map
	roleEmojis := map[string]string{
		"developer": "👨‍💻",
		"sre":       "🛡️",
		"qa":        "🧪",
		"pm":        "📋",
		"frontend":  "🎨",
		"custom":    "🤖",
	}
	emoji := roleEmojis[a.Role]
	if emoji == "" {
		emoji = "🤖"
	}

	// Tool call summary with icons
	var toolParts []string
	for _, tc := range a.ToolLog {
		status := "✅"
		if !tc.Success {
			status = "❌"
		}
		icon := toolIcon(tc.Tool)
		toolParts = append(toolParts, fmt.Sprintf("%s %s%s · <code>%dms</code>", status, icon, tc.Tool, tc.DurationMs))
	}
	toolSummary := strings.Join(toolParts, "\n")
	if toolSummary == "" {
		toolSummary = "📋 no tools used"
	}

	// Duration
	durStr := formatDuration(duration)

	// Token count → human readable
	tokensStr := ""
	if a.TokensUsed > 0 {
		if a.TokensUsed >= 1000 {
			tokensStr = fmt.Sprintf("%.1fk", float64(a.TokensUsed)/1000)
		} else {
			tokensStr = fmt.Sprintf("%d", a.TokensUsed)
		}
	}

	// Build footer: role badge on left, stats on right, tools listed below
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s <b>%s</b>  ·  ⏱ <code>%s</code>  ·  🔁 %d iters", emoji, a.Role, durStr, a.Iterations))
	if tokensStr != "" {
		sb.WriteString(fmt.Sprintf("  ·  🪙 %s tok", tokensStr))
	}
	sb.WriteString("\n")
	sb.WriteString(toolSummary)

	return sb.String()
}

// formatCompactFooter returns a footer for the Telegram response.
// Shows each tool call with its argument detail (file path, command, etc.)
// Uses MARKDOWN syntax — sendMessageHTML will convert it.
func formatCompactFooter(result *ActivityResult, duration time.Duration) string {
	if result == nil {
		return ""
	}

	roleEmoji := roleIconForRole(result.Role)
	durStr := formatDuration(duration)

	tokensStr := ""
	if result.TokensUsed > 0 {
		if result.TokensUsed >= 1000 {
			tokensStr = fmt.Sprintf("%.1fk", float64(result.TokensUsed)/1000)
		} else {
			tokensStr = fmt.Sprintf("%d", result.TokensUsed)
		}
	}

	passed := 0
	failed := 0
	for _, tc := range result.ToolLog {
		if tc.Success {
			passed++
		} else {
			failed++
		}
	}

	// Build: "🎨 frontend · 8.1s · 🔁 3 iters · 🪙 7.2k tok · 4 tools ✅"
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s **%s** · `%s` · 🔁 %d iters",
		roleEmoji, result.Role, durStr, result.Iterations))
	if tokensStr != "" {
		sb.WriteString(fmt.Sprintf(" · 🪙 %s tok", tokensStr))
	}
	if len(result.ToolLog) > 0 {
		if failed > 0 {
			sb.WriteString(fmt.Sprintf(" · %d tools (%d✅ %d❌)", len(result.ToolLog), passed, failed))
		} else {
			sb.WriteString(fmt.Sprintf(" · %d tools ✅", len(result.ToolLog)))
		}
	}

	// Add per-tool detail lines showing what was actually done
	for _, tc := range result.ToolLog {
		detail := toolDetailFromArgs(tc.Tool, tc.Args)
		status := "✅"
		if !tc.Success {
			status = "❌"
		}
		if detail != "" {
			sb.WriteString(fmt.Sprintf("\n%s %s %s", status, toolIcon(tc.Tool), detail))
		} else {
			sb.WriteString(fmt.Sprintf("\n%s %s %s", status, toolIcon(tc.Tool), tc.Tool))
		}
	}

	return sb.String()
}

// toolDetailFromArgs extracts a short human-readable detail from tool arguments JSON.
func toolDetailFromArgs(toolName, argsJSON string) string {
	if argsJSON == "" {
		return ""
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}

	switch toolName {
	case "file":
		op, _ := args["operation"].(string)
		if op == "" {
			if _, hasContent := args["content"]; hasContent {
				op = "write"
			} else {
				op = "read"
			}
		}
		path, _ := args["path"].(string)
		if path != "" {
			return op + " " + shortPath(path)
		}
		return op
	case "terminal":
		cmd, _ := args["command"].(string)
		if cmd == "" {
			return ""
		}
		// Show first line only, truncated
		if idx := strings.IndexByte(cmd, '\n'); idx > 0 {
			cmd = cmd[:idx]
		}
		if len(cmd) > 60 {
			cmd = cmd[:60] + "…"
		}
		return cmd
	case "browser":
		url, _ := args["url"].(string)
		if url != "" {
			if len(url) > 50 {
				url = url[:50] + "…"
			}
			return "open " + url
		}
		action, _ := args["action"].(string)
		return action
	case "web_search":
		q, _ := args["query"].(string)
		if q != "" {
			return "search: " + q
		}
		return ""
	case "http_client":
		method, _ := args["method"].(string)
		url, _ := args["url"].(string)
		if url != "" {
			if len(url) > 50 {
				url = url[:50] + "…"
			}
			return method + " " + url
		}
		return ""
	case "calculator":
		expr, _ := args["expression"].(string)
		if expr != "" {
			return expr
		}
		return ""
	case "delegation":
		goal, _ := args["goal"].(string)
		if goal != "" {
			if len(goal) > 60 {
				goal = goal[:60] + "…"
			}
			return "delegate: " + goal
		}
		return ""
	default:
		return ""
	}
}

// formatDuration formats a duration compactly.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// =============================================================================
// Activity Tracker — global in-memory log of recent task activity
// =============================================================================

// ActivityEntry records one completed task for the activity log.
type ActivityEntry struct {
	Timestamp  time.Time
	Task       string           // Truncated task text
	Role       string           // Role used
	Success    bool             // Whether the task succeeded
	ToolCalls  int              // Number of tool calls
	Duration   time.Duration    // Total execution time
	TokensUsed int              // Tokens consumed
}

// ActivityTracker keeps a rolling log of recent task activity.
// Thread-safe. Used by /activity command in Telegram.
type ActivityTracker struct {
	mu      sync.RWMutex
	entries []ActivityEntry
	maxLen  int
}

// NewActivityTracker creates a new tracker with the given history length.
func NewActivityTracker(maxLen int) *ActivityTracker {
	if maxLen <= 0 {
		maxLen = 50
	}
	return &ActivityTracker{
		entries: make([]ActivityEntry, 0, maxLen),
		maxLen:  maxLen,
	}
}

// Record adds a new activity entry.
func (at *ActivityTracker) Record(entry ActivityEntry) {
	at.mu.Lock()
	defer at.mu.Unlock()

	at.entries = append(at.entries, entry)
	if len(at.entries) > at.maxLen {
		at.entries = at.entries[len(at.entries)-at.maxLen:]
	}
}

// Recent returns the last N activity entries (newest first).
func (at *ActivityTracker) Recent(n int) []ActivityEntry {
	at.mu.RLock()
	defer at.mu.RUnlock()

	if n <= 0 || n > len(at.entries) {
		n = len(at.entries)
	}

	// Return in reverse chronological order
	result := make([]ActivityEntry, n)
	for i := 0; i < n; i++ {
		result[i] = at.entries[len(at.entries)-1-i]
	}
	return result
}

// FormatRecent returns a human-readable summary of recent activity for Telegram.
func (at *ActivityTracker) FormatRecent(n int) string {
	entries := at.Recent(n)
	if len(entries) == 0 {
		return "No activity yet."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 Last %d activities:\n\n", len(entries)))

	for i, e := range entries {
		// Truncate task text
		task := e.Task
		if len(task) > 60 {
			task = task[:57] + "..."
		}

		status := "✅"
		if !e.Success {
			status = "❌"
		}

		roleEmojis := map[string]string{
			"developer": "👨‍💻", "sre": "🛡️", "qa": "🧪",
			"pm": "📋", "frontend": "🎨", "custom": "🤖",
		}
		emoji := roleEmojis[e.Role]
		if emoji == "" {
			emoji = "🤖"
		}

		sb.WriteString(fmt.Sprintf("%d. %s %s %s\n   %s | %d tools | %s\n\n",
			i+1, status, emoji, e.Role, task, e.ToolCalls, formatDuration(e.Duration)))
	}

	return sb.String()
}

// Stats returns aggregate statistics.
func (at *ActivityTracker) Stats() (total, successCount int, avgDuration time.Duration, totalTools int) {
	at.mu.RLock()
	defer at.mu.RUnlock()

	total = len(at.entries)
	if total == 0 {
		return
	}

	var totalDur time.Duration
	for _, e := range at.entries {
		if e.Success {
			successCount++
		}
		totalDur += e.Duration
		totalTools += e.ToolCalls
	}
	avgDuration = totalDur / time.Duration(total)
	return
}
