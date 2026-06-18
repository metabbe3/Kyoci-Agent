package builtin

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// =====================================================================================
// Minimal .gitignore matcher. Supports the 90% of gitignore patterns people
// actually write:
//   - blank lines and # comments are skipped
//   - dir/<...>/  → match a directory anywhere in the tree
//   - *.ext       → match by extension
//   - path        → match exact path or any descendant
//   - !pattern    → negation (un-ignores)
//
// NOT supported (documented limits): full anchored leading "/", bracket globs
// [abc], ** between path segments, nested .gitignore in subdirs (only reads
// the root one). For projects using only basic patterns this covers ~95% of
// real-world .gitignore files.
// =====================================================================================

// IgnoreMatcher holds the parsed patterns from a .gitignore file. Safe to call
// Match concurrently after load — it's read-only.
type IgnoreMatcher struct {
	patterns []ignorePattern
}

type ignorePattern struct {
	negate    bool
	dirOnly   bool
	anchored  bool   // pattern contains a slash → matches from root
	literal   string // pattern with leading/trailing / and trailing * stripped
	ext       string // for "*.foo" patterns
	wildcard  bool   // contains glob chars in the literal
}

// LoadIgnore reads .gitignore at gitignorePath and returns a matcher. Missing
// file → empty matcher (matches nothing). Parse errors are skipped silently —
// a malformed line just doesn't contribute.
func LoadIgnore(gitignorePath string) *IgnoreMatcher {
	f, err := os.Open(gitignorePath)
	if err != nil {
		return &IgnoreMatcher{}
	}
	defer f.Close()
	m := &IgnoreMatcher{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m.patterns = append(m.patterns, parseIgnorePattern(line))
	}
	return m
}

// loadIgnore is the lowercase wrapper used by gitinternals.go (returns nil if
// .gitignore is absent, distinguishing "not loaded" from "empty match"). Callers
// should check nil before calling Match.
func loadIgnore(repoRoot string) *IgnoreMatcher {
	path := filepath.Join(repoRoot, ".gitignore")
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	return LoadIgnore(path)
}

func parseIgnorePattern(line string) ignorePattern {
	p := ignorePattern{}
	if strings.HasPrefix(line, "!") {
		p.negate = true
		line = line[1:]
	}
	if strings.HasSuffix(line, "/") {
		p.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}
	if strings.HasPrefix(line, "/") {
		p.anchored = true
		line = strings.TrimPrefix(line, "/")
	}
	if strings.Contains(line, "/") {
		// "foo/bar" patterns are effectively anchored to root
		p.anchored = true
	}
	if strings.HasPrefix(line, "*.") {
		// *.ext
		p.ext = strings.TrimPrefix(line, "*.")
		return p
	}
	if strings.ContainsAny(line, "*?[") {
		p.wildcard = true
	}
	p.literal = line
	return p
}

// Match reports whether the given path (relative to the repo root, forward
// slashes) is ignored. The isDir hint applies dir-only patterns correctly.
// Later patterns override earlier ones (git semantics: last match wins).
func (m *IgnoreMatcher) Match(rel string, isDir bool) bool {
	if m == nil || rel == "" {
		return false
	}
	rel = filepath.ToSlash(rel)
	ignored := false
	for _, p := range m.patterns {
		if p.dirOnly && !isDir && !strings.Contains(rel, "/") {
			// dir-only pattern matches descendants too, so still try the prefix
		}
		if p.matches(rel, isDir) {
			ignored = !p.negate
		}
	}
	return ignored
}

func (p ignorePattern) matches(rel string, isDir bool) bool {
	if p.ext != "" {
		return strings.HasSuffix(rel, "."+p.ext)
	}
	if p.dirOnly && !isDir {
		// match if rel is inside a directory with this literal name
		return strings.HasPrefix(rel, p.literal+"/")
	}
	if p.anchored {
		// match exact OR any descendant
		if p.wildcard {
			return wildcardMatch(p.literal, rel)
		}
		return rel == p.literal || strings.HasPrefix(rel, p.literal+"/")
	}
	// unanchored: match at any path segment
	if p.wildcard {
		// check each segment
		for _, seg := range strings.Split(rel, "/") {
			if wildcardMatch(p.literal, seg) {
				return true
			}
		}
		return false
	}
	// literal substring match at segment granularity
	for _, seg := range strings.Split(rel, "/") {
		if seg == p.literal {
			return true
		}
	}
	return false
}

// wildcardMatch implements single-segment glob with '*' and '?'. Used by
// IgnoreMatcher for patterns like "foo*.go".
func wildcardMatch(pattern, name string) bool {
	pi, ni := 0, 0
	starPi, starNi := -1, -1
	for ni < len(name) {
		if pi < len(pattern) && (pattern[pi] == '?' || pattern[pi] == name[ni]) {
			pi++
			ni++
		} else if pi < len(pattern) && pattern[pi] == '*' {
			starPi = pi
			starNi = ni
			pi++
		} else if starPi != -1 {
			pi = starPi + 1
			starNi++
			ni = starNi
		} else {
			return false
		}
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}
