package builtin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =====================================================================================
// Project skill tests — 12 skills (status, structure, languages, deps,
// entry_points, test_map, todo_scan, git_log, git_branches, ignore_check,
// env_check, explore).
//
// Uses the shared runSkillCases driver (see encoding_test.go). Each skill is
// also exercised end-to-end against a temp repo fixture so we verify real
// filesystem behavior, not just keyword matching.
// =====================================================================================

// setupProjectFixture creates a temp directory with a small fake repo layout:
//   - .git/HEAD, .git/refs/heads/main, .git/logs/HEAD
//   - main.go (Go entry point with main())
//   - foo_test.go (Go test)
//   - utils.py (Python source)
//   - .gitignore (ignores node_modules/, *.log, build/)
//   - go.mod, requirements.txt
//   - README.md with a TODO comment
// Returns the temp dir path (caller should defer os.RemoveAll).
func setupProjectFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// .git internals — simulate a repo with one commit on main.
	gitDir := filepath.Join(dir, ".git")
	mustMkdir(t, filepath.Join(gitDir, "refs", "heads"))
	mustMkdir(t, filepath.Join(gitDir, "logs"))
	mustWrite(t, filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"))
	mustWrite(t, filepath.Join(gitDir, "refs", "heads", "main"), []byte("abcdef1234567890abcdef1234567890abcdef12\n"))
	mustWrite(t, filepath.Join(gitDir, "logs", "HEAD"), []byte(
		"0000000000000000000000000000000000000000 abcdef1234567890abcdef1234567890abcdef12 Nicholas <nich@example.com> 1700000000 +0000\tinitial commit\n" +
			"abcdef1234567890abcdef1234567890abcdef12 1234567890abcdef1234567890abcdef1234567 Nicholas <nich@example.com> 1700000100 +0000\tadd tests\n",
	))

	// Source files.
	mustWrite(t, filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"))
	mustWrite(t, filepath.Join(dir, "foo_test.go"), []byte("package main\n\nimport \"testing\"\n\nfunc TestFoo(t *testing.T) {}\nfunc TestBar(t *testing.T) {}\n"))
	mustWrite(t, filepath.Join(dir, "utils.py"), []byte("import os\n\ndef hello():\n    return os.getenv('API_KEY')\n"))
	mustWrite(t, filepath.Join(dir, "app.js"), []byte("console.log(process.env.DATABASE_URL);\n"))
	mustWrite(t, filepath.Join(dir, "README.md"), []byte("# Project\n\nTODO: write more docs.\nFIXME: this is broken.\n"))
	mustWrite(t, filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n\nrequire (\n\tgithub.com/foo/bar v1.2.3\n\tgithub.com/baz/qux v2.0.0\n)\n"))
	mustWrite(t, filepath.Join(dir, "requirements.txt"), []byte("requests==2.31.0\nflask>=2.0.0\nnumpy\n"))
	mustWrite(t, filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"))
	mustWrite(t, filepath.Join(dir, ".gitignore"), []byte("node_modules/\n*.log\nbuild/\n.env\n"))
	mustWrite(t, filepath.Join(dir, ".env.example"), []byte("API_KEY=changeme\nDATABASE_URL=postgres://localhost\n"))
	mustWrite(t, filepath.Join(dir, "app.log"), []byte("ignored log line\n"))

	// Build dir that should be skipped.
	mustMkdir(t, filepath.Join(dir, "build"))
	mustWrite(t, filepath.Join(dir, "build", "artifact.bin"), []byte{0x00, 0x01, 0x02, 0x03})

	return dir
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// =====================================================================================
// Match() smoke tests — verifies keyword targeting.
// =====================================================================================

func TestProjectStatusSkill_Match(t *testing.T) {
	s := NewProjectStatusSkill()
	positives := []string{"project status: .", "repo status", "repository status overview"}
	negatives := []string{"slugify this", "project git log", "base64 encode"}
	for _, q := range positives {
		if !s.Match(q) {
			t.Errorf("Match(%q) = false, want true", q)
		}
	}
	for _, q := range negatives {
		if s.Match(q) {
			t.Errorf("Match(%q) = true, want false", q)
		}
	}
}

func TestProjectStructureSkill_Match(t *testing.T) {
	s := NewProjectStructureSkill()
	positives := []string{"project structure: .", "directory tree", "folder structure"}
	negatives := []string{"base64 encode", "uuid v4"}
	for _, q := range positives {
		if !s.Match(q) {
			t.Errorf("Match(%q) = false, want true", q)
		}
	}
	for _, q := range negatives {
		if s.Match(q) {
			t.Errorf("Match(%q) = true, want false", q)
		}
	}
}

func TestProjectDepsSkill_Match(t *testing.T) {
	s := NewProjectDepsSkill()
	positives := []string{"project deps: .", "list dependencies", "project dependencies"}
	negatives := []string{"checksum md5", "sha256 of foo"}
	for _, q := range positives {
		if !s.Match(q) {
			t.Errorf("Match(%q) = false, want true", q)
		}
	}
	for _, q := range negatives {
		if s.Match(q) {
			t.Errorf("Match(%q) = true, want false", q)
		}
	}
}

func TestProjectEntryPointsSkill_Match(t *testing.T) {
	s := NewProjectEntryPointsSkill()
	positives := []string{"project entry points: .", "find entry points", "project entrypoints"}
	negatives := []string{"uuid generate", "roman to int"}
	for _, q := range positives {
		if !s.Match(q) {
			t.Errorf("Match(%q) = false, want true", q)
		}
	}
	for _, q := range negatives {
		if s.Match(q) {
			t.Errorf("Match(%q) = true, want false", q)
		}
	}
}

func TestProjectTODOScanSkill_Match(t *testing.T) {
	s := NewProjectTODOScanSkill()
	positives := []string{"project todo: .", "scan todos", "find fixme"}
	negatives := []string{"sha256 of foo", "base64 decode"}
	for _, q := range positives {
		if !s.Match(q) {
			t.Errorf("Match(%q) = false, want true", q)
		}
	}
	for _, q := range negatives {
		if s.Match(q) {
			t.Errorf("Match(%q) = true, want false", q)
		}
	}
}

func TestProjectGitLogSkill_Match(t *testing.T) {
	s := NewProjectGitLogSkill()
	positives := []string{"project git log: .", "recent commits", "git log recent"}
	negatives := []string{"roman to int: X", "now time"}
	for _, q := range positives {
		if !s.Match(q) {
			t.Errorf("Match(%q) = false, want true", q)
		}
	}
	for _, q := range negatives {
		if s.Match(q) {
			t.Errorf("Match(%q) = true, want false", q)
		}
	}
}

func TestProjectIgnoreCheckSkill_Match(t *testing.T) {
	s := NewProjectIgnoreCheckSkill()
	positives := []string{"project ignore check: app.log", "gitignore check foo", "is gitignored foo"}
	negatives := []string{"json flatten", "uuid v4"}
	for _, q := range positives {
		if !s.Match(q) {
			t.Errorf("Match(%q) = false, want true", q)
		}
	}
	for _, q := range negatives {
		if s.Match(q) {
			t.Errorf("Match(%q) = true, want false", q)
		}
	}
}

func TestProjectExploreSkill_Match(t *testing.T) {
	s := NewProjectExploreSkill()
	positives := []string{"project explore: .", "explore project", "project overview"}
	negatives := []string{"now time", "uuid v4"}
	for _, q := range positives {
		if !s.Match(q) {
			t.Errorf("Match(%q) = false, want true", q)
		}
	}
	for _, q := range negatives {
		if s.Match(q) {
			t.Errorf("Match(%q) = true, want false", q)
		}
	}
}

// =====================================================================================
// ReadOnlyFSSkill marker — every project_* skill must satisfy this so the
// orchestrator can identify read-only filesystem access.
// =====================================================================================

func TestProjectSkills_ImplementReadOnlyFS(t *testing.T) {
	skills := []kyociReadOnlyFS{
		NewProjectStatusSkill(),
		NewProjectStructureSkill(),
		NewProjectLanguagesSkill(),
		NewProjectDepsSkill(),
		NewProjectEntryPointsSkill(),
		NewProjectTestMapSkill(),
		NewProjectTODOScanSkill(),
		NewProjectGitLogSkill(),
		NewProjectGitBranchesSkill(),
		NewProjectIgnoreCheckSkill(),
		NewProjectEnvCheckSkill(),
		NewProjectExploreSkill(),
	}
	for i, s := range skills {
		if !s.IsReadOnlyFS() {
			t.Errorf("skill %d (%T) does not report IsReadOnlyFS=true", i, s)
		}
	}
}

// kyociReadOnlyFS is a local helper interface to avoid an import cycle in the
// test file — it mirrors kyoci.ReadOnlyFSSkill.IsReadOnlyFS().
type kyociReadOnlyFS interface {
	IsReadOnlyFS() bool
}

// =====================================================================================
// End-to-end Execute() tests against a temp repo fixture.
// =====================================================================================

func TestProjectStatus_Execute(t *testing.T) {
	dir := setupProjectFixture(t)
	out, err := NewProjectStatusSkill().Execute(context.Background(), "project status: "+dir)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"Branch: main", "Commit: abcdef1"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestProjectStructure_Execute(t *testing.T) {
	dir := setupProjectFixture(t)
	out, err := NewProjectStructureSkill().Execute(context.Background(), "project structure: "+dir)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"main.go", "foo_test.go", "Dockerfile"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in tree, got:\n%s", want, out)
		}
	}
	// .git contents should be pruned.
	if strings.Contains(out, "refs/") {
		t.Errorf("tree should not descend into .git; got:\n%s", out)
	}
}

func TestProjectLanguages_Execute(t *testing.T) {
	dir := setupProjectFixture(t)
	out, err := NewProjectLanguagesSkill().Execute(context.Background(), "project languages: "+dir)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"Go", "Python", "JavaScript", "Markdown"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in language list, got:\n%s", want, out)
		}
	}
}

func TestProjectDeps_Execute(t *testing.T) {
	dir := setupProjectFixture(t)
	out, err := NewProjectDepsSkill().Execute(context.Background(), "project deps: "+dir)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{
		"github.com/foo/bar", "v1.2.3",
		"requests", "2.31.0",
		"go.mod", "requirements.txt",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in deps, got:\n%s", want, out)
		}
	}
}

