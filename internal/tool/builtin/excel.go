package builtin

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/metabbe3/Kyoci-Agent/pkg"
)

// ExcelTool reads .xlsx spreadsheets uploaded by the user. Uses xuri/excelize
// (the de-facto Go xlsx library). All file access is sandboxed to UploadDir.
//
// The tool emits results as fenced ```csv blocks so the dashboard's Markdown
// renderer can pick them up and (when applicable) render them as interactive
// tables / charts via the CsvBlock component.
type ExcelTool struct {
	logger *slog.Logger
}

func NewExcelTool() *ExcelTool {
	return &ExcelTool{logger: slog.Default()}
}

func (t *ExcelTool) Name() string { return "excel" }

func (t *ExcelTool) Description() string {
	return "Read and analyze uploaded .xlsx spreadsheets. Operations: sheets (list sheet names), info (dimensions of a sheet), header (first row as column names), sample (first N rows), stats (min/max/mean/sum/count for numeric columns), summarize (combined one-shot summary). Path is the filename from [Attached files]. Output is fenced CSV blocks."
}

func (t *ExcelTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{
			Name:        "operation",
			Type:        "string",
			Description: "Operation: sheets, info, header, sample, stats, summarize",
			Required:    true,
			EnumValues:  []string{"sheets", "info", "header", "sample", "stats", "summarize"},
		},
		{
			Name:        "path",
			Type:        "string",
			Description: "Filename from the [Attached files] list (e.g. 'a1b2c3d4-sales.xlsx')",
			Required:    true,
		},
		{
			Name:        "sheet",
			Type:        "string",
			Description: "Sheet name (optional; defaults to the first sheet)",
			Required:    false,
		},
		{
			Name:        "rows",
			Type:        "integer",
			Description: "Row count for sample/summarize (optional; default 10 for sample, 20 for summarize)",
			Required:    false,
		},
	}
}

func (t *ExcelTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	op, _ := params["operation"].(string)
	path, _ := params["path"].(string)
	sheet, _ := params["sheet"].(string)
	rows := 10
	if n, ok := toInt(params["rows"]); ok && n > 0 {
		rows = n
	}
	if op == "summarize" && rows == 10 {
		rows = 20
	}

	if path == "" {
		return "", fmt.Errorf("path is required (use the exact filename from the [Attached files] list)")
	}

	resolved, err := resolveUpload(path)
	if err != nil {
		return "", err
	}

	f, err := excelize.OpenFile(resolved)
	if err != nil {
		return "", fmt.Errorf("cannot open %s as xlsx: %w (is it really an .xlsx file?)", filepath.Base(resolved), err)
	}
	defer f.Close()

	if sheet == "" {
		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			return "", fmt.Errorf("file has no sheets")
		}
		sheet = sheets[0]
	}

	switch op {
	case "sheets":
		return t.opSheets(f)
	case "info":
		return t.opInfo(f, sheet)
	case "header":
		return t.opHeader(f, sheet)
	case "sample":
		return t.opSample(f, sheet, rows)
	case "stats":
		return t.opStats(f, sheet)
	case "summarize":
		return t.opSummarize(f, sheet, rows)
	default:
		return "", fmt.Errorf("unknown operation %q (want: sheets, info, header, sample, stats, summarize)", op)
	}
}

func (t *ExcelTool) opSheets(f *excelize.File) (string, error) {
	sheets := f.GetSheetList()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d sheet(s):\n", len(sheets)))
	for i, s := range sheets {
		fmt.Fprintf(&b, "%d. %s\n", i+1, s)
	}
	return strings.TrimSpace(b.String()), nil
}

func (t *ExcelTool) opInfo(f *excelize.File, sheet string) (string, error) {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return "", fmt.Errorf("cannot read sheet %q: %w", sheet, err)
	}
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	return fmt.Sprintf("sheet: %s\nrows: %d\ncols: %d", sheet, len(rows), cols), nil
}

func (t *ExcelTool) opHeader(f *excelize.File, sheet string) (string, error) {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("sheet %q is empty", sheet)
	}
	header := rows[0]
	var b strings.Builder
	b.WriteString("```csv\n")
	b.WriteString(strings.Join(header, ","))
	b.WriteString("\n```\n")
	return strings.TrimSpace(b.String()), nil
}

