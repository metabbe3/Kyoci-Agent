package builtin

import (
	"context"
	"fmt"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// MarkdownSkill formats a markdown table from comma- or pipe-separated rows.
type MarkdownSkill struct {
	*kyoci.BaseSkill
}

// NewMarkdownSkill creates a new markdown table skill.
func NewMarkdownSkill() *MarkdownSkill {
	return &MarkdownSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"markdown",
			"Format a markdown table from comma- or pipe-separated rows",
			[]string{"markdown", "md table", "format table"},
		),
	}
}

// Match checks if the query is asking for a markdown table.
//
// Defers to the specific markdown skills (outline, toc, strip, link_extract)
// when their tighter patterns would fire. This legacy MarkdownSkill builds
// tables from CSV-like input — it shouldn't shadow the specific skills.
func (s *MarkdownSkill) Match(query string) bool {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	// Defer to specific markdown skills.
	deferPhrases := []string{
		"markdown outline", "outline of markdown", "extract markdown outline",
		"markdown toc", "table of contents", "generate toc",
		"markdown strip", "strip markdown", "markdown to text", "md to text",
		"extract links", "link extract", "urls in markdown", "find urls in",
	}
	for _, p := range deferPhrases {
		if strings.Contains(queryLower, p) {
			return false
		}
	}
	if strings.Contains(queryLower, "markdown") {
		return true
	}
	if strings.Contains(queryLower, "table") && (strings.Contains(queryLower, ",") || strings.Contains(queryLower, "|")) {
		return true
	}
	return false
}

// Execute builds a markdown table from the query.
func (s *MarkdownSkill) Execute(ctx context.Context, query string) (string, error) {
	rows := extractTableRows(query)
	if len(rows) == 0 {
		return "", fmt.Errorf("no tabular rows found in query")
	}

	delim := detectDelimiter(rows)
	parsed := make([][]string, 0, len(rows))
	maxCols := 0
	for _, r := range rows {
		cells := splitCells(r, delim)
		parsed = append(parsed, cells)
		if len(cells) > maxCols {
			maxCols = len(cells)
		}
	}
	if maxCols == 0 {
		return "", fmt.Errorf("no cells parsed from rows")
	}

	// Normalize every row to maxCols.
	for i := range parsed {
		for len(parsed[i]) < maxCols {
			parsed[i] = append(parsed[i], "")
		}
	}

	var b strings.Builder
	// Header row.
	writeRow(&b, parsed[0])
	// Separator row of dashes.
	sep := make([]string, maxCols)
	for i := range sep {
		sep[i] = "---"
	}
	writeRow(&b, sep)
	// Body rows.
	for _, r := range parsed[1:] {
		writeRow(&b, r)
	}
	return b.String(), nil
}

// extractTableRows pulls candidate rows out of the query.
func extractTableRows(query string) []string {
	query = strings.TrimSpace(query)
	// Strip a leading command like "markdown table:" or "markdown".
	lines := strings.Split(query, "\n")
	var rows []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip pure command lines.
		lower := strings.ToLower(line)
		if lower == "markdown" || lower == "markdown table" || lower == "table" {
			continue
		}
		// If a line ends with ":", treat the part before the colon as a label.
		if idx := strings.Index(line, ":"); idx >= 0 {
			rest := strings.TrimSpace(line[idx+1:])
			if rest != "" {
				line = rest
			}
		}
		if line == "" {
			continue
		}
		rows = append(rows, line)
	}
	return rows
}

// detectDelimiter picks the delimiter used across rows.
func detectDelimiter(rows []string) string {
	pipeCount := 0
	commaCount := 0
	for _, r := range rows {
		pipeCount += strings.Count(r, "|")
		commaCount += strings.Count(r, ",")
	}
	if pipeCount >= commaCount && pipeCount > 0 {
		return "|"
	}
	return ","
}

// splitCells splits a row by the delimiter and trims each cell.
func splitCells(row, delim string) []string {
	// Trim outer pipes for pipe-delimited rows.
	if delim == "|" {
		row = strings.TrimSpace(row)
		row = strings.TrimPrefix(row, "|")
		row = strings.TrimSuffix(row, "|")
	}
	parts := strings.Split(row, delim)
	cells := make([]string, 0, len(parts))
	for _, p := range parts {
		cells = append(cells, strings.TrimSpace(p))
	}
	return cells
}

// writeRow writes a row as a markdown table line.
func writeRow(b *strings.Builder, cells []string) {
	b.WriteString("|")
	for _, c := range cells {
		b.WriteString(" ")
		b.WriteString(c)
		b.WriteString(" |")
	}
	b.WriteString("\n")
}
