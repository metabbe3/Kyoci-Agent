package builtin

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"unicode"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Text-manipulation skills — slugify, case-convert, levenshtein, counts, sort,
// dedupe, indent, pad, reverse, regex_replace.
// =====================================================================================

// ---- slugify ----

type SlugifySkill struct{ *kyoci.BaseSkill }

func NewSlugifySkill() *SlugifySkill {
	return &SlugifySkill{BaseSkill: kyoci.NewBaseSkill(
		"slugify", "Convert text to URL-safe slug (lowercase, hyphens, no special chars)",
		[]string{"slugify", "slug from", "url slug"},
	)}
}
func (s *SlugifySkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "slugify") || strings.Contains(q, "url slug") ||
		strings.Contains(q, "make a slug") || strings.Contains(q, "slug from")
}
func (s *SlugifySkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	if in == "" {
		return "", fmt.Errorf("no text to slugify")
	}
	var b strings.Builder
	for _, r := range in {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(unicode.ToLower(r))
		case r == ' ' || r == '_' || r == '-':
			// collapse runs of separators into a single hyphen
			if b.Len() == 0 || strings.HasSuffix(b.String(), "-") {
				continue
			}
			b.WriteRune('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "", fmt.Errorf("slug is empty after sanitization")
	}
	return slug, nil
}

// ---- case convert ----

type CaseConvertSkill struct{ *kyoci.BaseSkill }

func NewCaseConvertSkill() *CaseConvertSkill {
	return &CaseConvertSkill{BaseSkill: kyoci.NewBaseSkill(
		"case_convert", "Convert between camelCase, snake_case, kebab-case, Title Case. Usage: 'to snake_case: helloWorld'",
		[]string{"case convert", "convert case", "to camel", "to snake", "to kebab", "to title case"},
	)}
}
func (s *CaseConvertSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "case convert") || strings.Contains(q, "convert case") ||
		strings.Contains(q, "to camelcase") || strings.Contains(q, "to camel case") ||
		strings.Contains(q, "to snake_case") || strings.Contains(q, "to snake case") ||
		strings.Contains(q, "to kebab-case") || strings.Contains(q, "to kebab case") ||
		strings.Contains(q, "to title case") || strings.Contains(q, "to pascalcase")
}
func (s *CaseConvertSkill) Execute(_ context.Context, q string) (string, error) {
	low := strings.ToLower(q)
	target := ""
	for _, t := range []string{"camelcase", "camel_case", "snake_case", "snake", "kebab", "title", "pascalcase", "pascal"} {
		if strings.Contains(low, "to "+t) {
			switch t {
			case "camelcase", "camel_case":
				target = "camel"
			case "snake_case", "snake":
				target = "snake"
			case "kebab":
				target = "kebab"
			case "pascalcase", "pascal":
				target = "pascal"
			case "title":
				target = "title"
			}
			break
		}
	}
	if target == "" {
		return "", fmt.Errorf("specify target case: camelCase, snake_case, kebab-case, Title Case, PascalCase")
	}
	in := extractPayload(q)
	words := splitWordsAny(in)
	if len(words) == 0 {
		return "", fmt.Errorf("no text to convert")
	}
	switch target {
	case "camel":
		var b strings.Builder
		for i, w := range words {
			if i == 0 {
				b.WriteString(strings.ToLower(w))
			} else {
				b.WriteString(strings.Title(strings.ToLower(w)))
			}
		}
		return b.String(), nil
	case "pascal":
		var b strings.Builder
		for _, w := range words {
			b.WriteString(strings.Title(strings.ToLower(w)))
		}
		return b.String(), nil
	case "snake":
		parts := make([]string, len(words))
		for i, w := range words {
			parts[i] = strings.ToLower(w)
		}
		return strings.Join(parts, "_"), nil
	case "kebab":
		parts := make([]string, len(words))
		for i, w := range words {
			parts[i] = strings.ToLower(w)
		}
		return strings.Join(parts, "-"), nil
	case "title":
		parts := make([]string, len(words))
		for i, w := range words {
			parts[i] = strings.Title(strings.ToLower(w))
		}
		return strings.Join(parts, " "), nil
	}
	return "", fmt.Errorf("unsupported target case")
}

