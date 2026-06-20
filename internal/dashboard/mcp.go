package dashboard

import (
	"fmt"
	"net/http"
	"os"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/metabbe3/Kyoci-Agent/internal/config"
)

// =====================================================================================
// MCP Manager — handlers for /api/dashboard/mcp/*
//
// Lets the UI list, toggle, add, and remove MCP servers without editing YAML.
// Changes persist to config/default.yaml and require a server restart to take
// effect (MCP servers initialize at startup, not at request time).
// =====================================================================================

// MCPServerSummary is one row of GET /api/dashboard/mcp/servers.
type MCPServerSummary struct {
	Name    string            `json:"name"`
	Enabled bool              `json:"enabled"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// handleMCPServers returns the list of configured MCP servers.
func (s *Server) handleMCPServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	servers := s.cfg.MCP.Servers
	out := make([]MCPServerSummary, 0, len(servers))
	for name, sc := range servers {
		out = append(out, MCPServerSummary{
			Name:    name,
			Enabled: sc.Enabled,
			Command: sc.Command,
			Args:    sc.Args,
			Env:     sc.Env,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, map[string]any{"servers": out})
}

// handleMCPServerUpdate handles PUT /api/dashboard/mcp/servers/{name} — toggles or adds.
func (s *Server) handleMCPServerUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		Name    string            `json:"name"`
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
		Enabled bool              `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	// Update in-memory config.
	if s.cfg.MCP.Servers == nil {
		s.cfg.MCP.Servers = make(map[string]config.MCPServerConfig)
	}
	s.cfg.MCP.Servers[req.Name] = config.MCPServerConfig{
		Enabled: req.Enabled,
		Command: req.Command,
		Args:    req.Args,
		Env:     req.Env,
	}
	// Persist to YAML.
	if err := s.saveConfig(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save config: " + err.Error()})
		return
	}
	s.logger.Info("MCP server updated", "name", req.Name, "enabled", req.Enabled)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "name": req.Name, "note": "restart server to apply"})
}

// handleMCPServerDelete handles DELETE /api/dashboard/mcp/servers/{name}.
func (s *Server) handleMCPServerDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name query param required"})
		return
	}
	delete(s.cfg.MCP.Servers, name)
	if err := s.saveConfig(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save config: " + err.Error()})
		return
	}
	s.logger.Info("MCP server deleted", "name", name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "name": name})
}

// saveConfig writes the current config back to the YAML file.
func (s *Server) saveConfig() error {
	data, err := yaml.Marshal(s.cfg)
	if err != nil {
		return fmt.Errorf("yaml marshal: %w", err)
	}
	backup := s.cfgPath + ".backup"
	if err := os.WriteFile(backup, data, 0644); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	return os.WriteFile(s.cfgPath, data, 0644)
}
