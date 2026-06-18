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
	"strconv"
	"strings"
	"time"
)

// This file contains the embedded "mini Telegram Bot API SDK": the wire types
// and HTTP methods used by TelegramGateway. It is the only place that knows how
// to talk to api.telegram.org, which keeps telegram.go focused on coordination
// and makes the HTTP layer httptest-able via the injected *http.Client.

// ── Telegram Bot API wire types ─────────────────────────────────

// tgUpdate is one entry from getUpdates long-polling.
type tgUpdate struct {
	UpdateID      int64            `json:"update_id"`
	Message       *tgMessage       `json:"message"`
	CallbackQuery *tgCallbackQuery `json:"callback_query"`
}

// tgCallbackQuery is a button-press event from an inline keyboard.
type tgCallbackQuery struct {
	ID      string     `json:"id"`
	From    *tgUser    `json:"from"`
	Data    string     `json:"data"`
	Message *tgMessage `json:"message"`
}

// tgMessage is a chat message (text or command).
type tgMessage struct {
	MessageID int64   `json:"message_id"`
	From      *tgUser `json:"from"`
	Chat      tgChat  `json:"chat"`
	Text      string  `json:"text"`
}

// tgUser is a Telegram user (or bot).
type tgUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

// tgChat is a chat (private / group / channel).
type tgChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// tgAPIResponse is the common envelope for every Bot API call.
type tgAPIResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
}

// tgSendMessageRequest is the body for a plain sendMessage call.
type tgSendMessageRequest struct {
	ChatID    int64  `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
	ReplyTo   int64  `json:"reply_to_message_id,omitempty"`
}

// ── tgClient — the HTTP client wrapper ──────────────────────────

// tgClient is a thin wrapper around the Telegram Bot API HTTP surface.
// All methods are goroutine-safe; the struct itself is immutable after
// construction (base/httpClient/logger never mutate).
type tgClient struct {
	base       string       // "https://api.telegram.org/bot<TOKEN>"
	httpClient *http.Client // injected so callers can httptest it
	logger     *slog.Logger
}

// newTGClient builds a tgClient. pollTimeout configures the HTTP client's
// overall timeout (long-poll blocks for up to that many seconds).
func newTGClient(token string, httpClient *http.Client, pollTimeout int, logger *slog.Logger) *tgClient {
	if logger == nil {
		logger = slog.Default()
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: time.Duration(pollTimeout+10) * time.Second}
	}
	return &tgClient{
		base:       fmt.Sprintf("https://api.telegram.org/bot%s", token),
		httpClient: httpClient,
		logger:     logger,
	}
}

// ── Bot API methods ─────────────────────────────────────────────

// getMe verifies the bot token and returns bot info.
func (c *tgClient) getMe(ctx context.Context) (*tgUser, error) {
	resp, err := c.apiCall(ctx, "getMe", nil)
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
func (c *tgClient) getUpdates(ctx context.Context, offset int64) ([]tgUpdate, error) {
	params := map[string]string{
		"timeout": "30",
	}
	if offset > 0 {
		params["offset"] = strconv.FormatInt(offset, 10)
	}

	resp, err := c.apiCall(ctx, "getUpdates", params)
	if err != nil {
		return nil, err
	}

	var updates []tgUpdate
	if err := json.Unmarshal(resp, &updates); err != nil {
		return nil, fmt.Errorf("failed to parse updates: %w", err)
	}
	return updates, nil
}

// sendMessage sends a plain-text message to a chat.
func (c *tgClient) sendMessage(ctx context.Context, chatID int64, text string, replyTo int64) error {
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

	apiURL := fmt.Sprintf("%s/sendMessage", c.base)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
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

// sendMessageWithID sends a message and returns the assigned message ID.
func (c *tgClient) sendMessageWithID(ctx context.Context, chatID int64, text string, replyTo int64) (int64, error) {
	req := tgSendMessageRequest{
		ChatID:  chatID,
		Text:    text,
		ReplyTo: replyTo,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal message: %w", err)
	}

	apiURL := fmt.Sprintf("%s/sendMessage", c.base)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("sendMessage failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Parse message ID from response.
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

// sendChatAction sends a typing (or other) indicator.
func (c *tgClient) sendChatAction(ctx context.Context, chatID int64, action string) error {
	params := map[string]string{
		"chat_id": strconv.FormatInt(chatID, 10),
		"action":  action,
	}
	_, err := c.apiCall(ctx, "sendChatAction", params)
	return err
}

// editMessageText edits an existing message's text. A "message is not modified"
// error from the API is swallowed as harmless.
func (c *tgClient) editMessageText(ctx context.Context, chatID int64, messageID int64, text string) error {
	params := map[string]string{
		"chat_id":    strconv.FormatInt(chatID, 10),
		"message_id": strconv.FormatInt(messageID, 10),
		"text":       text,
	}
	_, err := c.apiCall(ctx, "editMessageText", params)
	if err != nil {
		// "message is not modified" is harmless — just means same content.
		if strings.Contains(err.Error(), "not modified") {
			return nil
		}
		c.logger.Debug("editMessageText failed", "error", err)
	}
	return err
}

// deleteMessage deletes a message by ID.
func (c *tgClient) deleteMessage(ctx context.Context, chatID int64, messageID int64) error {
	params := map[string]string{
		"chat_id":    strconv.FormatInt(chatID, 10),
		"message_id": strconv.FormatInt(messageID, 10),
	}
	_, err := c.apiCall(ctx, "deleteMessage", params)
	return err
}

// answerCallbackQuery answers a callback query (removes the loading spinner).
func (c *tgClient) answerCallbackQuery(ctx context.Context, callbackID, text string) {
	params := map[string]string{
		"callback_query_id": callbackID,
	}
	if text != "" {
		params["text"] = text
		params["show_alert"] = "false"
	}
	if _, err := c.apiCall(ctx, "answerCallbackQuery", params); err != nil {
		c.logger.Debug("answerCallbackQuery failed", "error", err)
	}
}

// apiCall makes a GET request to a Bot API method.
func (c *tgClient) apiCall(ctx context.Context, method string, params map[string]string) (json.RawMessage, error) {
	apiURL := fmt.Sprintf("%s/%s", c.base, method)
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

	resp, err := c.httpClient.Do(req)
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

// uploadFile uploads a file to Telegram via multipart/form-data (for future use).
func (c *tgClient) uploadFile(ctx context.Context, method string, fieldName string, fileName string, fileData []byte, params map[string]string) (json.RawMessage, error) {
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

	apiURL := fmt.Sprintf("%s/%s", c.base, method)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
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
