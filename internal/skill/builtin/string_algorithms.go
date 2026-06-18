package builtin

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// String-algorithm skills — soundex, metaphone, jaro, jaro_winkler, hamming,
// lcs, lcs_substr, ngram, ngram_frequency, ratcliff_obershelp.
//
// All pure Go, stdlib only — no LLM/network/side effects.
// =====================================================================================

// ---- shared helpers for two-operand skills ----

// extractPayloadStrict is like extractPayload but treats a present colon as
// authoritative: if the query contains a colon, the operand is exactly the
// (trimmed) text after the FIRST colon — empty if nothing follows. This avoids
// extractPayload's fallback that returns the whole query when the post-colon
// text is empty (which would otherwise feed the verb back to the algorithm).
func extractPayloadStrict(q string) string {
	q = strings.TrimSpace(q)
	if idx := strings.Index(q, ":"); idx >= 0 {
		return strings.TrimSpace(q[idx+1:])
	}
	return extractPayload(q)
}

// splitPair splits a payload into two operands, supporting "|" or "," as the
// separator. Operands are trimmed of whitespace and surrounding quotes.
// Returns ok=false if exactly two parts are not produced.
func splitPair(payload string) (string, string, bool) {
	parts := strings.SplitN(payload, "|", 2)
	if len(parts) != 2 {
		parts = strings.SplitN(payload, ",", 2)
	}
	if len(parts) != 2 {
		return "", "", false
	}
	a := quoteStripped(strings.TrimSpace(parts[0]))
	b := quoteStripped(strings.TrimSpace(parts[1]))
	return a, b, true
}

// extractSingleWord pulls a single-word operand out of a query like
// "soundex of robert" or "soundex: robert". Returns the trimmed word, or ""
// if no operand is present (e.g. bare "soundex:").
func extractSingleWord(q string) string {
	// If the query contains a colon, the operand is whatever follows it —
	// even if extractPayload would fall through to its strippers and return
	// the whole query when the post-colon text is empty.
	if idx := strings.Index(q, ":"); idx >= 0 {
		rest := strings.TrimSpace(q[idx+1:])
		if rest == "" {
			return ""
		}
		if fields := strings.Fields(rest); len(fields) > 0 {
			return quoteStripped(fields[0])
		}
		return ""
	}
	// "verb of WORD" / "verb for WORD" shapes.
	if in := extractAfter(q, " of ", " for "); in != "" {
		if fields := strings.Fields(in); len(fields) > 0 {
			return quoteStripped(fields[0])
		}
	}
	in := strings.TrimSpace(extractPayload(q))
	if fields := strings.Fields(in); len(fields) > 0 {
		return quoteStripped(fields[0])
	}
	return ""
}

// formatFloat3 formats a similarity float with three decimal digits
// (always 3 places) — e.g. 0.833. Clamps to [0,1]. Returns "0.000" for
// non-finite values.
func formatFloat3(v float64) string {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return strconv.FormatFloat(v, 'f', 3, 64)
}

// ---- soundex ----

// soundex computes the American Soundex code (Letter + 3 digits) of a word.
// Standard rules: B,F,P,V→1; C,G,J,K,Q,S,X,Z→2; D,T→3; L→4; M,N→5; R→6;
// vowels (A,E,I,O,U,Y) are dropped and reset the adjacency window; H,W are
// dropped but are transparent (do NOT reset) so two same-coded letters
// separated only by H or W collapse. Adjacent same-coded letters collapse to
// one code. Output is zero-padded to 4 chars and truncated to 4. Empty input
// returns "0000".
func soundex(word string) string {
	word = strings.ToUpper(strings.TrimSpace(word))
	if word == "" {
		return "0000"
	}
	// Keep only A-Z.
	var letters []rune
	for _, r := range word {
		if r >= 'A' && r <= 'Z' {
			letters = append(letters, r)
		}
	}
	if len(letters) == 0 {
		return "0000"
	}
	codeOf := func(r rune) byte {
		switch r {
		case 'B', 'F', 'P', 'V':
			return '1'
		case 'C', 'G', 'J', 'K', 'Q', 'S', 'X', 'Z':
			return '2'
		case 'D', 'T':
			return '3'
		case 'L':
			return '4'
		case 'M', 'N':
			return '5'
		case 'R':
			return '6'
		}
		return '0' // vowels + H,W,Y
	}
	var b strings.Builder
	// First letter is always kept.
	b.WriteRune(letters[0])
	prev := codeOf(letters[0])
	// Soundex's "first letter" rule: the first letter's code is NOT emitted
	// even if it would have one, so a leading coded letter suppresses an
	// adjacent same-coded letter (e.g. "Pfister" → P236 not P123).
	for _, r := range letters[1:] {
		// H and W are transparent: they don't emit and DON'T reset prev, so
		// two same-coded letters separated only by H/W collapse (e.g.
		// "Ashcraft" → A261, where S-H-C collapse to a single 2).
		if r == 'H' || r == 'W' {
			continue
		}
		c := codeOf(r)
		if c == '0' {
			// vowel or Y — resets adjacency but doesn't emit.
			prev = '0'
			continue
		}
		if c != prev {
			b.WriteByte(c)
		}
		prev = c
		if b.Len() == 4 {
			break
		}
	}
	out := b.String()
	// Pad with zeros, then truncate to 4.
	for len(out) < 4 {
		out += "0"
	}
	return out[:4]
}

