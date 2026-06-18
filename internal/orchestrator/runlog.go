package orchestrator

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/config"
)

// =====================================================================================
// Per-run execution logger
//
// Each orchestrated agent run gets a JSON-lines trace at
// <cfg.Logging.PerRunDir>/<YYYY-MM-DD>/run_<task_id>.log. The handler is an
// io.MultiWriter over os.Stdout AND the per-run file, so 12-factor stdout
// behavior is preserved (every line still hits stdout) while the file captures
// a per-task slice for post-hoc debugging.
//
// Failure to open the file degrades gracefully — the caller falls back to
// stdout-only logging. A task must never fail because of logging setup.
//
// The level is taken from cfg.Logging.Level so per-run traces respect the
// same threshold as stdout.
// =====================================================================================

// RunLogPath returns the absolute path to the per-run log file for the given
// task_id on the given date, or "" if per-run logging is disabled.
//
// Exposed so the manifest writer can stamp the same path the logger actually
// wrote to — no second source of truth for the date format.
func RunLogPath(cfg *config.Config, taskID string, now time.Time) string {
	if cfg == nil || !cfg.Logging.PerRunEnabled || cfg.Logging.PerRunDir == "" || taskID == "" {
		return ""
	}
	return filepath.Join(cfg.Logging.PerRunDir, now.Format("2006-01-02"), "run_"+taskID+".log")
}

// OpenRunLogger sets up the per-run logger. Returns:
//   - logger: a *slog.Logger writing JSON to stdout AND the per-run file
//   - closer: a func that closes the file; safe to defer; no-op on fallback
//   - logPath: the relative path the file was opened at (for the manifest)
//   - err: only when cfg is malformed in a way the caller wants to surface
//
// On any I/O error opening the file, the function logs a warning and returns
// a stdout-only logger with a no-op closer — task execution continues.
// Time is injected (rather than time.Now()) so callers can pin it in tests
// and so all parts of a single run share one timestamp.
func OpenRunLogger(cfg *config.Config, taskID string, now time.Time) (*slog.Logger, func(), string) {
	if cfg == nil || !cfg.Logging.PerRunEnabled || cfg.Logging.PerRunDir == "" || taskID == "" {
		// Per-run logging disabled — return the default stdout logger and a
		// no-op closer. The caller still defer-closes safely.
		return slog.Default(), func() {}, ""
	}

	logPath := RunLogPath(cfg, taskID, now)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		slog.Warn("orchestrator: per-run log dir unavailable; falling back to stdout",
			"path", filepath.Dir(logPath), "err", err)
		return slog.Default(), func() {}, ""
	}

	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		slog.Warn("orchestrator: per-run log file open failed; falling back to stdout",
			"path", logPath, "err", err)
		return slog.Default(), func() {}, ""
	}

	closer := func() {
		if cerr := file.Close(); cerr != nil {
			slog.Warn("orchestrator: per-run log close failed", "path", logPath, "err", cerr)
		}
	}

	opts := &slog.HandlerOptions{Level: parseSlogLevel(cfg.Logging.Level)}
	handler := slog.NewJSONHandler(io.MultiWriter(os.Stdout, file), opts)
	return slog.New(handler), closer, logPath
}

// parseSlogLevel maps the config string to a slog.Level. Unknown values fall
// back to info — same as the server's setupLogger in cmd/server/main.go.
func parseSlogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// runLoggerKey is the context key under which the active per-run logger is
// stashed. Workers read it via RunLoggerFromCtx so their per-step logging
// lands in the right file without explicit plumbing through every signature.
type runLoggerKey struct{}

// WithRunLogger returns a ctx carrying the per-run logger. Empty logger clears
// the slot (worker falls back to the agent's static logger).
func WithRunLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, runLoggerKey{}, l)
}

// RunLoggerFromCtx returns the per-run logger, or nil if none is set.
func RunLoggerFromCtx(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Value(runLoggerKey{}).(*slog.Logger); ok {
		return v
	}
	return nil
}

// formatLogPath relativizes an absolute log path against the CWD so manifests
// show "logs/2026-06-18/run_xxx.log" rather than /Users/.../logs/... TrimRoot
// falls back to the input when the relativization fails (different volume).
func formatLogPath(abs string) string {
	if abs == "" {
		return ""
	}
	if rel, err := filepath.Rel(".", abs); err == nil && !startsWithDotDot(rel) {
		return rel
	}
	return abs
}

// startsWithDotDot reports whether p begins with ".." — filepath.Rel produces
// such paths when the target is outside CWD, and we don't want to advertise
// those as "relative" in the manifest.
func startsWithDotDot(p string) bool {
	return p == ".." || len(p) >= 3 && p[0] == '.' && p[1] == '.' && (p[2] == '/' || p[2] == filepath.Separator)
}
