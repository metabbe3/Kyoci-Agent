package builtin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/metabbe3/Kyoci-Agent/internal/taskctx"
)

// TestFileTool_NoWorkspace_LegacyBehavior confirms that without a workspace
// in ctx, the file tool behaves exactly as before — relative paths resolve
// against the process CWD, and the legacy allowedDirs (".", home) apply.
func TestFileTool_NoWorkspace_LegacyBehavior(t *testing.T) {
	tmp := t.TempDir()
	tool := NewFileTool()
	// Confine writes to tmp via SetAllowedDirs so the test doesn't scribble
	// in the repo. This is the same hook the role layer uses for per-role
	// sandboxing.
	tool.SetAllowedDirs([]string{tmp})

	abs := filepath.Join(tmp, "hello.txt")
	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "write",
		"path":      abs,
		"content":   "hi",
	})
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if !strings.Contains(out, "Successfully wrote") {
		t.Fatalf("unexpected output: %s", out)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

// TestFileTool_WorkspaceInCtx_RelativePathResolvesIntoWorkspace verifies the
// core per-task isolation contract: when taskctx.WithWorkspace sets a
// workspace, a relative write path lands inside it without the caller having
// to construct the absolute path.
func TestFileTool_WorkspaceInCtx_RelativePathResolvesIntoWorkspace(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "task-123", "deliverable")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	tool := NewFileTool()
	ctx := taskctx.WithWorkspace(context.Background(), workspace)

	relPath := "src/main.go"
	if _, err := tool.Execute(ctx, map[string]interface{}{
		"operation": "write",
		"path":      relPath,
		"content":   "package main",
	}); err != nil {
		t.Fatalf("write relative path with workspace failed: %v", err)
	}

	absPath := filepath.Join(workspace, relPath)
	if _, err := os.Stat(absPath); err != nil {
		t.Fatalf("expected file at %s: %v", absPath, err)
	}
}

// TestFileTool_WorkspaceInCtx_AbsolutePathOutsideWorkspacePasses confirms
// the "Add taskDir, keep repo access" containment policy: even with a
// workspace set, an absolute path the tool was already allowed to touch
// (like a tmp dir) continues to work.
func TestFileTool_WorkspaceInCtx_AbsolutePathOutsideWorkspacePasses(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "task-123", "deliverable")
	otherAllowed := filepath.Join(tmp, "elsewhere")
	for _, d := range []string{workspace, otherAllowed} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	tool := NewFileTool()
	// Legacy allowedDirs include "elsewhere" (e.g. user home in production).
	tool.SetAllowedDirs([]string{otherAllowed})

	ctx := taskctx.WithWorkspace(context.Background(), workspace)

	// Absolute write into the other-allowed dir should still pass.
	absElsewhere := filepath.Join(otherAllowed, "outside.txt")
	if _, err := tool.Execute(ctx, map[string]interface{}{
		"operation": "write",
		"path":      absElsewhere,
		"content":   "ok",
	}); err != nil {
		t.Fatalf("write to other allowed dir failed: %v", err)
	}
	if _, err := os.Stat(absElsewhere); err != nil {
		t.Fatalf("expected file at %s: %v", absElsewhere, err)
	}

	// Absolute write into the workspace should also pass.
	absInWorkspace := filepath.Join(workspace, "inside.txt")
	if _, err := tool.Execute(ctx, map[string]interface{}{
		"operation": "write",
		"path":      absInWorkspace,
		"content":   "ok",
	}); err != nil {
		t.Fatalf("write into workspace failed: %v", err)
	}
}

// TestFileTool_WorkspaceInCtx_PathOutsideEverythingDenied verifies the
// sandbox still rejects paths that are neither in the workspace nor in the
// legacy allowedDirs.
func TestFileTool_WorkspaceInCtx_PathOutsideEverythingDenied(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tool := NewFileTool()
	tool.SetAllowedDirs([]string{workspace})

	forbidden := filepath.Join(tmp, "not-allowed", "secret.txt")
	if err := os.MkdirAll(filepath.Dir(forbidden), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ctx := taskctx.WithWorkspace(context.Background(), workspace)
	_, err := tool.Execute(ctx, map[string]interface{}{
		"operation": "write",
		"path":      forbidden,
		"content":   "nope",
	})
	if err == nil {
		t.Fatalf("expected access-denied error for path outside allowed + workspace")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("expected 'access denied' in error, got: %v", err)
	}
}

// TestFileTool_WorkspaceInCtx_ReadStillWorks confirms the workspace change
// doesn't break read operations on absolute paths the tool was already
// allowed to read.
func TestFileTool_WorkspaceInCtx_ReadStillWorks(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tool := NewFileTool()
	tool.SetAllowedDirs([]string{tmp})
	ctx := taskctx.WithWorkspace(context.Background(), workspace)

	abs := filepath.Join(tmp, "readme.md")
	if err := os.WriteFile(abs, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	out, err := tool.Execute(ctx, map[string]interface{}{
		"operation": "read",
		"path":      abs,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !strings.Contains(out, "hello world") {
		t.Fatalf("unexpected read output: %s", out)
	}
}
