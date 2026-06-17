package tool

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/tool/builtin"
	"github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// Registry Tests
// =============================================================================

func TestRegistry(t *testing.T) {
	ctx := context.Background()

	t.Run("register and get", func(t *testing.T) {
		registry := NewRegistry()

		// Create a test tool
		tool := &TestTool{
			name:        "test_tool",
			description: "Test tool",
		}

		// Register tool
		err := registry.Register(tool)
		if err != nil {
			t.Fatalf("Failed to register tool: %v", err)
		}

		// Get tool
		retrieved, err := registry.Get("test_tool")
		if err != nil {
			t.Fatalf("Failed to get tool: %v", err)
		}

		if retrieved.Name() != "test_tool" {
			t.Errorf("Expected tool name 'test_tool', got '%s'", retrieved.Name())
		}
	})

	t.Run("duplicate registration", func(t *testing.T) {
		registry := NewRegistry()

		tool := &TestTool{
			name:        "duplicate_tool",
			description: "Duplicate tool",
		}

		// Register first tool
		err := registry.Register(tool)
		if err != nil {
			t.Fatalf("Failed to register tool: %v", err)
		}

		// Try to register duplicate
		err = registry.Register(tool)
		if err == nil {
			t.Error("Expected error for duplicate registration, got nil")
		}
	})

	t.Run("list tools", func(t *testing.T) {
		registry := NewRegistry()

		// Register multiple tools
		tools := []kyoci.Tool{
			&TestTool{name: "tool1", description: "Tool 1"},
			&TestTool{name: "tool2", description: "Tool 2"},
			&TestTool{name: "tool3", description: "Tool 3"},
		}

		for _, tool := range tools {
			if err := registry.Register(tool); err != nil {
				t.Fatalf("Failed to register tool: %v", err)
			}
		}

		// List tools
		definitions := registry.List()
		if len(definitions) != 3 {
			t.Errorf("Expected 3 tools, got %d", len(definitions))
		}

		// Verify names
		names := make(map[string]bool)
		for _, def := range definitions {
			names[def.Name] = true
		}

		expectedNames := []string{"tool1", "tool2", "tool3"}
		for _, name := range expectedNames {
			if !names[name] {
				t.Errorf("Expected tool '%s' not found in list", name)
			}
		}
	})

	t.Run("execute tool", func(t *testing.T) {
		registry := NewRegistry()

		tool := &TestTool{
			name:        "exec_tool",
			description: "Execution test tool",
			result:      "success",
		}

		if err := registry.Register(tool); err != nil {
			t.Fatalf("Failed to register tool: %v", err)
		}

		// Execute tool
		result, err := registry.Execute(ctx, "exec_tool", map[string]interface{}{})
		if err != nil {
			t.Fatalf("Failed to execute tool: %v", err)
		}

		if result != "success" {
			t.Errorf("Expected result 'success', got '%s'", result)
		}
	})

	t.Run("get non-existent tool", func(t *testing.T) {
		registry := NewRegistry()

		_, err := registry.Get("non_existent")
		if err == nil {
			t.Error("Expected error for non-existent tool, got nil")
		}
	})

	t.Run("register builtin tools", func(t *testing.T) {
		registry := NewRegistry()

		err := registry.RegisterBuiltin()
		if err != nil {
			t.Fatalf("Failed to register builtin tools: %v", err)
		}

		// Check that all builtin tools are registered
		expectedTools := []string{"terminal", "file", "http_client", "web_search", "calculator"}
		for _, name := range expectedTools {
			if !registry.Has(name) {
				t.Errorf("Expected builtin tool '%s' not registered", name)
			}
		}
	})
}

// =============================================================================
// Terminal Tool Tests
// =============================================================================

