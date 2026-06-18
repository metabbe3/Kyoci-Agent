package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/agent"
)

// TelegramGateway connects Kyoci Agent to Telegram via Bot API long-polling.
// Goroutine-safe: handles multiple concurrent chats.
//
// The gateway is split across focused files:
//   - tgclient.go: the Bot API HTTP client (tg *tgClient) and wire types.
//   - store.go:    per-chat state (ChatStore) behind a single mutex.
//   - approval.go: severity classification + approval-flow helpers.
//   - render.go:   markdown→HTML and message-sizing pure helpers.
//   - activity.go: the ActivityTracker + result telemetry types.
//
// telegram.go is the coordinator: Start / handleMessage / handleCallbackQuery
// and the command router.
type TelegramGateway struct {
	// tg is the Bot API HTTP client wrapper (httptest-able via injected *http.Client).
	tg     *tgClient
	token  string // kept for the "has token" log line in Start
	orch   OrchestratorClient
	logger *slog.Logger

	// Polling state
	offset  int64
	started bool
	mu      sync.RWMutex

	// Allowed chat IDs (empty = allow all)
	allowedChats map[int64]bool

	// Activity tracker — logs recent task activity for /activity command
	activity *ActivityTracker

	// Pending approvals: maps callback data prefix to approval channel
	pendingApprovals map[string]chan approvalResponse
	approvalMu       sync.Mutex

	// store holds all per-chat mutable state (roles, history, rate-limit,
	// trust, session whitelist) behind a single RWMutex.
	store *ChatStore
}

// approvalResponse carries the user's decision from callback to the waiting goroutine.
type approvalResponse struct {
	approved  bool
	whitelist bool // if true, add this tool to session whitelist
}

// Name returns the gateway identifier (satisfies Gateway).
func (gw *TelegramGateway) Name() string { return "telegram" }

// Stop drains pending approvals so any in-flight Decide callers unblock with a
// deny instead of waiting on the (now-shutting-down) gateway. Start itself
// returns once its context is canceled by the caller.
func (gw *TelegramGateway) Stop(_ context.Context) error {
	gw.approvalMu.Lock()
	defer gw.approvalMu.Unlock()
	for key, ch := range gw.pendingApprovals {
		select {
		case ch <- approvalResponse{approved: false}:
		default:
		}
		delete(gw.pendingApprovals, key)
	}
	return nil
}

// OrchestratorClient is the interface the gateway needs to execute tasks.
type OrchestratorClient interface {
	Execute(ctx context.Context, task string, role string) (string, int, error)
	ExecuteRich(ctx context.Context, task string, role string) (*ActivityResult, error)
	Status() (providers []string, roles int, tools int, skills int)
}

// TelegramConfig holds Telegram-specific configuration.
type TelegramConfig struct {
	Enabled      bool   `yaml:"enabled" env:"KYOCI_TELEGRAM_ENABLED"`
	Token        string `yaml:"token" env:"KYOCI_TELEGRAM_TOKEN"`
	AllowedUsers string `yaml:"allowed_users" env:"KYOCI_TELEGRAM_ALLOWED_USERS"` // comma-separated user IDs
	PollTimeout  int    `yaml:"poll_timeout" env:"KYOCI_TELEGRAM_POLL_TIMEOUT"`   // seconds, default 30
}

// NewTelegramGateway creates a new Telegram bot gateway.
func NewTelegramGateway(cfg TelegramConfig, orch OrchestratorClient, logger *slog.Logger) *TelegramGateway {
	if logger == nil {
		logger = slog.Default()
	}

	pollTimeout := cfg.PollTimeout
	if pollTimeout <= 0 {
		pollTimeout = 30
	}

	gw := &TelegramGateway{
		token:            cfg.Token,
		orch:             orch,
		logger:           logger.With("component", "telegram-gateway"),
		tg:               newTGClient(cfg.Token, &http.Client{Timeout: time.Duration(pollTimeout+10) * time.Second}, pollTimeout, logger.With("component", "telegram-api")),
		allowedChats:     make(map[int64]bool),
		activity:         NewActivityTracker(50),
		pendingApprovals: make(map[string]chan approvalResponse),
		store:            NewChatStore(),
	}

	// Parse allowed users
	if cfg.AllowedUsers != "" {
		for _, idStr := range strings.Split(cfg.AllowedUsers, ",") {
			idStr = strings.TrimSpace(idStr)
			if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
				gw.allowedChats[id] = true
			}
		}
	}

	return gw
}

