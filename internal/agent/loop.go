package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// AgentConfig contains configuration for the Agent.
// Goroutine-safe: AgentConfig values should be treated as immutable after creation.
type AgentConfig struct {
	// MaxIterations is the maximum number of ReAct iterations per round (default: 20)
	MaxIterations int
	// MaxContinuations is the number of auto-continue rounds when the agent
	// exhausts MaxIterations but is still making progress (default: 3).
	// This makes the agent behave like Hermes — it keeps working until done.
	MaxContinuations int
	// SystemPrompt is the system prompt to use
	SystemPrompt string
	// ToolChoice controls when tools are used: "auto" or "none"
	ToolChoice string
	// Temperature controls randomness (default: 0.7)
	Temperature float64
	// MaxTokens is the maximum tokens to generate (default: 8192)
	MaxTokens int
	// PreferredProvider is the preferred provider to use
	PreferredProvider string
	// Model overrides the provider's default model (e.g. "qwen2.5-coder:14b")
	Model string
	// EnableSkills enables skill matching (default: true)
	EnableSkills bool
	// EnableMemory enables memory storage/recall (default: true)
	EnableMemory bool
	// EnableStreaming enables streaming responses (default: true)
	EnableStreaming bool
	// MaxContextTokens is the context size threshold for auto-compaction (default: 8000)
	MaxContextTokens int
	// EnableThinking switches the agent from the free-ReAct loop to the
	// hybrid thinking state machine (Assess → Plan → Execute → Verify → Reflect → Done).
	// When false (current default), the legacy ReAct loop in Execute() runs unchanged.
	EnableThinking bool
	// ThinkingToolBudget is the max tool calls allowed in one thinking loop
	// before forced Reflect or honest termination. Default: 15.
	ThinkingToolBudget int
	// ThinkingMaxReflections is the hard cap on Reflect state visits per task.
	// Default: 3.
	ThinkingMaxReflections int
	// ThinkingMaxReplans is the hard cap on replanning from Reflect. Default: 2.
	ThinkingMaxReplans int
	// ThinkingConfidenceThreshold is the minimum Assess confidence (0.0-1.0)
	// for the fast path (skip Plan). Default: 0.7.
	ThinkingConfidenceThreshold float64
	// ThinkingFewShot controls whether the worked JSON example is prepended
	// to the system prompt. Default: true.
	ThinkingFewShot bool
	// Orchestration toggles the Orchestrator-Worker pipeline (Planner →
	// Dispatcher → Workers → Synthesizer). When Enabled, Execute() routes
	// through executeOrchestrated() instead of the legacy ReAct loop or the
	// thinking state machine. This is the reliable default for multi-step
	// tasks on 14B models — each LLM call gets ONE job.
	Orchestration OrchestratorConfig
}

// DefaultAgentConfig returns a configuration with sensible defaults.
func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		MaxIterations:               20,
		MaxContinuations:            1,
		SystemPrompt:                "You are a helpful AI assistant. Use tools when necessary to complete tasks.",
		ToolChoice:                  "auto",
		Temperature:                 0.7,
		MaxTokens:                   8192,
		PreferredProvider:           "",
		Model:                       "",
		EnableSkills:                true,
		EnableMemory:                true,
		EnableStreaming:             true,
		MaxContextTokens:            8000,
		ThinkingToolBudget:          15,
		ThinkingMaxReflections:      3,
		ThinkingMaxReplans:          2,
		ThinkingConfidenceThreshold: 0.7,
		ThinkingFewShot:             true,
	}
}

// Agent implements the ReAct (Reason + Act) pattern for autonomous AI reasoning.
// The agent maintains context, executes tools, and iterates to complete tasks.
// Goroutine-safe: All methods are safe for concurrent use.
// Uses internal synchronization (RWMutex) for thread-safe operations.
type Agent struct {
	config   AgentConfig
	router   Router
	tools    ToolProvider
	skills   SkillProvider
	memory   kyoci.MemoryStore
	injector ContextInjector
	recorder TaskRecorder
	logger   *slog.Logger
	mu       sync.RWMutex

	// orchWorkerFn is a test seam for the orchestrator dispatcher. When nil,
	// executeWorkers binds to (*Agent).runWorker. Tests set it to inject
	// deterministic workers (e.g., one that sleeps to prove concurrency).
	orchWorkerFn workerFunc
}

