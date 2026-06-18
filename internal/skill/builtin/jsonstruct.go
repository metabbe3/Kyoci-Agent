package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// JSON structure skills — operate on parsed JSON structure: flatten, unflatten,
// keys, values, path, pick, omit. Distinct from datafmt.go (format conversion)
// and jsonfmt.go (pretty/minify). All implementations are pure Go stdlib —
// no LLM, no network.
//
// Two-argument skills (path, pick, omit) accept their input as either:
//   - "<JSON>\n<args>"  (newline-separated)
//   - "<JSON> | <args>" (pipe-separated)
//
// so embedded JSON values containing spaces don't get split incorrectly.
// =====================================================================================

// ---- json_flatten ----

// JSONFlattenSkill flattens a nested JSON object into dot-notation keys.
// Arrays are indexed as arr.0, arr.1, ...
type JSONFlattenSkill struct{ *kyoci.BaseSkill }

// NewJSONFlattenSkill creates a new json_flatten skill.
func NewJSONFlattenSkill() *JSONFlattenSkill {
	return &JSONFlattenSkill{BaseSkill: kyoci.NewBaseSkill(
		"json_flatten", "Flatten a nested JSON object using dot notation (arrays: arr.0)",
		[]string{"json flatten", "flatten json"},
	)}
}

// Match returns true if the query references json_flatten.
func (s *JSONFlattenSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "json flatten") || strings.Contains(q, "flatten json")
}

// Execute flattens the JSON object found in the query payload.
func (s *JSONFlattenSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	in = strings.TrimSpace(in)
	if in == "" {
		return "", fmt.Errorf("no JSON to flatten")
	}
	var v any
	if err := json.Unmarshal([]byte(in), &v); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}
	flat := map[string]any{}
	flattenInto("", v, flat)
	out, err := json.MarshalIndent(flat, "", "  ")
	if err != nil {
		return "", fmt.Errorf("JSON encode failed: %w", err)
	}
	return string(out), nil
}

// flattenInto walks v and writes dotted keys into out. Objects recurse with a
// "k." prefix; arrays recurse with "0.", "1.", etc.; scalars are written
// directly. Empty objects/arrays become a leaf so the key isn't dropped.
func flattenInto(prefix string, v any, out map[string]any) {
	switch x := v.(type) {
	case map[string]any:
		if len(x) == 0 {
			out[prefix] = map[string]any{}
			return
		}
		for k, child := range x {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flattenInto(key, child, out)
		}
	case []any:
		if len(x) == 0 {
			out[prefix] = []any{}
			return
		}
		for i, child := range x {
			key := strconv.Itoa(i)
			if prefix != "" {
				key = prefix + "." + strconv.Itoa(i)
			}
			flattenInto(key, child, out)
		}
	default:
		out[prefix] = v
	}
}

// ---- json_unflatten ----

// JSONUnflattenSkill expands dot-notation keys back into a nested object.
type JSONUnflattenSkill struct{ *kyoci.BaseSkill }

// NewJSONUnflattenSkill creates a new json_unflatten skill.
func NewJSONUnflattenSkill() *JSONUnflattenSkill {
	return &JSONUnflattenSkill{BaseSkill: kyoci.NewBaseSkill(
		"json_unflatten", "Reverse of json_flatten — expand dot-notation keys into nested objects",
		[]string{"json unflatten", "unflatten json"},
	)}
}

// Match returns true if the query references json_unflatten.
func (s *JSONUnflattenSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "json unflatten") || strings.Contains(q, "unflatten json")
}

// Execute unflattens the JSON object found in the query payload.
func (s *JSONUnflattenSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	in = strings.TrimSpace(in)
	if in == "" {
		return "", fmt.Errorf("no JSON to unflatten")
	}
	var flat map[string]any
	if err := json.Unmarshal([]byte(in), &flat); err != nil {
		return "", fmt.Errorf("invalid JSON (expected flat object): %w", err)
	}
	root, err := unflatten(flat)
	if err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", fmt.Errorf("JSON encode failed: %w", err)
	}
	return string(out), nil
}

