package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// ChatRequest is the body of POST /api/dashboard/chat. Messages is the
// conversation history (the user's latest message is the last one). Mode
// "chat" calls provider.Stream directly; "agent" delegates to the v5
// orchestrator pipeline via ExecuteStream.
type ChatRequest struct {
	Mode     string        `json:"mode"`     // "chat" (default) or "agent"
	Provider string        `json:"provider"` // required for chat mode
	Model    string        `json:"model"`    // optional; falls back to provider default
	Messages []ChatMessage `json:"messages"`
	Task     string        `json:"task,omitempty"` // optional shortcut for agent mode (last user message is used if empty)
	Timeout  int           `json:"timeout,omitempty"`
	Files    []UploadedFile `json:"files,omitempty"` // files uploaded via /api/dashboard/upload, surfaced to the agent as available context
}

// ChatMessage is the wire form of kyoci.Message — role comes in as a string.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req ChatRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if len(req.Messages) == 0 && req.Task == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "messages or task required"})
		return
	}

	mode := req.Mode
	if mode == "" {
		mode = "chat"
	}

	switch mode {
	case "agent":
		s.chatAgent(w, r, req)
	case "chat":
		s.chatDirect(w, r, req)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mode must be 'chat' or 'agent'"})
	}
}

// chatDirect streams the chosen provider's response token-by-token. This is
// the lightweight path: no planner/worker/synthesizer, no tools — just a
// straight chat completion. Reuses kyoci.Provider.Stream() so any of the 11
// providers (Anthropic, Ollama, OpenAI, etc.) work out of the box.
func (s *Server) chatDirect(w http.ResponseWriter, r *http.Request, req ChatRequest) {
	if req.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider required for chat mode"})
		return
	}
	reg := s.orch.GetProviderRegistry()
	provider, err := reg.Get(req.Provider)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider not enabled: " + err.Error()})
		return
	}
	if !provider.IsAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "provider not available (check API key)"})
		return
	}

	msgs := make([]kyoci.Message, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, kyoci.Message{
			Role:    roleFromString(m.Role),
			Content: m.Content,
		})
	}

	completionReq := kyoci.CompletionRequest{
		Messages: msgs,
		Model:    req.Model,
		Stream:   true,
	}

	timeout := time.Duration(req.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	chunkChan, err := provider.Stream(ctx, completionReq)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "stream start failed: " + err.Error()})
		return
	}

	streamSSE(w, r, func(writeErr *string) <-chan kyoci.StreamChunk {
		return chunkChan
	})
}

// chatAgent delegates to the orchestrator pipeline. The user's latest message
// becomes the task. SSE shape matches /api/v1/execute stream.
func (s *Server) chatAgent(w http.ResponseWriter, r *http.Request, req ChatRequest) {
	task := req.Task
	if task == "" {
		// last user message becomes the task
		for i := len(req.Messages) - 1; i >= 0; i-- {
			if roleFromString(req.Messages[i].Role) == kyoci.RoleUser {
				task = req.Messages[i].Content
				break
			}
		}
	}
	if task == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no user message to run as task"})
		return
	}

	// Surface uploaded files at the top of the task so the planner knows they
	// exist and can route work to the uploaded_file / excel tools. Filenames
	// are the server-generated UUID-prefixed names the tools resolve directly.
	if len(req.Files) > 0 {
		var b strings.Builder
		b.WriteString(task)
		b.WriteString("\n\n[Attached files — use the uploaded_file or excel tool with these exact filenames to read them]\n")
		for _, f := range req.Files {
			fmt.Fprintf(&b, "- %s (%d bytes, %s)\n", f.Filename, f.Size, f.MimeType)
		}
		task = b.String()
	}

	timeout := time.Duration(req.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	// Use Background context like the existing /api/v1/execute handler —
	// orchestrator sub-agents should survive HTTP client disconnects.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	chunkChan, err := s.orch.ExecuteStream(ctx, task, kyoci.RoleCustom)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "orchestrator failed to start: " + err.Error()})
		return
	}

	streamSSE(w, r, func(writeErr *string) <-chan kyoci.StreamChunk {
		return chunkChan
	})
}

// streamSSE writes a kyoci.StreamChunk channel to the client as Server-Sent
// Events. Each chunk becomes one `data: {...}` line. The terminal sentinel is
// `data: [DONE]`. Errors mid-stream are emitted as a chunk with an `error`
// field so the client can surface them.
func streamSSE(w http.ResponseWriter, r *http.Request, getChan func(*string) <-chan kyoci.StreamChunk) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering

	var writeErr string
	ch := getChan(&writeErr)

	for chunk := range ch {
		payload := map[string]any{
			"content":       chunk.Content,
			"done":          chunk.Done,
			"finish_reason": string(chunk.FinishReason),
		}
		if chunk.Usage != nil {
			payload["usage"] = chunk.Usage
		}
		if chunk.Error != nil {
			payload["error"] = chunk.Error.Error()
		}
		if chunk.Activity != nil {
			payload["activity"] = chunk.Activity
		}
		data, _ := json.Marshal(payload)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		if chunk.Done || chunk.Error != nil {
			break
		}
	}
	if writeErr != "" {
		errPayload, _ := json.Marshal(map[string]string{"error": writeErr})
		fmt.Fprintf(w, "data: %s\n\n", errPayload)
		flusher.Flush()
	}
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}
