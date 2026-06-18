package orchestrator

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/config"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// TestRunLogPath_FormatsDatePartitioned verifies the on-disk layout the
// manifest's log_path field needs to point at: logs/<YYYY-MM-DD>/run_<id>.log.
func TestRunLogPath_FormatsDatePartitioned(t *testing.T) {
	cfg := &config.Config{Logging: config.LogConfig{PerRunEnabled: true, PerRunDir: "logs"}}
	now := time.Date(2026, 6, 18, 14, 30, 0, 0, time.UTC)
	got := RunLogPath(cfg, "01ABC", now)
	want := filepath.Join("logs", "2026-06-18", "run_01ABC.log")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestRunLogPath_DisabledReturnsEmpty verifies the no-op path the
// orchestrator relies on when per-run logging is off.
func TestRunLogPath_DisabledReturnsEmpty(t *testing.T) {
	cfg := &config.Config{Logging: config.LogConfig{PerRunEnabled: false, PerRunDir: "logs"}}
	if got := RunLogPath(cfg, "01ABC", time.Now()); got != "" {
		t.Fatalf("expected empty when disabled, got %q", got)
	}
	cfg.Logging.PerRunEnabled = true
	cfg.Logging.PerRunDir = ""
	if got := RunLogPath(cfg, "01ABC", time.Now()); got != "" {
		t.Fatalf("expected empty when dir empty, got %q", got)
	}
}

// TestOpenRunLogger_CreatesFileAndWrites verifies the happy path: opening
// the per-run logger creates the date-partitioned dir + file and a slog
// Info call lands in the file.
func TestOpenRunLogger_CreatesFileAndWrites(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Logging: config.LogConfig{
		PerRunEnabled: true,
		PerRunDir:     filepath.Join(root, "logs"),
		Level:         "info",
	}}
	now := time.Date(2026, 6, 18, 14, 30, 0, 0, time.UTC)

	logger, closer, logPath := OpenRunLogger(cfg, "task-1", now)
	defer closer()

	if logPath == "" {
		t.Fatal("expected non-empty logPath")
	}
	logger.Info("hello from test", "task_id", "task-1")

	// Close before reading so the buffer is flushed.
	closer()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "hello from test") {
		t.Fatalf("log file missing message; contents:\n%s", data)
	}
	if !strings.Contains(string(data), "task-1") {
		t.Fatalf("log file missing task_id; contents:\n%s", data)
	}
}

// TestOpenRunLogger_DisabledReturnsDefaultLogger verifies the fast path used
// in hermetic tests and when operators explicitly turn off per-run logging —
// returns slog.Default() with a no-op closer and empty path.
func TestOpenRunLogger_DisabledReturnsDefaultLogger(t *testing.T) {
	cfg := &config.Config{Logging: config.LogConfig{PerRunEnabled: false, PerRunDir: "logs"}}
	logger, closer, logPath := OpenRunLogger(cfg, "task-1", time.Now())
	defer closer()
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	if logPath != "" {
		t.Fatalf("expected empty logPath when disabled, got %q", logPath)
	}
}

// TestOpenRunLogger_PermissionDeniedFallsBackToStdout confirms the
// "must never fail the task" contract: if the log dir can't be created (here
// simulated by pointing PerRunDir under a non-existent nested path under a
// file we don't have permission to traverse), OpenRunLogger still returns a
// working logger and a no-op closer.
func TestOpenRunLogger_PermissionDeniedFallsBackToStdout(t *testing.T) {
	// "/dev/null/x" cannot be created as a directory — /dev/null is a char device.
	cfg := &config.Config{Logging: config.LogConfig{
		PerRunEnabled: true,
		PerRunDir:     "/dev/null/cannot/exist",
		Level:         "info",
	}}
	logger, closer, logPath := OpenRunLogger(cfg, "task-1", time.Now())
	defer closer()
	if logger == nil {
		t.Fatal("expected non-nil fallback logger")
	}
	if logPath != "" {
		t.Fatalf("expected empty logPath on fallback, got %q", logPath)
	}
	// Logger should still be usable.
	logger.Info("fallback path still logs")
}

