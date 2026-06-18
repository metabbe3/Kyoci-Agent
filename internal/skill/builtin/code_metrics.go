package builtin

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Code-metrics skills — zero-AI, pure-Go heuristics over source code text.
//
// These skills operate on source code provided as input text. They do NOT read
// files and make NO network/LLM calls. All heuristics are intentionally
// language-agnostic but biased toward Go/Python/JS/Java/C-family syntax.
//
// Because source code frequently contains colons (Python type hints, JS object
// literals, URLs, ternaries), these skills use stripVerb instead of
// extractPayload to recover the operand — stripVerb does not split on the
// first ':' so multi-line source survives intact.
//
// stripSource normalizes the operand: it removes the leading verb, optionally
// strips a leading ':' separator, and trims surrounding whitespace and
// matching quotes. Callers pass multiple verbs and the first non-empty match
// wins.
// =====================================================================================

// stripSource strips the first matching verb from q, then removes an optional
// leading ':' separator and surrounding whitespace/quotes. Returns "" if no
// verb is found OR if the operand after the verb is empty.
//
// Critical: we check the verb is actually present (strings.Contains) before
// calling stripVerb, because stripVerb returns the WHOLE query when the verb
// is missing — that would leak the verb itself into the operand.
func stripSource(q string, verbs ...string) string {
	low := strings.ToLower(q)
	for _, v := range verbs {
		if !strings.Contains(low, strings.ToLower(v)) {
			continue
		}
		rest := strings.TrimSpace(stripVerb(q, v))
		// Allow an optional ':' separator after the verb: "loc count: ..." → "...".
		rest = strings.TrimPrefix(rest, ":")
		rest = strings.TrimPrefix(rest, "：") // fullwidth colon, defensive
		rest = strings.TrimSpace(rest)
		if rest != "" {
			return quoteStripped(rest)
		}
		// Verb present but operand empty — stop here so we don't fall through
		// to a later verb whose stripVerb would return the whole query.
		return ""
	}
	return ""
}

// ---- loc_count ----

// LOCCountSkill counts lines of code: total, blank, comment, and code.
// A line is "comment" if its trimmed form starts with //, #, or /* (or it is
// inside a /* ... */ block). A line is "blank" if it is empty or only
// whitespace. Everything else is "code".
type LOCCountSkill struct{ *kyoci.BaseSkill }

func NewLOCCountSkill() *LOCCountSkill {
	return &LOCCountSkill{BaseSkill: kyoci.NewBaseSkill(
		"loc_count", "Count lines of code: total, blank, comment, code",
		[]string{"loc count", "count loc", "lines of code"},
	)}
}

func (s *LOCCountSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "loc count") || strings.Contains(q, "count loc") ||
		strings.Contains(q, "lines of code") || strings.Contains(q, "count lines of code")
}

func (s *LOCCountSkill) Execute(_ context.Context, q string) (string, error) {
	src := stripSource(q, "loc count", "count loc", "count lines of code", "lines of code")
	if src == "" {
		return "", fmt.Errorf("no source code provided")
	}

	total, blank, comment, code := countLOC(src)
	var b strings.Builder
	fmt.Fprintf(&b, "Total: %d\n", total)
	fmt.Fprintf(&b, "Code: %d\n", code)
	fmt.Fprintf(&b, "Comment: %d\n", comment)
	fmt.Fprintf(&b, "Blank: %d", blank)
	return b.String(), nil
}

// countLOC walks src line-by-line and partitions each into blank/comment/code.
// It tracks open /* ... */ block comments so that every line inside counts as
// a comment until the closing */ is seen.
func countLOC(src string) (total, blank, comment, code int) {
	scanner := bufio.NewScanner(strings.NewReader(src))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	inBlock := false
	for scanner.Scan() {
		line := scanner.Text()
		total++
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if !inBlock {
				blank++
			} else {
				// whitespace inside a block comment still counts as comment
				comment++
			}
			continue
		}
		if inBlock {
			comment++
			if strings.Contains(trimmed, "*/") {
				inBlock = false
			}
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "//"),
			strings.HasPrefix(trimmed, "#"),
			strings.HasPrefix(trimmed, "/*"):
			comment++
			if strings.HasPrefix(trimmed, "/*") && !strings.Contains(trimmed, "*/") {
				inBlock = true
			}
		default:
			// An inline trailing comment (// or #) does not reclassify the
			// whole line as a comment — it still counts as a code line.
			code++
		}
	}
	return total, blank, comment, code
}

// ---- complexity_estimate ----

