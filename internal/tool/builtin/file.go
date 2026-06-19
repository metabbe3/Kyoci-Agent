package builtin

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/metabbe3/Kyoci-Agent/internal/taskctx"
	"github.com/metabbe3/Kyoci-Agent/pkg"
)

// FileTool implements the kyoci.Tool interface for file operations.
type FileTool struct {
	logger      *slog.Logger
	allowedDirs []string
}

// NewFileTool creates a new file tool instance.
func NewFileTool() *FileTool {
	// Default to allowing current directory and user home
	homeDir, _ := os.UserHomeDir()
	return &FileTool{
		logger:      slog.Default(),
		allowedDirs: []string{".", homeDir},
	}
}

// Name returns the tool name.
func (f *FileTool) Name() string {
	return "file"
}

// Description returns the tool description.
func (f *FileTool) Description() string {
	return "Read, write, append, list, search, or check files. operation=\"read\" path=\"main.go\"; operation=\"write\" path=\"hello.txt\" content=\"hi\"; operation=\"list\" path=\"./src\"; operation=\"search\" path=\".\" pattern=\"TODO\"."
}

// Parameters returns the tool parameter definition.
func (f *FileTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{
			Name:        "operation",
			Type:        "string",
			Description: "Operation to perform: read, write, edit, append, exists, list, search",
			Required:    true,
			EnumValues:  []string{"read", "write", "edit", "append", "exists", "list", "search"},
		},
		{
			Name:        "path",
			Type:        "string",
			Description: "Absolute file or directory path. Tilde (~) expands to your home directory. Examples: '/Users/$USER/Documents', '~/Documents'. Avoid relative paths like 'documents' or '/documents' — they often resolve to the wrong place.",
			Required:    true,
		},
		{
			Name:        "content",
			Type:        "string",
			Description: "Content to write or append (required for write/append operations)",
			Required:    false,
		},
		{
			Name:        "old_string",
			Type:        "string",
			Description: "Exact string to find in the file (required for edit operation). Must match exactly once for a safe replacement.",
			Required:    false,
		},
		{
			Name:        "new_string",
			Type:        "string",
			Description: "String to replace old_string with (required for edit operation). Set to empty string to delete old_string.",
			Required:    false,
		},
		{
			Name:        "offset",
			Type:        "integer",
			Description: "Read-only: byte offset to start reading from (default 0). Use for chunked reads of large files — see the trailer in the response for the next offset to use.",
			Required:    false,
		},
		{
			Name:        "limit",
			Type:        "integer",
			Description: "Read-only: max bytes to return (default 0 = no limit). When both offset and limit are 0 AND the file is large, auto-chunking returns the first 4096 bytes plus instructions to continue.",
			Required:    false,
		},
		{
			Name:        "pattern",
			Type:        "string",
			Description: "Pattern to search for (required for search operation)",
			Required:    false,
		},
	}
}

