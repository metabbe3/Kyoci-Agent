package promptskill

import (
	"testing"
)

func sampleRegistry() *Registry {
	return &Registry{
		skills: []PromptSkill{
			{
				Name:     "macos-control",
				Category: "os-control",
				Priority: "high",
				Triggers: Triggers{
					Keywords: []string{"macos", "mac", "osascript"},
					Regex:    []string{`\bmac ?os\b`},
				},
				Body: "macos body",
			},
			{
				Name:     "linux-control",
				Category: "os-control",
				Priority: "normal",
				Triggers: Triggers{
					Keywords: []string{"linux", "systemd", "ubuntu"},
				},
				Body: "linux body",
			},
			{
				Name:     "nginx",
				Category: "server-admin",
				Priority: "normal",
				Triggers: Triggers{
					Keywords: []string{"nginx"},
					Regex:    []string{`nginx\s+(config|restart|logs)`},
				},
				Body: "nginx body",
			},
		},
		byName: map[string]int{"macos-control": 0, "linux-control": 1, "nginx": 2},
	}
}

// TestMatch_KeywordHit verifies keyword substring matching (case-insensitive).
func TestMatch_KeywordHit(t *testing.T) {
	reg := sampleRegistry()

	got := reg.Match("restart my macos service", MatchOptions{})
	if len(got) != 1 {
		t.Fatalf("expected 1 match, got %d: %+v", len(got), got)
	}
	if got[0].Name != "macos-control" {
		t.Errorf("matched %q, want macos-control", got[0].Name)
	}
}

// TestMatch_RegexHit verifies regex pattern matching.
func TestMatch_RegexHit(t *testing.T) {
	reg := sampleRegistry()

	// "nginx restart" matches the nginx regex but not any keyword-only path.
	got := reg.Match("please nginx restart now", MatchOptions{})
	if len(got) != 1 {
		t.Fatalf("expected 1 match, got %d", len(got))
	}
	if got[0].Name != "nginx" {
		t.Errorf("matched %q, want nginx", got[0].Name)
	}
}

// TestMatch_MultipleMatchesSortedByPriority verifies that high-priority
// skills sort before normal-priority ones.
func TestMatch_MultipleMatchesSortedByPriority(t *testing.T) {
	reg := sampleRegistry()

	// "macos vs linux" hits both keywords.
	got := reg.Match("compare macos and linux networking", MatchOptions{})
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(got))
	}
	if got[0].Name != "macos-control" {
		t.Errorf("first match = %q, want macos-control (priority high)", got[0].Name)
	}
	if got[1].Name != "linux-control" {
		t.Errorf("second match = %q, want linux-control", got[1].Name)
	}
}

// TestMatch_NoMatchEmpty verifies unmatched tasks return nil.
func TestMatch_NoMatchEmpty(t *testing.T) {
	reg := sampleRegistry()
	got := reg.Match("what is the capital of france", MatchOptions{})
	if len(got) != 0 {
		t.Errorf("expected 0 matches, got %d: %+v", len(got), got)
	}
}

// TestMatch_MaxSkillsCap verifies the MaxSkills option limits the result count.
func TestMatch_MaxSkillsCap(t *testing.T) {
	// Build a registry where all 10 skills match the same keyword.
	reg := emptyRegistry()
	for i := 0; i < 10; i++ {
		s := PromptSkill{
			Name:     "skill-" + string(rune('a'+i)),
			Priority: "normal",
			Triggers: Triggers{Keywords: []string{"alpha"}},
			Body:     "x",
		}
		reg.byName[s.Name] = len(reg.skills)
		reg.skills = append(reg.skills, s)
	}

	got := reg.Match("alpha task", MatchOptions{MaxSkills: 3})
	if len(got) != 3 {
		t.Fatalf("expected 3 matches (capped), got %d", len(got))
	}
}

// TestMatch_MaxCharsCap verifies the MaxTotalChars option truncates results to
// stay under the byte budget.
func TestMatch_MaxCharsCap(t *testing.T) {
	reg := emptyRegistry()
	// Each skill body is ~200 bytes; cap at 500 should yield ~2.
	body := make([]byte, 200)
	for i := range body {
		body[i] = 'x'
	}
	for i := 0; i < 5; i++ {
		s := PromptSkill{
			Name:     "skill-" + string(rune('a'+i)),
			Priority: "normal",
			Triggers: Triggers{Keywords: []string{"alpha"}},
			Body:     string(body),
		}
		reg.byName[s.Name] = len(reg.skills)
		reg.skills = append(reg.skills, s)
	}

	got := reg.Match("alpha task", MatchOptions{MaxTotalChars: 500})
	total := 0
	for _, s := range got {
		total += len(s.Body)
	}
	if total > 500 {
		t.Errorf("total body bytes %d exceeds cap 500", total)
	}
	if len(got) == 0 {
		t.Error("expected at least 1 match under char cap, got 0")
	}
}

// TestMatch_KeywordPrecision_ShortKeywordNoFalseSubstringMatch verifies that
// short keywords like "ss" do NOT match as substrings inside larger words
// (e.g. "ss" in "processes"). The matcher must require a word boundary so
// "ss" only matches the standalone `ss` command, not random letter pairs.
func TestMatch_KeywordPrecision_ShortKeywordNoFalseSubstringMatch(t *testing.T) {
	reg := &Registry{
		skills: []PromptSkill{
			{
				Name:     "networking",
				Priority: "high",
				Triggers: Triggers{Keywords: []string{"ss"}},
				Body:     "networking body",
			},
		},
		byName: map[string]int{"networking": 0},
	}
	// "processes" contains "ss" as a substring — must NOT match keyword "ss".
	got := reg.Match("check what processes are using the most memory", MatchOptions{})
	if len(got) != 0 {
		t.Errorf("short keyword 'ss' must not match substring in 'processes'; "+
			"got %d false match(es): %+v", len(got), got)
	}
}

// TestMatch_KeywordPrecision_ShortKeywordMatchesStandalone verifies that a
// short keyword DOES match when it appears as a standalone word.
func TestMatch_KeywordPrecision_ShortKeywordMatchesStandalone(t *testing.T) {
	reg := &Registry{
		skills: []PromptSkill{
			{
				Name:     "networking",
				Priority: "high",
				Triggers: Triggers{Keywords: []string{"ss"}},
				Body:     "networking body",
			},
		},
		byName: map[string]int{"networking": 0},
	}
	// "ss" as a standalone command — SHOULD match.
	got := reg.Match("run ss -tlnp to check ports", MatchOptions{})
	if len(got) != 1 {
		t.Errorf("keyword 'ss' should match as standalone word; got %d matches", len(got))
	}
}

// TestMatch_KeywordPrecision_PrefixMatchStillWorks verifies that keywords
// still match prefixed words (e.g. "network" matches "networking"). This
// ensures the word-boundary fix doesn't over-restrict matching.
func TestMatch_KeywordPrecision_PrefixMatchStillWorks(t *testing.T) {
	reg := &Registry{
		skills: []PromptSkill{
			{
				Name:     "networking",
				Priority: "high",
				Triggers: Triggers{Keywords: []string{"network"}},
				Body:     "networking body",
			},
		},
		byName: map[string]int{"networking": 0},
	}
	// "network" should match "networking" (prefix at word boundary).
	got := reg.Match("diagnose my networking issues", MatchOptions{})
	if len(got) != 1 {
		t.Errorf("keyword 'network' should match 'networking' (prefix match); got %d matches", len(got))
	}
}