// Start begins long-polling for Telegram updates. Blocks until context is cancelled.
func (gw *TelegramGateway) Start(ctx context.Context) error {
	gw.mu.Lock()
	if gw.started {
		gw.mu.Unlock()
		return fmt.Errorf("gateway already started")
	}
	gw.started = true
	gw.mu.Unlock()

	gw.logger.Info("Telegram gateway starting", "has_token", gw.token != "")

	// Verify bot token by calling getMe
	botInfo, err := gw.tg.getMe(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify bot token: %w", err)
	}
	gw.logger.Info("bot authenticated", "username", botInfo.Username, "id", botInfo.ID)

	// Send startup message to log
	gw.logger.Info("Telegram gateway polling started")

	// Main polling loop
	for {
		select {
		case <-ctx.Done():
			gw.logger.Info("Telegram gateway stopping")
			return nil
		default:
		}

		updates, err := gw.tg.getUpdates(ctx, gw.offset)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			gw.logger.Error("failed to get updates", "error", err)
			time.Sleep(2 * time.Second)
			continue
		}

		for _, update := range updates {
			gw.offset = update.UpdateID + 1
			if update.Message != nil && update.Message.Text != "" {
				go gw.handleMessage(ctx, update.Message)
			}
			if update.CallbackQuery != nil {
				go gw.handleCallbackQuery(ctx, update.CallbackQuery)
			}
		}
	}
}

