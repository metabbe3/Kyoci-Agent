package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// JSON structure skill tests — 7 skills (flatten, unflatten, keys, values,
// path, pick, omit). Uses the shared runSkillCases driver from encoding_test.go.
// =====================================================================================

// ---- json_flatten ----

func TestJSONFlattenSkill(t *testing.T) {
	runSkillCases(t, "json_flatten", NewJSONFlattenSkill(), []skillCase{
		{
			name:        "positive: json flatten nested object",
			query:       `json flatten: {"a":{"b":1,"c":2}}`,
			shouldMatch: true,
			want:        `"a.b": 1`,
		},
		{
			name:        "positive: flatten json alternate phrasing",
			query:       `flatten json: {"x":{"y":true}}`,
			shouldMatch: true,
			want:        `"x.y": true`,
		},
		{
			name:        "positive: flatten with array index",
			query:       `json flatten: {"arr":[10,20,30]}`,
			shouldMatch: true,
			want:        `"arr.1": 20`,
		},
		{
			name:        "positive: flatten deeply nested",
			query:       `json flatten: {"a":{"b":{"c":{"d":42}}}}`,
			shouldMatch: true,
			want:        `"a.b.c.d": 42`,
		},
		{name: "negative: unflatten", query: `json unflatten: {"a.b":1}`, shouldMatch: false},
		{name: "negative: bare json", query: `format json: {"a":1}`, shouldMatch: false},
		{name: "negative: unrelated", query: `base64 encode: hi`, shouldMatch: false},
		{name: "edge: invalid json", query: `json flatten: {not valid`, shouldMatch: true, wantErr: true},
		{name: "edge: empty payload", query: `json flatten:`, shouldMatch: true, wantErr: true},
	})

	// Independent assertion: flattened form contains both dotted keys.
	t.Run("flatten produces both keys", func(t *testing.T) {
		skill := NewJSONFlattenSkill()
		out, err := skill.Execute(context.Background(), `json flatten: {"a":{"b":1,"c":2}}`)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(out, `"a.b": 1`) || !strings.Contains(out, `"a.c": 2`) {
			t.Errorf("expected both a.b and a.c keys, got %q", out)
		}
	})

	t.Run("flatten scalar input", func(t *testing.T) {
		skill := NewJSONFlattenSkill()
		out, err := skill.Execute(context.Background(), `json flatten: 42`)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		// Top-level scalar — empty prefix; flattenInto writes to "" key.
		// Just ensure it doesn't error and returns valid JSON.
		if !strings.Contains(out, "42") {
			t.Errorf("expected 42 in output, got %q", out)
		}
	})
}

// ---- json_unflatten ----

func TestJSONUnflattenSkill(t *testing.T) {
	runSkillCases(t, "json_unflatten", NewJSONUnflattenSkill(), []skillCase{
		{
			name:        "positive: json unflatten basic",
			query:       `json unflatten: {"a.b":1}`,
			shouldMatch: true,
			want:        `"b": 1`,
		},
		{
			name:        "positive: unflatten json alternate phrasing",
			query:       `unflatten json: {"x.y.z":true}`,
			shouldMatch: true,
			want:        `"z": true`,
		},
		{
			name:        "positive: unflatten two-key flat",
			query:       `json unflatten: {"a.b":1,"a.c":2}`,
			shouldMatch: true,
			want:        `"c": 2`,
		},
		{name: "negative: flatten", query: `json flatten: {"a":{"b":1}}`, shouldMatch: false},
		{name: "negative: unrelated", query: `hash this`, shouldMatch: false},
		{name: "edge: invalid json", query: `json unflatten: {broken`, shouldMatch: true, wantErr: true},
		{name: "edge: empty payload", query: `json unflatten:`, shouldMatch: true, wantErr: true},
	})

	t.Run("unflatten reconstructs nested object", func(t *testing.T) {
		skill := NewJSONUnflattenSkill()
		out, err := skill.Execute(context.Background(), `json unflatten: {"a.b":1,"a.c":2}`)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		// Expect nested {"a":{"b":1,"c":2}}.
		if !strings.Contains(out, `"a": {`) {
			t.Errorf("expected nested 'a' object, got %q", out)
		}
		if !strings.Contains(out, `"b": 1`) || !strings.Contains(out, `"c": 2`) {
			t.Errorf("expected nested b and c keys, got %q", out)
		}
	})

	t.Run("unflatten numeric path becomes array", func(t *testing.T) {
		skill := NewJSONUnflattenSkill()
		// {"arr.0":10,"arr.1":20} → {"arr":[10,20]}
		out, err := skill.Execute(context.Background(), `json unflatten: {"arr.0":10,"arr.1":20}`)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(out, `"arr"`) {
			t.Errorf("expected arr key, got %q", out)
		}
		// Materialized as a JSON array (square brackets).
		if !strings.Contains(out, "[") || !strings.Contains(out, "10") || !strings.Contains(out, "20") {
			t.Errorf("expected array materialization with 10 and 20, got %q", out)
		}
	})
}

