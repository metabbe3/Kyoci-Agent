package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileHandlerTool reads and writes files
type FileHandlerTool struct {
	workDir string
}

func NewFileHandlerTool(workDir string) *FileHandlerTool {
	if workDir == "" {
		workDir, _ = os.Getwd()
	}
	return &FileHandlerTool{workDir: workDir}
}

func (t *FileHandlerTool) Name() string { return "file_handler" }

func (t *FileHandlerTool) Description() string {
	return "Read from and write to files on the filesystem. Operations: read, write, list, append."
}

func (t *FileHandlerTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"operation": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"read", "write", "list", "append"},
				"description": "The file operation to perform",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File or directory path (relative to workdir)",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content to write/append (for write/append operations)",
			},
		},
		"required": []string{"operation", "path"},
	}
}

func (t *FileHandlerTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var params struct {
		Operation string `json:"operation"`
		Path      string `json:"path"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	// Sanitize path — prevent directory traversal
	cleanPath := filepath.Clean(filepath.Join(t.workDir, params.Path))
	if !strings.HasPrefix(cleanPath, t.workDir) {
		return "", fmt.Errorf("access denied: path outside workdir")
	}

	switch params.Operation {
	case "read":
		data, err := os.ReadFile(cleanPath)
		if err != nil {
			return "", fmt.Errorf("read failed: %w", err)
		}
		return string(data), nil

	case "write":
		dir := filepath.Dir(cleanPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("mkdir failed: %w", err)
		}
		if err := os.WriteFile(cleanPath, []byte(params.Content), 0644); err != nil {
			return "", fmt.Errorf("write failed: %w", err)
		}
		return fmt.Sprintf("Written %d bytes to %s", len(params.Content), params.Path), nil

	case "append":
		dir := filepath.Dir(cleanPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("mkdir failed: %w", err)
		}
		f, err := os.OpenFile(cleanPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return "", fmt.Errorf("open failed: %w", err)
		}
		defer f.Close()
		n, err := f.WriteString(params.Content)
		if err != nil {
			return "", fmt.Errorf("append failed: %w", err)
		}
		return fmt.Sprintf("Appended %d bytes to %s", n, params.Path), nil

	case "list":
		entries, err := os.ReadDir(cleanPath)
		if err != nil {
			return "", fmt.Errorf("list failed: %w", err)
		}
		var lines []string
		for _, e := range entries {
			prefix := "  "
			if e.IsDir() {
				prefix = "📁"
			} else {
				prefix = "📄"
			}
			lines = append(lines, fmt.Sprintf("%s %s", prefix, e.Name()))
		}
		return strings.Join(lines, "\n"), nil

	default:
		return "", fmt.Errorf("unknown operation: %s", params.Operation)
	}
}
