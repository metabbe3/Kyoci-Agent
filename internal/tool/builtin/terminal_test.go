//go:build !windows

package builtin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/metabbe3/Kyoci-Agent/internal/taskctx"
)

// The terminal tool defaults its working directory to the per-task workspace
// from ctx (the same root the file tool writes into), so a relative file write
// and a shell command land in the same place.
func TestTerminalTool_WorkspaceIsDefaultWorkdir(t *testing.T) {
	ws, err := filepath.Abs(filepath.Join(t.TempDir(), "deliverable"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := taskctx.WithWorkspace(context.Background(), ws)

	out, err := NewTerminalTool().Execute(ctx, map[string]interface{}{"command": "pwd"})
	if err != nil {
		t.Fatalf("pwd: %v", err)
	}
	if !strings.Contains(out, ws) {
		t.Errorf("expected pwd to run in workspace %s, got %q", ws, out)
	}
}

// An explicit workdir param must override the workspace default.
func TestTerminalTool_WorkdirParamOverridesWorkspace(t *testing.T) {
	root := t.TempDir()
	ws, _ := filepath.Abs(filepath.Join(root, "deliverable"))
	other, _ := filepath.Abs(filepath.Join(root, "other"))
	for _, d := range []string{ws, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ctx := taskctx.WithWorkspace(context.Background(), ws)

	out, err := NewTerminalTool().Execute(ctx, map[string]interface{}{
		"command": "pwd",
		"workdir": other,
	})
	if err != nil {
		t.Fatalf("pwd: %v", err)
	}
	if !strings.Contains(out, other) {
		t.Errorf("explicit workdir param must win; expected %s, got %q", other, out)
	}
}

// With no workspace in ctx (tests, ExecuteDirect), behavior is unchanged: the
// command runs in the process cwd (workdir stays empty → cmd.Dir unset).
func TestTerminalTool_NoWorkspaceFallsBackToCwd(t *testing.T) {
	out, err := NewTerminalTool().Execute(context.Background(), map[string]interface{}{"command": "pwd"})
	if err != nil {
		t.Fatalf("pwd: %v", err)
	}
	if out == "" {
		t.Fatal("expected pwd output, got empty")
	}
}

// A non-zero exit must be surfaced as a machine-readable marker so Go-side
// honesty gates can detect build/test failures without trusting the model.
func TestTerminalTool_NonZeroExitMarked(t *testing.T) {
	out, err := NewTerminalTool().Execute(context.Background(), map[string]interface{}{"command": "false"})
	if err != nil {
		t.Fatalf("terminal returns output on non-zero (not an error); got err=%v", err)
	}
	if !strings.Contains(out, "[exit_status: non-zero") {
		t.Errorf("expected [exit_status: non-zero ...] marker in output, got: %q", out)
	}
}