// handleMessage processes a single incoming Telegram message.
func (gw *TelegramGateway) handleMessage(ctx context.Context, msg *tgMessage) {
	chatID := msg.Chat.ID
	userID := msg.From.ID
	text := strings.TrimSpace(msg.Text)

	// Rate limit check
	if !gw.store.checkRate(userID) {
		gw.logger.Warn("rate limited", "user_id", userID)
		return
	}

	// Access control
	if len(gw.allowedChats) > 0 && !gw.allowedChats[userID] {
		gw.logger.Warn("unauthorized user", "user_id", userID, "username", msg.From.Username)
		gw.tg.sendMessage(ctx, chatID, "You are not authorized to use this bot.", msg.MessageID)
		return
	}

	gw.logger.Info("message received",
		"user_id", userID,
		"username", msg.From.Username,
		"chat_id", chatID,
		"text_len", len(text))

	// Handle commands
	if strings.HasPrefix(text, "/") {
		gw.handleCommand(ctx, chatID, msg)
		return
	}

	// Determine role for this chat
	role := gw.store.getChatRole(chatID)

	// Build task with conversation context
	taskWithCtx := gw.store.buildTaskWithContext(chatID, text)

	// Send initial status message that we'll EDIT as progress comes in
	roleEmoji := roleIconForRole(role)
	taskPreview := text
	if len(taskPreview) > 60 {
		taskPreview = taskPreview[:60] + "..."
	}
	statusMsgID := gw.sendStatusMessage(ctx, chatID, fmt.Sprintf("🚀 %s %s — %q",
		roleEmoji, role, taskPreview))

	// Track progress state for editing
	progressState := &progressTracker{
		chatID:       chatID,
		statusMsgID:  statusMsgID,
		roleEmoji:    roleEmoji,
		roleName:     role,
		iteration:    0,
		toolHistory:  []toolEntry{},
		lastUpdateAt: time.Now(),
	}

	// Send periodic "typing" indicators while task runs
	typingCtx, typingCancel := context.WithCancel(ctx)
	defer typingCancel()
	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		gw.tg.sendChatAction(typingCtx, chatID, "typing")
		for {
			select {
			case <-typingCtx.Done():
				return
			case <-ticker.C:
				gw.tg.sendChatAction(typingCtx, chatID, "typing")
			}
		}
	}()

	// Wire progress streaming — edit the status message with rich detail.
	// taskCtx is declared here (assigned below) so the progress closure can
	// capture it by reference — it is only invoked during Execute, by which
	// point taskCtx has been wired up.
	var taskCtx context.Context
	progressFn := func(ev agent.ProgressEvent) {
		switch ev.Type {
		case "think":
			// Don't spam edits for think — just update iteration count
			progressState.mu.Lock()
			progressState.iteration = ev.Iteration
			count := len(progressState.toolHistory)
			progressState.mu.Unlock()
			if count > 0 {
				gw.updateProgressMessage(taskCtx, progressState, fmt.Sprintf("🤔 Thinking (iter %d) · %d tools done", ev.Iteration, count))
			} else {
				gw.updateProgressMessage(taskCtx, progressState, fmt.Sprintf("🤔 Thinking (iter %d)...", ev.Iteration))
			}

		case "act":
			// Show what the tool is actually doing — file path, command, etc.
			progressState.mu.Lock()
			progressState.currentTool = ev.Tool
			progressState.currentParams = ev.ToolParams
			progressState.mu.Unlock()

			icon := toolIcon(ev.Tool)
			detail := shortToolDetail(ev.Tool, ev.ToolParams)
			var msg string
			if detail != "" {
				msg = fmt.Sprintf("⏳ %s %s", icon, detail)
			} else {
				msg = fmt.Sprintf("⏳ %s %s...", icon, ev.Tool)
			}
			gw.updateProgressMessage(taskCtx, progressState, msg)

		case "observe":
			// Record result and update progress
			progressState.mu.Lock()
			entry := toolEntry{
				tool:       ev.Tool,
				params:     progressState.currentParams,
				success:    ev.Success,
				durationMs: ev.DurationMs,
				result:     ev.Result,
			}
			progressState.toolHistory = append(progressState.toolHistory, entry)
			count := len(progressState.toolHistory)
			passed := 0
			for _, h := range progressState.toolHistory {
				if h.success {
					passed++
				}
			}
			currentDetail := shortToolDetail(ev.Tool, progressState.currentParams)
			progressState.mu.Unlock()

			// Build: "✅ 📄 write style.css · 3/3 ok"
			// Or:    "❌ ⚡ npm run build · 2/3 ok"
			statusIcon := "✅"
			if !ev.Success {
				statusIcon = "❌"
			}
			var msg string
			if currentDetail != "" {
				msg = fmt.Sprintf("%s %s %s · %d/%d ok",
					statusIcon, toolIcon(ev.Tool), currentDetail, passed, count)
			} else {
				msg = fmt.Sprintf("%s %s · %d/%d ok",
					statusIcon, toolIcon(ev.Tool), passed, count)
			}
			gw.updateProgressMessage(taskCtx, progressState, msg)

		case "done":
			// Task complete — show finishing message before deletion
			progressState.mu.Lock()
			count := len(progressState.toolHistory)
			progressState.mu.Unlock()
			if count > 0 {
				gw.updateProgressMessage(taskCtx, progressState, fmt.Sprintf("📝 Finishing up... %d tools completed", count))
			}
		}
	}

	taskCtx = agent.WithProgress(ctx, progressFn)

	// Wire approval system — severity-based, with session whitelist
	approvalFn := func(toolName, argsJSON string) (bool, error) {
		// Trust mode: auto-approve everything
		if gw.store.isTrusted(chatID) {
			return true, nil
		}

		severity := assessSeverity(toolName, argsJSON)

		// LOW: auto-approve silently
		if severity == "low" {
			return true, nil
		}

		// MEDIUM: check session whitelist first
		if severity == "medium" && gw.store.isWhitelisted(chatID, toolName) {
			return true, nil
		}

		// CRITICAL or un-whitelisted MEDIUM: ask the user
		summary := summarizeToolForApproval(toolName, argsJSON)
		return gw.requestApproval(taskCtx, chatID, toolName, summary, severity), nil
	}
	taskCtx = agent.WithApproval(taskCtx, approvalFn)

	// Add a task-level timeout so the agent can't hang forever
	const taskTimeout = 5 * time.Minute
	timeoutCtx, timeoutCancel := context.WithTimeout(taskCtx, taskTimeout)
	defer timeoutCancel()
	taskCtx = timeoutCtx

	taskStart := time.Now()

	// Background ticker to update progress message with elapsed time
	// so the user knows the bot is still alive during long LLM calls
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-typingCtx.Done():
				return
			case <-ticker.C:
				elapsed := time.Since(taskStart).Round(time.Second)
				progressState.mu.Lock()
				count := len(progressState.toolHistory)
				progressState.mu.Unlock()
				if count > 0 {
					gw.updateProgressMessage(taskCtx, progressState, fmt.Sprintf(
						"⏳ Still working... %d tools done · %s elapsed",
						count, elapsed))
				}
			}
		}
	}()

	result, err := gw.orch.ExecuteRich(taskCtx, taskWithCtx, role)
	taskDuration := time.Since(taskStart)
	typingCancel()

	// Edit the progress message to show clear completion — DON'T delete it
	// This gives visual separation: [progress msg = DONE] then [new msg = result]
	if statusMsgID != 0 {
		var doneSummary string
		if err != nil {
			if taskCtx.Err() == context.DeadlineExceeded {
				doneSummary = fmt.Sprintf("⏰ Timed out · %s", formatDuration(taskDuration))
			} else {
				doneSummary = fmt.Sprintf("❌ Failed · %s", formatDuration(taskDuration))
			}
		} else {
			progressState.mu.Lock()
			count := len(progressState.toolHistory)
			passed := 0
			for _, h := range progressState.toolHistory {
				if h.success {
					passed++
				}
			}
			progressState.mu.Unlock()
			if count > 0 && passed < count {
				doneSummary = fmt.Sprintf("⚠️ Done with errors · %d/%d tools ok · %s",
					passed, count, formatDuration(taskDuration))
			} else {
				doneSummary = fmt.Sprintf("✅ Done! · %s", formatDuration(taskDuration))
			}
		}
		gw.tg.editMessageText(ctx, chatID, statusMsgID, fmt.Sprintf("%s %s · %s", roleEmoji, role, doneSummary))
	}

	if err != nil {
		gw.logger.Error("task execution failed", "error", err)
		gw.activity.Record(ActivityEntry{
			Timestamp: time.Now(),
			Task:      text,
			Role:      role,
			Success:   false,
			Duration:  taskDuration,
		})
		errSummary := err.Error()
		if len(errSummary) > 200 {
			errSummary = errSummary[:200]
		}
		// Check if it was a timeout
		if taskCtx.Err() == context.DeadlineExceeded {
			gw.sendMessageHTML(ctx, chatID,
				fmt.Sprintf("⏰ **Task timed out** after %s.\n\nThe task was too complex or the model was slow. Try breaking it into smaller steps.",
					formatDuration(taskDuration)), msg.MessageID)
			return
		}
		gw.sendMessageHTML(ctx, chatID, fmt.Sprintf("❌ **Task failed**\n\n`%s`\n\nThis may be a timeout or model error. Try rephrasing your request.", errSummary), msg.MessageID)
		return
	}

	// Store conversation exchange for future context
	gw.store.addHistory(chatID, text, result.Content)

	// Build clean response — progress message already shows ✅ Done!
	// so the result message just shows the content + footer
	// Use MARKDOWN syntax — sendMessageHTML converts it
	footer := formatCompactFooter(result, taskDuration)
	responseText := result.Content
	if footer != "" {
		responseText += "\n\n┈┈┈┈┈┈┈┈┈┈┈\n" + footer
	}

	// Record successful activity
	gw.activity.Record(ActivityEntry{
		Timestamp:  time.Now(),
		Task:       text,
		Role:       result.Role,
		Success:    true,
		ToolCalls:  result.ToolCalls,
		Duration:   taskDuration,
		TokensUsed: result.TokensUsed,
	})

	// Send response — split into chunks for Telegram's 4096 char limit
	// Use HTML parse mode so bold/italic/code render natively.
	// Use the handler ctx (not taskCtx, which typingCancel just canceled).
	chunks := splitMessage(responseText, 4000)
	for i, chunk := range chunks {
		if err := gw.sendMessageHTML(ctx, chatID, chunk, msg.MessageID); err != nil {
			gw.logger.Error("failed to send message to Telegram",
				"chat_id", chatID, "chunk", i, "error", err)
		}
	}

	gw.logger.Info("task completed",
		"chat_id", chatID,
		"role", result.Role,
		"tool_calls", result.ToolCalls,
		"iterations", result.Iterations,
		"duration", taskDuration,
		"response_len", len(result.Content),
		"chunks", len(chunks))
}

