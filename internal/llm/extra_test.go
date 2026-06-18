package llm

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// ==============================================================================
// New LLM Client Function Tests
// ==============================================================================

func TestStripToMinimal(t *testing.T) {
	tests := []struct {
		name     string
		messages []kyoci.Message
		expected []kyoci.Message
	}{
		{
			name: "Keep only system prompt and last user message",
			messages: []kyoci.Message{
				{Role: kyoci.RoleSystem, Content: "You are a helpful assistant."},
				{Role: kyoci.RoleUser, Content: "Hello"},
				{Role: kyoci.RoleAssistant, Content: "Hi there!"},
				{Role: kyoci.RoleUser, Content: "How are you?"},
			},
			expected: []kyoci.Message{
				{Role: kyoci.RoleSystem, Content: "You are a helpful assistant."},
				{Role: kyoci.RoleUser, Content: "How are you?"},
			},
		},
		{
			name: "No system prompt - keep only last user message",
			messages: []kyoci.Message{
				{Role: kyoci.RoleUser, Content: "Hello"},
				{Role: kyoci.RoleAssistant, Content: "Hi there!"},
				{Role: kyoci.RoleUser, Content: "How are you?"},
			},
			expected: []kyoci.Message{
				{Role: kyoci.RoleUser, Content: "How are you?"},
			},
		},
		{
			name: "Only user messages - keep all (len <= 2)",
			messages: []kyoci.Message{
				{Role: kyoci.RoleUser, Content: "First message"},
				{Role: kyoci.RoleUser, Content: "Second message"},
			},
			expected: []kyoci.Message{
				{Role: kyoci.RoleUser, Content: "First message"},
				{Role: kyoci.RoleUser, Content: "Second message"},
			},
		},
		{
			name: "Single message - unchanged",
			messages: []kyoci.Message{
				{Role: kyoci.RoleUser, Content: "Hello"},
			},
			expected: []kyoci.Message{
				{Role: kyoci.RoleUser, Content: "Hello"},
			},
		},
		{
			name: "Two messages - unchanged",
			messages: []kyoci.Message{
				{Role: kyoci.RoleSystem, Content: "System prompt"},
				{Role: kyoci.RoleUser, Content: "Hello"},
			},
			expected: []kyoci.Message{
				{Role: kyoci.RoleSystem, Content: "System prompt"},
				{Role: kyoci.RoleUser, Content: "Hello"},
			},
		},
		{
			name: "Complex conversation with tool calls",
			messages: []kyoci.Message{
				{Role: kyoci.RoleSystem, Content: "You are a helpful assistant."},
				{Role: kyoci.RoleUser, Content: "Check the weather"},
				{Role: kyoci.RoleAssistant, Content: "", ToolCalls: []kyoci.ToolCall{{Name: "get_weather", Arguments: "{}"}}},
				{Role: kyoci.RoleTool, Content: "It's sunny"},
				{Role: kyoci.RoleUser, Content: "What about tomorrow?"},
			},
			expected: []kyoci.Message{
				{Role: kyoci.RoleSystem, Content: "You are a helpful assistant."},
				{Role: kyoci.RoleUser, Content: "What about tomorrow?"},
			},
		},
		{
			name: "Empty messages - return as is",
			messages: []kyoci.Message{},
			expected: []kyoci.Message{},
		},
		{
			name: "Last message is assistant - keep system + last user",
			messages: []kyoci.Message{
				{Role: kyoci.RoleSystem, Content: "System prompt"},
				{Role: kyoci.RoleUser, Content: "Hello"},
				{Role: kyoci.RoleAssistant, Content: "Hi there!"},
			},
			expected: []kyoci.Message{
				{Role: kyoci.RoleSystem, Content: "System prompt"},
				{Role: kyoci.RoleUser, Content: "Hello"},
			},
		},
		{
			name: "No user messages - return system only (found system, no user)",
			messages: []kyoci.Message{
				{Role: kyoci.RoleSystem, Content: "System prompt"},
				{Role: kyoci.RoleAssistant, Content: "Hello"},
				{Role: kyoci.RoleAssistant, Content: "Hi there!"},
			},
			expected: []kyoci.Message{
				{Role: kyoci.RoleSystem, Content: "System prompt"},
			},
		},
		{
			name: "Three user messages - keep system + last user",
			messages: []kyoci.Message{
				{Role: kyoci.RoleSystem, Content: "System prompt"},
				{Role: kyoci.RoleUser, Content: "First"},
				{Role: kyoci.RoleAssistant, Content: "Response 1"},
				{Role: kyoci.RoleUser, Content: "Second"},
				{Role: kyoci.RoleAssistant, Content: "Response 2"},
				{Role: kyoci.RoleUser, Content: "Third"},
			},
			expected: []kyoci.Message{
				{Role: kyoci.RoleSystem, Content: "System prompt"},
				{Role: kyoci.RoleUser, Content: "Third"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := kyoci.ProviderConfig{
				BaseURL:      "http://localhost:11434/v1",
				DefaultModel: "llama2",
			}
			client, err := newTestClient("ollama", config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			result := client.stripToMinimal(tt.messages)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d messages, got %d", len(tt.expected), len(result))
			}

			for i, msg := range result {
				if i >= len(tt.expected) {
					break
				}
				if msg.Role != tt.expected[i].Role {
					t.Errorf("Message %d: expected role %s, got %s", i, tt.expected[i].Role, msg.Role)
				}
				if msg.Content != tt.expected[i].Content {
					t.Errorf("Message %d: expected content %q, got %q", i, tt.expected[i].Content, msg.Content)
				}
			}
		})
	}
}

func TestXMLCharacterEscaping(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "Escape ampersand",
			input:    "A & B",
			maxLen:   100,
			expected: "A &amp; B",
		},
		{
			name:     "Escape less than",
			input:    "5 < 10",
			maxLen:   100,
			expected: "5 &lt; 10",
		},
		{
			name:     "Escape greater than",
			input:    "10 > 5",
			maxLen:   100,
			expected: "10 &gt; 5",
		},
		{
			name:     "Escape all three special chars",
			input:    "<div>Hello & Welcome</div>",
			maxLen:   100,
			expected: "&lt;div&gt;Hello &amp; Welcome&lt;/div&gt;",
		},
		{
			name:     "Already escaped ampersand",
			input:    "A &amp; B",
			maxLen:   100,
			expected: "A &amp;amp; B",
		},
		{
			name:     "Multiple special chars",
			input:    "if (a < b && c > d) return true;",
			maxLen:   100,
			expected: "if (a &lt; b &amp;&amp; c &gt; d) return true;",
		},
		{
			name:     "HTML code snippet",
			input:    `<html><body><h1>Hello</h1></body></html>`,
			maxLen:   100,
			expected: `&lt;html&gt;&lt;body&gt;&lt;h1&gt;Hello&lt;/h1&gt;&lt;/body&gt;&lt;/html&gt;`,
		},
		{
			name:     "Truncate long text",
			input:    "This is a very long string that should be truncated to fit within the maximum length limit",
			maxLen:   30,
			expected: "This is a very long string tha\n... [truncated for context limit]",
		},
		{
			name:     "Truncate at exact limit",
			input:    "123456789012345678901234567890",
			maxLen:   30,
			expected: "123456789012345678901234567890",
		},
		{
			name:     "Truncate by one character",
			input:    "1234567890123456789012345678901",
			maxLen:   30,
			expected: "123456789012345678901234567890\n... [truncated for context limit]",
		},
		{
			name:     "Empty string",
			input:    "",
			maxLen:   100,
			expected: "",
		},
		{
			name:     "String with no special chars",
			input:    "Hello world",
			maxLen:   100,
			expected: "Hello world",
		},
		{
			name:     "Complex JSON with HTML inside",
			input:    `{"html": "<div>Text & more</div>"}`,
			maxLen:   100,
			expected: `{"html": "&lt;div&gt;Text &amp; more&lt;/div&gt;"}`,
		},
		{
			name:     "JavaScript code",
			input:    "function test() { return x < y && z > w; }",
			maxLen:   100,
			expected: "function test() { return x &lt; y &amp;&amp; z &gt; w; }",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeForOllama(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	config := kyoci.ProviderConfig{
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
		DefaultModel: "gpt-4",
	}
	client, err := newTestClient("openai", config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	tests := []struct {
		name     string
		header   string
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{
			name:     "Empty header returns 0",
			header:   "",
			minDelay: 0,
			maxDelay: 0,
		},
		{
			name:     "Parse seconds as integer",
			header:   "5",
			minDelay: 5 * time.Second,
			maxDelay: 5 * time.Second,
		},
		{
			name:     "Parse seconds as large integer",
			header:   "60",
			minDelay: 60 * time.Second,
			maxDelay: 60 * time.Second,
		},
		{
			name:     "Parse seconds as zero",
			header:   "0",
			minDelay: 0,
			maxDelay: 0,
		},
		{
			name:     "Parse HTTP date format",
			header:   time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat),
			minDelay: 29 * time.Second,
			maxDelay: 31 * time.Second,
		},
		{
			name:     "Parse HTTP date far in the future",
			header:   time.Now().Add(5 * time.Minute).UTC().Format(http.TimeFormat),
			minDelay: 4*time.Minute + 59*time.Second,
			maxDelay: 5*time.Minute + 1*time.Second,
		},
		{
			name:     "Invalid format returns 0",
			header:   "invalid",
			minDelay: 0,
			maxDelay: 0,
		},
		{
			name:     "Negative seconds return negative duration",
			header:   "-5",
			minDelay: -5 * time.Second,
			maxDelay: -5 * time.Second,
		},
		{
			name:     "Float seconds not parsed",
			header:   "5.5",
			minDelay: 0,
			maxDelay: 0,
		},
		{
			name:     "Whitespace around seconds",
			header:   "  10  ",
			minDelay: 0,
			maxDelay: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.parseRetryAfter(tt.header)

			if tt.minDelay == 0 && tt.maxDelay == 0 {
				if result != 0 {
					t.Errorf("Expected 0 duration, got %v", result)
				}
			} else if result < tt.minDelay || result > tt.maxDelay {
				t.Errorf("Expected duration between %v and %v, got %v", tt.minDelay, tt.maxDelay, result)
			}
		})
	}
}

func TestParseRetryAfterEdgeCases(t *testing.T) {
	config := kyoci.ProviderConfig{
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
		DefaultModel: "gpt-4",
	}
	client, err := newTestClient("openai", config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	t.Run("Past HTTP date returns 0", func(t *testing.T) {
		pastTime := time.Now().Add(-5 * time.Minute).UTC().Format(http.TimeFormat)
		result := client.parseRetryAfter(pastTime)
		// A past date should return 0 or negative, but we expect it to be handled gracefully
		if result > 0 {
			t.Errorf("Expected non-positive duration for past date, got %v", result)
		}
	})
}

// ==============================================================================
// Model-agnostic response normalizer tests
// (Gemma emits answer in `reasoning`; qwen emits bare-JSON tool calls in content)
// ==============================================================================

func TestNormalizeContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		reasoning string
		expected string
	}{
		{
			name:      "content non-empty -> returned unchanged",
			content:   "The answer is 42",
			reasoning: "some chain of thought",
			expected:  "The answer is 42",
		},
		{
			name:      "content empty, reasoning non-empty -> fall back to reasoning (Gemma)",
			content:   "",
			reasoning: "Step 1: list files\nStep 2: read timezone",
			expected:  "Step 1: list files\nStep 2: read timezone",
		},
		{
			name:      "both empty -> empty string",
			content:   "",
			reasoning: "",
			expected:  "",
		},
		{
			name:      "whitespace-only content, reasoning non-empty -> fall back to reasoning",
			content:   "   \n\t ",
			reasoning: "actual answer",
			expected:  "actual answer",
		},
		{
			name:      "content present, reasoning empty -> content",
			content:   "hello",
			reasoning: "",
			expected:  "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeContent(tt.content, tt.reasoning)
			if got != tt.expected {
				t.Errorf("normalizeContent(%q, %q) = %q, want %q",
					tt.content, tt.reasoning, got, tt.expected)
			}
		})
	}
}

