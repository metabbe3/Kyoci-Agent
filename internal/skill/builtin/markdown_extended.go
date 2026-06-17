package builtin

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Markdown skills — extensions to the general MarkdownSkill. These handle
// specific operations (outline, TOC, strip formatting, link extraction).
// =====================================================================================

// ---- markdown_outline ----

type MarkdownOutlineSkill struct{ *kyoci.BaseSkill }

func NewMarkdownOutlineSkill() *MarkdownOutlineSkill {
	return &MarkdownOutlineSkill{BaseSkill: kyoci.NewBaseSkill(
		"markdown_outline", "Extract a heading outline from markdown (indented by depth)",
		[]string{"markdown outline", "outline of markdown", "extract markdown outline"},
	)}
}
func (s *MarkdownOutlineSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "markdown outline") || strings.Contains(q, "outline of markdown") ||
		strings.Contains(q, "extract markdown outline")
}
func (s *MarkdownOutlineSkill) Execute(_ context.Context, q string) (string, error) {
	text := extractPayload(q)
	if text == "" {
		text = q
	}
	var b strings.Builder
	headingRe := regexp.MustCompile(`(?m)^(#{1,6})\s+(.+?)\s*$`)
	for _, m := range headingRe.FindAllStringSubmatch(text, -1) {
		depth := len(m[1])
		indent := strings.Repeat("  ", depth-1)
		fmt.Fprintf(&b, "%s%s\n", indent, strings.TrimSpace(m[2]))
	}
	out := strings.TrimRight(b.String(), "\n")
	if out == "" {
		return "(no headings found)", nil
	}
	return out, nil
}

// ---- markdown_toc ----

type MarkdownTOCSkill struct{ *kyoci.BaseSkill }

func NewMarkdownTOCSkill() *MarkdownTOCSkill {
	return &MarkdownTOCSkill{BaseSkill: kyoci.NewBaseSkill(
		"markdown_toc", "Generate a table-of-contents with anchor links from markdown",
		[]string{"markdown toc", "table of contents", "generate toc"},
	)}
}
func (s *MarkdownTOCSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "markdown toc") || strings.Contains(q, "table of contents") ||
		strings.Contains(q, "generate toc")
}
func (s *MarkdownTOCSkill) Execute(_ context.Context, q string) (string, error) {
	text := extractPayload(q)
	if text == "" {
		text = q
	}
	headingRe := regexp.MustCompile(`(?m)^(#{1,6})\s+(.+?)\s*$`)
	var b strings.Builder
	for _, m := range headingRe.FindAllStringSubmatch(text, -1) {
		depth := len(m[1])
		title := strings.TrimSpace(m[2])
		anchor := slugifyAnchor(title)
		indent := strings.Repeat("  ", depth-1)
		fmt.Fprintf(&b, "%s- [%s](#%s)\n", indent, title, anchor)
	}
	out := strings.TrimRight(b.String(), "\n")
	if out == "" {
		return "(no headings found)", nil
	}
	return out, nil
}

// slugifyAnchor produces a GitHub-flavored anchor from a heading: lowercase,
// spaces to hyphens, strip non-word chars except hyphens.
func slugifyAnchor(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	out := strings.Builder{}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// ---- markdown_strip ----

type MarkdownStripSkill struct{ *kyoci.BaseSkill }

func NewMarkdownStripSkill() *MarkdownStripSkill {
	return &MarkdownStripSkill{BaseSkill: kyoci.NewBaseSkill(
		"markdown_strip", "Strip markdown formatting → plain text",
		[]string{"markdown strip", "strip markdown", "markdown to text", "md to text"},
	)}
}
func (s *MarkdownStripSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "markdown strip") || strings.Contains(q, "strip markdown") ||
		strings.Contains(q, "markdown to text") || strings.Contains(q, "md to text")
}
func (s *MarkdownStripSkill) Execute(_ context.Context, q string) (string, error) {
	text := extractPayload(q)
	if text == "" {
		text = q
	}
	// Headers
	text = regexp.MustCompile(`(?m)^#{1,6}\s+`).ReplaceAllString(text, "")
	// Bold/italic
	text = regexp.MustCompile(`\*\*([^*]+)\*\*`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`__([^_]+)__`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`\*([^*]+)\*`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`_([^_]+)_`).ReplaceAllString(text, "$1")
	// Strikethrough
	text = regexp.MustCompile(`~~([^~]+)~~`).ReplaceAllString(text, "$1")
	// Inline code + code blocks
	text = regexp.MustCompile("`([^`]+)`").ReplaceAllString(text, "$1")
	text = regexp.MustCompile("(?s)```[a-z]*\\n(.*?)\\n```").ReplaceAllString(text, "$1")
	// Images (alt text survives)
	text = regexp.MustCompile(`!\[([^\]]*)\]\([^)]+\)`).ReplaceAllString(text, "$1")
	// Links
	text = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`).ReplaceAllString(text, "$1")
	// Blockquotes
	text = regexp.MustCompile(`(?m)^>\s*`).ReplaceAllString(text, "")
	// List bullets
	text = regexp.MustCompile(`(?m)^[-*+]\s+`).ReplaceAllString(text, "")
	text = regexp.MustCompile(`(?m)^\d+\.\s+`).ReplaceAllString(text, "")
	// Horizontal rules
	text = regexp.MustCompile(`(?m)^[-*_]{3,}\s*$`).ReplaceAllString(text, "")
	// Collapse runs of blank lines.
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text), nil
}

// ---- markdown_link_extract ----

type MarkdownLinkExtractSkill struct{ *kyoci.BaseSkill }

func NewMarkdownLinkExtractSkill() *MarkdownLinkExtractSkill {
	return &MarkdownLinkExtractSkill{BaseSkill: kyoci.NewBaseSkill(
		"markdown_link_extract", "Extract all URLs from markdown text",
		[]string{"extract links", "link extract", "urls in markdown", "find urls in"},
	)}
}
func (s *MarkdownLinkExtractSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "extract links") || strings.Contains(q, "link extract") ||
		strings.Contains(q, "urls in markdown") || strings.Contains(q, "find urls in")
}
func (s *MarkdownLinkExtractSkill) Execute(_ context.Context, q string) (string, error) {
	text := extractPayload(q)
	if text == "" {
		text = q
	}
	// [text](url) — capture url
	linkRe := regexp.MustCompile(`\[([^\]]*)\]\(([^)]+)\)`)
	// Bare URLs
	bareRe := regexp.MustCompile(`(?:(?:https?|ftp)://)[^\s)\]]+`)

	seen := map[string]bool{}
	var out []string

	for _, m := range linkRe.FindAllStringSubmatch(text, -1) {
		u := strings.TrimSpace(m[2])
		if !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	for _, m := range bareRe.FindAllString(text, -1) {
		u := strings.TrimSpace(m)
		if !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	if len(out) == 0 {
		return "(no URLs found)", nil
	}
	return strings.Join(out, "\n"), nil
}