// handleCommand processes bot commands.
func (gw *TelegramGateway) handleCommand(ctx context.Context, chatID int64, msg *tgMessage) {
	// reply is a thin wrapper over gw.tg.sendMessage so each case stays a
	// one-liner; errors are logged (sendMessage already wraps them).
	reply := func(text string) {
		gw.tg.sendMessage(ctx, chatID, text, msg.MessageID)
	}

	text := strings.TrimSpace(msg.Text)
	parts := strings.Fields(text)
	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "/start":
		reply("Kyoci Agent v5\n\n" +
			"Your plug-and-play AI agent platform.\n\n" +
			"Commands:\n" +
			"/role [developer|sre|qa|pm|frontend] -- Switch role\n" +
			"/activity -- Recent task activity\n" +
			"/tools -- List available tools\n" +
			"/roles -- List available roles\n" +
			"/status -- System status\n" +
			"/help -- Show help\n\n" +
			"Just send any message and I'll process it!")

	case "/help":
		reply("Kyoci Agent v5 Help\n\n" +
			"Commands:\n" +
			"- /role developer -- Developer mode\n" +
			"- /role sre -- SRE mode\n" +
			"- /role qa -- QA mode\n" +
			"- /role pm -- PM mode\n" +
			"- /role frontend -- Frontend mode\n" +
			"- /activity -- Show recent task activity\n" +
			"- /tools -- List all tools\n" +
			"- /roles -- List all roles\n" +
			"- /status -- System status\n" +
			"- /trust -- Toggle auto-approve all (skip ALL prompts)\n" +
			"- /whitelist -- Show whitelisted tools this session\n" +
			"- /clearwl -- Clear session whitelist\n" +
			"- /reset -- Reset to auto-detect\n\n" +
			"Usage:\n" +
			"Just type your task and I'll handle it.\n" +
			"Role auto-detects based on your message content.\n" +
			"Every response shows what tools were used.")

	case "/role":
		if len(args) == 0 {
			current := gw.store.getChatRole(chatID)
			if current == "" {
				current = "auto-detect"
			}
			reply(fmt.Sprintf("Current role: %s\n\nUse /role [developer|sre|qa|pm|frontend] to switch.", current))
			return
		}
		role := strings.ToLower(args[0])
		validRoles := map[string]string{
			"developer": "developer", "dev": "developer",
			"sre": "sre", "ops": "sre",
			"qa": "qa", "test": "qa",
			"pm": "pm", "manager": "pm",
			"frontend": "frontend", "ui": "frontend", "ux": "frontend",
			"auto": "", "reset": "",
		}
		if mapped, ok := validRoles[role]; ok {
			gw.store.setChatRole(chatID, mapped)
			if mapped == "" {
				reply("Role reset to auto-detect")
			} else {
				reply(fmt.Sprintf("Role set to %s", mapped))
			}
		} else {
			reply("Invalid role. Use: developer, sre, qa, pm, frontend")
		}

	case "/reset":
		gw.store.setChatRole(chatID, "")
		gw.store.clearHistory(chatID)
		reply("Role reset to auto-detect. Conversation history cleared.")

	case "/activity":
		reply(gw.activity.FormatRecent(10))

	case "/tools":
		_, _, toolCount, _ := gw.orch.Status()
		toolNames := []string{
			"terminal -- Run shell commands",
			"file -- Read/write files",
			"browser -- Open/fetch web pages",
			"docs -- Fetch library docs (React, CSS, TS, etc.)",
			"http_client -- Raw HTTP requests",
			"web_search -- Search the web",
			"calculator -- Math calculations",
			"todo -- Manage task lists",
			"skill -- Save/load reusable procedures",
			"process -- Background process management",
			"delegation -- Spawn sub-agents for parallel work",
			"memory_recall -- Search memories",
			"remember -- Store facts/preferences",
		}
		toolsMsg := fmt.Sprintf("Available Tools (%d registered):\n\n", toolCount)
		for _, t := range toolNames {
			toolsMsg += "- " + t + "\n"
		}
		reply(toolsMsg)

	case "/roles":
		_, roleCount, _, _ := gw.orch.Status()
		rolesMsg := fmt.Sprintf("Available Roles (%d):\n\n", roleCount)
		rolesList := []struct {
			name string
			desc string
		}{
			{"developer", "Code, files, scripts, general programming"},
			{"sre", "System admin, monitoring, infrastructure"},
			{"qa", "Testing, review, security validation"},
			{"pm", "Planning, timelines, project management"},
			{"frontend", "UI/UX, HTML/CSS/React/TypeScript"},
		}
		for _, r := range rolesList {
			emoji := map[string]string{
				"developer": "👨‍💻", "sre": "🛡️", "qa": "🧪",
				"pm": "📋", "frontend": "🎨",
			}[r.name]
			rolesMsg += fmt.Sprintf("%s %s -- %s\n", emoji, r.name, r.desc)
		}
		rolesMsg += "\nRole auto-detects from your message.\nUse /role <name> to force a role."
		reply(rolesMsg)

	case "/status":
		providers, roles, tools, skills := gw.orch.Status()
		// Add activity stats
		total, success, avgDur, totalTools := gw.activity.Stats()
		statusMsg := fmt.Sprintf(
			"Kyoci Agent v5 Status\n\n"+
				"- Providers: %d (%s)\n"+
				"- Roles: %d\n"+
				"- Tools: %d\n"+
				"- Skills: %d\n"+
				"- Status: Running\n\n"+
				"Activity (this session):\n"+
				"- Total tasks: %d\n"+
				"- Success rate: %.0f%%\n"+
				"- Avg duration: %s\n"+
				"- Total tool calls: %d",
			len(providers), strings.Join(providers, ", "),
			roles, tools, skills,
			total, float64(success)/float64(max(1, total))*100,
			formatDuration(avgDur), totalTools)
		reply(statusMsg)

	case "/trust":
		if gw.store.isTrusted(chatID) {
			gw.store.setTrusted(chatID, false)
			reply("🔒 Trust mode OFF. Severity-based approvals active.")
		} else {
			gw.store.setTrusted(chatID, true)
			reply("🔓 Trust mode ON. All tools auto-approved. Use /trust again to disable.")
		}

	case "/whitelist":
		tools := gw.store.whitelistedTools(chatID)
		if len(tools) == 0 {
			reply("📋 No tools whitelisted this session.\n\nWhen you click ✅✅ on a 🟡 MEDIUM approval, that tool gets whitelisted — no more prompts for it until restart.")
		} else {
			for i, t := range tools {
				tools[i] = "• " + t
			}
			sort.Strings(tools)
			reply(fmt.Sprintf("📋 <b>Whitelisted tools this session:</b>\n%s\n\nThese tools auto-approve without asking. /clearwl to clear.", strings.Join(tools, "\n")))
		}

	case "/clearwl":
		gw.store.clearWhitelist(chatID)
		reply("🔄 Session whitelist cleared. All tools will ask for approval again.")

	default:
		reply("Unknown command. Use /help to see available commands.")
	}
}

