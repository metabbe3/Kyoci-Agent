package gateway

import (
	"testing"
)

func TestAssessCommandSeverity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cmd  string
		want string
	}{
		{"empty", "", "low"},
		{"whitespace", "   ", "low"},
		// CRITICAL — destructive patterns.
		{"rm-rf", "rm -rf /", "critical"},
		{"rmdir", "rmdir foo", "critical"},
		{"sudo", "sudo apt install x", "critical"},
		{"git-force-push", "git push --force origin main", "critical"},
		{"docker-rm", "docker rm abc", "critical"},
		{"drop-table", "psql -c 'drop table users'", "critical"},
		{"kill", "kill -9 1234", "critical"},
		// LOW — safe read/dev commands.
		{"ls", "ls -la", "low"},
		{"git-status", "git status", "low"},
		{"cat", "cat file.txt", "low"},
		{"go-test", "go test ./...", "low"},
		{"grep", "grep -r foo .", "low"},
		{"curl-get", "curl https://example.com", "low"},
		// MEDIUM — unknown / mutating.
		{"curl-post", "curl -X POST https://example.com", "medium"},
		{"unknown", "some-random-binary --flag", "medium"},
	}
	for _, c := range cases {
		if got := assessCommandSeverity(c.cmd); got != c.want {
			t.Errorf("assessCommandSeverity(%q) = %q, want %q [%s]", c.cmd, got, c.want, c.name)
		}
	}
}

func TestIsSafeCommand(t *testing.T) {
	t.Parallel()
	safe := []string{"ls", "cat foo", "git diff", "go build"}
	risky := []string{"rm -rf /", "sudo x", "git push --force", "unknown-binary"}
	for _, c := range safe {
		if !isSafeCommand(c) {
			t.Errorf("isSafeCommand(%q) = false, want true", c)
		}
	}
	for _, c := range risky {
		if isSafeCommand(c) {
			t.Errorf("isSafeCommand(%q) = true, want false", c)
		}
	}
}

func TestAssessSeverity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tool string
		args string // JSON
		want string
	}{
		{"terminal", `{"command":"rm -rf /"}`, "critical"},
		{"terminal", `{"command":"ls -la"}`, "low"},
		{"terminal", `{"command":"curl -X POST http://x"}`, "medium"},
		{"file", `{"action":"read","path":"/x"}`, "low"},
		{"file", `{"action":"write","path":"/x"}`, "medium"},
		{"file", `{"action":"delete","path":"/x"}`, "critical"},
		{"security_scan", `{}`, "low"},
		{"delegation", `{"goal":"x"}`, "medium"},
		{"unknown-tool", `{}`, "low"},
		// Malformed args must not panic.
		{"terminal", "not-json", "low"},
		{"file", "not-json", "medium"},
	}
	for _, c := range cases {
		if got := assessSeverity(c.tool, c.args); got != c.want {
			t.Errorf("assessSeverity(%q, %q) = %q, want %q", c.tool, c.args, got, c.want)
		}
	}
}

func TestKyociParseArgs(t *testing.T) {
	t.Parallel()
	// Valid JSON → populated map.
	m := kyociParseArgs(`{"command":"ls","n":3}`)
	if m == nil {
		t.Fatal("kyociParseArgs(valid) = nil")
	}
	if m["command"] != "ls" {
		t.Errorf("command = %v, want ls", m["command"])
	}
	// Invalid JSON → nil (no panic).
	if got := kyociParseArgs("not json"); got != nil {
		t.Errorf("kyociParseArgs(invalid) = %v, want nil", got)
	}
	// Empty JSON object → empty non-nil map.
	if got := kyociParseArgs(`{}`); got == nil {
		t.Error("kyociParseArgs({}) = nil, want empty map")
	}
}
