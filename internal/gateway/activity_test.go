package gateway

import (
	"strings"
	"testing"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// formatCompactFooter Tests
// =============================================================================

func TestFormatCompactFooter_NilResult(t *testing.T) {
	result := formatCompactFooter(nil, 5*time.Second)
	if result != "" {
		t.Errorf("Expected empty string for nil result, got: %s", result)
	}
}

func TestFormatCompactFooter_BasicFields(t *testing.T) {
	result := &ActivityResult{
		Content:    "Done",
		Role:       "developer",
		ToolCalls:  3,
		Iterations: 5,
		TokensUsed: 7200,
		ToolLog: []kyoci.ToolCallEntry{
			{Tool: "terminal", Success: true, DurationMs: 100},
			{Tool: "file", Success: true, DurationMs: 50},
		},
	}

	footer := formatCompactFooter(result, 8*time.Second+100*time.Millisecond)

	// Should contain role emoji and name
	if !strings.Contains(footer, "👨‍💻") {
		t.Errorf("Expected developer emoji in footer, got: %s", footer)
	}
	if !strings.Contains(footer, "developer") {
		t.Errorf("Expected role name 'developer' in footer, got: %s", footer)
	}
	// Should contain duration
	if !strings.Contains(footer, "8.1s") {
		t.Errorf("Expected duration '8.1s' in footer, got: %s", footer)
	}
	// Should contain iteration count
	if !strings.Contains(footer, "5 iters") {
		t.Errorf("Expected '5 iters' in footer, got: %s", footer)
	}
	// Should contain tokens (formatted as k)
	if !strings.Contains(footer, "7.2k") {
		t.Errorf("Expected '7.2k' tokens in footer, got: %s", footer)
	}
	// Should contain tool count (all passed)
	if !strings.Contains(footer, "2 tools ✅") {
		t.Errorf("Expected '2 tools ✅' in footer, got: %s", footer)
	}
}

func TestFormatCompactFooter_WithFailures(t *testing.T) {
	result := &ActivityResult{
		Role:       "frontend",
		Iterations: 2,
		TokensUsed: 500,
		ToolLog: []kyoci.ToolCallEntry{
			{Tool: "terminal", Success: true, DurationMs: 100},
			{Tool: "file", Success: false, DurationMs: 50},
		},
	}

	footer := formatCompactFooter(result, 3*time.Second)

	// Should show both pass and fail counts
	if !strings.Contains(footer, "2 tools (1✅ 1❌)") {
		t.Errorf("Expected '2 tools (1✅ 1❌)' in footer, got: %s", footer)
	}
	// Should contain frontend emoji
	if !strings.Contains(footer, "🎨") {
		t.Errorf("Expected frontend emoji in footer, got: %s", footer)
	}
}

func TestFormatCompactFooter_ZeroTokens(t *testing.T) {
	result := &ActivityResult{
		Role:       "sre",
		Iterations: 1,
		TokensUsed: 0,
	}

	footer := formatCompactFooter(result, 500*time.Millisecond)

	// Should NOT contain token info when 0
	if strings.Contains(footer, "tok") {
		t.Errorf("Should not show token info for 0 tokens, got: %s", footer)
	}
	// Should contain millisecond duration
	if !strings.Contains(footer, "500ms") {
		t.Errorf("Expected '500ms' in footer, got: %s", footer)
	}
}

func TestFormatCompactFooter_NoTools(t *testing.T) {
	result := &ActivityResult{
		Role:       "qa",
		Iterations: 1,
		TokensUsed: 100,
	}

	footer := formatCompactFooter(result, 2*time.Second)

	// Should NOT contain tool info when empty
	if strings.Contains(footer, "tools") {
		t.Errorf("Should not show tool info for empty tool log, got: %s", footer)
	}
}

// =============================================================================
// formatDuration Tests
// =============================================================================

func TestFormatDuration_Milliseconds(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"zero", 0, "0ms"},
		{"500ms", 500 * time.Millisecond, "500ms"},
		{"999ms", 999 * time.Millisecond, "999ms"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestFormatDuration_Seconds(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"1 second", 1 * time.Second, "1.0s"},
		{"1.5 seconds", 1500 * time.Millisecond, "1.5s"},
		{"10 seconds", 10 * time.Second, "10.0s"},
		{"65.3 seconds", 65*time.Second + 300*time.Millisecond, "65.3s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// =============================================================================
// roleIconForRole Tests
// =============================================================================

func TestRoleIconForRole(t *testing.T) {
	tests := []struct {
		role     string
		expected string
	}{
		{"developer", "👨‍💻"},
		{"sre", "🛡️"},
		{"qa", "🧪"},
		{"pm", "📋"},
		{"frontend", "🎨"},
		{"custom", "🤖"},
		{"unknown_role", "🤖"},   // fallback
		{"", "🤖"},                // empty string
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			result := roleIconForRole(tt.role)
			if result != tt.expected {
				t.Errorf("roleIconForRole(%q) = %q, want %q", tt.role, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// toolIcon Tests
// =============================================================================

func TestToolIcon(t *testing.T) {
	tests := []struct {
		tool     string
		expected string
	}{
		{"terminal", "⚡"},
		{"file", "📄"},
		{"browser", "🌐"},
		{"docs", "📚"},
		{"http_client", "🔗"},
		{"web_search", "🔍"},
		{"calculator", "🧮"},
		{"todo", "📝"},
		{"skill", "💡"},
		{"process", "⚙️"},
		{"memory_recall", "🧠"},
		{"remember", "💾"},
		{"delegation", "🤖"},
		{"security_scan", "🛡️"},
		{"unknown_tool", "🔧"},   // fallback
		{"", "🔧"},               // empty string
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			result := toolIcon(tt.tool)
			if result != tt.expected {
				t.Errorf("toolIcon(%q) = %q, want %q", tt.tool, result, tt.expected)
			}
		})
	}
}