// ── Rich Progress System ───────────────────────────────────────

// progressTracker tracks state for editing a single status message.
type progressTracker struct {
	chatID        int64
	statusMsgID   int64
	roleEmoji     string // e.g. "🛡️"
	roleName      string // e.g. "sre"
	iteration     int
	currentTool   string
	currentParams string
	toolHistory   []toolEntry
	lastUpdateAt  time.Time
	mu            sync.Mutex
}

// toolEntry records a completed tool call for the progress summary.
type toolEntry struct {
	tool       string
	params     string // short summary of what was done (file path, command, etc.)
	success    bool
	durationMs int64
	result     string
}

// shortToolDetail formats tool params into a compact, readable detail string.
// Shortens file paths to last 2 segments, truncates long commands.
func shortToolDetail(toolName, params string) string {
	if params == "" {
		return ""
	}

	switch toolName {
	case "file":
		// params format: "action /full/path/to/file.ext"
		// Shorten to: "action ...path/to/file.ext"
		parts := strings.SplitN(params, " ", 2)
		if len(parts) < 2 {
			return params
		}
		action := parts[0]
		path := parts[1]
		return action + " " + shortPath(path)

	case "terminal":
		// params is the raw command — show first meaningful part
		cmd := strings.TrimSpace(params)
		// Multi-line? show just first line
		if idx := strings.IndexByte(cmd, '\n'); idx > 0 {
			cmd = cmd[:idx]
		}
		if len(cmd) > 60 {
			cmd = cmd[:60] + "…"
		}
		return cmd

	case "browser":
		return params // "navigate https://..." — already short enough

	case "web_search":
		return params // "search: query"

	case "http_client":
		return params // "GET https://..."

	case "delegation":
		return params // "delegate: goal text"

	case "skill":
		return params // "load skill-name"

	case "security_scan":
		// "scan: /path/to/project" — shorten path
		parts := strings.SplitN(params, ": ", 2)
		if len(parts) == 2 {
			return parts[0] + ": " + shortPath(parts[1])
		}
		return params

	default:
		if len(params) > 60 {
			return params[:60] + "…"
		}
		return params
	}
}