func TestProjectEntryPoints_Execute(t *testing.T) {
	dir := setupProjectFixture(t)
	out, err := NewProjectEntryPointsSkill().Execute(context.Background(), "project entry points: "+dir)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"main.go", "Dockerfile"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in entry points, got:\n%s", want, out)
		}
	}
}

func TestProjectTestMap_Execute(t *testing.T) {
	dir := setupProjectFixture(t)
	out, err := NewProjectTestMapSkill().Execute(context.Background(), "project test map: "+dir)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "foo_test.go") {
		t.Errorf("expected foo_test.go in test map, got:\n%s", out)
	}
	if !strings.Contains(out, "2 test functions") { // we wrote 2 Go tests
		t.Errorf("expected 2 test functions in output, got:\n%s", out)
	}
}

func TestProjectTODOScan_Execute(t *testing.T) {
	dir := setupProjectFixture(t)
	out, err := NewProjectTODOScanSkill().Execute(context.Background(), "project todo: "+dir)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "TODO") || !strings.Contains(out, "FIXME") {
		t.Errorf("expected TODO and FIXME in scan, got:\n%s", out)
	}
	if !strings.Contains(out, "README.md") {
		t.Errorf("expected README.md referenced, got:\n%s", out)
	}
}

func TestProjectGitLog_Execute(t *testing.T) {
	dir := setupProjectFixture(t)
	out, err := NewProjectGitLogSkill().Execute(context.Background(), "project git log: "+dir)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"initial commit", "add tests", "Nicholas", "nich@example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in git log, got:\n%s", want, out)
		}
	}
}

