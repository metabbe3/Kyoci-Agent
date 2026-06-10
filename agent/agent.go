package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"text/template"
	"time"

	"github.com/nicholas/ai-agent/config"
	"github.com/nicholas/ai-agent/llm"
	"github.com/nicholas/ai-agent/memory"
	"github.com/nicholas/ai-agent/selfimprove"
	"github.com/nicholas/ai-agent/tools"
)

// Agent v2 — autonomous AI agent with auto-compaction, tool guides, self-improvement
type Agent struct {
	config    *config.Config
	router    *llm.Router
	tools     *tools.Registry
	shortMem  *memory.ConversationBuffer
	longMem   *memory.JSONMemoryStore
	compactor *memory.ConversationCompactor
	learner   *selfimprove.SelfImprover
	mode      string // system, coder, researcher, analyst, browser, creative
	tmpl      *template.Template
	orchestrator *Orchestrator // sub-agent delegation system
}

// NewV2 creates an advanced agent with all subsystems
func NewV2(cfg *config.Config, router *llm.Router, toolReg *tools.Registry) *Agent {
	// Short-term: conversation buffer
	shortMem := memory.NewConversationBuffer(cfg.Memory.MaxTokens)

	// Long-term: persistent JSON store
	longMem, err := memory.NewJSONMemoryStore(cfg.Memory.LongTermPath)
	if err != nil {
		slog.Warn("Long-term memory init failed", "error", err)
	}

	// Compactor: auto-summarize when tokens > threshold
	threshold := cfg.Memory.CompactionThreshold
	if threshold <= 0 {
		threshold = 0.75
	}
	compactor := memory.NewConversationCompactor(shortMem, threshold)

	// Self-improvement
	learner := selfimprove.NewSelfImprover(cfg.Agent.DataDir)

	// Load prompt template
	tmpl := loadTemplate(cfg.Agent.Template)

	_ = longMem // used below

	agent := &Agent{
		config:    cfg,
		router:    router,
		tools:     toolReg,
		shortMem:  shortMem,
		longMem:   longMem,
		compactor: compactor,
		learner:   learner,
		mode:      cfg.Agent.Template,
		tmpl:      tmpl,
	}

	// Initialize orchestrator for sub-agent delegation
	agent.orchestrator = NewOrchestrator(
		agent,
		WithMaxParallel(3),
		WithMaxDepth(1),
		WithTokenBudget(10000),
	)

	return agent
}

// Run executes the full agent loop with self-improvement
func (a *Agent) Run(ctx context.Context, userInput string) (string, error) {
	start := time.Now()

	// Get self-improvement advice for this task
	advice := a.learner.GetAdvice(userInput)

	// Build system prompt from template + tool guide
	systemPrompt := a.buildSystemPrompt(advice)

	// Initialize memory if fresh
	msgs := a.shortMem.GetMessages()
	if len(msgs) == 0 {
		a.shortMem.Add("system", systemPrompt)
	}

	// Add user message
	a.shortMem.Add("user", userInput)

	// Auto-compact if needed
	if a.compactor.ShouldCompact() {
		if a.config.Agent.Verbose {
			slog.Debug("Auto-compacting context", "tokens", a.shortMem.TokenCount())
		}
		err := a.compactor.Compact()
		if err != nil && a.config.Agent.Verbose {
			slog.Warn("Compaction failed", "error", err)
		} else if a.config.Agent.Verbose {
			stats := a.compactor.GetStats()
			slog.Debug("Compacted", "compactions", stats.TotalCompactions, "tokens_saved", stats.TokensSaved)
		}
	}

	// Get tool schemas
	toolSchemas := a.convertToolSchemas()

	// ReAct loop
	for i := 0; i < a.config.Agent.MaxIterations; i++ {
		messages := a.convertMessages()
		resp, err := a.router.Chat(ctx, messages, toolSchemas)
		if err != nil {
			return "", fmt.Errorf("LLM failed (iter %d): %w", i+1, err)
		}

		if a.config.Agent.Verbose {
			slog.Debug("Agent iteration", "iter", i+1, "stop_reason", resp.StopReason, "input_tokens", resp.Usage.InputTokens, "output_tokens", resp.Usage.OutputTokens, "content_len", len(resp.Content))
		}

		// Done — no tool calls
		if resp.StopReason == "stop" || resp.StopReason == "max_tokens" {
			a.shortMem.Add("assistant", resp.Content)

			// Auto-extract important facts to long-term memory
			go a.extractFacts(userInput, resp.Content)

			// Record outcome for self-improvement
			duration := time.Since(start)
			if err := a.learner.RecordOutcome(userInput, "agent", true, duration); err != nil {
				slog.Warn("Self-improve record failed", "error", err)
			} else if a.config.Agent.Verbose {
				slog.Debug("Self-improve recorded", "task", truncate(userInput, 40), "duration", duration)
			}

			if a.config.Agent.Verbose {
				slog.Debug("Agent completed", "duration", duration, "iterations", i+1)
			}
			return resp.Content, nil
		}

		// Tool calls
		if resp.StopReason == "tool_use" && len(resp.ToolCalls) > 0 {
			if resp.Content != "" {
				a.shortMem.Add("assistant", resp.Content)
			}

			for _, tc := range resp.ToolCalls {
				toolStart := time.Now()
				if a.config.Agent.Verbose {
					slog.Debug("Tool call", "tool", tc.Name, "args", truncate(tc.Arguments, 200))
				}

				result, err := a.tools.ExecuteTool(ctx, tc.Name, json.RawMessage(tc.Arguments))
				duration := time.Since(toolStart)

				if err != nil {
					result = fmt.Sprintf("Error: %v", err)
					if recErr := a.learner.RecordOutcome(tc.Name, tc.Name, false, duration); recErr != nil {
						slog.Warn("Self-improve record failed", "tool", tc.Name, "error", recErr)
					}
				} else {
					if recErr := a.learner.RecordOutcome(tc.Name, tc.Name, true, duration); recErr != nil {
						slog.Warn("Self-improve record failed", "tool", tc.Name, "error", recErr)
					}
				}

				if a.config.Agent.Verbose {
					slog.Debug("Tool result", "tool", tc.Name, "result", truncate(result, 100), "duration", duration)
				}

				a.shortMem.Add("tool", fmt.Sprintf("[Tool: %s] %s", tc.Name, result))
			}
			continue
		}

		// Unexpected
		a.shortMem.Add("assistant", resp.Content)
		return resp.Content, nil
	}

	return "", fmt.Errorf("max iterations (%d)", a.config.Agent.MaxIterations)
}