func TestTerminalTool(t *testing.T) {
	ctx := context.Background()
	tool := builtin.NewTerminalTool()

	t.Run("valid command", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"command": "echo 'hello world'",
		})

		if err != nil {
			t.Fatalf("Command failed: %v", err)
		}

		if result == "" {
			t.Error("Expected non-empty result")
		}
	})

	t.Run("command with timeout", func(t *testing.T) {
		_, err := tool.Execute(ctx, map[string]interface{}{
			"command": "sleep 1",
			"timeout": 5,
		})

		if err != nil {
			t.Fatalf("Command failed: %v", err)
		}
	})

	t.Run("blocked dangerous command", func(t *testing.T) {
		_, err := tool.Execute(ctx, map[string]interface{}{
			"command": "rm -rf /",
		})

		if err == nil {
			t.Error("Expected error for dangerous command, got nil")
		}
	})

	t.Run("command not found", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"command": "nonexistent_command_xyz123",
		})

		// Terminal tool returns command-not-found as output (not error) so the
		// agent can see what went wrong and adapt. The output should mention
		// "not found" or "No such file".
		if err != nil {
			t.Logf("Got error (acceptable): %v", err)
			return
		}
		// If no error, the result should indicate the command was not found
		resultStr := fmt.Sprintf("%v", result)
		if !strings.Contains(resultStr, "not found") && !strings.Contains(resultStr, "No such file") {
			t.Errorf("Expected 'not found' or 'No such file' in output, got: %v", result)
		}
	})

	t.Run("missing command parameter", func(t *testing.T) {
		_, err := tool.Execute(ctx, map[string]interface{}{})

		if err == nil {
			t.Error("Expected error for missing command parameter, got nil")
		}
	})
}

// =============================================================================
// File Tool Tests
// =============================================================================

func TestFileTool(t *testing.T) {
	ctx := context.Background()
	tool := builtin.NewFileTool()

	// Create a temporary directory for tests
	tempDir, err := os.MkdirTemp("", "tool_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tool.SetAllowedDirs([]string{tempDir})

	t.Run("write and read file", func(t *testing.T) {
		testFile := filepath.Join(tempDir, "test.txt")
		content := "Hello, World!"

		// Write
		_, err := tool.Execute(ctx, map[string]interface{}{
			"operation": "write",
			"path":      testFile,
			"content":   content,
		})

		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		// Read
		result, err := tool.Execute(ctx, map[string]interface{}{
			"operation": "read",
			"path":      testFile,
		})

		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		if result != content {
			t.Errorf("Expected content '%s', got '%s'", content, result)
		}
	})

	t.Run("append to file", func(t *testing.T) {
		testFile := filepath.Join(tempDir, "append.txt")

		// Write initial content
		_, err := tool.Execute(ctx, map[string]interface{}{
			"operation": "write",
			"path":      testFile,
			"content":   "Hello",
		})

		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		// Append
		_, err = tool.Execute(ctx, map[string]interface{}{
			"operation": "append",
			"path":      testFile,
			"content":   " World",
		})

		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}

		// Read
		result, err := tool.Execute(ctx, map[string]interface{}{
			"operation": "read",
			"path":      testFile,
		})

		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		if result != "Hello World" {
			t.Errorf("Expected 'Hello World', got '%s'", result)
		}
	})

	t.Run("check file exists", func(t *testing.T) {
		testFile := filepath.Join(tempDir, "exists.txt")

		// Create file
		_, err := tool.Execute(ctx, map[string]interface{}{
			"operation": "write",
			"path":      testFile,
			"content":   "test",
		})

		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		// Check exists
		result, err := tool.Execute(ctx, map[string]interface{}{
			"operation": "exists",
			"path":      testFile,
		})

		if err != nil {
			t.Fatalf("Exists check failed: %v", err)
		}

		if result == "" {
			t.Error("Expected non-empty result")
		}
	})

	t.Run("list directory", func(t *testing.T) {
		// Create test files
		for i := 0; i < 3; i++ {
			file := filepath.Join(tempDir, "file"+string(rune('0'+i))+".txt")
			_, err := tool.Execute(ctx, map[string]interface{}{
				"operation": "write",
				"path":      file,
				"content":   "test",
			})
			if err != nil {
				t.Fatalf("Write failed: %v", err)
			}
		}

		// List
		_, err := tool.Execute(ctx, map[string]interface{}{
			"operation": "list",
			"path":      tempDir,
		})

		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
	})

	t.Run("read non-existent file", func(t *testing.T) {
		_, err := tool.Execute(ctx, map[string]interface{}{
			"operation": "read",
			"path":      filepath.Join(tempDir, "nonexistent.txt"),
		})

		if err == nil {
			t.Error("Expected error for non-existent file, got nil")
		}
	})

	t.Run("path outside allowed directories", func(t *testing.T) {
		_, err := tool.Execute(ctx, map[string]interface{}{
			"operation": "read",
			"path":      "/etc/passwd",
		})

		if err == nil {
			t.Error("Expected error for path outside allowed directories, got nil")
		}
	})
}

