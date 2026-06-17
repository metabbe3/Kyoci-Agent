package agent

import (
	"context"
	"strings"
	"testing"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// Verification gate + generalized tool filter (Layers A + B)
//
// These tests pin the load-bearing defense against hallucinated file creation.
// The worker's prose answer is NOT trusted until at least one of the paths it
// wrote is confirmed to exist on disk via the file tool's operation=exists.
//
// The gate is tested directly rather than through runWorker because the input
// it depends on (the worker's tool-call history) is straightforward to
// construct by hand, and isolating it from the LLM mock dance makes the
// three failure modes crisp:
//   - worker claimed creation but never invoked file write → fail closed
//   - worker invoked file write but file is missing on disk → fail
//   - worker invoked file write and file exists → pass (out unchanged)
// =============================================================================

// stubFileTool is a minimal Tool implementation whose `exists` output mimics
// the real FileTool.checkExists format ("Path exists: <p> (type: ..., size: N bytes)"
// / "Path does not exist: <p>"). The test controls which paths "exist" and at
// what size via the existPaths / emptyPaths maps.
type stubFileTool struct {
	existPaths map[string]bool // path → exists with non-zero size
	emptyPaths map[string]bool // path → exists but 0 bytes
	calls      []map[string]interface{}
}

func (s *stubFileTool) Name() string        { return "file" }
func (s *stubFileTool) Description() string { return "stub file tool for verification tests" }
func (s *stubFileTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{Name: "operation", Type: "string"},
		{Name: "path", Type: "string"},
		{Name: "content", Type: "string"},
	}
}

func (s *stubFileTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	s.calls = append(s.calls, params)
	op, _ := params["operation"].(string)
	path, _ := params["path"].(string)
	if op == "exists" {
		switch {
		case s.existPaths[path]:
			return "Path exists: " + path + " (type: file, size: 100 bytes)", nil
		case s.emptyPaths[path]:
			return "Path exists: " + path + " (type: file, size: 0 bytes)", nil
		default:
			return "Path does not exist: " + path, nil
		}
	}
	return "ok", nil
}

// newVerificationTestAgent wires up a minimal Agent whose tool registry holds
// only the stub file tool. The LLM provider is irrelevant here — we call
// verifyFileCreation directly.
func newVerificationTestAgent(stub *stubFileTool) *Agent {
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	provider := &sequencerProvider{name: "mock", responses: []*kyoci.CompletionResponse{
		{Content: "unused", FinishReason: kyoci.FinishStop},
	}}
	a := createTestAgent(cfg, provider)
	if err := a.tools.Register(stub); err != nil {
		panic("failed to register stub file tool: " + err.Error())
	}
	return a
}

// msgWithFileWrite builds an assistant message carrying a single `file` tool
// call with the given operation and path. Used to simulate the worker's
// attempted write.
func msgWithFileWrite(op, path string) kyoci.Message {
	args := `{"operation":"` + op + `","path":` + jsonString(path) + `,"content":"hi"}`
	return kyoci.Message{
		Role: kyoci.RoleAssistant,
		ToolCalls: []kyoci.ToolCall{
			{ID: "1", Name: "file", Arguments: args},
		},
	}
}

// jsonString quotes a string for embedding inside hand-built JSON. Good enough
// for the path values used in tests (no escapes needed).
func jsonString(s string) string {
	return `"` + s + `"`
}

// --- Layer B: generalized filterTools + filterToolsForFileCreation ---

func TestFilterTools_Generalized_KeepSubset(t *testing.T) {
	all := sampleToolDefs()
	got := filterTools(all, []string{"file", "terminal", "web_search"})
	gotNames := namesFromDefs(got)
	if !equalStringSet(gotNames, []string{"file", "terminal", "web_search"}) {
		t.Fatalf("filterTools = %v, want [file terminal web_search]", gotNames)
	}
}

