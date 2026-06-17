// Package dashboard exposes the management API surface that the embedded SPA
// (and any other UI client) calls: provider/model enumeration, config read
// and atomic write-back, hardware detection + recommendations, direct-stream
// chat, and the orchestrator-backed agent mode.
package dashboard

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/metabbe3/Kyoci-Agent/internal/config"
	"github.com/metabbe3/Kyoci-Agent/internal/hardware"
	"github.com/metabbe3/Kyoci-Agent/internal/orchestrator"
	"github.com/metabbe3/Kyoci-Agent/internal/recommend"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// MaskedAPIKey is what GET /api/dashboard/config returns in place of the real
// key. The frontend sends it back unchanged to mean "leave the key alone"; an
// actual key value in a PUT means "replace".
const MaskedAPIKey = "••••"

// Server wires the orchestrator + config path to the dashboard HTTP handlers.
// All handlers are methods on Server so they share state cleanly.
type Server struct {
	orch    *orchestrator.Orchestrator
	cfg     *config.Config
	cfgPath string
	logger  *slog.Logger
}

// NewServer constructs a dashboard server. cfgPath is where PUT /config will
// write back atomically (with a .backup sibling).
func NewServer(orch *orchestrator.Orchestrator, cfg *config.Config, cfgPath string) *Server {
	return &Server{
		orch:    orch,
		cfg:     cfg,
		cfgPath: cfgPath,
		logger:  slog.Default().With("component", "dashboard"),
	}
}

// Routes returns the dashboard route registrations (mounted under
// /api/dashboard/* by the caller). The SPA fallback is intentionally NOT
// registered here — it must be added LAST on the parent mux so it doesn't
// shadow API routes.
func (s *Server) Routes() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/api/dashboard/providers":       s.handleProviders,
		"/api/dashboard/models":          s.handleModels,
		"/api/dashboard/config":          s.handleConfig,
		"/api/dashboard/test-connection": s.handleTestConnection,
		"/api/dashboard/chat":            s.handleChat,
		"/api/dashboard/upload":          s.handleUpload,
		"/api/dashboard/hardware":        s.handleHardware,
		"/api/dashboard/recommendations": s.handleRecommendations,
		"/api/dashboard/skills":          s.handleSkills,
	}
}

// ProviderSummary is one row of GET /api/dashboard/providers.
type ProviderSummary struct {
	Name         string `json:"name"`
	Enabled      bool   `json:"enabled"`
	Available    bool   `json:"available"`
	DefaultModel string `json:"default_model"`
	BaseURL      string `json:"base_url"`
	ModelCount   int    `json:"model_count"`
}

func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	reg := s.orch.GetProviderRegistry()
	regProviders := reg.List() // map[name]kyoci.Provider

	out := make([]ProviderSummary, 0, len(s.cfg.Providers))
	for name, pc := range s.cfg.Providers {
		summary := ProviderSummary{
			Name:         name,
			Enabled:      pc.Enabled,
			DefaultModel: pc.DefaultModel,
			BaseURL:      pc.BaseURL,
		}
		if p, ok := regProviders[name]; ok {
			summary.Available = p.IsAvailable()
			summary.ModelCount = len(p.Models())
		}
		out = append(out, summary)
	}

	writeJSON(w, http.StatusOK, map[string]any{"providers": out})
}