// splitWordsAny splits on any word boundary (whitespace, _, -, camelCase transitions).
func splitWordsAny(s string) []string {
	var words []string
	var cur strings.Builder
	prevLower := false
	for _, r := range s {
		if r == ' ' || r == '_' || r == '-' || r == '\t' {
			if cur.Len() > 0 {
				words = append(words, cur.String())
				cur.Reset()
			}
			prevLower = false
			continue
		}
		isUpper := unicode.IsUpper(r)
		if isUpper && prevLower && cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
		cur.WriteRune(r)
		prevLower = !isUpper && unicode.IsLetter(r)
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}
	return words
}

// ---- levenshtein ----

type LevenshteinSkill struct{ *kyoci.BaseSkill }

func NewLevenshteinSkill() *LevenshteinSkill {
	return &LevenshteinSkill{BaseSkill: kyoci.NewBaseSkill(
		"levenshtein", "Levenshtein edit distance between two strings. Input: 's1|s2'",
		[]string{"levenshtein", "edit distance", "string distance"},
	)}
}
func (s *LevenshteinSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "levenshtein") || strings.Contains(q, "edit distance") ||
		strings.Contains(q, "string distance")
}
func (s *LevenshteinSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	parts := strings.SplitN(payload, "|", 2)
	if len(parts) != 2 {
		// Try comma-separation.
		parts = strings.SplitN(payload, ",", 2)
	}
	if len(parts) != 2 {
		return "", fmt.Errorf("expected 's1|s2' or 's1,s2' format")
	}
	s1 := strings.TrimSpace(parts[0])
	s2 := strings.TrimSpace(parts[1])
	d := levenshtein(s1, s2)
	pct := 0.0
	maxLen := len(s1)
	if len(s2) > maxLen {
		maxLen = len(s2)
	}
	if maxLen > 0 {
		pct = (1.0 - float64(d)/float64(maxLen)) * 100
	}
	return fmt.Sprintf("distance: %d\nsimilarity: %.1f%%", d, pct), nil
}

// levenshtein computes the edit distance using a single-row DP table.
func levenshtein(s, t string) int {
	rs := []rune(s)
	rt := []rune(t)
	if len(rs) == 0 {
		return len(rt)
	}
	if len(rt) == 0 {
		return len(rs)
	}
	prev := make([]int, len(rt)+1)
	curr := make([]int, len(rt)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(rs); i++ {
		curr[0] = i
		for j := 1; j <= len(rt); j++ {
			cost := 1
			if rs[i-1] == rt[j-1] {
				cost = 0
			}
			min := prev[j] + 1
			if curr[j-1]+1 < min {
				min = curr[j-1] + 1
			}
			if prev[j-1]+cost < min {
				min = prev[j-1] + cost
			}
			curr[j] = min
		}
		prev, curr = curr, prev
	}
	return prev[len(rt)]
}

// ---- counts (4 skills) ----

type CountSkill struct {
	*kyoci.BaseSkill
	mode string // "char", "word", "line", "byte"
}

func (s *CountSkill) Execute(_ context.Context, q string) (string, error) {
	t := extractPayload(q)
	if t == "" {
		t = q
	}
	switch s.mode {
	case "char":
		return fmt.Sprintf("%d", len([]rune(t))), nil
	case "word":
		return fmt.Sprintf("%d", len(strings.Fields(t))), nil
	case "line":
		return fmt.Sprintf("%d", strings.Count(t, "\n")+1), nil
	case "byte":
		return fmt.Sprintf("%d", len(t)), nil
	}
	return "", fmt.Errorf("unknown count mode")
}
func (s *CountSkill) Match(q string) bool {
	q = strings.ToLower(q)
	switch s.mode {
	case "char":
		return strings.Contains(q, "char count") || strings.Contains(q, "count chars") ||
			strings.Contains(q, "count characters") || strings.Contains(q, "character count")
	case "word":
		return strings.Contains(q, "word count") || strings.Contains(q, "count words")
	case "line":
		return strings.Contains(q, "line count") || strings.Contains(q, "count lines")
	case "byte":
		return strings.Contains(q, "byte count") || strings.Contains(q, "count bytes")
	}
	return false
}

func NewCharCountSkill() *CountSkill {
	return &CountSkill{BaseSkill: kyoci.NewBaseSkill(
		"char_count", "Count characters (runes) in text", []string{"char count"}),
		mode: "char",
	}
}
func NewWordCountSkill() *CountSkill {
	return &CountSkill{BaseSkill: kyoci.NewBaseSkill(
		"word_count", "Count words (whitespace-split) in text", []string{"word count"}),
		mode: "word",
	}
}
func NewLineCountSkill() *CountSkill {
	return &CountSkill{BaseSkill: kyoci.NewBaseSkill(
		"line_count", "Count lines in text", []string{"line count"}),
		mode: "line",
	}
}
func NewByteCountSkill() *CountSkill {
	return &CountSkill{BaseSkill: kyoci.NewBaseSkill(
		"byte_count", "Count bytes (UTF-8) in text", []string{"byte count"}),
		mode: "byte",
	}
}

// ---- truncate ----

type TruncateSkill struct{ *kyoci.BaseSkill }

func NewTruncateSkill() *TruncateSkill {
	return &TruncateSkill{BaseSkill: kyoci.NewBaseSkill(
		"truncate", "Truncate text to N chars with optional ellipsis. Usage: 'truncate 50: <text>'",
		[]string{"truncate"},
	)}
}
func (s *TruncateSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "truncate ")
}
func (s *TruncateSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	// Match "50:" or "50 chars:" or "50 chars to:" prefix.
	re := regexp.MustCompile(`^\s*(\d+)\s*(?:chars?|characters)?\s*(?::|to)?\s*`)
	m := re.FindStringSubmatch(payload)
	if m == nil {
		// Default to a sensible truncation length.
		const defaultLen = 80
		r := []rune(payload)
		if len(r) <= defaultLen {
			return payload, nil
		}
		return string(r[:defaultLen]) + "...", nil
	}
	n := 0
	_, err := fmt.Sscanf(m[1], "%d", &n)
	if err != nil || n < 0 {
		return "", fmt.Errorf("invalid truncation length")
	}
	rest := strings.TrimSpace(strings.TrimPrefix(payload, m[0]))
	r := []rune(rest)
	if len(r) <= n {
		return rest, nil
	}
	if n <= 3 {
		return string(r[:n]), nil
	}
	return string(r[:n-1]) + "…", nil
}