// Stream runs the agent with streaming response
func (a *Agent) Stream(ctx context.Context, userInput string) (<-chan string, error) {
	ch := make(chan string, 100)

	msgs := a.shortMem.GetMessages()
	if len(msgs) == 0 {
		a.shortMem.Add("system", a.buildSystemPrompt(""))
	}
	a.shortMem.Add("user", userInput)

	toolSchemas := a.convertToolSchemas()
	messages := a.convertMessages()

	streamCh, err := a.router.Stream(ctx, messages, toolSchemas)
	if err != nil {
		close(ch)
		return nil, err
	}

	go func() {
		defer close(ch)
		var full string
		for chunk := range streamCh {
			if chunk.Done {
				break
			}
			if chunk.Content != "" {
				full += chunk.Content
				ch <- chunk.Content
			}
		}
		a.shortMem.Add("assistant", full)
	}()

	return ch, nil
}

// SetMode changes the agent's prompt template
func (a *Agent) SetMode(mode string) {
	a.mode = mode
	a.tmpl = loadTemplate(mode)
	a.Reset()
}

// GetMemory returns short-term memory
func (a *Agent) GetMemory() *memory.ConversationBuffer {
	return a.shortMem
}

// GetLongTermMemory returns long-term memory
func (a *Agent) GetLongTermMemory() *memory.JSONMemoryStore {
	return a.longMem
}

// GetSelfImprover returns the self-improvement engine
func (a *Agent) GetSelfImprover() *selfimprove.SelfImprover {
	return a.learner
}

// SetSelfImprover replaces the self-improvement engine (for pipeline sharing)
func (a *Agent) SetSelfImprover(si *selfimprove.SelfImprover) {
	a.learner = si
}

// Reset clears short-term memory
func (a *Agent) Reset() {
	a.shortMem.Clear()
}

// ── Internal ──

// buildSystemPrompt creates the full system prompt from template + tool guide + advice
func (a *Agent) buildSystemPrompt(advice string) string {
	toolGuide := a.buildToolGuide()

	var sb strings.Builder
	data := map[string]string{
		"SystemPrompt": a.config.Agent.SystemPrompt,
		"ToolGuide":    toolGuide,
	}

	if a.tmpl != nil {
		a.tmpl.Execute(&sb, data)
	} else {
		sb.WriteString(a.config.Agent.SystemPrompt)
	}

	// Append tool guide
	sb.WriteString("\n\n## Available Tools\n")
	sb.WriteString(toolGuide)

	// Append long-term memory facts if any
	if a.longMem != nil {
		entries, _ := a.longMem.GetByImportance(7)
		if len(entries) > 0 {
			sb.WriteString("\n\n## Remembered Facts\n")
			for i, f := range entries {
				if i >= 5 {
					break
				}
				sb.WriteString(fmt.Sprintf("- %s\n", f.Content))
			}
		}
	}

	// Append self-improvement advice
	if advice != "" {
		sb.WriteString("\n\n## Smart Suggestion\n")
		sb.WriteString(advice)
	}

	return sb.String()
}

