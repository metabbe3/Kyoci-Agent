package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Project-exploration skills — zero-AI, deterministic, pure-Go filesystem and
// .git-internal readers. No shell-out to git, no LLM calls. Works identically
// on macOS, Linux, and Windows.
//
// All skills implement kyoci.ReadOnlyFSSkill marker so the orchestrator can
// identify filesystem-reading skills for sandbox enforcement. They never write
// or mutate.
//
// Each skill accepts an optional path operand (defaults to the current working
// directory). Sandboxing happens at the orchestrator layer; the skills
// themselves just walk files.
// =====================================================================================

// ---- helpers shared across project_* skills ----

// extractProjectPath pulls the path operand out of a query. Strips a leading
// ":" separator and surrounding quotes. Returns "." if empty.
func extractProjectPath(q string, verb string) string {
	low := strings.ToLower(q)
	idx := strings.Index(low, strings.ToLower(verb))
	if idx < 0 {
		return "."
	}
	rest := strings.TrimSpace(q[idx+len(verb):])
	rest = strings.TrimPrefix(rest, ":")
	rest = strings.TrimSpace(rest)
	rest = quoteStripped(rest)
	if rest == "" {
		return "."
	}
	return rest
}

// isBinaryContent returns true if data looks like a binary file (high ratio of
// non-text bytes in the first 512 bytes). Used by project_languages to skip
// binaries that happen to have a known extension.
func isBinaryContent(data []byte) bool {
	const sample = 512
	end := len(data)
	if end > sample {
		end = sample
	}
	if end == 0 {
		return false
	}
	nonText := 0
	for i := 0; i < end; i++ {
		b := data[i]
		if b == 0 { // NUL → definitely binary
			return true
		}
		if b < 0x09 || (b > 0x0d && b < 0x20) { // other control chars
			nonText++
		}
	}
	return nonText*4 > end // >25% control chars → binary
}

// extensionLanguage maps a file extension to a language label. Used by
// project_languages and project_status for the LOC breakdown.
var extensionLanguage = map[string]string{
	".go":    "Go",
	".rs":    "Rust",
	".py":    "Python",
	".ts":    "TypeScript",
	".tsx":   "TypeScript (React)",
	".js":    "JavaScript",
	".jsx":   "JavaScript (React)",
	".java":  "Java",
	".kt":    "Kotlin",
	".swift": "Swift",
	".rb":    "Ruby",
	".php":   "PHP",
	".c":     "C",
	".h":     "C/C++ header",
	".cpp":   "C++",
	".cc":    "C++",
	".hpp":   "C++ header",
	".cs":    "C#",
	".scala": "Scala",
	".sh":    "Shell",
	".bash":  "Shell",
	".zsh":   "Shell",
	".lua":   "Lua",
	".dart":  "Dart",
	".ex":    "Elixir",
	".exs":   "Elixir",
	".erl":   "Erlang",
	".clj":   "Clojure",
	".hs":    "Haskell",
	".ml":    "OCaml",
	".vim":   "Vimscript",
	".sql":   "SQL",
	".html":  "HTML",
	".css":   "CSS",
	".scss":  "SCSS",
	".less":  "Less",
	".vue":   "Vue",
	".svelte": "Svelte",
	".yaml":  "YAML",
	".yml":   "YAML",
	".json":  "JSON",
	".toml":  "TOML",
	".xml":   "XML",
	".md":    "Markdown",
	".markdown": "Markdown",
}

// skipDirs are directory names we never descend into during a walk. They're
// either build artifacts, dependency caches, or VCS state.
var skipDirs = map[string]bool{
	".git":          true,
	"node_modules":  true,
	"vendor":        true,
	"dist":          true,
	"build":         true,
	"target":        true,
	"bin":           true,
	"obj":           true,
	".next":         true,
	".nuxt":         true,
	".cache":        true,
	".idea":         true,
	".vscode":       true,
	"__pycache__":   true,
	".pytest_cache": true,
	".mypy_cache":   true,
	"coverage":      true,
	".gradle":       true,
	".terraform":    true,
}

// walkProjectTree walks repoRoot skipping skipDirs and any ignored paths.
// The visitor receives the absolute path, the dir entry, and the
// repo-relative path. Returning fs.SkipDir from the visitor prunes that
// subtree. The walk is bounded by maxFiles (0 = unlimited) — a safety cap to
// prevent runaway traversal on huge repos.
func walkProjectTree(repoRoot string, maxFiles int, visit func(abs, rel string, d os.DirEntry) error) error {
	ignore := loadIgnore(repoRoot)
	visited := 0
	return filepath.WalkDir(repoRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // tolerate broken symlinks / permission errors
		}
		rel, _ := filepath.Rel(repoRoot, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if p == repoRoot {
				return nil
			}
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			if ignore != nil && ignore.Match(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if ignore != nil && ignore.Match(rel, false) {
			return nil
		}
		if maxFiles > 0 && visited >= maxFiles {
			return filepath.SkipAll
		}
		visited++
		return visit(p, rel, d)
	})
}

// formatBytes produces a human-readable byte size (e.g. "1.5 KB"). Reused here
// so project_status and project_explore share formatting.
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for nn := n / unit; nn >= unit; nn /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// ---- project_status ----

// ProjectStatusSkill produces a one-shot repo snapshot: branch, dirty tree
// indicator, current commit, and a language breakdown.
type ProjectStatusSkill struct{ *kyoci.BaseSkill }