func TestFilterTools_Generalized_EmptyKeepReturnsEmpty(t *testing.T) {
	all := sampleToolDefs()
	got := filterTools(all, nil)
	if len(got) != 0 {
		t.Fatalf("filterTools with empty keep = %v, want empty", got)
	}
}

func TestFilterToolsForFileCreation_OnlyFileAndTerminal(t *testing.T) {
	all := sampleToolDefs()
	got := filterToolsForFileCreation(all)
	gotNames := namesFromDefs(got)
	if !equalStringSet(gotNames, []string{"file", "terminal"}) {
		t.Fatalf("filterToolsForFileCreation = %v, want [file terminal]", gotNames)
	}
	// The substitutes the model reaches for MUST be absent.
	for _, banned := range []string{"web_search", "memory_recall", "remember"} {
		if sliceContains(gotNames, banned) {
			t.Fatalf("filterToolsForFileCreation kept banned tool %q", banned)
		}
	}
}

// --- extractWrittenPaths ---

func TestExtractWrittenPaths_CollectsWriteAppendEdit(t *testing.T) {
	messages := []kyoci.Message{
		{Role: kyoci.RoleUser, Content: "task"},
		msgWithFileWrite("write", "config.ini"),
		msgWithFileWrite("append", "logs.txt"),
		msgWithFileWrite("edit", "src/main.go"),
	}
	got := extractWrittenPaths(messages)
	if !equalStringSet(got, []string{"config.ini", "logs.txt", "src/main.go"}) {
		t.Fatalf("extractWrittenPaths = %v, want all three write targets", got)
	}
}

func TestExtractWrittenPaths_IgnoresReadListExists(t *testing.T) {
	messages := []kyoci.Message{
		msgWithFileWrite("read", "should-not-collect.txt"),
		msgWithFileWrite("list", "should-not-collect-dir"),
		msgWithFileWrite("exists", "should-not-collect-either.txt"),
	}
	got := extractWrittenPaths(messages)
	if len(got) != 0 {
		t.Fatalf("extractWrittenPaths = %v, want empty (non-mutating ops must not be collected)", got)
	}
}

func TestExtractWrittenPaths_DeduplicatesPaths(t *testing.T) {
	messages := []kyoci.Message{
		msgWithFileWrite("write", "config.ini"),
		msgWithFileWrite("write", "config.ini"), // same path again
		msgWithFileWrite("append", "config.ini"), // still same path
	}
	got := extractWrittenPaths(messages)
	if len(got) != 1 || got[0] != "config.ini" {
		t.Fatalf("extractWrittenPaths = %v, want [config.ini] (deduped)", got)
	}
}

// --- verifyFileCreation: the three terminal cases ---

// TestVerifyFileCreation_FailClosedWhenNoFileWriteCall covers the single most
// common hallucination pattern: the worker claims "config.ini was generated"
// in its prose answer, but its tool-call history shows no file write/append/edit.
// The gate must fail closed so the synthesizer cannot amplify the claim.
func TestVerifyFileCreation_FailClosedWhenNoFileWriteCall(t *testing.T) {
	stub := &stubFileTool{existPaths: map[string]bool{}, emptyPaths: map[string]bool{}}
	a := newVerificationTestAgent(stub)

	// No assistant messages with file-write tool calls.
	messages := []kyoci.Message{
		{Role: kyoci.RoleUser, Content: "task"},
		{Role: kyoci.RoleAssistant, Content: "I created config.ini."},
	}
	step := OrchStep{ID: 1, Description: "Generate config.ini at the project root."}

	out := a.verifyFileCreation(context.Background(), step, messages, "config.ini was generated")

	if !strings.HasPrefix(out, "[VERIFICATION FAILED") {
		t.Fatalf("expected [VERIFICATION FAILED] prefix; got:\n%s", out)
	}
	if !strings.Contains(out, "no file-write tool calls") {
		t.Errorf("expected 'no file-write tool calls' in failure tag; got:\n%s", out)
	}
	// The original prose must still be present so the synthesizer can quote the
	// claim it's now contradicting.
	if !strings.Contains(out, "config.ini was generated") {
		t.Errorf("expected original out preserved after tag; got:\n%s", out)
	}
}

