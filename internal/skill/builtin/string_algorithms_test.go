package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// String-algorithm skill tests — 10 skills (soundex, metaphone, jaro,
// jaro_winkler, hamming_distance, lcs, lcs_substr, ngram, ngram_frequency,
// ratcliff_obershelp).
//
// Uses the shared runSkillCases driver defined in encoding_test.go.
// =====================================================================================

// ---- soundex ----

func TestSoundexSkill(t *testing.T) {
	runSkillCases(t, "soundex", NewSoundexSkill(), []skillCase{
		{"positive: robert", "soundex: robert", true, "R163", false},
		{"positive: Rupert", "soundex: Rupert", true, "R163", false},
		{"positive: Tymczak", "soundex: Tymczak", true, "T522", false},
		{"positive: Pfister", "soundex: Pfister", true, "P236", false},
		{"positive: Ashcraft", "soundex: Ashcraft", true, "A261", false},
		{"positive: Honeyman", "soundex: Honeyman", true, "H555", false},
		{"positive: of-word form", "soundex of Robert", true, "R163", false},
		{"negative: unrelated", "slugify: hello world", false, "", false},
		{"edge: empty input", "soundex:", true, "", true},
	})
}

// ---- metaphone ----

func TestMetaphoneSkill(t *testing.T) {
	skill := NewMetaphoneSkill()
	// Metaphone (simplified) is intentionally a loose spec; we check
	// structural properties and a few stable mappings rather than exact
	// outputs for every word.
	stableCases := []struct {
		query, want string
	}{
		{"metaphone: phone", "FN"},  // PH→F, trailing vowels dropped
		{"metaphone: knight", "NT"}, // KN- initial → N, GH silent, T
		{"metaphone: quick", "KK"},  // QU → K + K (Q→K, U vowel dropped)
		{"metaphone: xerox", "SRK"}, // initial X→S, R, trailing X→K
	}
	for _, tc := range stableCases {
		t.Run(tc.query, func(t *testing.T) {
			if !skill.Match(tc.query) {
				t.Fatalf("expected match for %q", tc.query)
			}
			out, err := skill.Execute(context.Background(), tc.query)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if out != tc.want {
				t.Errorf("metaphone %q = %q, want %q", tc.query, out, tc.want)
			}
		})
	}
	// Structural / negative cases.
	runSkillCases(t, "metaphone-cases", skill, []skillCase{
		{"positive: single word", "metaphone: hello", true, "", false},
		{"positive: metaphone of form", "metaphone of hello", true, "", false},
		{"negative: unrelated", "soundex: robert", false, "", false},
		{"edge: empty input", "metaphone:", true, "", true},
	})
}

// ---- jaro ----

func TestJaroSkill(t *testing.T) {
	skill := NewJaroSkill()
	checks := []struct {
		query, want string
	}{
		{"jaro: MARTHA, MARHTA", "0.944"},
		{"jaro: DWAYNE, DUANE", "0.822"},
		{"jaro: CRATE, TRACE", "0.733"},
		{"jaro: same | same", "1.000"},
	}
	for _, c := range checks {
		t.Run(c.query, func(t *testing.T) {
			if !skill.Match(c.query) {
				t.Fatalf("expected match for %q", c.query)
			}
			out, err := skill.Execute(context.Background(), c.query)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if out != c.want {
				t.Errorf("jaro %q = %q, want %q", c.query, out, c.want)
			}
		})
	}
	runSkillCases(t, "jaro-cases", skill, []skillCase{
		{"pipe format matches", "jaro: foo | bar", true, "", false},
		{"negative: jaro_winkler should not match jaro", "jaro_winkler: foo, bar", false, "", false},
		{"negative: jaro-winkler hyphen should not match jaro", "jaro-winkler: foo, bar", false, "", false},
		{"negative: unrelated", "soundex: robert", false, "", false},
		{"edge: missing operand", "jaro: onlyone", true, "", true},
	})
}

// ---- jaro_winkler ----

func TestJaroWinklerSkill(t *testing.T) {
	skill := NewJaroWinklerSkill()
	checks := []struct {
		query, want string
	}{
		{"jaro_winkler: MARTHA, MARHTA", "0.961"},
		{"jaro_winkler: DWAYNE, DUANE", "0.840"},
		{"jaro-winkler: MARTHA | MARHTA", "0.961"},
	}
	for _, c := range checks {
		t.Run(c.query, func(t *testing.T) {
			if !skill.Match(c.query) {
				t.Fatalf("expected match for %q", c.query)
			}
			out, err := skill.Execute(context.Background(), c.query)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if out != c.want {
				t.Errorf("jaro_winkler %q = %q, want %q", c.query, out, c.want)
			}
		})
	}
	runSkillCases(t, "jaro_winkler-cases", skill, []skillCase{
		{"negative: jaro (plain) should not match winkler", "jaro: foo, bar", false, "", false},
		{"negative: unrelated", "soundex: robert", false, "", false},
		{"edge: missing operand", "jaro_winkler: onlyone", true, "", true},
	})
}