// Execute performs file operations.
//
// Parameters:
//   - ctx: Context for cancellation. Also carries the per-task workspace dir
//     (set by the orchestrator via taskctx.WithWorkspace) — when present,
//     relative paths resolve into the workspace and the workspace is added
//     to the per-call allowed dirs so writes there pass the sandbox check.
//   - params: Map containing "operation", "path", and optionally "content" or "pattern"
//
// Returns:
//   - string: Result of the operation
//   - error: Error if operation fails
func (f *FileTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// Extract operation
	operation, ok := params["operation"].(string)
	if !ok || operation == "" {
		return "", fmt.Errorf("operation parameter is required and must be a string")
	}

	// Extract path
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("path parameter is required and must be a string")
	}

	// Read the per-task workspace from ctx. When set, relative paths resolve
	// into it and it's added to the allowed-dirs check for this call only —
	// no mutation of the shared f.allowedDirs slice, so concurrent tasks on
	// other workspaces are unaffected.
	workspace := taskctx.WorkspaceFromCtx(ctx)
	sandbox := taskctx.SandboxFromCtx(ctx)

	// Expand ~ and resolve to an absolute path BEFORE the allowed-dirs check,
	// so the model's ~/Documents resolves to $HOME/Documents and passes the
	// allow-check against the home directory. Without this the tilde is left
	// literal and the call fails with an opaque "directory not found".
	absPath, err := f.expandPath(path, workspace)
	if err != nil {
		return "", err
	}

	// Sandbox ceiling: when set, reject any path that escapes the sandbox
	// root even if it's otherwise allowed. This is the 8B-wandering fix —
	// a task scoped to /projects/calculator/ cannot read /projects/auth/
	// even if both are in the static allowed-dirs list.
	if sandbox != "" {
		sandboxAbs, err := filepath.Abs(sandbox)
		if err != nil {
			return "", fmt.Errorf("invalid sandbox root %q: %w", sandbox, err)
		}
		rel, err := filepath.Rel(sandboxAbs, absPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			f.logger.Warn("path outside sandbox", "path", absPath, "sandbox", sandboxAbs)
			return "", fmt.Errorf("access denied: path %q outside sandbox %q", absPath, sandboxAbs)
		}
	}

	// Validate path
	if !f.isPathAllowed(absPath, workspace) {
		// RECOVERY: 8B models routinely emit "/projects/foo" when they mean
		// "projects/foo" relative to the cwd. The leading slash makes the path
		// look absolute-from-root, which fails the allowed-dirs check.
		//
		// Heuristic: only recover if (a) the stripped form is allowed AND
		// (b) the stripped form's PARENT DIRECTORY EXISTS. The parent-exists
		// check is the key safeguard — it stops us from rewriting deep system
		// paths like "/var/folders/abc/T/..." into "<cwd>/var/folders/abc/T/..."
		// and silently creating garbage. Real project subdirs like "projects/"
		// exist; "/var/folders/..." stripped to "var/folders/..." does not.
		if strings.HasPrefix(path, "/") {
			stripped := strings.TrimPrefix(path, "/")
			if strippedAbs, stripErr := f.expandPath(stripped, workspace); stripErr == nil &&
				f.isPathAllowed(strippedAbs, workspace) {
				parent := filepath.Dir(strippedAbs)
				if pinfo, perr := os.Stat(parent); perr == nil && pinfo.IsDir() {
					f.logger.Info("file tool: rewrote absolute-style path to cwd-relative",
						"original", path, "rewritten", stripped)
					absPath = strippedAbs
				}
			}
		}
		if !f.isPathAllowed(absPath, workspace) {
			f.logger.Warn("path access denied", "path", absPath,
				"hint", "if you meant a project-relative path, drop the leading slash")
			return "", fmt.Errorf("access denied: path %q outside allowed directories (try %q without the leading slash)",
				path, strings.TrimPrefix(path, "/"))
		}
	}

	// Execute operation based on type
	switch operation {
	case "read":
		// Optional offset/limit for chunked reads — protects the agent's
		// context budget when reading large files. Both default to 0;
		// when 0+0 AND file > 8KB, auto-chunk kicks in (returns first 4KB
		// + a "call again with offset=N" trailer).
		offset := paramInt(params, "offset")
		limit := paramInt(params, "limit")
		return f.readFile(absPath, offset, limit)
	case "write":
		return f.writeFile(absPath, params)
	case "edit":
		return f.editFile(absPath, params)
	case "append":
		return f.appendFile(absPath, params)
	case "exists":
		return f.checkExists(absPath)
	case "list":
		return f.listDirectory(absPath)
	case "search":
		return f.searchFiles(absPath, params)
	default:
		return "", fmt.Errorf("unknown operation: %s", operation)
	}
}

// expandPath normalizes a path argument from the LLM. It expands a leading ~
// to the user's home directory, then resolves relative paths. When workspace
// is non-empty, relative paths resolve against the workspace dir (so a worker
// writing "src/main.go" lands in <workspace>/src/main.go, not the process
// CWD); absolute paths and ~ are unaffected. Errors surface a clear message
// rather than silently producing a path that won't exist.
func (f *FileTool) expandPath(path, workspace string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	// Expand ~ to home dir. Models commonly try ~/Documents when they don't
	// know the absolute home path; without this the call fails with an opaque
	// "directory not found" error.
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot resolve home directory: %w", err)
		}
		if path == "~" {
			path = home
		} else if strings.HasPrefix(path, "~/") {
			path = filepath.Join(home, path[2:])
		}
	}
	// Relative paths: prefer the per-task workspace when set, else the
	// process CWD. filepath.IsAbs is true for both Unix-absolute and ~-expanded
	// (which became absolute in the branch above), so we don't double-resolve.
	if !filepath.IsAbs(path) && workspace != "" {
		path = filepath.Join(workspace, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("cannot resolve path %q: %w", path, err)
	}
	return abs, nil
}