// TestVerifyFileCreation_FailsWhenFileMissing: the worker did invoke file write,
// but the file is not on disk (e.g., the tool call failed silently or the model
// lied about the path). The gate must catch this.
func TestVerifyFileCreation_FailsWhenFileMissing(t *testing.T) {
	stub := &stubFileTool{
		existPaths: map[string]bool{}, // nothing exists
		emptyPaths: map[string]bool{},
	}
	a := newVerificationTestAgent(stub)

	messages := []kyoci.Message{
		{Role: kyoci.RoleUser, Content: "task"},
		msgWithFileWrite("write", "config.ini"),
		{Role: kyoci.RoleTool, Content: "ok"},
		{Role: kyoci.RoleAssistant, Content: "config.ini was generated"},
	}
	step := OrchStep{ID: 1, Description: "Generate config.ini at the project root."}

	out := a.verifyFileCreation(context.Background(), step, messages, "config.ini was generated")

	if !strings.HasPrefix(out, "[VERIFICATION FAILED") {
		t.Fatalf("expected [VERIFICATION FAILED] prefix; got:\n%s", out)
	}
	if !strings.Contains(out, "none of [config.ini] found on disk") {
		t.Errorf("expected 'none of [config.ini] found on disk' in tag; got:\n%s", out)
	}
}

// TestVerifyFileCreation_PassesWhenFileExists: happy path. The worker wrote the
// file and it actually exists. The gate must leave `out` unchanged so the
// synthesizer sees the worker's evidence-based claim as-is.
func TestVerifyFileCreation_PassesWhenFileExists(t *testing.T) {
	stub := &stubFileTool{
		existPaths: map[string]bool{"config.ini": true},
		emptyPaths: map[string]bool{},
	}
	a := newVerificationTestAgent(stub)

	messages := []kyoci.Message{
		{Role: kyoci.RoleUser, Content: "task"},
		msgWithFileWrite("write", "config.ini"),
		{Role: kyoci.RoleTool, Content: "ok"},
		{Role: kyoci.RoleAssistant, Content: "config.ini was generated"},
	}
	step := OrchStep{ID: 1, Description: "Generate config.ini at the project root."}

	original := "config.ini was generated"
	out := a.verifyFileCreation(context.Background(), step, messages, original)

	if out != original {
		t.Fatalf("expected output unchanged on success; got:\n%s", out)
	}
	// The gate must have probed the file's existence via the stub.
	if len(stub.calls) != 1 {
		t.Fatalf("expected 1 file-tool call (the exists probe); got %d", len(stub.calls))
	}
	if stub.calls[0]["operation"] != "exists" {
		t.Errorf("probe operation = %v, want exists", stub.calls[0]["operation"])
	}
}

// TestVerifyFileCreation_PartialWhenFileEmpty: a sentinel/empty file is suspicious
// enough to flag but not fail closed — some files legitimately have 0 bytes.
func TestVerifyFileCreation_PartialWhenFileEmpty(t *testing.T) {
	stub := &stubFileTool{
		existPaths: map[string]bool{},
		emptyPaths: map[string]bool{"config.ini": true},
	}
	a := newVerificationTestAgent(stub)

	messages := []kyoci.Message{
		msgWithFileWrite("write", "config.ini"),
	}
	step := OrchStep{ID: 1, Description: "Generate config.ini at the project root."}

	out := a.verifyFileCreation(context.Background(), step, messages, "config.ini was generated")

	if !strings.HasPrefix(out, "[VERIFICATION PARTIAL") {
		t.Fatalf("expected [VERIFICATION PARTIAL] prefix; got:\n%s", out)
	}
	if !strings.Contains(out, "empty=[config.ini]") {
		t.Errorf("expected 'empty=[config.ini]' in partial tag; got:\n%s", out)
	}
}