// ---- json_keys ----

func TestJSONKeysSkill(t *testing.T) {
	runSkillCases(t, "json_keys", NewJSONKeysSkill(), []skillCase{
		{
			name:        "positive: json keys top-level",
			query:       `json keys: {"a":1,"b":2,"c":3}`,
			shouldMatch: true,
			want:        "a",
		},
		{
			name:        "positive: extract json keys phrasing",
			query:       `extract json keys: {"x":true}`,
			shouldMatch: true,
			want:        "x",
		},
		{
			name:        "positive: recursive keys",
			query:       `json keys --recursive: {"a":{"b":1}}`,
			shouldMatch: true,
			want:        "a.b",
		},
		{name: "negative: json values", query: `json values: {"a":1}`, shouldMatch: false},
		{name: "negative: bare json", query: `format json: {"a":1}`, shouldMatch: false},
		{name: "edge: invalid json", query: `json keys: {bad`, shouldMatch: true, wantErr: true},
		{name: "edge: empty payload", query: `json keys:`, shouldMatch: true, wantErr: true},
	})

	t.Run("top-level keys only", func(t *testing.T) {
		skill := NewJSONKeysSkill()
		out, err := skill.Execute(context.Background(), `json keys: {"a":1,"b":2,"z":3}`)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		// Sorted top-level keys.
		if strings.Contains(out, "a.") {
			t.Errorf("expected top-level only, got nested path: %q", out)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		want := []string{"a", "b", "z"}
		if len(lines) != len(want) {
			t.Fatalf("expected %d keys, got %d (%q)", len(want), len(lines), out)
		}
		for i, w := range want {
			if strings.TrimSpace(lines[i]) != w {
				t.Errorf("line %d: got %q want %q", i, lines[i], w)
			}
		}
	})

	t.Run("recursive keys include nested", func(t *testing.T) {
		skill := NewJSONKeysSkill()
		out, err := skill.Execute(context.Background(), `json keys --recursive: {"a":{"b":1},"c":[10,20]}`)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(out, "a") || !strings.Contains(out, "a.b") {
			t.Errorf("expected 'a' and 'a.b' paths, got %q", out)
		}
		if !strings.Contains(out, "c.0") || !strings.Contains(out, "c.1") {
			t.Errorf("expected array-indexed paths c.0 and c.1, got %q", out)
		}
	})
}

// ---- json_values ----

func TestJSONValuesSkill(t *testing.T) {
	runSkillCases(t, "json_values", NewJSONValuesSkill(), []skillCase{
		{
			name:        "positive: json values scalar leaves",
			query:       `json values: {"a":1,"b":"hi"}`,
			shouldMatch: true,
			want:        "1",
		},
		{
			name:        "positive: extract json values phrasing",
			query:       `extract json values: {"x":[true,false]}`,
			shouldMatch: true,
			want:        "true",
		},
		{
			name:        "positive: deeply nested values",
			query:       `json values: {"a":{"b":{"c":42}}}`,
			shouldMatch: true,
			want:        "42",
		},
		{name: "negative: json keys", query: `json keys: {"a":1}`, shouldMatch: false},
		{name: "negative: unrelated", query: `slugify: hi`, shouldMatch: false},
		{name: "edge: invalid json", query: `json values: {bad`, shouldMatch: true, wantErr: true},
		{name: "edge: empty payload", query: `json values:`, shouldMatch: true, wantErr: true},
	})

	t.Run("values are leaf-only", func(t *testing.T) {
		skill := NewJSONValuesSkill()
		out, err := skill.Execute(context.Background(), `json values: {"a":1,"b":{"c":2},"d":[3,4]}`)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		// Should contain 1, 2, 3, 4 — NOT "b" or "d" keys, NOT JSON objects.
		for _, want := range []string{"1", "2", "3", "4"} {
			if !strings.Contains(out, want) {
				t.Errorf("expected leaf value %s in output, got %q", want, out)
			}
		}
	})
}

// ---- json_path ----

func TestJSONPathSkill(t *testing.T) {
	runSkillCases(t, "json_path", NewJSONPathSkill(), []skillCase{
		{
			name:        "positive: json path with newline",
			query:       "json path: {\"a\":{\"b\":5}}\n$.a.b",
			shouldMatch: true,
			want:        "5",
		},
		{
			name:        "positive: json query with pipe",
			query:       `json query: {"x":[10,20,30]} | $.x[1]`,
			shouldMatch: true,
			want:        "20",
		},
		{
			name:        "positive: path without dollar",
			query:       "json path: {\"a\":{\"c\":\"hi\"}}\na.c",
			shouldMatch: true,
			want:        "hi",
		},
		{name: "negative: json keys", query: `json keys: {"a":1}`, shouldMatch: false},
		{name: "negative: bare json", query: `format json: {"a":1}`, shouldMatch: false},
		{name: "edge: missing args", query: `json path: {"a":1}`, shouldMatch: true, wantErr: true},
		{name: "edge: missing path", query: "json path: {\"a\":1}\n", shouldMatch: true, wantErr: true},
	})

	t.Run("numeric path returns bare number", func(t *testing.T) {
		skill := NewJSONPathSkill()
		out, err := skill.Execute(context.Background(), "json path: {\"a\":{\"b\":42}}\n$.a.b")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if strings.TrimSpace(out) != "42" {
			t.Errorf("expected bare '42', got %q", out)
		}
	})

	t.Run("array index via bracket", func(t *testing.T) {
		skill := NewJSONPathSkill()
		out, err := skill.Execute(context.Background(), `json path: {"arr":[100,200,300]}`+"\n"+"arr[2]")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if strings.TrimSpace(out) != "300" {
			t.Errorf("expected '300', got %q", out)
		}
	})

	t.Run("path resolves to object", func(t *testing.T) {
		skill := NewJSONPathSkill()
		out, err := skill.Execute(context.Background(), `json path: {"a":{"b":{"c":1}}}`+"\n"+"a.b")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(out, `"c": 1`) {
			t.Errorf("expected nested object output, got %q", out)
		}
	})
}

// ---- json_pick ----

func TestJSONPickSkill(t *testing.T) {
	runSkillCases(t, "json_pick", NewJSONPickSkill(), []skillCase{
		{
			name:        "positive: json pick with newline",
			query:       "json pick: {\"a\":1,\"b\":2,\"c\":3}\na,c",
			shouldMatch: true,
			want:        `"a": 1`,
		},
		{
			name:        "positive: pick with pipe and comma",
			query:       `json pick: {"a":1,"b":2} | a,b`,
			shouldMatch: true,
			want:        `"b": 2`,
		},
		{
			name:        "positive: pick preserves nested value",
			query:       "json pick: {\"a\":{\"x\":1},\"b\":2}\na",
			shouldMatch: true,
			want:        `"x": 1`,
		},
		{name: "negative: json omit", query: `json omit: {"a":1}`, shouldMatch: false},
		{name: "negative: bare json", query: `format json: {"a":1}`, shouldMatch: false},
		{name: "edge: missing keys arg", query: `json pick: {"a":1}`, shouldMatch: true, wantErr: true},
		{name: "edge: invalid json", query: `json pick: {bad\na`, shouldMatch: true, wantErr: true},
	})

	t.Run("pick excludes non-listed keys", func(t *testing.T) {
		skill := NewJSONPickSkill()
		out, err := skill.Execute(context.Background(), `json pick: {"a":1,"b":2,"c":3}`+"\n"+"a,c")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(out, `"a"`) || !strings.Contains(out, `"c"`) {
			t.Errorf("expected a and c keys, got %q", out)
		}
		if strings.Contains(out, `"b": 2`) {
			t.Errorf("expected b to be omitted, got %q", out)
		}
	})
}

// ---- json_omit ----

func TestJSONOmitSkill(t *testing.T) {
	runSkillCases(t, "json_omit", NewJSONOmitSkill(), []skillCase{
		{
			name:        "positive: json omit single key",
			query:       "json omit: {\"a\":1,\"b\":2,\"c\":3}\nb",
			shouldMatch: true,
			want:        `"a": 1`,
		},
		{
			name:        "positive: omit with pipe and comma list",
			query:       `json omit: {"a":1,"b":2,"c":3} | a,c`,
			shouldMatch: true,
			want:        `"b": 2`,
		},
		{name: "negative: json pick", query: `json pick: {"a":1}`, shouldMatch: false},
		{name: "negative: bare json", query: `format json: {"a":1}`, shouldMatch: false},
		{name: "edge: missing keys arg", query: `json omit: {"a":1}`, shouldMatch: true, wantErr: true},
		{name: "edge: invalid json", query: `json omit: {bad\na`, shouldMatch: true, wantErr: true},
	})

	t.Run("omit removes only listed keys", func(t *testing.T) {
		skill := NewJSONOmitSkill()
		out, err := skill.Execute(context.Background(), `json omit: {"a":1,"b":2,"c":3}`+"\n"+"b")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if strings.Contains(out, `"b": 2`) {
			t.Errorf("expected b to be omitted, got %q", out)
		}
		if !strings.Contains(out, `"a": 1`) || !strings.Contains(out, `"c": 3`) {
			t.Errorf("expected a and c to remain, got %q", out)
		}
	})
}

// ---- integration: round-trip flatten → unflatten ----

func TestJSONFlattenUnflattenRoundTrip(t *testing.T) {
	original := `{"a":{"b":1,"c":2},"d":3}`
	flat, err := NewJSONFlattenSkill().Execute(context.Background(), "json flatten: "+original)
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}
	roundTrip, err := NewJSONUnflattenSkill().Execute(context.Background(), "json unflatten: "+flat)
	if err != nil {
		t.Fatalf("unflatten: %v", err)
	}
	// The round-trip must contain every leaf value from the original.
	for _, want := range []string{`"b": 1`, `"c": 2`, `"d": 3`} {
		if !strings.Contains(roundTrip, want) {
			t.Errorf("expected %s in round-tripped JSON, got %q", want, roundTrip)
		}
	}
}

// ---- integration: pick is inverse of omit ----

func TestJSONPickAndOmitComplement(t *testing.T) {
	src := `{"a":1,"b":2,"c":3}`
	picked, err := NewJSONPickSkill().Execute(context.Background(), "json pick: "+src+" | a,c")
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	omitted, err := NewJSONOmitSkill().Execute(context.Background(), "json omit: "+src+" | b")
	if err != nil {
		t.Fatalf("omit: %v", err)
	}
	// Both operations should produce identical results when applied to the same source.
	if picked != omitted {
		t.Errorf("pick(a,c) != omit(b):\npick = %q\nomit = %q", picked, omitted)
	}
}