// =============================================================================
// Change D: Tilde expansion + home-dir hints in file tool
// =============================================================================
//
// Small models (8B) commonly try ~/Documents when they don't know the absolute
// home path, and they invent bogus paths like /documents when guessing. Without
// tilde expansion the call fails with an opaque "directory not found"; with a
// home-dir hint in the error, the model can recover on the next turn.

// TestFileTool_ExpandTilde_ToHomeDir verifies that ~/filename resolves to
// $HOME/filename. The file tool must expand the tilde before checking allowed
// directories and before the operation dispatch.
func TestFileTool_ExpandTilde_ToHomeDir(t *testing.T) {
	ctx := context.Background()

	tempHome, err := os.MkdirTemp("", "tilde_home_")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	// Point HOME at tempHome BEFORE constructing the tool — NewFileTool
	// captures os.UserHomeDir() at init for allowedDirs.
	t.Setenv("HOME", tempHome)

	tool := builtin.NewFileTool()

	// Place a real file under the fake home.
	targetFile := filepath.Join(tempHome, "target.txt")
	if err := os.WriteFile(targetFile, []byte("home content"), 0644); err != nil {
		t.Fatalf("Failed to write target: %v", err)
	}

	result, err := tool.Execute(ctx, map[string]interface{}{
		"operation": "read",
		"path":      "~/target.txt",
	})
	if err != nil {
		t.Fatalf("read ~/target.txt failed: %v — tilde must expand to $HOME", err)
	}
	if result != "home content" {
		t.Errorf("expected 'home content', got %q", result)
	}
}