// TestVerifyFileCreation_MixedBatch: when the worker claims multiple writes and
// only some are confirmed, the gate must report which is which so the
// synthesizer can be honest about the partial success.
func TestVerifyFileCreation_MixedBatch(t *testing.T) {
	stub := &stubFileTool{
		existPaths: map[string]bool{"config.ini": true}, // confirmed
		emptyPaths: map[string]bool{},
		// logs.txt → missing
	}
	a := newVerificationTestAgent(stub)

	messages := []kyoci.Message{
		msgWithFileWrite("write", "config.ini"),
		msgWithFileWrite("write", "logs.txt"),
	}
	step := OrchStep{ID: 1, Description: "Generate config.ini and logs.txt at the project root."}

	out := a.verifyFileCreation(context.Background(), step, messages, "both files generated")

	if !strings.HasPrefix(out, "[VERIFICATION PARTIAL") {
		t.Fatalf("expected [VERIFICATION PARTIAL] for mixed batch; got:\n%s", out)
	}
	if !strings.Contains(out, "missing=[logs.txt]") {
		t.Errorf("expected missing=[logs.txt] in partial tag; got:\n%s", out)
	}
	if !strings.Contains(out, "confirmed=[config.ini]") {
		t.Errorf("expected confirmed=[config.ini] in partial tag; got:\n%s", out)
	}
}

// TestRunWorker_FiltersToolsForFileCreationStep is an integration-flavored test:
// it invokes runWorker with a file-creation step description and asserts that
// the LLM request payload exposes ONLY file+terminal (no web_search, no
// memory_recall). This pins Layer B end-to-end.
func TestRunWorker_FiltersToolsForFileCreationStep(t *testing.T) {
	// Register a full-ish tool set on the agent so the filter has something to
	// strip. Use simple stubs that only need to satisfy the Tool interface for
	// registry purposes — they will not actually be called (the mock LLM
	// terminates with a plain-text answer).
	stub := &stubFileTool{existPaths: map[string]bool{"config.ini": true}}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	seq := &sequencerProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			// Worker tries to answer from memory on iter 0 (no tool calls).
			// The evidence nudge then fires; we satisfy it with another
			// memory-only answer so the loop terminates with the [no tool
			// evidence] tag. The tool-def inspection happens on the FIRST
			// request regardless.
			{Content: "I created config.ini.", FinishReason: kyoci.FinishStop},
			{Content: "I created config.ini.", FinishReason: kyoci.FinishStop},
		},
	}
	a := createTestAgent(cfg, seq)
	// Register a few extra tools so the filter has something to strip.
	for _, t := range []kyoci.Tool{
		stub,
		&trivialTool{name: "terminal"},
		&trivialTool{name: "web_search"},
		&trivialTool{name: "memory_recall"},
		&trivialTool{name: "browser"},
	} {
		if err := a.tools.Register(t); err != nil {
			panic("register " + t.Name() + ": " + err.Error())
		}
	}

	step := OrchStep{ID: 1, Description: "Generate config.ini at the project root."}
	if _, err := a.runWorker(context.Background(), "build", step, map[int]string{}); err != nil {
		t.Fatalf("runWorker failed: %v", err)
	}
	if len(seq.captured) == 0 {
		t.Fatalf("no LLM requests captured")
	}
	gotNames := toolDefNames(seq.captured[0].Tools)
	if !equalStringSet(gotNames, []string{"file", "terminal"}) {
		t.Fatalf("worker tool payload = %v, want exactly [file terminal] for file-creation step", gotNames)
	}
}

// trivialTool is the minimal Tool implementation used to populate a registry
// for filtering tests. Its Execute is never called.
type trivialTool struct {
	name string
}

func (t *trivialTool) Name() string        { return t.name }
func (t *trivialTool) Description() string { return "trivial stub" }
func (t *trivialTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{{Name: "x", Type: "string"}}
}
func (t *trivialTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	return "trivial", nil
}

func toolDefNames(defs []kyoci.ToolDefinition) []string {
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Name)
	}
	return out
}