type SoundexSkill struct{ *kyoci.BaseSkill }

func NewSoundexSkill() *SoundexSkill {
	return &SoundexSkill{BaseSkill: kyoci.NewBaseSkill(
		"soundex", "American Soundex phonetic code (4-char, e.g. R163 for Robert). Input: single word",
		[]string{"soundex", "phonetic code"},
	)}
}
func (s *SoundexSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "soundex")
}
func (s *SoundexSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractSingleWord(q)
	if in == "" {
		return "", fmt.Errorf("no word to encode")
	}
	return soundex(in), nil
}

// ---- metaphone (simplified) ----

// metaphone computes a simplified Metaphone phonetic key — a consonant
// skeleton with the classic letter-cluster substitutions:
//   - initial: KN→N, GN→N, PN→N, AE→E, WR→R, PS→S, X→S (initial), WH→W (initial)
//   - X→KS (non-initial)
//   - CK→K, PH→F, TH→0 (theta), SH→X, SCH→SK, CIA→X, CH→X, TIA/TIO→X
//   - C: front-of-E,I,Y → S; else → K. Drop silent C in SCI/SCE/SCY → S.
//   - DG → J before I,E,Y, else T
//   - D → T (simplified), G → J before I,E,Y else K, GH→ (silent/F)
//   - vowels: keep only when leading (leading vowel is kept)
//   - B: silent after M at end
//   - drop trailing vowels
//
// This is intentionally a "simplified" Metaphone (no full rule set) — it
// preserves the consonant skeleton with the most common substitutions,
// sufficient for fuzzy-match use cases.
func metaphone(word string) string {
	word = strings.ToUpper(strings.TrimSpace(word))
	if word == "" {
		return ""
	}
	var w []byte
	for i := 0; i < len(word); i++ {
		c := word[i]
		if c >= 'A' && c <= 'Z' {
			w = append(w, c)
		}
	}
	if len(w) == 0 {
		return ""
	}
	isVowel := func(b byte) bool {
		return b == 'A' || b == 'E' || b == 'I' || b == 'O' || b == 'U'
	}
	// Pre-apply common initial transformations.
	switch {
	case strings.HasPrefix(string(w), "KN") || strings.HasPrefix(string(w), "GN") || strings.HasPrefix(string(w), "PN"):
		w = w[1:]
	case strings.HasPrefix(string(w), "AE"):
		w = append([]byte{'E'}, w[2:]...)
	case strings.HasPrefix(string(w), "WR"):
		w = w[1:]
	case strings.HasPrefix(string(w), "PS"):
		w = w[1:]
	case len(w) == 1 && w[0] == 'X':
		w = []byte{'S'}
	case strings.HasPrefix(string(w), "X"):
		w[0] = 'S'
	case strings.HasPrefix(string(w), "WH"):
		w[0] = 'W'
		w = append(w[:1], w[2:]...)
	}

	var b strings.Builder
	n := len(w)
	for i := 0; i < n; i++ {
		c := w[i]
		prev := byte(0)
		if i > 0 {
			prev = w[i-1]
		}
		next := byte(0)
		if i+1 < n {
			next = w[i+1]
		}
		next2 := byte(0)
		if i+2 < n {
			next2 = w[i+2]
		}
		emit := byte(0)
		switch c {
		case 'A', 'E', 'I', 'O', 'U':
			if i == 0 {
				emit = c
			}
			// else: drop (silent in non-initial position)
		case 'B':
			if !(prev == 'M' && i == n-1) { // silent B after M at end
				emit = 'B'
			}
		case 'C':
			if next == 'I' && next2 == 'A' {
				emit = 'X' // -CIA-
				i += 2
			} else if next == 'H' {
				emit = 'X' // CH
				i++
			} else if prev == 'S' && (next == 'I' || next == 'E' || next == 'Y') {
				// SC[I,E,Y] → already emitted S, drop the C silently.
				emit = 0
			} else if next == 'I' || next == 'E' || next == 'Y' {
				emit = 'S'
			} else {
				emit = 'K'
			}
		case 'D':
			if next == 'G' && (next2 == 'I' || next2 == 'E' || next2 == 'Y') {
				emit = 'J' // DGE/DGI/DGY
				i += 2
			} else {
				emit = 'T'
			}
		case 'F':
			emit = 'F'
		case 'G':
			if i == n-1 && next == 0 {
				if prev == 'H' {
					emit = 0
				} else {
					emit = 'K'
				}
			} else if next == 'H' {
				if isVowel(next2) {
					emit = 'F'
					i++
				} else {
					emit = 0
					i++
				}
			} else if next == 'N' && i == 0 {
				emit = 0
			} else if next == 'I' || next == 'E' || next == 'Y' {
				emit = 'J'
			} else {
				emit = 'K'
			}
		case 'H':
			if isVowel(prev) && !isVowel(next) {
				emit = 0
			} else {
				emit = 'H'
			}
		case 'J':
			emit = 'J'
		case 'K':
			if prev == 'C' {
				emit = 0 // CK: C already emitted K, drop the K
			} else {
				emit = 'K'
			}
		case 'L':
			emit = 'L'
		case 'M':
			emit = 'M'
		case 'N':
			emit = 'N'
		case 'P':
			if next == 'H' {
				emit = 'F'
				i++
			} else {
				emit = 'P'
			}
		case 'Q':
			emit = 'K'
		case 'R':
			emit = 'R'
		case 'S':
			if next == 'H' {
				emit = 'X'
				i++
			} else if next == 'I' && (next2 == 'A' || next2 == 'O') {
				emit = 'X' // SIA/SIO
				i++
			} else if next == 'C' && (next2 == 'I' || next2 == 'E' || next2 == 'Y') {
				emit = 'X' // SC[I,E,Y] → X
				i += 2
			} else {
				emit = 'S'
			}
		case 'T':
			if next == 'I' && (next2 == 'A' || next2 == 'O') {
				emit = 'X' // TIA/TIO
				i++
			} else if next == 'H' {
				emit = '0' // TH as in "think"
				i++
			} else if next == 'C' && next2 == 'H' {
				emit = 'X' // TCH
				i += 2
			} else {
				emit = 'T'
			}
		case 'V':
			emit = 'F'
		case 'W', 'Y':
			if i == 0 && isVowel(next) {
				emit = c
			}
		case 'X':
			emit = 'K' // X → KS simplified to K (S handled by adjacency drop)
		case 'Z':
			emit = 'S'
		default:
			emit = c
		}
		if emit != 0 {
			b.WriteByte(emit)
		}
	}
	return b.String()
}