// readFile reads the content of a file.
func (f *FileTool) readFile(path string, offset, limit int) (string, error) {
	f.logger.Info("reading file", "path", path, "offset", offset, "limit", limit)

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			home, _ := os.UserHomeDir()
			return "", fmt.Errorf(
				"file not found: %s. Hint: your home directory is %s. "+
					"Try file read %s/<filename> or file list ~ to see your home contents.",
				path, home, home,
			)
		}
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied: %s", path)
		}
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	totalLen := len(content)

	// Default chunk size — keeps each read under ~1000 tokens so the agent's
	// 8K-token context budget isn't blown by a single large file.
	const defaultChunk = 4096

	// AUTO-CHUNK: if caller passed offset=0 + limit=0 AND file is large,
	// return the first defaultChunk bytes + a "call again" trailer. This
	// protects the agent from accidentally loading a 50KB file in one shot
	// and exhausting its context window.
	if offset == 0 && limit == 0 && totalLen > 2*defaultChunk {
		end := defaultChunk
		if end > totalLen {
			end = totalLen
		}
		chunk := content[:end]
		remaining := totalLen - end
		return fmt.Sprintf("=== %s (chunk 1, %d/%d bytes) ===\n%s\n=== end chunk — call file:read again with offset=%d to fetch the next %d bytes (%d bytes remaining) ===",
			path, end, totalLen, string(chunk), end, defaultChunk, remaining), nil
	}

	// Explicit offset/limit: return the requested range + trailer if more remains.
	if offset > 0 || limit > 0 {
		if offset >= totalLen {
			return "", fmt.Errorf("offset %d >= file size %d — nothing to read at that offset", offset, totalLen)
		}
		end := totalLen
		if limit > 0 {
			end = offset + limit
			if end > totalLen {
				end = totalLen
			}
		}
		chunk := content[offset:end]
		remaining := totalLen - end
		header := fmt.Sprintf("=== %s (offset=%d, %d/%d bytes) ===\n", path, offset, end-offset, totalLen)
		trailer := ""
		if remaining > 0 {
			nextChunk := defaultChunk
			if limit > 0 && limit < defaultChunk {
				nextChunk = limit
			}
			trailer = fmt.Sprintf("\n=== end chunk — call file:read again with offset=%d to fetch the next %d bytes (%d bytes remaining) ===",
				end, nextChunk, remaining)
		}
		return header + string(chunk) + trailer, nil
	}

	// Small file, no offset/limit — return full content unchanged.
	return string(content), nil
}

// writeFile writes content to a file, overwriting if it exists.
func (f *FileTool) writeFile(path string, params map[string]interface{}) (string, error) {
	content, ok := params["content"].(string)
	if !ok {
		return "", fmt.Errorf("content parameter is required for write operation")
	}

	f.logger.Info("writing file", "path", path)

	// Create parent directories if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directories: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied: %s", path)
		}
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}

// editFile replaces the first occurrence of old_string with new_string in a file.
// It errors if old_string is empty or not found, mirroring the semantics of a
// safe find-and-replace: the model must name exactly what it wants to change.
func (f *FileTool) editFile(path string, params map[string]interface{}) (string, error) {
	oldStr, ok := params["old_string"].(string)
	if !ok || oldStr == "" {
		return "", fmt.Errorf("old_string parameter is required for edit operation")
	}
	newStr, _ := params["new_string"].(string)

	f.logger.Info("editing file", "path", path, "old_len", len(oldStr), "new_len", len(newStr))

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", path)
		}
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied: %s", path)
		}
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	if !strings.Contains(string(content), oldStr) {
		return "", fmt.Errorf("old_string not found in %s", path)
	}

	newContent := strings.Replace(string(content), oldStr, newStr, 1)
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied: %s", path)
		}
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully edited %s (%d → %d bytes)", path, len(content), len(newContent)), nil
}

// appendFile appends content to a file.
func (f *FileTool) appendFile(path string, params map[string]interface{}) (string, error) {
	content, ok := params["content"].(string)
	if !ok {
		return "", fmt.Errorf("content parameter is required for append operation")
	}

	f.logger.Info("appending to file", "path", path)

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied: %s", path)
		}
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(content); err != nil {
		return "", fmt.Errorf("failed to append to file: %w", err)
	}

	return fmt.Sprintf("Successfully appended %d bytes to %s", len(content), path), nil
}

