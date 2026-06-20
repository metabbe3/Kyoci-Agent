package dashboard

import (
	"net/http"
)

// =====================================================================================
// Routing config handlers — GET/PUT /api/dashboard/routing
//
// Reads and writes the agent.orchestration.model_routing section of the config.
// The Combo panel in the chat header uses these to let the user configure which
// model handles each orchestrator phase (planner, worker, qa, etc.).
// =====================================================================================

// handleRouting routes GET/PUT to the appropriate handler.
func (s *Server) handleRouting(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleRoutingGet(w, r)
	case http.MethodPut:
		s.handleRoutingPut(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleRoutingGet returns the current model routing configuration.
func (s *Server) handleRoutingGet(w http.ResponseWriter, r *http.Request) {
	mr := s.cfg.Agent.Orchestration.ModelRouting
	writeJSON(w, http.StatusOK, map[string]any{
		"planner":             map[string]string{"provider": mr.Planner.Provider, "model": mr.Planner.Model},
		"worker":              map[string]string{"provider": mr.Worker.Provider, "model": mr.Worker.Model},
		"worker_file_creation": map[string]string{"provider": mr.WorkerFileCreation.Provider, "model": mr.WorkerFileCreation.Model},
		"synthesizer":         map[string]string{"provider": mr.Synthesizer.Provider, "model": mr.Synthesizer.Model},
		"qa":                  map[string]string{"provider": mr.QA.Provider, "model": mr.QA.Model},
	})
}

// handleRoutingPut updates the model routing configuration and saves to disk.
func (s *Server) handleRoutingPut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req map[string]map[string]string
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	// Update in-memory config.
	for phase, vals := range req {
		provider := vals["provider"]
		model := vals["model"]
		if provider == "" {
			continue // empty = clear routing for this phase
		}
		switch phase {
		case "planner":
			s.cfg.Agent.Orchestration.ModelRouting.Planner.Provider = provider
			s.cfg.Agent.Orchestration.ModelRouting.Planner.Model = model
		case "worker":
			s.cfg.Agent.Orchestration.ModelRouting.Worker.Provider = provider
			s.cfg.Agent.Orchestration.ModelRouting.Worker.Model = model
		case "worker_file_creation":
			s.cfg.Agent.Orchestration.ModelRouting.WorkerFileCreation.Provider = provider
			s.cfg.Agent.Orchestration.ModelRouting.WorkerFileCreation.Model = model
		case "synthesizer":
			s.cfg.Agent.Orchestration.ModelRouting.Synthesizer.Provider = provider
			s.cfg.Agent.Orchestration.ModelRouting.Synthesizer.Model = model
		case "qa":
			s.cfg.Agent.Orchestration.ModelRouting.QA.Provider = provider
			s.cfg.Agent.Orchestration.ModelRouting.QA.Model = model
		}
	}
	// Persist to YAML.
	if err := s.saveConfig(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save config: " + err.Error()})
		return
	}
	s.logger.Info("model routing updated via Combo panel")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "note": "restart server to apply changes"})
}
