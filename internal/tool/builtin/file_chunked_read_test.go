package builtin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/metabbe3/Kyoci-Agent/internal/taskctx"
)

// =====================================================================================
// Chunked read tests — verifies the auto-chunk + explicit-offset paths that
// protect the agent's context window when reading large files.
// =====================================================================================

func TestReadFile_SmallFileNoChunk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := NewFileTool()
	tool.SetAllowedDirs([]string{dir})
	ctx := taskctx.WithWorkspace(context.Background(), dir)

	out, err := tool.Execute(ctx, map[string]interface{}{
		"operation": "read",
		"path":      path,
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Small file → no chunk markers, content returned as-is.
	if strings.Contains(out, "=== ") {
		t.Errorf("small file should not get chunk markers, got:\n%s", out)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("content missing: %q", out)
	}
}

func TestReadFile_AutoChunkLargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.txt")
	// 20000 bytes — well over the 8192 auto-chunk threshold.
	body := strings.Repeat("x", 20000)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := NewFileTool()
	tool.SetAllowedDirs([]string{dir})
	ctx := taskctx.WithWorkspace(context.Background(), dir)

	out, err := tool.Execute(ctx, map[string]interface{}{
		"operation": "read",
		"path":      path,
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Should include chunk header + "call again with offset=" trailer.
	if !strings.Contains(out, "=== ") {
		t.Errorf("auto-chunk should add header marker")
	}
	if !strings.Contains(out, "offset=4096") {
		t.Errorf("trailer should point to offset=4096; got tail: %q", tail(out, 200))
	}
	if !strings.Contains(out, "15904 bytes remaining") {
		t.Errorf("should report 15904 bytes remaining (20000-4096); got tail: %q", tail(out, 200))
	}
}

func TestReadFile_ExplicitOffsetAndLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ranged.txt")
	body := strings.Repeat("0123456789", 1000) // 10000 bytes
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := NewFileTool()
	tool.SetAllowedDirs([]string{dir})
	ctx := taskctx.WithWorkspace(context.Background(), dir)

	out, err := tool.Execute(ctx, map[string]interface{}{
		"operation": "read",
		"path":      path,
		"offset":    1000,
		"limit":     500,
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(out, "offset=1000") {
		t.Errorf("header missing offset=1000")
	}
	if !strings.Contains(out, "500/10000 bytes") {
		t.Errorf("header should show 500/10000 bytes; got: %q", head(out, 200))
	}
	if !strings.Contains(out, "offset=1500") {
		t.Errorf("trailer should point to next offset=1500; got tail: %q", tail(out, 200))
	}
}

func TestReadFile_OffsetBeyondEOF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tiny.txt")
	if err := os.WriteFile(path, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := NewFileTool()
	tool.SetAllowedDirs([]string{dir})
	ctx := taskctx.WithWorkspace(context.Background(), dir)

	_, err := tool.Execute(ctx, map[string]interface{}{
		"operation": "read",
		"path":      path,
		"offset":    100,
	})
	if err == nil {
		t.Errorf("expected error for offset beyond EOF")
	}
}

func TestParamInt_ToleratesFloat(t *testing.T) {
	// LLM tool-call args arrive as float64 via JSON; paramInt must coerce.
	cases := []struct {
		in   interface{}
		want int
	}{
		{5, 5},
		{int64(7), 7},
		{float64(9), 9},
		{"not a number", 0},
		{nil, 0},
	}
	for _, c := range cases {
		params := map[string]interface{}{"x": c.in}
		if got := paramInt(params, "x"); got != c.want {
			t.Errorf("paramInt(%v) = %d, want %d", c.in, got, c.want)
		}
	}
	// Missing key.
	if got := paramInt(map[string]interface{}{}, "missing"); got != 0 {
		t.Errorf("missing key should return 0, got %d", got)
	}
}

func head(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
