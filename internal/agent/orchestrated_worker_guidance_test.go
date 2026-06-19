package agent

import (
	"context"
	"strings"
	"testing"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// Worker output guardrails (Fix A + Fix B)
//
// Fix B — WorkerMaxTokens: a single worker completion must not blow the session
// time budget. The L3 benchmark observed a worker generating 8,192 tokens
// (192s) on one step, which single-handedly exhausted the 360s webhook cap.
// A per-worker token cap (default 4096) halves that worst case while leaving
// room for file content passed as tool-call arguments.
//
// Fix A — file-creation directive: when a step description expresses a
// creation intent (create/write/generate + a filename), the worker must be
// told explicitly to use the `file` tool with operation=write. Without it,
// gemma4:12b substitutes low-effort ops (list/search/recall) and never writes
// the artifact — the same behavioral defect observed for MCP tools before
// Go-side enforcement was added.
// =============================================================================

// --- Fix B: per-worker token cap ---

// TestRunWorker_CapsMaxTokensAtWorkerBudget verifies that runWorker uses the
// WorkerMaxTokens budget (not the global, larger MaxTokens) when building each
// CompletionRequest. This is the load-bearing fix for the 8,192-token blowup
// that exhausted the benchmark's 360s session cap.
func TestRunWorker_CapsMaxTokensAtWorkerBudget(t *testing.T) {
	seq := &sequencerProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			{Content: "done", FinishReason: kyoci.FinishStop},
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	cfg.MaxTokens = 8192                 // global cap (the blowup observed in prod)
	cfg.Orchestration.WorkerMaxTokens = 4096 // per-worker budget
	a := createTestAgent(cfg, seq)

	step := OrchStep{ID: 1, Description: "do something", ToolHint: ""}
	if _, err := a.runWorker(context.Background(), "task", step, map[int]string{}); err != nil {
		t.Fatalf("runWorker failed: %v", err)
	}
	if len(seq.captured) == 0 {
		t.Fatalf("no LLM requests captured")
	}
	got := seq.captured[0].MaxTokens
	if got != 4096 {
		t.Fatalf("worker MaxTokens = %d, want 4096 (worker budget must override global 8192)", got)
	}
}

// TestEffectiveOrchConfig_DefaultsWorkerMaxTokens verifies the default kicks in
// when the field is left zero (the normal case for existing configs).
func TestEffectiveOrchConfig_DefaultsWorkerMaxTokens(t *testing.T) {
	cfg := DefaultOrchestratorConfig()
	if cfg.WorkerMaxTokens <= 0 {
		t.Fatalf("DefaultOrchestratorConfig WorkerMaxTokens = %d, want a positive default", cfg.WorkerMaxTokens)
	}

	a := &Agent{config: AgentConfig{Orchestration: OrchestratorConfig{Enabled: true}}}
	got := a.effectiveOrchConfig().WorkerMaxTokens
	if got <= 0 {
		t.Fatalf("effectiveOrchConfig WorkerMaxTokens = %d, want the default (was left zero)", got)
	}
}

// --- Fix A: file-creation step detection ---

func TestIsFileCreationStep_DetectsCreateWithFilename(t *testing.T) {
	cases := []string{
		"Create app_test_env/main.go with Go data models and a net/http handler.",
		"Write the index.html file with the landing page markup.",
		"Generate a new user_service.go containing the repository layer.",
		"Initialize main.go for the user profile service.",
		"Implement the handler in app_test_env/main.go.",
	}
	for _, desc := range cases {
		if !isFileCreationStep(desc) {
			t.Errorf("isFileCreationStep(%q) = false, want true", desc)
		}
	}
}

func TestIsFileCreationStep_FalseForReadAndExploreSteps(t *testing.T) {
	cases := []string{
		"Read the config file.",
		"List the contents of app_test_env/.",
		"Search for existing user schemas.",
		"Fetch the user schema via the MCP tool.",
		"Recall preferences from memory.",
		"Summarize the findings.",
	}
	for _, desc := range cases {
		if isFileCreationStep(desc) {
			t.Errorf("isFileCreationStep(%q) = true, want false (non-creation step must not trigger the directive)", desc)
		}
	}
}

// TestRunWorker_InjectsFileCreationDirective verifies that when a step expresses
// creation intent, the worker's SYSTEM prompt includes the EXECUTION-MODE
// directive — steering the model away from prose-first descriptions and toward
// file:write tool calls. The directive lives in the system prompt (not the
// user message) because models pay 3-5× more attention to system content.
func TestRunWorker_InjectsFileCreationDirective(t *testing.T) {
	seq := &sequencerProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			{Content: "done", FinishReason: kyoci.FinishStop},
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	a := createTestAgent(cfg, seq)

	step := OrchStep{ID: 1, Description: "Create app_test_env/main.go with Go data models and a net/http handler.", ToolHint: "file"}
	if _, err := a.runWorker(context.Background(), "build the service", step, map[int]string{}); err != nil {
		t.Fatalf("runWorker failed: %v", err)
	}
	if len(seq.captured) == 0 {
		t.Fatalf("no LLM requests captured")
	}
	sysMsg := seq.captured[0].Messages[0].Content
	if !strings.Contains(sysMsg, "EXECUTION MODE") {
		t.Errorf("file-creation directive missing from worker SYSTEM prompt;\nsysMsg tail:\n%s", sysMsg[len(sysMsg)-min(400, len(sysMsg)):])
	}
}

// TestRunWorker_NoDirectiveForNonCreationSteps verifies the directive is absent
// for pure-read steps so we don't pollute exploration logic.
func TestRunWorker_NoDirectiveForNonCreationSteps(t *testing.T) {
	seq := &sequencerProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			{Content: "done", FinishReason: kyoci.FinishStop},
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	a := createTestAgent(cfg, seq)

	step := OrchStep{ID: 1, Description: "List the contents of app_test_env/.", ToolHint: "file"}
	if _, err := a.runWorker(context.Background(), "explore", step, map[int]string{}); err != nil {
		t.Fatalf("runWorker failed: %v", err)
	}
	sysMsg := seq.captured[0].Messages[0].Content
	if strings.Contains(sysMsg, "EXECUTION MODE") {
		t.Errorf("file-creation directive leaked into a non-creation step SYSTEM prompt;\nsysMsg:\n%s", sysMsg)
	}
}
