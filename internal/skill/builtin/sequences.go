package builtin

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Number-sequence skills — range, fibonacci, arithmetic/geometric progressions,
// primes (Sieve of Eratosthenes), and the Collatz conjecture sequence.
//
// All are pure Go, stdlib-only, and deterministic. No LLM, no network. Counts are
// capped to prevent runaway output.
// =====================================================================================

const (
	// sequenceMaxCount caps the number of terms any single skill will emit, so a
	// fat-fingered "fibonacci: 1000000000" can't OOM the agent.
	sequenceMaxCount = 1000
	// sequenceLineBreakThreshold switches from space-separated to newline-separated
	// output once a sequence gets long enough that a single line becomes unreadable.
	sequenceLineBreakThreshold = 20
)

// formatSequence joins the given stringified terms with a separator chosen by length:
// space for short sequences, newline for longer ones.
func formatSequence(terms []string) string {
	if len(terms) == 0 {
		return ""
	}
	sep := " "
	if len(terms) > sequenceLineBreakThreshold {
		sep = "\n"
	}
	return strings.Join(terms, sep)
}

// parseIntStrict parses s as a base-10 integer. Returns an error on malformed input
// (unlike fmt.Sscanf which silently returns 0). We want explicit failure here so
// callers can surface a useful error message to the user.
func parseIntStrict(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty integer")
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q: %w", s, err)
	}
	return n, nil
}

// pickFirstInt extracts the first base-10 integer token from s. Useful for
// payloads like "first 10 terms" or "up to 20 please".
func pickFirstInt(s string) (int, error) {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= '0' && r <= '9') && r != '-'
	})
	for _, f := range fields {
		if n, err := strconv.Atoi(strings.TrimPrefix(f, "-")); err == nil {
			if strings.HasPrefix(f, "-") {
				return -n, nil
			}
			return n, nil
		}
	}
	return 0, fmt.Errorf("no integer found in %q", s)
}

// ---- range ----

// RangeSkill generates an integer range [start, stop] with an optional step.
// Input forms (case-insensitive):
//   - "range: 1 10"        → 1..10 inclusive, step 1
//   - "range: 1 10 2"      → 1,3,5,7,9 (step 2)
//   - "range from 1 to 10" → 1..10 inclusive, step 1
type RangeSkill struct{ *kyoci.BaseSkill }

func NewRangeSkill() *RangeSkill {
	return &RangeSkill{BaseSkill: kyoci.NewBaseSkill(
		"range", "Generate a number range [start stop] or [start stop step]",
		[]string{"range", "range from", "generate range", "number range"},
	)}
}

func (s *RangeSkill) Match(q string) bool {
	low := strings.ToLower(q)
	// Match "range:" or "range " followed (eventually) by a digit, as well as
	// the natural-language "generate range" / "range from" phrasings. We avoid
	// matching bare "range" since that's also a Go keyword and shows up in docs.
	if strings.Contains(low, "generate range") || strings.Contains(low, "range from") ||
		strings.Contains(low, "number range") {
		return true
	}
	if idx := strings.Index(low, "range"); idx >= 0 {
		rest := low[idx+len("range"):]
		if len(rest) > 0 && (rest[0] == ':' || rest[0] == ' ') {
			// Look for a digit somewhere after "range".
			for i := 0; i < len(rest); i++ {
				c := rest[i]
				if c >= '0' && c <= '9' {
					return true
				}
			}
		}
	}
	return false
}

