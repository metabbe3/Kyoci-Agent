package agent

import (
	"strings"
	"testing"
)

// TestParseOrchSteps_TiersBackwardCompat confirms the existing tiers still
// catch what they caught before the trailing-comma tier was added.
func TestParseOrchSteps_TiersBackwardCompat(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "strict JSON",
			input: `[{"id":1,"description":"a","depends_on":[],"tool_hint":""}]`,
			want:  1,
		},
		{
			name:  "markdown fence stripped",
			input: "```json\n[{\"id\":1,\"description\":\"a\",\"depends_on\":[],\"tool_hint\":\"\"}]\n```",
			want:  1,
		},
		{
			name:  "leading prose + array",
			input: "Here is the plan:\n[{\"id\":1,\"description\":\"a\",\"depends_on\":[],\"tool_hint\":\"\"}]",
			want:  1,
		},
		{
			name:  "multi-step",
			input: `[{"id":1,"description":"a","depends_on":[],"tool_hint":"x"},{"id":2,"description":"b","depends_on":[1],"tool_hint":"y"}]`,
			want:  2,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseOrchSteps(c.input)
			if err != nil {
				t.Fatalf("parseOrchSteps(%q) returned error: %v", c.input, err)
			}
			if len(got) != c.want {
				t.Fatalf("parseOrchSteps(%q) returned %d steps, want %d", c.input, len(got), c.want)
			}
		})
	}
}

// TestParseOrchSteps_TrailingComma verifies the Tier 2.5 fix for the #1
// small-model JSON mistake: trailing commas before ] or }. Without this tier,
// these inputs fail all 3 existing tiers and bubble up as "planner output
// parse failed" — the root cause of stochastic agent-mode failures.
func TestParseOrchSteps_TrailingComma(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  int
	}{
		{
			name: "trailing comma after last object in array",
			input: `[
  {"id":1,"description":"a","depends_on":[],"tool_hint":""},
]`,
			want: 1,
		},
		{
			name: "trailing comma in nested depends_on",
			input: `[{"id":1,"description":"a","depends_on":[,],"tool_hint":""}]`,
			want: 1,
		},
		{
			name: "trailing comma in object field",
			input: `[{"id":1,"description":"a","depends_on":[],"tool_hint":"",}]`,
			want: 1,
		},
		{
			name: "trailing comma in fence-wrapped block",
			input: "```json\n" + `[
  {"id":1,"description":"a","depends_on":[],"tool_hint":""},
]` + "\n```",
			want: 1,
		},
		{
			name: "real-world gemma-4-e4b-style output with trailing comma",
			input: strings.NewReplacer("~", "`").Replace(`[
  {
    "id": 1,
    "description": "Define goal",
    "depends_on": [],
    "tool_hint": "planning"
  },
  {
    "id": 2,
    "description": "Build page",
    "depends_on": [1],
    "tool_hint": "file"
  },
]`),
			want: 2,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseOrchSteps(c.input)
			if err != nil {
				t.Fatalf("parseOrchSteps returned error for trailing-comma case %q: %v", c.name, err)
			}
			if len(got) != c.want {
				t.Fatalf("parseOrchSteps returned %d steps, want %d (case %q)", len(got), c.want, c.name)
			}
		})
	}
}

// TestStripTrailingCommas is a focused unit test on the helper regex.
func TestStripTrailingCommas(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`[1,2,3,]`, `[1,2,3]`},
		{`{"a":1,}`, `{"a":1}`},
		{`[{"x":1,},{"y":2,}]`, `[{"x":1},{"y":2}]`},
		{`[1, 2, 3,   ]`, `[1, 2, 3]`}, // regex absorbs whitespace before ] too
		{`{"a":1,"b":[2,3,]}`, `{"a":1,"b":[2,3]}`},
		{`no commas here`, `no commas here`},
		{`,`, `,`}, // not before } or ] — unchanged
	}
	for _, c := range cases {
		got := stripTrailingCommas(c.in)
		if got != c.want {
			t.Errorf("stripTrailingCommas(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestParseOrchSteps_EnvelopeUnwrap confirms that wrapped envelopes like
// `{"steps":[...]}` or `{"plan":[...]}` are unwrapped for free by the existing
// extractOutermostArray tier — small models sometimes wrap the array instead
// of emitting it bare.
func TestParseOrchSteps_EnvelopeUnwrap(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  int
	}{
		{
			name: "steps envelope",
			input: `{"steps":[
  {"id":1,"description":"a","depends_on":[],"tool_hint":""}
]}`,
			want: 1,
		},
		{
			name: "plan envelope",
			input: `{"plan":[{"id":1,"description":"a","depends_on":[],"tool_hint":""}]}`,
			want: 1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseOrchSteps(c.input)
			if err != nil {
				t.Fatalf("parseOrchSteps(%q) failed: %v", c.input, err)
			}
			if len(got) != c.want {
				t.Fatalf("got %d steps, want %d", len(got), c.want)
			}
		})
	}
}

// TestParseOrchSteps_StillFailsOnGarbage confirms we haven't made the parser
// accept actual garbage — only trailing-comma salvage. Truly malformed input
// still surfaces an error so callers can react.
//
// Note: `{"steps":[...]}` envelopes are NOT in the "garbage" list — the
// existing extractOutermostArray tier already unwraps them for free by
// finding the inner array.
func TestParseOrchSteps_StillFailsOnGarbage(t *testing.T) {
	bad := []string{
		"",
		"this is not json at all",
		"[{no closing brace",
		"[\n  broken json without proper structure\n",
	}
	for _, in := range bad {
		if _, err := parseOrchSteps(in); err == nil {
			t.Errorf("parseOrchSteps(%q) unexpectedly succeeded", in)
		}
	}
}