// unflatten reconstructs a nested structure from flat dotted keys. Pure-numeric
// path segments become array indices; mixed keys stay as object keys.
func unflatten(flat map[string]any) (any, error) {
	// Use a generic node type: a node is either a map[string]*node or a slice
	// of *node. Leaf values live in a side-table so we can keep ordering sane.
	type node struct {
		obj  map[string]*node
		arr  []*node
		leaf any // non-nil iff this node holds a scalar
	}
	root := &node{obj: map[string]*node{}}
	getOrCreate := func(parent *node, seg string) *node {
		if idx, err := strconv.Atoi(seg); err == nil && idx >= 0 {
			// numeric segment — treat parent as array
			if parent.obj != nil {
				// Mixed parent — keep as object so we don't lose keys.
				child, ok := parent.obj[seg]
				if !ok {
					child = &node{obj: map[string]*node{}}
					parent.obj[seg] = child
				}
				return child
			}
			for len(parent.arr) <= idx {
				parent.arr = append(parent.arr, &node{obj: map[string]*node{}})
			}
			if parent.arr[idx] == nil {
				parent.arr[idx] = &node{obj: map[string]*node{}}
			}
			return parent.arr[idx]
		}
		// string segment
		if parent.arr != nil {
			// parent is array but segment is non-numeric — bail to a stable
			// error rather than corrupting the array.
			return nil
		}
		child, ok := parent.obj[seg]
		if !ok {
			child = &node{obj: map[string]*node{}}
			parent.obj[seg] = child
		}
		return child
	}

	// Deterministic iteration order so output is stable.
	keys := make([]string, 0, len(flat))
	for k := range flat {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		segs := strings.Split(k, ".")
		cur := root
		for i, seg := range segs {
			if seg == "" {
				return nil, fmt.Errorf("invalid key %q: empty segment", k)
			}
			if i == len(segs)-1 {
				// leaf
				if cur.obj != nil {
					if existing, ok := cur.obj[seg]; ok && (existing.leaf != nil || len(existing.obj) > 0 || len(existing.arr) > 0) {
						// Overwriting an intermediate with a leaf — keep the
						// last value (typical map semantics).
					}
					cur.obj[seg] = &node{leaf: flat[k]}
				} else {
					// numeric leaf on array
					idx, err := strconv.Atoi(seg)
					if err != nil || idx < 0 {
						return nil, fmt.Errorf("cannot assign leaf %q to array path", seg)
					}
					for len(cur.arr) <= idx {
						cur.arr = append(cur.arr, &node{leaf: nil})
					}
					cur.arr[idx] = &node{leaf: flat[k]}
				}
				break
			}
			next := getOrCreate(cur, seg)
			if next == nil {
				return nil, fmt.Errorf("cannot traverse %q: mixed array/object path", k)
			}
			cur = next
		}
	}

	var materialize func(*node) any
	materialize = func(n *node) any {
		if n.leaf != nil || (n.obj == nil && n.arr == nil) {
			return n.leaf
		}
		if n.obj != nil {
			// Decide: array or object? If every key is numeric and contiguous,
			// emit as array. Otherwise object.
			allNumeric := true
			maxIdx := -1
			for k := range n.obj {
				idx, err := strconv.Atoi(k)
				if err != nil || idx < 0 {
					allNumeric = false
					break
				}
				if idx > maxIdx {
					maxIdx = idx
				}
			}
			if allNumeric && len(n.obj) > 0 && maxIdx == len(n.obj)-1 {
				arr := make([]any, len(n.obj))
				for k, child := range n.obj {
					idx, _ := strconv.Atoi(k)
					arr[idx] = materialize(child)
				}
				return arr
			}
			out := map[string]any{}
			for k, child := range n.obj {
				out[k] = materialize(child)
			}
			return out
		}
		arr := make([]any, len(n.arr))
		for i, child := range n.arr {
			arr[i] = materialize(child)
		}
		return arr
	}

	return materialize(root), nil
}

