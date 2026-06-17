package skill

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// =====================================================================================
// Match() collision test — lives in package skill (not skill/builtin) to avoid
// an import cycle: the builtin package imports skill, so a test in builtin
// importing skill creates a cycle.
//
// What this guards against:
// The skill registry iterates r.skills as a Go map (non-deterministic order).
// If two skills BothMatch the same query, the registry's Match() returns
// whichever the map happens to yield first — meaning different runs could
// route the same query to different skills. This test catches such ambiguities.
// =====================================================================================

func TestSkillMatchNoAmbiguity(t *testing.T) {
	reg := NewRegistry()
	if err := reg.RegisterBuiltin(); err != nil {
		t.Fatalf("RegisterBuiltin: %v", err)
	}

	queries := []struct {
		query   string
		wantCat string // for diagnostic output
	}{
		// encoding
		{"base64 encode: hi", "encoding"},
		{"base64 decode: aGk=", "encoding"},
		{"url encode: hi there", "encoding"},
		{"hex encode: A", "encoding"},
		{"html escape: <b>", "encoding"},
		// hashing
		{"md5 of hello", "hashing"},
		{"sha256 of hello", "hashing"},
		{"sha1 of hello", "hashing"},
		// security
		{"identify hash: 5d41402abc4b2a76b9719d911017c592", "security"},
		{"redact secrets: AKIAIOSFODNN7EXAMPLE", "security"},
		// datafmt
		{"yaml to json: name: Bob", "datafmt"},
		{"json to yaml: {\"a\":1}", "datafmt"},
		// text
		{"slugify: Hello World!", "text"},
		{"reverse: hello", "text"},
		// generators
		{"generate uuid v4", "generators"},
		{"generate nanoid", "generators"},
		// net
		{"validate ip 8.8.8.8", "net"},
		{"parse url https://example.com/", "net"},
		// color
		{"hex to rgb: #ff0000", "color"},
		{"rgb to hex: rgb(255,0,0)", "color"},
		{"contrast ratio between #fff #000", "color"},
		// math
		{"gcd of 12 18", "math"},
		{"is prime 17", "math"},
		{"factorial of 5", "math"},
		// time
		{"what time is it now", "time"},
		{"epoch convert 1700000000", "time"},
		// markdown
		{"markdown toc: # heading", "markdown"},
	}

	type violation struct {
		query   string
		matches []string
	}
	var violations []violation
	noMatches := []string{}

	for _, tc := range queries {
		matches := reg.Kyoci().MatchAll(tc.query)
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, m.Name())
		}
		sort.Strings(names)
		switch len(matches) {
		case 0:
			noMatches = append(noMatches, tc.query)
		case 1:
			// happy path
		default:
			violations = append(violations, violation{tc.query, names})
		}
	}

	if len(noMatches) > 0 {
		t.Errorf("%d query/queries matched NOTHING (registry failed to route):\n%s",
			len(noMatches), strings.Join(noMatches, "\n"))
	}
	if len(violations) > 0 {
		var b strings.Builder
		b.WriteString(fmt.Sprintf("%d ambiguous queries (matched >1 skill):\n", len(violations)))
		for _, v := range violations {
			fmt.Fprintf(&b, "  %q → %v\n", v.query, v.matches)
		}
		t.Error(b.String())
	}
}

// TestSkillMatchNoFalsePositives verifies that generic unrelated queries
// don't accidentally trigger any skill. The "Hello world" / "tell me a joke"
// class of queries should fall through to the worker LLM, not run a skill.
func TestSkillMatchNoFalsePositives(t *testing.T) {
	reg := NewRegistry()
	if err := reg.RegisterBuiltin(); err != nil {
		t.Fatalf("RegisterBuiltin: %v", err)
	}

	queries := []string{
		"hello world",
		"tell me a joke",
		"who are you",
		"write a story",
		"how are you",
		"good morning",
		"thanks",
	}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			matches := reg.Kyoci().MatchAll(q)
			if len(matches) > 0 {
				names := make([]string, len(matches))
				for i, m := range matches {
					names[i] = m.Name()
				}
				t.Errorf("query %q matched %d skill(s): %v — should have matched none", q, len(matches), names)
			}
		})
	}
}
