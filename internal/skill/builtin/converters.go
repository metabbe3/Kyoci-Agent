package builtin

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Markdown-table and delimiter conversion skills.
//
// These six skills round-trip tabular data between CSV/TSV/JSON and the markdown
// table format, plus a flat list → markdown bullet/numbered converter. All are
// pure-Go: no LLM, no network. Stdlib only (encoding/csv, encoding/json, bytes,
// strings, sort, fmt).
//
// Conventions (shared with datafmt.go):
//   - Match() lowercases the query and matches on the from→to phrase.
//   - Execute() pulls the operand with extractPayload() (splits at the first
//     colon). Tabular input often contains commas and pipes but rarely a
//     leading colon, so extractPayload is safe.
// =====================================================================================

// ---- csv to markdown table ----

// CSVToMarkdownTableSkill converts CSV text into a GitHub-flavored markdown table.
type CSVToMarkdownTableSkill struct{ *kyoci.BaseSkill }

// NewCSVToMarkdownTableSkill constructs the csv_to_markdown_table skill.
func NewCSVToMarkdownTableSkill() *CSVToMarkdownTableSkill {
	return &CSVToMarkdownTableSkill{BaseSkill: kyoci.NewBaseSkill(
		"csv_to_markdown_table", "Convert CSV to a markdown table",
		[]string{"csv to markdown", "csv to markdown table", "csv → markdown"},
	)}
}

// Match recognises "csv to markdown" / "csv to markdown table".
func (s *CSVToMarkdownTableSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "csv to markdown") || strings.Contains(q, "csv → markdown") ||
		strings.Contains(q, "convert csv to markdown")
}

// Execute parses the CSV operand and emits a header row, a separator row of
// "---", then one row per body record. Pipes and newlines inside CSV fields
// are escaped per markdown conventions.
func (s *CSVToMarkdownTableSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	r := csv.NewReader(strings.NewReader(in))
	r.FieldsPerRecord = -1 // tolerate ragged rows
	rows, err := r.ReadAll()
	if err != nil {
		return "", fmt.Errorf("invalid CSV: %w", err)
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("no rows in CSV input")
	}
	header := rows[0]
	width := len(header)
	for _, row := range rows[1:] {
		if len(row) > width {
			width = len(row)
		}
	}
	// Pad header to full width so every row has a consistent column count.
	for len(header) < width {
		header = append(header, "")
	}

	var b strings.Builder
	writeMarkdownRow(&b, header)
	sep := make([]string, width)
	for i := range sep {
		sep[i] = "---"
	}
	writeMarkdownRow(&b, sep)
	for _, row := range rows[1:] {
		for len(row) < width {
			row = append(row, "")
		}
		writeMarkdownRow(&b, row)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// ---- markdown table to csv ----

// MarkdownTableToCSVSkill parses a markdown table back into CSV.
type MarkdownTableToCSVSkill struct{ *kyoci.BaseSkill }

// NewMarkdownTableToCSVSkill constructs the markdown_table_to_csv skill.
func NewMarkdownTableToCSVSkill() *MarkdownTableToCSVSkill {
	return &MarkdownTableToCSVSkill{BaseSkill: kyoci.NewBaseSkill(
		"markdown_table_to_csv", "Convert a markdown table to CSV",
		[]string{"markdown table to csv", "markdown to csv", "md to csv"},
	)}
}

// Match recognises "markdown table to csv" / "markdown to csv".
func (s *MarkdownTableToCSVSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "markdown table to csv") || strings.Contains(q, "markdown to csv") ||
		strings.Contains(q, "md to csv") || strings.Contains(q, "convert markdown to csv")
}

// Execute filters pipe-delimited rows, drops the separator row, and writes the
// remaining cells as CSV (quoting any cell containing commas, quotes, or
// newlines).
func (s *MarkdownTableToCSVSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	var rows [][]string
	for _, raw := range strings.Split(in, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if !strings.Contains(line, "|") {
			continue
		}
		// Trim outer pipes and surrounding whitespace on the line itself.
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(line), "|"), "|"))
		if line == "" {
			continue
		}
		cells := strings.Split(line, "|")
		trimmed := make([]string, len(cells))
		for i, c := range cells {
			trimmed[i] = strings.TrimSpace(c)
		}
		// Skip the GFM separator row (every cell is only dashes/colons).
		if isSeparatorRow(trimmed) {
			continue
		}
		rows = append(rows, trimmed)
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("no markdown table rows found")
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return "", fmt.Errorf("CSV encode failed: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", fmt.Errorf("CSV flush failed: %w", err)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

// isSeparatorRow reports whether every cell is the GFM delimiter (only dashes
// and colons, e.g. "---", ":--:", "---:"). Used to skip the header/body divider.
func isSeparatorRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		c = strings.TrimSpace(c)
		if c == "" {
			return false
		}
		for _, r := range c {
			if r != '-' && r != ':' {
				return false
			}
		}
	}
	return true
}

