package gateway

import (
	"strings"
	"sync"
	"time"
)

// This file holds all per-chat mutable state for TelegramGateway, behind a
// single ChatStore with one RWMutex. The maps are disjoint — no code path
// ever locks two of them in a cross-map critical section — so collapsing the
// former chatMu/rateMu/whitelistMu into one lock is safe and simpler.

// maxHistoryTurns is how many previous exchanges to include as context.
const maxHistoryTurns = 4

// convTurn stores one user-assistant exchange for conversation history.
type convTurn struct {
	user      string
	assistant string
}

// ChatStore holds the per-chat state for a TelegramGateway: chat roles,
// conversation history, rate-limit timestamps, trust flags, and the session
// whitelist. All accessors are goroutine-safe under a single RWMutex.
type ChatStore struct {
	mu sync.RWMutex

	chatRoles        map[int64]string          // chatID → role ("" = auto-detect)
	chatHistory      map[int64][]convTurn      // chatID → recent exchanges
	lastMsg          map[int64]time.Time       // userID → last message time (rate limit)
	trustedChats     map[int64]bool            // chatID → auto-approve-all
	sessionWhitelist map[int64]map[string]bool // chatID → tool names approved once
}

// NewChatStore builds an empty ChatStore.
func NewChatStore() *ChatStore {
	return &ChatStore{
		chatRoles:        make(map[int64]string),
		chatHistory:      make(map[int64][]convTurn),
		lastMsg:          make(map[int64]time.Time),
		trustedChats:     make(map[int64]bool),
		sessionWhitelist: make(map[int64]map[string]bool),
	}
}

// ── Chat role ───────────────────────────────────────────────────

// getChatRole returns the forced role for a chat ("" = auto-detect).
func (s *ChatStore) getChatRole(chatID int64) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.chatRoles[chatID]
}

// setChatRole forces a role for a chat ("" resets to auto-detect).
func (s *ChatStore) setChatRole(chatID int64, role string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chatRoles[chatID] = role
}

// ── Conversation history ────────────────────────────────────────

// addHistory stores a user-assistant exchange and prunes old entries.
func (s *ChatStore) addHistory(chatID int64, user, assistant string) {
	// Truncate assistant response to keep context manageable.
	if len(assistant) > 500 {
		assistant = assistant[:500] + "..."
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.chatHistory[chatID] = append(s.chatHistory[chatID], convTurn{
		user:      user,
		assistant: assistant,
	})

	// Keep only last maxHistoryTurns exchanges.
	if len(s.chatHistory[chatID]) > maxHistoryTurns {
		s.chatHistory[chatID] = s.chatHistory[chatID][len(s.chatHistory[chatID])-maxHistoryTurns:]
	}
}

// clearHistory removes all conversation history for a chat.
func (s *ChatStore) clearHistory(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.chatHistory, chatID)
}

// buildTaskWithContext prepends recent conversation history to the current
// task so the LLM can understand follow-ups like "all of it" or "yes, do that".
func (s *ChatStore) buildTaskWithContext(chatID int64, currentMessage string) string {
	s.mu.RLock()
	history := s.chatHistory[chatID]
	s.mu.RUnlock()

	if len(history) == 0 {
		return currentMessage
	}

	// Keep only last 3 exchanges to avoid context bloat.
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
		// Truncate assistant response to key info.
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

// ── Rate limiting ───────────────────────────────────────────────

// checkRate returns false if userID messaged within the 2s spam window,
// otherwise records now and returns true.
func (s *ChatStore) checkRate(userID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	last, exists := s.lastMsg[userID]
	if exists && time.Since(last) < 2*time.Second {
		return false
	}
	s.lastMsg[userID] = time.Now()
	return true
}

// ── Trust mode ──────────────────────────────────────────────────

// isTrusted reports whether a chat has auto-approve-all (trust) mode on.
func (s *ChatStore) isTrusted(chatID int64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.trustedChats[chatID]
}

// setTrusted toggles trust mode for a chat.
func (s *ChatStore) setTrusted(chatID int64, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if on {
		s.trustedChats[chatID] = true
	} else {
		delete(s.trustedChats, chatID)
	}
}

// ── Session whitelist ───────────────────────────────────────────

// isWhitelisted reports whether a tool has been session-whitelisted for a chat.
func (s *ChatStore) isWhitelisted(chatID int64, toolName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionWhitelist[chatID] != nil && s.sessionWhitelist[chatID][toolName]
}

// whitelistTool adds a tool to a chat's session whitelist.
func (s *ChatStore) whitelistTool(chatID int64, toolName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessionWhitelist[chatID] == nil {
		s.sessionWhitelist[chatID] = make(map[string]bool)
	}
	s.sessionWhitelist[chatID][toolName] = true
}

// whitelistedTools returns the sorted tool names whitelisted for a chat.
func (s *ChatStore) whitelistedTools(chatID int64) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	wl := s.sessionWhitelist[chatID]
	if len(wl) == 0 {
		return nil
	}
	tools := make([]string, 0, len(wl))
	for t := range wl {
		tools = append(tools, t)
	}
	return tools
}

// clearWhitelist empties a chat's session whitelist.
func (s *ChatStore) clearWhitelist(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionWhitelist[chatID] = make(map[string]bool)
}
