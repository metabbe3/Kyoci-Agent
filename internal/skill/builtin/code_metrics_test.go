package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// Code-metrics skill tests — 5 skills: loc_count, complexity_estimate,
// todo_extract, import_extract, function_signature_extract.
//
// These skills operate on multi-line source code, which frequently contains
// colons (Python type hints, JS object literals, URLs). We therefore drive
// Match/Execute directly with the full natural-language verb prefix in each
// case so the operand is recovered via stripVerb, not extractPayload.
// =====================================================================================

// ---- loc_count ----

func TestLOCCount(t *testing.T) {
	skill := NewLOCCountSkill()
	ctx := context.Background()

	// Positive: a tiny snippet with 1 code line, 1 comment, 1 blank.
	src := "package main\n\n// a comment\nfunc main() {}"
	runSkillCases(t, "loc_count", skill, []skillCase{
		{"positive: loc count", "loc count: " + src, true, "Total: 4", false},
		{"positive: count loc", "count loc: " + src, true, "Code: 2", false},
		{"positive: lines of code", "lines of code: " + src, true, "Comment: 1", false},
		{"negative: unrelated", "how many bytes in hi", false, "", false},
	})

	// Detail check: verify the exact numbers add up on a well-known input.
	// Source has 5 lines: 3 code, 1 comment, 1 blank. (stripSource trims the
	// implicit empty first line created by the trailing newline of the verb.)
	out, err := skill.Execute(ctx, "loc count:\n"+strings.Join([]string{
		"package main",           // code
		"",                       // blank
		"// header",              // comment
		"func a() {}",            // code
		"func b() { // inline }", // code (inline comment does not reclassify)
	}, "\n"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"Total: 5", "Code: 3", "Comment: 1", "Blank: 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got %q", want, out)
		}
	}

	// Block-comment spanning lines: every line inside counts as comment.
	// 5 lines total: opening `/*`, 2 middle, closing `*/`, 1 code line →
	// Comment: 4, Code: 1.
	blockSrc := "/*\nline one\nline two\n*/\nvar x = 1"
	out2, err := skill.Execute(ctx, "loc count: "+blockSrc)
	if err != nil {
		t.Fatalf("Execute(block): %v", err)
	}
	if !strings.Contains(out2, "Comment: 4") {
		t.Errorf("expected 4 comment lines for block-comment span, got %q", out2)
	}
	if !strings.Contains(out2, "Code: 1") {
		t.Errorf("expected 1 code line for block-comment span, got %q", out2)
	}

	// Empty input → error.
	if _, err := skill.Execute(ctx, "loc count:"); err == nil {
		t.Error("expected error for empty loc_count input")
	}
}

// ---- complexity_estimate ----

func TestComplexity(t *testing.T) {
	skill := NewComplexityEstimateSkill()
	ctx := context.Background()

	// A trivial function: just one `if` → complexity 2.
	src := "if x > 0 {\n  return 1\n}"
	runSkillCases(t, "complexity_estimate", skill, []skillCase{
		{"positive: complexity", "complexity: " + src, true, "2", false},
		{"positive: complexity estimate", "complexity estimate: " + src, true, "2", false},
		{"positive: cyclomatic complexity", "cyclomatic complexity: " + src, true, "2", false},
		{"negative: unrelated", "slugify hello world", false, "", false},
	})

	// Counted branches add up correctly: if + else if + for + while + case + catch
	// = 6 keywords + 2 (&& and ||) = 8 added to the base 1 → 9.
	branchy := "if a {} else if b {}\nfor x {}\nwhile y {}\ncase z:\ncatch e\na && b || c"
	out, err := skill.Execute(ctx, "complexity estimate: "+branchy)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := "9"
	if strings.TrimSpace(out) != want {
		t.Errorf("complexity = %q, want %q", out, want)
	}

	// Plain function body with no branches → complexity 1.
	out, err = skill.Execute(ctx, "complexity: return 1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.TrimSpace(out) != "1" {
		t.Errorf("complexity = %q, want 1", out)
	}

	// Empty input → error.
	if _, err := skill.Execute(ctx, "complexity estimate:"); err == nil {
		t.Error("expected error for empty complexity input")
	}
}

// ---- todo_extract ----

func TestTODOExtract(t *testing.T) {
	skill := NewTODOExtractSkill()
	ctx := context.Background()

	src := strings.Join([]string{
		"package main",
		"// TODO: refactor this",
		"func a() {}",
		"# FIXME broken",
		"// XXX hot path",
		"// HACK temporary",
	}, "\n")
	runSkillCases(t, "todo_extract", skill, []skillCase{
		{"positive: todo extract", "todo extract: " + src, true, "TODO: refactor this", false},
		{"positive: extract todos", "extract todos: " + src, true, "FIXME broken", false},
		{"positive: find todos", "find todos: " + src, true, "HACK temporary", false},
		{"negative: unrelated", "base64 encode: hello", false, "", false},
	})

	// Line numbers are correct: TODO is on line 2, FIXME on line 4.
	out, err := skill.Execute(ctx, "todo extract: "+src)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Line 2: TODO: refactor this") {
		t.Errorf("expected Line 2 for TODO, got %q", out)
	}
	if !strings.Contains(out, "Line 4: FIXME broken") {
		t.Errorf("expected Line 4 for FIXME, got %q", out)
	}

	// No markers found → friendly message, not an error.
	out, err = skill.Execute(ctx, "todo extract: package main\nfunc main() {}")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(strings.ToLower(out), "no todo") {
		t.Errorf("expected 'no todo' message, got %q", out)
	}

	// Empty input → error.
	if _, err := skill.Execute(ctx, "todo extract:"); err == nil {
		t.Error("expected error for empty todo_extract input")
	}
}