// ---- pad ----

type PadSkill struct{ *kyoci.BaseSkill }

func NewPadSkill() *PadSkill {
	return &PadSkill{BaseSkill: kyoci.NewBaseSkill(
		"pad", "Pad text to a width. Usage: 'pad left 10 0: 42' or 'pad right 20 : hi'",
		[]string{"pad text", "pad string", "pad left", "pad right", "pad center"},
	)}
}
func (s *PadSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.HasPrefix(q, "pad ") || strings.Contains(q, "pad left") ||
		strings.Contains(q, "pad right") || strings.Contains(q, "pad center")
}
func (s *PadSkill) Execute(_ context.Context, q string) (string, error) {
	low := strings.ToLower(q)
	side := "right"
	if strings.Contains(low, "left") {
		side = "left"
	} else if strings.Contains(low, "center") || strings.Contains(low, "centre") {
		side = "center"
	}
	payload := extractPayload(q)
	re := regexp.MustCompile(`^\s*(?:left|right|center)?\s*(\d+)\s*(\S)\s*:\s*(.+)$`)
	m := re.FindStringSubmatch(payload)
	if m == nil {
		return "", fmt.Errorf("expected format: 'pad [left|right|center] <width> <char>: <text>'")
	}
	width := 0
	_, _ = fmt.Sscanf(m[1], "%d", &width)
	padChar := " "
	if len(m[2]) > 0 {
		padChar = m[2]
	}
	text := m[3]
	if len([]rune(text)) >= width {
		return text, nil
	}
	needed := width - len([]rune(text))
	switch side {
	case "left":
		return strings.Repeat(padChar, needed) + text, nil
	case "right":
		return text + strings.Repeat(padChar, needed), nil
	case "center":
		left := needed / 2
		right := needed - left
		return strings.Repeat(padChar, left) + text + strings.Repeat(padChar, right), nil
	}
	return "", fmt.Errorf("unknown side")
}

// ---- reverse ----

type ReverseSkill struct{ *kyoci.BaseSkill }

func NewReverseSkill() *ReverseSkill {
	return &ReverseSkill{BaseSkill: kyoci.NewBaseSkill(
		"reverse", "Reverse the characters in text",
		[]string{"reverse", "reverse text", "reverse string"},
	)}
}
func (s *ReverseSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.HasPrefix(q, "reverse ") || strings.Contains(q, "reverse text") ||
		strings.Contains(q, "reverse string") || strings.Contains(q, "reverse the")
}
func (s *ReverseSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	if in == "" {
		return "", fmt.Errorf("no text to reverse")
	}
	r := []rune(in)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r), nil
}

