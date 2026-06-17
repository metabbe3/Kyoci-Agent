package promptskill

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSkillFile is a test helper that writes a markdown skill file with the
// given frontmatter + body into dir under category/name.md.
func writeSkillFile(t *testing.T, dir, category, name, content string) {
	t.Helper()
	sub := filepath.Join(dir, category)
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", sub, err)
	}
	path := filepath.Join(sub, name+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

const macosSkillMD = `---
name: macos-control
description: Control macOS via osascript and shell
category: os-control
triggers:
  keywords: [macos, mac, osascript, applescript]
  regex: ["\\bmac ?os\\b"]
requires: []
priority: high
---

# macOS Control

Use osascript and shell to control the Mac.

## Disk
` + "`df -h`" + `
`

// TestLoad_ReadsMarkdownWithFrontmatter verifies that Load walks a directory
// tree, parses YAML frontmatter, and captures the markdown body.
func TestLoad_ReadsMarkdownWithFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "os-control", "macos", macosSkillMD)

	reg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	skills := reg.List()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	s := skills[0]
	if s.Name != "macos-control" {
		t.Errorf("Name = %q, want %q", s.Name, "macos-control")
	}
	if s.Category != "os-control" {
		t.Errorf("Category = %q, want %q", s.Category, "os-control")
	}
	if s.Description != "Control macOS via osascript and shell" {
		t.Errorf("Description = %q", s.Description)
	}
	if s.Priority != "high" {
		t.Errorf("Priority = %q, want %q", s.Priority, "high")
	}
	if len(s.Triggers.Keywords) != 4 {
		t.Errorf("Keywords len = %d, want 4", len(s.Triggers.Keywords))
	}
	if len(s.Triggers.Regex) != 1 {
		t.Errorf("Regex len = %d, want 1", len(s.Triggers.Regex))
	}
	// Body must contain the markdown AFTER the frontmatter delimiter.
	if s.Body == "" {
		t.Error("Body is empty; expected markdown content after frontmatter")
	}
	if !contains(s.Body, "# macOS Control") {
		t.Errorf("Body missing heading; got: %q", s.Body)
	}
	if !contains(s.Body, "df -h") {
		t.Errorf("Body missing code; got: %q", s.Body)
	}
}

// TestLoad_MultipleFilesAcrossCategories verifies Load walks nested
// subdirectories and indexes all skills.
func TestLoad_MultipleFilesAcrossCategories(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "os-control", "macos", macosSkillMD)
	writeSkillFile(t, dir, "server-admin", "nginx", `---
name: nginx
description: nginx admin
category: server-admin
triggers:
  keywords: [nginx]
priority: normal
---

# nginx
`)
	writeSkillFile(t, dir, "software-development", "code-review", `---
name: code-review
description: code review
category: software-development
triggers:
  keywords: [code review, review my code]
priority: normal
---

# code review
`)

	reg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := len(reg.List()); got != 3 {
		t.Errorf("expected 3 skills, got %d", got)
	}

	// Lookup by name
	s, ok := reg.Get("nginx")
	if !ok {
		t.Fatal("Get(nginx) not found")
	}
	if s.Category != "server-admin" {
		t.Errorf("nginx Category = %q", s.Category)
	}
}

// TestLoad_MissingDir_NotFatal verifies a nonexistent directory yields an
// empty registry and no error (lenient — lets the agent boot without skills).
func TestLoad_MissingDir_NotFatal(t *testing.T) {
	reg, err := Load(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("Load(missing dir) returned error: %v", err)
	}
	if got := len(reg.List()); got != 0 {
		t.Errorf("expected 0 skills for missing dir, got %d", got)
	}
}

// TestLoad_SkipsInvalidFrontmatter verifies a file missing required fields is
// skipped with a warning, not fatal — one bad file shouldn't break loading.
func TestLoad_SkipsInvalidFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "bad", "empty", `---
description: no name, no triggers
---

# broken
`)
	writeSkillFile(t, dir, "good", "math", `---
name: math
description: math ops
category: good
triggers:
  keywords: [calculate]
priority: normal
---

# math
`)

	reg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	// Only the valid skill should load.
	if got := len(reg.List()); got != 1 {
		t.Errorf("expected 1 valid skill, got %d", got)
	}
	if _, ok := reg.Get("math"); !ok {
		t.Error("expected math to load; not found")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > len(sub) && containsSub(s, sub))
}

func containsSub(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
