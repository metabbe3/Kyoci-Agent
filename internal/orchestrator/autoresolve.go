package orchestrator

import (
	"context"
	"strings"
	"sync"

	"github.com/metabbe3/Kyoci-Agent/internal/agentdef"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Deterministic task→agent resolver (auto-routing)
//
// resolveAgent is the single routing entry point used by Execute / ExecuteStream
// when a task arrives without an explicit role (RoleCustom). It is a thin,
// behavior-preserving wrapper around the existing pure-Go classifier
// (ClassifyRole → agentdef.BestMatch) that adds two things WITHOUT changing the
// routing decision itself:
//
//  1. An exact task-hash cache (Tier 0) so repeat / identical tasks resolve in
//     O(1) with zero re-scoring.
//  2. A structured provenance log line per routing decision — the
//     instrumentation needed to measure routing quality and to decide later
//     whether future tiers (an LLM selector, or on-the-fly agent synthesis)
//     are warranted.
//
// The ctx parameter is reserved for those future tiers (an LLM selector call
// needs timeout / cancellation); the deterministic path does not use it.
//
// Why no LLM here: see classifier.go — "a small model making a bad route
// decision costs a full pipeline run." Routing stays deterministic. When a task
// is ambiguous and no specialist clears MinSpecialistScore, it falls to the
// generalist, which already re-delegates to a specialist via delegation.go's
// recursive re-classification (o.Execute(..., RoleCustom)).
// =====================================================================================

// routeCache is a bounded, concurrency-safe exact-hash cache mapping a
// normalized task hash → resolved RoleType. It is strictly best-effort:
// correctness never depends on a hit (a miss simply re-runs the deterministic
// classifier). Eviction is arbitrary (one key) when the cap is reached — the
// only cost of eviction is a re-score, which is exactly what a miss does.
type routeCache struct {
	mu         sync.RWMutex
	entries    map[uint64]kyoci.RoleType
	maxEntries int
}

func newRouteCache(maxEntries int) *routeCache {
	if maxEntries <= 0 {
		maxEntries = 1024
	}
	return &routeCache{
		entries:    make(map[uint64]kyoci.RoleType),
		maxEntries: maxEntries,
	}
}

func (c *routeCache) get(h uint64) (kyoci.RoleType, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	r, ok := c.entries[h]
	return r, ok
}

func (c *routeCache) put(h uint64, r kyoci.RoleType) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[h]; exists {
		c.entries[h] = r
		return
	}
	if len(c.entries) >= c.maxEntries {
		// Drop one arbitrary entry. Map iteration order is unspecified; for a
		// routing cache where eviction only costs a re-score, this is fine.
		for k := range c.entries {
			delete(c.entries, k)
			break
		}
	}
	c.entries[h] = r
}

// len returns the number of cached entries (test / observability helper).
func (c *routeCache) len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// hashTask returns a stable FNV-1a 64-bit hash of a normalized task string.
// Normalization lowercases and collapses all whitespace runs to single spaces,
// so trivial differences (surrounding spaces, capitalization, re-wrapped text)
// hash identically. Genuine paraphrases still differ — semantic / embedding
// based dedup is deliberately deferred (see the plan's "deferred" section).
func hashTask(task string) uint64 {
	s := strings.Join(strings.Fields(strings.ToLower(task)), " ")
	const (
		offsetBasis64 uint64 = 14695981039346656037
		prime64       uint64 = 1099511628211
	)
	h := offsetBasis64
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return h
}

// topScoringDef returns the BestMatch winner + its raw MatchScore for the task.
// It reuses agentdef.BestMatch (the exact routing decision) so the logged
// top_specialist is always identical to the chosen role — including on score
// ties, which BestMatch breaks by priority then load-order. Used ONLY for
// provenance / confidence logging — it never influences the pick. Returns
// ("", 0) when no agents are loaded; otherwise ("generalist", 0) when nothing
// clears the threshold (BestMatch's fallback).
func topScoringDef(task string) (name string, score int) {
	defs := currentAgentDefs()
	name = agentdef.BestMatch(task, defs) // the actual routing winner
	for _, d := range defs {
		if d.Name == name {
			return name, agentdef.MatchScore(d, task)
		}
	}
	return name, 0
}

// resolveAgent resolves which agent should handle a task. See the file header
// for the full rationale. Returns RoleGeneralist when no agents are loaded or
// no specialist clears the threshold — identical to the pre-existing
// ClassifyRole behavior, just cached and logged.
func (o *Orchestrator) resolveAgent(ctx context.Context, task string) kyoci.RoleType {
	_ = ctx // reserved for future LLM-based routing tiers

	// Normalize the task ONCE and use the SAME normalized form for the cache key
	// AND for scoring (ClassifyRole/topScoringDef). hashTask collapses whitespace
	// but the scorer does not — feeding the original task to ClassifyRole while
	// keying on the normalized hash could mis-route two whitespace-variants that
	// hash identically but score differently (e.g. "run go\ntest" vs "run go test").
	nt := normalizeTask(task)

	// Tier 0 — exact-hash cache (when enabled).
	if o.routing.CacheEnabled && o.routeCache != nil {
		h := hashTask(nt)
		if role, ok := o.routeCache.get(h); ok {
			o.logRouting(task, "cache_hit", role, 0, "", false)
			return role
		}
		// Tier 1 — deterministic pick, then cache it.
		role := ClassifyRole(nt)
		o.routeCache.put(h, role)
		if o.routing.LogProvenance {
			topName, topScore := topScoringDef(nt)
			o.logRouting(task, "scored", role, topScore, topName, topScore < agentdef.MinSpecialistScore)
		}
		return role
	}

	// Cache disabled — still resolve (and optionally log), just don't memoize.
	role := ClassifyRole(nt)
	if o.routing.LogProvenance {
		topName, topScore := topScoringDef(nt)
		o.logRouting(task, "scored_nocache", role, topScore, topName, topScore < agentdef.MinSpecialistScore)
	}
	return role
}

// normalizeTask lowercases and collapses all whitespace runs to single spaces,
// matching hashTask's normalization. resolveAgent uses this for BOTH the cache
// key and the scoring input so they can never diverge.
func normalizeTask(task string) string {
	return strings.Join(strings.Fields(strings.ToLower(task)), " ")
}

// logRouting emits one structured routing-decision line. No-op when provenance
// logging is disabled or no logger is wired. Fields:
//   - source: "cache_hit" | "scored" | "scored_nocache"
//   - chosen_role: the RoleType the task was routed to
//   - top_specialist / top_score: the best-scoring loaded specialist (for tuning)
//   - abstain: true when no specialist cleared MinSpecialistScore (→ generalist)
func (o *Orchestrator) logRouting(task, source string, role kyoci.RoleType, topScore int, topName string, abstain bool) {
	if o.logger == nil || !o.routing.LogProvenance {
		return
	}
	o.logger.Info("routing decision",
		"source", source,
		"chosen_role", role.String(),
		"top_specialist", topName,
		"top_score", topScore,
		"abstain", abstain,
		"task", truncateForLog(task, 120),
	)
}

// truncateForLog shortens a string to maxRunes runes, appending an ellipsis if
// truncated. Rune-safe so it never splits a multi-byte character.
func truncateForLog(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}