// ---- sort_lines / dedupe_lines ----

type SortLinesSkill struct{ *kyoci.BaseSkill }

func NewSortLinesSkill() *SortLinesSkill {
	return &SortLinesSkill{BaseSkill: kyoci.NewBaseSkill(
		"sort_lines", "Sort lines alphabetically (case-insensitive)",
		[]string{"sort lines", "sort text", "alphabetize"},
	)}
}
func (s *SortLinesSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "sort lines") || strings.Contains(q, "sort text") ||
		strings.Contains(q, "alphabetize")
}
func (s *SortLinesSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	lines := strings.Split(in, "\n")
	sort.SliceStable(lines, func(i, j int) bool {
		return strings.ToLower(strings.TrimSpace(lines[i])) < strings.ToLower(strings.TrimSpace(lines[j]))
	})
	return strings.Join(lines, "\n"), nil
}

type DedupeLinesSkill struct{ *kyoci.BaseSkill }

func NewDedupeLinesSkill() *DedupeLinesSkill {
	return &DedupeLinesSkill{BaseSkill: kyoci.NewBaseSkill(
		"dedupe_lines", "Remove duplicate lines (preserves order)",
		[]string{"dedupe lines", "dedupe", "deduplicate", "remove duplicate lines", "unique lines"},
	)}
}
func (s *DedupeLinesSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "dedupe") || strings.Contains(q, "deduplicate") ||
		strings.Contains(q, "remove duplicate lines") || strings.Contains(q, "unique lines")
}
func (s *DedupeLinesSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(in, "\n") {
		key := strings.TrimSpace(line)
		if key == "" {
			out = append(out, line)
			continue
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n"), nil
}

// ---- indent / dedent ----

type IndentSkill struct {
	*kyoci.BaseSkill
	mode string // "indent" or "dedent"
}

func (s *IndentSkill) Match(q string) bool {
	q = strings.ToLower(q)
	if s.mode == "indent" {
		return strings.Contains(q, "indent ") && !strings.Contains(q, "indent level")
	}
	return strings.Contains(q, "dedent ") || strings.Contains(q, "unindent ")
}
func (s *IndentSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	// Optional numeric prefix: "indent 4: text" → 4 spaces
	re := regexp.MustCompile(`^\s*(\d+)\s*:\s*`)
	prefix := "    "
	if m := re.FindStringSubmatch(in); m != nil {
		n := 0
		_, _ = fmt.Sscanf(m[1], "%d", &n)
		prefix = strings.Repeat(" ", n)
		in = strings.TrimPrefix(in, m[0])
	}
	lines := strings.Split(in, "\n")
	if s.mode == "indent" {
		for i, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			lines[i] = prefix + line
		}
	} else {
		for i, line := range lines {
			// Strip up to len(prefix) leading spaces.
			stripped := strings.TrimLeft(line, " ")
			delta := len(line) - len(stripped)
			if delta > len(prefix) {
				delta = len(prefix)
			}
			lines[i] = strings.Repeat(" ", delta-len(prefix)) + stripped
			if strings.HasPrefix(lines[i], "-") {
				lines[i] = stripped
			}
		}
	}
	return strings.Join(lines, "\n"), nil
}

func NewIndentSkill() *IndentSkill {
	return &IndentSkill{BaseSkill: kyoci.NewBaseSkill(
		"indent", "Indent text by 4 spaces (or a custom count)", []string{"indent"}),
		mode: "indent",
	}
}
func NewDedentSkill() *IndentSkill {
	return &IndentSkill{BaseSkill: kyoci.NewBaseSkill(
		"dedent", "Remove up to 4 leading spaces from each line", []string{"dedent"}),
		mode: "dedent",
	}
}

// ---- regex_replace ----

type RegexReplaceSkill struct{ *kyoci.BaseSkill }

func NewRegexReplaceSkill() *RegexReplaceSkill {
	return &RegexReplaceSkill{BaseSkill: kyoci.NewBaseSkill(
		"regex_replace", "Replace regex matches. Usage: 'regex_replace /pattern/replacement/flags: text'",
		[]string{"regex replace", "regex_replace", "substitute regex", "re.sub"},
	)}
}
func (s *RegexReplaceSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "regex replace") || strings.Contains(q, "regex_replace") ||
		strings.Contains(q, "substitute regex") || strings.HasPrefix(q, "re.sub")
}
func (s *RegexReplaceSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	// Expect: /pattern/replacement/[flags]: text
	re := regexp.MustCompile(`^\s*/(.+?)/(.*)/[gim]*\s*:\s*([\s\S]+)$`)
	m := re.FindStringSubmatch(payload)
	if m == nil {
		// Fall back to pipe-separated: "pattern|replacement|text"
		parts := strings.SplitN(payload, "|", 3)
		if len(parts) != 3 {
			return "", fmt.Errorf("expected '/pattern/replacement/: text' or 'pattern|replacement|text'")
		}
		m = []string{"", parts[0], parts[1], parts[2]}
	}
	pattern, replacement, text := m[1], m[2], m[3]
	// Support $1-style backrefs (Go uses $ syntax). Convert \1 → $1.
	replacement = regexp.MustCompile(`\\(\d)`).ReplaceAllString(replacement, "$$$1")
	r, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex: %w", err)
	}
	return r.ReplaceAllString(text, replacement), nil
}

