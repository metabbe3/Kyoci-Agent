package dashboard

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// =====================================================================================
// Skill Maker — handlers for creating/deleting prompt-skill markdown files.
// =====================================================================================

// CustomSkillSummary is one row of the skill maker's list.
type CustomSkillSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	SourcePath  string `json:"source_path"`
}

// handleSkillCreate handles POST /api/dashboard/skills/create.
func (s *Server) handleSkillCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Category    string `json:"category"`
		Keywords    string `json:"keywords"` // comma-separated
		Body        string `json:"body"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Name == "" || req.Body == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and body are required"})
		return
	}
	// Sanitize name — slug only.
	name := strings.ToLower(strings.TrimSpace(req.Name))
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "/", "-")
	category := strings.TrimSpace(req.Category)
	if category == "" {
		category = "custom"
	}
	// Build the markdown file.
	keywords := strings.Split(req.Keywords, ",")
	for i, k := range keywords {
		keywords[i] = strings.TrimSpace(k)
	}
	var kwYAML strings.Builder
	kwYAML.WriteString("    keywords:\n")
	for _, k := range keywords {
		if k != "" {
			kwYAML.WriteString(fmt.Sprintf("      - %s\n", k))
		}
	}
	kwYAML.WriteString("    regex: []\n")
	md := fmt.Sprintf("---\nname: %s\ndescription: %s\ncategory: %s\ntriggers:\n%spriority: normal\n---\n\n%s\n",
		name, req.Description, category, kwYAML.String(), req.Body)
	// Write to data/skills/<name>.md.
	skillsDir := filepath.Join("data", "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "mkdir: " + err.Error()})
		return
	}
	path := filepath.Join(skillsDir, name+".md")
	if err := os.WriteFile(path, []byte(md), 0644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write: " + err.Error()})
		return
	}
	s.logger.Info("skill created", "name", name, "path", path)
	writeJSON(w, http.StatusOK, map[string]string{"status": "created", "name": name, "path": path})
}

// handleSkillDelete handles DELETE /api/dashboard/skills/{name}.
func (s *Server) handleSkillDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name query param required"})
		return
	}
	name = strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	// Path traversal defense: only allow alphanumeric + hyphens/underscores.
	name = filepath.Base(name) // strips any directory components
	if strings.ContainsAny(name, "/\\..") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid skill name"})
		return
	}
	path := filepath.Join("data", "skills", name+".md")
	if err := os.Remove(path); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found: " + name})
		return
	}
	s.logger.Info("skill deleted", "name", name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "name": name})
}

// handleSkillListCustom handles GET /api/dashboard/skills/custom.
func (s *Server) handleSkillListCustom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	skillsDir := filepath.Join("data", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"skills": []any{}})
		return
	}
	var out []CustomSkillSummary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		path := filepath.Join(skillsDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		desc, cat := parseSkillFrontmatter(string(data))
		out = append(out, CustomSkillSummary{
			Name:        name,
			Description: desc,
			Category:    cat,
			SourcePath:  path,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, map[string]any{"skills": out})
}

// parseSkillFrontmatter extracts description + category from YAML frontmatter.
func parseSkillFrontmatter(content string) (desc, cat string) {
	lines := strings.Split(content, "\n")
	inFrontmatter := false
	for _, line := range lines {
		if line == "---" {
			if inFrontmatter {
				break
			}
			inFrontmatter = true
			continue
		}
		if inFrontmatter {
			if strings.HasPrefix(line, "description:") {
				desc = strings.TrimPrefix(line, "description:")
				desc = strings.TrimSpace(strings.Trim(desc, "\""))
			}
			if strings.HasPrefix(line, "category:") {
				cat = strings.TrimPrefix(line, "category:")
				cat = strings.TrimSpace(cat)
			}
		}
	}
	return desc, cat
}
