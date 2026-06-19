package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/config"
)

// =====================================================================================
// Per-task workspace management
//
// Each orchestrated agent run that may produce user-facing files gets a private
// workspace dir at <cfg.Tasks.Dir>/<task_id>/. Files land in a "deliverable/"
// subdir so the per-task manifest.json can sit alongside without colliding
// with the agent's output tree.
//
// The folder is created eagerly at dispatch time. If the task ends up writing
// nothing (research/Q&A tasks), CleanupIfEmpty removes it so the tasks/ tree
// only contains folders with actual deliverables.
// =====================================================================================

// PrepareWorkspace creates the per-task workspace directory tree and returns
// the deliverable path the agent should write into.
//
// Returns ("", nil) when cfg.Tasks.Dir is empty — callers should treat this as
// "workspace disabled" and skip both manifest writing and cleanup. This keeps
// the orchestrator path identical to legacy behavior when the operator opts out.
//
// The deliverable subdir is created eagerly (MkdirAll is idempotent) so the
// agent's first file write doesn't race against concurrent setup.
func PrepareWorkspace(cfg *config.Config, taskID string) (string, error) {
	if cfg == nil || cfg.Tasks.Dir == "" || taskID == "" {
		return "", nil
	}
	dir := filepath.Join(cfg.Tasks.Dir, taskID, "deliverable")
	// Absolutize so the `file` tool's join and the `terminal` tool's default
	// workdir converge on one root regardless of the process cwd (cfg.Tasks.Dir
	// defaults to the relative "tasks").
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("prepare workspace %s: %w", dir, err)
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return "", fmt.Errorf("prepare workspace %s: %w", absDir, err)
	}
	return absDir, nil
}

// TaskDir returns the per-task root folder (<cfg.Tasks.Dir>/<task_id>) without
// creating it. Returns "" when workspaces are disabled. Used by the
// manifest writer and the cleanup helper so they share a single path format.
func TaskDir(cfg *config.Config, taskID string) string {
	if cfg == nil || cfg.Tasks.Dir == "" || taskID == "" {
		return ""
	}
	dir := filepath.Join(cfg.Tasks.Dir, taskID)
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// CleanupIfEmpty removes the per-task folder when it contains no deliverable
// files. Used after a task that turned out to be research-only — leaves no
// empty folder behind in tasks/.
//
// A folder counts as "empty" if walking it finds zero regular files. Empty
// subdirs are still removed (the deliverable/ dir created by PrepareWorkspace,
// for example). Symlinks count as content. Removal errors are returned but
// the orchestrator treats them as best-effort.
func CleanupIfEmpty(cfg *config.Config, taskID string) error {
	root := TaskDir(cfg, taskID)
	if root == "" {
		return nil
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	}
	if !workspaceIsEmpty(root) {
		return nil
	}
	return os.RemoveAll(root)
}

// workspaceIsEmpty reports whether root contains zero regular files. Used to
// decide whether to remove a task folder. Symlinks count as content so a
// task that only created a symlink still preserves the folder.
func workspaceIsEmpty(root string) bool {
	empty := true
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // ignore unreadable entries; they don't count as content
		}
		if info.IsDir() {
			return nil
		}
		empty = false
		return filepath.SkipDir // any file is enough; stop walking
	})
	return empty
}

// taskManifest is the on-disk record of one orchestrated task run. Written at
// the end of executeWithRetry when at least one file was produced; absent for
// research/Q&A tasks (the whole folder is removed by CleanupIfEmpty).
type taskManifest struct {
	TaskID       string    `json:"task_id"`
	Role         string    `json:"role"`
	Task         string    `json:"task"`
	StartedAt    time.Time `json:"started_at"`
	CompletedAt  time.Time `json:"completed_at"`
	Status       string    `json:"status"`             // "completed" | "failed"
	Summary      string    `json:"summary,omitempty"`  // short result excerpt
	FilesCreated []string  `json:"files_created"`      // absolute paths the agent wrote
	LogPath      string    `json:"log_path,omitempty"` // relative path to per-run log
}
