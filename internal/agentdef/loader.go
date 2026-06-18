package agentdef

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

// LoadAgents walks dir recursively, parsing every *.md file with YAML
// frontmatter into an AgentDef. Files missing required fields (name, body)
// are skipped with a warning. A missing dir yields an empty slice and no
// error so the orchestrator can boot without agents installed (it will fall
// back to the legacy generalist constant in the classifier).
//
// Returned slice is sorted by SourcePath for deterministic load-order
// tiebreaks in BestMatch.
func LoadAgents(dir string) ([]AgentDef, error) {
	return LoadAgentsWithLogger(dir, slog.Default())
}

// LoadAgentsWithLogger is like LoadAgents but accepts a logger for skip
// warnings. Used by the orchestrator to fan warnings into its own log stream.
func LoadAgentsWithLogger(dir string, log *slog.Logger) ([]AgentDef, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warn("agents dir not found; loading empty agent set", "dir", dir)
			return nil, nil
		}
		return nil, fmt.Errorf("stat agents dir %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("agents path %q is not a directory", dir)
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
		return nil, fmt.Errorf("walk agents dir %q: %w", dir, err)
	}
	sort.Strings(paths) // deterministic load order

	var defs []AgentDef
	seen := map[string]bool{}
	loaded := 0
	for _, p := range paths {
		def, perr := parseAgentFile(p)
		if perr != nil {
			log.Warn("skipping invalid agent file", "path", p, "error", perr)
			continue
		}
		if seen[def.Name] {
			log.Warn("duplicate agent name; skipping", "name", def.Name, "path", p)
			continue
		}
		seen[def.Name] = true
		defs = append(defs, def)
		loaded++
	}
	log.Info("agents loaded", "dir", dir, "count", loaded, "skipped", len(paths)-loaded)
	return defs, nil
}

// parseAgentFile reads one .md file and produces a fully composed AgentDef.
// Returns an error if the file has no frontmatter, missing required `name`
// or empty body, or frontmatter fails to parse.
func parseAgentFile(path string) (AgentDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentDef{}, fmt.Errorf("read: %w", err)
	}

	parts, err := splitFrontmatter(string(data))
	if err != nil {
		return AgentDef{}, err
	}

	var def AgentDef
	if err := yaml.Unmarshal([]byte(parts.front), &def); err != nil {
		return AgentDef{}, fmt.Errorf("parse frontmatter: %w", err)
	}
	if strings.TrimSpace(def.Name) == "" {
		return AgentDef{}, fmt.Errorf("missing required field: name")
	}
	def.Body = strings.TrimSpace(parts.rest)
	if def.Body == "" {
		return AgentDef{}, fmt.Errorf("empty body — agent must have a soul (system prompt)")
	}

	// Apply runtime substitutions + shared closing blocks. The composed
	// SystemPrompt is what the role registry installs.
	def.SystemPrompt = Compose(SubstitutePlatformTokens(def.Body))

	abs, _ := filepath.Abs(path)
	def.SourcePath = abs
	return def, nil
}

// frontmatterParts holds the result of splitting a markdown file.
type frontmatterParts struct {
	front string // YAML between the first two --- delimiters
	rest  string // everything after the closing delimiter
}

// splitFrontmatter separates leading `---\n...\n---` YAML frontmatter from the
// markdown body. Returns an error if no opening delimiter is present. Mirrors
// internal/promptskill/loader.go:splitFrontmatter.
func splitFrontmatter(content string) (frontmatterParts, error) {
	trimmed := strings.TrimLeft(content, "\r\n")
	if !strings.HasPrefix(trimmed, "---") {
		return frontmatterParts{}, fmt.Errorf("missing frontmatter opening delimiter")
	}
	afterOpen := trimmed[len("---"):]
	afterOpen = strings.TrimLeft(afterOpen, "\r\n")

	closeIdx := findDelimiterLine(afterOpen, "---")
	if closeIdx < 0 {
		return frontmatterParts{}, fmt.Errorf("missing frontmatter closing delimiter")
	}
	front := afterOpen[:closeIdx]
	rest := afterOpen[closeIdx:]
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[nl+1:]
	} else {
		rest = ""
	}
	return frontmatterParts{front: front, rest: rest}, nil
}

// findDelimiterLine returns the byte index of the first line that is exactly
// `delim` (ignoring trailing whitespace), or -1 if none. Mirrors
// internal/promptskill/loader.go.
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

// FindByName returns the AgentDef with the given name from defs, or false.
// Used by the role registry to look up a def when constructing an agent.
func FindByName(defs []AgentDef, name string) (AgentDef, bool) {
	for _, d := range defs {
		if d.Name == name {
			return d, true
		}
	}
	return AgentDef{}, false
}