type MetaphoneSkill struct{ *kyoci.BaseSkill }

func NewMetaphoneSkill() *MetaphoneSkill {
	return &MetaphoneSkill{BaseSkill: kyoci.NewBaseSkill(
		"metaphone", "Metaphone phonetic key (simplified consonant skeleton). Input: single word",
		[]string{"metaphone", "metaphone key"},
	)}
}
func (s *MetaphoneSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "metaphone")
}
func (s *MetaphoneSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractSingleWord(q)
	if in == "" {
		return "", fmt.Errorf("no word to encode")
	}
	return metaphone(in), nil
}

// ---- jaro ----

// jaro computes the Jaro similarity in [0.0, 1.0] between two strings.
// Two characters match if they are the same AND within
// floor(max(|s1|,|s2|)/2)-1 positions of each other. Transpositions are
// counted and divided by 2.
func jaro(s1, s2 string) float64 {
	r1 := []rune(s1)
	r2 := []rune(s2)
	if len(r1) == 0 && len(r2) == 0 {
		return 1.0
	}
	if len(r1) == 0 || len(r2) == 0 {
		return 0.0
	}
	if s1 == s2 {
		return 1.0
	}
	matchWindow := (max(len(r1), len(r2))/2 - 1)
	if matchWindow < 0 {
		matchWindow = 0
	}
	m1 := make([]bool, len(r1))
	m2 := make([]bool, len(r2))
	matches := 0
	for i := 0; i < len(r1); i++ {
		start := i - matchWindow
		if start < 0 {
			start = 0
		}
		end := i + matchWindow + 1
		if end > len(r2) {
			end = len(r2)
		}
		for j := start; j < end; j++ {
			if m2[j] || r1[i] != r2[j] {
				continue
			}
			m1[i] = true
			m2[j] = true
			matches++
			break
		}
	}
	if matches == 0 {
		return 0.0
	}
	k := 0
	transpositions := 0
	for i := 0; i < len(r1); i++ {
		if !m1[i] {
			continue
		}
		for !m2[k] {
			k++
		}
		if r1[i] != r2[k] {
			transpositions++
		}
		k++
	}
	transpositions /= 2
	m := float64(matches)
	return (m/float64(len(r1)) + m/float64(len(r2)) + (m-float64(transpositions))/m) / 3.0
}

