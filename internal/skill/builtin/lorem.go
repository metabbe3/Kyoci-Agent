package builtin

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// loremWords is the classic Lorem Ipsum word pool.
var loremWords = []string{
	"lorem", "ipsum", "dolor", "sit", "amet", "consectetur", "adipiscing", "elit",
	"sed", "do", "eiusmod", "tempor", "incididunt", "ut", "labore", "et", "dolore",
	"magna", "aliqua", "enim", "ad", "minim", "veniam", "quis", "nostrud",
	"exercitation", "ullamco", "laboris", "nisi", "aliquip", "ex", "ea", "commodo",
	"consequat", "duis", "aute", "irure", "in", "reprehenderit", "voluptate",
	"velit", "esse", "cillum", "fugiat", "nulla", "pariatur", "excepteur", "sint",
	"occaecat", "cupidatat", "non", "proident", "sunt", "culpa", "qui", "officia",
	"deserunt", "mollit", "anim", "id", "est", "laborum",
}

// LoremSkill generates Lorem Ipsum placeholder text.
type LoremSkill struct {
	*kyoci.BaseSkill
}

// NewLoremSkill creates a new lorem ipsum skill.
func NewLoremSkill() *LoremSkill {
	return &LoremSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"lorem",
			"Generate Lorem Ipsum placeholder text — words, sentences, or paragraphs",
			[]string{"lorem", "ipsum", "placeholder", "lorem ipsum"},
		),
	}
}

// Match checks if the query is asking for lorem ipsum text.
func (s *LoremSkill) Match(query string) bool {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	if strings.Contains(queryLower, "lorem") || strings.Contains(queryLower, "ipsum") || strings.Contains(queryLower, "placeholder") {
		return true
	}
	return false
}

// Execute generates the requested amount of lorem ipsum text.
func (s *LoremSkill) Execute(ctx context.Context, query string) (string, error) {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	count, unit := parseLoremSpec(queryLower)

	// Deterministic seed derived from query length (no time/random allowed).
	seed := uint32(len(query))
	if seed == 0 {
		seed = 1
	}
	rng := newLoremRng(seed)

	switch unit {
	case "words":
		return generateWords(&rng, count), nil
	case "sentences":
		return generateSentences(&rng, count), nil
	default: // paragraphs
		return generateParagraphs(&rng, count), nil
	}
}

// parseLoremSpec extracts the count and unit from the query.
func parseLoremSpec(query string) (int, string) {
	// Defaults: 1 paragraph, but Execute() splits paragraphs into 5 sentences each.
	fields := strings.Fields(query)
	for i, f := range fields {
		// First, check for "<n> paragraphs|sentences|words".
		if f == "paragraphs" || f == "paragraph" {
			if i > 0 {
				if n, err := strconv.Atoi(fields[i-1]); err == nil && n > 0 {
					return n, "paragraphs"
				}
			}
		}
		if f == "sentences" || f == "sentence" {
			if i > 0 {
				if n, err := strconv.Atoi(fields[i-1]); err == nil && n > 0 {
					return n, "sentences"
				}
			}
		}
		if f == "words" || f == "word" {
			if i > 0 {
				if n, err := strconv.Atoi(fields[i-1]); err == nil && n > 0 {
					return n, "words"
				}
			}
		}
		// Also accept "Np"/"Ns"/"Nw" suffixes.
		if n, suffix, ok := splitNumSuffix(f); ok {
			switch suffix {
			case "p":
				return n, "paragraphs"
			case "s":
				return n, "sentences"
			case "w":
				return n, "words"
			}
		}
	}
	return 1, "paragraphs"
}

// splitNumSuffix splits tokens like "3p" into (3, "p").
func splitNumSuffix(s string) (int, string, bool) {
	if len(s) < 2 {
		return 0, "", false
	}
	// Last rune determines the suffix.
	last := s[len(s)-1]
	if last != 'p' && last != 's' && last != 'w' {
		return 0, "", false
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n <= 0 {
		return 0, "", false
	}
	return n, string(last), true
}

// loremRng is a tiny xorshift PRNG (deterministic; no global state).
type loremRng struct {
	state uint32
}

func newLoremRng(seed uint32) loremRng {
	if seed == 0 {
		seed = 0x9E3779B9
	}
	return loremRng{state: seed}
}

func (r *loremRng) next() uint32 {
	x := r.state
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	r.state = x
	return x
}

func (r *loremRng) intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.next() % uint32(n))
}

// generateWords produces `count` lorem words.
func generateWords(rng *loremRng, count int) string {
	words := make([]string, count)
	for i := 0; i < count; i++ {
		words[i] = loremWords[rng.intn(len(loremWords))]
	}
	if len(words) == 0 {
		return ""
	}
	words[0] = capitalize(words[0])
	out := strings.Join(words, " ")
	out += "."
	return out
}

// generateSentences produces `count` sentences of varying length.
func generateSentences(rng *loremRng, count int) string {
	sentences := make([]string, count)
	for i := 0; i < count; i++ {
		// Each sentence has between 6 and 18 words.
		n := 6 + rng.intn(13)
		words := make([]string, n)
		for j := 0; j < n; j++ {
			words[j] = loremWords[rng.intn(len(loremWords))]
		}
		words[0] = capitalize(words[0])
		sentences[i] = strings.Join(words, " ") + "."
	}
	return strings.Join(sentences, " ")
}

// generateParagraphs produces `count` paragraphs of 5 sentences each.
func generateParagraphs(rng *loremRng, count int) string {
	paragraphs := make([]string, count)
	for i := 0; i < count; i++ {
		paragraphs[i] = generateSentences(rng, 5)
	}
	return strings.Join(paragraphs, "\n\n")
}

// capitalize capitalizes the first letter of a word.
func capitalize(w string) string {
	if w == "" {
		return w
	}
	return strings.ToUpper(w[:1]) + w[1:]
}

// (fmt import guard: keep the formatter linked for future variants)
var _ = fmt.Sprintf
