package agent

import (
	"strings"
	"testing"
)

// =====================================================================================
// Strategy 2: code-block interceptor tests.
//
// The interceptor is the most behavior-sensitive of the three strategies —
// false positives mean overwriting real files. These tests pin down the
// decision rules.
// =====================================================================================

func TestExtractCodeBlocks_Basic(t *testing.T) {
	input := "Here's the fix:\n\n```go\npackage main\n\nfunc main() {}\n```\n"
	blocks := extractCodeBlocks(input)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Lang != "go" {
		t.Errorf("lang = %q, want go", blocks[0].Lang)
	}
	if blocks[0].Filename != "" {
		t.Errorf("filename = %q, want empty", blocks[0].Filename)
	}
	if !strings.Contains(blocks[0].Body, "package main") {
		t.Errorf("body doesn't contain code: %q", blocks[0].Body)
	}
}

func TestExtractCodeBlocks_FilenameInInfoString(t *testing.T) {
	input := "```go main.go\npackage main\n```\n"
	blocks := extractCodeBlocks(input)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks", len(blocks))
	}
	if blocks[0].Filename != "main.go" {
		t.Errorf("filename = %q, want main.go", blocks[0].Filename)
	}
}

func TestExtractCodeBlocks_None(t *testing.T) {
	cases := []string{
		"",
		"plain text, no code",
		"```\nno language\n```", // no lang → still extracted, just with empty lang
	}
	// Only the empty/plain cases return nil.
	if got := extractCodeBlocks(cases[0]); got != nil {
		t.Errorf("empty input: got %v, want nil", got)
	}
	if got := extractCodeBlocks(cases[1]); got != nil {
		t.Errorf("plain text: got %v, want nil", got)
	}
	// "no language" block is still extracted (we may want to skip it later).
	if got := extractCodeBlocks(cases[2]); len(got) != 1 {
		t.Errorf("fence without lang: got %d blocks, want 1", len(got))
	}
}

func TestInterceptCodeBlocks_NoFilenameAmbiguousSkip(t *testing.T) {
	// Two unnamed Go blocks → ambiguous; interceptor should skip both to
	// avoid overwriting main.go twice with potentially different content.
	input := "```go\nfirst\n```\n\n```go\nsecond\n```\n"
	calls := interceptCodeBlocks(input, "fix the bug")
	if len(calls) != 0 {
		t.Errorf("ambiguous multi-block: got %d calls, want 0", len(calls))
	}
}

func TestInterceptCodeBlocks_DescriptionGuessesFilename(t *testing.T) {
	// Step says "fix script.js"; model emits a ```js block with no filename.
	// Interceptor should write to script.js.
	input := "```js\nconsole.log('hi');\n```\n"
	calls := interceptCodeBlocks(input, "fix the bug in script.js")
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0].Name != "file" {
		t.Errorf("tool name = %q, want file", calls[0].Name)
	}
	if !strings.Contains(calls[0].Arguments, "script.js") {
		t.Errorf("args missing script.js: %s", calls[0].Arguments)
	}
	if !strings.Contains(calls[0].Arguments, "console.log('hi');") {
		t.Errorf("args missing body: %s", calls[0].Arguments)
	}
}

func TestInterceptCodeBlocks_ExplicitFilenameWins(t *testing.T) {
	// Fence names a file explicitly; description mentions a different one.
	// Fence wins (the model told us where it wants the file).
	input := "```go app.go\npackage main\n```\n"
	calls := interceptCodeBlocks(input, "fix main.go")
	if len(calls) != 1 {
		t.Fatalf("got %d calls", len(calls))
	}
	if !strings.Contains(calls[0].Arguments, "app.go") {
		t.Errorf("expected app.go from fence, got: %s", calls[0].Arguments)
	}
}

func TestInterceptCodeBlocks_NoLangSkipped(t *testing.T) {
	input := "```\nsome plain text\n```\n"
	calls := interceptCodeBlocks(input, "fix main.go")
	if len(calls) != 0 {
		t.Errorf("no-lang block: got %d calls, want 0", len(calls))
	}
}

func TestInterceptCodeBlocks_NoCodeReturns(t *testing.T) {
	calls := interceptCodeBlocks("just text, no code", "fix main.go")
	if calls != nil {
		t.Errorf("plain text: got %v, want nil", calls)
	}
}

// =====================================================================================
// Strategy 3: sprint mode helper tests.
// =====================================================================================

func TestTruncateTask(t *testing.T) {
	// Short input → unchanged.
	if got := truncateTask("short", 10); got != "short" {
		t.Errorf("short input: got %q", got)
	}
	// Input exactly at limit → unchanged.
	exact := strings.Repeat("a", 80)
	if got := truncateTask(exact, 80); got != exact {
		t.Errorf("exact-limit input: got %q (len %d), want unchanged", got, len(got))
	}
	// Over limit → clipped to n-1 chars + ellipsis char.
	long := strings.Repeat("a", 100)
	got := truncateTask(long, 20)
	// len() counts bytes; "…" is 3 bytes UTF-8. Expect 19 + 3 = 22 bytes.
	if len(got) != 22 {
		t.Errorf("over-limit output byte len = %d, want 22 (n-1 + ellipsis UTF-8)", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("over-limit output should end with ellipsis, got %q", got)
	}
}
