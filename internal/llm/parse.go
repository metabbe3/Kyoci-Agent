package llm

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// Tool-call parsers for LLM text output. Extracted from client.go:
// extractTextToolCalls strips [Tool Call: ...] artifacts; extractBareJSONToolCalls
// parses bare {"name","arguments"} JSON objects; normalizeContent falls back to
// the reasoning field when content is empty. All are best-effort and panic-free.

// textToolCallPattern matches [Tool Call: name({...json...})] blocks that the model
// echoes from the flattened Ollama conversation history.
var textToolCallPattern = regexp.MustCompile(`(?s)\[Tool Call:\s*(\w+)\((\{.*?\})\)\]`)

// extractTextToolCalls strips [Tool Call: ...] text artifacts from content and
// optionally parses them into structured ToolCall objects.
func extractTextToolCalls(content string) (string, []kyoci.ToolCall) {
	matches := textToolCallPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return content, nil
	}

	var calls []kyoci.ToolCall
	for i, m := range matches {
		calls = append(calls, kyoci.ToolCall{
			ID:        fmt.Sprintf("textcall_%d_%d", i, time.Now().UnixNano()),
			Name:      m[1],
			Arguments: m[2],
		})
	}

	// Remove all matched text from content
	cleaned := textToolCallPattern.ReplaceAllString(content, "")
	// Clean up extra whitespace/newlines left behind
	cleaned = regexp.MustCompile(`\n{3,}`).ReplaceAllString(cleaned, "\n\n")
	cleaned = strings.TrimSpace(cleaned)

	return cleaned, calls
}

// normalizeContent returns the model's textual answer, falling back to the
// `reasoning` field when `content` is empty or whitespace-only.
//
// Some models (Gemma, o1-style, DeepSeek-R1) emit their final answer inside a
// `reasoning` chain-of-thought field while leaving `content` empty. Without
// this fallback the caller would see an empty string and treat the call as a
// failure. The fallback keeps the pipeline model-agnostic.
func normalizeContent(content, reasoning string) string {
	if strings.TrimSpace(content) != "" {
		return content
	}
	if strings.TrimSpace(reasoning) != "" {
		return strings.TrimSpace(reasoning)
	}
	return ""
}

// bareJSONToolCallPattern matches a JSON object containing a "name" and
// "arguments" pair, as emitted by qwen2.5-coder and a few other Ollama models
// when they ignore OpenAI-style tool_calls and instead emit tool intent
// directly in `content`. Optional surrounding <tool_call>...</tool_call> tags
// or ```json fences are tolerated.
var bareJSONToolCallPattern = regexp.MustCompile(
	"(?s)(?:<tool_call>\\s*)?(?:(?:```)(?:json)?\\s*)?\\{\\s*\"name\"\\s*:\\s*\"([^\"]+)\"\\s*,\\s*\"arguments\"\\s*:\\s*(\\{.*?\\})\\s*\\}(?:\\s*</tool_call>)?(?:\\s*```)?",
)

// extractBareJSONToolCalls scans content for bare-JSON tool call objects and
// returns (cleaned content, parsed tool calls). When no calls are found, the
// content is returned unchanged. Malformed JSON is ignored silently — the
// function is best-effort and never panics.
func extractBareJSONToolCalls(content string) (string, []kyoci.ToolCall) {
	if strings.TrimSpace(content) == "" {
		return content, nil
	}
	matches := bareJSONToolCallPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return content, nil
	}

	var calls []kyoci.ToolCall
	for i, m := range matches {
		name := strings.TrimSpace(m[1])
		args := strings.TrimSpace(m[2])
		if name == "" {
			continue
		}
		// Validate args is parseable JSON; skip if not.
		var js interface{}
		if err := json.Unmarshal([]byte(args), &js); err != nil {
			continue
		}
		raw, _ := json.Marshal(js) // re-marshal to canonical form
		calls = append(calls, kyoci.ToolCall{
			ID:        fmt.Sprintf("barecall_%d_%d", i, time.Now().UnixNano()),
			Name:      name,
			Arguments: string(raw),
		})
	}

	if len(calls) == 0 {
		return content, nil
	}

	// Remove the matched tool-call objects from content so the synthesizer /
	// user doesn't see raw JSON. Preserve any surrounding prose.
	cleaned := bareJSONToolCallPattern.ReplaceAllString(content, "")
	cleaned = regexp.MustCompile(`\n{3,}`).ReplaceAllString(cleaned, "\n\n")
	cleaned = strings.TrimSpace(cleaned)
	return cleaned, calls
}
