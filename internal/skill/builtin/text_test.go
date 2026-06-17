package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// Text skill tests — 15 skills (slugify, case_convert, levenshtein, counts,
// truncate, pad, reverse, sort_lines, dedupe_lines, indent, dedent, regex_replace).
// =====================================================================================

func TestSlugifySkill(t *testing.T) {
	skill := NewSlugifySkill()
	if !skill.Match("slugify: Hello, World!") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "slugify: Hello, World!")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "hello-world" {
		t.Errorf("expected 'hello-world', got %q", out)
	}
}

func TestCaseConvertSkill(t *testing.T) {
	skill := NewCaseConvertSkill()
	if !skill.Match("to snake_case: helloWorld") {
		t.Error("expected match")
	}
	cases := []struct {
		query string
		want  string
	}{
		{"to snake_case: helloWorld", "hello_world"},
		{"to camelcase: hello_world", "helloWorld"},
		{"to kebab-case: helloWorld", "hello-world"},
		{"to title case: hello world", "Hello World"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			out, err := skill.Execute(context.Background(), tc.query)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("expected %q in output, got %q", tc.want, out)
			}
		})
	}
}

func TestLevenshteinSkill(t *testing.T) {
	skill := NewLevenshteinSkill()
	if !skill.Match("levenshtein: kitten|sitting") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "levenshtein: kitten|sitting")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "distance: 3") {
		t.Errorf("expected distance 3 for kitten→sitting, got %q", out)
	}
}

func TestCountSkills(t *testing.T) {
	// char
	cs := NewCharCountSkill()
	if !cs.Match("char count: hello") {
		t.Error("char: expected match")
	}
	out, _ := cs.Execute(context.Background(), "char count: hello")
	if !strings.Contains(out, "5") {
		t.Errorf("char count of 'hello' should be 5, got %q", out)
	}

	// word
	ws := NewWordCountSkill()
	if !ws.Match("word count: one two three") {
		t.Error("word: expected match")
	}
	out, _ = ws.Execute(context.Background(), "word count: one two three")
	if !strings.Contains(out, "3") {
		t.Errorf("word count of 3 words should be 3, got %q", out)
	}

	// line
	ls := NewLineCountSkill()
	if !ls.Match("line count: a\nb\nc") {
		t.Error("line: expected match")
	}
	out, _ = ls.Execute(context.Background(), "line count: a\nb\nc")
	if !strings.Contains(out, "3") {
		t.Errorf("line count of 3 lines should be 3, got %q", out)
	}

	// byte
	bs := NewByteCountSkill()
	if !bs.Match("byte count: hello") {
		t.Error("byte: expected match")
	}
	out, _ = bs.Execute(context.Background(), "byte count: hello")
	if !strings.Contains(out, "5") {
		t.Errorf("byte count of 'hello' should be 5, got %q", out)
	}
}

func TestTruncateSkill(t *testing.T) {
	skill := NewTruncateSkill()
	if !skill.Match("truncate 5: hello world") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "truncate 5: hello world")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// "hell…" = 4 ASCII + 1 ellipsis rune (3 bytes in UTF-8). The skill
	// truncates to n-1 chars then adds ellipsis, so the result should be
	// "hell…" (rune count 5, byte count 7).
	if !strings.HasPrefix(out, "hell") {
		t.Errorf("expected 'hell' prefix (truncate to 5), got %q", out)
	}
	if !strings.HasSuffix(out, "…") && !strings.HasSuffix(out, "...") {
		t.Errorf("expected ellipsis suffix, got %q", out)
	}
}

func TestPadSkill(t *testing.T) {
	skill := NewPadSkill()
	if !skill.Match("pad left 5 0: 42") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "pad left 5 0: 42")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "00042" {
		t.Errorf("expected '00042', got %q", out)
	}
}

func TestReverseSkill(t *testing.T) {
	skill := NewReverseSkill()
	if !skill.Match("reverse: hello") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "reverse: hello")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "olleh" {
		t.Errorf("expected 'olleh', got %q", out)
	}
	// Unicode safety: "café" reversed should be "éfac" (rune-aware, not byte).
	out2, _ := skill.Execute(context.Background(), "reverse: café")
	if out2 != "éfac" {
		t.Errorf("rune-aware reverse of 'café' should be 'éfac', got %q", out2)
	}
}

func TestSortLinesSkill(t *testing.T) {
	skill := NewSortLinesSkill()
	if !skill.Match("sort lines: banana\napple\ncherry") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "sort lines: banana\napple\ncherry")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 3 || lines[0] != "apple" || lines[1] != "banana" || lines[2] != "cherry" {
		t.Errorf("expected sorted lines, got %q", out)
	}
}

func TestDedupeLinesSkill(t *testing.T) {
	skill := NewDedupeLinesSkill()
	if !skill.Match("dedupe lines: a\nb\na\nc") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "dedupe lines: a\nb\na\nc")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 unique lines, got %d: %q", len(lines), out)
	}
}

func TestIndentSkill(t *testing.T) {
	skill := NewIndentSkill()
	if !skill.Match("indent: hello\nworld") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "indent: hello\nworld")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "    hello") {
		t.Errorf("expected 4-space indent, got %q", out)
	}
}

func TestDedentSkill(t *testing.T) {
	skill := NewDedentSkill()
	if !skill.Match("dedent:     hello\n    world") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "dedent:     hello\n    world")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(out, "    hello") {
		t.Errorf("expected leading spaces stripped, got %q", out)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("expected 'hello' content, got %q", out)
	}
}

func TestRegexReplaceSkill(t *testing.T) {
	skill := NewRegexReplaceSkill()
	if !skill.Match(`regex_replace /world/earth/: hello world`) {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), `regex_replace /world/earth/: hello world`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "hello earth") {
		t.Errorf("expected 'hello earth', got %q", out)
	}
	if strings.Contains(out, "world") {
		t.Errorf("world should be replaced, got %q", out)
	}
}
