package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/metabbe3/Kyoci-Agent/internal/config"
)

// TestPrepareWorkspace_CreatesDeliverableSubdir verifies the workspace layout
// tasks/<id>/deliverable/ — the manifest sits at tasks/<id>/manifest.json so
// it can't collide with deliverable content.
func TestPrepareWorkspace_CreatesDeliverableSubdir(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Tasks: config.TasksConfig{Dir: filepath.Join(root, "tasks")}}

	got, err := PrepareWorkspace(cfg, "abc-123")
	if err != nil {
		t.Fatalf("PrepareWorkspace: %v", err)
	}
	want := filepath.Join(root, "tasks", "abc-123", "deliverable")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("deliverable dir not created at %s (err=%v)", want, err)
	}
}

// TestPrepareWorkspace_DisabledByEmptyDir confirms the no-op behavior the
// orchestrator relies on when workspaces are turned off — empty cfg.Tasks.Dir
// returns ("", nil) and creates nothing.
func TestPrepareWorkspace_DisabledByEmptyDir(t *testing.T) {
	cfg := &config.Config{Tasks: config.TasksConfig{Dir: ""}}
	got, err := PrepareWorkspace(cfg, "abc-123")
	if err != nil {
		t.Fatalf("expected nil err when disabled, got %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty path when disabled, got %q", got)
	}
}

// TestPrepareWorkspace_IdempotentOnRepeatedCalls confirms MkdirAll semantics
// — calling twice with the same task_id doesn't fail.
func TestPrepareWorkspace_IdempotentOnRepeatedCalls(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Tasks: config.TasksConfig{Dir: filepath.Join(root, "tasks")}}
	if _, err := PrepareWorkspace(cfg, "x"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := PrepareWorkspace(cfg, "x"); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

// TestCleanupIfEmpty_RemovesEmptyTaskFolder is the manifest-skipping path:
// research/Q&A tasks leave the deliverable folder empty, and the orchestrator
// drops it so tasks/ only contains folders with actual content.
func TestCleanupIfEmpty_RemovesEmptyTaskFolder(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Tasks: config.TasksConfig{Dir: filepath.Join(root, "tasks")}}
	taskDir, err := PrepareWorkspace(cfg, "abc")
	if err != nil {
		t.Fatalf("PrepareWorkspace: %v", err)
	}
	_ = taskDir

	if err := CleanupIfEmpty(cfg, "abc"); err != nil {
		t.Fatalf("CleanupIfEmpty: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Tasks.Dir, "abc")); !os.IsNotExist(err) {
		t.Fatalf("expected task dir removed, got err=%v", err)
	}
}

// TestCleanupIfEmpty_KeepsFolderWhenFilesExist verifies the contract: once
// the agent has written a file into the workspace, cleanup is a no-op.
func TestCleanupIfEmpty_KeepsFolderWhenFilesExist(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Tasks: config.TasksConfig{Dir: filepath.Join(root, "tasks")}}
	taskDir, err := PrepareWorkspace(cfg, "abc")
	if err != nil {
		t.Fatalf("PrepareWorkspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "out.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if err := CleanupIfEmpty(cfg, "abc"); err != nil {
		t.Fatalf("CleanupIfEmpty: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Tasks.Dir, "abc")); err != nil {
		t.Fatalf("expected task dir kept, got err=%v", err)
	}
}

// TestCleanupIfEmpty_NoopWhenWorkspacesDisabled verifies graceful no-op when
// the operator opts out of per-task workspaces.
func TestCleanupIfEmpty_NoopWhenWorkspacesDisabled(t *testing.T) {
	cfg := &config.Config{Tasks: config.TasksConfig{Dir: ""}}
	if err := CleanupIfEmpty(cfg, "abc"); err != nil {
		t.Fatalf("expected nil err when disabled, got %v", err)
	}
}

// TestCleanupIfEmpty_NoopWhenDirMissing confirms statting a never-created
// folder doesn't error — relevant for the path where PrepareWorkspace failed
// mid-run but the orchestrator still calls CleanupIfEmpty defensively.
func TestCleanupIfEmpty_NoopWhenDirMissing(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Tasks: config.TasksConfig{Dir: filepath.Join(root, "tasks")}}
	if err := CleanupIfEmpty(cfg, "never-created"); err != nil {
		t.Fatalf("expected nil err for missing dir, got %v", err)
	}
}

// TestTaskDir_ReturnsEmptyWhenDisabled verifies the path helper the manifest
// writer uses to decide whether to skip itself.
func TestTaskDir_ReturnsEmptyWhenDisabled(t *testing.T) {
	cfg := &config.Config{Tasks: config.TasksConfig{Dir: ""}}
	if got := TaskDir(cfg, "abc"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	cfg.Tasks.Dir = "tasks"
	if got := TaskDir(cfg, ""); got != "" {
		t.Fatalf("expected empty for empty taskID, got %q", got)
	}
}
