package promptskill

import (
	"context"
	"strings"
	"testing"
)

// TestInjector_InjectsMatchedSkillBody verifies that when a task matches a
// skill, the injector returns a prompt fragment containing the skill body.
func TestInjector_InjectsMatchedSkillBody(t *testing.T) {
	reg := &Registry{
		skills: []PromptSkill{
			{
				Name:     "macos-control",
				Priority: "high",
				Triggers: Triggers{Keywords: []string{"macos"}},
				Body:     "Run `df -h` for disk info on macOS.",
			},
		},
		byName: map[string]int{"macos-control": 0},
	}
	inj := NewInjector(reg, nil)

	got := inj.Inject("show me disk usage on my macos machine")
	if got == "" {
		t.Fatal("Inject() returned empty for a matching task")
	}
	if !strings.Contains(got, "df -h") {
		t.Errorf("expected body content in injection; got: %q", got)
	}
	if !strings.Contains(got, "macos-control") {
		t.Errorf("expected skill name header in injection; got: %q", got)
	}
}

// TestInjector_NoMatch_ReturnsEmpty verifies unmatched tasks produce no
// injection (empty string) so the system prompt is not bloated.
func TestInjector_NoMatch_ReturnsEmpty(t *testing.T) {
	reg := &Registry{
		skills: []PromptSkill{
			{
				Name:     "macos-control",
				Triggers: Triggers{Keywords: []string{"macos"}},
				Body:     "should not appear",
			},
		},
		byName: map[string]int{"macos-control": 0},
	}
	inj := NewInjector(reg, nil)

	if got := inj.Inject("what is the weather today"); got != "" {
		t.Errorf("expected empty injection for non-matching task; got: %q", got)
	}
}

// TestInjector_NilRegistry verifies the injector is safe with a nil registry.
func TestInjector_NilRegistry(t *testing.T) {
	inj := NewInjector(nil, nil)
	if got := inj.Inject("anything"); got != "" {
		t.Errorf("expected empty injection with nil registry; got: %q", got)
	}
}

// fakeInjector is a minimal Injector for composite tests.
type fakeInjector struct{ text string }

func (f fakeInjector) Inject(_ string) string { return f.text }

// TestCompositeInjector_Concatenates verifies the composite joins injector
// outputs with a blank-line separator.
func TestCompositeInjector_Concatenates(t *testing.T) {
	c := CompositeInjector{
		Injectors: []Injector{
			fakeInjector{"MEMORY-CTX"},
			fakeInjector{"SKILL-CTX"},
		},
	}
	got := c.Inject("task")
	if got != "MEMORY-CTX\n\nSKILL-CTX" {
		t.Errorf("composite = %q; want MEMORY-CTX\\n\\nSKILL-CTX", got)
	}
}

// TestCompositeInjector_SkipsEmpty verifies empty injector outputs are dropped
// (no leading/trailing/double separators).
func TestCompositeInjector_SkipsEmpty(t *testing.T) {
	c := CompositeInjector{
		Injectors: []Injector{
			fakeInjector{""},
			fakeInjector{"ONLY"},
			fakeInjector{""},
		},
	}
	got := c.Inject("task")
	if got != "ONLY" {
		t.Errorf("composite = %q; want ONLY", got)
	}
}

// TestCompositeInjector_AllEmpty verifies all-empty yields empty.
func TestCompositeInjector_AllEmpty(t *testing.T) {
	c := CompositeInjector{
		Injectors: []Injector{fakeInjector{""}, fakeInjector{""}},
	}
	if got := c.Inject("task"); got != "" {
		t.Errorf("expected empty; got %q", got)
	}
}

// Compile-time guard: Injector must be context-compatible (unused import is
// intentional to keep the signature flexible for future ctx-aware injectors).
var _ = context.Background

// TestInjector_TruncatesSingleLargeSkillToMaxTotalChars verifies that when a
// single matched skill's body exceeds MaxTotalChars, the injector truncates
// the output to respect the cap. Without this, a 10k-char skill body bypasses
// a 4k-char cap because the matcher includes the first skill unconditionally.
func TestInjector_TruncatesSingleLargeSkillToMaxTotalChars(t *testing.T) {
	reg := &Registry{
		skills: []PromptSkill{
			{
				Name:     "big-skill",
				Priority: "high",
				Triggers: Triggers{Keywords: []string{"alpha"}},
				Body:     strings.Repeat("x", 5000),
			},
		},
		byName: map[string]int{"big-skill": 0},
	}
	inj := NewInjectorWithOptions(reg, nil, MatchOptions{MaxSkills: 4, MaxTotalChars: 1000})

	got := inj.Inject("alpha task")
	if len(got) > 1000 {
		t.Errorf("injection is %d chars, exceeds MaxTotalChars cap of 1000; "+
			"single large skills must be truncated to respect the cap", len(got))
	}
}