func (s *RangeSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	if payload == "" {
		// Fall back to everything after the word "range".
		payload = stripVerb(q, "range")
	}
	payload = strings.TrimSpace(payload)
	// Strip connector words from "range from 1 to 10" → "1 10".
	payload = strings.ReplaceAll(payload, " to ", " ")
	payload = strings.ReplaceAll(payload, " from ", " ")
	// Split on commas or whitespace.
	fields := strings.FieldsFunc(payload, func(r rune) bool {
		return r == ' ' || r == '\t' || r == ',' || r == ';'
	})
	if len(fields) < 2 {
		return "", fmt.Errorf("range needs at least start and stop, e.g. 'range: 1 10'")
	}
	start, err := parseIntStrict(fields[0])
	if err != nil {
		return "", err
	}
	stop, err := parseIntStrict(fields[1])
	if err != nil {
		return "", err
	}
	step := 1
	if len(fields) >= 3 {
		step, err = parseIntStrict(fields[2])
		if err != nil {
			return "", err
		}
	}
	if step == 0 {
		return "", fmt.Errorf("step must be non-zero")
	}

	var terms []string
	if step > 0 {
		for n := start; n <= stop; n += step {
			terms = append(terms, strconv.Itoa(n))
			if len(terms) >= sequenceMaxCount {
				break
			}
		}
	} else {
		for n := start; n >= stop; n += step {
			terms = append(terms, strconv.Itoa(n))
			if len(terms) >= sequenceMaxCount {
				break
			}
		}
	}
	if len(terms) == 0 {
		return "", fmt.Errorf("range produced no values (start=%d stop=%d step=%d)", start, stop, step)
	}
	return formatSequence(terms), nil
}

// ---- fibonacci ----

// FibonacciSkill emits the first N Fibonacci numbers using math/big so large
// inputs don't overflow. The conventional sequence starts 0, 1, 1, 2, 3, 5, ...
type FibonacciSkill struct{ *kyoci.BaseSkill }

func NewFibonacciSkill() *FibonacciSkill {
	return &FibonacciSkill{BaseSkill: kyoci.NewBaseSkill(
		"fibonacci", "Generate the first N Fibonacci numbers (0,1,1,2,3,5,...)",
		[]string{"fibonacci", "fib", "fib sequence"},
	)}
}

func (s *FibonacciSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "fibonacci") || strings.Contains(low, "fib ") ||
		strings.Contains(low, "fib:") || strings.HasSuffix(low, " fib")
}

func (s *FibonacciSkill) Execute(_ context.Context, q string) (string, error) {
	payload := strings.TrimSpace(extractPayload(q))
	if payload == "" {
		// "fibonacci 10" form — take trailing integer from the whole query.
		payload = stripVerb(q, "fibonacci")
	}
	if payload == "" {
		return "", fmt.Errorf("fibonacci needs a count, e.g. 'fibonacci: 10'")
	}
	// The payload may contain extra words ("first 10 terms"); pull out the first
	// integer-looking token.
	n, err := pickFirstInt(payload)
	if err != nil {
		return "", fmt.Errorf("fibonacci count: %w", err)
	}
	if n <= 0 {
		return "", fmt.Errorf("count must be positive")
	}
	if n > sequenceMaxCount {
		n = sequenceMaxCount
	}

	terms := make([]string, 0, n)
	a, b := big.NewInt(0), big.NewInt(1)
	for i := 0; i < n; i++ {
		terms = append(terms, a.String())
		a, b = b, new(big.Int).Add(a, b)
	}
	return formatSequence(terms), nil
}

// ---- arithmetic_sequence ----

// ArithmeticSequenceSkill generates start, start+step, start+2*step, ... for count terms.
// Input forms:
//   - "arithmetic: start=2 step=3 count=5"
//   - "arithmetic sequence: 2, 3, 5"  (start, step, count)
type ArithmeticSequenceSkill struct{ *kyoci.BaseSkill }

func NewArithmeticSequenceSkill() *ArithmeticSequenceSkill {
	return &ArithmeticSequenceSkill{BaseSkill: kyoci.NewBaseSkill(
		"arithmetic_sequence", "Generate an arithmetic sequence (start, step, count)",
		[]string{"arithmetic sequence", "arithmetic progression", "arithmetic"},
	)}
}

func (s *ArithmeticSequenceSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "arithmetic sequence") ||
		strings.Contains(low, "arithmetic progression") ||
		strings.Contains(low, "arithmetic:")
}

func (s *ArithmeticSequenceSkill) Execute(_ context.Context, q string) (string, error) {
	start, step, count, err := parseSeqParams(q, "arithmetic")
	if err != nil {
		return "", err
	}
	if count <= 0 {
		return "", fmt.Errorf("count must be positive")
	}
	if count > sequenceMaxCount {
		count = sequenceMaxCount
	}
	terms := make([]string, 0, count)
	cur := big.NewInt(int64(start))
	delta := big.NewInt(int64(step))
	for i := 0; i < count; i++ {
		terms = append(terms, cur.String())
		cur = new(big.Int).Add(cur, delta)
	}
	return formatSequence(terms), nil
}

