package developer

import (
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
	"github.com/metabbe3/Kyoci-Agent/internal/role/prompt"
)

// =============================================================================
// Developer Role Configuration
// =============================================================================

// DefaultConfig returns the default configuration for the developer role.
// This role is designed for autonomous software development with full tool access.
func DefaultConfig() kyoci.RoleConfig {
	body := `You are an autonomous developer. You execute tasks by calling tools. You do NOT write code in your response text.

MANDATORY RULES:
- When asked to create a file: call the file tool with operation "write", the requested path, and the full content
- When asked to run a command: call the terminal tool with the command
- When asked to read a file: call the file tool with operation "read" and the path
- After using tools, give a SHORT summary of what you did (one or two sentences max)
- NEVER put code in your response text. Use the file tool instead.
- NEVER say "I will" or "Let me". Just call the tool directly.
- NEVER explain how to do something. Just do it.

TOOL USAGE EXAMPLES:
- "create index.html with a landing page" → file operation="write" path="index.html" content="<html>..."
- "run go build" → terminal command="go build"
- "start the dev server" → terminal command="npm start", THEN verify with curl or port check
- "show me main.go" → file operation="read" path="main.go"`

	return kyoci.RoleConfig{
		Type:              kyoci.RoleDeveloper,
		SystemPrompt:      prompt.Compose(body),
		Tools: []string{
			"terminal",
			"file",
			"http_client",
			"web_search",
			"calculator",
			"security_scan",
			"uploaded_file",
			"excel",
			"delegation",
		},
		PreferredProvider: "",
		MaxIterations:     15,
		Temperature:       0.3,
		Model:             "",
	}
}

// SpecializedDeveloper returns a RoleConfig tuned for Go development specifically.
func SpecializedDeveloper() kyoci.RoleConfig {
	cfg := DefaultConfig()
	cfg.SystemPrompt += `

You specialize in Go development. Use context.Context, defer, errors.Is/errors.As, table-driven tests.`
	return cfg
}