// ---- json_keys ----

// JSONKeysSkill lists top-level (or recursive) keys of a JSON object.
type JSONKeysSkill struct{ *kyoci.BaseSkill }

// NewJSONKeysSkill creates a new json_keys skill.
func NewJSONKeysSkill() *JSONKeysSkill {
	return &JSONKeysSkill{BaseSkill: kyoci.NewBaseSkill(
		"json_keys", "List JSON keys. Top-level by default; pass --recursive for nested keys (dot-notation)",
		[]string{"json keys", "extract json keys"},
	)}
}

// Match returns true if the query references json_keys.
func (s *JSONKeysSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "json keys") || strings.Contains(q, "extract json keys")
}

// Execute lists keys in the JSON payload.
func (s *JSONKeysSkill) Execute(_ context.Context, q string) (string, error) {
	low := strings.ToLower(q)
	recursive := strings.Contains(low, "--recursive") || strings.Contains(low, "recursive")
	in := extractPayload(q)
	// strip leading flags from the payload
	in = strings.TrimSpace(stripLeadingFlags(in))
	if in == "" {
		return "", fmt.Errorf("no JSON to list keys from")
	}
	var v any
	if err := json.Unmarshal([]byte(in), &v); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}
	var keys []string
	if recursive {
		collectKeysRecursive("", v, &keys)
	} else {
		collectKeysTopLevel(v, &keys)
	}
	sort.Strings(keys)
	return strings.Join(keys, "\n"), nil
}

// collectKeysTopLevel appends the top-level keys of v (if object) to out.
func collectKeysTopLevel(v any, out *[]string) {
	if m, ok := v.(map[string]any); ok {
		for k := range m {
			*out = append(*out, k)
		}
	}
}

// collectKeysRecursive walks v collecting dotted key paths (including array
// indices). Objects and arrays add their own path components; scalars don't.
func collectKeysRecursive(prefix string, v any, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			*out = append(*out, path)
			collectKeysRecursive(path, child, out)
		}
	case []any:
		for i, child := range x {
			path := strconv.Itoa(i)
			if prefix != "" {
				path = prefix + "." + strconv.Itoa(i)
			}
			*out = append(*out, path)
			collectKeysRecursive(path, child, out)
		}
	}
}

// ---- json_values ----

// JSONValuesSkill lists every leaf value (recursive) in a JSON document.
type JSONValuesSkill struct{ *kyoci.BaseSkill }

// NewJSONValuesSkill creates a new json_values skill.
func NewJSONValuesSkill() *JSONValuesSkill {
	return &JSONValuesSkill{BaseSkill: kyoci.NewBaseSkill(
		"json_values", "List all leaf values in JSON (recursive)",
		[]string{"json values", "extract json values"},
	)}
}

// Match returns true if the query references json_values.
func (s *JSONValuesSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "json values") || strings.Contains(q, "extract json values")
}

// Execute lists leaf values in the JSON payload.
func (s *JSONValuesSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	if in == "" {
		return "", fmt.Errorf("no JSON to list values from")
	}
	var v any
	if err := json.Unmarshal([]byte(in), &v); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}
	var leaves []string
	collectLeafValues(v, &leaves)
	return strings.Join(leaves, "\n"), nil
}

// collectLeafValues walks v and appends each leaf (scalar) as its JSON-encoded
// representation. Empty objects/arrays are treated as leaves.
func collectLeafValues(v any, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		if len(x) == 0 {
			js, _ := json.Marshal(v)
			*out = append(*out, string(js))
			return
		}
		for _, child := range x {
			collectLeafValues(child, out)
		}
	case []any:
		if len(x) == 0 {
			js, _ := json.Marshal(v)
			*out = append(*out, string(js))
			return
		}
		for _, child := range x {
			collectLeafValues(child, out)
		}
	default:
		js, _ := json.Marshal(v)
		*out = append(*out, string(js))
	}
}

