package builtin

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// EmojiInfoSkill reports the name and codepoints for emoji(s) in the query.
type EmojiInfoSkill struct {
	*kyoci.BaseSkill
}

// NewEmojiInfoSkill creates a new emoji info skill.
func NewEmojiInfoSkill() *EmojiInfoSkill {
	return &EmojiInfoSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"emojinfo",
			"Report name and codepoints for emoji(s) in the query",
			[]string{"emoji", "emoji info", "codepoint"},
		),
	}
}

// Match returns true if the query contains an emoji-ish rune or the word "emoji".
func (s *EmojiInfoSkill) Match(query string) bool {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	if strings.Contains(queryLower, "emoji") {
		return true
	}
	for _, r := range query {
		if r > 0x2600 {
			return true
		}
	}
	return false
}

// Execute scans the query for non-ASCII runes and reports their details.
func (s *EmojiInfoSkill) Execute(ctx context.Context, query string) (string, error) {
	// Strip the trigger word "emoji" from consideration.
	cleaned := strings.ReplaceAll(strings.ToLower(query), "emoji", " ")

	// Group consecutive non-ASCII runs into single entries (multi-codepoint emoji).
	type group struct {
		runes []rune
	}
	var groups []group
	current := group{}
	flush := func() {
		if len(current.runes) > 0 {
			groups = append(groups, current)
			current = group{}
		}
	}

	for _, r := range cleaned {
		if r > 0x7F {
			current.runes = append(current.runes, r)
		} else {
			flush()
		}
	}
	flush()

	if len(groups) == 0 {
		return "", fmt.Errorf("no emoji or non-ASCII runes found")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Found %d emoji/non-ASCII group(s):\n\n", len(groups))
	for i, g := range groups {
		// Skip groups that are pure variation selectors or combining marks without a base.
		hasBase := false
		for _, r := range g.runes {
			if !unicode.Is(unicode.Variation_Selector, r) && !unicode.Is(unicode.Mark, r) {
				hasBase = true
				break
			}
		}
		if !hasBase {
			continue
		}

		// Combined glyph.
		fmt.Fprintf(&b, "%d) %s\n", i+1, string(g.runes))

		// Per-rune detail.
		for _, r := range g.runes {
			cp := fmt.Sprintf("U+%04X", r)
			goLit := fmt.Sprintf("\\U%08X", r)
			cat := categoryLabel(r)
			name := nameOrNil(r)
			if name != "" {
				fmt.Fprintf(&b, "   - %s  Go: %s  category: %s  name: %s\n", cp, goLit, cat, name)
			} else {
				fmt.Fprintf(&b, "   - %s  Go: %s  category: %s\n", cp, goLit, cat)
			}
			// Sanity check encoding length (avoids unused import).
			_ = utf8.RuneLen(r)
		}
		// Multibyte byte length of the group.
		fmt.Fprintf(&b, "   bytes: %d\n", len(string(g.runes)))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// categoryLabel returns a short label for a rune's Unicode category.
func categoryLabel(r rune) string {
	if unicode.Is(unicode.Variation_Selector, r) {
		return "Variation_Selector"
	}
	if unicode.Is(unicode.Symbol, r) {
		return "Symbol"
	}
	if unicode.Is(unicode.P, r) {
		return "Punctuation"
	}
	if unicode.Is(unicode.Mark, r) {
		return "Mark"
	}
	if unicode.Is(unicode.Number, r) {
		return "Number"
	}
	if unicode.Is(unicode.Letter, r) {
		return "Letter"
	}
	if unicode.IsControl(r) {
		return "Control"
	}
	return "Other"
}

// nameOrNil returns a friendly name for a handful of well-known emoji,
// since the stdlib does not ship the full Unicode character database names.
func nameOrNil(r rune) string {
	switch r {
	case 0x1F600:
		return "GRINNING FACE"
	case 0x1F603:
		return "SMILING FACE WITH OPEN MOUTH"
	case 0x1F604:
		return "SMILING FACE WITH OPEN MOUTH & SMILING EYES"
	case 0x1F60A:
		return "SMILING FACE WITH SMILING EYES"
	case 0x1F60D:
		return "SMILING FACE WITH HEART-EYES"
	case 0x1F622:
		return "CRYING FACE"
	case 0x1F62D:
		return "LOUDLY CRYING FACE"
	case 0x1F44D:
		return "THUMBS UP SIGN"
	case 0x1F44E:
		return "THUMBS DOWN SIGN"
	case 0x1F389:
		return "PARTY POPPER"
	case 0x1F525:
		return "FIRE"
	case 0x1F44C:
		return "OK HAND SIGN"
	case 0x1F600 + 0x29: // 0x1F61D ; quick alias
		return "SQUINTING FACE WITH TONGUE"
	case 0x2764:
		return "HEAVY BLACK HEART"
	case 0x2600:
		return "BLACK SUN WITH RAYS"
	case 0x2601:
		return "CLOUD"
	case 0x2602:
		return "UMBRELLA"
	case 0x2603:
		return "SNOWMAN"
	case 0x2604:
		return "COMET"
	case 0x2615:
		return "HOT BEVERAGE"
	case 0x2705:
		return "WHITE HEAVY CHECK MARK"
	case 0x2728:
		return "SPARKLES"
	case 0x2763:
		return "HEART EXCLAMATION"
	}
	return ""
}
