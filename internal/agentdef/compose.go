package agentdef

import (
	"strings"

	"github.com/metabbe3/Kyoci-Agent/internal/platform"
)

// =====================================================================================
// Shared prompt fragments
//
// Migrated verbatim from internal/role/prompt/parts.go when the role system
// moved to markdown-driven definitions. Centralizing these blocks keeps the
// per-agent markdown bodies short (small models drift on long prompts) and
// prevents drift between agents — the verification rules and delegation block
// read identically whether a Developer or a QA agent is holding them.
//
// Every agent body composes these parts:
//
//	<agent-specific identity, mandatory rules, tool examples>   (from .md body)
//	<VerificationRules>
//	<DelegationBlock>
//	"Keep responses SHORT. Execute. Verify. Report."
// =====================================================================================

// platformSection is computed once at package init — detects OS and builds
// the full command reference table for the detected platform. Injected into
// agent bodies via the {{platform}} token at load time.
var platformSection = platform.GetPlatformSection()

// VerificationRules is the canonical "investigate, don't ask" block. Tuned
// for 8B-14B models: imperative verbs, concrete examples, no abstract nouns.
// Appended by Compose to every agent body.
const VerificationRules = `VERIFICATION RULES:
- When a command fails, READ the error output, fix the root cause, retry. Try at least 2 approaches before reporting failure.
- NEVER say "please provide more details" or "please share the error". Use your tools to investigate and fix it yourself.
- Only say "Done" after you have VERIFIED the result yourself (curl returns 200, build succeeds, file exists, test passes).`

// DelegationBlock teaches every agent that the `delegation` tool exists and
// tells it when to reach for it. Without this block the tool is invisible —
// small models never call a tool they haven't been told about.
const DelegationBlock = `DELEGATION:
- If a subtask fits another specialist (Developer, Frontend, QA, SRE, PM) or is open-ended research, call the delegation tool with action="spawn", goal="<one focused sentence>".
- Use action="wait_all" before reporting Done so sub-agents finish first.
- Max 3 concurrent sub-agents. Each gets a 180s budget — give it a single, complete goal.`

// ClosingDirective is the standard one-line closer that signals the agent's
// output style: terse, action-first, honest.
const ClosingDirective = "Keep responses SHORT. Execute. Verify. Report."

// Compose glues the agent body together with the shared parts in the standard
// order: body → verification → delegation → closer. body is expected to be
// the agent-specific identity + rules + tool examples (no trailing newline).
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

// SubstitutePlatformTokens replaces every supported {{token}} placeholder in
// body with its runtime-computed value. Today only {{platform}} is supported
// — it expands to the OS-specific command reference table detected at init.
//
// This is intentionally a fixed token enum, not a general templating system:
// no path traversal, no cyclic includes, no user-defined tokens. The set of
// substitutions is compile-time-bounded.
func SubstitutePlatformTokens(body string) string {
	return strings.ReplaceAll(body, "{{platform}}", platformSection)
}