// =====================================================================================
// Generators — UUID variants, nanoid, GUID, random_int/string/bytes, nonce, fake data.
// =====================================================================================

// ---- uuid_v4 / uuid_v7 ----

type UUIDV4Skill struct{ *kyoci.BaseSkill }

func NewUUIDV4Skill() *UUIDV4Skill {
	return &UUIDV4Skill{BaseSkill: kyoci.NewBaseSkill(
		"uuid_v4", "Generate a v4 UUID (random)",
		[]string{"uuid v4", "uuidv4", "generate uuid v4", "random uuid"},
	)}
}
func (s *UUIDV4Skill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "uuid v4") || strings.Contains(q, "uuidv4") ||
		(strings.Contains(q, "generate uuid") && strings.Contains(q, "v4"))
}
func (s *UUIDV4Skill) Execute(_ context.Context, _ string) (string, error) {
	return randomUUIDv4(), nil
}

// randomUUIDv4 generates an RFC 4122 v4 UUID using crypto/rand.
func randomUUIDv4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fallback to a less-random but functional ID. crypto/rand failing
		// would be a critical system issue; better than returning "".
		for i := range b {
			b[i] = byte(i)
		}
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 1
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

type UUIDV7Skill struct{ *kyoci.BaseSkill }

func NewUUIDV7Skill() *UUIDV7Skill {
	return &UUIDV7Skill{BaseSkill: kyoci.NewBaseSkill(
		"uuid_v7", "Generate a v7 UUID (time-ordered, RFC 9562)",
		[]string{"uuid v7", "uuidv7", "generate uuid v7", "time-ordered uuid"},
	)}
}
func (s *UUIDV7Skill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "uuid v7") || strings.Contains(q, "uuidv7") ||
		strings.Contains(q, "time-ordered uuid")
}
func (s *UUIDV7Skill) Execute(_ context.Context, _ string) (string, error) {
	return randomUUIDv7(), nil
}

// randomUUIDv7 generates an RFC 9562 v7 UUID: 48 bits of unix-ms timestamp
// + 12 bits of rand_a + 62 bits of rand_b (with version/variant bits set).
// Time-ordered — naturally sortable, good for database keys.
func randomUUIDv7() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return randomUUIDv4() // graceful fallback
	}
	// Unix milliseconds (best-effort, no time package import wanted here for clarity).
	// We use a host-provided timestamp via the ctx — but for skills without one,
	// read /dev/urandom twice is fine. Just use UnixNano/1e6 inline.
	ms := uint64(nowUnixMillis())
	// High 48 bits = timestamp
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	// version 7 in high nibble of byte 6
	b[6] = (b[6] & 0x0f) | 0x70
	// variant in high bits of byte 8
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// nowUnixMillis is split out so tests can stub it. Returns Unix epoch ms.
// Implemented in time.go since time is already imported there.
// (Forward declaration; see time.go's nowUnixMillis.)
// (Now defined below the type for clarity. The skill file does not need to
// import the time package directly — that lives in time.go.)

// ---- nanoid ----

type NanoidSkill struct{ *kyoci.BaseSkill }

