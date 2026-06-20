package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// SkillInstallTool — lets the agent install prompt-skills from GitHub URLs
// and MCP servers from npm packages. Mirrors Claude Code's skill auto-discovery.
//
// Usage:
//   skill_install action=skill url="https://raw.githubusercontent.com/user/repo/main/skill.md"
//   skill_install action=mcp name="server-name" command="npx" args="-y @scope/mcp-server"
// =====================================================================================

type SkillInstallTool struct{}

func NewSkillInstallTool() *SkillInstallTool { return &SkillInstallTool{} }

func (t *SkillInstallTool) Name() string { return "skill_install" }

func (t *SkillInstallTool) Description() string {
	return "Install prompt-skills from GitHub URLs or MCP servers from npm. " +
		"Use action=skill with a raw GitHub URL to download a .md skill file. " +
		"Use action=mcp with name/command/args to add an MCP server. " +
		"Use action=list to see what's installed. " +
		"Skills are saved to data/skills/ and become active on restart."
}

func (t *SkillInstallTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{Name: "action", Type: "string", Required: true,
			Description: "skill (install a .md skill from URL), mcp (add an MCP server), list (show installed)",
			EnumValues: []string{"skill", "mcp", "list"}},
		{Name: "url", Type: "string", Required: false,
			Description: "For action=skill: raw GitHub URL to the .md skill file (e.g. https://raw.githubusercontent.com/user/repo/main/my-skill.md)"},
		{Name: "name", Type: "string", Required: false,
			Description: "For action=skill: skill name (defaults to URL filename). For action=mcp: server name"},
		{Name: "command", Type: "string", Required: false,
			Description: "For action=mcp: the command to run (e.g. npx, node, python3)"},
		{Name: "args", Type: "string", Required: false,
			Description: "For action=mcp: space-separated args (e.g. '-y @modelcontextprotocol/server-github')"},
	}
}

func (t *SkillInstallTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	action, _ := params["action"].(string)
	switch action {
	case "skill":
		return t.installSkill(params)
	case "mcp":
		return t.installMCP(params)
	case "list":
		return t.listInstalled()
	default:
		return "", fmt.Errorf("unknown action: %s (use skill, mcp, or list)", action)
	}
}

func (t *SkillInstallTool) installSkill(params map[string]interface{}) (string, error) {
	url, _ := params["url"].(string)
	if url == "" {
		return "", fmt.Errorf("url is required for action=skill")
	}
	// Fetch the raw content.
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	content := string(body)
	// Determine skill name.
	name, _ := params["name"].(string)
	if name == "" {
		// Extract from URL filename.
		base := filepath.Base(url)
		name = strings.TrimSuffix(base, ".md")
		name = strings.TrimSuffix(name, ".markdown")
	}
	name = strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	// Validate.
	if strings.ContainsAny(name, "/\\..") {
		return "", fmt.Errorf("invalid skill name: %s", name)
	}
	// Save.
	skillsDir := filepath.Join("data", "skills")
	os.MkdirAll(skillsDir, 0755)
	path := filepath.Join(skillsDir, name+".md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write failed: %w", err)
	}
	return fmt.Sprintf("✓ Skill '%s' installed to %s. It will be active after the next server restart. Preview:\n\n%s", name, path, truncatePreview(content, 500)), nil
}

func (t *SkillInstallTool) installMCP(params map[string]interface{}) (string, error) {
	name, _ := params["name"].(string)
	command, _ := params["command"].(string)
	argsStr, _ := params["args"].(string)
	if name == "" || command == "" {
		return "", fmt.Errorf("name and command are required for action=mcp")
	}
	args := strings.Fields(argsStr)
	// Read current config.
	cfgPath := filepath.Join("config", "default.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return "", fmt.Errorf("read config: %w", err)
	}
	cfgStr := string(data)
	// Build the YAML block for the new server.
	var envBlock string
	envStr, _ := params["env"].(string)
	if envStr != "" {
		envBlock = "\n            env:\n"
		for _, line := range strings.Split(envStr, "\n") {
			envBlock += fmt.Sprintf("                %s\n", line)
		}
	}
	serverBlock := fmt.Sprintf("        %s:\n            enabled: true\n            command: \"%s\"\n            args: %s%s\n",
		name, command, formatYAMLArgs(args), envBlock)
	// Insert before the "# ── LLM Providers" marker.
	marker := "# ── LLM Providers"
	idx := strings.Index(cfgStr, marker)
	if idx < 0 {
		return "", fmt.Errorf("could not find MCP section in config — please add manually")
	}
	// Find the "servers:" closing (empty {}) — replace with server entries.
	cfgStr = strings.Replace(cfgStr, "    servers: {}", "    servers:\n"+serverBlock, 1)
	if !strings.Contains(cfgStr, name+":") {
		// Already has servers — append before the LLM marker.
		cfgStr = cfgStr[:idx] + serverBlock + cfgStr[idx:]
	}
	if err := os.WriteFile(cfgPath, []byte(cfgStr), 0644); err != nil {
		return "", fmt.Errorf("write config: %w", err)
	}
	return fmt.Sprintf("✓ MCP server '%s' added to config. Command: %s %s. Restart the server to activate it.", name, command, argsStr), nil
}

func (t *SkillInstallTool) listInstalled() (string, error) {
	var b strings.Builder
	// List skills.
	b.WriteString("Installed Prompt Skills:\n")
	skillsDir := filepath.Join("data", "skills")
	if entries, err := os.ReadDir(skillsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				b.WriteString(fmt.Sprintf("  - %s\n", strings.TrimSuffix(e.Name(), ".md")))
			}
		}
	}
	// List MCP servers from config.
	b.WriteString("\nConfigured MCP Servers:\n")
	cfgPath := filepath.Join("config", "default.yaml")
	data, _ := os.ReadFile(cfgPath)
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasSuffix(t, ":") && !strings.HasPrefix(t, "#") {
			name := strings.TrimSuffix(t, ":")
			if name == "context7" || name == "filesystem" || name == "fetch" || name == "github" {
				b.WriteString(fmt.Sprintf("  - %s\n", name))
			}
		}
	}
	return b.String(), nil
}

func formatYAMLArgs(args []string) string {
	if len(args) == 0 {
		return "[]"
	}
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = fmt.Sprintf("%q", a)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func truncatePreview(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n...[truncated]"
}

// Ensure it satisfies the Tool interface.
var _ kyoci.Tool = (*SkillInstallTool)(nil)

// Raw marshaling helper (unused but keeps the linter happy for json import).
var _ = json.Marshal