// TestWithRunLogger_RoundTrip covers the ctx plumbing the orchestrator uses
// to expose the per-run logger to worker goroutines.
func TestWithRunLogger_RoundTrip(t *testing.T) {
	ctx := context.Background()
	if got := RunLoggerFromCtx(ctx); got != nil {
		t.Fatalf("expected nil before WithRunLogger, got %v", got)
	}
	l := slog.Default()
	ctx2 := WithRunLogger(ctx, l)
	if got := RunLoggerFromCtx(ctx2); got != l {
		t.Fatal("expected same logger pointer back from ctx")
	}
}

// TestExtractFilesWritten_PicksFileWrites verifies the manifest logic that
// scans TaskResult.ToolCallLog for the `file` tool with write/append/edit
// operations.
func TestExtractFilesWritten_PicksFileWrites(t *testing.T) {
	entries := []kyociEntry{
		{Tool: "file", Args: `{"operation":"write","path":"/tmp/a.go","content":"x"}`},
		{Tool: "file", Args: `{"operation":"read","path":"/tmp/b.go"}`},   // ignored: read
		{Tool: "file", Args: `{"operation":"append","path":"/tmp/c.go"}`}, // picked: append
		{Tool: "file", Args: `{"operation":"edit","path":"/tmp/a.go"}`},   // dedup: same path
		{Tool: "grep", Args: `{}`},                                 // ignored: not file
		{Tool: "file", Args: `{"operation":"list","path":"/tmp"}`}, // ignored: list
		{Tool: "file", Args: `not-json`},                           // ignored: bad JSON
	}
	got := extractFilesWritten(toKyociEntries(entries))
	want := []string{"/tmp/a.go", "/tmp/c.go"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, p := range want {
		if got[i] != p {
			t.Errorf("idx %d: got %q, want %q", i, got[i], p)
		}
	}
}

// TestExtractFilesWritten_EmptyForResearchTasks is the path that triggers
// CleanupIfEmpty — research/Q&A tasks produce no file tool writes.
func TestExtractFilesWritten_EmptyForResearchTasks(t *testing.T) {
	got := extractFilesWritten(nil)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
	got = extractFilesWritten(toKyociEntries([]kyociEntry{
		{Tool: "http", Args: `{"method":"GET","url":"https://example.com"}`},
		{Tool: "search", Args: `{"query":"foo"}`},
	}))
	if len(got) != 0 {
		t.Fatalf("expected empty for non-file tool calls, got %v", got)
	}
}

// TestWriteManifest_WritesJsonAtTaskRoot verifies the on-disk layout:
// tasks/<id>/manifest.json, written atomically via tmp-then-rename.
func TestWriteManifest_WritesJsonAtTaskRoot(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Tasks: config.TasksConfig{Dir: filepath.Join(root, "tasks")}}
	m := taskManifest{
		TaskID:       "abc",
		Role:         "developer",
		Task:         "do the thing",
		Status:       "completed",
		FilesCreated: []string{"/abs/path/out.go"},
		LogPath:      "logs/2026-06-18/run_abc.log",
	}
	if err := writeManifest(cfg, "abc", m); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}
	path := filepath.Join(cfg.Tasks.Dir, "abc", "manifest.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("manifest not written: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	body := string(data)
	for _, want := range []string{"\"task_id\": \"abc\"", "\"role\": \"developer\"", "/abs/path/out.go"} {
		if !strings.Contains(body, want) {
			t.Errorf("manifest missing %q\nbody:\n%s", want, body)
		}
	}
	// No leftover .tmp file — rename succeeded.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("leftover tmp file: %v", err)
	}
}

// TestWriteManifest_NoopWhenDisabled verifies graceful skip when workspaces
// are off.
func TestWriteManifest_NoopWhenDisabled(t *testing.T) {
	cfg := &config.Config{Tasks: config.TasksConfig{Dir: ""}}
	if err := writeManifest(cfg, "abc", taskManifest{TaskID: "abc"}); err != nil {
		t.Fatalf("expected nil err when disabled, got %v", err)
	}
}

// kyociEntry is a local mirror of kyoci.ToolCallEntry used to keep the test
// fixtures compact; toKyociEntries converts to the real type.
type kyociEntry struct {
	Tool string
	Args string
}

func toKyociEntries(in []kyociEntry) []kyoci.ToolCallEntry {
	out := make([]kyoci.ToolCallEntry, len(in))
	for i, e := range in {
		out[i] = kyoci.ToolCallEntry{Tool: e.Tool, Args: e.Args}
	}
	return out
}