// ---- hamming_distance ----

func TestHammingDistanceSkill(t *testing.T) {
	skill := NewHammingDistanceSkill()
	checks := []struct {
		query, want string
	}{
		{"hamming: karolin, kathrin", "3"},
		{"hamming: toned, roses", "3"},
		{"hamming: 1011101, 1001001", "2"},
		{"hamming: same | same", "0"},
	}
	for _, c := range checks {
		t.Run(c.query, func(t *testing.T) {
			if !skill.Match(c.query) {
				t.Fatalf("expected match for %q", c.query)
			}
			out, err := skill.Execute(context.Background(), c.query)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if out != c.want {
				t.Errorf("hamming %q = %q, want %q", c.query, out, c.want)
			}
		})
	}
	runSkillCases(t, "hamming-cases", skill, []skillCase{
		{"negative: unrelated", "soundex: robert", false, "", false},
		{"edge: unequal lengths", "hamming: abcd, abc", true, "", true},
		{"edge: missing operand", "hamming: onlyone", true, "", true},
	})
}

// ---- lcs (subsequence) ----

func TestLCSSkill(t *testing.T) {
	skill := NewLCSSkill()
	lengthChecks := []struct {
		query, wantLen string
	}{
		{"lcs: AGCAT, GAC", "length: 2"},
		{"lcs: ABCBDAB, BDCAB", "length: 4"},
		{"lcs: AGGTAB, GXTXAYB", "length: 4"},
		{"lcs: completelydifferent, totallyunrelated", "length: 7"},
	}
	for _, c := range lengthChecks {
		t.Run(c.query, func(t *testing.T) {
			if !skill.Match(c.query) {
				t.Fatalf("expected match for %q", c.query)
			}
			out, err := skill.Execute(context.Background(), c.query)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !strings.Contains(out, c.wantLen) {
				t.Errorf("lcs %q = %q, want containing %q", c.query, out, c.wantLen)
			}
		})
	}
	// Deterministic identical-input subsequence.
	out, err := skill.Execute(context.Background(), "lcs: ABCDEFG, ABCDEFG")
	if err != nil {
		t.Fatalf("Execute identical: %v", err)
	}
	if !strings.Contains(out, "subsequence: ABCDEFG") {
		t.Errorf("expected subsequence ABCDEFG, got %q", out)
	}
	runSkillCases(t, "lcs-cases", skill, []skillCase{
		{"positive: 'lcs of' form", "lcs of ABCD, BCDE", true, "length: 3", false},
		{"positive: 'lcs between' form", "lcs between foo, bar", true, "", false},
		{"negative: lcs substring must NOT match LCS", "lcs substring: foo, bar", false, "", false},
		{"negative: lcs_substr must NOT match LCS", "lcs_substr: foo, bar", false, "", false},
		{"negative: longest common substring must NOT match LCS", "longest common substring: foo, bar", false, "", false},
		{"positive: longest common subsequence phrase", "longest common subsequence: ABC, ABC", true, "length: 3", false},
		{"negative: unrelated", "soundex: robert", false, "", false},
		{"edge: missing operand", "lcs: onlyone", true, "", true},
	})
}

// ---- lcs_substr (longest common substring) ----

func TestLCSSubstrSkill(t *testing.T) {
	skill := NewLCSSubstrSkill()
	checks := []struct {
		query, wantLen, wantSub string
	}{
		{"lcs_substr: abcdef, zcdema", "length: 3", "substring: cde"},
		{"lcs substring: abcsite, xyzsite", "length: 4", "substring: site"},
		{"lcs_substr: programming, grammar", "length: 5", "substring: gramm"},
	}
	for _, c := range checks {
		t.Run(c.query, func(t *testing.T) {
			if !skill.Match(c.query) {
				t.Fatalf("expected match for %q", c.query)
			}
			out, err := skill.Execute(context.Background(), c.query)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !strings.Contains(out, c.wantLen) {
				t.Errorf("lcs_substr %q: expected %q in %q", c.query, c.wantLen, out)
			}
			if !strings.Contains(out, c.wantSub) {
				t.Errorf("lcs_substr %q: expected %q in %q", c.query, c.wantSub, out)
			}
		})
	}
	runSkillCases(t, "lcs_substr-cases", skill, []skillCase{
		{"positive: longest common substring phrase", "longest common substring: abcdef, abcdef", true, "length: 6", false},
		{"positive: longest substring phrase", "longest substring: foo, foo", true, "", false},
		{"negative: plain lcs must NOT match substr", "lcs: foo, bar", false, "", false},
		{"negative: longest common subsequence must NOT match substr", "longest common subsequence: foo, bar", false, "", false},
		{"negative: unrelated", "soundex: robert", false, "", false},
		{"edge: missing operand", "lcs_substr: onlyone", true, "", true},
	})
}

