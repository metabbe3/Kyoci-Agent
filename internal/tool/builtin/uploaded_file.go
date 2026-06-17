package builtin

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/metabbe3/Kyoci-Agent/pkg"
)

// UploadDir is the same path used by internal/dashboard/upload.go. Duplicated
// here to avoid an import cycle (dashboard → orchestrator → tool → builtin
// would close a loop). If the dashboard path changes, update both.
const UploadDir = "data/uploads"

// UploadedFileTool lets the agent read files the user attached via the
// dashboard's upload endpoint. The dashboard's ChatRequest prepends a list
// of uploaded filenames to the task; the agent resolves them through this
// tool. All access is sandboxed to UploadDir — the agent cannot read
// arbitrary filesystem paths through this tool.
type UploadedFileTool struct {
	logger *slog.Logger
}

func NewUploadedFileTool() *UploadedFileTool {
	return &UploadedFileTool{logger: slog.Default()}
}

func (t *UploadedFileTool) Name() string { return "uploaded_file" }

func (t *UploadedFileTool) Description() string {
	return "Read files the user attached to the chat. Use the EXACT filename from the [Attached files] list in the task. Operations: read (text content), info (size/mtime/mime), list (all uploads). For .xlsx/.xls use the `excel` tool instead. For images (PNG/JPG) the read operation returns a base64 data URI."
}

func (t *UploadedFileTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{
			Name:        "operation",
			Type:        "string",
			Description: "Operation: read (full text or base64), info (metadata), list (all uploads)",
			Required:    true,
			EnumValues:  []string{"read", "info", "list"},
		},
		{
			Name:        "filename",
			Type:        "string",
			Description: "Exact filename from the [Attached files] list (e.g. 'a1b2c3d4e5f6a7b8-report.xlsx'). Ignored for list operation.",
			Required:    false,
		},
	}
}

func (t *UploadedFileTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	op, _ := params["operation"].(string)
	switch op {
	case "list":
		return t.listUploads()
	case "info":
		fn, _ := params["filename"].(string)
		return t.info(fn)
	case "read":
		fn, _ := params["filename"].(string)
		return t.read(fn)
	default:
		return "", fmt.Errorf("unknown operation %q (want: read, info, list)", op)
	}
}

func (t *UploadedFileTool) listUploads() (string, error) {
	entries, err := os.ReadDir(UploadDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "No files have been uploaded yet.", nil
		}
		return "", fmt.Errorf("cannot read upload dir: %w", err)
	}
	if len(entries) == 0 {
		return "No files have been uploaded yet.", nil
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d uploaded file(s):\n", len(entries)))
	for _, e := range entries {
		info, err := e.Info()
		size := int64(0)
		if err == nil {
			size = info.Size()
		}
		fmt.Fprintf(&b, "- %s (%d bytes)\n", e.Name(), size)
	}
	return b.String(), nil
}

func (t *UploadedFileTool) info(filename string) (string, error) {
	path, err := resolveUpload(filename)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	mimeType := mime.TypeByExtension(filepath.Ext(path))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return fmt.Sprintf(
		"filename: %s\nsize: %d bytes\nmodified: %s\nmime: %s\n",
		filepath.Base(path), info.Size(), info.ModTime().Format("2006-01-02 15:04:05"), mimeType,
	), nil
}

func (t *UploadedFileTool) read(filename string) (string, error) {
	path, err := resolveUpload(filename)
	if err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(path))

	// Images → base64 data URI. The agent itself can't see images, but the
	// frontend can render them inline via the markdown renderer if the model
	// emits the data URI in an ![](data:...) image tag.
	if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		mimeType := "image/png"
		if ext == ".jpg" || ext == ".jpeg" {
			mimeType = "image/jpeg"
		}
		return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data)), nil
	}

	// Binary-ish formats we cannot usefully decode here. Direct the agent to
	// a more specific tool.
	if ext == ".xlsx" || ext == ".xls" {
		return "", fmt.Errorf("%s is a spreadsheet — use the `excel` tool instead (operation: summarize, path: %s)", filepath.Base(path), filepath.Base(path))
	}
	if ext == ".pdf" || ext == ".docx" {
		// Read what we can as raw bytes and surface a hint — agents on small
		// models often do better with this honest signal than with garbled text.
		return "", fmt.Errorf("%s is a binary %s document — text extraction not supported by uploaded_file; ask the user to paste the relevant content or to export it as .txt/.csv", filepath.Base(path), ext)
	}

	// Text-like formats: read directly. Cap at 64 KB so we don't blow out
	// the agent's context window with a 10 MB log upload.
	const maxTextBytes = 64 * 1024
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	buf := make([]byte, maxTextBytes+1)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	truncated := n > maxTextBytes
	if truncated {
		n = maxTextBytes
	}
	out := string(buf[:n])
	if truncated {
		out += fmt.Sprintf("\n\n[... file truncated at %d bytes — use file tool to read in chunks if needed ...]", maxTextBytes)
	}
	return out, nil
}

// resolveUpload sanitizes user input and resolves it against UploadDir only.
// Accepts the UUID-prefixed server-generated filename (the common path) or a
// bare UUID (less common). Rejects path traversal and absolute paths.
func resolveUpload(userInput string) (string, error) {
	userInput = strings.TrimSpace(userInput)
	if userInput == "" {
		return "", fmt.Errorf("filename is required")
	}
	// Strip any directory components the model tried to inject.
	userInput = filepath.Base(userInput)
	if strings.Contains(userInput, "..") {
		return "", fmt.Errorf("invalid filename: %q", userInput)
	}

	// Exact match first.
	candidate := filepath.Join(UploadDir, userInput)
	if isRegular(candidate) {
		return candidate, nil
	}

	// Bare-UUID match: find the first file starting with this prefix.
	entries, err := os.ReadDir(UploadDir)
	if err != nil {
		return "", fmt.Errorf("upload dir not readable: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, userInput+"-") {
			p := filepath.Join(UploadDir, name)
			if isRegular(p) {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("no uploaded file matches %q (use operation=list to see available files)", userInput)
}

func isRegular(p string) bool {
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}