// NewAgent creates a new agent with the given configuration.
func NewAgent(
	config AgentConfig,
	router Router,
	tools ToolProvider,
	skills SkillProvider,
	memory kyoci.MemoryStore,
	opts ...Option,
) *Agent {
	if config.MaxIterations <= 0 {
		config.MaxIterations = 10
	}
	if config.MaxContinuations <= 0 {
		config.MaxContinuations = 3
	}
	if config.MaxContextTokens <= 0 {
		config.MaxContextTokens = 8000
	}
	if config.Temperature <= 0 {
		config.Temperature = 0.7
	}
	if config.MaxTokens <= 0 {
		config.MaxTokens = 8192
	}

	a := &Agent{
		config:   config,
		router:   router,
		tools:    tools,
		skills:   skills,
		memory:   memory,
		injector: noopInjector{},
		recorder: noopRecorder{},
		logger:   slog.Default(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(a)
		}
	}
	return a
}

// Execute runs the agent with a non-streaming response.
// This is the main entry point for task execution.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - task: The user task to complete
//
// Returns:
//   - *kyoci.TaskResult: The result of task execution
//   - error: Any error that occurred
func (a *Agent) Execute(ctx context.Context, task string) (*kyoci.TaskResult, error) {
	a.logger.Info("agent executing task", "task", task)

	// a. Check skills first (zero-AI fast path)
	if a.config.EnableSkills && a.skills != nil {
		if skill, matched := a.skills.Match(task); matched {
			a.logger.Info("skill matched, executing (zero-AI)", "skill", skill.Name())
			result, err := a.skills.Execute(ctx, skill.Name(), task)
			if err != nil {
				return &kyoci.TaskResult{
					Error:           fmt.Errorf("skill execution failed: %w", err),
					ToolCallsMade:   0,
					Iterations:      0,
					Usage:           kyoci.TokenUsage{},
					Content:         "",
				}, err
			}
			return &kyoci.TaskResult{
				Content:         result,
				ToolCallsMade:   0,
				Iterations:      0,
				Usage:           kyoci.TokenUsage{},
				Error:           nil,
			}, nil
		}
	}

	// a.2 Hybrid thinking state machine — when EnableThinking is set, dispatch
	// to the structured Assess→Plan→Execute→Verify→Reflect→Done loop instead
	// of the free-ReAct path below. The legacy loop is left unchanged when the
	// flag is false, so existing tests and behavior are preserved.
	if a.config.EnableThinking {
		return a.executeWithThinking(ctx, task)
	}

	// a.1 Orchestrator-Worker pipeline — when Orchestration.Enabled is set,
	// run the 4-phase Planner → Dispatcher → Workers → Synthesizer pipeline.
	// This is the reliable default for multi-step tasks: each LLM call gets
	// exactly one focused job (decompose, execute-one-step, or synthesize).
	// Checked AFTER thinking so an explicit thinking flag still wins; in
	// practice config/default.yaml sets only one of the two.
	if a.config.Orchestration.Enabled {
		return a.executeOrchestrated(ctx, task)
	}

	// Legacy ReAct loop (structured reasoning / orchestration disabled).
	return a.executeReact(ctx, task)
}