// ---- ngram ----

func TestNgramSkill(t *testing.T) {
	skill := NewNgramSkill()
	// Default n=2 over "hello" → he, el, ll, lo.
	out, err := skill.Execute(context.Background(), "ngram: hello")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"n=2", "count=4", "he, el, ll, lo"} {
		if !strings.Contains(out, want) {
			t.Errorf("ngram default: expected %q in %q", want, out)
		}
	}
	// Explicit n=3 over "hello world" → 9 trigrams.
	out, err = skill.Execute(context.Background(), "ngram 3: hello world")
	if err != nil {
		t.Fatalf("Execute trigrams: %v", err)
	}
	for _, want := range []string{"n=3", "count=9", "hel", "ell", "llo", "orl", "rld"} {
		if !strings.Contains(out, want) {
			t.Errorf("ngram 3: expected %q in %q", want, out)
		}
	}
	runSkillCases(t, "ngram-cases", skill, []skillCase{
		{"positive: n-gram hyphen form", "n-gram: hi", true, "", false},
		{"negative: ngram_frequency must NOT match ngram", "ngram_frequency 2: hello", false, "", false},
		{"negative: n-gram frequency must NOT match ngram", "n-gram frequency 2: hello", false, "", false},
		{"negative: unrelated", "soundex: robert", false, "", false},
		{"edge: n too large", "ngram 99: hi", true, "", true},
		{"edge: empty input", "ngram: ", true, "", true},
	})
}

// ---- ngram_frequency ----

func TestNgramFrequencySkill(t *testing.T) {
	skill := NewNgramFrequencySkill()
	// "banana" with n=2: ba, an, na, an, na → an:2, na:2, ba:1.
	out, err := skill.Execute(context.Background(), "ngram_frequency 2: banana")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"n=2", "unique=3", "total=5", "an: 2", "na: 2", "ba: 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("ngram_frequency: expected %q in %q", want, out)
		}
	}
	// Sort order: count desc, then alphabetical. ba:1 should come AFTER an:2 and na:2.
	anIdx := strings.Index(out, "an: 2")
	naIdx := strings.Index(out, "na: 2")
	baIdx := strings.Index(out, "ba: 1")
	if anIdx < 0 || naIdx < 0 || baIdx < 0 {
		t.Fatalf("missing entries in %q", out)
	}
	if baIdx < anIdx || baIdx < naIdx {
		t.Errorf("expected ba:1 to come AFTER an:2 and na:2; out=%q", out)
	}
	runSkillCases(t, "ngram_frequency-cases", skill, []skillCase{
		{"positive: n-gram frequency hyphen form", "n-gram frequency 2: hi", true, "", false},
		{"positive: ngram_frequency underscore form", "ngram_frequency 2: hi", true, "", false},
		{"negative: plain ngram must NOT match frequency", "ngram 2: hi", false, "", false},
		{"negative: unrelated", "soundex: robert", false, "", false},
		{"edge: empty input", "ngram_frequency 2: ", true, "", true},
	})
}

// ---- ratcliff_obershelp ----

func TestRatcliffObershelpSkill(t *testing.T) {
	skill := NewRatcliffObershelpSkill()
	// "WIKIMEDIA" vs "WIKIMANIA": matched common substrings
	// WIKI(4) + IA(2) + M(1) = 7 → 2*7/18 = 0.778.
	out, err := skill.Execute(context.Background(), "ratcliff: WIKIMEDIA, WIKIMANIA")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "0.778" {
		t.Errorf("ratcliff WIKIMEDIA vs WIKIMANIA = %q, want 0.778", out)
	}
	// Identity.
	out, err = skill.Execute(context.Background(), "ratcliff: same, same")
	if err != nil {
		t.Fatalf("Execute identity: %v", err)
	}
	if out != "1.000" {
		t.Errorf("ratcliff identity = %q, want 1.000", out)
	}
	// Disjoint.
	out, err = skill.Execute(context.Background(), "ratcliff: abc | xyz")
	if err != nil {
		t.Fatalf("Execute disjoint: %v", err)
	}
	if out != "0.000" {
		t.Errorf("ratcliff disjoint = %q, want 0.000", out)
	}
	runSkillCases(t, "ratcliff-cases", skill, []skillCase{
		{"positive: obershelp keyword", "obershelp: abc, abc", true, "1.000", false},
		{"positive: ratcliff obershelp phrase", "ratcliff obershelp: abc, abc", true, "1.000", false},
		{"negative: unrelated", "soundex: robert", false, "", false},
		{"edge: missing operand", "ratcliff: onlyone", true, "", true},
	})
}

