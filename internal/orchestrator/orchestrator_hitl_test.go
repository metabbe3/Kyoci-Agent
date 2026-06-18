package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"

	"github.com/metabbe3/Kyoci-Agent/internal/hitl"
	"github.com/metabbe3/Kyoci-Agent/internal/memory"
)

// stubRole is a minimal kyoci.Role implementation for retry-loop tests.
// It records every task it sees and returns the configured sequence of
// TaskResults / errors across successive Execute calls.
type stubRole struct {
	mu        sync.Mutex
	seenTasks []string
	responses []stubResponse
	callIdx   int
}

type stubResponse struct {
	content string
	err     error
}

func (s *stubRole) Type() kyoci.RoleType      { return kyoci.RoleCustom }
func (s *stubRole) SystemPrompt() string      { return "" }
func (s *stubRole) Tools() []string           { return nil }
func (s *stubRole) PreferredProvider() string { return "" }
func (s *stubRole) MaxIterations() int        { return 1 }
func (s *stubRole) Execute(_ context.Context, task string, _ kyoci.MemoryStore) (*kyoci.TaskResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seenTasks = append(s.seenTasks, task)
	idx := s.callIdx
	s.callIdx++
	if idx >= len(s.responses) {
		return &kyoci.TaskResult{Content: "default"}, nil
	}
	r := s.responses[idx]
	if r.err != nil {
		return nil, r.err
	}
	return &kyoci.TaskResult{Content: r.content}, nil
}

func (s *stubRole) SeenTasks() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.seenTasks))
	copy(out, s.seenTasks)
	return out
}

// stubHITLHook captures the requests it receives and returns the configured hint.
type stubHITLHook struct {
	mu       sync.Mutex
	requests []hitl.HelpRequest
	hint     string
	err      error
}

func (h *stubHITLHook) RequestHelp(_ context.Context, req hitl.HelpRequest) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requests = append(h.requests, req)
	if h.err != nil {
		return "", h.err
	}
	return h.hint, nil
}

func (h *stubHITLHook) RequestCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.requests)
}

// newTestOrchestrator builds a minimal Orchestrator wired for hitl-loop tests.
// It uses a temp SQLite DB so the lesson-recording path can be exercised
// end-to-end without touching the production DB.
func newTestOrchestrator(t *testing.T, role kyoci.Role, hook hitl.HITLHook, maxRetries int) (*Orchestrator, func()) {
	t.Helper()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")

	// We can't easily construct a full Orchestrator without subsystems, so
	// build a partial one with only the fields executeWithRetry needs.
	// Role lookup goes through roleRegistry — for the test we bypass it by
	// passing the role directly to executeWithRetry.
	o := &Orchestrator{
		logger:       testLogger(t),
		shutdownChan: make(chan struct{}),
	}
	o.started = true

	// Set up a real LongTermMemory so RecordLesson actually persists.
	ltm, err := memory.NewLongTermMemory(dbPath, testLogger(t))
	if err != nil {
		t.Fatalf("NewLongTermMemory: %v", err)
	}
	o.reflectionEngine = memory.NewReflectionEngine(ltm, nil, testLogger(t))

	o.hitlCfg = &HITLConfig{MaxRetries: maxRetries, Hook: hook}

	cleanup := func() {
		_ = ltm.Close()
	}
	return o, cleanup
}

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	if testing.Verbose() {
		return slog.Default()
	}
	// Discard logs in non-verbose mode.
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ---- Helper-function tests ----