// ExecuteStream runs the agent with a streaming response.
// Same logic as Execute but yields chunks as they arrive.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - task: The user task to complete
//
// Returns:
//   - <-chan kyoci.StreamChunk: Channel receiving stream chunks
//   - error: Any error that occurred before streaming started
func (a *Agent) ExecuteStream(ctx context.Context, task string) (<-chan kyoci.StreamChunk, error) {
	ch := make(chan kyoci.StreamChunk, 100)

	go func() {
		defer close(ch)

		a.logger.Info("agent executing task (streaming)", "task", task)

		// a. Check skills first (zero-AI fast path)
		if a.config.EnableSkills && a.skills != nil {
			if skill, matched := a.skills.Match(task); matched {
				a.logger.Info("skill matched, executing (zero-AI)", "skill", skill.Name())
				result, err := a.skills.Execute(ctx, skill.Name(), task)
				if err != nil {
					ch <- kyoci.StreamChunk{
						Error: fmt.Errorf("skill execution failed: %w", err),
						Done:  true,
					}
					return
				}
				ch <- kyoci.FinalChunk(result, kyoci.TokenUsage{}, kyoci.FinishStop)
				return
			}
		}

		// a.1 Orchestrator-Worker pipeline (streaming wrapper). When
		// Orchestration.Enabled is true, route through the 4-phase pipeline
		// (Planner → Workers → Synthesizer) instead of the legacy streaming
		// ReAct loop below. Mirrors the dispatch in Execute() at line 202.
		// The pipeline emits its final synthesizer answer as one chunk; the
		// frontend's ThinkingDots animation covers the 10-30s pipeline runtime.
		if a.config.Orchestration.Enabled {
			a.executeOrchestratedStream(ctx, task, ch)
			return
		}

		// b. Build context with system prompt (+ injected L3 intelligence), task
		conversationCtx := NewContext()

		// Inject L3 memory context
		systemPrompt := a.config.SystemPrompt
		if injected := a.injector.Inject(task); injected != "" {
			systemPrompt = systemPrompt + "\n\n" + injected
		}

		conversationCtx.AddMessage(kyoci.RoleSystem, systemPrompt)
		conversationCtx.AddMessage(kyoci.RoleUser, task)

		// NOTE: STM cross-conversation loading disabled — see Execute() for explanation.

		// c. Enter ReAct loop (streaming version)
		totalToolCalls := 0
		totalTokens := kyoci.TokenUsage{}
		iteration := 1

		for iteration <= a.config.MaxIterations {
			a.logger.Debug("ReAct iteration (streaming)", "iteration", iteration)

			// THINK: Stream LLM response
			streamCh, err := a.thinkStream(ctx, conversationCtx)
			if err != nil {
				a.logger.Error("LLM stream failed", "iteration", iteration, "error", err)
				ch <- kyoci.StreamChunk{
					Error: err,
					Done:  true,
				}
				return
			}

			// Process stream chunks
			var fullContent string
			var toolCalls []kyoci.ToolCall
			var streamTokens kyoci.TokenUsage
			var streamFinished bool

			for chunk := range streamCh {
				// Yield content chunks
				if chunk.Content != "" {
					ch <- kyoci.ContentChunk(chunk.Content)
					fullContent += chunk.Content
				}

				// Yield tool call chunks
				if chunk.ToolCall != nil {
					ch <- kyoci.ToolCallChunk(*chunk.ToolCall)
					toolCalls = append(toolCalls, *chunk.ToolCall)
				}

				// Track completion
				if chunk.Done {
					streamFinished = true
					if chunk.Usage != nil {
						streamTokens = *chunk.Usage
					}
				}

				// Handle errors
				if chunk.Error != nil {
					ch <- chunk
					return
				}
			}

			// Accumulate token usage
			totalTokens.PromptTokens += streamTokens.PromptTokens
			totalTokens.CompletionTokens += streamTokens.CompletionTokens
			totalTokens.TotalTokens += streamTokens.TotalTokens

			a.logger.Info("THINK (streaming)",
				"iteration", iteration,
				"tokens", streamTokens.TotalTokens,
				"tool_calls", len(toolCalls))

			// Check if we need to act (tool calls)
			if len(toolCalls) > 0 {
				// Add assistant tool_call message to conversation history
				conversationCtx.AddAssistantMessage(fullContent, toolCalls)

				// ACT: Execute each tool call
				for _, toolCall := range toolCalls {
					startTime := time.Now()

					a.logger.Info("ACT (streaming)",
						"iteration", iteration,
						"tool", toolCall.Name)

					result, err := a.act(ctx, toolCall)
					duration := time.Since(startTime)

					// OBSERVE: Log tool result and add to context
					a.logger.Info("OBSERVE (streaming)",
						"iteration", iteration,
						"tool", toolCall.Name,
						"duration", duration)

					if err != nil {
						result = fmt.Sprintf("Error: %v", err)
					}

					conversationCtx.AddToolResult(toolCall.ID, result)
					totalToolCalls++
				}

				// Continue loop to process tool results
				iteration++
				continue
			}

			// No tool calls means we have a final answer
			if streamFinished {
				ch <- kyoci.FinalChunk(fullContent, totalTokens, kyoci.FinishStop)
				return
			}

			iteration++
		}

		// Exceeded max iterations
		ch <- kyoci.StreamChunk{
			Error: kyoci.ErrMaxIterations,
			Done:  true,
		}
	}()

	return ch, nil
}