func TestProjectGitBranches_Execute(t *testing.T) {
	dir := setupProjectFixture(t)
	out, err := NewProjectGitBranchesSkill().Execute(context.Background(), "project git branches: "+dir)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "main") || !strings.Contains(out, "*") {
		t.Errorf("expected main branch with current marker, got:\n%s", out)
	}
}

func TestProjectIgnoreCheck_Execute(t *testing.T) {
	dir := setupProjectFixture(t)
	// app.log is ignored by *.log pattern in .gitignore.
	out, err := NewProjectIgnoreCheckSkill().Execute(context.Background(), "project ignore check "+dir+": app.log")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "IGNORED") {
		t.Errorf("expected app.log to be IGNORED, got: %s", out)
	}
}

func TestProjectEnvCheck_Execute(t *testing.T) {
	dir := setupProjectFixture(t)
	out, err := NewProjectEnvCheckSkill().Execute(context.Background(), "project env check: "+dir)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// API_KEY and DATABASE_URL are referenced and declared.
	for _, want := range []string{"API_KEY", "DATABASE_URL"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in env check, got:\n%s", want, out)
		}
	}
}

func TestProjectExplore_Execute(t *testing.T) {
	dir := setupProjectFixture(t)
	out, err := NewProjectExploreSkill().Execute(context.Background(), "project explore: "+dir)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Capstone should include sections from the sub-skills.
	for _, want := range []string{"## Status", "## Languages", "## Dependencies", "## Tests", "## TODOs", "Branch: main"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in explore report, got:\n%s", want, out)
		}
	}
}