// ---- json to markdown table ----

// JSONToMarkdownTableSkill converts a JSON array of flat objects to a markdown table.
type JSONToMarkdownTableSkill struct{ *kyoci.BaseSkill }

// NewJSONToMarkdownTableSkill constructs the json_to_markdown_table skill.
func NewJSONToMarkdownTableSkill() *JSONToMarkdownTableSkill {
	return &JSONToMarkdownTableSkill{BaseSkill: kyoci.NewBaseSkill(
		"json_to_markdown_table", "Convert an array of flat JSON objects to a markdown table",
		[]string{"json to markdown", "json to markdown table", "json → markdown"},
	)}
}

// Match recognises "json to markdown table" / "json to markdown".
func (s *JSONToMarkdownTableSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "json to markdown") || strings.Contains(q, "json → markdown") ||
		strings.Contains(q, "convert json to markdown")
}

// Execute unmarshals the JSON array, collects the union of keys (sorted for
// stable column order), and renders a markdown table. Nested values are
// serialized as compact JSON strings.
func (s *JSONToMarkdownTableSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	var arr []map[string]any
	if err := json.Unmarshal([]byte(in), &arr); err != nil {
		// Allow []map[string]string (rare but valid) by widening.
		var arr2 []map[string]string
		if err2 := json.Unmarshal([]byte(in), &arr2); err2 != nil {
			return "", fmt.Errorf("invalid JSON (expected array of objects): %w", err)
		}
		arr = make([]map[string]any, len(arr2))
		for i, m := range arr2 {
			arr[i] = make(map[string]any, len(m))
			for k, v := range m {
				arr[i][k] = v
			}
		}
	}
	if len(arr) == 0 {
		return "", fmt.Errorf("empty array, no header to emit")
	}

	colSet := map[string]bool{}
	for _, row := range arr {
		for k := range row {
			colSet[k] = true
		}
	}
	cols := make([]string, 0, len(colSet))
	for k := range colSet {
		cols = append(cols, k)
	}
	sort.Strings(cols)

	var b strings.Builder
	writeMarkdownRow(&b, cols)
	sep := make([]string, len(cols))
	for i := range sep {
		sep[i] = "---"
	}
	writeMarkdownRow(&b, sep)
	for _, row := range arr {
		rec := make([]string, len(cols))
		for i, c := range cols {
			rec[i] = cellValue(row[c])
		}
		writeMarkdownRow(&b, rec)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// cellValue renders a JSON value as a markdown cell string: scalars become
// their natural text form, nil → "", nested objects/arrays → compact JSON.
func cellValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		// JSON numbers unmarshal as float64. Print without trailing zeros
		// when the value is integral.
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	default:
		js, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprintf("%v", x)
		}
		return string(js)
	}
}

// ---- tsv to csv ----

// TSVToCSVSkill converts TSV text to CSV.
type TSVToCSVSkill struct{ *kyoci.BaseSkill }

// NewTSVToCSVSkill constructs the tsv_to_csv skill.
func NewTSVToCSVSkill() *TSVToCSVSkill {
	return &TSVToCSVSkill{BaseSkill: kyoci.NewBaseSkill(
		"tsv_to_csv", "Convert TSV to CSV (quoting fields with commas or quotes)",
		[]string{"tsv to csv", "tsv → csv", "convert tsv to csv"},
	)}
}

// Match recognises "tsv to csv".
func (s *TSVToCSVSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "tsv to csv") || strings.Contains(q, "tsv → csv") ||
		strings.Contains(q, "convert tsv to csv")
}