// TestFileTool_TildeAlone_IsHome verifies that the bare tilde ~ resolves to
// the home directory and can be used with the list operation.
func TestFileTool_TildeAlone_IsHome(t *testing.T) {
	ctx := context.Background()

	tempHome, err := os.MkdirTemp("", "tilde_alone_")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	t.Setenv("HOME", tempHome)
	tool := builtin.NewFileTool()

	// Drop a marker file so we can verify the listing is the home dir.
	if err := os.WriteFile(filepath.Join(tempHome, "marker.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("Failed to write marker: %v", err)
	}

	result, err := tool.Execute(ctx, map[string]interface{}{
		"operation": "list",
		"path":      "~",
	})
	if err != nil {
		t.Fatalf("list ~ failed: %v — bare tilde must expand to $HOME", err)
	}
	if !strings.Contains(result, "marker.txt") {
		t.Errorf("expected marker.txt in home listing; got: %s", result)
	}
}

// TestFileTool_ListMissingDir_IncludesHomeHint verifies that the "directory
// not found" error includes a hint pointing at the user's home directory. The
// 8B model frequently guesses /documents or similar; the hint lets it recover.
func TestFileTool_ListMissingDir_IncludesHomeHint(t *testing.T) {
	ctx := context.Background()

	tempHome, err := os.MkdirTemp("", "hint_home_")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	t.Setenv("HOME", tempHome)
	tool := builtin.NewFileTool()

	// A nonexistent subdir WITHIN the allowed dir so isPathAllowed passes and
	// listDirectory produces the "not found" error.
	missingDir := filepath.Join(tempHome, "does_not_exist")
	_, err = tool.Execute(ctx, map[string]interface{}{
		"operation": "list",
		"path":      missingDir,
	})
	if err == nil {
		t.Fatal("Expected error for missing directory, got nil")
	}
	// The hint must mention the home directory path so the model can recover.
	if !strings.Contains(err.Error(), tempHome) {
		t.Errorf("error should include home-dir hint %q for model recovery; got: %v", tempHome, err)
	}
}

// TestFileTool_EmptyPath_ReturnsClearError verifies that an empty path produces
// a clear "required" error rather than a panic or an opaque failure.
func TestFileTool_EmptyPath_ReturnsClearError(t *testing.T) {
	ctx := context.Background()
	tool := builtin.NewFileTool()

	_, err := tool.Execute(ctx, map[string]interface{}{
		"operation": "read",
		"path":      "",
	})
	if err == nil {
		t.Fatal("Expected error for empty path, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "required") {
		t.Errorf("error should mention 'required'; got: %v", err)
	}
}

// =============================================================================
// File Tool Edit Operation (Change D extension for Orchestrator-Worker)
// =============================================================================
//
// The edit operation does in-place string replacement (like Claude Code's Edit
// tool). This is critical for the worker pattern: workers need to make precise
// surgical edits without rewriting entire files.

// TestFileTool_Edit_ReplacesString verifies the happy path: write a file, edit
// one substring, read it back to confirm the replacement landed.
func TestFileTool_Edit_ReplacesString(t *testing.T) {
	ctx := context.Background()
	tool := builtin.NewFileTool()

	tempDir, err := os.MkdirTemp("", "edit_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	tool.SetAllowedDirs([]string{tempDir})

	testFile := filepath.Join(tempDir, "target.txt")
	_, err = tool.Execute(ctx, map[string]interface{}{
		"operation": "write",
		"path":      testFile,
		"content":   "hello world",
	})
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	_, err = tool.Execute(ctx, map[string]interface{}{
		"operation":  "edit",
		"path":       testFile,
		"old_string": "world",
		"new_string": "earth",
	})
	if err != nil {
		t.Fatalf("edit failed: %v", err)
	}

	result, err := tool.Execute(ctx, map[string]interface{}{
		"operation": "read",
		"path":      testFile,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if result != "hello earth" {
		t.Errorf("expected 'hello earth' after edit, got %q", result)
	}
}

// TestFileTool_Edit_OldStringNotFound_ReturnsError verifies that editing with
// a nonexistent old_string produces a clear error rather than silently no-op'ing.
func TestFileTool_Edit_OldStringNotFound_ReturnsError(t *testing.T) {
	ctx := context.Background()
	tool := builtin.NewFileTool()

	tempDir, err := os.MkdirTemp("", "edit_notfound_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	tool.SetAllowedDirs([]string{tempDir})

	testFile := filepath.Join(tempDir, "target.txt")
	_, _ = tool.Execute(ctx, map[string]interface{}{
		"operation": "write",
		"path":      testFile,
		"content":   "hello world",
	})

	_, err = tool.Execute(ctx, map[string]interface{}{
		"operation":  "edit",
		"path":       testFile,
		"old_string": "nonexistent_text",
		"new_string": "replacement",
	})
	if err == nil {
		t.Fatal("Expected error for nonexistent old_string, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found'; got: %v", err)
	}
}

// TestFileTool_Edit_EmptyOldString_ReturnsError verifies the guard: an empty
// old_string would match everything (or cause a panic in some replace impls).
func TestFileTool_Edit_EmptyOldString_ReturnsError(t *testing.T) {
	ctx := context.Background()
	tool := builtin.NewFileTool()

	tempDir, err := os.MkdirTemp("", "edit_empty_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	tool.SetAllowedDirs([]string{tempDir})

	testFile := filepath.Join(tempDir, "target.txt")
	_, _ = tool.Execute(ctx, map[string]interface{}{
		"operation": "write",
		"path":      testFile,
		"content":   "hello world",
	})

	_, err = tool.Execute(ctx, map[string]interface{}{
		"operation":  "edit",
		"path":       testFile,
		"old_string": "",
		"new_string": "replacement",
	})
	if err == nil {
		t.Fatal("Expected error for empty old_string, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "required") {
		t.Errorf("error should mention 'required'; got: %v", err)
	}
}

// =============================================================================
// HTTP Tool Tests
// =============================================================================

func TestHTTPTool(t *testing.T) {
	ctx := context.Background()
	tool := builtin.NewHTTPTool()

	t.Run("GET request success", func(t *testing.T) {
		// Create mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("Expected GET request, got %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Hello, World!"))
		}))
		defer server.Close()

		result, err := tool.Execute(ctx, map[string]interface{}{
			"url": server.URL,
		})

		if err != nil {
			t.Fatalf("HTTP request failed: %v", err)
		}

		if result == "" {
			t.Error("Expected non-empty result")
		}
	})

	t.Run("POST request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("Expected POST request, got %s", r.Method)
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("Created"))
		}))
		defer server.Close()

		_, err := tool.Execute(ctx, map[string]interface{}{
			"url":    server.URL,
			"method": "POST",
			"body":   `{"test": "data"}`,
		})

		if err != nil {
			t.Fatalf("HTTP request failed: %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(5 * time.Second)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		_, err := tool.Execute(ctx, map[string]interface{}{
			"url":     server.URL,
			"timeout": 1,
		})

		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})

	t.Run("non-2xx status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Not Found"))
		}))
		defer server.Close()

		result, err := tool.Execute(ctx, map[string]interface{}{
			"url": server.URL,
		})

		if err != nil {
			t.Fatalf("HTTP request failed: %v", err)
		}

		if result == "" {
			t.Error("Expected non-empty result even for 404")
		}
	})

	t.Run("invalid URL", func(t *testing.T) {
		_, err := tool.Execute(ctx, map[string]interface{}{
			"url": "not-a-valid-url",
		})

		if err == nil {
			t.Error("Expected error for invalid URL, got nil")
		}
	})

	t.Run("missing URL parameter", func(t *testing.T) {
		_, err := tool.Execute(ctx, map[string]interface{}{})

		if err == nil {
			t.Error("Expected error for missing URL, got nil")
		}
	})
}