// =====================================================================================
// Helper-level unit tests for gitinternals.go and gitignore.go.
// =====================================================================================

func TestGitInternals_CurrentBranch(t *testing.T) {
	dir := setupProjectFixture(t)
	if got := gitCurrentBranch(dir); got != "main" {
		t.Errorf("gitCurrentBranch = %q, want main", got)
	}
	if got := gitCurrentCommit(dir); !strings.HasPrefix(got, "abcdef1") {
		t.Errorf("gitCurrentCommit = %q, want abcdef1...", got)
	}
}

func TestGitInternals_Log(t *testing.T) {
	dir := setupProjectFixture(t)
	entries := gitLog(dir, 10)
	if len(entries) != 2 {
		t.Fatalf("gitLog = %d entries, want 2", len(entries))
	}
	if entries[0].Summary != "add tests" {
		t.Errorf("first entry should be most recent, got %q", entries[0].Summary)
	}
}

func TestGitInternals_NotARepo(t *testing.T) {
	dir := t.TempDir()
	if got := gitCurrentBranch(dir); got != "" {
		t.Errorf("gitCurrentBranch on non-repo = %q, want empty", got)
	}
}

func TestGitIgnore_Basic(t *testing.T) {
	dir := setupProjectFixture(t)
	m := loadIgnore(dir)
	if m == nil {
		t.Fatal("expected matcher for fixture .gitignore")
	}
	cases := []struct {
		path string
		isDir bool
		want bool
	}{
		{"app.log", false, true},
		{"debug.log", false, true},
		{"node_modules", true, true},
		{"node_modules/foo.js", false, true},
		{"main.go", false, false},
		{"build", true, true},
		{"build/artifact.bin", false, true},
	}
	for _, c := range cases {
		if got := m.Match(c.path, c.isDir); got != c.want {
			t.Errorf("Match(%q, isDir=%v) = %v, want %v", c.path, c.isDir, got, c.want)
		}
	}
}

func TestGitIgnore_Negation(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".gitignore"), []byte("*.log\n!important.log\n"))
	m := loadIgnore(dir)
	if got := m.Match("debug.log", false); !got {
		t.Errorf("debug.log should be ignored")
	}
	if got := m.Match("important.log", false); got {
		t.Errorf("important.log should be un-ignored by negation")
	}
}

func TestWildcardMatch(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"*.go", "main.go", true},
		{"*.go", "main.rs", false},
		{"foo*", "foobar", true},
		{"foo*", "barfoo", false},
		{"?at", "cat", true},
		{"?at", "scat", false},
		{"*", "anything", true},
		{"a*c", "abc", true},
		{"a*c", "abcd", false}, // single segment, "abcd" has stuff after c
	}
	for _, c := range cases {
		if got := wildcardMatch(c.pattern, c.name); got != c.want {
			t.Errorf("wildcardMatch(%q, %q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}
