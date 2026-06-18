package agentdef

import "testing"

func TestMatchScore_KeywordsScoreOne(t *testing.T) {
	def := AgentDef{
		Triggers: TriggerSpec{
			Keywords: []string{"code", "function"},
		},
	}
	if got := MatchScore(def, "write me a function to parse code"); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
	if got := MatchScore(def, "nothing relevant here"); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestMatchScore_AnchorsScoreThree(t *testing.T) {
	def := AgentDef{
		Triggers: TriggerSpec{
			Anchors: []string{".tsx", " react "},
		},
	}
	// Both anchors match → 3+3=6.
	if got := MatchScore(def, "build a react component in .tsx"); got != 6 {
		t.Errorf("got %d, want 6", got)
	}
	// One anchor matches → 3.
	if got := MatchScore(def, "edit the file.tsx"); got != 3 {
		t.Errorf("got %d, want 3", got)
	}
}

func TestMatchScore_CaseInsensitive(t *testing.T) {
	def := AgentDef{Triggers: TriggerSpec{Keywords: []string{"deploy"}}}
	for _, tc := range []string{"DEPLOY it", "Please deploy", "DEPLOYMENT"} {
		// "DEPLOYMENT" contains "deploy" as substring, so matches.
		if MatchScore(def, tc) == 0 {
			t.Errorf("expected match for %q", tc)
		}
	}
}

func TestMatchScore_Regex(t *testing.T) {
	def := AgentDef{
		Triggers: TriggerSpec{
			Regex: []string{`\bgo\s+build\b`, `\bpython3?\b`},
		},
	}
	if got := MatchScore(def, "run go build ./..."); got != 1 {
		t.Errorf("got %d, want 1", got)
	}
	if got := MatchScore(def, "use python3 and go build"); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
	// Malformed regex silently ignored, does not panic.
	def2 := AgentDef{Triggers: TriggerSpec{Regex: []string{`(`}}}
	if got := MatchScore(def2, "anything"); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestPriorityRank(t *testing.T) {
	cases := map[string]int{
		"high":   2,
		"HIGH":   2,
		"normal": 1,
		"":       1,
		"low":    0,
		"weird":  1, // unknown → normal
	}
	for in, want := range cases {
		if got := PriorityRank(in); got != want {
			t.Errorf("PriorityRank(%q): got %d, want %d", in, got, want)
		}
	}
}

func TestBestMatch_SpecialistClearsThreshold(t *testing.T) {
	defs := []AgentDef{
		{Name: "developer", Triggers: TriggerSpec{Keywords: []string{"code", "function"}}},
		{Name: "generalist"},
	}
	// "write me a function to parse code" hits developer twice → score 2 ≥ threshold.
	if got := BestMatch("write me a function to parse code", defs); got != "developer" {
		t.Errorf("got %q, want developer", got)
	}
}

func TestBestMatch_FallsBackToGeneralist(t *testing.T) {
	defs := []AgentDef{
		{Name: "developer", Triggers: TriggerSpec{Keywords: []string{"code"}}},
		{Name: "generalist"},
	}
	// "what's the capital of France" — no keyword matches → generalist fallback.
	if got := BestMatch("what's the capital of France", defs); got != "generalist" {
		t.Errorf("got %q, want generalist", got)
	}
}

func TestBestMatch_NoGeneralistReturnsFirstDef(t *testing.T) {
	defs := []AgentDef{
		{Name: "developer", Triggers: TriggerSpec{Keywords: []string{"code"}}},
		{Name: "sre"},
	}
	// No match, no generalist → first def (deterministic).
	if got := BestMatch("what's the capital of France", defs); got != "developer" {
		t.Errorf("got %q, want developer (first def)", got)
	}
}

func TestBestMatch_EmptyDefsReturnsEmpty(t *testing.T) {
	if got := BestMatch("anything", nil); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestBestMatch_PriorityBreaksTies(t *testing.T) {
	defs := []AgentDef{
		{Name: "a", Triggers: TriggerSpec{Anchors: []string{"xyz"}}, Priority: "normal"},
		{Name: "b", Triggers: TriggerSpec{Anchors: []string{"xyz"}}, Priority: "high"},
	}
	// Both hit anchor "xyz" → both score 3. Priority high wins.
	if got := BestMatch("xxx xyz xxx", defs); got != "b" {
		t.Errorf("got %q, want b (high priority)", got)
	}
}

func TestBestMatch_HigherScoreWins(t *testing.T) {
	defs := []AgentDef{
		{Name: "weak", Triggers: TriggerSpec{Keywords: []string{"foo"}}, Priority: "high"},
		{Name: "strong", Triggers: TriggerSpec{Anchors: []string{"foo"}}, Priority: "low"},
	}
	// weak scores 1 (keyword), strong scores 3 (anchor). Strong wins despite low priority.
	if got := BestMatch("foo bar baz", defs); got != "strong" {
		t.Errorf("got %q, want strong (higher score)", got)
	}
}

func TestBestMatch_SubthresholdSpecialistFallsBack(t *testing.T) {
	defs := []AgentDef{
		// Single keyword match = score 1 < MinSpecialistScore (2).
		{Name: "developer", Triggers: TriggerSpec{Keywords: []string{"code"}}, Priority: "high"},
		{Name: "generalist"},
	}
	// "code" alone matches with score 1 — below threshold — so generalist wins.
	if got := BestMatch("show me the code of conduct", defs); got != "generalist" {
		t.Errorf("got %q, want generalist (subthreshold)", got)
	}
}