// Execute reads the TSV with Comma='\t' and writes it back as standard CSV.
// encoding/csv handles quoting automatically for fields that need it.
func (s *TSVToCSVSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	r := csv.NewReader(strings.NewReader(in))
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	rows, err := r.ReadAll()
	if err != nil {
		return "", fmt.Errorf("invalid TSV: %w", err)
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("no rows in TSV input")
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return "", fmt.Errorf("CSV encode failed: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", fmt.Errorf("CSV flush failed: %w", err)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

// ---- csv to tsv ----

// CSVToTSVSkill converts CSV text to TSV.
type CSVToTSVSkill struct{ *kyoci.BaseSkill }

// NewCSVToTSVSkill constructs the csv_to_tsv skill.
func NewCSVToTSVSkill() *CSVToTSVSkill {
	return &CSVToTSVSkill{BaseSkill: kyoci.NewBaseSkill(
		"csv_to_tsv", "Convert CSV to TSV",
		[]string{"csv to tsv", "csv → tsv", "convert csv to tsv"},
	)}
}

// Match recognises "csv to tsv".
func (s *CSVToTSVSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "csv to tsv") || strings.Contains(q, "csv → tsv") ||
		strings.Contains(q, "convert csv to tsv")
}

// Execute reads standard CSV and writes it back with tabs as the field
// delimiter. Embedded tabs/newlines are escaped (\t → "\\t", \n → "\\n") so
// the TSV stays row-aligned.
func (s *CSVToTSVSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	r := csv.NewReader(strings.NewReader(in))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	rows, err := r.ReadAll()
	if err != nil {
		return "", fmt.Errorf("invalid CSV: %w", err)
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("no rows in CSV input")
	}
	var b strings.Builder
	for i, row := range rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		escaped := make([]string, len(row))
		for j, cell := range row {
			escaped[j] = strings.NewReplacer("\t", "\\t", "\n", "\\n", "\r", "\\r").Replace(cell)
		}
		b.WriteString(strings.Join(escaped, "\t"))
	}
	return b.String(), nil
}

// ---- list to markdown ----

// ListToMarkdownSkill converts a newline-separated list to a markdown bullet
// or numbered list.
type ListToMarkdownSkill struct{ *kyoci.BaseSkill }

// NewListToMarkdownSkill constructs the list_to_markdown skill.
func NewListToMarkdownSkill() *ListToMarkdownSkill {
	return &ListToMarkdownSkill{BaseSkill: kyoci.NewBaseSkill(
		"list_to_markdown", "Convert a list (one item per line) to a markdown bullet or numbered list",
		[]string{"list to markdown", "to markdown list", "convert list to markdown"},
	)}
}

// Match recognises "list to markdown" / "to markdown list".
func (s *ListToMarkdownSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "list to markdown") || strings.Contains(q, "to markdown list") ||
		strings.Contains(q, "convert list to markdown")
}

// Execute renders each non-empty line as a list item. A "numbered" keyword
// anywhere in the query switches from bullets ("- ") to ordered items
// ("1. ", "2. ", ...). Existing markdown markers ("- ", "*", "1. ") on items
// are stripped to avoid double-prefixing.
func (s *ListToMarkdownSkill) Execute(_ context.Context, q string) (string, error) {
	low := strings.ToLower(q)
	numbered := strings.Contains(low, "numbered") || strings.Contains(low, "ordered") ||
		strings.Contains(low, "numbered list") || strings.Contains(low, "ordered list")
	in := extractPayload(q)
	if in == "" {
		return "", fmt.Errorf("no list items found")
	}
	var b strings.Builder
	n := 1
	for _, raw := range strings.Split(in, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		line = stripMarkdownBullet(line)
		if numbered {
			fmt.Fprintf(&b, "%d. %s\n", n, line)
			n++
		} else {
			fmt.Fprintf(&b, "- %s\n", line)
		}
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("no list items found")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// stripMarkdownBullet removes a leading markdown list marker ("-", "*", "+",
// or "N." with optional trailing parenthesis) from a single line.
func stripMarkdownBullet(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	// Unordered markers: -, *, + followed by a space.
	if len(trimmed) >= 2 && (trimmed[0] == '-' || trimmed[0] == '*' || trimmed[0] == '+') && trimmed[1] == ' ' {
		return strings.TrimSpace(trimmed[2:])
	}
	// Ordered markers: "1." or "1)" followed by a space.
	i := 0
	for i < len(trimmed) && trimmed[i] >= '0' && trimmed[i] <= '9' {
		i++
	}
	if i > 0 && i+1 < len(trimmed) && (trimmed[i] == '.' || trimmed[i] == ')') && trimmed[i+1] == ' ' {
		return strings.TrimSpace(trimmed[i+2:])
	}
	return line
}

// =====================================================================================
// Shared markdown table helpers.
// =====================================================================================

// writeMarkdownRow writes one pipe-delimited row to b in GFM table form:
// "| a | b |". Cells are sanitized via escapeMarkdownCell.
func writeMarkdownRow(b *strings.Builder, cells []string) {
	b.WriteString("|")
	for _, c := range cells {
		b.WriteString(" ")
		b.WriteString(escapeMarkdownCell(c))
		b.WriteString(" |")
	}
	b.WriteString("\n")
}

// escapeMarkdownCell sanitizes a cell value for inline table rendering: pipes
// become the HTML entity, newlines become <br>. Backslashes are not escaped,
// matching the behavior of most GFM renderers.
func escapeMarkdownCell(c string) string {
	if c == "" {
		return ""
	}
	c = strings.ReplaceAll(c, "|", "\\|")
	c = strings.ReplaceAll(c, "\n", "<br>")
	return c
}