// ---- geometric_sequence ----

// GeometricSequenceSkill generates start, start*ratio, start*ratio^2, ... for count terms.
// Input forms mirror arithmetic_sequence.
type GeometricSequenceSkill struct{ *kyoci.BaseSkill }

func NewGeometricSequenceSkill() *GeometricSequenceSkill {
	return &GeometricSequenceSkill{BaseSkill: kyoci.NewBaseSkill(
		"geometric_sequence", "Generate a geometric sequence (start, ratio, count)",
		[]string{"geometric sequence", "geometric progression", "geometric"},
	)}
}

func (s *GeometricSequenceSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "geometric sequence") ||
		strings.Contains(low, "geometric progression") ||
		strings.Contains(low, "geometric:")
}

func (s *GeometricSequenceSkill) Execute(_ context.Context, q string) (string, error) {
	start, ratio, count, err := parseSeqParams(q, "geometric")
	if err != nil {
		return "", err
	}
	if count <= 0 {
		return "", fmt.Errorf("count must be positive")
	}
	if count > sequenceMaxCount {
		count = sequenceMaxCount
	}
	terms := make([]string, 0, count)
	cur := big.NewInt(int64(start))
	mul := big.NewInt(int64(ratio))
	for i := 0; i < count; i++ {
		terms = append(terms, cur.String())
		cur = new(big.Int).Mul(cur, mul)
	}
	return formatSequence(terms), nil
}

// parseSeqParams extracts (start, delta, count) from a query in either the
// "name: start=X delta=Y count=Z" form or the positional "name: X, Y, Z" form.
// For arithmetic the delta is "step"; for geometric it is "ratio"; we accept
// both synonyms for robustness.
func parseSeqParams(q, name string) (start, delta, count int, err error) {
	payload := extractPayload(q)
	if payload == "" {
		payload = stripVerb(q, name+" sequence")
	}
	if payload == "" {
		payload = stripVerb(q, name)
	}
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return 0, 0, 0, fmt.Errorf("%s needs start, step/ratio, and count", name)
	}

	// Form 1: key=value pairs.
	if strings.Contains(payload, "=") {
		m := map[string]string{}
		for _, field := range strings.FieldsFunc(payload, func(r rune) bool {
			return r == ' ' || r == '\t' || r == ',' || r == ';'
		}) {
			kv := strings.SplitN(field, "=", 2)
			if len(kv) == 2 {
				m[strings.ToLower(strings.TrimSpace(kv[0]))] = strings.TrimSpace(kv[1])
			}
		}
		start, err = getParamInt(m, "start")
		if err != nil {
			return 0, 0, 0, err
		}
		delta, err = getParamInt(m, "step", "ratio", "common")
		if err != nil {
			return 0, 0, 0, err
		}
		count, err = getParamInt(m, "count", "terms", "n")
		if err != nil {
			return 0, 0, 0, err
		}
		return start, delta, count, nil
	}

	// Form 2: positional "start, delta, count".
	fields := strings.FieldsFunc(payload, func(r rune) bool {
		return r == ' ' || r == '\t' || r == ',' || r == ';'
	})
	if len(fields) < 3 {
		return 0, 0, 0, fmt.Errorf("%s needs three values: start, step/ratio, count", name)
	}
	start, err = parseIntStrict(fields[0])
	if err != nil {
		return 0, 0, 0, err
	}
	delta, err = parseIntStrict(fields[1])
	if err != nil {
		return 0, 0, 0, err
	}
	count, err = parseIntStrict(fields[2])
	if err != nil {
		return 0, 0, 0, err
	}
	return start, delta, count, nil
}

// getParamInt looks up the first present key in m and parses it.
func getParamInt(m map[string]string, keys ...string) (int, error) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return parseIntStrict(v)
		}
	}
	return 0, fmt.Errorf("missing %s", keys[0])
}

// ---- primes_upto ----