func NewNanoidSkill() *NanoidSkill {
	return &NanoidSkill{BaseSkill: kyoci.NewBaseSkill(
		"nanoid", "Generate a Nano ID (21 chars, URL-safe alphabet). Optional length: 'nanoid 32'",
		[]string{"nanoid", "nano id"},
	)}
}
func (s *NanoidSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "nanoid") || strings.Contains(q, "nano id")
}
func (s *NanoidSkill) Execute(_ context.Context, q string) (string, error) {
	size := 21
	low := strings.ToLower(strings.TrimSpace(q))
	if n := parseIntSuffix(low); n > 0 {
		size = n
	}
	const alphabet = "_-0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	max := big.NewInt(int64(len(alphabet)))
	var b strings.Builder
	for i := 0; i < size; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("rand failed: %w", err)
		}
		b.WriteByte(alphabet[idx.Int64()])
	}
	return b.String(), nil
}

// parseIntSuffix extracts a trailing integer from s ("nanoid 32" → 32).
// Returns 0 if no trailing integer is found.
func parseIntSuffix(s string) int {
	re := regexp.MustCompile(`\b(\d+)\s*$`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	n := 0
	_, _ = fmt.Sscanf(m[1], "%d", &n)
	return n
}

// ---- guid (Microsoft-style) ----

type GUIDSkill struct{ *kyoci.BaseSkill }

func NewGUIDSkill() *GUIDSkill {
	return &GUIDSkill{BaseSkill: kyoci.NewBaseSkill(
		"guid", "Generate a Microsoft-style GUID (braced, uppercase)",
		[]string{"guid", "microsoft guid", "braced guid"},
	)}
}
func (s *GUIDSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "guid") || strings.Contains(q, "microsoft uuid")
}
func (s *GUIDSkill) Execute(_ context.Context, _ string) (string, error) {
	id := randomUUIDv4()
	return fmt.Sprintf("{%s}", strings.ToUpper(id)), nil
}

// ---- random_int ----

type RandomIntSkill struct{ *kyoci.BaseSkill }

func NewRandomIntSkill() *RandomIntSkill {
	return &RandomIntSkill{BaseSkill: kyoci.NewBaseSkill(
		"random_int", "Generate a random integer in [min, max]. Usage: 'random_int 1-100'",
		[]string{"random int", "random integer", "rand int", "random number"},
	)}
}
func (s *RandomIntSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "random int") || strings.Contains(q, "random number") ||
		strings.Contains(q, "rand int")
}
func (s *RandomIntSkill) Execute(_ context.Context, q string) (string, error) {
	low := strings.ToLower(q)
	re := regexp.MustCompile(`(\d+)\s*[-to]+\s*(\d+)`)
	m := re.FindStringSubmatch(low)
	if m == nil {
		// Single integer = upper bound, 0 is lower.
		n := parseIntSuffix(low)
		if n <= 0 {
			n = 100
		}
		v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", v.Int64()), nil
	}
	min, max := 0, 0
	_, _ = fmt.Sscanf(m[1], "%d", &min)
	_, _ = fmt.Sscanf(m[2], "%d", &max)
	if min > max {
		min, max = max, min
	}
	span := big.NewInt(int64(max - min + 1))
	v, err := rand.Int(rand.Reader, span)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", int(v.Int64())+min), nil
}

// ---- random_string ----

type RandomStringSkill struct{ *kyoci.BaseSkill }

func NewRandomStringSkill() *RandomStringSkill {
	return &RandomStringSkill{BaseSkill: kyoci.NewBaseSkill(
		"random_string", "Generate a random string of N chars. Usage: 'random_string 16 alphanumeric'",
		[]string{"random string", "random chars", "random text"},
	)}
}
func (s *RandomStringSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "random string") || strings.Contains(q, "random chars") ||
		strings.Contains(q, "random text")
}
func (s *RandomStringSkill) Execute(_ context.Context, q string) (string, error) {
	low := strings.ToLower(q)
	size := 16
	if n := parseIntSuffix(low); n > 0 {
		size = n
	}
	alpha := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	if strings.Contains(low, "alpha") && !strings.Contains(low, "digit") {
		// alphabetic only
		alpha = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	} else if strings.Contains(low, "digit") && !strings.Contains(low, "alpha") {
		alpha = "0123456789"
	} else if strings.Contains(low, "lower") {
		alpha = "abcdefghijklmnopqrstuvwxyz"
	} else if strings.Contains(low, "upper") {
		alpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	} else if strings.Contains(low, "hex") {
		alpha = "0123456789abcdef"
	}
	max := big.NewInt(int64(len(alpha)))
	var b strings.Builder
	for i := 0; i < size; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b.WriteByte(alpha[idx.Int64()])
	}
	return b.String(), nil
}