func NewProjectStatusSkill() *ProjectStatusSkill {
	return &ProjectStatusSkill{BaseSkill: kyoci.NewBaseSkill(
		"project_status",
		"One-shot repo snapshot: branch, dirty file indicator, last commit, language breakdown",
		[]string{"project status", "repo status", "git status overview"},
	)}
}

func (s *ProjectStatusSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "project status") || strings.Contains(q, "repo status") ||
		strings.Contains(q, "repository status")
}

func (s *ProjectStatusSkill) IsReadOnlyFS() bool { return true }

func (s *ProjectStatusSkill) Execute(_ context.Context, q string) (string, error) {
	root := extractProjectPath(q, "project status")
	repoRoot, err := resolveRepoRoot(root)
	if err != nil {
		return "", err
	}
	branch := gitCurrentBranch(repoRoot)
	if branch == "" {
		return "", fmt.Errorf("not a git repository: %s", repoRoot)
	}
	commit := gitCurrentCommit(repoRoot)
	changed, _ := gitDirtyCount(repoRoot)
	langBreakdown := languageBreakdown(repoRoot, 2000)
	var b strings.Builder
	fmt.Fprintf(&b, "Repo: %s\n", repoRoot)
	fmt.Fprintf(&b, "Branch: %s\n", branch)
	if commit != "" {
		fmt.Fprintf(&b, "Commit: %s\n", commit)
	}
	if changed > 0 {
		fmt.Fprintf(&b, "Working tree: may have uncommitted changes\n")
	} else {
		fmt.Fprintf(&b, "Working tree: clean\n")
	}
	fmt.Fprintf(&b, "\nLanguages (top 10):\n")
	for i, lc := range langBreakdown.topN(10) {
		fmt.Fprintf(&b, "  %2d. %-22s %8d files  %8d lines\n", i+1, lc.Language, lc.Files, lc.Lines)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// langCount is one row in the language breakdown.
type langCount struct {
	Language string
	Files    int
	Lines    int
	Bytes    int64
}

// langBreakdownResult holds the full breakdown plus helpers.
type langBreakdownResult struct {
	rows []langCount
}

func (r langBreakdownResult) topN(n int) []langCount {
	rows := append([]langCount(nil), r.rows...)
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Lines > rows[j].Lines
	})
	if n > len(rows) {
		n = len(rows)
	}
	return rows[:n]
}

// languageBreakdown walks repoRoot and aggregates file counts and line counts
// per language. Skips binary-looking files and files larger than a sane cap.
// maxFiles caps the walk (0 = unlimited).
func languageBreakdown(repoRoot string, maxFiles int) langBreakdownResult {
	byLang := map[string]*langCount{}
	_ = walkProjectTree(repoRoot, maxFiles, func(abs, rel string, d os.DirEntry) error {
		ext := strings.ToLower(filepath.Ext(rel))
		lang, ok := extensionLanguage[ext]
		if !ok {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Size() > 5*1024*1024 {
			return nil // skip files >5MB
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return nil
		}
		if isBinaryContent(data) {
			return nil
		}
		lines := strings.Count(string(data), "\n") + 1
		row := byLang[lang]
		if row == nil {
			row = &langCount{Language: lang}
			byLang[lang] = row
		}
		row.Files++
		row.Lines += lines
		row.Bytes += info.Size()
		return nil
	})
	rows := make([]langCount, 0, len(byLang))
	for _, v := range byLang {
		rows = append(rows, *v)
	}
	return langBreakdownResult{rows: rows}
}

// ---- project_structure ----

// ProjectStructureSkill produces a depth-limited directory tree of repoRoot,
// respecting .gitignore and skipping common noise dirs.
type ProjectStructureSkill struct{ *kyoci.BaseSkill }

func NewProjectStructureSkill() *ProjectStructureSkill {
	return &ProjectStructureSkill{BaseSkill: kyoci.NewBaseSkill(
		"project_structure",
		"Directory tree of the project, depth-limited, respects .gitignore",
		[]string{"project structure", "directory tree", "repo structure"},
	)}
}

func (s *ProjectStructureSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "project structure") || strings.Contains(q, "directory tree") ||
		strings.Contains(q, "repo structure") || strings.Contains(q, "folder structure")
}

func (s *ProjectStructureSkill) IsReadOnlyFS() bool { return true }

func (s *ProjectStructureSkill) Execute(_ context.Context, q string) (string, error) {
	root := extractProjectPath(q, "project structure")
	repoRoot, err := resolveRepoRoot(root)
	if err != nil {
		return "", err
	}
	maxDepth := 3
	const maxEntries = 1000
	var b strings.Builder
	visited := 0
	_ = filepath.WalkDir(repoRoot, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		rel, _ := filepath.Rel(repoRoot, p)
		rel = filepath.ToSlash(rel)
		depth := strings.Count(rel, "/")
		if d.IsDir() {
			if p == repoRoot {
				fmt.Fprintf(&b, ".\n")
				return nil
			}
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			if depth > maxDepth {
				return filepath.SkipDir
			}
		}
		if visited >= maxEntries {
			return filepath.SkipAll
		}
		visited++
		indent := strings.Repeat("  ", depth)
		marker := ""
		if d.IsDir() {
			marker = "/"
		}
		fmt.Fprintf(&b, "%s%s%s\n", indent, d.Name(), marker)
		return nil
	})
	if visited >= maxEntries {
		fmt.Fprintf(&b, "\n... (truncated at %d entries)\n", maxEntries)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// ---- project_languages ----

// ProjectLanguagesSkill produces a per-language LOC breakdown (top 15).
type ProjectLanguagesSkill struct{ *kyoci.BaseSkill }

func NewProjectLanguagesSkill() *ProjectLanguagesSkill {
	return &ProjectLanguagesSkill{BaseSkill: kyoci.NewBaseSkill(
		"project_languages",
		"Line-count per programming language across the project",
		[]string{"project languages", "language breakdown", "lines per language"},
	)}
}

func (s *ProjectLanguagesSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "project languages") || strings.Contains(q, "language breakdown") ||
		strings.Contains(q, "languages used")
}

