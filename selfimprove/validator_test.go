package selfimprove

import (
	"testing"
)

func TestValidationResultStruct(t *testing.T) {
	result := &ValidationResult{
		Valid:       true,
		Errors:      []string{},
		Warnings:    []string{},
		TestsPassed: 10,
		TestsFailed: 0,
		Coverage:    85.5,
		LintScore:   95,
	}

	if !result.Valid {
		t.Errorf("Expected valid to be true")
	}

	if result.TestsPassed != 10 {
		t.Errorf("Expected 10 tests passed")
	}
}

func TestImprovementPhaseConstants(t *testing.T) {
	phases := []ImprovementPhase{
		PhaseSearch,
		PhasePlan,
		PhaseExecute,
		PhaseValidate,
		PhaseReview,
		PhasePR,
		PhaseComplete,
		PhaseFailed,
	}

	if len(phases) != 8 {
		t.Errorf("Expected 8 phases")
	}
}