type JaroSkill struct{ *kyoci.BaseSkill }

func NewJaroSkill() *JaroSkill {
	return &JaroSkill{BaseSkill: kyoci.NewBaseSkill(
		"jaro", "Jaro similarity (0.0-1.0) between two strings. Input: 'jaro: s1, s2' or 's1 | s2'",
		[]string{"jaro similarity", "jaro distance"},
	)}
}
func (s *JaroSkill) Match(q string) bool {
	low := strings.ToLower(q)
	if strings.Contains(low, "jaro-winkler") || strings.Contains(low, "jaro_winkler") {
		return false
	}
	return strings.Contains(low, "jaro")
}
func (s *JaroSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	if payload == "" {
		return "", fmt.Errorf("expected two strings: 'jaro: s1, s2'")
	}
	a, b, ok := splitPair(payload)
	if !ok {
		return "", fmt.Errorf("expected two strings separated by ',' or '|'")
	}
	return formatFloat3(jaro(a, b)), nil
}

// ---- jaro_winkler ----

// jaroWinkler computes Jaro-Winkler similarity = jaro + l*p*(1-jaro), where l
// is the length of the common prefix (up to 4) and p is the scaling factor
// (default 0.1).
func jaroWinkler(s1, s2 string) float64 {
	j := jaro(s1, s2)
	prefix := 0
	r1 := []rune(s1)
	r2 := []rune(s2)
	for i := 0; i < len(r1) && i < len(r2) && i < 4; i++ {
		if r1[i] != r2[i] {
			break
		}
		prefix++
	}
	return j + float64(prefix)*0.1*(1.0-j)
}

type JaroWinklerSkill struct{ *kyoci.BaseSkill }

func NewJaroWinklerSkill() *JaroWinklerSkill {
	return &JaroWinklerSkill{BaseSkill: kyoci.NewBaseSkill(
		"jaro_winkler", "Jaro-Winkler similarity (with prefix bonus) between two strings. Input: 'jaro_winkler: s1, s2'",
		[]string{"jaro-winkler", "jaro_winkler"},
	)}
}
func (s *JaroWinklerSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "jaro-winkler") || strings.Contains(low, "jaro_winkler")
}
func (s *JaroWinklerSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	if payload == "" {
		return "", fmt.Errorf("expected two strings: 'jaro_winkler: s1, s2'")
	}
	a, b, ok := splitPair(payload)
	if !ok {
		return "", fmt.Errorf("expected two strings separated by ',' or '|'")
	}
	return formatFloat3(jaroWinkler(a, b)), nil
}

// ---- hamming_distance ----

// hammingDistance returns the Hamming distance between two equal-length rune
// strings. Returns -1 as a sentinel if lengths differ (caller must guard).
func hammingDistance(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	if len(ra) != len(rb) {
		return -1
	}
	d := 0
	for i := range ra {
		if ra[i] != rb[i] {
			d++
		}
	}
	return d
}

type HammingDistanceSkill struct{ *kyoci.BaseSkill }