// ---- direct-algorithm unit tests (no skill plumbing) ----

func TestSoundexAlgorithm(t *testing.T) {
	cases := map[string]string{
		"Robert":   "R163",
		"Rupert":   "R163",
		"Rubin":    "R150",
		"Tymczak":  "T522",
		"Pfister":  "P236",
		"Honeyman": "H555",
		"Ashcraft": "A261",
		"":         "0000",
		"!@#$":     "0000",
	}
	for in, want := range cases {
		if got := soundex(in); got != want {
			t.Errorf("soundex(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJaroAlgorithm(t *testing.T) {
	cases := []struct {
		a, b string
		want string
	}{
		{"MARTHA", "MARHTA", "0.944"},
		{"DIXON", "DICKSONX", "0.767"},
		{"JELLYFISH", "SMELLYFISH", "0.896"},
		{"", "", "1.000"},
		{"a", "", "0.000"},
	}
	for _, c := range cases {
		got := formatFloat3(jaro(c.a, c.b))
		if got != c.want {
			t.Errorf("jaro(%q,%q) = %s, want %s", c.a, c.b, got, c.want)
		}
	}
}

func TestHammingAlgorithm(t *testing.T) {
	if d := hammingDistance("karolin", "kathrin"); d != 3 {
		t.Errorf("hamming karolin/kathrin = %d, want 3", d)
	}
	if d := hammingDistance("same", "same"); d != 0 {
		t.Errorf("hamming identity = %d, want 0", d)
	}
	if d := hammingDistance("a", "ab"); d != -1 {
		t.Errorf("hamming unequal-length should return -1, got %d", d)
	}
}

func TestLCSAlgorithm(t *testing.T) {
	sub, n := longestCommonSubsequence("AGGTAB", "GXTXAYB")
	if n != 4 {
		t.Errorf("LCS length = %d, want 4", n)
	}
	if sub != "GTAB" {
		t.Errorf("LCS = %q, want GTAB", sub)
	}
	_, n = longestCommonSubsequence("", "abc")
	if n != 0 {
		t.Errorf("LCS with empty input should be 0, got %d", n)
	}
}

func TestLCSSubstringAlgorithm(t *testing.T) {
	sub, n := longestCommonSubstring("abcdef", "zcdema")
	if n != 3 || sub != "cde" {
		t.Errorf("LCS-substring = (%q,%d), want (cde,3)", sub, n)
	}
	sub, n = longestCommonSubstring("aaa", "aa")
	if n != 2 || sub != "aa" {
		t.Errorf("LCS-substring aaa/aa = (%q,%d), want (aa,2)", sub, n)
	}
}

func TestRatcliffObershelpAlgorithm(t *testing.T) {
	got := formatFloat3(ratcliffObershelp("WIKIMEDIA", "WIKIMANIA"))
	if got != "0.778" {
		t.Errorf("ratcliff = %s, want 0.778", got)
	}
	got = formatFloat3(ratcliffObershelp("", ""))
	if got != "1.000" {
		t.Errorf("ratcliff empty/empty = %s, want 1.000", got)
	}
	got = formatFloat3(ratcliffObershelp("abc", "xyz"))
	if got != "0.000" {
		t.Errorf("ratcliff abc/xyz = %s, want 0.000", got)
	}
}

func TestNgramsHelper(t *testing.T) {
	if grams := ngrams("hello", 2); len(grams) != 4 || grams[0] != "he" || grams[3] != "lo" {
		t.Errorf("ngrams(hello,2) = %v, want [he el ll lo]", grams)
	}
	if grams := ngrams("hi", 5); grams != nil {
		t.Errorf("ngrams too long should be nil, got %v", grams)
	}
	if grams := ngrams("hi", 0); grams != nil {
		t.Errorf("ngrams n=0 should be nil, got %v", grams)
	}
}