func TestExtractBareJSONToolCalls(t *testing.T) {
	// qwen emits: content = `{"name":"file","arguments":{"operation":"list","path":"/tmp"}}`
	// or wrapped in markdown fences, or embedded in prose.
	listArgs := map[string]interface{}{"operation": "list", "path": "/tmp"}
	listArgsJSON, _ := json.Marshal(listArgs)

	readArgs := map[string]interface{}{"operation": "read", "path": "/etc/timezone"}
	readArgsJSON, _ := json.Marshal(readArgs)

	tests := []struct {
		name           string
		content        string
		wantRemaining  string
		wantCalls      []wantToolCall
	}{
		{
			name:          "clean bare JSON object with name+arguments",
			content:       `{"name":"file","arguments":` + string(listArgsJSON) + `}`,
			wantRemaining: "",
			wantCalls: []wantToolCall{
				{Name: "file", ArgsContains: []string{`"operation":"list"`, `"/tmp"`}},
			},
		},
		{
			name:    "markdown-fenced JSON object",
			content: "```json\n" + `{"name":"file","arguments":` + string(listArgsJSON) + `}` + "\n```",
			wantCalls: []wantToolCall{
				{Name: "file", ArgsContains: []string{`"operation":"list"`}},
			},
		},
		{
			name:    "bare object wrapped in <tool_call> tags (some gemma/qwen variants)",
			content: `<tool_call>{"name":"file","arguments":` + string(readArgsJSON) + `}</tool_call>`,
			wantCalls: []wantToolCall{
				{Name: "file", ArgsContains: []string{`"operation":"read"`, `"/etc/timezone"`}},
			},
		},
		{
			name:          "plain prose with no JSON -> returned unchanged, no calls",
			content:       "I did not find any timezone settings.",
			wantRemaining: "I did not find any timezone settings.",
			wantCalls:     nil,
		},
		{
			name:          "JSON object without name field -> ignored, content kept",
			content:       `{"foo":"bar","baz":42}`,
			wantRemaining: `{"foo":"bar","baz":42}`,
			wantCalls:     nil,
		},
		{
			name:    "prose surrounding a bare JSON tool call -> JSON extracted, prose kept",
			content: "Let me list files.\n" + `{"name":"file","arguments":` + string(listArgsJSON) + `}` + "\nDone.",
			wantCalls: []wantToolCall{
				{Name: "file"},
			},
		},
		{
			name:    "multiple bare JSON tool calls in sequence",
			content: `{"name":"file","arguments":` + string(listArgsJSON) + `}` + "\n" + `{"name":"file","arguments":` + string(readArgsJSON) + `}`,
			wantCalls: []wantToolCall{
				{Name: "file"},
				{Name: "file"},
			},
		},
		{
			name:          "empty content -> empty, no calls",
			content:       "",
			wantRemaining: "",
			wantCalls:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remaining, calls := extractBareJSONToolCalls(tt.content)

			if tt.wantRemaining != "" && remaining != tt.wantRemaining {
				t.Errorf("remaining = %q, want %q", remaining, tt.wantRemaining)
			}
			if len(calls) != len(tt.wantCalls) {
				t.Fatalf("got %d calls, want %d (remaining=%q calls=%v)",
					len(calls), len(tt.wantCalls), remaining, calls)
			}
			for i, want := range tt.wantCalls {
				if i >= len(calls) {
					break
				}
				if calls[i].Name != want.Name {
					t.Errorf("call %d: name = %q, want %q", i, calls[i].Name, want.Name)
				}
				for _, wantArg := range want.ArgsContains {
					if !strings.Contains(calls[i].Arguments, wantArg) {
						t.Errorf("call %d (%s): arguments %q missing substring %q",
							i, calls[i].Name, calls[i].Arguments, wantArg)
					}
				}
			}
		})
	}
}

// wantToolCall is a lightweight expectation for tool call assertions.
type wantToolCall struct {
	Name         string
	ArgsContains []string
}

// TestExtractBareJSONToolCalls_PreservesValidNativeCalls verifies the extraction
// is only invoked when structured tool_calls are absent. The caller (Complete)
// decides; this test just confirms extractBareJSONToolCalls is safe to call
// on arbitrary content and never panics.
func TestExtractBareJSONToolCalls_NoPanicOnWeirdInput(t *testing.T) {
	weird := []string{
		"{{{{{",
		"}{}{}{",
		`{"name":`,
		`{"name":"x","arguments":`,
		strings.Repeat("{", 10000),
		"",
	}
	for _, w := range weird {
		_, _ = extractBareJSONToolCalls(w) // must not panic
	}
}