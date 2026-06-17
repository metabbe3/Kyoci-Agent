package builtin

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Data-format conversion skills — YAML/JSON/TOML/CSV/XML round-trips.
//
// Each skill follows the same shape: Match on (from_format, to_format) pair,
// parse input into map[string]any, re-serialize in target format. Common
// helpers below; per-skill Execute calls the helpers via the named pipeline.
// =====================================================================================

// ---- YAML <-> JSON ----

type YAMLToJSONSkill struct{ *kyoci.BaseSkill }

func NewYAMLToJSONSkill() *YAMLToJSONSkill {
	return &YAMLToJSONSkill{BaseSkill: kyoci.NewBaseSkill(
		"yaml_to_json", "Convert YAML to JSON",
		[]string{"yaml to json", "yaml → json", "yaml 2 json", "convert yaml to json"},
	)}
}
func (s *YAMLToJSONSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "yaml to json") || strings.Contains(q, "yaml → json") ||
		strings.Contains(q, "convert yaml to json") || strings.Contains(q, "yaml 2 json")
}
func (s *YAMLToJSONSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	var v any
	if err := yaml.Unmarshal([]byte(in), &v); err != nil {
		return "", fmt.Errorf("invalid YAML: %w", err)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("JSON encode failed: %w", err)
	}
	return string(out), nil
}

type JSONToYAMLSkill struct{ *kyoci.BaseSkill }

func NewJSONToYAMLSkill() *JSONToYAMLSkill {
	return &JSONToYAMLSkill{BaseSkill: kyoci.NewBaseSkill(
		"json_to_yaml", "Convert JSON to YAML",
		[]string{"json to yaml", "json → yaml", "convert json to yaml"},
	)}
}
func (s *JSONToYAMLSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "json to yaml") || strings.Contains(q, "json → yaml") ||
		strings.Contains(q, "convert json to yaml")
}
func (s *JSONToYAMLSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	var v any
	if err := json.Unmarshal([]byte(in), &v); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}
	out, err := yaml.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("YAML encode failed: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// ---- TOML <-> JSON ----

type TOMLToJSONSkill struct{ *kyoci.BaseSkill }

func NewTOMLToJSONSkill() *TOMLToJSONSkill {
	return &TOMLToJSONSkill{BaseSkill: kyoci.NewBaseSkill(
		"toml_to_json", "Convert TOML to JSON",
		[]string{"toml to json", "toml → json", "convert toml to json"},
	)}
}
func (s *TOMLToJSONSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "toml to json") || strings.Contains(q, "toml → json") ||
		strings.Contains(q, "convert toml to json")
}
func (s *TOMLToJSONSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	var v any
	if err := toml.Unmarshal([]byte(in), &v); err != nil {
		return "", fmt.Errorf("invalid TOML: %w", err)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("JSON encode failed: %w", err)
	}
	return string(out), nil
}

type JSONToTOMLSkill struct{ *kyoci.BaseSkill }

func NewJSONToTOMLSkill() *JSONToTOMLSkill {
	return &JSONToTOMLSkill{BaseSkill: kyoci.NewBaseSkill(
		"json_to_toml", "Convert JSON to TOML",
		[]string{"json to toml", "json → toml", "convert json to toml"},
	)}
}
func (s *JSONToTOMLSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "json to toml") || strings.Contains(q, "json → toml") ||
		strings.Contains(q, "convert json to toml")
}
func (s *JSONToTOMLSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	var v any
	if err := json.Unmarshal([]byte(in), &v); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}
	out, err := toml.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("TOML encode failed: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// ---- CSV <-> JSON ----

type CSVToJSONSkill struct{ *kyoci.BaseSkill }

func NewCSVToJSONSkill() *CSVToJSONSkill {
	return &CSVToJSONSkill{BaseSkill: kyoci.NewBaseSkill(
		"csv_to_json", "Convert CSV (with header row) to a JSON array of objects",
		[]string{"csv to json", "csv → json", "convert csv to json"},
	)}
}
func (s *CSVToJSONSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "csv to json") || strings.Contains(q, "csv → json") ||
		strings.Contains(q, "convert csv to json")
}
func (s *CSVToJSONSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	r := csv.NewReader(strings.NewReader(in))
	rows, err := r.ReadAll()
	if err != nil {
		return "", fmt.Errorf("invalid CSV: %w", err)
	}
	if len(rows) == 0 {
		return "[]", nil
	}
	header := rows[0]
	arr := []map[string]string{}
	for _, row := range rows[1:] {
		obj := map[string]string{}
		for i, col := range header {
			if i < len(row) {
				obj[col] = row[i]
			}
		}
		arr = append(arr, obj)
	}
	out, err := json.MarshalIndent(arr, "", "  ")
	if err != nil {
		return "", fmt.Errorf("JSON encode failed: %w", err)
	}
	return string(out), nil
}

