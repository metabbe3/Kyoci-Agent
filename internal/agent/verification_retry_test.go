package agent

import (
	"strings"
	"testing"
)

// =====================================================================================
// extractClaimedFiles — verifies the prose-scanning helper that feeds the
// verification retry nudge. Pinned because the retry loop is only useful when
// this correctly extracts the model's claimed filenames.
// =====================================================================================

func TestExtractClaimedFiles_ConfigExtensions(t *testing.T) {
	prose := "I created projects/calculator/package.json and tsconfig.json, plus src/main.ts."
	got := extractClaimedFiles(prose)
	want := []string{"package.json", "tsconfig.json", "main.ts"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, g := range got {
		if g != want[i] {
			t.Errorf("[%d] got %q, want %q", i, g, want[i])
		}
	}
}

func TestExtractClaimedFiles_NoCodeExtensions(t *testing.T) {
	got := extractClaimedFiles("nothing to see here, just prose")
	if got != nil {
		t.Errorf("expected nil for plain prose, got %v", got)
	}
}

func TestExtractClaimedFiles_DedupesAndBasenames(t *testing.T) {
	// Same file mentioned twice with different paths → one basename entry.
	prose := "I wrote projects/calculator/src/main.ts. Then I updated projects/calculator/src/main.ts again."
	got := extractClaimedFiles(prose)
	if len(got) != 1 || got[0] != "main.ts" {
		t.Errorf("expected [main.ts], got %v", got)
	}
}

func TestExtractClaimedFiles_Empty(t *testing.T) {
	if got := extractClaimedFiles(""); got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
}

func TestExtractClaimedFiles_StyledExtensions(t *testing.T) {
	// Modern frontend files — .tsx, .jsx, .scss, .vue, .svelte.
	prose := "I created App.tsx, styles.scss, and Button.vue"
	got := extractClaimedFiles(prose)
	if len(got) != 3 {
		t.Fatalf("got %v, want 3 files", got)
	}
}

// =====================================================================================
// VerificationRetryNudge — the prompt text. Pinned because the model's
// behavior depends on the exact wording (file-specific, demand tool calls).
// =====================================================================================

func TestVerificationRetryNudge_NamesEachFile(t *testing.T) {
	claimed := []string{"package.json", "tsconfig.json", "main.ts"}
	nudge := VerificationRetryNudge(claimed)
	for _, f := range claimed {
		if !strings.Contains(nudge, f) {
			t.Errorf("nudge missing file %q:\n%s", f, nudge)
		}
	}
}

func TestVerificationRetryNudge_DemandsFileWrite(t *testing.T) {
	nudge := VerificationRetryNudge([]string{"main.ts"})
	low := strings.ToLower(nudge)
	for _, want := range []string{"file:write", "verification failed", "content", "main.ts"} {
		if !strings.Contains(low, strings.ToLower(want)) {
			t.Errorf("nudge missing %q:\n%s", want, nudge)
		}
	}
}

func TestVerificationRetryNudge_EmptyClaimedList(t *testing.T) {
	// Defensive — caller should check, but the function should not crash.
	nudge := VerificationRetryNudge(nil)
	if !strings.Contains(nudge, "file:write") {
		t.Errorf("empty-claims fallback should still demand file:write: %q", nudge)
	}
}

func TestVerificationRetryNudge_FirstFileStarts(t *testing.T) {
	// The nudge ends with "Start with <first-file> NOW" — pins the model's
	// attention on the first task instead of letting it pick.
	claimed := []string{"package.json", "tsconfig.json"}
	nudge := VerificationRetryNudge(claimed)
	if !strings.HasSuffix(nudge, "Start with package.json NOW.") {
		t.Errorf("nudge should end with first-file directive, got tail: %q",
			nudge[len(nudge)-min(80, len(nudge)):])
	}
}