// ---- random_bytes ----

type RandomBytesSkill struct{ *kyoci.BaseSkill }

func NewRandomBytesSkill() *RandomBytesSkill {
	return &RandomBytesSkill{BaseSkill: kyoci.NewBaseSkill(
		"random_bytes", "Generate N random bytes (hex-encoded). Usage: 'random_bytes 32'",
		[]string{"random bytes", "random hex", "rand bytes"},
	)}
}
func (s *RandomBytesSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "random bytes") || strings.Contains(q, "random hex")
}
func (s *RandomBytesSkill) Execute(_ context.Context, q string) (string, error) {
	n := 32
	if v := parseIntSuffix(strings.ToLower(q)); v > 0 {
		n = v
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

// ---- nonce ----

type NonceSkill struct{ *kyoci.BaseSkill }

func NewNonceSkill() *NonceSkill {
	return &NonceSkill{BaseSkill: kyoci.NewBaseSkill(
		"nonce", "Generate a cryptographic nonce (16 random bytes, base64URL)",
		[]string{"nonce", "crypto nonce"},
	)}
}
func (s *NonceSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "nonce")
}
func (s *NonceSkill) Execute(_ context.Context, _ string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64URLEncode(b), nil
}

// base64URLEncode is a local helper to avoid pulling encoding/base64 here
// (which would create a name clash with our base64 skill helpers elsewhere).
func base64URLEncode(b []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	var out strings.Builder
	for i := 0; i < len(b); i += 3 {
		end := i + 3
		if end > len(b) {
			end = len(b)
		}
		chunk := b[i:end]
		var n uint
		for j := 0; j < 3; j++ {
			n <<= 8
			if j < len(chunk) {
				n |= uint(chunk[j])
			}
		}
		out.WriteByte(alphabet[(n>>18)&0x3f])
		out.WriteByte(alphabet[(n>>12)&0x3f])
		if len(chunk) > 1 {
			out.WriteByte(alphabet[(n>>6)&0x3f])
		}
		if len(chunk) > 2 {
			out.WriteByte(alphabet[n&0x3f])
		}
	}
	return out.String()
}

// ---- fake_name / fake_email ----

type FakeNameSkill struct{ *kyoci.BaseSkill }

func NewFakeNameSkill() *FakeNameSkill {
	return &FakeNameSkill{BaseSkill: kyoci.NewBaseSkill(
		"fake_name", "Generate a fake person name (first + last)",
		[]string{"fake name", "random name", "fake person"},
	)}
}
func (s *FakeNameSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "fake name") || strings.Contains(q, "random name") ||
		strings.Contains(q, "fake person")
}
func (s *FakeNameSkill) Execute(_ context.Context, _ string) (string, error) {
	firsts := []string{"Alex", "Jordan", "Taylor", "Morgan", "Casey", "Riley", "Quinn", "Avery", "Skyler", "Drew"}
	lasts := []string{"Smith", "Johnson", "Patel", "Garcia", "Nguyen", "Kim", "Brown", "Jones", "Davis", "Miller"}
	fi, _ := rand.Int(rand.Reader, big.NewInt(int64(len(firsts))))
	li, _ := rand.Int(rand.Reader, big.NewInt(int64(len(lasts))))
	return firsts[fi.Int64()] + " " + lasts[li.Int64()], nil
}

type FakeEmailSkill struct{ *kyoci.BaseSkill }

func NewFakeEmailSkill() *FakeEmailSkill {
	return &FakeEmailSkill{BaseSkill: kyoci.NewBaseSkill(
		"fake_email", "Generate a fake email address",
		[]string{"fake email", "random email", "fake address"},
	)}
}
func (s *FakeEmailSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "fake email") || strings.Contains(q, "random email")
}
func (s *FakeEmailSkill) Execute(_ context.Context, _ string) (string, error) {
	users := []string{"alice", "bob", "carol", "dave", "eve", "frank", "grace", "heidi"}
	domains := []string{"example.com", "test.org", "mail.dev", "fake.io"}
	ui, _ := rand.Int(rand.Reader, big.NewInt(int64(len(users))))
	di, _ := rand.Int(rand.Reader, big.NewInt(int64(len(domains))))
	return users[ui.Int64()] + "@" + domains[di.Int64()], nil
}
