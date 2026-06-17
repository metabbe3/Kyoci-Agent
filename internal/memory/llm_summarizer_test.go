package memory

import (
	"context"
	"fmt"
	"strings"
	"testing"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// LLM Summarizer — LLM-backed implementation of SummarizerFunc
//
// The summarizer asks an LLM to extract durable preferences, facts, and
// lessons from a batch of short-term entries. When the LLM decides nothing
// is worth remembering, it returns the sentinel "NOTHING_TO_REMEMBER" and the
// summarizer returns ("", nil) so the compactor skips long-term storage.
// =============================================================================

// mockLLMClient is a test double for the LLMClient interface.
type mockLLMClient struct {
	response *kyoci.CompletionResponse
	err      error
	captured kyoci.CompletionRequest
}

func (m *mockLLMClient) Complete(ctx context.Context, req kyoci.CompletionRequest) (*kyoci.CompletionResponse, error) {
	m.captured = req
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

// makeTestEntries creates n standalone MemoryEntries for summarizer tests.
func makeTestEntries(n int) []*kyoci.MemoryEntry {
	entries := make([]*kyoci.MemoryEntry, n)
	for i := 0; i < n; i++ {
		entries[i] = &kyoci.MemoryEntry{
			ID:      fmt.Sprintf("msg-%d", i),
			Content: fmt.Sprintf("user message %d about golang preferences", i),
			Type:    kyoci.MemoryShortTerm,
		}
	}
	return entries
}

// TestLLMSummarizer_Success verifies that prose from the LLM is returned verbatim.
func TestLLMSummarizer_Success(t *testing.T) {
	cli := &mockLLMClient{
		response: &kyoci.CompletionResponse{
			Content: "The user prefers Go with net/http and snake_case JSON tags.",
		},
	}
	summarize := NewLLMSummarizer(cli, LLMSummarizerConfig{}, silentLogger())

	summary, err := summarize(context.Background(), makeTestEntries(3))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(summary, "Go") || !strings.Contains(summary, "snake_case") {
		t.Errorf("summary does not contain expected content: %q", summary)
	}
}

// TestLLMSummarizer_NothingToRemember verifies that the NOTHING_TO_REMEMBER
// sentinel causes the summarizer to return ("", nil), signaling the compactor
// to skip long-term storage.
func TestLLMSummarizer_NothingToRemember(t *testing.T) {
	cli := &mockLLMClient{
		response: &kyoci.CompletionResponse{
			Content: "NOTHING_TO_REMEMBER",
		},
	}
	summarize := NewLLMSummarizer(cli, LLMSummarizerConfig{}, silentLogger())

	summary, err := summarize(context.Background(), makeTestEntries(3))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary != "" {
		t.Errorf("expected empty summary for NOTHING_TO_REMEMBER, got %q", summary)
	}
}

// TestLLMSummarizer_ClientError verifies that an LLM failure propagates as an
// error (the compactor's fallback path handles this).
func TestLLMSummarizer_ClientError(t *testing.T) {
	cli := &mockLLMClient{
		err: fmt.Errorf("LLM unavailable"),
	}
	summarize := NewLLMSummarizer(cli, LLMSummarizerConfig{}, silentLogger())

	_, err := summarize(context.Background(), makeTestEntries(3))
	if err == nil {
		t.Errorf("expected error from client failure, got nil")
	}
}

// TestLLMSummarizer_PromptShape verifies the CompletionRequest is well-formed:
// system message instructs extraction of preferences/facts/lessons, user
// message contains the conversation entries, and the model field is propagated.
func TestLLMSummarizer_PromptShape(t *testing.T) {
	cli := &mockLLMClient{
		response: &kyoci.CompletionResponse{Content: "ok"},
	}
	summarize := NewLLMSummarizer(cli, LLMSummarizerConfig{Model: "test-model"}, silentLogger())

	_, _ = summarize(context.Background(), makeTestEntries(3))

	req := cli.captured
	if len(req.Messages) < 2 {
		t.Fatalf("expected >=2 messages, got %d", len(req.Messages))
	}
	// System message must instruct the LLM to extract preferences/facts/lessons.
	sysMsg := req.Messages[0].Content
	for _, keyword := range []string{"preference", "fact", "lesson"} {
		if !strings.Contains(strings.ToLower(sysMsg), keyword) {
			t.Errorf("system prompt missing keyword %q;\nsysMsg: %s", keyword, sysMsg)
		}
	}
	// The NOTHING_TO_REMEMBER sentinel must be documented in the system prompt.
	if !strings.Contains(sysMsg, "NOTHING_TO_REMEMBER") {
		t.Errorf("system prompt must mention NOTHING_TO_REMEMBER sentinel;\nsysMsg: %s", sysMsg)
	}
	// User message must contain the conversation entries.
	userMsg := req.Messages[1].Content
	if !strings.Contains(userMsg, "golang preferences") {
		t.Errorf("user prompt missing conversation content;\nuserMsg: %s", userMsg)
	}
	// Model must be propagated from config.
	if req.Model != "test-model" {
		t.Errorf("model = %q, want %q", req.Model, "test-model")
	}
	// Tools must be empty (summarizer is a plain completion, no tool calling).
	if len(req.Tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(req.Tools))
	}
}
