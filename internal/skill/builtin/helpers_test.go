package builtin

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// =====================================================================================
// Shared helpers for the per-skill test files. Keeps each _test.go small and
// the assertions uniform across the catalog.
// =====================================================================================

// mustContain fails the test if got does not contain want. Prints both for
// easy diagnosis.
func mustContain(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("output missing expected fragment\nwant: %q\ngot:  %q", want, got)
	}
}

// mustNotContain fails the test if got contains unwanted.
func mustNotContain(t *testing.T, got, unwanted string) {
	t.Helper()
	if strings.Contains(got, unwanted) {
		t.Errorf("output contains unwanted fragment %q\n got: %q", unwanted, got)
	}
}

// assertMatch uniformly asserts a skill's Match() result.
func assertMatch(t *testing.T, skillName, query string, match bool, expected bool) {
	t.Helper()
	if match != expected {
		t.Errorf("%s.Match(%q) = %v, want %v", skillName, query, match, expected)
	}
}

// skipIfNetwork marks the test as skipped when running in CI (CI=true env).
// Used by dns_lookup, port_check, and other live-network skills.
func skipIfNetwork(t *testing.T) {
	t.Helper()
	if os.Getenv("CI") != "" {
		t.Skip("skipping network-dependent test in CI")
	}
}

// asFloat extracts the first number from s and returns it. Used by tests
// that need to check a numeric output (stats, percentage, etc.) without
// hard-coding formatting.
func asFloat(s string) float64 {
	var num strings.Builder
	started := false
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' {
			num.WriteRune(r)
			started = true
		} else if started {
			break
		}
	}
	v, _ := strconv.ParseFloat(num.String(), 64)
	return v
}
