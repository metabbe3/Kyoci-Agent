package kyoci

import "testing"

// TestCompletionRequest_ToolChoiceField verifies the public field exists and
// round-trips. The orchestrator worker uses it to send `tool_choice: "required"`
// on iteration 0 (forcing the model to call a tool before answering), then
// switches to "" (auto) for subsequent iterations.
//
// Empty = "use the client's default" (backward compatible). Non-empty values
// are forwarded by the OpenAI-compatible client (`internal/llm/client.go`) into
// the Ollama payload.
func TestCompletionRequest_ToolChoiceField(t *testing.T) {
	req := CompletionRequest{ToolChoice: "required"}
	if req.ToolChoice != "required" {
		t.Errorf("expected ToolChoice='required'; got %q", req.ToolChoice)
	}

	// Empty default — must not break existing callers.
	empty := CompletionRequest{}
	if empty.ToolChoice != "" {
		t.Errorf("expected zero-value ToolChoice=''; got %q", empty.ToolChoice)
	}
}
