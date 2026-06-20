package agent

import (
	"context"
	"fmt"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Deep Research worker — multi-source web research with cited report output.
//
// Triggered by the "research:" delegation prefix. Routes to a read-only worker
// that has web_search + web_fetch + file:read + glob + grep tools. Returns a
// structured Markdown report with numbered citations [1], [2], etc.
//
// Unlike the explore worker (which investigates the local codebase), the
// research worker investigates the WEB — fetching multiple sources, reading
// them, and synthesizing a cited report.
// =====================================================================================

// ResearchSystemPrompt directs the model to research, synthesize, and cite.
const ResearchSystemPrompt = `/no_think

You are a Deep Research agent. Your job is to investigate a topic using web search and web fetch tools, then produce a STRUCTURED RESEARCH REPORT with citations.

WORKFLOW:
1. Search for the topic using web_search (at least 3 different queries).
2. Read the top 3-5 results using web_fetch.
3. Synthesize what you found into a coherent report.

OUTPUT FORMAT (Markdown only — no preamble, no closing remarks):

## Summary
[2-3 sentence direct answer to the research question]

## Key Findings
- Finding 1 [1]
- Finding 2 [2]
- Finding 3 [1,3]

## Detailed Analysis
[2-4 paragraphs synthesizing the sources. Cite inline as [N] where N maps to the source list.]

## Sources
[1] Title — URL
[2] Title — URL
[3] Title — URL

RULES:
- Every factual claim MUST have a citation [N].
- Only cite sources you actually read via web_fetch.
- If sources disagree, note the disagreement honestly.
- If you cannot find enough sources, say so explicitly.
- Do NOT make up URLs or sources.
- Do NOT include sources you didn't read.`

// ResearchToolAllowlist — the research worker gets web tools + read-only file tools.
var ResearchToolAllowlist = map[string]bool{
	"web_search": true,
	"web_fetch":  true,
	"http_client": true,
	"file":       true, // file:read for local context
	"glob":       true,
	"grep":       true,
	"todo":       true, // planning aid
}

// ResearchWorker runs a web-research investigation and returns a cited report.
func (a *Agent) ResearchWorker(ctx context.Context, question string) (string, error) {
	research := NewAgent(
		a.config,
		a.router,
		NewReadOnlyToolFilter(a.tools, ResearchToolAllowlist),
		a.skills,
		a.memory,
		WithLogger(a.logger),
	)
	research.config.SystemPrompt = ResearchSystemPrompt
	research.config.Orchestration.Enabled = false
	research.config.EnableThinking = false
	research.config.EnableSkills = false
	research.config.MaxIterations = 20 // research needs more iterations than explore
	// Route to cloud — research needs reasoning + web access
	if route := a.config.Orchestration.ModelRouting.QA; route.Provider != "" {
		research.config.PreferredProvider = route.Provider
		research.config.Model = route.Model
	}

	result, err := research.Execute(ctx, question)
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

// HasResearchPrefix reports whether goal is a research request.
func HasResearchPrefix(goal string) bool {
	low := strings.ToLower(strings.TrimSpace(goal))
	return strings.HasPrefix(low, "research:") || strings.HasPrefix(low, "research ")
}

// StripResearchPrefix removes the "research:" / "research " prefix.
func StripResearchPrefix(goal string) string {
	out := strings.TrimSpace(goal)
	low := strings.ToLower(out)
	if strings.HasPrefix(low, "research:") {
		return strings.TrimSpace(out[len("research:"):])
	}
	if strings.HasPrefix(low, "research ") {
		return strings.TrimSpace(out[len("research "):])
	}
	return out
}

// =====================================================================================
// Model Compare — blind side-by-side model testing.
//
// Triggered by the "compare:" delegation prefix. Sends the same prompt to two
// different providers/models and returns both responses side by side.
// =====================================================================================

// CompareResult holds the two model responses for comparison.
type CompareResult struct {
	Prompt    string
	ModelA    string
	ResponseA string
	ModelB    string
	ResponseB string
}

// RunCompare sends the same prompt to two providers and returns both responses.
func (a *Agent) RunCompare(ctx context.Context, prompt string) (*CompareResult, error) {
	// Determine two providers to compare
	providerA := "lmstudio"
	modelA := a.config.Model
	providerB := "anthropic"
	modelB := "glm-5.2"

	// Override from routing config if available
	if route := a.config.Orchestration.ModelRouting.Worker; route.Provider != "" {
		providerA = route.Provider
		modelA = route.Model
	}
	if route := a.config.Orchestration.ModelRouting.Planner; route.Provider != "" {
		providerB = route.Provider
		modelB = route.Model
	}

	messages := []kyoci.Message{
		{Role: kyoci.RoleSystem, Content: "Answer the user's question concisely and accurately."},
		{Role: kyoci.RoleUser, Content: prompt},
	}

	// Send to model A
	respA, errA := a.router.Route(ctx, kyoci.CompletionRequest{
		Messages:    messages,
		Temperature: 0.3,
		MaxTokens:   2048,
		Model:       modelA,
	}, providerA)
	responseA := ""
	if errA != nil {
		responseA = fmt.Sprintf("Error: %v", errA)
	} else if respA != nil {
		responseA = respA.Content
	}

	// Send to model B
	respB, errB := a.router.Route(ctx, kyoci.CompletionRequest{
		Messages:    messages,
		Temperature: 0.3,
		MaxTokens:   2048,
		Model:       modelB,
	}, providerB)
	responseB := ""
	if errB != nil {
		responseB = fmt.Sprintf("Error: %v", errB)
	} else if respB != nil {
		responseB = respB.Content
	}

	return &CompareResult{
		Prompt:    prompt,
		ModelA:    fmt.Sprintf("%s/%s", providerA, modelA),
		ResponseA: responseA,
		ModelB:    fmt.Sprintf("%s/%s", providerB, modelB),
		ResponseB: responseB,
	}, nil
}

// FormatCompareReport renders a CompareResult as Markdown for the chat.
func FormatCompareReport(r *CompareResult) string {
	var b strings.Builder
	b.WriteString("## Model Comparison\n\n")
	b.WriteString(fmt.Sprintf("**Prompt:** %s\n\n", r.Prompt))
	b.WriteString("---\n\n")
	b.WriteString(fmt.Sprintf("### Model A: %s\n\n", r.ModelA))
	b.WriteString(r.ResponseA)
	b.WriteString("\n\n---\n\n")
	b.WriteString(fmt.Sprintf("### Model B: %s\n\n", r.ModelB))
	b.WriteString(r.ResponseB)
	b.WriteString("\n\n---\n\n")
	b.WriteString("*Which response is better? You decide.*")
	return b.String()
}

// HasComparePrefix reports whether goal is a compare request.
func HasComparePrefix(goal string) bool {
	low := strings.ToLower(strings.TrimSpace(goal))
	return strings.HasPrefix(low, "compare:") || strings.HasPrefix(low, "compare ")
}

// StripComparePrefix removes the "compare:" / "compare " prefix.
func StripComparePrefix(goal string) string {
	out := strings.TrimSpace(goal)
	low := strings.ToLower(out)
	if strings.HasPrefix(low, "compare:") {
		return strings.TrimSpace(out[len("compare:"):])
	}
	if strings.HasPrefix(low, "compare ") {
		return strings.TrimSpace(out[len("compare "):])
	}
	return out
}
