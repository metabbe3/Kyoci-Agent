package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/metabbe3/Kyoci-Agent/internal/llm"
	"github.com/metabbe3/Kyoci-Agent/internal/skill"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Skill fast-path integration test (the capstone).
//
// Wires up the REAL production skill registry (all 125 skills) and verifies
// that when the planner emits tool_hint="skill" for a natural-language query,
// the orchestrator routes through the skill fast-path — NO worker LLM call
// is made. This proves the agent can auto-use the new skills.
//
// The planner output is injected via a custom mock provider so the test runs
// with zero LLM access. A sentinel worker is installed via
// setOrchWorkerForTest that fails the test if invoked.
// =====================================================================================

// dispatchProvider returns canned planner output based on the user-message
// content of the incoming request. This lets one provider instance answer
// many different "planner for task X" requests in the same test run.
//
// The first request for any task returns the planner's JSON step array.
// The SECOND request for the same task (the synthesizer pass-through) returns
// a generic "Skill output: <X>" acknowledgement — we keep the synthesizer
// call cheap since the test only cares that the skill ran, not what the
// synthesizer composed.
type dispatchProvider struct {
	mu            sync.Mutex
	name          string
	plannerOutput map[string]string // task-prefix → JSON plan
	callCount     int
}

func newDispatchProvider() *dispatchProvider {
	return &dispatchProvider{
		name: "dispatch",
		plannerOutput: map[string]string{
			"sha256 of hello":              `[{"id":1,"description":"sha256 of hello","depends_on":[],"tool_hint":"skill"}]`,
			"base64 encode: hello":         `[{"id":1,"description":"base64 encode: hello","depends_on":[],"tool_hint":"skill"}]`,
			"yaml to json: name: Bob":      `[{"id":1,"description":"yaml to json: name: Bob","depends_on":[],"tool_hint":"skill"}]`,
			"generate uuid v4":             `[{"id":1,"description":"generate uuid v4","depends_on":[],"tool_hint":"skill"}]`,
			"contrast ratio between":       `[{"id":1,"description":"contrast ratio between #fff #000","depends_on":[],"tool_hint":"skill"}]`,
			"stats for":                    `[{"id":1,"description":"stats for 1 2 3 4 5","depends_on":[],"tool_hint":"skill"}]`,
			"slugify":                      `[{"id":1,"description":"slugify: Hello World!","depends_on":[],"tool_hint":"skill"}]`,
			"md5 of":                       `[{"id":1,"description":"md5 of hello","depends_on":[],"tool_hint":"skill"}]`,
			"identify hash":                `[{"id":1,"description":"identify hash: 5d41402abc4b2a76b9719d911017c592","depends_on":[],"tool_hint":"skill"}]`,
			"now":                          `[{"id":1,"description":"what time is it now","depends_on":[],"tool_hint":"skill"}]`,
		},
	}
}

func (p *dispatchProvider) Name() string { return p.name }
func (p *dispatchProvider) IsAvailable() bool { return true }
func (p *dispatchProvider) Models() []kyoci.ModelInfo {
	return []kyoci.ModelInfo{{ID: "dispatch-mock", Provider: p.name}}
}
func (p *dispatchProvider) Stream(_ context.Context, _ kyoci.CompletionRequest) (<-chan kyoci.StreamChunk, error) {
	return nil, fmt.Errorf("streaming not supported")
}

// Complete inspects the request's user message and dispatches to the
// configured planner output. The synthesizer pass-through (second call for
// the same task) gets a generic ack.
func (p *dispatchProvider) Complete(_ context.Context, req kyoci.CompletionRequest) (*kyoci.CompletionResponse, error) {
	p.mu.Lock()
	p.callCount++
	count := p.callCount
	p.mu.Unlock()

	// Reconstruct the task from the user messages.
	task := ""
	for _, m := range req.Messages {
		if m.Role == kyoci.RoleUser {
			task = m.Content
			break
		}
	}
	taskLower := strings.ToLower(task)

	// Planner call (first call for this task) — return the matching plan.
	for prefix, plan := range p.plannerOutput {
		if strings.HasPrefix(taskLower, prefix) || strings.Contains(taskLower, prefix) {
			// If this is the synthesizer call (ToolChoice none, no tools),
			// return a generic ack instead.
			if (req.ToolChoice == "none" || len(req.Tools) == 0) && count >= 2 {
				return &kyoci.CompletionResponse{
					Content:      "Skill output: " + task,
					FinishReason: kyoci.FinishStop,
				}, nil
			}
			return &kyoci.CompletionResponse{
				Content:      plan,
				FinishReason: kyoci.FinishStop,
			}, nil
		}
	}

	// Fallback synthesizer ack.
	return &kyoci.CompletionResponse{
		Content:      "Skill output: " + task,
		FinishReason: kyoci.FinishStop,
	}, nil
}