func (s *ProjectLanguagesSkill) IsReadOnlyFS() bool { return true }

func (s *ProjectLanguagesSkill) Execute(_ context.Context, q string) (string, error) {
	root := extractProjectPath(q, "project languages")
	repoRoot, err := resolveRepoRoot(root)
	if err != nil {
		return "", err
	}
	breakdown := languageBreakdown(repoRoot, 0)
	top := breakdown.topN(15)
	if len(top) == 0 {
		return "no recognized source files found", nil
	}
	var totalLines, totalFiles int
	for _, r := range top {
		totalLines += r.Lines
		totalFiles += r.Files
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Total: %d files, %d lines across %d languages\n\n", totalFiles, totalLines, len(top))
	for i, lc := range top {
		pct := 0.0
		if totalLines > 0 {
			pct = 100.0 * float64(lc.Lines) / float64(totalLines)
		}
		fmt.Fprintf(&b, "  %2d. %-22s %6d files  %8d lines  (%.1f%%)\n", i+1, lc.Language, lc.Files, lc.Lines, pct)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// ---- project_deps ----

// ProjectDepsSkill parses dep manifests (go.mod, package.json, Cargo.toml,
// requirements.txt, pyproject.toml) and lists the dependencies.
type ProjectDepsSkill struct{ *kyoci.BaseSkill }

func NewProjectDepsSkill() *ProjectDepsSkill {
	return &ProjectDepsSkill{BaseSkill: kyoci.NewBaseSkill(
		"project_deps",
		"List dependencies from go.mod / package.json / Cargo.toml / requirements.txt",
		[]string{"project deps", "list dependencies", "project dependencies"},
	)}
}

func (s *ProjectDepsSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "project deps") || strings.Contains(q, "list dependencies") ||
		strings.Contains(q, "project dependencies") || strings.Contains(q, "repo dependencies")
}

func (s *ProjectDepsSkill) IsReadOnlyFS() bool { return true }

func (s *ProjectDepsSkill) Execute(_ context.Context, q string) (string, error) {
	root := extractProjectPath(q, "project deps")
	repoRoot, err := resolveRepoRoot(root)
	if err != nil {
		return "", err
	}
	deps := collectDependencies(repoRoot)
	if len(deps) == 0 {
		return "no dependency manifests found (go.mod, package.json, Cargo.toml, requirements.txt, pyproject.toml)", nil
	}
	var b strings.Builder
	for _, manifest := range deps {
		fmt.Fprintf(&b, "%s (%s):\n", manifest.Path, manifest.Type)
		if len(manifest.Deps) == 0 {
			fmt.Fprintf(&b, "  (no deps listed)\n")
		}
		for _, d := range manifest.Deps {
			if d.Version != "" {
				fmt.Fprintf(&b, "  %-40s %s\n", d.Name, d.Version)
			} else {
				fmt.Fprintf(&b, "  %s\n", d.Name)
			}
		}
		fmt.Fprintf(&b, "\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// depManifest is one parsed dependency file.
type depManifest struct {
	Path string
	Type string
	Deps []depEntry
}

type depEntry struct {
	Name    string
	Version string
}

// collectDependencies scans repoRoot for known dep manifests and parses them.
// Handles go.mod, package.json, Cargo.toml, requirements.txt, pyproject.toml.
func collectDependencies(repoRoot string) []depManifest {
	var out []depManifest

	// go.mod (text-parseable — avoid pulling x/mod/modfile unless we need it).
	goMod := filepath.Join(repoRoot, "go.mod")
	if data, err := os.ReadFile(goMod); err == nil {
		deps := parseGoMod(string(data))
		out = append(out, depManifest{Path: "go.mod", Type: "Go", Deps: deps})
	}

	// package.json.
	pkgJSON := filepath.Join(repoRoot, "package.json")
	if data, err := os.ReadFile(pkgJSON); err == nil {
		deps := parsePackageJSON(data)
		out = append(out, depManifest{Path: "package.json", Type: "Node.js", Deps: deps})
	}

	// Cargo.toml.
	cargoToml := filepath.Join(repoRoot, "Cargo.toml")
	if data, err := os.ReadFile(cargoToml); err == nil {
		deps := parseCargoToml(string(data))
		out = append(out, depManifest{Path: "Cargo.toml", Type: "Rust", Deps: deps})
	}

	// requirements.txt.
	reqTxt := filepath.Join(repoRoot, "requirements.txt")
	if data, err := os.ReadFile(reqTxt); err == nil {
		deps := parseRequirementsTxt(string(data))
		out = append(out, depManifest{Path: "requirements.txt", Type: "Python", Deps: deps})
	}

	// pyproject.toml (PEP 621 [project.dependencies]).
	pyProject := filepath.Join(repoRoot, "pyproject.toml")
	if data, err := os.ReadFile(pyProject); err == nil {
		deps := parsePyProject(string(data))
		if len(deps) > 0 {
			out = append(out, depManifest{Path: "pyproject.toml", Type: "Python", Deps: deps})
		}
	}
	return out
}

func parseGoMod(data string) []depEntry {
	var out []depEntry
	inRequireBlock := false
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "require ") && !strings.Contains(line, "(") {
			// single-line require
			parts := strings.Fields(strings.TrimPrefix(line, "require "))
			if len(parts) >= 2 {
				out = append(out, depEntry{Name: parts[0], Version: parts[1]})
			}
			continue
		}
		if strings.HasSuffix(line, "require (") {
			inRequireBlock = true
			continue
		}
		if inRequireBlock {
			if line == ")" {
				inRequireBlock = false
				continue
			}
			if line == "" || strings.HasPrefix(line, "//") {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				out = append(out, depEntry{Name: parts[0], Version: parts[1]})
			}
		}
	}
	return out
}

func parsePackageJSON(data []byte) []depEntry {
	var doc struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	var out []depEntry
	// sort keys for deterministic output
	keys := make([]string, 0, len(doc.Dependencies)+len(doc.DevDependencies))
	for k := range doc.Dependencies {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, depEntry{Name: k, Version: doc.Dependencies[k]})
	}
	devKeys := make([]string, 0, len(doc.DevDependencies))
	for k := range doc.DevDependencies {
		devKeys = append(devKeys, k)
	}
	sort.Strings(devKeys)
	for _, k := range devKeys {
		out = append(out, depEntry{Name: k + " (dev)", Version: doc.DevDependencies[k]})
	}
	return out
}

func parseCargoToml(data string) []depEntry {
	var out []depEntry
	inDeps := false
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[dependencies]") {
			inDeps = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inDeps = false
			continue
		}
		if !inDeps || line == "" {
			continue
		}
		// "name = \"version\"" or "name = { version = \"1.0\" }"
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		name := strings.TrimSpace(line[:eq])
		rest := strings.TrimSpace(line[eq+1:])
		version := ""
		if strings.HasPrefix(rest, "\"") {
			version = strings.Trim(rest, "\"")
		} else if i := strings.Index(rest, "version"); i >= 0 {
			sub := rest[i+len("version"):]
			q1 := strings.Index(sub, "\"")
			if q1 >= 0 {
				sub2 := sub[q1+1:]
				q2 := strings.Index(sub2, "\"")
				if q2 >= 0 {
					version = sub2[:q2]
				}
			}
		}
		out = append(out, depEntry{Name: name, Version: version})
	}
	return out
}