// ModelRow is one row of GET /api/dashboard/models.
type ModelRow struct {
	Provider          string `json:"provider"`
	ID                string `json:"id"`
	ContextLength     int    `json:"context_length"`
	SupportsTools     bool   `json:"supports_tools"`
	SupportsStreaming bool   `json:"supports_streaming"`
	SupportsImages    bool   `json:"supports_images"`
	MaxOutputTokens   int    `json:"max_output_tokens"`
	Description       string `json:"description,omitempty"`
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	reg := s.orch.GetProviderRegistry()
	rows := []ModelRow{}
	for name, p := range reg.List() {
		for _, m := range p.Models() {
			rows = append(rows, ModelRow{
				Provider:          name,
				ID:                m.ID,
				ContextLength:     m.ContextLength,
				SupportsTools:     m.SupportsTools,
				SupportsStreaming: m.SupportsStreaming,
				SupportsImages:    m.SupportsImages,
				MaxOutputTokens:   m.MaxOutputTokens,
				Description:       m.Description,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": rows})
}

// ProviderConfigDTO mirrors config.ProviderConfig but with a masked API key
// on read. On PUT, an api_key of MaskedAPIKey (or empty) means "don't touch".
type ProviderConfigDTO struct {
	Enabled      bool   `json:"enabled"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
	DefaultModel string `json:"default_model"`
	Timeout      int    `json:"timeout"`
	MaxRetries   int    `json:"max_retries"`
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.configGet(w, r)
	case http.MethodPut:
		s.configPut(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) configGet(w http.ResponseWriter, r *http.Request) {
	out := map[string]ProviderConfigDTO{}
	for name, pc := range s.cfg.Providers {
		out[name] = ProviderConfigDTO{
			Enabled:      pc.Enabled,
			BaseURL:      pc.BaseURL,
			APIKey:       MaskedAPIKey,
			DefaultModel: pc.DefaultModel,
			Timeout:      pc.Timeout,
			MaxRetries:   pc.MaxRetries,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": out})
}

// configPut accepts {providers: {name: ProviderConfigDTO}}. Only providers
// present in the body are updated; others are left alone. The new config is
// written to disk atomically (with .backup) and reflected in s.cfg in-memory
// so the UI sees the updated values immediately. Provider *behavior* still
// requires a server restart — that's a documented v1 limitation.
func (s *Server) configPut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Providers map[string]ProviderConfigDTO `json:"providers"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if len(body.Providers) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no providers in request"})
		return
	}

	if err := persistProviderUpdates(s.cfgPath, body.Providers); err != nil {
		s.logger.Error("config writeback failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to write config: " + err.Error()})
		return
	}

	// Reflect in-memory so subsequent GETs see the new values.
	for name, dto := range body.Providers {
		pc := s.cfg.Providers[name] // copy
		pc.Enabled = dto.Enabled
		if dto.BaseURL != "" {
			pc.BaseURL = dto.BaseURL
		}
		if dto.DefaultModel != "" {
			pc.DefaultModel = dto.DefaultModel
		}
		if dto.APIKey != "" && dto.APIKey != MaskedAPIKey {
			pc.APIKey = dto.APIKey
		}
		if dto.Timeout > 0 {
			pc.Timeout = dto.Timeout
		}
		if dto.MaxRetries > 0 {
			pc.MaxRetries = dto.MaxRetries
		}
		s.cfg.Providers[name] = pc
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "config saved. Restart server for provider changes to take effect.",
	})
}

func (s *Server) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider required"})
		return
	}

	reg := s.orch.GetProviderRegistry()
	p, err := reg.Get(body.Provider)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "error": "provider not registered (enable it in config and restart)"})
		return
	}
	available := false
	errStr := ""
	if p.IsAvailable() {
		available = true
	} else {
		errStr = "provider registered but IsAvailable() returned false — check API key / network"
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": available, "error": errStr})
}

func (s *Server) handleHardware(w http.ResponseWriter, r *http.Request) {
	specs, _ := hardware.Detect()
	writeJSON(w, http.StatusOK, specs)
}

// handleSkills returns the list of registered prompt-skills (zero-AI fast
// paths). The UI renders them in the Skills gallery.
func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	reg := s.orch.GetSkillRegistry()
	infos := reg.List()
	writeJSON(w, http.StatusOK, map[string]any{"skills": infos})
}

func (s *Server) handleRecommendations(w http.ResponseWriter, r *http.Request) {
	specs, _ := hardware.Detect()
	res := recommend.Recommend(specs)
	writeJSON(w, http.StatusOK, res)
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// roleFromString accepts user/assistant/system and falls back to user — used
// by the chat handler when mapping frontend messages to kyoci.Message.
func roleFromString(s string) kyoci.MessageRole {
	switch strings.ToLower(s) {
	case "system":
		return kyoci.RoleSystem
	case "assistant":
		return kyoci.RoleAssistant
	case "tool":
		return kyoci.RoleTool
	default:
		return kyoci.RoleUser
	}
}