func TestExtractVerifyCommand(t *testing.T) {
	tests := []struct {
		name string
		task string
		want string
	}{
		{"no directive", "fix the bug in calculator.go", ""},
		{"simple directive", "fix the bug\nVERIFY: go test ./...", "go test ./..."},
		{"directive with path", "fix it\nVERIFY: cd app_test_env && go test ./...", "cd app_test_env && go test ./..."},
		{"lowercase verify", "fix it\nverify: go test", "go test"},
		{"indented verify", "fix it\n  VERIFY: go test", "go test"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractVerifyCommand(tc.task)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStripVerifyDirective(t *testing.T) {
	task := "fix the bug\nVERIFY: go test ./...\ndocument the fix"
	stripped := stripVerifyDirective(task)
	if strings.Contains(stripped, "VERIFY") {
		t.Errorf("VERIFY not stripped: %q", stripped)
	}
	if !strings.Contains(stripped, "fix the bug") {
		t.Errorf("lost non-VERIFY content: %q", stripped)
	}
	if !strings.Contains(stripped, "document the fix") {
		t.Errorf("lost non-VERIFY content: %q", stripped)
	}
}

// ---- Retry-loop tests ----

// TestExecuteWithRetry_NoVerifyDirective verifies that tasks without a VERIFY
// directive take the single-shot fast path — the stub role is called exactly
// once, regardless of MaxRetries.
func TestExecuteWithRetry_NoVerifyDirective(t *testing.T) {
	role := &stubRole{responses: []stubResponse{{content: "done"}}}
	hook := &stubHITLHook{hint: "no need"}
	o, cleanup := newTestOrchestrator(t, role, hook, 3)
	defer cleanup()

	_, err := o.executeWithRetry(context.Background(), "fix the bug", role, kyoci.RoleCustom)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(role.SeenTasks()) != 1 {
		t.Errorf("expected 1 attempt, got %d", len(role.SeenTasks()))
	}
	if hook.RequestCount() != 0 {
		t.Errorf("HITL should not be invoked, got %d calls", hook.RequestCount())
	}
}

// TestExecuteWithRetry_VerifyPasses verifies that a passing VERIFY command
// returns immediately on the first attempt — no retries, no HITL.
func TestExecuteWithRetry_VerifyPasses(t *testing.T) {
	role := &stubRole{responses: []stubResponse{{content: "fix applied"}}}
	hook := &stubHITLHook{}

	tmp := t.TempDir()
	// A verify command that always succeeds.
	verifyCmd := fmt.Sprintf("echo success > %s/ok && test -f %s/ok", tmp, tmp)
	task := "fix the bug\nVERIFY: " + verifyCmd

	o, cleanup := newTestOrchestrator(t, role, hook, 3)
	defer cleanup()

	result, err := o.executeWithRetry(context.Background(), task, role, kyoci.RoleCustom)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(role.SeenTasks()) != 1 {
		t.Errorf("expected 1 attempt, got %d", len(role.SeenTasks()))
	}
	if hook.RequestCount() != 0 {
		t.Errorf("HITL should not be invoked, got %d calls", hook.RequestCount())
	}
	if !strings.Contains(result.Content, "fix applied") {
		t.Errorf("expected agent content returned, got %q", result.Content)
	}
}

// TestExecuteWithRetry_HITLSuccess verifies the canonical L4 flow:
//
//	attempt 1: fail
//	attempt 2: fail (= preHITLAttempts when MaxRetries=1)
//	HITL: hook called, hint returned
//	attempt 3: succeed
//	→ lesson recorded
func TestExecuteWithRetry_HITLSuccess(t *testing.T) {
	// Build a verify command that fails on attempts 1-2 and passes on attempt 3.
	// We use a file-based sentinel: the test's third call removes the file,
	// letting the verify command succeed.
	tmp := t.TempDir()
	sentinel := filepath.Join(tmp, "failing")
	f, _ := os.Create(sentinel)
	f.Close()

	// Verify fails while sentinel exists.
	verifyCmd := fmt.Sprintf("! test -f %s", sentinel)

	// Stub role does nothing — the verify command alone drives pass/fail.
	// But we need to remove the sentinel on the post-HITL attempt so verify
	// passes. Use the task content to detect which attempt we're on.
	var roleMu sync.Mutex
	roleCalls := 0
	role := &stubRole{}
	role.responses = []stubResponse{
		{content: "fix attempt 1"},
		{content: "fix attempt 2"},
		{content: "fix attempt 3 (post-HITL)"},
	}
	// Wrap Execute via a custom role impl that also clears the sentinel on
	// attempt 3.
	wrappedRole := &wrappingRole{
		inner: role,
		onCall: func(task string) {
			roleMu.Lock()
			defer roleMu.Unlock()
			roleCalls++
			// On the 3rd call (post-HITL attempt), make the verify pass.
			if roleCalls == 3 {
				_ = os.Remove(sentinel)
			}
		},
	}

	hook := &stubHITLHook{hint: "use + not *"}
	o, cleanup := newTestOrchestrator(t, wrappedRole, hook, 1)
	defer cleanup()

	result, err := o.executeWithRetry(
		context.Background(),
		"fix the bug in calculator.go\nVERIFY: "+verifyCmd,
		wrappedRole,
		kyoci.RoleCustom,
	)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if got := len(role.SeenTasks()); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
	if hook.RequestCount() != 1 {
		t.Errorf("expected exactly 1 HITL request, got %d", hook.RequestCount())
	}

	// The third task text should contain the hint.
	thirdTask := role.SeenTasks()[2]
	if !strings.Contains(thirdTask, "use + not *") {
		t.Errorf("post-HITL task should contain hint; got: %q", thirdTask)
	}
	if !strings.Contains(thirdTask, "HINT FROM HUMAN OPERATOR") {
		t.Errorf("post-HITL task should contain HINT marker; got prefix: %q", truncateForHitl(thirdTask, 200))
	}

	// Second task should contain "PREVIOUS ATTEMPT FAILED".
	secondTask := role.SeenTasks()[1]
	if !strings.Contains(secondTask, "PREVIOUS ATTEMPT") {
		t.Errorf("retry task should contain PREVIOUS ATTEMPT marker; got: %q", truncateForHitl(secondTask, 200))
	}

	// Lesson should have been recorded in L3 SQLite.
	if !resultHasLessonRecorded(o, t, "calculator") {
		t.Errorf("expected a lesson row mentioning 'calculator' in L3 memory")
	}
}

// TestExecuteWithRetry_HITLStillFails verifies the exhaustion path:
//
//	attempt 1: fail
//	attempt 2: fail
//	HITL: hook called
//	attempt 3: still fail
//	→ returns ErrHITLExhausted
func TestExecuteWithRetry_HITLStillFails(t *testing.T) {
	tmp := t.TempDir()
	sentinel := filepath.Join(tmp, "always-failing")
	f, _ := os.Create(sentinel)
	f.Close()
	defer os.Remove(sentinel)

	verifyCmd := fmt.Sprintf("! test -f %s", sentinel) // always fails

	role := &stubRole{responses: []stubResponse{
		{content: "fix 1"}, {content: "fix 2"}, {content: "fix 3"},
	}}
	hook := &stubHITLHook{hint: "ignored hint"}
	o, cleanup := newTestOrchestrator(t, role, hook, 1)
	defer cleanup()

	_, err := o.executeWithRetry(
		context.Background(),
		"fix bug\nVERIFY: "+verifyCmd,
		role,
		kyoci.RoleCustom,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var exhausted *ErrHITLExhausted
	if !errors.As(err, &exhausted) {
		t.Errorf("expected ErrHITLExhausted, got %T: %v", err, err)
	}
	if hook.RequestCount() != 1 {
		t.Errorf("expected 1 HITL call, got %d", hook.RequestCount())
	}
	if len(role.SeenTasks()) != 3 {
		t.Errorf("expected 3 attempts (2 pre-HITL + 1 post-HITL), got %d", len(role.SeenTasks()))
	}
}

// TestExecuteWithRetry_NoHook verifies that without a HITL hook, the loop
// simply runs MaxRetries+1 attempts and returns ErrVerifyFailed on exhaustion.
func TestExecuteWithRetry_NoHook(t *testing.T) {
	tmp := t.TempDir()
	sentinel := filepath.Join(tmp, "failing")
	f, _ := os.Create(sentinel)
	f.Close()
	defer os.Remove(sentinel)

	verifyCmd := fmt.Sprintf("! test -f %s", sentinel)

	role := &stubRole{responses: []stubResponse{
		{content: "1"}, {content: "2"}, {content: "3"},
	}}
	o, cleanup := newTestOrchestrator(t, role, nil, 2)
	defer cleanup()

	_, err := o.executeWithRetry(
		context.Background(),
		"fix bug\nVERIFY: "+verifyCmd,
		role,
		kyoci.RoleCustom,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var verifyErr *ErrVerifyFailed
	if !errors.As(err, &verifyErr) {
		t.Errorf("expected ErrVerifyFailed, got %T: %v", err, err)
	}
	if len(role.SeenTasks()) != 3 { // 1 + maxRetries(2)
		t.Errorf("expected 3 attempts, got %d", len(role.SeenTasks()))
	}
}

// TestExecuteWithRetry_HITLNoSubscriber verifies the ErrNoSubscriber fast-fail:
// when the hook returns ErrNoSubscriber, the orchestrator gives up immediately
// rather than blocking on the timeout.
func TestExecuteWithRetry_HITLNoSubscriber(t *testing.T) {
	tmp := t.TempDir()
	sentinel := filepath.Join(tmp, "failing")
	f, _ := os.Create(sentinel)
	f.Close()
	defer os.Remove(sentinel)

	verifyCmd := fmt.Sprintf("! test -f %s", sentinel)

	role := &stubRole{responses: []stubResponse{{content: "1"}, {content: "2"}}}
	hook := &stubHITLHook{err: hitl.ErrNoSubscriber}
	o, cleanup := newTestOrchestrator(t, role, hook, 1)
	defer cleanup()

	start := time.Now()
	_, err := o.executeWithRetry(
		context.Background(),
		"fix bug\nVERIFY: "+verifyCmd,
		role,
		kyoci.RoleCustom,
	)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("ErrNoSubscriber should fail fast; took %v", elapsed)
	}
	if !strings.Contains(err.Error(), "HITL unavailable") {
		t.Errorf("error should mention HITL unavailable; got %v", err)
	}
}

// ---- helpers ----

// wrappingRole lets the test inject side effects (like clearing a sentinel)
// between the stub role's Execute calls.
type wrappingRole struct {
	inner  kyoci.Role
	onCall func(task string)
}

func (w *wrappingRole) Type() kyoci.RoleType      { return w.inner.Type() }
func (w *wrappingRole) SystemPrompt() string      { return w.inner.SystemPrompt() }
func (w *wrappingRole) Tools() []string           { return w.inner.Tools() }
func (w *wrappingRole) PreferredProvider() string { return w.inner.PreferredProvider() }
func (w *wrappingRole) MaxIterations() int        { return w.inner.MaxIterations() }
func (w *wrappingRole) Execute(ctx context.Context, task string, m kyoci.MemoryStore) (*kyoci.TaskResult, error) {
	if w.onCall != nil {
		w.onCall(task)
	}
	return w.inner.Execute(ctx, task, m)
}

// resultHasLessonRecorded queries L3 memory for any lesson row whose content
// contains needle. Used to verify the self-learning step recorded a permanent entry.
func resultHasLessonRecorded(o *Orchestrator, t *testing.T, needle string) bool {
	t.Helper()
	if o.reflectionEngine == nil {
		return false
	}
	got := o.reflectionEngine.GetRelevantLessons(context.Background(), needle, 20)
	return strings.Contains(got, needle)
}
