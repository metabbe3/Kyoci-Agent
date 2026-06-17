package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/agent"
)

// TelegramGateway connects Kyoci Agent to Telegram via Bot API long-polling.
// Goroutine-safe: handles multiple concurrent chats.
type TelegramGateway struct {
	token      string
	apiBase    string
	orch       OrchestratorClient
	logger     *slog.Logger
	httpClient *http.Client

	// Polling state
	offset    int64
	started   bool
	mu        sync.RWMutex

	// Per-chat state: tracks which users are in which role mode
	chatRoles  map[int64]string
	chatMu     sync.RWMutex

	// Per-chat conversation history for context
	chatHistory map[int64][]convTurn

	// Rate limiting per chat: prevents spam
	lastMsg    map[int64]time.Time
	rateMu     sync.Mutex

	// Allowed chat IDs (empty = allow all)
	allowedChats map[int64]bool

	// Activity tracker — logs recent task activity for /activity command
	activity *ActivityTracker

	// Pending approvals: maps callback data prefix to approval channel
	pendingApprovals map[string]chan approvalResponse
	approvalMu       sync.Mutex

	// Trust mode: when enabled, auto-approves all tools for that chat
	trustedChats map[int64]bool

	// Session whitelist: per-chat, tool names that were approved once → auto-pass
	sessionWhitelist map[int64]map[string]bool
	whitelistMu      sync.Mutex
}

// approvalResponse carries the user's decision from callback to the waiting goroutine.
type approvalResponse struct {
	approved   bool
	whitelist  bool // if true, add this tool to session whitelist
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

// convTurn stores one user-assistant exchange for conversation history.
type convTurn struct {
	user      string
	assistant string
}

// Telegram API types — only what we need.

type tgUpdate struct {
	UpdateID      int64            `json:"update_id"`
	Message       *tgMessage       `json:"message"`
	CallbackQuery *tgCallbackQuery `json:"callback_query"`
}

type tgCallbackQuery struct {
	ID      string     `json:"id"`
	From    *tgUser    `json:"from"`
	Data    string     `json:"data"`
	Message *tgMessage `json:"message"`
}

type tgMessage struct {
	MessageID int64  `json:"message_id"`
	From      *tgUser `json:"from"`
	Chat      tgChat  `json:"chat"`
	Text      string  `json:"text"`
}

type tgUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

type tgChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type tgAPIResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
}