// checkExists checks if a file or directory exists.
func (f *FileTool) checkExists(path string) (string, error) {
	f.logger.Info("checking if path exists", "path", path)

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Path does not exist: %s", path), nil
		}
		return "", fmt.Errorf("failed to check path: %w", err)
	}

	fileType := "file"
	if info.IsDir() {
		fileType = "directory"
	}

	return fmt.Sprintf("Path exists: %s (type: %s, size: %d bytes)", path, fileType, info.Size()), nil
}

// listDirectory lists the contents of a directory.
func (f *FileTool) listDirectory(path string) (string, error) {
	f.logger.Info("listing directory", "path", path)

	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			home, _ := os.UserHomeDir()
			return "", fmt.Errorf(
				"directory not found: %s. Hint: your home directory is %s. "+
					"Try file list %s/Documents or file list ~ to see your home contents.",
				path, home, home,
			)
		}
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied: %s", path)
		}
		return "", fmt.Errorf("failed to list directory: %w", err)
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Contents of %s (%d entries):\n", path, len(entries)))

	for _, entry := range entries {
		info, _ := entry.Info()
		fileType := "FILE"
		if entry.IsDir() {
			fileType = "DIR "
		}
		builder.WriteString(fmt.Sprintf("  %s %s (%d bytes)\n", fileType, entry.Name(), info.Size()))
	}

	return builder.String(), nil
}

// searchFiles searches for files matching a pattern.
func (f *FileTool) searchFiles(path string, params map[string]interface{}) (string, error) {
	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		return "", fmt.Errorf("pattern parameter is required for search operation")
	}

	f.logger.Info("searching files", "path", path, "pattern", pattern)

	var results []string

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip files we can't access
			return nil
		}

		// Search in file name
		if strings.Contains(strings.ToLower(filepath.Base(filePath)), strings.ToLower(pattern)) {
			fileType := "FILE"
			if info.IsDir() {
				fileType = "DIR "
			}
			results = append(results, fmt.Sprintf("%s %s", fileType, filePath))
			return nil
		}

		// Search in file content (only for text files)
		if !info.IsDir() && info.Size() < 1024*1024 { // Limit to 1MB files
			if content, err := os.ReadFile(filePath); err == nil {
				if strings.Contains(strings.ToLower(string(content)), strings.ToLower(pattern)) {
					results = append(results, fmt.Sprintf("MATCH %s", filePath))
				}
			}
		}

		return nil
	})

	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		return fmt.Sprintf("No results found for pattern '%s' in %s", pattern, path), nil
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Found %d matches for pattern '%s':\n", len(results), pattern))
	for _, result := range results {
		builder.WriteString(fmt.Sprintf("  - %s\n", result))
	}

	return builder.String(), nil
}

// isPathAllowed checks if a path is within allowed directories. The per-call
// workspace (when non-empty) is added to the allowed set for this check only.
func (f *FileTool) isPathAllowed(path, workspace string) bool {
	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// Build the effective allowed-dirs list: static config dirs plus the
	// per-call workspace if set. Allocating a small slice per call is cheaper
	// than tracking workspace membership with maps and avoids mutating the
	// shared f.allowedDirs slice (which would race across concurrent tasks).
	allowed := f.allowedDirs
	if workspace != "" {
		allowed = append(allowed, workspace)
	}

	for _, allowedDir := range allowed {
		// Get absolute path of allowed directory
		allowedAbs, err := filepath.Abs(allowedDir)
		if err != nil {
			continue
		}

		// Check if path starts with allowed directory
		rel, err := filepath.Rel(allowedAbs, absPath)
		if err != nil {
			continue
		}

		// Check if the relative path doesn't go outside allowed directory
		if !strings.HasPrefix(rel, "..") {
			return true
		}
	}

	return false
}

// SetAllowedDirs sets the list of allowed directories.
func (f *FileTool) SetAllowedDirs(dirs []string) {
	f.allowedDirs = dirs
}

// paramInt extracts an integer parameter from the tool-call params map,
// tolerating both int and float64 encodings (JSON numbers arrive as
// float64 via the LLM tool-call JSON path). Returns 0 when the key is
// missing or the value is the wrong type.
func paramInt(params map[string]interface{}, key string) int {
	if v, ok := params[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return 0
}