// think performs the THINK phase by calling the LLM.
func (a *Agent) think(ctx context.Context, conversationCtx *Context) (kyoci.TokenUsage, *kyoci.CompletionResponse, error) {
	messages := conversationCtx.GetMessages()

	// Build completion request
	req := kyoci.CompletionRequest{
		Messages:    messages,
		Temperature: a.config.Temperature,
		MaxTokens:   a.config.MaxTokens,
		Model:       a.config.Model,
	}

	// Add tool definitions if tool choice is auto
	if a.config.ToolChoice == "auto" && a.tools != nil {
		toolDefs := a.tools.List()
		req.Tools = make([]kyoci.ToolDefinition, len(toolDefs))
		for i, def := range toolDefs {
			req.Tools[i] = def
		}
	}

	// Route request to LLM
	resp, err := a.router.Route(ctx, req, a.config.PreferredProvider)
	if err != nil {
		return kyoci.TokenUsage{}, nil, fmt.Errorf("LLM routing failed: %w", err)
	}

	return resp.Usage, resp, nil
}

// thinkStream performs the THINK phase with streaming.
func (a *Agent) thinkStream(ctx context.Context, conversationCtx *Context) (<-chan kyoci.StreamChunk, error) {
	messages := conversationCtx.GetMessages()

	// Build completion request
	req := kyoci.CompletionRequest{
		Messages:    messages,
		Temperature: a.config.Temperature,
		MaxTokens:   a.config.MaxTokens,
		Model:       a.config.Model,
	}

	// Add tool definitions if tool choice is auto
	if a.config.ToolChoice == "auto" && a.tools != nil {
		toolDefs := a.tools.List()
		req.Tools = make([]kyoci.ToolDefinition, len(toolDefs))
		for i, def := range toolDefs {
			req.Tools[i] = def
		}
	}

	// Route streaming request to LLM
	return a.router.RouteStream(ctx, req, a.config.PreferredProvider)
}

// act performs the ACT phase by executing a tool.
func (a *Agent) act(ctx context.Context, toolCall kyoci.ToolCall) (string, error) {
	if a.tools == nil {
		return "", fmt.Errorf("no tool registry available")
	}

	// Parse tool arguments
	params, err := kyoci.ParseToolCallArguments(toolCall.Arguments)
	if err != nil {
		return "", fmt.Errorf("failed to parse tool arguments: %w", err)
	}

	// Execute tool
	return a.tools.Execute(ctx, toolCall.Name, params)
}

// SetConfig updates the agent configuration.
func (a *Agent) SetConfig(config AgentConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.config = config
}

// WithLogger returns a shallow clone of the agent with its logger replaced.
// Used by the orchestrator to scope per-task events to a per-run log file
// without mutating the shared role agent — the clone shares the underlying
// router/tools/skills/memory (which are read-only during execution) and only
// diverges on the logger pointer. Safe to call per-task; the clone is local
// to the calling goroutine and is garbage-collected when the task returns.
//
// Returns the receiver unchanged when l is nil (no per-run logging in scope).
func (a *Agent) WithLogger(l *slog.Logger) *Agent {
	if a == nil || l == nil {
		return a
	}
	clone := *a // shallow copy — shared pointers are intentional
	clone.logger = l
	return &clone
}

// GetConfig returns the current agent configuration.
func (a *Agent) GetConfig() AgentConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config
}

// ToolNames returns the names of all tools currently registered on this agent.
// Used by the role layer to verify per-role tool filtering (Step 7) —
// specifically, that MCP/dynamic tools pass through the allowlist while
// non-allowlisted built-ins are stripped. Safe for concurrent use.
func (a *Agent) ToolNames() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.tools == nil {
		return nil
	}
	defs := a.tools.List()
	names := make([]string, 0, len(defs))
	for _, d := range defs {
		names = append(names, d.Name)
	}
	return names
}

// ApprovalFunc is called before a tool executes. Return false to deny.
type ApprovalFunc func(toolName string, argsJSON string) (bool, error)

type approvalKey struct{}

