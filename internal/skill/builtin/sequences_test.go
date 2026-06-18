package builtin

import (
	"testing"
)

// =====================================================================================
// Sequence skill tests — range, fibonacci, arithmetic/geometric progressions,
// primes_upto, collatz.
// =====================================================================================

// ---- range ----

func TestRangeSkill(t *testing.T) {
	runSkillCases(t, "range", NewRangeSkill(), []skillCase{
		{"positive: colon form", "range: 1 10", true, "1 2 3 4 5 6 7 8 9 10", false},
		{"positive: step", "range: 1 10 2", true, "1 3 5 7 9", false},
		{"positive: from-to phrasing", "range from 1 to 5", true, "1 2 3 4 5", false},
		{"positive: descending", "range: 5 1 -1", true, "5 4 3 2 1", false},
		{"positive: comma-separated", "range: 0, 4", true, "0 1 2 3 4", false},
		{"positive: generate range", "generate range 1 to 3", true, "1 2 3", false},
		{"edge: single value range", "range: 5 5", true, "5", false},
		{"negative: bare range", "range", false, "", false},
		{"negative: unrelated", "explain Go range loops", false, "", false},
		{"error: missing stop", "range: 5", true, "", true},
		{"error: zero step", "range: 1 5 0", true, "", true},
	})
}

// ---- fibonacci ----

func TestFibonacciSkill(t *testing.T) {
	runSkillCases(t, "fibonacci", NewFibonacciSkill(), []skillCase{
		{"positive: colon form", "fibonacci: 10", true, "0 1 1 2 3 5 8 13 21 34", false},
		{"positive: small", "fibonacci: 5", true, "0 1 1 2 3", false},
		{"positive: min", "fibonacci: 1", true, "0", false},
		{"positive: abbreviation", "fib: 7", true, "0 1 1 2 3 5 8", false},
		{"positive: large stays correct", "fibonacci: 100", true, "218922995834555169026", false},
		{"negative: unrelated", "sha256 hello", false, "", false},
		{"error: zero count", "fibonacci: 0", true, "", true},
		{"error: negative", "fibonacci: -3", true, "", true},
		{"error: missing count", "fibonacci:", true, "", true},
	})
}

// ---- arithmetic_sequence ----

func TestArithmeticSequenceSkill(t *testing.T) {
	runSkillCases(t, "arithmetic_sequence", NewArithmeticSequenceSkill(), []skillCase{
		{"positive: kv form", "arithmetic: start=2 step=3 count=5", true, "2 5 8 11 14", false},
		{"positive: positional", "arithmetic sequence: 2, 3, 5", true, "2 5 8 11 14", false},
		{"positive: progression phrasing", "arithmetic progression: 10, -2, 4", true, "10 8 6 4", false},
		{"positive: zero step", "arithmetic: start=7 step=0 count=3", true, "7 7 7", false},
		{"positive: negative start", "arithmetic: -5 1 4", true, "-5 -4 -3 -2", false},
		{"negative: unrelated", "explain arithmetic", false, "", false},
		{"error: missing values", "arithmetic sequence: 5", true, "", true},
		{"error: bad count", "arithmetic: start=1 step=1 count=0", true, "", true},
	})
}

// ---- geometric_sequence ----

func TestGeometricSequenceSkill(t *testing.T) {
	runSkillCases(t, "geometric_sequence", NewGeometricSequenceSkill(), []skillCase{
		{"positive: kv form", "geometric: start=1 ratio=2 count=5", true, "1 2 4 8 16", false},
		{"positive: positional", "geometric sequence: 3, 2, 4", true, "3 6 12 24", false},
		{"positive: progression phrasing", "geometric progression: 1, 10, 4", true, "1 10 100 1000", false},
		{"positive: ratio 1", "geometric: start=5 ratio=1 count=3", true, "5 5 5", false},
		{"positive: large values (big.Int)", "geometric: 1 2 80", true, "604462909807314587353088", false},
		{"negative: unrelated", "explain geometry", false, "", false},
		{"error: missing values", "geometric sequence: 5", true, "", true},
		{"error: bad count", "geometric: start=1 ratio=2 count=0", true, "", true},
	})
}

// ---- primes_upto ----

func TestPrimesUptoSkill(t *testing.T) {
	runSkillCases(t, "primes_upto", NewPrimesUptoSkill(), []skillCase{
		{"positive: colon form", "primes upto: 20", true, "2 3 5 7 11 13 17 19", false},
		{"positive: spaced form", "primes up to 30", true, "2 3 5 7 11 13 17 19 23 29", false},
		{"positive: list primes", "list primes up to 11", true, "2 3 5 7 11", false},
		{"positive: sieve phrasing", "sieve of eratosthenes 13", true, "2 3 5 7 11 13", false},
		{"positive: smallest", "primes upto: 2", true, "2", false},
		{"negative: unrelated", "hash hello", false, "", false},
		{"error: below 2", "primes upto: 1", true, "", true},
		{"error: missing bound", "primes upto:", true, "", true},
	})
}

// ---- collatz ----

func TestCollatzSkill(t *testing.T) {
	runSkillCases(t, "collatz", NewCollatzSkill(), []skillCase{
		{"positive: typical", "collatz: 6", true, "6 3 10 5 16 8 4 2 1", false},
		{"positive: small", "collatz: 1", true, "1", false},
		{"positive: even power of 2", "collatz: 8", true, "8 4 2 1", false},
		{"positive: sequence phrasing", "collatz sequence: 7", true, "7 22 11 34 17 52 26 13 40 20 10 5 16 8 4 2 1", false},
		{"positive: larger start (newline-separated)", "collatz: 27", true, "27\n82\n41\n124\n62\n31", false},
		{"negative: unrelated", "base64 encode: hello", false, "", false},
		{"error: zero", "collatz: 0", true, "", true},
		{"error: negative", "collatz: -5", true, "", true},
		{"error: missing start", "collatz:", true, "", true},
	})
}
