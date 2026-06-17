package agent

import (
	"testing"
)

// =============================================================================
// sanitizeContent Tests
// =============================================================================

func TestSanitizeContent_EmptyString(t *testing.T) {
	result := sanitizeContent("")
	if result != "" {
		t.Errorf("Expected empty string, got: %q", result)
	}
}

func TestSanitizeContent_NoToolCalls(t *testing.T) {
	input := "This is a clean response with no tool call artifacts."
	result := sanitizeContent(input)
	if result != input {
		t.Errorf("Expected unchanged content, got: %q", result)
	}
}

func TestSanitizeContent_SingleLineToolCall(t *testing.T) {
	input := "Some text\n[Tool Call: file({\"operation\": \"read\"})]\nMore text"
	result := sanitizeContent(input)

	if containsStr(result, "[Tool Call:") {
		t.Errorf("Expected [Tool Call:] to be stripped, got: %q", result)
	}
	if !containsStr(result, "Some text") {
		t.Errorf("Expected 'Some text' to be preserved, got: %q", result)
	}
	if !containsStr(result, "More text") {
		t.Errorf("Expected 'More text' to be preserved, got: %q", result)
	}
}

func TestSanitizeContent_MultiLineToolCall(t *testing.T) {
	input := `Before tool call
[Tool Call: file({
  "operation": "write",
  "path": "/tmp/test.txt",
  "content": "hello"
})]
After tool call`

	result := sanitizeContent(input)

	if containsStr(result, "[Tool Call:") {
		t.Errorf("Expected [Tool Call:] block to be stripped, got: %q", result)
	}
	if !containsStr(result, "Before tool call") {
		t.Errorf("Expected 'Before tool call' to be preserved, got: %q", result)
	}
	if !containsStr(result, "After tool call") {
		t.Errorf("Expected 'After tool call' to be preserved, got: %q", result)
	}
}

func TestSanitizeContent_MultipleToolCalls(t *testing.T) {
	input := `Start
[Tool Call: terminal({"command": "ls"})]
Middle
[Tool Call: file({"operation": "read", "path": "/etc/hosts"})]
End`

	result := sanitizeContent(input)

	if containsStr(result, "[Tool Call:") {
		t.Errorf("Expected all [Tool Call:] to be stripped, got: %q", result)
	}
	if !containsStr(result, "Start") {
		t.Errorf("Expected 'Start' to be preserved")
	}
	if !containsStr(result, "Middle") {
		t.Errorf("Expected 'Middle' to be preserved")
	}
	if !containsStr(result, "End") {
		t.Errorf("Expected 'End' to be preserved")
	}
}

func TestSanitizeContent_PreservesNormalText(t *testing.T) {
	input := `Here's the analysis:

The disk has 196GB used out of 228GB total (86% full).
Only 9.9GB free — you should clean up temporary files.

I recommend:
1. Remove old Docker images
2. Clear npm cache
3. Delete old log files`

	result := sanitizeContent(input)

	// Content should be unchanged (no tool calls)
	if result != input {
		t.Errorf("Expected content to be unchanged, got: %q", result)
	}
}

func TestSanitizeContent_OnlyToolCall(t *testing.T) {
	input := `[Tool Call: terminal({"command": "echo hello"})]`
	result := sanitizeContent(input)

	// The tool call should be stripped, result should be empty or minimal
	if containsStr(result, "[Tool Call:") {
		t.Errorf("Expected tool call to be stripped, got: %q", result)
	}
}

// containsStr is a simple substring check for test assertions.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstr(s, substr))
}

// containsSubstr helper.
func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
