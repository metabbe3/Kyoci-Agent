package gateway

import (
	"regexp"
	"strings"
)

// Markdown→HTML and message-sizing helpers for Telegram's HTML parse mode.
// These are pure functions (no gateway state), extracted from telegram.go for
// testability. The markdown regexps are pre-compiled at package scope so they
// compile once instead of on every markdownToHTML call.

var (
	mdCodeBlockRe   = regexp.MustCompile("(?s)```\\w*\\n?(.*?)```")
	mdInlineCodeRe  = regexp.MustCompile("`([^`]+)`")
	mdBoldRe        = regexp.MustCompile(`\*\*(.+?)\*\*`)
	mdBoldUnderRe   = regexp.MustCompile(`__(.+?)__`)
	mdItalicRe      = regexp.MustCompile(`\*(.+?)\*`)
	mdItalicUnderRe = regexp.MustCompile(`(^|\s)_([^_]+?)_(\s|$)`)
	mdStrikeRe      = regexp.MustCompile(`~~(.+?)~~`)
	mdSpoilerRe     = regexp.MustCompile(`\|\|(.+?)\|\|`)
	mdLinkRe        = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	mdHeadingRe     = regexp.MustCompile(`(?m)^(#{1,3})\s+(.+)$`)
	mdQuoteRe       = regexp.MustCompile(`(?m)^&gt;\s*(.+)$`)
	mdNewlinesRe    = regexp.MustCompile(`\n{4,}`)
)

// truncateForTG truncates text for Telegram display.
func truncateForTG(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// splitMessage splits a long message into chunks that fit within Telegram's
// 4096 character limit. Splits on newline boundaries when possible.
func splitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	var chunks []string
	for len(text) > 0 {
		if len(text) <= maxLen {
			chunks = append(chunks, text)
			break
		}

		// Try to split at a newline near the limit
		splitAt := maxLen
		for i := maxLen; i > maxLen/2; i-- {
			if i < len(text) && text[i] == '\n' {
				splitAt = i + 1
				break
			}
		}

		chunks = append(chunks, text[:splitAt])
		text = text[splitAt:]
	}

	return chunks
}

// htmlEscape escapes special HTML characters for Telegram's HTML parse mode.
func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// markdownToHTML converts markdown text to Telegram HTML format.
// Supports: **bold**, *italic*, ~~strike~~, __underline__, `code`, ```code blocks```,
// > quotes, # headings, [links](url).
// First escapes HTML chars, then applies markdown replacements.
func markdownToHTML(input string) string {
	// Escape HTML entities first
	s := htmlEscape(input)

	// Code blocks ```...```
	s = mdCodeBlockRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := mdCodeBlockRe.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		return "<pre><code>" + strings.TrimSpace(sub[1]) + "</code></pre>"
	})

	// Inline code `...`
	s = mdInlineCodeRe.ReplaceAllString(s, "<code>$1</code>")

	// Bold **text**
	s = mdBoldRe.ReplaceAllString(s, "<b>$1</b>")

	// Bold __text__ (alternative)
	s = mdBoldUnderRe.ReplaceAllString(s, "<b>$1</b>")

	// Italic *text* (but not inside bold)
	s = mdItalicRe.ReplaceAllString(s, "<i>$1</i>")

	// Italic _text_ (but not word_parts)
	s = mdItalicUnderRe.ReplaceAllString(s, "$1<i>$2</i>$3")

	// Strikethrough ~~text~~
	s = mdStrikeRe.ReplaceAllString(s, "<s>$1</s>")

	// Spoiler ||text||
	s = mdSpoilerRe.ReplaceAllString(s, "<tg-spoiler>$1</tg-spoiler>")

	// Links [text](url)
	s = mdLinkRe.ReplaceAllString(s, `<a href="$2">$1</a>`)

	// Headings: # → bold
	s = mdHeadingRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := mdHeadingRe.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		return "\n<b>" + sub[2] + "</b>"
	})

	// Block quotes > text
	s = mdQuoteRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := mdQuoteRe.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		return "<blockquote>" + sub[1] + "</blockquote>"
	})

	// Clean up: collapse multiple newlines
	s = mdNewlinesRe.ReplaceAllString(s, "\n\n\n")

	return s
}