func parseRequirementsTxt(data string) []depEntry {
	var out []depEntry
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		// "package==1.0.0", "package>=1.0", "package~=1.0", "package"
		name := line
		version := ""
		for _, sep := range []string{"==", ">=", "<=", "~=", "!=", ">", "<"} {
			if i := strings.Index(line, sep); i > 0 {
				name = strings.TrimSpace(line[:i])
				version = strings.TrimSpace(line[i+len(sep):])
				break
			}
		}
		// Strip environment markers: "package ; python_version >= '3.8'"
		if i := strings.Index(name, ";"); i > 0 {
			name = strings.TrimSpace(name[:i])
		}
		// Strip extras: "package[extra1,extra2]"
		if i := strings.Index(name, "["); i > 0 {
			name = strings.TrimSpace(name[:i])
		}
		out = append(out, depEntry{Name: name, Version: version})
	}
	return out
}

func parsePyProject(data string) []depEntry {
	// Find [project] section, look for "dependencies = [...]".
	inProject := false
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "[project]" {
			inProject = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inProject = false
		}
		if !inProject {
			continue
		}
		if !strings.HasPrefix(line, "dependencies") {
			continue
		}
		// Find the array literal.
		bracketStart := strings.Index(line, "[")
		if bracketStart < 0 {
			continue
		}
		rest := line[bracketStart+1:]
		// Could be inline or multi-line. We handle inline; for multi-line we'd
		// need to keep scanning. Acceptable simplification.
		bracketEnd := strings.Index(rest, "]")
		if bracketEnd < 0 {
			// collect until next "]"
			continue
		}
		contents := rest[:bracketEnd]
		var deps []depEntry
		for _, item := range strings.Split(contents, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			item = strings.Trim(item, "\"'")
			// "package>=1.0" / "package[extra]>=1.0" / "package (with marker)"
			deps = append(deps, depEntry{Name: item})
		}
		return deps
	}
	return nil
}

// ---- project_entry_points ----

// ProjectEntryPointsSkill locates conventional entry points: main packages,
// main() funcs, index.{js,ts}, __init__.py, Dockerfile, Makefile, etc.
type ProjectEntryPointsSkill struct{ *kyoci.BaseSkill }

func NewProjectEntryPointsSkill() *ProjectEntryPointsSkill {
	return &ProjectEntryPointsSkill{BaseSkill: kyoci.NewBaseSkill(
		"project_entry_points",
		"Find entry points: main.go, main(), index.{js,ts}, __init__.py, Dockerfile, Makefile",
		[]string{"project entry points", "find entry points", "project entrypoints"},
	)}
}

func (s *ProjectEntryPointsSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "project entry points") || strings.Contains(q, "find entry points") ||
		strings.Contains(q, "project entrypoints")
}

func (s *ProjectEntryPointsSkill) IsReadOnlyFS() bool { return true }