// buildToolGuide creates a concise guide of when to use each tool
func (a *Agent) buildToolGuide() string {
	guides := map[string]string{
		"web_search":   "🔍 Search the web → when you need current info, news, facts you don't know",
		"calculator":   "🧮 Math → when you need to compute numbers, formulas, stats",
		"file_handler": "📁 Files → when you need to read/write/list files",
		"browser":      "🌐 Browser → when you need to browse websites, fill forms, take screenshots, handle JS pages",
		"terminal":     "💻 Terminal → when you need to run shell commands, install packages, manage processes",
		"vision":       "👁️ Vision → when you need to analyze images, screenshots, diagrams",
		"http_client":  "🔗 HTTP → when you need to call APIs, fetch URLs with custom headers/auth",
		"web_scraper":  "📄 Scraper → when you need to extract specific data from web pages (CSS selectors)",
		"pdf":          "📑 PDF → when you need to read/extract text from PDF files",
		"code_exec":    "⚡ Code → when you need to run Python/JS/Go code for computation or data processing",
		"database":     "🗄️ Database → when you need to query SQL databases (Postgres/MySQL/SQLite)",
		"email":        "📧 Email → when you need to send emails via SMTP",
		"image_gen":    "🎨 Image Gen → when you need to generate AI images",
		"scheduler":    "⏰ Scheduler → when you need to create recurring/scheduled tasks",
		"delegation":   "🤝 Delegate → when you need to spawn sub-agents for parallel work",
	}

	var sb strings.Builder
	toolList := a.tools.List()
	for _, t := range toolList {
		if guide, ok := guides[t.Name()]; ok {
			sb.WriteString(guide)
			sb.WriteString("\n")
		} else {
			sb.WriteString(fmt.Sprintf("🔧 %s — %s\n", t.Name(), truncate(t.Description(), 80)))
		}
	}
	return sb.String()
}

// extractFacts auto-extracts important facts from conversation to long-term memory
func (a *Agent) extractFacts(userInput, response string) {
	if a.longMem == nil {
		return
	}

	text := userInput + " " + response

	// Look for preference patterns
	preferences := []string{"i prefer", "i like", "i want", "i need", "always use", "never use"}
	for _, p := range preferences {
		if idx := strings.Index(strings.ToLower(text), p); idx != -1 {
			end := min(idx+len(p)+100, len(text))
			fact := text[idx:end]
			a.longMem.AddEntry(&memory.MemoryEntry{
				Category:   memory.CategoryPreference,
				Content:    fact,
				Importance: 7,
			})
		}
	}

	// Look for factual statements
	factMarkers := []string{"my name is", "i work at", "i'm using", "the project is"}
	for _, m := range factMarkers {
		if idx := strings.Index(strings.ToLower(text), m); idx != -1 {
			end := min(idx+len(m)+80, len(text))
			fact := text[idx:end]
			a.longMem.AddEntry(&memory.MemoryEntry{
				Category:   memory.CategoryFact,
				Content:    fact,
				Importance: 8,
			})
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (a *Agent) convertMessages() []llm.Message {
	msgs := a.shortMem.GetMessages()
	result := make([]llm.Message, len(msgs))
	for i, m := range msgs {
		result[i] = llm.Message{Role: m.Role, Content: m.Content}
	}
	return result
}

func (a *Agent) convertToolSchemas() []llm.ToolSchema {
	schemas := a.tools.Schemas()
	result := make([]llm.ToolSchema, len(schemas))
	for i, s := range schemas {
		name, _ := s["name"].(string)
		desc, _ := s["description"].(string)
		params, _ := s["parameters"].(map[string]interface{})
		result[i] = llm.ToolSchema{Name: name, Description: desc, Parameters: params}
	}
	return result
}

func loadTemplate(name string) *template.Template {
	tmplStr := ""
	switch name {
	case "coder":
		tmplStr = "{{.SystemPrompt}}\n\nMode: CODER. Focus on writing clean, efficient code. Use file_handler to read/write, terminal to run commands, calculator for math."
	case "researcher":
		tmplStr = "{{.SystemPrompt}}\n\nMode: RESEARCHER. Use web_search, browser, http_client to gather info. Cross-reference sources. Summarize findings."
	case "analyst":
		tmplStr = "{{.SystemPrompt}}\n\nMode: ANALYST. Use database for queries, calculator for stats, code_exec for Python/pandas. Provide data-driven insights."
	case "browser":
		tmplStr = "{{.SystemPrompt}}\n\nMode: BROWSER. Use browser for navigation, web_scraper for extraction, http_client for APIs. Handle JS-rendered pages."
	case "creative":
		tmplStr = "{{.SystemPrompt}}\n\nMode: CREATIVE. Generate original content. Use web_search for inspiration, file_handler to save drafts."
	default:
		tmplStr = "{{.SystemPrompt}}"
	}

	tmpl, err := template.New("system").Parse(tmplStr)
	if err != nil {
		slog.Error("Template parse error", "error", err)
		return nil
	}
	return tmpl
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