// WithApproval attaches an approval callback to the context.
func WithApproval(ctx context.Context, fn ApprovalFunc) context.Context {
	return context.WithValue(ctx, approvalKey{}, fn)
}

func getApproval(ctx context.Context) ApprovalFunc {
	fn, _ := ctx.Value(approvalKey{}).(ApprovalFunc)
	return fn
}

// summarizeToolCall extracts a human-readable summary from tool call arguments.
func summarizeToolCall(toolName, argsJSON string) string {
	args, _ := kyoci.ParseToolCallArguments(argsJSON)
	if args == nil {
		return ""
	}

	switch toolName {
	case "terminal":
		if cmd, ok := args["command"].(string); ok {
			if len(cmd) > 100 {
				return cmd[:100] + "..."
			}
			return cmd
		}
	case "file":
		if action, ok := args["action"].(string); ok {
			path, _ := args["path"].(string)
			return action + " " + path
		}
	case "browser":
		if action, ok := args["action"].(string); ok {
			url, _ := args["url"].(string)
			if url != "" {
				return action + " " + url
			}
			return action
		}
	case "http_client":
		if method, ok := args["method"].(string); ok {
			url, _ := args["url"].(string)
			return method + " " + url
		}
	case "web_search":
		if q, ok := args["query"].(string); ok {
			return "search: " + q
		}
	case "todo":
		if action, ok := args["action"].(string); ok {
			return "todo: " + action
		}
	case "delegation":
		if action, ok := args["action"].(string); ok {
			goal, _ := args["goal"].(string)
			if goal != "" {
				return action + ": " + truncateStr(goal, 60)
			}
			return action
		}
	case "remember":
		if k, ok := args["key"].(string); ok {
			v, _ := args["value"].(string)
			return k + " = " + truncateStr(v, 50)
		}
	case "memory_recall":
		if q, ok := args["query"].(string); ok {
			return "recall: " + q
		}
	case "skill":
		if action, ok := args["action"].(string); ok {
			name, _ := args["name"].(string)
			return action + " " + name
		}
	case "security_scan":
		if p, ok := args["path"].(string); ok {
			return "scan: " + p
		}
	}
	return ""
}