func (s *ProjectEntryPointsSkill) Execute(_ context.Context, q string) (string, error) {
	root := extractProjectPath(q, "project entry points")
	repoRoot, err := resolveRepoRoot(root)
	if err != nil {
		return "", err
	}
	var entries []string
	const maxEntries = 200
	// Special files (root-level).
	for _, name := range []string{"Dockerfile", "Makefile", "makefile", "docker-compose.yml", "docker-compose.yaml", "Taskfile.yml"} {
		full := filepath.Join(repoRoot, name)
		if _, err := os.Stat(full); err == nil {
			entries = append(entries, name+" (root)")
		}
	}
	_ = walkProjectTree(repoRoot, 0, func(abs, rel string, d os.DirEntry) error {
		if len(entries) >= maxEntries {
			return filepath.SkipAll
		}
		base := d.Name()
		ext := strings.ToLower(filepath.Ext(rel))
		// Go: files containing package main OR a main() function.
		if ext == ".go" {
			if data, err := os.ReadFile(abs); err == nil {
				text := string(data)
				if strings.Contains(text, "package main") &&
					(strings.Contains(text, "func main(") || strings.Contains(text, "func Main(")) {
					entries = append(entries, rel)
					return nil
				}
			}
		}
		// JS/TS: index.{js,ts} and bin shebang scripts.
		if base == "index.js" || base == "index.ts" ||
			base == "main.py" || base == "app.py" ||
			base == "__main__.py" || base == "__init__.py" ||
			base == "main.rs" || base == "Main.java" ||
			base == "Program.cs" || base == "main.go" {
			entries = append(entries, rel)
			return nil
		}
		return nil
	})
	if len(entries) == 0 {
		return "no entry points found", nil
	}
	sort.Strings(entries)
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "  %s\n", e)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// ---- project_test_map ----

// ProjectTestMapSkill finds test files and counts test functions per language.
type ProjectTestMapSkill struct{ *kyoci.BaseSkill }

func NewProjectTestMapSkill() *ProjectTestMapSkill {
	return &ProjectTestMapSkill{BaseSkill: kyoci.NewBaseSkill(
		"project_test_map",
		"List test files (Go _test.go, JS/TS *.test.ts) and count test functions",
		[]string{"project test map", "find tests", "test inventory"},
	)}
}

func (s *ProjectTestMapSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "project test map") || strings.Contains(q, "find tests") ||
		strings.Contains(q, "test inventory") || strings.Contains(q, "list tests")
}

func (s *ProjectTestMapSkill) IsReadOnlyFS() bool { return true }

var (
	goTestFuncRe   = regexp.MustCompile(`^func\s+(Test|Benchmark|Example)[A-Z]\w*\s*\(`)
	pyTestFuncRe   = regexp.MustCompile(`^\s*def\s+(test_\w+)\s*\(`)
	jsTestFuncRe   = regexp.MustCompile(`^\s*(it|test|describe)\s*\(\s*['"]`)
)

func (s *ProjectTestMapSkill) Execute(_ context.Context, q string) (string, error) {
	root := extractProjectPath(q, "project test map")
	repoRoot, err := resolveRepoRoot(root)
	if err != nil {
		return "", err
	}
	type fileEntry struct {
		path  string
		lang  string
		count int
	}
	var entries []fileEntry
	_ = walkProjectTree(repoRoot, 0, func(abs, rel string, d os.DirEntry) error {
		base := d.Name()
		ext := strings.ToLower(filepath.Ext(rel))
		lang := ""
		isTest := false
		switch {
		case ext == ".go" && strings.HasSuffix(base, "_test.go"):
			lang = "Go"
			isTest = true
		case (ext == ".ts" || ext == ".tsx") && strings.HasSuffix(base, ".test.ts"):
			lang = "TypeScript"
			isTest = true
		case (ext == ".ts" || ext == ".tsx") && strings.HasSuffix(base, ".spec.ts"):
			lang = "TypeScript"
			isTest = true
		case (ext == ".js" || ext == ".jsx") && strings.HasSuffix(base, ".test.js"):
			lang = "JavaScript"
			isTest = true
		case (ext == ".js" || ext == ".jsx") && strings.HasSuffix(base, ".spec.js"):
			lang = "JavaScript"
			isTest = true
		case ext == ".py" && strings.HasSuffix(base, "_test.py"):
			lang = "Python"
			isTest = true
		case ext == ".py" && strings.HasPrefix(base, "test_"):
			lang = "Python"
			isTest = true
		case ext == ".rs" && strings.HasSuffix(base, "_test.rs"):
			lang = "Rust"
			isTest = true
		}
		if !isTest {
			return nil
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return nil
		}
		count := 0
		for _, raw := range strings.Split(string(data), "\n") {
			var matched bool
			switch lang {
			case "Go":
				matched = goTestFuncRe.MatchString(raw)
			case "Python":
				matched = pyTestFuncRe.MatchString(raw)
			case "TypeScript", "JavaScript":
				matched = jsTestFuncRe.MatchString(raw)
			case "Rust":
				matched = strings.HasPrefix(strings.TrimSpace(raw), "#[test]")
			}
			if matched {
				count++
			}
		}
		entries = append(entries, fileEntry{path: rel, lang: lang, count: count})
		return nil
	})
	if len(entries) == 0 {
		return "no test files found", nil
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].path < entries[j].path
	})
	total := 0
	var b strings.Builder
	for _, e := range entries {
		total += e.count
		fmt.Fprintf(&b, "  [%-11s] %3d  %s\n", e.lang, e.count, e.path)
	}
	fmt.Fprintf(&b, "\n%d test files, %d test functions\n", len(entries), total)
	return strings.TrimRight(b.String(), "\n"), nil
}

// ---- project_todo_scan ----

// ProjectTODOScanSkill aggregates TODO/FIXME/HACK/XXX comments across the
// project. Reuses the TODO pattern from code_metrics but walks the tree.
type ProjectTODOScanSkill struct{ *kyoci.BaseSkill }

