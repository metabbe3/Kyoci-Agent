package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/smtp"
	"strings"
)

// EmailTool sends emails via SMTP
type EmailTool struct {
	host     string
	port     string
	username string
	password string
	from     string
}

type EmailConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

func NewEmailTool(cfg EmailConfig) *EmailTool {
	if cfg.Port == "" {
		cfg.Port = "587"
	}
	return &EmailTool{
		host: cfg.Host, port: cfg.Port,
		username: cfg.Username, password: cfg.Password, from: cfg.From,
	}
}

func (t *EmailTool) Name() string { return "email" }

func (t *EmailTool) Description() string {
	return "Send emails via SMTP. Configure host/user/pass first."
}

func (t *EmailTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action":    map[string]interface{}{"type": "string", "enum": []string{"send"}, "description": "Send email"},
			"to":        map[string]interface{}{"type": "string", "description": "Recipient email (comma-separated for multiple)"},
			"subject":   map[string]interface{}{"type": "string", "description": "Email subject"},
			"body":      map[string]interface{}{"type": "string", "description": "Email body (plain text)"},
			"html_body": map[string]interface{}{"type": "string", "description": "Email body (HTML, optional)"},
		},
		"required": []string{"to", "subject", "body"},
	}
}

func (t *EmailTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	if t.host == "" || t.username == "" {
		return "email not configured: set SMTP host/user/pass", nil
	}

	var params struct {
		To       string `json:"to"`
		Subject  string `json:"subject"`
		Body     string `json:"body"`
		HTMLBody string `json:"html_body"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", fmt.Errorf("invalid params: %w", err)
	}

	recipients := strings.Split(params.To, ",")
	for i, r := range recipients {
		recipients[i] = strings.TrimSpace(r)
	}

	var msg strings.Builder
	msg.WriteString("From: " + t.from + "\r\n")
	msg.WriteString("To: " + params.To + "\r\n")
	msg.WriteString("Subject: " + params.Subject + "\r\n")

	if params.HTMLBody != "" {
		msg.WriteString("MIME-version: 1.0;\r\nContent-Type: text/html; charset=\"UTF-8\";\r\n\r\n")
		msg.WriteString(params.HTMLBody)
	} else {
		msg.WriteString("\r\n")
		msg.WriteString(params.Body)
	}

	auth := smtp.PlainAuth("", t.username, t.password, t.host)
	addr := t.host + ":" + t.port

	err := smtp.SendMail(addr, auth, t.from, recipients, []byte(msg.String()))
	if err != nil {
		return "", fmt.Errorf("send failed: %w", err)
	}

	return fmt.Sprintf("Email sent to %s", params.To), nil
}