// =============================================================================
// Calculator Tool Tests
// =============================================================================

func TestCalculatorTool(t *testing.T) {
	ctx := context.Background()
	tool := builtin.NewCalculatorTool()

	t.Run("basic addition", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"expression": "2 + 3",
		})

		if err != nil {
			t.Fatalf("Calculation failed: %v", err)
		}

		if result != "2 + 3 = 5" {
			t.Errorf("Expected '2 + 3 = 5', got '%s'", result)
		}
	})

	t.Run("subtraction", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"expression": "10 - 4",
		})

		if err != nil {
			t.Fatalf("Calculation failed: %v", err)
		}

		if result != "10 - 4 = 6" {
			t.Errorf("Expected '10 - 4 = 6', got '%s'", result)
		}
	})

	t.Run("multiplication", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"expression": "6 * 7",
		})

		if err != nil {
			t.Fatalf("Calculation failed: %v", err)
		}

		if result != "6 * 7 = 42" {
			t.Errorf("Expected '6 * 7 = 42', got '%s'", result)
		}
	})

	t.Run("division", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"expression": "20 / 4",
		})

		if err != nil {
			t.Fatalf("Calculation failed: %v", err)
		}

		if result != "20 / 4 = 5" {
			t.Errorf("Expected '20 / 4 = 5', got '%s'", result)
		}
	})

	t.Run("parentheses", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"expression": "(2 + 3) * 4",
		})

		if err != nil {
			t.Fatalf("Calculation failed: %v", err)
		}

		if result != "(2 + 3) * 4 = 20" {
			t.Errorf("Expected '(2 + 3) * 4 = 20', got '%s'", result)
		}
	})

	t.Run("complex expression", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"expression": "2 + 3 * 4 - 1",
		})

		if err != nil {
			t.Fatalf("Calculation failed: %v", err)
		}

		if result != "2 + 3 * 4 - 1 = 13" {
			t.Errorf("Expected '2 + 3 * 4 - 1 = 13', got '%s'", result)
		}
	})

	t.Run("division by zero", func(t *testing.T) {
		_, err := tool.Execute(ctx, map[string]interface{}{
			"expression": "10 / 0",
		})

		if err == nil {
			t.Error("Expected error for division by zero, got nil")
		}
	})

	t.Run("invalid expression", func(t *testing.T) {
		_, err := tool.Execute(ctx, map[string]interface{}{
			"expression": "2 + * 3",
		})

		if err == nil {
			t.Error("Expected error for invalid expression, got nil")
		}
	})

	t.Run("missing expression parameter", func(t *testing.T) {
		_, err := tool.Execute(ctx, map[string]interface{}{})

		if err == nil {
			t.Error("Expected error for missing expression, got nil")
		}
	})
}

// =============================================================================
// Test Helper: TestTool
// =============================================================================

// TestTool is a simple tool for testing.
type TestTool struct {
	name        string
	description string
	result      string
}

func (t *TestTool) Name() string {
	return t.name
}

func (t *TestTool) Description() string {
	return t.description
}

func (t *TestTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{}
}

func (t *TestTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	return t.result, nil
}