// TestSkillFastPath_RealCatalog verifies that for 10 representative
// natural-language queries spanning every skill category, the orchestrator
// routes through the skill fast-path and NEVER calls the worker LLM.
func TestSkillFastPath_RealCatalog(t *testing.T) {
	// 1. Real production skill registry.
	realReg := skill.NewRegistry()
	if err := realReg.RegisterBuiltin(); err != nil {
		t.Fatalf("RegisterBuiltin: %v", err)
	}

	// 2. Build the agent via the existing test helper.
	provider := newDispatchProvider()
	providerRegistry := llm.NewProviderRegistry()
	providerRegistry.Register("dispatch", provider)
	router := llm.NewRouter(providerRegistry, llm.StrategyFallback)

	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	cfg.MaxIterations = 3
	cfg.Orchestration.WorkerMaxIterations = 2

	tools := kyoci.NewToolRegistry()
	memory := &mockMemory{}
	ag := NewAgent(cfg, router, tools, realReg.Kyoci(), memory)

	// 3. Sentinel worker — if invoked, the skill fast-path failed to fire.
	workerInvoked := false
	ag.setOrchWorkerForTest(func(_ context.Context, _ string, step OrchStep, _ map[int]string) (string, error) {
		workerInvoked = true
		return "", fmt.Errorf("sentinel: worker should NOT have been called for step %d (tool_hint=%q) — skill fast-path failed", step.ID, step.ToolHint)
	})

	// 4. Representative scenarios — one per major skill category.
	scenarios := []struct {
		name           string
		task           string
		wantFragment   string // expected substring in the agent's final output
		skipOnNoLLM    bool   // skip if the skill needs a real LLM call (none do today)
	}{
		{"hashing: sha256", "sha256 of hello", "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", false},
		{"hashing: md5", "md5 of hello", "5d41402abc4b2a76b9719d911017c592", false},
		{"encoding: base64", "base64 encode: hello", "aGVsbG8=", false},
		{"datafmt: yaml_to_json", "yaml to json: name: Bob", `"Bob"`, false},
		{"generators: uuid_v4", "generate uuid v4", "-", false}, // format check (random)
		{"color: contrast_ratio", "contrast ratio between #fff #000", "21", false},
		{"math: stats", "stats for 1 2 3 4 5", "mean:", false},
		{"text: slugify", "slugify: Hello World!", "hello-world", false},
		{"security: hash_identify", "identify hash: 5d41402abc4b2a76b9719d911017c592", "MD5", false},
		{"time: now", "now", "T", false}, // ISO 8601 contains 'T'
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			if sc.skipOnNoLLM {
				t.Skip("scenario requires external resource")
			}
			workerInvoked = false
			result, err := ag.Execute(context.Background(), sc.task)
			if err != nil {
				t.Fatalf("Execute(%q): %v", sc.task, err)
			}
			if workerInvoked {
				t.Fatalf("worker was invoked for %q — skill fast-path did NOT fire", sc.task)
			}
			if result == nil {
				t.Fatalf("nil result")
			}
			if !strings.Contains(result.Content, sc.wantFragment) {
				t.Errorf("output missing %q\n got: %q", sc.wantFragment, result.Content)
			}
		})
	}
}

// TestSkillFastPath_FallbackToWorkerOnNoMatch verifies the inverse: when the
// planner emits tool_hint="skill" but no skill actually Matches the task,
// the orchestrator DOES fall back to the worker (proving the sentinel path
// is conditional on a real match, not unconditional).
func TestSkillFastPath_FallbackToWorkerOnNoMatch(t *testing.T) {
	// Empty skill registry — no skill will Match.
	emptyReg := kyoci.NewSkillRegistry()

	provider := newDispatchProvider()
	provider.plannerOutput = map[string]string{
		// A task no skill matches — the worker MUST be called.
		"unmatchable": `[{"id":1,"description":"unmatchable task","depends_on":[],"tool_hint":"skill"}]`,
	}
	// Override Complete to also handle the worker call (since the worker
	// path requires LLM response too). Wrap the dispatch in a fallback that
	// returns "worker ran" for non-planner calls.
	providerRegistry := llm.NewProviderRegistry()
	providerRegistry.Register("dispatch", provider)
	router := llm.NewRouter(providerRegistry, llm.StrategyFallback)

	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	tools := kyoci.NewToolRegistry()
	ag := NewAgent(cfg, router, tools, emptyReg, &mockMemory{})

	// Real worker this time — record that it ran.
	workerRan := false
	ag.setOrchWorkerForTest(func(_ context.Context, _ string, _ OrchStep, _ map[int]string) (string, error) {
		workerRan = true
		return "worker handled the step", nil
	})

	_, err := ag.Execute(context.Background(), "unmatchable task that no skill claims")
	if err != nil {
		// Some error path may fire (synth failure, etc.) — what matters is
		// whether the worker ran.
		_ = err
	}
	if !workerRan {
		t.Error("expected worker to run when no skill matches; fast-path should have fallen through")
	}
}