// ---- json_path ----

// JSONPathSkill queries a JSON document with a simple dot-path expression.
// Path may begin with an optional leading '$'. Arrays indexed with [N] or .N.
type JSONPathSkill struct{ *kyoci.BaseSkill }

// NewJSONPathSkill creates a new json_path skill.
func NewJSONPathSkill() *JSONPathSkill {
	return &JSONPathSkill{BaseSkill: kyoci.NewBaseSkill(
		"json_path", "Query JSON with a dot path. Input: '<JSON>\\n<path>' or '<JSON> | <path>'",
		[]string{"json path", "json query"},
	)}
}

// Match returns true if the query references json_path.
func (s *JSONPathSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "json path") || strings.Contains(q, "json query")
}

// Execute evaluates the path against the JSON document.
func (s *JSONPathSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	jsonStr, pathStr, ok := splitJSONAndArgs(payload)
	if !ok {
		return "", fmt.Errorf("expected '<JSON>\\n<path>' or '<JSON> | <path>'")
	}
	jsonStr = strings.TrimSpace(jsonStr)
	pathStr = strings.TrimSpace(pathStr)
	if jsonStr == "" || pathStr == "" {
		return "", fmt.Errorf("both JSON document and path are required")
	}
	var v any
	if err := json.Unmarshal([]byte(jsonStr), &v); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}
	result, err := evalJSONPath(v, pathStr)
	if err != nil {
		return "", err
	}
	// Bare scalar result — return as a plain string (no JSON quotes) for the
	// common case of "give me the value". Structured results serialize as JSON.
	switch x := result.(type) {
	case nil:
		return "null", nil
	case string:
		return x, nil
	case bool:
		return strconv.FormatBool(x), nil
	case float64:
		// JSON numbers unmarshal as float64. Render integer-valued floats
		// without a decimal part for friendlier output.
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10), nil
		}
		return strconv.FormatFloat(x, 'f', -1, 64), nil
	default:
		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return "", fmt.Errorf("JSON encode failed: %w", err)
		}
		return string(out), nil
	}
}

// evalJSONPath navigates v using a dot-and-bracket path. Supports forms:
// "$.a.b", "a.b", "a[0].b", "a.0.b". Empty path returns v unchanged.
func evalJSONPath(v any, path string) (any, error) {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "$")
	path = strings.TrimSpace(path)
	// normalize: turn "a[0]" into "a.0"
	path = strings.ReplaceAll(path, "[", ".")
	path = strings.ReplaceAll(path, "]", "")
	segs := strings.Split(path, ".")
	cur := v
	for _, seg := range segs {
		if seg == "" {
			continue
		}
		switch node := cur.(type) {
		case map[string]any:
			next, ok := node[seg]
			if !ok {
				return nil, fmt.Errorf("key %q not found", seg)
			}
			cur = next
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, fmt.Errorf("index %q out of range", seg)
			}
			cur = node[idx]
		default:
			return nil, fmt.Errorf("cannot descend into scalar at %q", seg)
		}
	}
	return cur, nil
}

// ---- json_pick ----

// JSONPickSkill keeps only the listed top-level keys from a JSON object.
type JSONPickSkill struct{ *kyoci.BaseSkill }

// NewJSONPickSkill creates a new json_pick skill.
func NewJSONPickSkill() *JSONPickSkill {
	return &JSONPickSkill{BaseSkill: kyoci.NewBaseSkill(
		"json_pick", "Pick subset of top-level keys. Input: '<JSON>\\n<a,b,c>' or '<JSON> | <a,b,c>'",
		[]string{"json pick", "pick json keys"},
	)}
}