type JSONToCSVSkill struct{ *kyoci.BaseSkill }

func NewJSONToCSVSkill() *JSONToCSVSkill {
	return &JSONToCSVSkill{BaseSkill: kyoci.NewBaseSkill(
		"json_to_csv", "Convert a JSON array of objects to CSV with a header row",
		[]string{"json to csv", "json → csv", "convert json to csv"},
	)}
}
func (s *JSONToCSVSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "json to csv") || strings.Contains(q, "json → csv") ||
		strings.Contains(q, "convert json to csv")
}
func (s *JSONToCSVSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	var arr []map[string]any
	if err := json.Unmarshal([]byte(in), &arr); err != nil {
		// Try map[string]string (less common but valid).
		var arr2 []map[string]string
		if err2 := json.Unmarshal([]byte(in), &arr2); err2 != nil {
			return "", fmt.Errorf("invalid JSON (expected array of objects): %w", err)
		}
		arr = make([]map[string]any, len(arr2))
		for i, m := range arr2 {
			arr[i] = map[string]any{}
			for k, v := range m {
				arr[i][k] = v
			}
		}
	}
	if len(arr) == 0 {
		return "", fmt.Errorf("empty array, no header to emit")
	}

	// Build the column set across all rows (sorted for stable output).
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
	w := csv.NewWriter(&b)
	w.Write(cols)
	for _, row := range arr {
		rec := make([]string, len(cols))
		for i, c := range cols {
			if v, ok := row[c]; ok {
				switch x := v.(type) {
				case string:
					rec[i] = x
				case nil:
					rec[i] = ""
				default:
					js, _ := json.Marshal(x)
					rec[i] = string(js)
				}
			}
		}
		w.Write(rec)
	}
	w.Flush()
	return strings.TrimRight(b.String(), "\n"), nil
}

// ---- XML <-> JSON (best-effort; XML ↔ JSON is fundamentally lossy) ----

type XMLToJSONSkill struct{ *kyoci.BaseSkill }

func NewXMLToJSONSkill() *XMLToJSONSkill {
	return &XMLToJSONSkill{BaseSkill: kyoci.NewBaseSkill(
		"xml_to_json", "Convert XML to a lossy JSON map. Attributes prefixed with '@', text with '#text'",
		[]string{"xml to json", "xml → json", "convert xml to json"},
	)}
}
func (s *XMLToJSONSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "xml to json") || strings.Contains(q, "xml → json") ||
		strings.Contains(q, "convert xml to json")
}
func (s *XMLToJSONSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	var v any
	if err := xml.Unmarshal([]byte(in), &v); err != nil {
		return "", fmt.Errorf("invalid XML: %w", err)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("JSON encode failed: %w", err)
	}
	return string(out), nil
}

type JSONToXMLSkill struct{ *kyoci.BaseSkill }

func NewJSONToXMLSkill() *JSONToXMLSkill {
	return &JSONToXMLSkill{BaseSkill: kyoci.NewBaseSkill(
		"json_to_xml", "Convert a JSON object to XML. Root element name = 'root'",
		[]string{"json to xml", "json → xml", "convert json to xml"},
	)}
}
func (s *JSONToXMLSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "json to xml") || strings.Contains(q, "json → xml") ||
		strings.Contains(q, "convert json to xml")
}
func (s *JSONToXMLSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	var m map[string]any
	if err := json.Unmarshal([]byte(in), &m); err != nil {
		return "", fmt.Errorf("invalid JSON (expected object): %w", err)
	}
	xmlData, err := mapToXML("root", m)
	if err != nil {
		return "", err
	}
	return xml.Header + xmlData, nil
}