func NewHammingDistanceSkill() *HammingDistanceSkill {
	return &HammingDistanceSkill{BaseSkill: kyoci.NewBaseSkill(
		"hamming_distance", "Hamming distance between two equal-length strings. Input: 'hamming: s1, s2'",
		[]string{"hamming distance", "hamming"},
	)}
}
func (s *HammingDistanceSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "hamming")
}
func (s *HammingDistanceSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	if payload == "" {
		return "", fmt.Errorf("expected two equal-length strings: 'hamming: s1, s2'")
	}
	a, b, ok := splitPair(payload)
	if !ok {
		return "", fmt.Errorf("expected two strings separated by ',' or '|'")
	}
	if len([]rune(a)) != len([]rune(b)) {
		return "", fmt.Errorf("hamming distance requires equal-length strings (got %d vs %d)", len([]rune(a)), len([]rune(b)))
	}
	return strconv.Itoa(hammingDistance(a, b)), nil
}

// ---- lcs (longest common subsequence) ----

// longestCommonSubsequence returns one actual LCS of s1 and s2 (not just its
// length). Builds the full DP table, then backtracks. Returns "" if either
// input is empty.
func longestCommonSubsequence(s1, s2 string) (string, int) {
	r1 := []rune(s1)
	r2 := []rune(s2)
	n, m := len(r1), len(r2)
	if n == 0 || m == 0 {
		return "", 0
	}
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if r1[i-1] == r2[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	var rev []rune
	i, j := n, m
	for i > 0 && j > 0 {
		if r1[i-1] == r2[j-1] {
			rev = append(rev, r1[i-1])
			i--
			j--
		} else if dp[i-1][j] >= dp[i][j-1] {
			i--
		} else {
			j--
		}
	}
	for l, r := 0, len(rev)-1; l < r; l, r = l+1, r-1 {
		rev[l], rev[r] = rev[r], rev[l]
	}
	return string(rev), dp[n][m]
}

type LCSSkill struct{ *kyoci.BaseSkill }

func NewLCSSkill() *LCSSkill {
	return &LCSSkill{BaseSkill: kyoci.NewBaseSkill(
		"lcs", "Longest common subsequence (length + the actual subsequence). Input: 'lcs: s1, s2'",
		[]string{"lcs", "longest common subsequence"},
	)}
}
func (s *LCSSkill) Match(q string) bool {
	low := strings.ToLower(q)
	// Reject the substring variant — handled by LCSSubstrSkill.
	if strings.Contains(low, "lcs substring") || strings.Contains(low, "lcs_substr") ||
		strings.Contains(low, "longest common substring") {
		return false
	}
	return strings.Contains(low, "longest common subsequence") ||
		strings.Contains(low, "lcs ") || strings.Contains(low, "lcs:") ||
		strings.Contains(low, "lcs of") || strings.Contains(low, "lcs between")
}
func (s *LCSSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	if payload == "" {
		return "", fmt.Errorf("expected two strings: 'lcs: s1, s2'")
	}
	a, b, ok := splitPair(payload)
	if !ok {
		return "", fmt.Errorf("expected two strings separated by ',' or '|'")
	}
	sub, n := longestCommonSubsequence(a, b)
	if sub == "" {
		return "length: 0\nsubsequence: (none)", nil
	}
	return fmt.Sprintf("length: %d\nsubsequence: %s", n, sub), nil
}

// ---- lcs_substr (longest common substring) ----

// longestCommonSubstring returns one of the longest common substrings of s1
// and s2 along with its length. Uses a single-row DP table to track the
// longest suffix match ending at each position.
func longestCommonSubstring(s1, s2 string) (string, int) {
	r1 := []rune(s1)
	r2 := []rune(s2)
	n, m := len(r1), len(r2)
	if n == 0 || m == 0 {
		return "", 0
	}
	dp := make([]int, m+1)
	bestLen := 0
	bestEndI := 0
	for i := 1; i <= n; i++ {
		for j := m; j >= 1; j-- {
			if r1[i-1] == r2[j-1] {
				dp[j] = dp[j-1] + 1
				if dp[j] > bestLen {
					bestLen = dp[j]
					bestEndI = i
				}
			} else {
				dp[j] = 0
			}
		}
	}
	if bestLen == 0 {
		return "", 0
	}
	start := bestEndI - bestLen
	return string(r1[start:bestEndI]), bestLen
}

type LCSSubstrSkill struct{ *kyoci.BaseSkill }

func NewLCSSubstrSkill() *LCSSubstrSkill {
	return &LCSSubstrSkill{BaseSkill: kyoci.NewBaseSkill(
		"lcs_substr", "Longest common substring (length + actual substring). Input: 'lcs_substr: s1, s2'",
		[]string{"lcs substring", "lcs_substr", "longest common substring"},
	)}
}
func (s *LCSSubstrSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "lcs substring") || strings.Contains(low, "lcs_substr") ||
		strings.Contains(low, "longest common substring") ||
		strings.Contains(low, "longest substring") || strings.Contains(low, "common substring")
}
func (s *LCSSubstrSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	if payload == "" {
		return "", fmt.Errorf("expected two strings: 'lcs_substr: s1, s2'")
	}
	a, b, ok := splitPair(payload)
	if !ok {
		return "", fmt.Errorf("expected two strings separated by ',' or '|'")
	}
	sub, n := longestCommonSubstring(a, b)
	if sub == "" {
		return "length: 0\nsubstring: (none)", nil
	}
	return fmt.Sprintf("length: %d\nsubstring: %s", n, sub), nil
}