// ---- import_extract ----

func TestImportExtract(t *testing.T) {
	skill := NewImportExtractSkill()
	ctx := context.Background()

	t.Run("positive: go single-line", func(t *testing.T) {
		out, err := skill.Execute(ctx, `import extract: import "fmt"`)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(out, "fmt") {
			t.Errorf("expected fmt, got %q", out)
		}
	})

	t.Run("positive: go block", func(t *testing.T) {
		src := "import (\n\t\"fmt\"\n\tf \"flag\"\n)\nfunc main() {}"
		out, err := skill.Execute(ctx, "extract imports: "+src)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(out, "fmt") || !strings.Contains(out, "flag") {
			t.Errorf("expected fmt and flag in %q", out)
		}
	})

	t.Run("positive: python", func(t *testing.T) {
		src := "import os\nfrom sys import argv\nprint('x')"
		out, err := skill.Execute(ctx, "find imports: "+src)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(out, "import os") {
			t.Errorf("expected 'import os', got %q", out)
		}
		if !strings.Contains(out, "from sys import argv") {
			t.Errorf("expected 'from sys import argv', got %q", out)
		}
	})

	t.Run("positive: js", func(t *testing.T) {
		src := "import React from 'react';\nconst fs = require('fs');"
		out, err := skill.Execute(ctx, "import extract: "+src)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(out, "react") || !strings.Contains(out, "fs") {
			t.Errorf("expected react and fs in %q", out)
		}
	})

	t.Run("positive: c include + java import", func(t *testing.T) {
		src := "#include <stdio.h>\n#include \"local.h\"\nimport org.foo.Bar;"
		out, err := skill.Execute(ctx, "extract imports: "+src)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(out, "#include stdio.h") {
			t.Errorf("expected stdio.h in %q", out)
		}
		if !strings.Contains(out, "#include local.h") {
			t.Errorf("expected local.h in %q", out)
		}
		if !strings.Contains(out, "org.foo.Bar") {
			t.Errorf("expected org.foo.Bar in %q", out)
		}
	})

	// Match negative cases.
	runSkillCases(t, "import_extract", skill, []skillCase{
		{"negative: unrelated", "slugify: hello world", false, "", false},
	})

	// No imports → friendly message.
	out, err := skill.Execute(ctx, "import extract: print('hello')")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(strings.ToLower(out), "no import") {
		t.Errorf("expected 'no imports' message, got %q", out)
	}

	// Empty input → error.
	if _, err := skill.Execute(ctx, "import extract:"); err == nil {
		t.Error("expected error for empty import_extract input")
	}
}

// ---- function_signature_extract ----

func TestFunctionSignature(t *testing.T) {
	skill := NewFunctionSignatureExtractSkill()
	ctx := context.Background()

	t.Run("positive: go func", func(t *testing.T) {
		src := "package main\n\nfunc Add(a, b int) int {\n  return a + b\n}\n\nfunc (s Server) Handle() {}"
		out, err := skill.Execute(ctx, "function signature extract: "+src)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(out, "func Add(a, b int) int") {
			t.Errorf("expected Add signature, got %q", out)
		}
		if !strings.Contains(out, "func (s Server) Handle()") {
			t.Errorf("expected Handle method, got %q", out)
		}
	})

	t.Run("positive: python def", func(t *testing.T) {
		// Def with type-annotated args containing colons — exercises stripVerb.
		src := "def greet(name: str) -> str:\n    return 'hi ' + name"
		out, err := skill.Execute(ctx, "extract function signatures: "+src)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(out, "def greet(name: str)") {
			t.Errorf("expected greet def, got %q", out)
		}
	})

	t.Run("positive: js function + arrow", func(t *testing.T) {
		src := "function add(a, b) { return a + b }\nconst sub = (a, b) => a - b"
		out, err := skill.Execute(ctx, "signature extract: "+src)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(out, "function add(a, b)") {
			t.Errorf("expected add function, got %q", out)
		}
		if !strings.Contains(out, "const sub =") {
			t.Errorf("expected sub arrow, got %q", out)
		}
	})

	t.Run("positive: java method", func(t *testing.T) {
		src := "public int compute(int x) {\n    return x + 1;\n}"
		out, err := skill.Execute(ctx, "function signature: "+src)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(out, "public int compute(int x)") {
			t.Errorf("expected compute method, got %q", out)
		}
	})

	t.Run("negative: control flow is not a signature", func(t *testing.T) {
		// `if (x) {` must NOT be reported as a function signature.
		src := "if (x) {\n  doThing();\n}"
		out, err := skill.Execute(ctx, "function signature extract: "+src)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if strings.Contains(out, "if (x)") {
			t.Errorf("control-flow line leaked into signatures: %q", out)
		}
	})

	// Match negative cases.
	runSkillCases(t, "function_signature_extract", skill, []skillCase{
		{"negative: unrelated", "base64 encode: hello", false, "", false},
	})

	// No signatures → friendly message.
	out, err := skill.Execute(ctx, "function signature: x = 1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(strings.ToLower(out), "no function") {
		t.Errorf("expected 'no function signatures' message, got %q", out)
	}

	// Empty input → error.
	if _, err := skill.Execute(ctx, "function signature:"); err == nil {
		t.Error("expected error for empty function_signature input")
	}
}