// mapToXML is a minimal recursive map[string]any → XML serializer. Nested
// objects become child elements; scalars become character data.
func mapToXML(tag string, v any) (string, error) {
	switch x := v.(type) {
	case map[string]any:
		var b strings.Builder
		fmt.Fprintf(&b, "<%s>", tag)
		for k, child := range x {
			childXML, err := mapToXML(k, child)
			if err != nil {
				return "", err
			}
			b.WriteString(childXML)
		}
		fmt.Fprintf(&b, "</%s>", tag)
		return b.String(), nil
	case []any:
		var b strings.Builder
		for _, item := range x {
			itemXML, err := mapToXML(tag, item)
			if err != nil {
				return "", err
			}
			b.WriteString(itemXML)
		}
		return b.String(), nil
	case nil:
		return fmt.Sprintf("<%s></%s>", tag, tag), nil
	default:
		return fmt.Sprintf("<%s>%v</%s>", tag, x, tag), nil
	}
}

// ---- JSON minify / pretty ----

type JSONMinifySkill struct{ *kyoci.BaseSkill }

func NewJSONMinifySkill() *JSONMinifySkill {
	return &JSONMinifySkill{BaseSkill: kyoci.NewBaseSkill(
		"json_minify", "Minify JSON (no whitespace)",
		[]string{"json minify", "minify json", "compact json"},
	)}
}
func (s *JSONMinifySkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "json minify") || strings.Contains(q, "minify json") ||
		strings.Contains(q, "compact json")
}
func (s *JSONMinifySkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	var v any
	if err := json.Unmarshal([]byte(in), &v); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

type JSONPrettySkill struct{ *kyoci.BaseSkill }

func NewJSONPrettySkill() *JSONPrettySkill {
	return &JSONPrettySkill{BaseSkill: kyoci.NewBaseSkill(
		"json_pretty", "Pretty-print JSON (2-space indent)",
		[]string{"json pretty", "pretty json", "beautify json", "format json"},
	)}
}
func (s *JSONPrettySkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "json pretty") || strings.Contains(q, "pretty json") ||
		strings.Contains(q, "beautify json") || (strings.Contains(q, "format json") && !strings.Contains(q, "json format"))
}
func (s *JSONPrettySkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	var v any
	if err := json.Unmarshal([]byte(in), &v); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ---- env <-> json ----

type EnvToJSONSkill struct{ *kyoci.BaseSkill }

func NewEnvToJSONSkill() *EnvToJSONSkill {
	return &EnvToJSONSkill{BaseSkill: kyoci.NewBaseSkill(
		"env_to_json", "Convert KEY=VALUE lines to a JSON object",
		[]string{"env to json", ".env to json", "dotenv to json", "env file to json"},
	)}
}
func (s *EnvToJSONSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "env to json") || strings.Contains(q, "dotenv to json") ||
		strings.Contains(q, ".env to json") || strings.Contains(q, "env file to json")
}
func (s *EnvToJSONSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	m := map[string]string{}
	for _, line := range strings.Split(in, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Env files use KEY=VALUE — split on first '=', not ':'.
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		val = quoteStripped(val)
		m[key] = val
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

type JSONToEnvSkill struct{ *kyoci.BaseSkill }

func NewJSONToEnvSkill() *JSONToEnvSkill {
	return &JSONToEnvSkill{BaseSkill: kyoci.NewBaseSkill(
		"json_to_env", "Convert a JSON object to KEY=VALUE lines",
		[]string{"json to env", "json to dotenv", "json to .env"},
	)}
}
func (s *JSONToEnvSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "json to env") || strings.Contains(q, "json to dotenv") ||
		strings.Contains(q, "json to .env")
}
func (s *JSONToEnvSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	var m map[string]any
	if err := json.Unmarshal([]byte(in), &m); err != nil {
		return "", fmt.Errorf("invalid JSON (expected object): %w", err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		v := m[k]
		switch x := v.(type) {
		case string:
			fmt.Fprintf(&b, "%s=%s\n", k, x)
		case nil:
			fmt.Fprintf(&b, "%s=\n", k)
		default:
			fmt.Fprintf(&b, "%s=%v\n", k, x)
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}
