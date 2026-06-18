package llm

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// TestAnthropicLive verifies the AnthropicClient end-to-end against a real
// Anthropic-compatible endpoint (z.ai). Skipped unless ZAI_TOKEN is set.
func TestAnthropicLive(t *testing.T) {
	token := os.Getenv("ZAI_TOKEN")
	if token == "" {
		t.Skip("set ZAI_TOKEN to run the live Anthropic client test")
	}
	p, err := NewProvider("zai", kyoci.ProviderConfig{
		BaseURL:      "https://api.z.ai/api/anthropic",
		APIKey:       token,
		DefaultModel: "glm-4.7",
		MaxRetries:   1,
		Timeout:      60 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// 1. Plain text.
	r, err := p.Complete(ctx, kyoci.CompletionRequest{
		Model: "glm-4.7", MaxTokens: 20,
		Messages: []kyoci.Message{{Role: kyoci.RoleUser, Content: "Reply with exactly: OK"}},
	})
	if err != nil {
		t.Fatalf("text Complete failed: %v", err)
	}
	fmt.Printf("[live] TEXT response: %q (finish=%s usage=%+v)\n", r.Content, r.FinishReason, r.Usage)

	// 2. Tool call (forced).
	r2, err := p.Complete(ctx, kyoci.CompletionRequest{
		Model: "glm-4.7", MaxTokens: 200, ToolChoice: "required",
		Messages: []kyoci.Message{{Role: kyoci.RoleUser, Content: "What is 7 * 6? Use the calc tool."}},
		Tools: []kyoci.ToolDefinition{{
			Name: "calc", Description: "Evaluate an arithmetic expression",
			Parameters: []kyoci.ToolParameter{{Name: "expr", Type: "string", Description: "e.g. 7*6", Required: true}},
		}},
	})
	if err != nil {
		t.Fatalf("tool Complete failed: %v", err)
	}
	fmt.Printf("[live] TOOL calls: %+v (finish=%s)\n", r2.ToolCalls, r2.FinishReason)
	if len(r2.ToolCalls) == 0 {
		t.Errorf("expected a tool call, got none (content=%q)", r2.Content)
	}

	// 3. Round-trip a tool result back (verifies tool_result message translation).
	if len(r2.ToolCalls) > 0 {
		tc := r2.ToolCalls[0]
		r3, err := p.Complete(ctx, kyoci.CompletionRequest{
			Model: "glm-4.7", MaxTokens: 50,
			Messages: []kyoci.Message{
				{Role: kyoci.RoleUser, Content: "What is 7 * 6? Use the calc tool."},
				{Role: kyoci.RoleAssistant, Content: r2.Content, ToolCalls: []kyoci.ToolCall{tc}},
				{Role: kyoci.RoleTool, ToolCallID: tc.ID, Content: "42"},
			},
		})
		if err != nil {
			t.Fatalf("tool-result Complete failed: %v", err)
		}
		fmt.Printf("[live] AFTER tool_result: %q\n", r3.Content)
	}
}