// shortPath reduces a long file path to its last 2 segments.
// "/Users/nicholas/projects/app/src/style.css" → "…app/src/style.css"
func shortPath(path string) string {
	path = strings.TrimSpace(path)
	segs := strings.Split(path, "/")
	if len(segs) <= 3 {
		return path
	}
	// Keep last 3 segments for context
	return "…" + strings.Join(segs[len(segs)-3:], "/")
}

// roleIconForRole returns emoji for a role name.
func roleIconForRole(role string) string {
	icons := map[string]string{
		"developer": "👨‍💻",
		"sre":       "🛡️",
		"qa":        "🧪",
		"pm":        "📋",
		"frontend":  "🎨",
		"custom":    "🤖",
	}
	if icon, ok := icons[role]; ok {
		return icon
	}
	return "🤖"
}

// toolIcon returns the emoji for a tool name.
func toolIcon(tool string) string {
	icons := map[string]string{
		"terminal":      "⚡",
		"file":          "📄",
		"browser":       "🌐",
		"docs":          "📚",
		"http_client":   "🔗",
		"web_search":    "🔍",
		"calculator":    "🧮",
		"todo":          "📝",
		"skill":         "💡",
		"process":       "⚙️",
		"memory_recall": "🧠",
		"remember":      "💾",
		"delegation":    "🤖",
		"security_scan": "🛡️",
	}
	if icon, ok := icons[tool]; ok {
		return icon
	}
	return "🔧"
}

