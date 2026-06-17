package gateway

import (
	"context"
	"fmt"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
	"github.com/metabbe3/Kyoci-Agent/internal/orchestrator"
)

// OrchestratorAdapter adapts the orchestrator.Orchestrator to the OrchestratorClient interface.
type OrchestratorAdapter struct {
	orch *orchestrator.Orchestrator
}

// NewOrchestratorAdapter creates a new adapter wrapping the orchestrator.
func NewOrchestratorAdapter(orch *orchestrator.Orchestrator) *OrchestratorAdapter {
	return &OrchestratorAdapter{orch: orch}
}

// Execute runs a task through the orchestrator and returns the result text.
// DEPRECATED — use ExecuteRich instead. Kept for backward compatibility.
func (a *OrchestratorAdapter) Execute(ctx context.Context, task string, role string) (string, int, error) {
	result, err := a.ExecuteRich(ctx, task, role)
	if err != nil {
		return "", 0, err
	}
	return result.Content, result.Iterations, nil
}

// ExecuteRich runs a task and returns the full ActivityResult with telemetry data.
func (a *OrchestratorAdapter) ExecuteRich(ctx context.Context, task string, role string) (*ActivityResult, error) {
	// Parse role string to RoleType
	var roleType kyoci.RoleType
	switch role {
	case "developer":
		roleType = kyoci.RoleDeveloper
	case "sre":
		roleType = kyoci.RoleSRE
	case "qa":
		roleType = kyoci.RoleQA
	case "pm":
		roleType = kyoci.RolePM
	case "frontend":
		roleType = kyoci.RoleFrontend
	default:
		roleType = kyoci.RoleCustom // triggers auto-detect
	}

	result, err := a.orch.Execute(ctx, task, roleType)
	if err != nil {
		return nil, fmt.Errorf("task failed: %w", err)
	}

	return &ActivityResult{
		Content:     result.Content,
		Role:        result.Role.String(),
		ToolCalls:   result.ToolCallsMade,
		ToolLog:     result.ToolCallLog,
		Iterations:  result.Iterations,
		TokensUsed:  result.Usage.TotalTokens,
	}, nil
}

// Status returns system status information.
func (a *OrchestratorAdapter) Status() ([]string, int, int, int) {
	status := a.orch.Status()
	return status.Providers, len(status.Roles), len(status.Tools), len(status.Skills)
}