// ComplexityEstimateSkill produces a rough cyclomatic-complexity estimate.
// Heuristic: start at 1, add 1 for every occurrence of a branching construct:
// `if`, `else if`, `for`, `while`, `case`, `catch`, and the logical operators
// `&&` and `||`. Language-agnostic; intended as a quick smell, not a precise
// McCabe number.
type ComplexityEstimateSkill struct{ *kyoci.BaseSkill }

func NewComplexityEstimateSkill() *ComplexityEstimateSkill {
	return &ComplexityEstimateSkill{BaseSkill: kyoci.NewBaseSkill(
		"complexity_estimate", "Rough cyclomatic complexity estimate (count branches + logical ops + 1)",
		[]string{"complexity", "cyclomatic complexity", "complexity estimate"},
	)}
}

func (s *ComplexityEstimateSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "complexity") || strings.Contains(q, "cyclomatic")
}

// branchWordRe matches the branching keywords we count, anchored on a
// word-boundary so that e.g. `forEach` or `defaultIfEmpty` do not inflate
// the count. The (?i) flag makes it case-insensitive.
var branchWordRe = regexp.MustCompile(`(?i)\b(if|else\s+if|for|while|case|catch)\b`)

func (s *ComplexityEstimateSkill) Execute(_ context.Context, q string) (string, error) {
	src := stripSource(q, "complexity estimate", "cyclomatic complexity", "complexity")
	if src == "" {
		return "", fmt.Errorf("no source code provided")
	}

	complexity := 1
	complexity += len(branchWordRe.FindAllString(src, -1))
	// Count `&&` and `||` as separate decision points. We do this after the
	// keyword scan to avoid double-counting "else if" (already counted as one
	// token by the regex).
	complexity += strings.Count(src, "&&")
	complexity += strings.Count(src, "||")

	return fmt.Sprintf("%d", complexity), nil
}

// ---- todo_extract ----

// TODOExtractSkill pulls TODO/FIXME/XXX/HACK comments out of source code and
// reports them with line numbers. Output: one match per line as `Line N: <text>`.
type TODOExtractSkill struct{ *kyoci.BaseSkill }

func NewTODOExtractSkill() *TODOExtractSkill {
	return &TODOExtractSkill{BaseSkill: kyoci.NewBaseSkill(
		"todo_extract", "Extract TODO/FIXME/XXX/HACK comments from source",
		[]string{"todo extract", "extract todos", "find todos"},
	)}
}

func (s *TODOExtractSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "todo extract") || strings.Contains(q, "extract todo") ||
		strings.Contains(q, "find todo") || strings.Contains(q, "extract todos") ||
		strings.Contains(q, "find todos")
}

// todoMarkerRe matches any of the supported markers on a word boundary so we
// do not pick up words like "TODOS" or "HACKATHON" as false positives.
var todoMarkerRe = regexp.MustCompile(`(?i)\b(TODO|FIXME|XXX|HACK)\b`)

func (s *TODOExtractSkill) Execute(_ context.Context, q string) (string, error) {
	src := stripSource(q, "todo extract", "extract todos", "find todos", "extract todo", "find todo")
	if src == "" {
		return "", fmt.Errorf("no source code provided")
	}

	var out []string
	lineNo := 0
	scanner := bufio.NewScanner(strings.NewReader(src))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if idx := todoMarkerRe.FindStringIndex(line); idx != nil {
			text := strings.TrimSpace(line[idx[0]:])
			// Trim trailing comment closers for tidy output.
			text = strings.TrimRight(text, "*/")
			text = strings.TrimRight(text, " \t")
			out = append(out, fmt.Sprintf("Line %d: %s", lineNo, text))
		}
	}
	if len(out) == 0 {
		return "No TODO/FIXME/XXX/HACK markers found.", nil
	}
	return strings.Join(out, "\n"), nil
}

// ---- import_extract ----

// ImportExtractSkill extracts import statements from source code. Supports:
//   - Go:    `import "..."` and `import ( ... )` blocks
//   - Py:    `import X` and `from X import Y`
//   - JS/TS: `import ... from '...'`, `import '...'`, and `require('...')`
//   - C/C++/Java: `#include <...>` / `#include "..."` and `import x.y.z;`
//
// Output: one import per line. Order preserved as encountered in the source.
type ImportExtractSkill struct{ *kyoci.BaseSkill }

func NewImportExtractSkill() *ImportExtractSkill {
	return &ImportExtractSkill{BaseSkill: kyoci.NewBaseSkill(
		"import_extract", "Extract import statements (Go, Python, JS/TS, Java, C/C++)",
		[]string{"import extract", "extract imports", "find imports"},
	)}
}

