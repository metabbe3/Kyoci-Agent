// Package recommend maps detected host specs to suggested LLMs — both local
// Ollama models that fit in RAM, and cloud guidance when local isn't viable.
package recommend

import (
	"github.com/metabbe3/Kyoci-Agent/internal/hardware"
)

// Pick is a single recommended model with the reasoning shown in the UI.
type Pick struct {
	Model       string `json:"model"`
	Provider    string `json:"provider"` // always "ollama" for local picks in v1
	ContextLen  int    `json:"context_len"`
	Reason      string `json:"reason"`
	Verdict     string `json:"verdict"` // "fits", "tight", "slow", "too_big"
	Recommended bool   `json:"recommended"` // true for the single best pick
}

// CloudAdvice is shown alongside local picks when local RAM is constrained.
type CloudAdvice struct {
	Needed             bool     `json:"needed"`
	Summary            string   `json:"summary"`
	RecommendedProviders []string `json:"recommended_providers,omitempty"`
}

// Result is what the dashboard's GET /api/dashboard/recommendations returns.
type Result struct {
	Summary string `json:"summary"`
	Local   []Pick `json:"local"`
	Cloud   CloudAdvice `json:"cloud"`
}

// Recommend returns local Ollama picks and cloud advice for the given specs.
// Specs may be nil or partially-populated (e.g. no GPU); the function still
// returns its best-effort recommendation rather than erroring.
func Recommend(specs *hardware.Specs) *Result {
	if specs == nil {
		specs = &hardware.Specs{}
	}
	ram := specs.EffectiveMemoryGB()
	appleSilicon := specs.IsAppleSilicon
	result := &Result{}

	// Walk tiers; classify each as fits/tight/slow/too_big relative to RAM.
	for _, t := range tiers {
		verdict, reason := classify(t, ram)
		result.Local = append(result.Local, Pick{
			Model:       t.Model,
			Provider:    "ollama",
			ContextLen:  t.ContextLen,
			Reason:      reason,
			Verdict:     verdict,
			Recommended: false, // set below
		})
	}

	// Recommended = largest tier that fits comfortably.
	bestIdx := -1
	for i := range tiers {
		if result.Local[i].Verdict == "fits" || result.Local[i].Verdict == "tight" {
			bestIdx = i
		}
	}
	if bestIdx >= 0 {
		result.Local[bestIdx].Recommended = true
	}

	result.Cloud = cloudAdvice(ram, appleSilicon)
	result.Summary = summary(specs, bestIdx)

	return result
}

// classify returns (verdict, reason) for a tier given the host's effective RAM.
func classify(t Tier, ramGB int) (verdict, reason string) {
	// Rule of thumb: Q4_K_M model needs ~ (params_in_B × 0.6) GB plus 20-30%
	// headroom for KV cache. Our tier table bakes in the headroom by treating
	// MinRAM as the *comfortable* floor.
	switch {
	case ramGB >= t.MinRAM*2:
		return "fits", "Fits comfortably with room for OS + KV cache."
	case ramGB >= t.MinRAM:
		return "tight", "Fits but leaves little headroom; expect some swap pressure under load."
	case ramGB >= t.MinRAM*3/4:
		return "slow", "Below the comfortable floor — will run but slowly, with frequent swap."
	default:
		return "too_big", "Will not fit in available RAM — likely to OOM or thrash."
	}
}

func cloudAdvice(ramGB int, appleSilicon bool) CloudAdvice {
	switch {
	case ramGB < 8:
		return CloudAdvice{
			Needed: true,
			Summary: "Your machine is too small for serious local inference. Use a cloud API for anything beyond toy prompts.",
			RecommendedProviders: []string{"groq", "anthropic"},
		}
	case ramGB <= 16:
		return CloudAdvice{
			Needed: true,
			Summary: "Local works for small tasks. For coding or long-context work, prefer cloud.",
			RecommendedProviders: []string{"anthropic", "openai"},
		}
	case ramGB < 32:
		return CloudAdvice{
			Needed: false,
			Summary: "Local is viable for everyday use. Cloud still wins for frontier quality on hard reasoning.",
			RecommendedProviders: []string{"anthropic"},
		}
	default:
		return CloudAdvice{
			Needed: false,
			Summary: "Local is sufficient for most workloads. Use cloud only when you need frontier quality (Claude Opus, GPT-4o).",
			RecommendedProviders: []string{"anthropic"},
		}
	}
}

func summary(specs *hardware.Specs, bestIdx int) string {
	ram := specs.EffectiveMemoryGB()
	chip := specs.ChipModel
	if chip == "" {
		chip = specs.OS + "/" + specs.Arch
	}
	if bestIdx < 0 {
		return "Detected " + chip + " with " + itoa(ram) + "GB. No local Ollama model fits comfortably — consider cloud."
	}
	return "Detected " + chip + " with " + itoa(ram) + "GB. Best local pick: " + tiers[bestIdx].Model + "."
}

// itoa avoids importing strconv for a single call.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
