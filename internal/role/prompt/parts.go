// Package prompt holds the shared prompt fragments reused across every role
// system prompt. Centralizing these blocks keeps the specialist prompts short
// (small models drift on long prompts) and prevents drift between roles —
// e.g. the verification rules and delegation block should read identically
// whether a Developer or a QA agent is holding them.
//
// Every role prompt composes these parts:
//
//	<role-specific identity, mandatory rules, tool examples>
//	<prompt.VerificationRules>
//	<prompt.DelegationBlock>
//	"Keep responses SHORT. Execute. Verify. Report."
package prompt

import "strings"

// VerificationRules is the canonical "investigate, don't ask" block. Tuned
// for 8B-14B models: imperative verbs, concrete examples, no abstract nouns.
// Appended by every specialist role.
const VerificationRules = `VERIFICATION RULES:
- When a command fails, READ the error output, fix the root cause, retry. Try at least 2 approaches before reporting failure.
- NEVER say "please provide more details" or "please share the error". Use your tools to investigate and fix it yourself.
- Only say "Done" after you have VERIFIED the result yourself (curl returns 200, build succeeds, file exists, test passes).`

// DelegationBlock teaches every specialist that the `delegation` tool exists
// and tells it when to reach for it. Without this block the tool is invisible
// — small models never call a tool they haven't been told about.
const DelegationBlock = `DELEGATION:
- If a subtask fits another specialist (Developer, Frontend, QA, SRE, PM) or is open-ended research, call the delegation tool with action="spawn", goal="<one focused sentence>".
- Use action="wait_all" before reporting Done so sub-agents finish first.
- Max 3 concurrent sub-agents. Each gets a 180s budget — give it a single, complete goal.`

// ClosingDirective is the standard one-line closer that signals the role's
// output style: terse, action-first, honest.
const ClosingDirective = "Keep responses SHORT. Execute. Verify. Report."

// RuleList formats a "MANDATORY RULES:" header followed by bulleted items.
// Each item is rendered as "- <item>". Empty items are skipped. Returns an
// empty string when no items remain after filtering.
func RuleList(items ...string) string {
	var b strings.Builder
	b.WriteString("MANDATORY RULES:\n")
	for _, item := range items {
		t := strings.TrimSpace(item)
		if t == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(t)
		b.WriteByte('\n')
	}
	return b.String()
}

// Compose glues the role body together with the shared parts in the standard
// order: body → verification → delegation → closer. body is expected to be
// the role-specific identity + rules + tool examples (no trailing newline).
func Compose(body string) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(body, "\n"))
	b.WriteString("\n\n")
	b.WriteString(VerificationRules)
	b.WriteString("\n\n")
	b.WriteString(DelegationBlock)
	b.WriteString("\n\n")
	b.WriteString(ClosingDirective)
	return b.String()
}
