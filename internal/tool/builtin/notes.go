package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// NotesTool is the agent's working scratchpad. Unlike `remember` (which
// persists user facts to L3 memory), notes are the agent's own working memory
// for the current task — useful for tracking multi-step progress without
// polluting the user-facing memory store. Persists to data/notes.md.
type NotesTool struct{}

func NewNotesTool() *NotesTool { return &NotesTool{} }

func (n *NotesTool) Name() string { return "notes" }

func (n *NotesTool) Description() string {
	return "Agent scratchpad. `notes operation=append content=\"step 1 done\"` adds a line. " +
		"`notes operation=read` returns the current notes. Persists to data/notes.md."
}

func (n *NotesTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{Name: "operation", Type: "string", Required: true,
			EnumValues:  []string{"read", "append", "write", "clear"},
			Description: "Notes operation: read (return current), append (add line), write (replace), clear (empty)"},
		{Name: "content", Type: "string", Required: false,
			Description: "Text to append/write (required for append and write)"},
	}
}

const notesDefaultPath = "data/notes.md"

func (n *NotesTool) Execute(_ context.Context, params map[string]interface{}) (string, error) {
	op, _ := params["operation"].(string)
	content, _ := params["content"].(string)

	path := notesDefaultPath
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("create notes dir: %w", err)
	}

	switch op {
	case "read":
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return "(no notes yet)", nil
			}
			return "", fmt.Errorf("read notes: %w", err)
		}
		s := strings.TrimRight(string(data), "\n")
		if s == "" {
			return "(notes file is empty)", nil
		}
		return s, nil

	case "append":
		if content == "" {
			return "", fmt.Errorf("content required for append")
		}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return "", fmt.Errorf("open notes: %w", err)
		}
		defer f.Close()
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		if _, err := f.WriteString(content); err != nil {
			return "", fmt.Errorf("append notes: %w", err)
		}
		return fmt.Sprintf("appended %d char(s) to %s", len(content), path), nil

	case "write":
		if content == "" {
			return "", fmt.Errorf("content required for write")
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("write notes: %w", err)
		}
		return fmt.Sprintf("wrote %d char(s) to %s", len(content), path), nil

	case "clear":
		if err := os.WriteFile(path, []byte(""), 0644); err != nil {
			return "", fmt.Errorf("clear notes: %w", err)
		}
		return fmt.Sprintf("cleared %s", path), nil
	}
	return "", fmt.Errorf("unknown operation: %s", op)
}

var _ kyoci.Tool = (*NotesTool)(nil)