func (s *ImportExtractSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "import extract") || strings.Contains(q, "extract import") ||
		strings.Contains(q, "find import") || strings.Contains(q, "extract imports") ||
		strings.Contains(q, "find imports")
}

var (
	// pyImportRe — `import os` or `import os.path`
	pyImportRe = regexp.MustCompile(`^\s*import\s+([\w.]+(?:\s*,\s*[\w.]+)*)\s*$`)
	// pyFromImportRe — `from os import path`
	pyFromImportRe = regexp.MustCompile(`^\s*from\s+([\w.]+)\s+import\s+(.+?)\s*$`)
	// goSingleImportRe — `import "fmt"` or `import f "fmt"`
	goSingleImportRe = regexp.MustCompile(`^\s*import\s+(?:[\w]+\s+)?"([^"]+)"\s*$`)
	// goBlockImportItemRe — one entry inside `import ( ... )`, e.g. `"fmt"` or `f "fmt"`
	goBlockImportItemRe = regexp.MustCompile(`^\s*(?:[\w]+\s+)?"([^"]+)"\s*(//.*)?$`)
	// jsImportRe — `import x from 'y'`, `import {x} from 'y'`, `import 'y'`
	jsImportRe = regexp.MustCompile(`^\s*import\b(?:[^'"]*?\sfrom\s*)?['"]([^'"]+)['"]`)
	// jsRequireRe — `require('y')` or `require("y")`
	jsRequireRe = regexp.MustCompile(`require\(\s*['"]([^'"]+)['"]\s*\)`)
	// cIncludeRe — `#include <stdio.h>` or `#include "file.h"`
	cIncludeRe = regexp.MustCompile(`^\s*#\s*include\s+[<"]([^>"]+)[>"]`)
	// javaImportRe — `import org.foo.Bar;`
	javaImportRe = regexp.MustCompile(`^\s*import\s+([\w.*]+)\s*;`)
)

func (s *ImportExtractSkill) Execute(_ context.Context, q string) (string, error) {
	src := stripSource(q, "import extract", "extract imports", "find imports", "extract import", "find import")
	if src == "" {
		return "", fmt.Errorf("no source code provided")
	}

	imports := extractImports(src)
	if len(imports) == 0 {
		return "No imports found.", nil
	}
	return strings.Join(imports, "\n"), nil
}

// extractImports walks the source once and collects imports across all
// supported languages. We do not need to know the language ahead of time —
// each line is tested against each pattern; first match wins.
func extractImports(src string) []string {
	var out []string
	scanner := bufio.NewScanner(strings.NewReader(src))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	inGoBlock := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Inside a Go `import ( ... )` block — every line that looks like a
		// quoted path is one import. The block ends at the closing `)`.
		if inGoBlock {
			if trimmed == ")" || strings.HasPrefix(trimmed, ")") {
				inGoBlock = false
				continue
			}
			if m := goBlockImportItemRe.FindStringSubmatch(line); m != nil {
				out = append(out, m[1])
			}
			continue
		}

		switch {
		case cIncludeRe.MatchString(line):
			if m := cIncludeRe.FindStringSubmatch(line); m != nil {
				out = append(out, "#include "+m[1])
			}
		case javaImportRe.MatchString(line):
			if m := javaImportRe.FindStringSubmatch(line); m != nil {
				out = append(out, m[1])
			}
		case pyFromImportRe.MatchString(line):
			if m := pyFromImportRe.FindStringSubmatch(line); m != nil {
				out = append(out, "from "+m[1]+" import "+strings.TrimSpace(m[2]))
			}
		case pyImportRe.MatchString(line):
			if m := pyImportRe.FindStringSubmatch(line); m != nil {
				out = append(out, "import "+m[1])
			}
		case jsImportRe.MatchString(line):
			if m := jsImportRe.FindStringSubmatch(line); m != nil {
				out = append(out, m[1])
			}
		case jsRequireRe.MatchString(line):
			// require() — capture every occurrence on the line, not just first.
			for _, match := range jsRequireRe.FindAllStringSubmatch(line, -1) {
				out = append(out, match[1])
			}
		case goSingleImportRe.MatchString(line):
			if m := goSingleImportRe.FindStringSubmatch(line); m != nil {
				out = append(out, m[1])
			}
		default:
			// Detect start of Go `import ( ... )` block.
			if strings.HasPrefix(trimmed, "import") && strings.HasSuffix(trimmed, "(") {
				inGoBlock = true
				continue
			}
		}
	}
	return out
}

// ---- function_signature_extract ----