func NewProjectTODOScanSkill() *ProjectTODOScanSkill {
	return &ProjectTODOScanSkill{BaseSkill: kyoci.NewBaseSkill(
		"project_todo_scan",
		"Aggregate TODO/FIXME/HACK/XXX across the project with file:line",
		[]string{"project todo", "scan todos", "find todos"},
	)}
}

func (s *ProjectTODOScanSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "project todo") || strings.Contains(q, "scan todos") ||
		strings.Contains(q, "find todos") || strings.Contains(q, "find fixme") ||
		strings.Contains(q, "find hack")
}

func (s *ProjectTODOScanSkill) IsReadOnlyFS() bool { return true }

var projectTodoMarkerRe = regexp.MustCompile(`\b(TODO|FIXME|HACK|XXX|BUG|NOTE)\b`)

func (s *ProjectTODOScanSkill) Execute(_ context.Context, q string) (string, error) {
	root := extractProjectPath(q, "project todo")
	repoRoot, err := resolveRepoRoot(root)
	if err != nil {
		return "", err
	}
	type todo struct {
		file string
		line int
		text string
		tag  string
	}
	var todos []todo
	const max = 500
	_ = walkProjectTree(repoRoot, 0, func(abs, rel string, d os.DirEntry) error {
		ext := strings.ToLower(filepath.Ext(rel))
		if _, ok := extensionLanguage[ext]; !ok {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 1024*1024 {
			return nil
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if len(todos) >= max {
				return filepath.SkipAll
			}
			loc := projectTodoMarkerRe.FindStringIndex(line)
			if loc == nil {
				continue
			}
			tag := line[loc[0]:loc[1]]
			text := strings.TrimSpace(line[loc[0]:])
			todos = append(todos, todo{file: rel, line: i + 1, text: text, tag: tag})
		}
		return nil
	})
	if len(todos) == 0 {
		return "no TODO/FIXME/HACK/XXX found", nil
	}
	var b strings.Builder
	for _, t := range todos {
		fmt.Fprintf(&b, "  [%s] %s:%d  %s\n", t.tag, t.file, t.line, t.text)
	}
	fmt.Fprintf(&b, "\n%d markers found\n", len(todos))
	return strings.TrimRight(b.String(), "\n"), nil
}

// ---- project_git_log ----

// ProjectGitLogSkill returns the recent commit history by parsing
// .git/logs/HEAD directly (no git binary).
type ProjectGitLogSkill struct{ *kyoci.BaseSkill }

func NewProjectGitLogSkill() *ProjectGitLogSkill {
	return &ProjectGitLogSkill{BaseSkill: kyoci.NewBaseSkill(
		"project_git_log",
		"Recent commits parsed from .git/logs/HEAD (no git binary needed)",
		[]string{"project git log", "git log recent", "recent commits"},
	)}
}

func (s *ProjectGitLogSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "project git log") || strings.Contains(q, "recent commits") ||
		strings.Contains(q, "git log recent")
}

func (s *ProjectGitLogSkill) IsReadOnlyFS() bool { return true }