// PrimesUptoSkill lists every prime ≤ N using the Sieve of Eratosthenes.
type PrimesUptoSkill struct{ *kyoci.BaseSkill }

func NewPrimesUptoSkill() *PrimesUptoSkill {
	return &PrimesUptoSkill{BaseSkill: kyoci.NewBaseSkill(
		"primes_upto", "List all primes up to N (Sieve of Eratosthenes)",
		[]string{"primes upto", "primes up to", "list primes", "sieve of eratosthenes"},
	)}
}

func (s *PrimesUptoSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "primes upto") || strings.Contains(low, "primes up to") ||
		strings.Contains(low, "list primes") || strings.Contains(low, "sieve of eratosthenes")
}

func (s *PrimesUptoSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	if payload == "" {
		payload = stripVerb(q, "primes up to")
	}
	if payload == "" {
		payload = stripVerb(q, "primes upto")
	}
	if payload == "" {
		payload = stripVerb(q, "list primes")
	}
	if payload == "" {
		payload = stripVerb(q, "sieve of eratosthenes")
	}
	payload = strings.TrimSpace(payload)
	n, err := pickFirstInt(payload)
	if err != nil {
		return "", fmt.Errorf("primes_upto needs an upper bound, e.g. 'primes upto: 20'")
	}
	if n < 2 {
		return "", fmt.Errorf("no primes below 2")
	}
	if n > sequenceMaxCount*1000 {
		// Sieve allocates a bool per integer; cap memory at ~1M entries.
		n = sequenceMaxCount * 1000
	}

	sieve := make([]bool, n+1) // false = prime (after we mark composites true)
	var primes []string
	for i := 2; i <= n; i++ {
		if sieve[i] {
			continue
		}
		primes = append(primes, strconv.Itoa(i))
		for j := i * i; j <= n; j += i {
			sieve[j] = true
		}
	}
	if len(primes) == 0 {
		return "", fmt.Errorf("no primes found up to %d", n)
	}
	return formatSequence(primes), nil
}

// ---- collatz ----

// CollatzSkill emits the Collatz sequence from n down to 1. Each step:
//   - if n is even, n → n/2
//   - if n is odd,  n → 3n+1
//
// The conjecture states this always terminates at 1; we cap iteration count as
// a safety net.
type CollatzSkill struct{ *kyoci.BaseSkill }

func NewCollatzSkill() *CollatzSkill {
	return &CollatzSkill{BaseSkill: kyoci.NewBaseSkill(
		"collatz", "Generate the Collatz (3n+1) sequence from n down to 1",
		[]string{"collatz", "collatz sequence", "3n+1"},
	)}
}

func (s *CollatzSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "collatz") || strings.Contains(low, "3n+1") ||
		strings.Contains(low, "3n + 1")
}

func (s *CollatzSkill) Execute(_ context.Context, q string) (string, error) {
	payload := strings.TrimSpace(extractPayload(q))
	if payload == "" {
		payload = stripVerb(q, "collatz sequence")
	}
	if payload == "" {
		payload = stripVerb(q, "collatz")
	}
	if payload == "" {
		return "", fmt.Errorf("collatz needs a starting number, e.g. 'collatz: 6'")
	}
	start, err := pickFirstInt(payload)
	if err != nil {
		return "", fmt.Errorf("collatz start: %w", err)
	}
	if start < 1 {
		return "", fmt.Errorf("collatz start must be a positive integer")
	}

	// Use math/big to avoid overflow on the (rare) paths that climb high.
	const maxSteps = 100000
	n := big.NewInt(int64(start))
	one := big.NewInt(1)
	two := big.NewInt(2)
	three := big.NewInt(3)
	terms := []string{n.String()}

	for n.Cmp(one) != 0 {
		// Even? n % 2 == 0
		if new(big.Int).Mod(n, two).Sign() == 0 {
			n = new(big.Int).Div(n, two)
		} else {
			// 3n+1
			n = new(big.Int).Add(new(big.Int).Mul(n, three), one)
		}
		terms = append(terms, n.String())
		if len(terms) > maxSteps {
			return "", fmt.Errorf("collatz sequence exceeded %d steps without reaching 1", maxSteps)
		}
	}
	return formatSequence(terms), nil
}