// ---- ngram ----

// ngrams returns all n-grams of length n from s. Operates on runes so UTF-8
// is handled correctly. Returns an empty slice if n <= 0 or len(runes) < n.
func ngrams(s string, n int) []string {
	r := []rune(s)
	if n <= 0 || len(r) < n {
		return nil
	}
	out := make([]string, 0, len(r)-n+1)
	for i := 0; i+n <= len(r); i++ {
		out = append(out, string(r[i:i+n]))
	}
	return out
}

// ngramSizeRe matches the leading "ngram N:" / "n-gram N:" / "ngram: N" shapes
// to extract the gram size N. Returns 0 if no N is present.
var ngramSizeRe = regexp.MustCompile(`(?i)^(?:n[\s_-]?gram|n-gram)\s*(\d+)?\s*:`)

// parseNgramSize extracts a leading integer N from "ngram 3:" / "ngram3:" /
// "3-gram:" shapes. Returns 0 if none found.
func parseNgramSize(q string) int {
	m := ngramSizeRe.FindStringSubmatch(q)
	if m != nil && m[1] != "" {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	// Also accept leading "3-gram of:" form.
	re2 := regexp.MustCompile(`(?i)^(\d+)[\s_-]?gram`)
	if mm := re2.FindStringSubmatch(q); mm != nil {
		n, _ := strconv.Atoi(mm[1])
		return n
	}
	return 0
}

type NgramSkill struct{ *kyoci.BaseSkill }

func NewNgramSkill() *NgramSkill {
	return &NgramSkill{BaseSkill: kyoci.NewBaseSkill(
		"ngram", "Generate n-grams of given size. Usage: 'ngram 3: hello world' (default n=2)",
		[]string{"ngram", "n-gram"},
	)}
}
func (s *NgramSkill) Match(q string) bool {
	low := strings.ToLower(q)
	if strings.Contains(low, "ngram frequency") || strings.Contains(low, "n-gram frequency") ||
		strings.Contains(low, "ngram_frequency") {
		return false
	}
	return strings.Contains(low, "ngram") || strings.Contains(low, "n-gram")
}
func (s *NgramSkill) Execute(_ context.Context, q string) (string, error) {
	n := parseNgramSize(q)
	if n <= 0 {
		n = 2
	}
	payload := extractPayloadStrict(q)
	if payload == "" {
		return "", fmt.Errorf("no text to generate n-grams from")
	}
	grams := ngrams(payload, n)
	if len(grams) == 0 {
		return "", fmt.Errorf("text is shorter than n=%d", n)
	}
	return fmt.Sprintf("n=%d, count=%d\n%s", n, len(grams), strings.Join(grams, ", ")), nil
}

// ---- ngram_frequency ----

type NgramFrequencySkill struct{ *kyoci.BaseSkill }

func NewNgramFrequencySkill() *NgramFrequencySkill {
	return &NgramFrequencySkill{BaseSkill: kyoci.NewBaseSkill(
		"ngram_frequency", "Frequency table of n-grams (sorted desc). Usage: 'ngram_frequency 3: hello world'",
		[]string{"ngram frequency", "n-gram frequency", "ngram_frequency"},
	)}
}
func (s *NgramFrequencySkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "ngram frequency") || strings.Contains(low, "n-gram frequency") ||
		strings.Contains(low, "ngram_frequency")
}
func (s *NgramFrequencySkill) Execute(_ context.Context, q string) (string, error) {
	n := parseNgramSize(q)
	if n <= 0 {
		n = 2
	}
	payload := extractPayloadStrict(q)
	if payload == "" {
		return "", fmt.Errorf("no text to generate n-grams from")
	}
	grams := ngrams(payload, n)
	if len(grams) == 0 {
		return "", fmt.Errorf("text is shorter than n=%d", n)
	}
	freq := map[string]int{}
	for _, g := range grams {
		freq[g]++
	}
	type entry struct {
		gram string
		n    int
	}
	entries := make([]entry, 0, len(freq))
	for g, c := range freq {
		entries = append(entries, entry{g, c})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].n != entries[j].n {
			return entries[i].n > entries[j].n
		}
		return entries[i].gram < entries[j].gram
	})
	var b strings.Builder
	fmt.Fprintf(&b, "n=%d, unique=%d, total=%d\n", n, len(entries), len(grams))
	for _, e := range entries {
		fmt.Fprintf(&b, "%s: %d\n", e.gram, e.n)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// ---- ratcliff_obershelp ----

// ratcliffObershelp computes the Ratcliff-Obershelp pattern-matching
// similarity: 2 * (sum of lengths of all matched characters) / (len(s1) + len(s2)),
// where matched characters come from recursively finding the longest common
// substrings of the two inputs (and the segments left/right of each match).
func ratcliffObershelp(s1, s2 string) float64 {
	r1 := []rune(s1)
	r2 := []rune(s2)
	if len(r1) == 0 && len(r2) == 0 {
		return 1.0
	}
	if len(r1) == 0 || len(r2) == 0 {
		return 0.0
	}
	matched := ratcliffMatches(r1, r2)
	return 2.0 * float64(matched) / float64(len(r1)+len(r2))
}

// ratcliffMatches recursively sums the lengths of longest common substrings
// between the two rune slices and the segments adjacent to each match.
func ratcliffMatches(a, b []rune) int {
	sub, n := longestCommonSubstringRune(a, b)
	if n == 0 {
		return 0
	}
	aIdx := indexRuneSlice(a, sub)
	bIdx := indexRuneSlice(b, sub)
	matched := n
	if aIdx > 0 && bIdx > 0 {
		matched += ratcliffMatches(a[:aIdx], b[:bIdx])
	}
	if aIdx+n < len(a) && bIdx+n < len(b) {
		matched += ratcliffMatches(a[aIdx+n:], b[bIdx+n:])
	}
	return matched
}

// longestCommonSubstringRune is the rune-slice variant of
// longestCommonSubstring — used by ratcliff to avoid string<->[]rune
// round-trips on every recursion.
func longestCommonSubstringRune(a, b []rune) ([]rune, int) {
	n, m := len(a), len(b)
	if n == 0 || m == 0 {
		return nil, 0
	}
	dp := make([]int, m+1)
	bestLen := 0
	bestEnd := 0
	for i := 1; i <= n; i++ {
		for j := m; j >= 1; j-- {
			if a[i-1] == b[j-1] {
				dp[j] = dp[j-1] + 1
				if dp[j] > bestLen {
					bestLen = dp[j]
					bestEnd = i
				}
			} else {
				dp[j] = 0
			}
		}
	}
	if bestLen == 0 {
		return nil, 0
	}
	start := bestEnd - bestLen
	return a[start:bestEnd], bestLen
}

// indexRuneSlice returns the index of the first occurrence of needle in hay,
// or -1 if not present. Both are rune slices.
func indexRuneSlice(hay, needle []rune) int {
	if len(needle) == 0 {
		return 0
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if hay[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

type RatcliffObershelpSkill struct{ *kyoci.BaseSkill }

func NewRatcliffObershelpSkill() *RatcliffObershelpSkill {
	return &RatcliffObershelpSkill{BaseSkill: kyoci.NewBaseSkill(
		"ratcliff_obershelp", "Ratcliff-Obershelp similarity (0.0-1.0) between two strings. Input: 'ratcliff: s1, s2'",
		[]string{"ratcliff", "obershelp", "ratcliff obershelp"},
	)}
}
func (s *RatcliffObershelpSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "ratcliff") || strings.Contains(low, "obershelp")
}
func (s *RatcliffObershelpSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	if payload == "" {
		return "", fmt.Errorf("expected two strings: 'ratcliff: s1, s2'")
	}
	a, b, ok := splitPair(payload)
	if !ok {
		return "", fmt.Errorf("expected two strings separated by ',' or '|'")
	}
	return formatFloat3(ratcliffObershelp(a, b)), nil
}