func (s *ProjectGitLogSkill) Execute(_ context.Context, q string) (string, error) {
	root := extractProjectPath(q, "project git log")
	repoRoot, err := resolveRepoRoot(root)
	if err != nil {
		return "", err
	}
	n := 10
	entries := gitLog(repoRoot, n)
	if len(entries) == 0 {
		return "no commits yet (empty reflog)", nil
	}
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%s  %s  <%s>  %s\n", e.ShortSHA, e.Author, e.Email, e.Summary)
		if e.Timestamp != "" {
			fmt.Fprintf(&b, "    ts: %s\n", e.Timestamp)
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// ---- project_git_branches ----

// ProjectGitBranchesSkill lists local + remote branches by scanning
// .git/refs/heads and .git/packed-refs.
type ProjectGitBranchesSkill struct{ *kyoci.BaseSkill }

func NewProjectGitBranchesSkill() *ProjectGitBranchesSkill {
	return &ProjectGitBranchesSkill{BaseSkill: kyoci.NewBaseSkill(
		"project_git_branches",
		"List local + remote branches by reading .git/refs and .git/packed-refs",
		[]string{"project git branches", "list branches", "git branches"},
	)}
}

func (s *ProjectGitBranchesSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "project git branches") || strings.Contains(q, "list branches") ||
		strings.Contains(q, "git branches")
}

func (s *ProjectGitBranchesSkill) IsReadOnlyFS() bool { return true }

func (s *ProjectGitBranchesSkill) Execute(_ context.Context, q string) (string, error) {
	root := extractProjectPath(q, "project git branches")
	repoRoot, err := resolveRepoRoot(root)
	if err != nil {
		return "", err
	}
	branches := gitListBranches(repoRoot)
	if len(branches) == 0 {
		return "no branches (or not a git repo)", nil
	}
	sort.Slice(branches, func(i, j int) bool {
		// current first, then local, then remote, alphabetical
		if branches[i].IsHead != branches[j].IsHead {
			return branches[i].IsHead
		}
		if branches[i].Remote != branches[j].Remote {
			return !branches[i].Remote
		}
		return branches[i].Name < branches[j].Name
	})
	var b strings.Builder
	for _, br := range branches {
		marker := "  "
		if br.IsHead {
			marker = "* "
		}
		scope := "local"
		if br.Remote {
			scope = "remote"
		}
		fmt.Fprintf(&b, "%s%-50s  %s  %s\n", marker, br.Name, scope, br.Commit)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// ---- project_ignore_check ----

// ProjectIgnoreCheckSkill tests whether a path is gitignored.
type ProjectIgnoreCheckSkill struct{ *kyoci.BaseSkill }

func NewProjectIgnoreCheckSkill() *ProjectIgnoreCheckSkill {
	return &ProjectIgnoreCheckSkill{BaseSkill: kyoci.NewBaseSkill(
		"project_ignore_check",
		"Test whether a path is gitignored",
		[]string{"project ignore check", "gitignore check", "is gitignored"},
	)}
}

func (s *ProjectIgnoreCheckSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "project ignore check") || strings.Contains(q, "gitignore check") ||
		strings.Contains(q, "is gitignored") || strings.Contains(q, "is ignored")
}

func (s *ProjectIgnoreCheckSkill) IsReadOnlyFS() bool { return true }

func (s *ProjectIgnoreCheckSkill) Execute(_ context.Context, q string) (string, error) {
	// Operand format: "project ignore check [repo-path]: <file-to-check>" OR
	// "project ignore check: <file-to-check>" (uses CWD as repo).
	// Strip the verb, then split the remainder at the FIRST colon — the part
	// before (if non-empty) is an optional repo path; the part after is the
	// file to check.
	low := strings.ToLower(q)
	verb := "project ignore check"
	idx := strings.Index(low, verb)
	if idx < 0 {
		return "", fmt.Errorf("not an ignore_check query")
	}
	rest := strings.TrimSpace(q[idx+len(verb):])
	rest = strings.TrimSpace(rest)

	repoPath := "."
	target := rest
	if colon := strings.Index(rest, ":"); colon >= 0 {
		before := strings.TrimSpace(rest[:colon])
		after := strings.TrimSpace(rest[colon+1:])
		if before != "" {
			repoPath = before
		}
		target = after
	}
	target = quoteStripped(target)
	if target == "" {
		return "", fmt.Errorf("usage: project ignore check [repo-path]: <file-to-check>")
	}

	repoRoot, err := resolveRepoRoot(repoPath)
	if err != nil {
		return "", err
	}
	ignore := loadIgnore(repoRoot)
	if ignore == nil {
		return fmt.Sprintf("no .gitignore in %s — file is NOT ignored", repoRoot), nil
	}
	target = strings.TrimPrefix(target, "./")
	isDir := false
	if info, err := os.Stat(filepath.Join(repoRoot, target)); err == nil && info.IsDir() {
		isDir = true
	}
	matched := ignore.Match(target, isDir)
	if matched {
		return fmt.Sprintf("IGNORED: %s", target), nil
	}
	return fmt.Sprintf("not ignored: %s", target), nil
}

// ---- project_env_check ----

// ProjectEnvCheckSkill detects env vars referenced in code (os.Getenv,
// process.env) and reports which are missing from .env / .env.example.
type ProjectEnvCheckSkill struct{ *kyoci.BaseSkill }

func NewProjectEnvCheckSkill() *ProjectEnvCheckSkill {
	return &ProjectEnvCheckSkill{BaseSkill: kyoci.NewBaseSkill(
		"project_env_check",
		"Detect env vars referenced in code; report which are missing from .env.example",
		[]string{"project env check", "check env vars", "missing env vars"},
	)}
}

func (s *ProjectEnvCheckSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "project env check") || strings.Contains(q, "check env vars") ||
		strings.Contains(q, "missing env vars") || strings.Contains(q, "env vars used")
}

func (s *ProjectEnvCheckSkill) IsReadOnlyFS() bool { return true }

var (
	goGetenvRe   = regexp.MustCompile(`os\.Getenv\(\s*"([A-Z_][A-Z0-9_]*)"\s*\)`)
	jsGetenvRe   = regexp.MustCompile(`process\.env\.([A-Z_][A-Z0-9_]*)`)
	pyGetenvRe   = regexp.MustCompile(`os\.getenv\(\s*['"]([A-Z_][A-Z0-9_]*)['"]\s*\)`)
	envDeclRe    = regexp.MustCompile(`^\s*([A-Z_][A-Z0-9_]*)\s*=`)
)

func (s *ProjectEnvCheckSkill) Execute(_ context.Context, q string) (string, error) {
	root := extractProjectPath(q, "project env check")
	repoRoot, err := resolveRepoRoot(root)
	if err != nil {
		return "", err
	}
	used := map[string][]string{} // var → file paths
	_ = walkProjectTree(repoRoot, 0, func(abs, rel string, d os.DirEntry) error {
		ext := strings.ToLower(filepath.Ext(rel))
		info, err := d.Info()
		if err != nil || info.Size() > 1024*1024 {
			return nil
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return nil
		}
		text := string(data)
		var matches [][]string
		switch ext {
		case ".go":
			matches = goGetenvRe.FindAllStringSubmatch(text, -1)
		case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs":
			matches = jsGetenvRe.FindAllStringSubmatch(text, -1)
		case ".py":
			matches = pyGetenvRe.FindAllStringSubmatch(text, -1)
		default:
			return nil
		}
		for _, m := range matches {
			used[m[1]] = append(used[m[1]], rel)
		}
		return nil
	})
	if len(used) == 0 {
		return "no os.Getenv / process.env references found", nil
	}
	// Load .env.example (or .env) to find declared vars.
	declared := map[string]bool{}
	for _, name := range []string{".env.example", ".env"} {
		data, err := os.ReadFile(filepath.Join(repoRoot, name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if m := envDeclRe.FindStringSubmatch(line); m != nil {
				declared[m[1]] = true
			}
		}
		break
	}
	var missing []string
	for v := range used {
		if !declared[v] {
			missing = append(missing, v)
		}
	}
	sort.Strings(missing)
	var b strings.Builder
	keys := make([]string, 0, len(used))
	for k := range used {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(&b, "Referenced env vars (%d):\n", len(keys))
	for _, k := range keys {
		files := used[k]
		if len(files) > 3 {
			files = append(files[:3], "...")
		}
		fmt.Fprintf(&b, "  %-30s used in: %s\n", k, strings.Join(files, ", "))
	}
	if len(declared) == 0 {
		fmt.Fprintf(&b, "\nNo .env / .env.example found — cannot check missing.\n")
	} else if len(missing) > 0 {
		fmt.Fprintf(&b, "\nMissing from .env.example (%d):\n", len(missing))
		for _, v := range missing {
			fmt.Fprintf(&b, "  %s\n", v)
		}
	} else {
		fmt.Fprintf(&b, "\nAll referenced vars are declared in .env.example.\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// ---- project_explore (capstone) ----

// ProjectExploreSkill is the composite — runs the other 11 skills and formats
// a single Markdown report. For very large repos it could dispatch the Explore
// sub-agent, but the inline path keeps this self-contained.
type ProjectExploreSkill struct{ *kyoci.BaseSkill }

func NewProjectExploreSkill() *ProjectExploreSkill {
	return &ProjectExploreSkill{BaseSkill: kyoci.NewBaseSkill(
		"project_explore",
		"One-shot project summary: status, structure, languages, deps, entry points, tests, TODOs",
		[]string{"project explore", "explore project", "project overview"},
	)}
}

func (s *ProjectExploreSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "project explore") || strings.Contains(q, "explore project") ||
		strings.Contains(q, "project overview")
}

func (s *ProjectExploreSkill) IsReadOnlyFS() bool { return true }

func (s *ProjectExploreSkill) Execute(ctx context.Context, q string) (string, error) {
	root := extractProjectPath(q, "project explore")
	repoRoot, err := resolveRepoRoot(root)
	if err != nil {
		return "", err
	}
	// Re-query each sub-skill so we get their formatted output without
	// duplicating the logic here.
	qbase := fmt.Sprintf("project %%s: %s", repoRoot)
	var sections []string
	add := func(title, body string) {
		sections = append(sections, fmt.Sprintf("## %s\n\n%s", title, body))
	}

	if out, err := NewProjectStatusSkill().Execute(ctx, fmt.Sprintf(qbase, "status")); err == nil && out != "" {
		add("Status", out)
	}
	if out, err := NewProjectLanguagesSkill().Execute(ctx, fmt.Sprintf(qbase, "languages")); err == nil && out != "" {
		add("Languages", out)
	}
	if out, err := NewProjectDepsSkill().Execute(ctx, fmt.Sprintf(qbase, "deps")); err == nil && out != "" {
		add("Dependencies", out)
	}
	if out, err := NewProjectEntryPointsSkill().Execute(ctx, fmt.Sprintf(qbase, "entry points")); err == nil && out != "" {
		add("Entry Points", out)
	}
	if out, err := NewProjectTestMapSkill().Execute(ctx, fmt.Sprintf(qbase, "test map")); err == nil && out != "" {
		add("Tests", out)
	}
	if out, err := NewProjectTODOScanSkill().Execute(ctx, fmt.Sprintf(qbase, "todo")); err == nil && out != "" {
		add("TODOs", out)
	}
	if out, err := NewProjectGitLogSkill().Execute(ctx, fmt.Sprintf(qbase, "git log")); err == nil && out != "" {
		add("Recent Commits", out)
	}
	if out, err := NewProjectGitBranchesSkill().Execute(ctx, fmt.Sprintf(qbase, "git branches")); err == nil && out != "" {
		add("Branches", out)
	}

	if len(sections) == 0 {
		return fmt.Sprintf("nothing to report for %s", repoRoot), nil
	}
	return strings.Join(sections, "\n\n"), nil
}

// compile-time assertion that all 12 project_* skills implement ReadOnlyFSSkill.
var (
	_ kyoci.ReadOnlyFSSkill = (*ProjectStatusSkill)(nil)
	_ kyoci.ReadOnlyFSSkill = (*ProjectStructureSkill)(nil)
	_ kyoci.ReadOnlyFSSkill = (*ProjectLanguagesSkill)(nil)
	_ kyoci.ReadOnlyFSSkill = (*ProjectDepsSkill)(nil)
	_ kyoci.ReadOnlyFSSkill = (*ProjectEntryPointsSkill)(nil)
	_ kyoci.ReadOnlyFSSkill = (*ProjectTestMapSkill)(nil)
	_ kyoci.ReadOnlyFSSkill = (*ProjectTODOScanSkill)(nil)
	_ kyoci.ReadOnlyFSSkill = (*ProjectGitLogSkill)(nil)
	_ kyoci.ReadOnlyFSSkill = (*ProjectGitBranchesSkill)(nil)
	_ kyoci.ReadOnlyFSSkill = (*ProjectIgnoreCheckSkill)(nil)
	_ kyoci.ReadOnlyFSSkill = (*ProjectEnvCheckSkill)(nil)
	_ kyoci.ReadOnlyFSSkill = (*ProjectExploreSkill)(nil)
)

// strconv import is for a future size formatting helper; keep so gofmt is happy
// when nothing references it yet.
var _ = strconv.Itoa
var _ = fs.SkipDir // ensure fs import is used
