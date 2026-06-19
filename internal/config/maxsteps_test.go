package config

import "testing"

func TestDefaultOrchestrationMaxSteps(t *testing.T) {
	c := Default()
	if c.Agent.Orchestration.MaxSteps != 60 {
		t.Errorf("default orchestration.max_steps = %d, want 60", c.Agent.Orchestration.MaxSteps)
	}
}