// Match returns true if the query references json_pick.
func (s *JSONPickSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "json pick")
}

// Execute picks the requested keys from the JSON object.
func (s *JSONPickSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	jsonStr, argsStr, ok := splitJSONAndArgs(payload)
	if !ok {
		return "", fmt.Errorf("expected '<JSON>\\n<keys>' or '<JSON> | <keys>'")
	}
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return "", fmt.Errorf("no JSON to pick from")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return "", fmt.Errorf("invalid JSON (expected object): %w", err)
	}
	want := splitKeyList(argsStr)
	if len(want) == 0 {
		return "", fmt.Errorf("no keys to pick")
	}
	out := map[string]any{}
	for _, k := range want {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("JSON encode failed: %w", err)
	}
	return string(encoded), nil
}

// ---- json_omit ----

// JSONOmitSkill drops the listed top-level keys from a JSON object.
type JSONOmitSkill struct{ *kyoci.BaseSkill }

// NewJSONOmitSkill creates a new json_omit skill.
func NewJSONOmitSkill() *JSONOmitSkill {
	return &JSONOmitSkill{BaseSkill: kyoci.NewBaseSkill(
		"json_omit", "Drop top-level keys from a JSON object. Input: '<JSON>\\n<a,b>' or '<JSON> | <a,b>'",
		[]string{"json omit", "omit json keys"},
	)}
}

// Match returns true if the query references json_omit.
func (s *JSONOmitSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "json omit")
}

// Execute omits the requested keys from the JSON object.
func (s *JSONOmitSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	jsonStr, argsStr, ok := splitJSONAndArgs(payload)
	if !ok {
		return "", fmt.Errorf("expected '<JSON>\\n<keys>' or '<JSON> | <keys>'")
	}
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return "", fmt.Errorf("no JSON to omit from")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return "", fmt.Errorf("invalid JSON (expected object): %w", err)
	}
	drop := splitKeyList(argsStr)
	out := map[string]any{}
	for k, v := range m {
		if !containsStr(drop, k) {
			out[k] = v
		}
	}
	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("JSON encode failed: %w", err)
	}
	return string(encoded), nil
}

// =====================================================================================
// Local helpers
// =====================================================================================

// splitJSONAndArgs splits a payload into the JSON document and its argument
// list. Supports two forms:
//   - newline-separated: "<json>\n<args>"
//   - pipe-separated:    "<json> | <args>"
//
// Returns ok=false if neither separator is found.
func splitJSONAndArgs(payload string) (jsonStr, argsStr string, ok bool) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return "", "", false
	}
	if idx := strings.Index(payload, "\n"); idx >= 0 {
		return payload[:idx], payload[idx+1:], true
	}
	if idx := strings.Index(payload, "|"); idx >= 0 {
		// Confirm the '|' is a real separator by checking both sides are
		// non-empty once trimmed.
		left := strings.TrimSpace(payload[:idx])
		right := strings.TrimSpace(payload[idx+1:])
		if left != "" && right != "" {
			return left, right, true
		}
	}
	return "", "", false
}

// splitKeyList splits a comma- or space-separated list of keys, trimming each.
func splitKeyList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Also allow whitespace-separated lists ("a b c").
		fields := strings.Fields(part)
		if len(fields) > 1 {
			out = append(out, fields...)
		} else {
			out = append(out, part)
		}
	}
	return out
}

// containsStr reports whether haystack contains needle.
func containsStr(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// stripLeadingFlags removes leading "--flag" tokens from s so flags like
// "--recursive" don't leak into the parsed JSON.
func stripLeadingFlags(s string) string {
	for {
		trimmed := strings.TrimSpace(s)
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			return trimmed
		}
		if !strings.HasPrefix(fields[0], "--") {
			return trimmed
		}
		// drop the leading flag token and continue
		idx := strings.Index(trimmed, fields[0])
		s = trimmed[idx+len(fields[0]):]
	}
}