// ============================================================
// Gateway-level helpers (Bot API wrappers + approval flow)
// ============================================================
//
// These sit above the tgClient SDK: they translate gateway concerns
// (markdown→HTML, throttled progress edits, inline-keyboard approvals) into
// the raw Bot API calls exposed by gw.tg. Stateful per-chat decisions route
// through gw.store.

// sendStatusMessage sends the initial status message and returns its message ID.
func (gw *TelegramGateway) sendStatusMessage(ctx context.Context, chatID int64, text string) int64 {
	msgID, err := gw.tg.sendMessageWithID(ctx, chatID, text, 0)
	if err != nil {
		gw.logger.Error("failed to send status message", "error", err)
		return 0
	}
	return msgID
}

// updateProgressMessage edits the status message with new text.
// Throttled to max 1 edit per 1.2 seconds to avoid Telegram rate limits.
// Every message is automatically prefixed with the agent identity.
func (gw *TelegramGateway) updateProgressMessage(ctx context.Context, pt *progressTracker, newMsg string) {
	pt.mu.Lock()
	// Throttle: don't edit more than once per 1.2s
	if time.Since(pt.lastUpdateAt) < 1200*time.Millisecond {
		pt.mu.Unlock()
		return
	}
	pt.lastUpdateAt = time.Now()
	msgID := pt.statusMsgID
	chatID := pt.chatID
	// Prefix with agent identity so user always knows which agent is working
	fullText := pt.roleEmoji + " " + pt.roleName + " · " + newMsg
	pt.mu.Unlock()

	if msgID == 0 {
		return
	}

	if len(fullText) > 800 {
		fullText = fullText[:800]
	}

	gw.tg.editMessageText(ctx, chatID, msgID, fullText)
}

// ── Approval flow ───────────────────────────────────────────────

// requestApproval sends an inline keyboard to the user and blocks until they
// respond. Returns the user's approve/deny decision.
func (gw *TelegramGateway) requestApproval(ctx context.Context, chatID int64, toolName, summary, severity string) bool {
	gw.approvalMu.Lock()
	approvalID := fmt.Sprintf("appr_%d_%d", chatID, time.Now().UnixNano())
	ch := make(chan approvalResponse, 1)
	gw.pendingApprovals[approvalID] = ch
	gw.approvalMu.Unlock()

	defer func() {
		gw.approvalMu.Lock()
		delete(gw.pendingApprovals, approvalID)
		gw.approvalMu.Unlock()
	}()

	// Build severity-specific UI
	var sevEmoji, sevLabel, sevColor string
	switch severity {
	case "critical":
		sevEmoji = "🔴"
		sevLabel = "CRITICAL"
		sevColor = "irreversible — always requires approval"
	case "medium":
		sevEmoji = "🟡"
		sevLabel = "MEDIUM"
		sevColor = "write operation — approve once to whitelist"
	default:
		sevEmoji = "🟢"
		sevLabel = "LOW"
		sevColor = "safe"
	}

	icon := toolIcon(toolName)
	text := fmt.Sprintf("%s <b>%s — %s</b>\n<i>%s</i>\n\n%s <b>%s</b>\n<blockquote>%s</blockquote>",
		sevEmoji, sevLabel, toolName, sevColor,
		icon, toolName, htmlEscape(truncateForTG(summary, 300)))

	params := map[string]string{
		"chat_id":    fmt.Sprintf("%d", chatID),
		"text":       text,
		"parse_mode": "HTML",
	}

	// Buttons differ by severity
	var keyboard string
	if severity == "critical" {
		// Critical: just approve/deny, no whitelist option
		keyboard = fmt.Sprintf(
			`{"inline_keyboard":[[{"text":"✅ Allow once","callback_data":"%s:yes"},{"text":"❌ Deny","callback_data":"%s:no"}]]}`,
			approvalID, approvalID)
	} else {
		// Medium: allow once + allow all (whitelist)
		keyboard = fmt.Sprintf(
			`{"inline_keyboard":[[{"text":"✅ Allow","callback_data":"%s:yes"},{"text":"✅✅ Allow all %s","callback_data":"%s:whitelist"},{"text":"❌ Deny","callback_data":"%s:no"}]]}`,
			approvalID, toolName, approvalID, approvalID)
	}
	params["reply_markup"] = keyboard

	if _, err := gw.tg.apiCall(ctx, "sendMessage", params); err != nil {
		gw.logger.Error("failed to send approval message", "error", err)
		return true // Fail open
	}

	gw.logger.Info("approval requested", "approval_id", approvalID, "tool", toolName, "severity", severity, "chat_id", chatID)

	select {
	case resp := <-ch:
		if resp.whitelist && resp.approved {
			gw.store.whitelistTool(chatID, toolName)
			gw.logger.Info("tool whitelisted for session", "tool", toolName, "chat_id", chatID)
		}
		return resp.approved
	case <-time.After(120 * time.Second):
		gw.tg.sendMessage(ctx, chatID, "⏰ Approval timed out (120s). Tool skipped.", 0)
		return false
	}
}

