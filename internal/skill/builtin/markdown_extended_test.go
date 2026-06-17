package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// Markdown skill tests — 4 skills (outline, toc, strip, link_extract).
// =====================================================================================

const sampleMarkdown = `# Title

Intro paragraph.

## Section A

Some content.

### Subsection

More content.

## Section B [link](https://example.com)

End.`

func TestMarkdownOutlineSkill(t *testing.T) {
	skill := NewMarkdownOutlineSkill()
	if !skill.Match("markdown outline of this") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "markdown outline: "+sampleMarkdown)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Outline should include all three headings, indented by depth.
	if !strings.Contains(out, "Title") {
		t.Errorf("expected 'Title' heading, got %q", out)
	}
	if !strings.Contains(out, "Section A") {
		t.Errorf("expected 'Section A' heading, got %q", out)
	}
	if !strings.Contains(out, "Subsection") {
		t.Errorf("expected 'Subsection' heading, got %q", out)
	}
	// Subsection should be more indented than Section A.
	if !strings.Contains(out, "    Subsection") {
		t.Errorf("expected deeper indent for Subsection, got %q", out)
	}
}

func TestMarkdownTOCSkill(t *testing.T) {
	skill := NewMarkdownTOCSkill()
	if !skill.Match("markdown toc of this") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "markdown toc: "+sampleMarkdown)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// TOC entries are markdown links "- [Title](#title)".
	if !strings.Contains(out, "[Title](#title)") {
		t.Errorf("expected Title anchor link, got %q", out)
	}
	if !strings.Contains(out, "[Section A](#section-a)") {
		t.Errorf("expected Section A anchor, got %q", out)
	}
}

func TestMarkdownStripSkill(t *testing.T) {
	skill := NewMarkdownStripSkill()
	if !skill.Match("markdown strip: "+sampleMarkdown) {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "markdown strip: "+sampleMarkdown)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Stripped text should have no '#' characters.
	if strings.Contains(out, "#") {
		t.Errorf("expected no markdown heading chars, got %q", out)
	}
	// Should still contain the content text.
	if !strings.Contains(out, "Title") || !strings.Contains(out, "Section A") {
		t.Errorf("expected content preserved, got %q", out)
	}
}

func TestMarkdownLinkExtractSkill(t *testing.T) {
	skill := NewMarkdownLinkExtractSkill()
	if !skill.Match("extract links from this") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "extract links from: "+sampleMarkdown)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "https://example.com") {
		t.Errorf("expected example.com URL extracted, got %q", out)
	}
}
