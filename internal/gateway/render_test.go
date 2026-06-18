package gateway

import (
	"strings"
	"testing"
)

func TestTruncateForTG(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in     string
		max    int
		want   string
		suffix bool // expect "..." appended
	}{
		{"hello", 10, "hello", false},
		{"  hello  ", 10, "hello", false},
		{"hello world", 5, "hello", true},
	}
	for _, c := range cases {
		got := truncateForTG(c.in, c.max)
		if c.suffix {
			if !strings.HasSuffix(got, "...") {
				t.Errorf("truncateForTG(%q,%d) = %q, want suffix ...", c.in, c.max, got)
			}
			continue
		}
		if got != c.want {
			t.Errorf("truncateForTG(%q,%d) = %q, want %q", c.in, c.max, got, c.want)
		}
	}
}

func TestSplitMessage(t *testing.T) {
	t.Parallel()
	// Under limit → single chunk.
	got := splitMessage("short", 100)
	if len(got) != 1 || got[0] != "short" {
		t.Errorf("splitMessage(short) = %v", got)
	}
	// Over limit → multiple chunks, each within limit.
	long := strings.Repeat("a", 50) + "\n" + strings.Repeat("b", 50) + "\n" + strings.Repeat("c", 50)
	chunks := splitMessage(long, 60)
	if len(chunks) < 2 {
		t.Fatalf("splitMessage: expected >=2 chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > 60 {
			t.Errorf("chunk %d len %d > 60", i, len(c))
		}
	}
}

func TestHTMLEscape(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"a&b":        "a&amp;b",
		"<tag>":      "&lt;tag&gt;",
		`"plain"`:    `"plain"`,
		"x<y>&z":     "x&lt;y&gt;&amp;z",
	}
	for in, want := range cases {
		if got := htmlEscape(in); got != want {
			t.Errorf("htmlEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMarkdownToHTML(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		in    string
		want  string
	}{
		{"bold", "**hi**", "<b>hi</b>"},
		{"italic", "*hi*", "<i>hi</i>"},
		{"strike", "~~hi~~", "<s>hi</s>"},
		{"inline-code", "`x`", "<code>x</code>"},
		{"link", "[t](https://u)", `<a href="https://u">t</a>`},
		{"escape-html", "a<b>&c", "a&lt;b&gt;&amp;c"},
		{"spoiler", "||secret||", "<tg-spoiler>secret</tg-spoiler>"},
	}
	for _, c := range cases {
		if got := markdownToHTML(c.in); got != c.want {
			t.Errorf("markdownToHTML(%s) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestMarkdownCodeBlock(t *testing.T) {
	t.Parallel()
	in := "```go\nfmt.Println()\n```"
	got := markdownToHTML(in)
	if !strings.Contains(got, "<pre><code>") || !strings.Contains(got, "fmt.Println()") {
		t.Errorf("markdownToHTML(codeblock) = %q, missing <pre><code>", got)
	}
}
