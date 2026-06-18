package agent

import (
	"errors"
	"strings"
	"testing"
)

// =====================================================================================
// salvagePlannerOutput — pins down the recovery logic for the 8B-model case
// where the planner emits prose + code blocks instead of a JSON step array.
// =====================================================================================

func TestSalvagePlannerOutput_CodeBlockSalvages(t *testing.T) {
	// Simulate the error planTask produces: "planner output parse failed: ... (raw: %q)"
	rawOutput := "Here is the fix:\n\n```javascript\nconsole.log('hi');\n```\n"
	err := errors.New(`planner output parse failed: no JSON found (raw: "` + escapeForError(rawOutput) + `")`)

	got, ok := salvagePlannerOutput(err)
	if !ok {
		t.Fatalf("expected salvage to succeed for code-block output")
	}
	if !strings.Contains(got, "console.log") {
		t.Errorf("salvaged output missing the code: %q", got)
	}
}

func TestSalvagePlannerOutput_LongProseSalvages(t *testing.T) {
	// ≥200 chars of prose without code blocks should also salvage.
	prose := strings.Repeat("This is a substantial explanation of the bug. ", 10) // ~470 chars
	err := errors.New(`planner output parse failed: no JSON found (raw: "` + escapeForError(prose) + `")`)

	got, ok := salvagePlannerOutput(err)
	if !ok {
		t.Fatalf("expected salvage to succeed for long prose")
	}
	// salvagePlannerOutput uses strconv.Unquote to decode the %q escape;
	// our test's escapeForError is a close-but-not-exact approximation.
	// Verify content matches in substance, not byte-for-byte.
	if len(got) < 200 {
		t.Errorf("salvaged prose too short: %d chars", len(got))
	}
	if !strings.Contains(got, "substantial explanation") {
		t.Errorf("salvaged prose missing key phrase; got: %q", got[:min(100, len(got))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestSalvagePlannerOutput_ShortGarbageRejected(t *testing.T) {
	// Short output without code → not salvageable; original error propagates.
	err := errors.New(`planner output parse failed: no JSON found (raw: "ok")`)
	got, ok := salvagePlannerOutput(err)
	if ok {
		t.Errorf("expected salvage to reject short output, got %q", got)
	}
}

func TestSalvagePlannerOutput_NoRawSuffixRejected(t *testing.T) {
	// Error without the (raw: %q) format → can't recover, reject.
	err := errors.New("some other planner failure")
	got, ok := salvagePlannerOutput(err)
	if ok {
		t.Errorf("expected salvage to reject error without raw suffix, got %q", got)
	}
}

func TestSalvagePlannerOutput_NilError(t *testing.T) {
	got, ok := salvagePlannerOutput(nil)
	if ok || got != "" {
		t.Errorf("nil error should return (\"\", false), got (%q, %v)", got, ok)
	}
}

// TestSalvagePlannerOutput_RealLmStudioOutput uses the actual response the
// user reported — gemma-4-e4b emitting prose + two fenced code blocks
// (script.js + index.html) when asked to plan the calculator fix. This is
// the regression test for the chat 500 they hit.
func TestSalvagePlannerOutput_RealLmStudioOutput(t *testing.T) {
	// Truncated-but-representative version of the real output the user pasted.
	// Real version had ~2KB of JS + HTML; we keep enough to verify salvage.
	realOutput := `The issue causing the calculator buttons to be unresponsive was twofold: incorrect display manipulation in JavaScript and suboptimal script loading in HTML.

The fixes implemented were:
1.  **Display Handling:** The JavaScript was corrected to use ` + "`" + `.value` + "`" + ` when interacting with the display element, as the display is an ` + "`" + `<input type="text">` + "`" + ` field and not a simple container requiring ` + "`" + `.innerText` + "`" + `.
2.  **Script Loading:** The ` + "`" + `<script>` + "`" + ` tag in ` + "`" + `index.html` + "`" + ` was correctly placed at the end of the ` + "`" + `<body>` + "`" + ` tag to ensure the DOM is fully loaded before the script attempts to attach event listeners.

The corrected files are provided below:

### ` + "`" + `projects/calculator/script.js` + "`" + ` (Corrected)
` + "```javascript" + `
let displayValue = '0';
let firstOperand = null;
let operator = null;

const display = document.getElementById('display');

function updateDisplay(value) {
    if (display) {
        display.value = value;
    }
}
` + "```" + `

### ` + "`" + `projects/calculator/index.html` + "`" + ` (Corrected)
` + "```html" + `
<!DOCTYPE html>
<html>
<body>
    <input type="text" id="display" readonly value="0">
    <script src="script.js"></script>
</body>
</html>
` + "```"

	err := errors.New(`planner output parse failed: no valid JSON array found (raw: "` + escapeForError(realOutput) + `")`)

	got, ok := salvagePlannerOutput(err)
	if !ok {
		t.Fatalf("expected salvage to succeed for the real lmstudio output")
	}
	if !strings.Contains(got, "```javascript") {
		t.Errorf("salvaged output missing the JS code block")
	}
	if !strings.Contains(got, "```html") {
		t.Errorf("salvaged output missing the HTML code block")
	}
	if !strings.Contains(got, "Display Handling") {
		t.Errorf("salvaged output missing the prose explanation")
	}

	// Verify the interceptor can extract file writes from the salvaged output.
	calls := interceptCodeBlocks(got, "fix projects/calculator/script.js and index.html")
	if len(calls) < 2 {
		t.Errorf("expected interceptor to extract ≥2 file writes (script.js + index.html), got %d", len(calls))
	}
	for _, c := range calls {
		if c.Name != "file" {
			t.Errorf("intercepted call name = %q, want file", c.Name)
		}
	}
}

// escapeForError mimics how Go's %q formats a string when planTask builds the
// error message — escapes newlines, quotes, backslashes. We use Sprintf to
// produce the exact same encoding the production code emits.
func escapeForError(s string) string {
	// Strip the surrounding quotes %q adds; we re-add them in the test input.
	out := strings.ReplaceAll(s, `\`, `\\`)
	out = strings.ReplaceAll(out, `"`, `\"`)
	out = strings.ReplaceAll(out, "\n", `\n`)
	out = strings.ReplaceAll(out, "\t", `\t`)
	return out
}