func (t *ExcelTool) opSample(f *excelize.File, sheet string, n int) (string, error) {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("sheet %q is empty", sheet)
	}
	header := rows[0]
	dataRows := rows[1:]
	if n > len(dataRows) {
		n = len(dataRows)
	}
	var b strings.Builder
	b.WriteString("```csv\n")
	b.WriteString(csvRow(header))
	b.WriteString("\n")
	for i := 0; i < n; i++ {
		b.WriteString(csvRow(dataRows[i]))
		b.WriteString("\n")
	}
	b.WriteString("```")
	return b.String(), nil
}

func (t *ExcelTool) opStats(f *excelize.File, sheet string) (string, error) {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return "", err
	}
	if len(rows) < 2 {
		return "", fmt.Errorf("sheet %q needs at least a header + 1 data row for stats", sheet)
	}
	header := rows[0]
	dataRows := rows[1:]
	cols := len(header)

	// Per-column stats. A column is classified numeric if >70% of its
	// non-empty cells parse as float; otherwise text. This matches the
	// heuristic the frontend CsvBlock uses for chart axis detection.
	type stats struct {
		nonEmpty    int
		numericHits int
		sum         float64
		min         float64
		max         float64
		uniq        map[string]int
	}
	out := make([]stats, cols)
	for i := range out {
		out[i].uniq = make(map[string]int)
	}

	for _, r := range dataRows {
		for i := 0; i < cols; i++ {
			if i >= len(r) {
				continue
			}
			v := strings.TrimSpace(r[i])
			if v == "" {
				continue
			}
			out[i].nonEmpty++
			out[i].uniq[v]++
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				out[i].numericHits++
				out[i].sum += n
				if out[i].numericHits == 1 || n < out[i].min {
					out[i].min = n
				}
				if out[i].numericHits == 1 || n > out[i].max {
					out[i].max = n
				}
			}
		}
	}

	var b strings.Builder
	b.WriteString("```csv\n")
	b.WriteString("column,type,count,sum,mean,min,max,unique_values\n")
	for i := 0; i < cols; i++ {
		name := strings.TrimSpace(header[i])
		uniq := len(out[i].uniq)
		isNumeric := out[i].nonEmpty > 0 && out[i].numericHits*10 >= out[i].nonEmpty*7 // ≥70%
		if isNumeric {
			mean := out[i].sum / float64(out[i].numericHits)
			fmt.Fprintf(&b, "%s,numeric,%d,%.2f,%.2f,%.2f,%.2f,%d\n",
				name, out[i].nonEmpty, out[i].sum, mean, out[i].min, out[i].max, uniq)
		} else {
			fmt.Fprintf(&b, "%s,text,%d,,,,,%d\n", name, out[i].nonEmpty, uniq)
		}
	}
	b.WriteString("```")
	return b.String(), nil
}

func (t *ExcelTool) opSummarize(f *excelize.File, sheet string, n int) (string, error) {
	info, err := t.opInfo(f, sheet)
	if err != nil {
		return "", err
	}
	sample, err := t.opSample(f, sheet, n)
	if err != nil {
		return "", err
	}
	stats, err := t.opStats(f, sheet)
	if err != nil {
		// Stats may fail for very small sheets; report info+sample only.
		stats = "(stats unavailable: " + err.Error() + ")"
	}
	return fmt.Sprintf("%s\n\n%s\n\n%s", info, sample, stats), nil
}

// csvRow emits a CSV row, quoting cells that contain commas/quotes/newlines.
func csvRow(cells []string) string {
	out := make([]string, len(cells))
	for i, c := range cells {
		c = strings.TrimSpace(c)
		if strings.ContainsAny(c, ",\"\n") {
			c = "\"" + strings.ReplaceAll(c, "\"", "\"\"") + "\""
		}
		out[i] = c
	}
	return strings.Join(out, ",")
}

func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(math.Round(n)), true
	case string:
		if i, err := strconv.Atoi(n); err == nil {
			return i, true
		}
	}
	return 0, false
}
