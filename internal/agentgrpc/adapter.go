package agentgrpc

import (
	"context"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/orchestrator"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// OrchestratorBackend adapts *orchestrator.Orchestrator to the Backend
// interface. It owns the build version and process start time so GetStatus can
// report them even though the orchestrator itself does not track uptime or
// version. Construct one in main.go and pass it to NewServer.
type OrchestratorBackend struct {
	orch    *orchestrator.Orchestrator
	version string
	started time.Time
}

// NewOrchestratorBackend wraps orch for gRPC serving. version is reported by
// GetStatus; started anchors the uptime calculation.
func NewOrchestratorBackend(orch *orchestrator.Orchestrator, version string, started time.Time) *OrchestratorBackend {
	return &OrchestratorBackend{
		orch:    orch,
		version: version,
		started: started,
	}
}

// Execute delegates to the orchestrator, mapping the role name to kyoci.RoleType
// ("" or unknown => RoleCustom, which the orchestrator auto-detects).
func (b *OrchestratorBackend) Execute(ctx context.Context, prompt, role string) (*TaskResult, error) {
	res, err := b.orch.Execute(ctx, prompt, ParseRoleName(role))
	if err != nil {
		return nil, err
	}
	tr := &TaskResult{
		Content:       res.Content,
		Role:          res.Role,
		Iterations:    res.Iterations,
		ToolCallsMade: res.ToolCallsMade,
	}
	usage := res.Usage // copy out of the result struct
	tr.Usage = &usage
	return tr, nil
}

// ExecuteStream delegates to the orchestrator's streaming path, re-wrapping the
// channel items into agentgrpc's StreamChunk (dropping fields the gRPC layer
// does not surface).
func (b *OrchestratorBackend) ExecuteStream(ctx context.Context, prompt, role string) (<-chan StreamChunk, error) {
	ch, err := b.orch.ExecuteStream(ctx, prompt, ParseRoleName(role))
	if err != nil {
		return nil, err
	}
	out := make(chan StreamChunk, 1)
	go func() {
		defer close(out)
		for c := range ch {
			s := StreamChunk{
				Partial: c.Content,
				Done:    c.Done,
			}
			if c.Usage != nil {
				u := *c.Usage
				s.Usage = &u
			}
			// Surface a stream error as a terminal chunk so the client observes
			// the failure cleanly; the orchestrator's StreamChunk carries Error.
			if c.Error != nil {
				s.Done = true
			}
			select {
			case out <- s:
			case <-ctx.Done():
				return
			}
			if s.Done {
				return
			}
		}
	}()
	return out, nil
}

// Status maps the orchestrator's SystemStatus to the agentgrpc Status snapshot.
func (b *OrchestratorBackend) Status() *Status {
	st := b.orch.Status()
	s := &Status{
		Version:       b.version,
		UptimeSeconds: int64(time.Since(b.started).Seconds()),
		Providers:     st.Providers,
		Started:       st.Started,
	}
	if st.MemoryStats != nil {
		s.MemoryEntries = int32(st.MemoryStats.TotalEntries)
	}
	s.ActiveRoles = make([]string, 0, len(st.Roles))
	for _, rc := range st.Roles {
		s.ActiveRoles = append(s.ActiveRoles, rc.Type.String())
	}
	return s
}

// ParseRoleName converts a role name string to kyoci.RoleType. Unknown or empty
// names map to RoleCustom, which the orchestrator treats as "auto-detect".
// Centralized here so the cmd/server, gateway, and gRPC paths stay consistent.
func ParseRoleName(role string) kyoci.RoleType {
	switch role {
	case "developer", "dev":
		return kyoci.RoleDeveloper
	case "sre", "ops":
		return kyoci.RoleSRE
	case "qa", "test":
		return kyoci.RoleQA
	case "pm", "manager":
		return kyoci.RolePM
	case "frontend", "ui", "ux":
		return kyoci.RoleFrontend
	default:
		return kyoci.RoleCustom // triggers orchestrator auto-detect
	}
}