// handleCallbackQuery processes inline keyboard button presses.
func (gw *TelegramGateway) handleCallbackQuery(ctx context.Context, cq *tgCallbackQuery) {
	if cq == nil || cq.Data == "" {
		return
	}

	// Answer the callback query first (removes loading state on button)
	gw.tg.answerCallbackQuery(ctx, cq.ID, "")

	// Parse callback data: "appr_<chatID>_<nano>:yes" / ":no" / ":whitelist" / ":trust"
	parts := strings.SplitN(cq.Data, ":", 2)
	if len(parts) != 2 {
		return
	}

	approvalID := parts[0]
	action := parts[1]

	gw.approvalMu.Lock()
	ch, exists := gw.pendingApprovals[approvalID]
	gw.approvalMu.Unlock()

	if !exists {
		gw.logger.Warn("callback query for unknown approval", "approval_id", approvalID)
		return
	}

	// Build response for the waiting goroutine
	resp := approvalResponse{approved: false, whitelist: false}
	switch action {
	case "yes":
		resp.approved = true
	case "whitelist":
		resp.approved = true
		resp.whitelist = true
	case "trust":
		resp.approved = true
		// Enable trust mode for this chat
		if cq.Message != nil {
			gw.store.setTrusted(cq.Message.Chat.ID, true)
		}
	default:
		resp.approved = false
	}

	// Update the approval message to show the decision
	if cq.Message != nil {
		var statusText string
		switch action {
		case "yes":
			statusText = "✅ <b>Approved</b>"
		case "whitelist":
			statusText = "✅✅ <b>Approved + Whitelisted</b>\nThis tool won't ask again this session."
		case "no":
			statusText = "❌ <b>Denied</b>"
		case "trust":
			statusText = "🔓 <b>Trust Mode Enabled</b>\nAll tools auto-approved until /untrust."
		default:
			statusText = "❓ Unknown"
		}
		params := map[string]string{
			"chat_id":    fmt.Sprintf("%d", cq.Message.Chat.ID),
			"message_id": fmt.Sprintf("%d", cq.Message.MessageID),
			"text":       statusText,
			"parse_mode": "HTML",
		}
		gw.tg.apiCall(ctx, "editMessageText", params)
	}

	// Send result to the waiting goroutine
	select {
	case ch <- resp:
	default:
	}

	gw.logger.Info("approval response", "approval_id", approvalID, "action", action, "approved", resp.approved, "whitelist", resp.whitelist)
}

// sendMessageHTML sends a message using HTML parse mode.
func (gw *TelegramGateway) sendMessageHTML(ctx context.Context, chatID int64, text string, replyTo int64) error {
	// Convert markdown to HTML
	htmlText := markdownToHTML(text)

	chunks := splitMessage(htmlText, 4000)
	for _, chunk := range chunks {
		params := map[string]string{
			"chat_id":    strconv.FormatInt(chatID, 10),
			"text":       chunk,
			"parse_mode": "HTML",
		}
		if replyTo > 0 {
			params["reply_to_message_id"] = strconv.FormatInt(replyTo, 10)
		}
		if _, err := gw.tg.apiCall(ctx, "sendMessage", params); err != nil {
			// Fallback: send plain text without HTML if parsing fails
			gw.logger.Warn("HTML sendMessage failed, falling back to plain text", "error", err)
			plainChunks := splitMessage(text, 4000)
			for _, pc := range plainChunks {
				gw.tg.sendMessage(ctx, chatID, pc, replyTo)
			}
			return nil
		}
	}
	return nil
}

// kyociParseArgs is a local helper to parse JSON args (avoids import cycle).
func kyociParseArgs(argsJSON string) map[string]interface{} {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil
	}
	return args
}

// summarizeToolForApproval creates a human-readable summary for the approval prompt.
func summarizeToolForApproval(toolName, argsJSON string) string {
	args := kyociParseArgs(argsJSON)
	if args == nil {
		return toolName
	}
	switch toolName {
	case "terminal":
		if cmd, ok := args["command"].(string); ok {
			return cmd
		}
	case "file":
		action, _ := args["action"].(string)
		path, _ := args["path"].(string)
		return action + " " + path
	case "delegation":
		if goal, ok := args["goal"].(string); ok {
			return goal
		}
	case "security_scan":
		if p, ok := args["path"].(string); ok {
			return "scan: " + p
		}
	}
	return toolName
}