type tgSendMessageRequest struct {
	ChatID    int64  `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
	ReplyTo   int64  `json:"reply_to_message_id,omitempty"`
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
		token:       cfg.Token,
		apiBase:     fmt.Sprintf("https://api.telegram.org/bot%s", cfg.Token),
		orch:        orch,
		logger:      logger.With("component", "telegram-gateway"),
		httpClient:  &http.Client{Timeout: time.Duration(pollTimeout+10) * time.Second},
		chatRoles:   make(map[int64]string),
		chatHistory: make(map[int64][]convTurn),
		lastMsg:     make(map[int64]time.Time),
		allowedChats: make(map[int64]bool),
		activity:          NewActivityTracker(50),
		pendingApprovals:  make(map[string]chan approvalResponse),
		trustedChats:      make(map[int64]bool),
		sessionWhitelist:  make(map[int64]map[string]bool),
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
	botInfo, err := gw.getMe()
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

		updates, err := gw.getUpdates(ctx)
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
	if !gw.checkRate(userID) {
		gw.logger.Warn("rate limited", "user_id", userID)
		return
	}

	// Access control
	if len(gw.allowedChats) > 0 && !gw.allowedChats[userID] {
		gw.logger.Warn("unauthorized user", "user_id", userID, "username", msg.From.Username)
		gw.sendMessage(chatID, "You are not authorized to use this bot.", msg.MessageID)
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
	role := gw.getChatRole(chatID)

	// Build task with conversation context
	taskWithCtx := gw.buildTaskWithContext(chatID, text)

	// Send initial status message that we'll EDIT as progress comes in
	roleEmoji := roleIconForRole(role)
	taskPreview := text
	if len(taskPreview) > 60 {
		taskPreview = taskPreview[:60] + "..."
	}
	statusMsgID := gw.sendStatusMessage(chatID, fmt.Sprintf("🚀 %s %s — %q",
		roleEmoji, role, taskPreview))

	// Track progress state for editing
	progressState := &progressTracker{
		chatID:        chatID,
		statusMsgID:   statusMsgID,
		roleEmoji:     roleEmoji,
		roleName:      role,
		iteration:     0,
		toolHistory:   []toolEntry{},
		lastUpdateAt:  time.Now(),
	}

	// Send periodic "typing" indicators while task runs
	typingCtx, typingCancel := context.WithCancel(ctx)
	defer typingCancel()
	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		gw.sendChatAction(chatID, "typing")
		for {
			select {
			case <-typingCtx.Done():
				return
			case <-ticker.C:
				gw.sendChatAction(chatID, "typing")
			}
		}
	}()

	// Wire progress streaming — edit the status message with rich detail
	progressFn := func(ev agent.ProgressEvent) {
		switch ev.Type {
		case "think":
			// Don't spam edits for think — just update iteration count
			progressState.mu.Lock()
			progressState.iteration = ev.Iteration
			count := len(progressState.toolHistory)
			progressState.mu.Unlock()
			if count > 0 {
				gw.updateProgressMessage(progressState, fmt.Sprintf("🤔 Thinking (iter %d) · %d tools done", ev.Iteration, count))
			} else {
				gw.updateProgressMessage(progressState, fmt.Sprintf("🤔 Thinking (iter %d)...", ev.Iteration))
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
			gw.updateProgressMessage(progressState, msg)

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
			gw.updateProgressMessage(progressState, msg)

		case "done":
			// Task complete — show finishing message before deletion
			progressState.mu.Lock()
			count := len(progressState.toolHistory)
			progressState.mu.Unlock()
			if count > 0 {
				gw.updateProgressMessage(progressState, fmt.Sprintf("📝 Finishing up... %d tools completed", count))
			}
		}
	}

	taskCtx := agent.WithProgress(ctx, progressFn)

	// Wire approval system — severity-based, with session whitelist
	approvalFn := func(toolName, argsJSON string) (bool, error) {
		// Trust mode: auto-approve everything
		if gw.trustedChats[chatID] {
			return true, nil
		}

		severity := assessSeverity(toolName, argsJSON)

		// LOW: auto-approve silently
		if severity == "low" {
			return true, nil
		}

		// MEDIUM: check session whitelist first
		if severity == "medium" && gw.isWhitelisted(chatID, toolName) {
			return true, nil
		}

		// CRITICAL or un-whitelisted MEDIUM: ask the user
		summary := summarizeToolForApproval(toolName, argsJSON)
		return gw.requestApproval(chatID, toolName, summary, severity), nil
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
					gw.updateProgressMessage(progressState, fmt.Sprintf(
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
		gw.editMessageText(chatID, statusMsgID, fmt.Sprintf("%s %s · %s", roleEmoji, role, doneSummary))
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
			gw.sendMessageHTML(chatID,
				fmt.Sprintf("⏰ **Task timed out** after %s.\n\nThe task was too complex or the model was slow. Try breaking it into smaller steps.",
					formatDuration(taskDuration)), msg.MessageID)
			return
		}
		gw.sendMessageHTML(chatID, fmt.Sprintf("❌ **Task failed**\n\n`%s`\n\nThis may be a timeout or model error. Try rephrasing your request.", errSummary), msg.MessageID)
		return
	}

	// Store conversation exchange for future context
	gw.addHistory(chatID, text, result.Content)

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
	// Use HTML parse mode so bold/italic/code render natively
	chunks := splitMessage(responseText, 4000)
	for i, chunk := range chunks {
		if err := gw.sendMessageHTML(chatID, chunk, msg.MessageID); err != nil {
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
	text := strings.TrimSpace(msg.Text)
	parts := strings.Fields(text)
	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "/start":
		gw.sendMessage(chatID,
			"Kyoci Agent v5\n\n"+
				"Your plug-and-play AI agent platform.\n\n"+
				"Commands:\n"+
				"/role [developer|sre|qa|pm|frontend] -- Switch role\n"+
				"/activity -- Recent task activity\n"+
				"/tools -- List available tools\n"+
				"/roles -- List available roles\n"+
				"/status -- System status\n"+
				"/help -- Show help\n\n"+
				"Just send any message and I'll process it!",
			msg.MessageID)

	case "/help":
		gw.sendMessage(chatID,
			"Kyoci Agent v5 Help\n\n"+
				"Commands:\n"+
				"- /role developer -- Developer mode\n"+
				"- /role sre -- SRE mode\n"+
				"- /role qa -- QA mode\n"+
				"- /role pm -- PM mode\n"+
				"- /role frontend -- Frontend mode\n"+
				"- /activity -- Show recent task activity\n"+
				"- /tools -- List all tools\n"+
				"- /roles -- List all roles\n"+
				"- /status -- System status\n"+
				"- /trust -- Toggle auto-approve all (skip ALL prompts)\n"+
				"- /whitelist -- Show whitelisted tools this session\n"+
				"- /clearwl -- Clear session whitelist\n"+
				"- /reset -- Reset to auto-detect\n\n"+
				"Usage:\n"+
				"Just type your task and I'll handle it.\n"+
				"Role auto-detects based on your message content.\n"+
				"Every response shows what tools were used.",
			msg.MessageID)

	case "/role":
		if len(args) == 0 {
			current := gw.getChatRole(chatID)
			if current == "" {
				current = "auto-detect"
			}
			gw.sendMessage(chatID, fmt.Sprintf("Current role: %s\n\nUse /role [developer|sre|qa|pm|frontend] to switch.", current), msg.MessageID)
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
			gw.setChatRole(chatID, mapped)
			if mapped == "" {
				gw.sendMessage(chatID, "Role reset to auto-detect", msg.MessageID)
			} else {
				gw.sendMessage(chatID, fmt.Sprintf("Role set to %s", mapped), msg.MessageID)
			}
		} else {
			gw.sendMessage(chatID, "Invalid role. Use: developer, sre, qa, pm, frontend", msg.MessageID)
		}

	case "/reset":
		gw.setChatRole(chatID, "")
		gw.clearHistory(chatID)
		gw.sendMessage(chatID, "Role reset to auto-detect. Conversation history cleared.", msg.MessageID)

	case "/activity":
		actMsg := gw.activity.FormatRecent(10)
		gw.sendMessage(chatID, actMsg, msg.MessageID)

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
		gw.sendMessage(chatID, toolsMsg, msg.MessageID)

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
		gw.sendMessage(chatID, rolesMsg, msg.MessageID)

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
		gw.sendMessage(chatID, statusMsg, msg.MessageID)

	case "/trust":
		if gw.trustedChats[chatID] {
			delete(gw.trustedChats, chatID)
			gw.sendMessage(chatID, "🔒 Trust mode OFF. Severity-based approvals active.", msg.MessageID)
		} else {
			gw.trustedChats[chatID] = true
			gw.sendMessage(chatID, "🔓 Trust mode ON. All tools auto-approved. Use /trust again to disable.", msg.MessageID)
		}

	case "/whitelist":
		gw.whitelistMu.Lock()
		wl := gw.sessionWhitelist[chatID]
		gw.whitelistMu.Unlock()
		if len(wl) == 0 {
			gw.sendMessage(chatID, "📋 No tools whitelisted this session.\n\nWhen you click ✅✅ on a 🟡 MEDIUM approval, that tool gets whitelisted — no more prompts for it until restart.", msg.MessageID)
		} else {
			var tools []string
			for t := range wl {
				tools = append(tools, "• "+t)
			}
			sort.Strings(tools)
			gw.sendMessage(chatID, fmt.Sprintf("📋 <b>Whitelisted tools this session:</b>\n%s\n\nThese tools auto-approve without asking. /clearwl to clear.", strings.Join(tools, "\n")), msg.MessageID)
		}

	case "/clearwl":
		gw.whitelistMu.Lock()
		gw.sessionWhitelist[chatID] = make(map[string]bool)
		gw.whitelistMu.Unlock()
		gw.sendMessage(chatID, "🔄 Session whitelist cleared. All tools will ask for approval again.", msg.MessageID)

	default:
		gw.sendMessage(chatID, "Unknown command. Use /help to see available commands.", msg.MessageID)
	}
}

// ── Telegram Bot API Calls ─────────────────────────────────────

// getMe verifies the bot token and returns bot info.
func (gw *TelegramGateway) getMe() (*tgUser, error) {
	resp, err := gw.apiCall("getMe", nil)
	if err != nil {
		return nil, err
	}
	var user tgUser
	if err := json.Unmarshal(resp, &user); err != nil {
		return nil, fmt.Errorf("failed to parse getMe response: %w", err)
	}
	return &user, nil
}

// getUpdates fetches pending updates via long-polling.
func (gw *TelegramGateway) getUpdates(ctx context.Context) ([]tgUpdate, error) {
	params := map[string]string{
		"timeout": "30",
	}
	if gw.offset > 0 {
		params["offset"] = strconv.FormatInt(gw.offset, 10)
	}

	resp, err := gw.apiCallWithCtx(ctx, "getUpdates", params)
	if err != nil {
		return nil, err
	}

	var updates []tgUpdate
	if err := json.Unmarshal(resp, &updates); err != nil {
		return nil, fmt.Errorf("failed to parse updates: %w", err)
	}
	return updates, nil
}

// sendMessage sends a text message to a chat.
func (gw *TelegramGateway) sendMessage(chatID int64, text string, replyTo int64) error {
	req := tgSendMessageRequest{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "", // Empty = plain text, avoids Markdown parse failures
		ReplyTo:   replyTo,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	apiURL := fmt.Sprintf("%s/sendMessage", gw.apiBase)
	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := gw.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("sendMessage failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// sendChatAction sends a typing indicator.
func (gw *TelegramGateway) sendChatAction(chatID int64, action string) error {
	params := map[string]string{
		"chat_id": strconv.FormatInt(chatID, 10),
		"action":  action,
	}
	_, err := gw.apiCall("sendChatAction", params)
	return err
}

// apiCall makes a GET request to the Telegram Bot API.
func (gw *TelegramGateway) apiCall(method string, params map[string]string) (json.RawMessage, error) {
	return gw.apiCallWithCtx(context.Background(), method, params)
}

// apiCallWithCtx makes a GET request to the Telegram Bot API with context.
func (gw *TelegramGateway) apiCallWithCtx(ctx context.Context, method string, params map[string]string) (json.RawMessage, error) {
	apiURL := fmt.Sprintf("%s/%s", gw.apiBase, method)
	if len(params) > 0 {
		values := url.Values{}
		for k, v := range params {
			values.Set(k, v)
		}
		apiURL += "?" + values.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := gw.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API call %s failed: %w", method, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API %s returned status %d: %s", method, resp.StatusCode, string(body))
	}

	var apiResp tgAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("API %s error: %s", method, apiResp.Description)
	}

	return apiResp.Result, nil
}

// uploadFile uploads a file to Telegram (for future use).
func (gw *TelegramGateway) uploadFile(method string, fieldName string, fileName string, fileData []byte, params map[string]string) (json.RawMessage, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(fileData); err != nil {
		return nil, err
	}

	for k, v := range params {
		writer.WriteField(k, v)
	}
	writer.Close()

	apiURL := fmt.Sprintf("%s/%s", gw.apiBase, method)
	req, err := http.NewRequest("POST", apiURL, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := gw.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var apiResp tgAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("API %s error: %s", method, apiResp.Description)
	}

	return apiResp.Result, nil
}

// ── Chat State Management ──────────────────────────────────────

// maxHistoryTurns is how many previous exchanges to include as context.
const maxHistoryTurns = 4

func (gw *TelegramGateway) getChatRole(chatID int64) string {
	gw.chatMu.RLock()
	defer gw.chatMu.RUnlock()
	return gw.chatRoles[chatID]
}

func (gw *TelegramGateway) setChatRole(chatID int64, role string) {
	gw.chatMu.Lock()
	defer gw.chatMu.Unlock()
	gw.chatRoles[chatID] = role
}

// addHistory stores a user-assistant exchange and prunes old entries.
func (gw *TelegramGateway) addHistory(chatID int64, user, assistant string) {
	// Truncate assistant response to keep context manageable
	if len(assistant) > 500 {
		assistant = assistant[:500] + "..."
	}

	gw.chatMu.Lock()
	defer gw.chatMu.Unlock()

	gw.chatHistory[chatID] = append(gw.chatHistory[chatID], convTurn{
		user:      user,
		assistant: assistant,
	})

	// Keep only last maxHistoryTurns exchanges
	if len(gw.chatHistory[chatID]) > maxHistoryTurns {
		gw.chatHistory[chatID] = gw.chatHistory[chatID][len(gw.chatHistory[chatID])-maxHistoryTurns:]
	}
}

// clearHistory removes all conversation history for a chat.
func (gw *TelegramGateway) clearHistory(chatID int64) {
	gw.chatMu.Lock()
	defer gw.chatMu.Unlock()
	delete(gw.chatHistory, chatID)
}

// buildTaskWithContext prepends recent conversation history to the current task
// so the LLM can understand follow-up messages like "all of it" or "yes, do that".
func (gw *TelegramGateway) buildTaskWithContext(chatID int64, currentMessage string) string {
	gw.chatMu.RLock()
	history := gw.chatHistory[chatID]
	gw.chatMu.RUnlock()

	if len(history) == 0 {
		return currentMessage
	}

	// Keep only last 3 exchanges to avoid context bloat
	start := 0
	if len(history) > 3 {
		start = len(history) - 3
	}
	recent := history[start:]

	var sb strings.Builder
	sb.WriteString("[Previous conversation — these tasks are ALREADY COMPLETED. Do NOT redo them.]\n\n")
	for _, turn := range recent {
		sb.WriteString("User: ")
		sb.WriteString(turn.user)
		sb.WriteString("\n")
		// Truncate assistant response to key info
		assistantSummary := turn.assistant
		if len(assistantSummary) > 500 {
			assistantSummary = assistantSummary[:500] + "... [truncated]"
		}
		sb.WriteString("Assistant (DONE): ")
		sb.WriteString(assistantSummary)
		sb.WriteString("\n\n")
	}
	sb.WriteString("[End of previous context — above tasks are COMPLETE]\n\n")
	sb.WriteString("Current message: ")
	sb.WriteString(currentMessage)

	return sb.String()
}

func (gw *TelegramGateway) checkRate(userID int64) bool {
	gw.rateMu.Lock()
	defer gw.rateMu.Unlock()

	last, exists := gw.lastMsg[userID]
	if exists && time.Since(last) < 2*time.Second {
		return false
	}
	gw.lastMsg[userID] = time.Now()
	return true
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

// sendStatusMessage sends the initial status message and returns its message ID.
func (gw *TelegramGateway) sendStatusMessage(chatID int64, text string) int64 {
	msgID, err := gw.sendMessageWithID(chatID, text, 0)
	if err != nil {
		gw.logger.Error("failed to send status message", "error", err)
		return 0
	}
	return msgID
}

// updateProgressMessage edits the status message with new text.
// Throttled to max 1 edit per 1.2 seconds to avoid Telegram rate limits.
// Every message is automatically prefixed with the agent identity.
func (gw *TelegramGateway) updateProgressMessage(pt *progressTracker, newMsg string) {
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

	gw.editMessageText(chatID, msgID, fullText)
}
// sendMessageWithID sends a message and returns the message ID.
func (gw *TelegramGateway) sendMessageWithID(chatID int64, text string, replyTo int64) (int64, error) {
	req := tgSendMessageRequest{
		ChatID:  chatID,
		Text:    text,
		ReplyTo: replyTo,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal message: %w", err)
	}

	apiURL := fmt.Sprintf("%s/sendMessage", gw.apiBase)
	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := gw.httpClient.Do(httpReq)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("sendMessage failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Parse message ID from response
	var apiResp struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return 0, err
	}
	return apiResp.Result.MessageID, nil
}

// editMessageText edits an existing message's text.
func (gw *TelegramGateway) editMessageText(chatID int64, messageID int64, text string) error {
	params := map[string]string{
		"chat_id":    strconv.FormatInt(chatID, 10),
		"message_id": strconv.FormatInt(messageID, 10),
		"text":       text,
	}
	_, err := gw.apiCall("editMessageText", params)
	if err != nil {
		// "message is not modified" is harmless — just means same content
		if strings.Contains(err.Error(), "not modified") {
			return nil
		}
		gw.logger.Debug("editMessageText failed", "error", err)
	}
	return err
}

// deleteMessage deletes a message by ID.
func (gw *TelegramGateway) deleteMessage(chatID int64, messageID int64) error {
	params := map[string]string{
		"chat_id":    strconv.FormatInt(chatID, 10),
		"message_id": strconv.FormatInt(messageID, 10),
	}
	_, err := gw.apiCall("deleteMessage", params)
	return err
}

// truncateForTG truncates text for Telegram display.
func truncateForTG(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// splitMessage splits a long message into chunks that fit within Telegram's
// 4096 character limit. Splits on newline boundaries when possible.
func splitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	var chunks []string
	for len(text) > 0 {
		if len(text) <= maxLen {
			chunks = append(chunks, text)
			break
		}

		// Try to split at a newline near the limit
		splitAt := maxLen
		for i := maxLen; i > maxLen/2; i-- {
			if i < len(text) && text[i] == '\n' {
				splitAt = i + 1
				break
			}
		}

		chunks = append(chunks, text[:splitAt])
		text = text[splitAt:]
	}

	return chunks
}

// ============================================================
// APPROVAL SYSTEM
// ============================================================

// assessSeverity returns the risk level of a tool call.
// "low" = auto-approve silently
// "medium" = ask once, then whitelist for session
// "critical" = always ask, never whitelisted
func assessSeverity(toolName, argsJSON string) string {
	switch toolName {
	case "terminal":
		args := kyociParseArgs(argsJSON)
		cmd, _ := args["command"].((string))
		return assessCommandSeverity(cmd)
	case "file":
		args := kyociParseArgs(argsJSON)
		action, _ := args["action"].(string)
		switch action {
		case "read", "list", "search":
			return "low"
		case "write", "mkdir":
			return "medium"
		case "delete":
			return "critical"
		default:
			return "medium"
		}
	case "security_scan":
		return "low"
	case "delegation":
		return "medium"
	default:
		return "low"
	}
}

// assessCommandSeverity classifies a terminal command by risk.
func assessCommandSeverity(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return "low"
	}
	lower := strings.ToLower(cmd)

	// ── CRITICAL: always ask, never whitelisted ──
	criticalPatterns := []string{
		"rm -rf", "rm -fr", "rmdir",
		"kill -9", "killall", "pkill",
		"shutdown", "reboot", "halt",
		"mkfs", "dd if=", "> /dev/sd",
		"chmod 777", "chown -R",
		"git push --force", "git push -f ", "git reset --hard",
		"git clean -fd",
		"docker rm", "docker rmi", "docker system prune",
		"docker volume rm", "docker network rm",
		"kubectl delete", "kubectl drain",
		"sudo ",
		"drop table", "drop database", "truncate ",
		"shred ",
		"iptables -F", "ufw disable",
		"systemctl stop", "systemctl disable",
		"launchctl unload",
	}
	for _, p := range criticalPatterns {
		if strings.Contains(lower, p) {
			return "critical"
		}
	}

	// ── LOW: safe dev/ops commands — auto-approve ──
	base := strings.Fields(cmd)
	if len(base) == 0 {
		return "low"
	}
	baseCmd := base[0]
	if idx := strings.LastIndex(baseCmd, "/"); idx >= 0 {
		baseCmd = baseCmd[idx+1:]
	}

	safeCommands := map[string]bool{
		// read-only inspection
		"ls": true, "cat": true, "head": true, "tail": true, "less": true,
		"pwd": true, "echo": true, "whoami": true, "id": true,
		"date": true, "uptime": true, "uname": true, "hostname": true,
		"df": true, "du": true, "free": true, "vm_stat": true,
		"ps": true, "top": true, "htop": true,
		"grep": true, "rg": true, "find": true, "fd": true,
		"wc": true, "sort": true, "uniq": true, "cut": true, "tr": true,
		"diff": true, "file": true, "stat": true, "touch": true,
		// dev tooling — safe to run
		"git": true, "npm": true, "npx": true, "node": true, "python3": true,
		"python": true, "pip": true, "uv": true, "go": true, "cargo": true,
		"rustc": true, "java": true, "mvn": true, "gradle": true,
		"make": true, "cmake": true,
		// docker — safe read/inspect/build
		"docker": true, "docker-compose": true,
		// network inspection
		"curl": true, "wget": true, "ping": true, "dig": true, "nslookup": true,
		"netstat": true, "ss": true, "lsof": true, "ifconfig": true,
		// system info
		"which": true, "whereis": true, "type": true,
		"env": true, "printenv": true,
		"mdfind": true, "mdls": true,
		"sw_vers": true, "sysctl": true,
		// text tools
		"sed": true, "awk": true, "jq": true, "yq": true,
		"tee": true, "xargs": true,
		"tar": true, "unzip": true, "gzip": true,
	}
	if safeCommands[baseCmd] {
		// Even safe base commands can be dangerous with certain flags
		if baseCmd == "git" && (strings.Contains(lower, "push --force") || strings.Contains(lower, "reset --hard")) {
			return "critical" // already caught above, but double-check
		}
		if baseCmd == "docker" && (strings.Contains(lower, "rm ") || strings.Contains(lower, "rmi ") || strings.Contains(lower, "prune")) {
			return "critical"
		}
		if baseCmd == "curl" && (strings.Contains(lower, "-x post") || strings.Contains(lower, "-x put") || strings.Contains(lower, "-x delete") || strings.Contains(lower, "--request post") || strings.Contains(lower, "--request delete")) {
			return "medium" // API mutations need one-time approval
		}
		return "low"
	}

	// ── MEDIUM: unknown commands — ask once ──
	return "medium"
}

// isSafeCommand is kept for backward compat (isRiskyTool replacement).
func isSafeCommand(cmd string) bool {
	return assessCommandSeverity(cmd) == "low"
}

// requestApproval sends an inline keyboard to the user and blocks until they respond.
// Returns approvalResponse with the user's decision.
func (gw *TelegramGateway) requestApproval(chatID int64, toolName, summary, severity string) bool {
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

	_, err := gw.apiCall("sendMessage", params)
	if err != nil {
		gw.logger.Error("failed to send approval message", "error", err)
		return true // Fail open
	}

	gw.logger.Info("approval requested", "approval_id", approvalID, "tool", toolName, "severity", severity, "chat_id", chatID)

	select {
	case resp := <-ch:
		if resp.whitelist && resp.approved {
			gw.whitelistMu.Lock()
			if gw.sessionWhitelist[chatID] == nil {
				gw.sessionWhitelist[chatID] = make(map[string]bool)
			}
			gw.sessionWhitelist[chatID][toolName] = true
			gw.whitelistMu.Unlock()
			gw.logger.Info("tool whitelisted for session", "tool", toolName, "chat_id", chatID)
		}
		return resp.approved
	case <-time.After(120 * time.Second):
		gw.sendMessage(chatID, "⏰ Approval timed out (120s). Tool skipped.", 0)
		return false
	}
}

// isWhitelisted checks if a tool has been session-whitelisted for a chat.
func (gw *TelegramGateway) isWhitelisted(chatID int64, toolName string) bool {
	gw.whitelistMu.Lock()
	defer gw.whitelistMu.Unlock()
	return gw.sessionWhitelist[chatID] != nil && gw.sessionWhitelist[chatID][toolName]
}

// handleCallbackQuery processes inline keyboard button presses.
func (gw *TelegramGateway) handleCallbackQuery(ctx context.Context, cq *tgCallbackQuery) {
	if cq == nil || cq.Data == "" {
		return
	}

	// Answer the callback query first (removes loading state on button)
	gw.answerCallbackQuery(cq.ID, "")

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
			gw.trustedChats[cq.Message.Chat.ID] = true
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
		gw.apiCall("editMessageText", params)
	}

	// Send result to the waiting goroutine
	select {
	case ch <- resp:
	default:
	}

	gw.logger.Info("approval response", "approval_id", approvalID, "action", action, "approved", resp.approved, "whitelist", resp.whitelist)
}

// answerCallbackQuery answers a callback query (removes loading spinner on button).
func (gw *TelegramGateway) answerCallbackQuery(callbackID, text string) {
	params := map[string]string{
		"callback_query_id": callbackID,
	}
	if text != "" {
		params["text"] = text
		params["show_alert"] = "false"
	}
	_, err := gw.apiCall("answerCallbackQuery", params)
	if err != nil {
		gw.logger.Debug("answerCallbackQuery failed", "error", err)
	}
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

// ============================================================
// MARKDOWN → HTML CONVERSION FOR TELEGRAM
// ============================================================

// htmlEscape escapes special HTML characters for Telegram's HTML parse mode.
func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// markdownToHTML converts markdown text to Telegram HTML format.
// Supports: **bold**, *italic*, ~~strike~~, __underline__, `code`, ```code blocks```,
// > quotes, # headings, - bullet lists, [links](url).
// First escapes HTML chars, then applies markdown replacements.
func markdownToHTML(input string) string {
	// Escape HTML entities first
	s := htmlEscape(input)

	// Code blocks ```...```
	codeBlockRe := regexp.MustCompile("(?s)```\\w*\\n?(.*?)```")
	s = codeBlockRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := codeBlockRe.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		return "<pre><code>" + strings.TrimSpace(sub[1]) + "</code></pre>"
	})

	// Inline code `...`
	inlineCodeRe := regexp.MustCompile("`([^`]+)`")
	s = inlineCodeRe.ReplaceAllString(s, "<code>$1</code>")

	// Bold **text**
	boldRe := regexp.MustCompile(`\*\*(.+?)\*\*`)
	s = boldRe.ReplaceAllString(s, "<b>$1</b>")

	// Bold __text__ (alternative)
	boldUnderRe := regexp.MustCompile(`__(.+?)__`)
	s = boldUnderRe.ReplaceAllString(s, "<b>$1</b>")

	// Italic *text* (but not inside bold)
	italicRe := regexp.MustCompile(`\*(.+?)\*`)
	s = italicRe.ReplaceAllString(s, "<i>$1</i>")

	// Italic _text_ (but not word_parts)
	italicUnderRe := regexp.MustCompile(`(^|\s)_([^_]+?)_(\s|$)`)
	s = italicUnderRe.ReplaceAllString(s, "$1<i>$2</i>$3")

	// Strikethrough ~~text~~
	strikeRe := regexp.MustCompile(`~~(.+?)~~`)
	s = strikeRe.ReplaceAllString(s, "<s>$1</s>")

	// Spoiler ||text||
	spoilerRe := regexp.MustCompile(`\|\|(.+?)\|\|`)
	s = spoilerRe.ReplaceAllString(s, "<tg-spoiler>$1</tg-spoiler>")

	// Links [text](url)
	linkRe := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	s = linkRe.ReplaceAllString(s, `<a href="$2">$1</a>`)

	// Headings: # → bold
	headingRe := regexp.MustCompile(`(?m)^(#{1,3})\s+(.+)$`)
	s = headingRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := headingRe.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		return "\n<b>" + sub[2] + "</b>"
	})

	// Block quotes > text
	quoteRe := regexp.MustCompile(`(?m)^&gt;\s*(.+)$`)
	s = quoteRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := quoteRe.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		return "<blockquote>" + sub[1] + "</blockquote>"
	})

	// Clean up: collapse multiple newlines
	s = regexp.MustCompile(`\n{4,}`).ReplaceAllString(s, "\n\n\n")

	return s
}

// sendMessageHTML sends a message using HTML parse mode.
func (gw *TelegramGateway) sendMessageHTML(chatID int64, text string, replyTo int64) error {
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
		_, err := gw.apiCall("sendMessage", params)
		if err != nil {
			// Fallback: send plain text without HTML if parsing fails
			gw.logger.Warn("HTML sendMessage failed, falling back to plain text", "error", err)
			plainChunks := splitMessage(text, 4000)
			for _, pc := range plainChunks {
				gw.sendMessage(chatID, pc, replyTo)
			}
			return nil
		}
	}
	return nil
}