// summarizeResult truncates a tool result to a readable summary.
func summarizeResult(result string, maxLen int) string {
	result = strings.TrimSpace(result)
	if len(result) <= maxLen {
		return result
	}
	// Try to get first meaningful line
	for _, line := range strings.Split(result, "\n") {
		line = strings.TrimSpace(line)
		if len(line) > 10 {
			return truncateStr(line, maxLen)
		}
	}
	return truncateStr(result, maxLen)
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// truncateToolResult truncates large tool outputs to prevent context bloat.
// Keeps the first 60% and last 30% of the result, with a truncation marker in between.
// This lets the model see both the beginning (headers, status) and end (errors, summaries)
// of long outputs like file listings or build logs.
func truncateToolResult(result string, maxLen int) string {
	if len(result) <= maxLen {
		return result
	}

	headLen := int(float64(maxLen) * 0.6)
	tailLen := int(float64(maxLen) * 0.3)

	head := result[:headLen]
	tail := result[len(result)-tailLen:]
	truncatedCount := len(result) - headLen - tailLen

	return head + fmt.Sprintf("\n\n... [%d characters truncated] ...\n\n", truncatedCount) + tail
}

// sanitizeContent strips tool-call artifacts and internal formatting that the model
// may leak into its final response. This is a safety net — the LLM client should
// already clean these, but small models are unpredictable.
func sanitizeContent(content string) string {
	if content == "" {
		return content
	}

	// Strip [Tool Call: name({...})] blocks
	// Use a simple scanner approach since regex for nested JSON is unreliable
	var result strings.Builder
	lines := strings.Split(content, "\n")
	skipBlock := false
	braceDepth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Strip single-line Thought JSON (defense in depth for the thinking
		// system). We only catch Thought-shaped JSON — lines that start with
		// '{' and contain a Thought-specific key — so ordinary JSON output
		// is preserved.
		if strings.HasPrefix(trimmed, "{") && isThoughtJSONLine(trimmed) {
			continue
		}

		// Detect start of a tool call artifact
		if strings.HasPrefix(trimmed, "[Tool Call:") {
			// Check if it's self-contained on one line
			if strings.HasSuffix(trimmed, "]") {
				continue // Skip this line entirely
			}
			// Multi-line block — start skipping
			skipBlock = true
			braceDepth = strings.Count(trimmed, "{") - strings.Count(trimmed, "}")
			continue
		}

		if skipBlock {
			braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
			if strings.Contains(trimmed, "]") && braceDepth <= 0 {
				skipBlock = false
			}
			continue
		}

		result.WriteString(line)
		result.WriteString("\n")
	}

	cleaned := strings.TrimSpace(result.String())

	// If everything was stripped, return a reasonable default
	if cleaned == "" {
		return "Done."
	}

	return cleaned
}

// isThoughtJSONLine reports whether a single line looks like a Thought JSON
// object emitted by the thinking system. It checks for the presence of any
// Thought-schema-specific key, which avoids false positives on ordinary JSON.
func isThoughtJSONLine(line string) bool {
	// Thought-specific keys that don't appear in normal tool output.
	markers := []string{
		`"task_understanding"`,
		`"next_action"`,
		`"verification_evidence"`,
		`"tool_rationale"`,
	}
	for _, m := range markers {
		if strings.Contains(line, m) {
			return true
		}
	}
	return false
}

// extractTextToolCalls detects and parses tool calls that a model outputs as
// JSON text instead of native function calls. This is essential for models
// like qwen2.5-coder that don't support Ollama's function calling but still
// produce correct JSON tool-call syntax in their text output.
//
// Supported formats:
//   {"name": "file", "arguments": {"path": "...", "content": "..."}}
//   {"name": "Write", "arguments": {"path": "...", ...}}
//   ```json\n{"name": "file", ...}\n```
//
// Returns parsed ToolCalls or nil if none found. Named parseFencedJSONToolCalls
// to disambiguate from the llm package's extractTextToolCalls text-artifact
// stripper (different signature/semantics).
func parseFencedJSONToolCalls(content string) []kyoci.ToolCall {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	// Strip markdown code fences if present
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		var jsonLines []string
		inBlock := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") {
				if inBlock {
					break // end of block
				}
				inBlock = true
				continue
			}
			if inBlock {
				jsonLines = append(jsonLines, line)
			}
		}
		if len(jsonLines) > 0 {
			content = strings.Join(jsonLines, "\n")
		}
	}

	content = strings.TrimSpace(content)

	// Quick check: must look like JSON with "name" and "arguments"
	if !strings.Contains(content, "\"name\"") || !strings.Contains(content, "\"arguments\"") {
		return nil
	}

	// Try to parse as a single tool call object
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		// Try to find the first { ... } block
		start := strings.Index(content, "{")
		end := strings.LastIndex(content, "}")
		if start == -1 || end == -1 || end <= start {
			return nil
		}
		extracted := content[start : end+1]
		if err2 := json.Unmarshal([]byte(extracted), &raw); err2 != nil {
			return nil
		}
	}

	name, _ := raw["name"].(string)
	args, ok := raw["arguments"]
	if name == "" || !ok {
		return nil
	}

	// Normalize tool names (models sometimes use capitalized names)
	nameLower := strings.ToLower(name)
	switch nameLower {
	case "write", "create", "save":
		name = "file"
	case "read", "open":
		name = "file"
	case "execute", "run", "bash", "shell":
		name = "terminal"
	}

	// Marshal arguments back to JSON string
	argsBytes, err := json.Marshal(args)
	if err != nil {
		return nil
	}

	// If it's a file tool, ensure operation is set
	if name == "file" {
		var argsMap map[string]interface{}
		if json.Unmarshal(argsBytes, &argsMap) == nil {
			if _, hasOp := argsMap["operation"]; !hasOp {
				// Infer operation from context
				if _, hasContent := argsMap["content"]; hasContent {
					argsMap["operation"] = "write"
				} else if _, hasPath := argsMap["path"]; hasPath {
					argsMap["operation"] = "read"
				}
				argsBytes, _ = json.Marshal(argsMap)
			}
		}
	}

	return []kyoci.ToolCall{
		{
			ID:        fmt.Sprintf("textcall_%d", time.Now().UnixNano()),
			Name:      name,
			Arguments: string(argsBytes),
		},
	}
}