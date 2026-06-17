package promptskill

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Registry holds all loaded skills, indexed by name for O(1) lookup.
// It is safe for concurrent read use after Load completes.
type Registry struct {
	skills []PromptSkill
	byName map[string]int // name → index into skills
}

// List returns all loaded skills. The returned slice is a copy; callers may
// sort or mutate it freely.
func (r *Registry) List() []PromptSkill {
	out := make([]PromptSkill, len(r.skills))
	copy(out, r.skills)
	return out
}

// Get fetches a skill by name. Returns (skill, true) on hit.
func (r *Registry) Get(name string) (PromptSkill, bool) {
	if r == nil || r.byName == nil {
		return PromptSkill{}, false
	}
	idx, ok := r.byName[name]
	if !ok {
		return PromptSkill{}, false
	}
	return r.skills[idx], true
}

// Len returns the number of loaded skills.
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.skills)
}

// emptyRegistry builds a valid but empty Registry.
func emptyRegistry() *Registry {
	return &Registry{byName: map[string]int{}}
}

// Load walks dir recursively, parsing every *.md file with YAML frontmatter
// into a PromptSkill. Files missing required fields (name) are skipped with a
// warning. A missing dir yields an empty registry and no error so the agent
// can boot without skills installed.
func Load(dir string) (*Registry, error) {
	return LoadWithLogger(dir, slog.Default())
}

// LoadWithLogger is like Load but accepts a logger for skip warnings.
func LoadWithLogger(dir string, log *slog.Logger) (*Registry, error) {
	reg := emptyRegistry()

	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warn("prompt skills dir not found; loading empty registry", "dir", dir)
			return reg, nil
		}
		return nil, fmt.Errorf("stat skill dir %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skill path %q is not a directory", dir)
	}

	var paths []string
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk skill dir %q: %w", dir, err)
	}
	sort.Strings(paths) // deterministic order

	loaded := 0
	for _, p := range paths {
		skill, perr := parseSkillFile(p)
		if perr != nil {
			log.Warn("skipping invalid skill file", "path", p, "error", perr)
			continue
		}
		if _, dup := reg.byName[skill.Name]; dup {
			log.Warn("duplicate skill name; skipping", "name", skill.Name, "path", p)
			continue
		}
		reg.byName[skill.Name] = len(reg.skills)
		reg.skills = append(reg.skills, skill)
		loaded++
	}
	log.Info("prompt skills loaded", "dir", dir, "count", loaded, "skipped", len(paths)-loaded)
	return reg, nil
}

// parseSkillFile reads one .md file and splits it into frontmatter + body.
// Returns an error if the file has no frontmatter or is missing the required
// `name` field.
func parseSkillFile(path string) (PromptSkill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PromptSkill{}, fmt.Errorf("read: %w", err)
	}

	body, err := splitFrontmatter(string(data))
	if err != nil {
		return PromptSkill{}, err
	}

	var s PromptSkill
	if err := yaml.Unmarshal([]byte(body.front), &s); err != nil {
		return PromptSkill{}, fmt.Errorf("parse frontmatter: %w", err)
	}
	if strings.TrimSpace(s.Name) == "" {
		return PromptSkill{}, fmt.Errorf("missing required field: name")
	}
	if s.Priority == "" {
		s.Priority = "normal"
	}
	s.Body = strings.TrimSpace(body.rest)
	return s, nil
}

// frontmatterParts holds the result of splitting a markdown file.
type frontmatterParts struct {
	front string // YAML between the first two --- delimiters
	rest  string // everything after the closing delimiter
}

// splitFrontmatter separates leading `---\n...\n---` YAML frontmatter from the
// markdown body. Returns an error if no opening delimiter is present.
func splitFrontmatter(content string) (frontmatterParts, error) {
	// Normalize leading whitespace/newlines.
	trimmed := strings.TrimLeft(content, "\r\n")
	if !strings.HasPrefix(trimmed, "---") {
		return frontmatterParts{}, fmt.Errorf("missing frontmatter opening delimiter")
	}
	// Drop the opening delimiter and its trailing newline.
	afterOpen := trimmed[len("---"):]
	afterOpen = strings.TrimLeft(afterOpen, "\r\n")

	// Find the closing delimiter on its own line.
	closeIdx := findDelimiterLine(afterOpen, "---")
	if closeIdx < 0 {
		return frontmatterParts{}, fmt.Errorf("missing frontmatter closing delimiter")
	}
	front := afterOpen[:closeIdx]
	rest := afterOpen[closeIdx:]
	// Strip the closing delimiter itself.
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[nl+1:]
	} else {
		rest = ""
	}
	return frontmatterParts{front: front, rest: rest}, nil
}

// findDelimiterLine returns the byte index of the first line that is exactly
// `delim` (ignoring trailing whitespace), or -1 if none.
func findDelimiterLine(s, delim string) int {
	for start := 0; start <= len(s); {
		nl := strings.IndexByte(s[start:], '\n')
		var line string
		end := len(s)
		if nl >= 0 {
			end = start + nl
		}
		line = s[start:end]
		if strings.TrimRight(line, "\r\t ") == delim {
			return start
		}
		if nl < 0 {
			break
		}
		start = end + 1
	}
	return -1
}