// FunctionSignatureExtractSkill extracts function/method signatures via regex
// across Go, Python, JS/TS, Java and C. Output: one signature per line as
// `Line N: <signature>`.
type FunctionSignatureExtractSkill struct{ *kyoci.BaseSkill }

func NewFunctionSignatureExtractSkill() *FunctionSignatureExtractSkill {
	return &FunctionSignatureExtractSkill{BaseSkill: kyoci.NewBaseSkill(
		"function_signature_extract",
		"Extract function/method signatures (Go, Python, JS/TS, Java, C)",
		[]string{"function signature", "extract function signatures", "signature extract"},
	)}
}

func (s *FunctionSignatureExtractSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "function signature") ||
		strings.Contains(q, "extract function signature") ||
		strings.Contains(q, "extract function signatures") ||
		strings.Contains(q, "signature extract")
}

var (
	// goFuncRe — `func Name(` or `func (r T) Name(` — capture up to and
	// including the opening paren so the user sees the start of the signature.
	goFuncRe = regexp.MustCompile(`^\s*func\s+(\([^)]*\)\s+)?[A-Za-z_]\w*\s*\(`)
	// pyFuncRe — `def name(` (Python 2/3). Capture up to the opening paren.
	pyFuncRe = regexp.MustCompile(`^\s*def\s+[A-Za-z_]\w*\s*\(`)
	// jsFuncRe — `function name(`. Anonymous `function (` is also matched
	// but the empty name case is handled by the same regex.
	jsFuncRe = regexp.MustCompile(`^\s*function\s*\*?\s*[A-Za-z_]?[\w$]*\s*\(`)
	// jsArrowRe — `const name = (x) =>` style arrow functions; we capture
	// the declaration name to keep output readable.
	jsArrowRe = regexp.MustCompile(`^\s*(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*\([^)]*\)\s*=>`)
	// javaCFuncRe — `<modifiers> returnType Name(` followed eventually by `{`.
	// We capture the leading non-{ portion of the line and require it to end
	// with `(` so we only latch onto true function declarations.
	javaCFuncRe = regexp.MustCompile(`^\s*(?:public|private|protected|static|final|synchronized|abstract|\s)*[A-Za-z_][\w.<>]*(?:\s*\[\s*\])?\s+[A-Za-z_]\w*\s*\([^)]*\)\s*(?:throws\s+[\w.,\s]+)?\s*\{?\s*$`)
)

func (s *FunctionSignatureExtractSkill) Execute(_ context.Context, q string) (string, error) {
	src := stripSource(q, "function signature extract", "extract function signatures",
		"extract function signature", "signature extract", "function signature")
	if src == "" {
		return "", fmt.Errorf("no source code provided")
	}

	sigs := extractSignatures(src)
	if len(sigs) == 0 {
		return "No function signatures found.", nil
	}
	return strings.Join(sigs, "\n"), nil
}

// extractSignatures scans each line and emits any line that looks like a
// function or method declaration in one of the supported languages. We
// preserve the original (untrimmed) line text after stripping trailing `{`
// or `{` whitespace, and prefix with the line number.
func extractSignatures(src string) []string {
	var out []string
	lineNo := 0
	scanner := bufio.NewScanner(strings.NewReader(src))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		var matched bool
		switch {
		case goFuncRe.MatchString(line):
			matched = true
		case pyFuncRe.MatchString(line):
			matched = true
		case jsFuncRe.MatchString(line):
			matched = true
		case jsArrowRe.MatchString(line):
			matched = true
		case javaCFuncRe.MatchString(line):
			// Avoid matching control-flow lines like `if (...) {` or
			// `while (...)` — the regex already excludes those by
			// requiring a return type, but we double-check the line is
			// not a keyword-led statement.
			if !isControlFlowLine(trimmed) {
				matched = true
			}
		}
		if !matched {
			continue
		}

		sig := strings.TrimRight(trimmed, " \t")
		// Strip a trailing `{` so the printed signature is clean. The body
		// opening brace is not part of the signature per se.
		sig = strings.TrimSpace(strings.TrimSuffix(sig, "{"))
		out = append(out, fmt.Sprintf("Line %d: %s", lineNo, sig))
	}
	return out
}

// isControlFlowLine returns true for lines that begin with a C-family control
// keyword followed by `(` — used to filter false positives from the Java/C
// function regex (which can also match `if (...) {`).
func isControlFlowLine(s string) bool {
	re := regexp.MustCompile(`^\s*(if|else|for|while|switch|return|catch|do)\b`)
	return re.MatchString(s)
}
