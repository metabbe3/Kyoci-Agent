package orchestrator

import (
	"context"
	"sync"
	"testing"

	"github.com/metabbe3/Kyoci-Agent/internal/agentdef"
	"github.com/metabbe3/Kyoci-Agent/internal/config"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// setDefsForTest swaps the package-global agent set used by ClassifyRole /
// currentAgentDefs and returns a restore func. Avoids cross-test pollution of
// the boot-loaded defaultDefs. NOT safe under t.Parallel — these tests mutate
// the package-global defaultDefs serially (the restore func relies on that).
func setDefsForTest(defs []agentdef.AgentDef) func() {
	defaultDefsMu.Lock()
	saved := defaultDefs
	defaultDefs = defs
	defaultDefsMu.Unlock()
	return func() {
		defaultDefsMu.Lock()
		defaultDefs = saved
		defaultDefsMu.Unlock()
	}
}

func autoresolveTestDefs() []agentdef.AgentDef {
	return []agentdef.AgentDef{
		{Name: "generalist", Body: "generalist", SystemPrompt: "g"},
		{
			Name:   "developer",
			Body:   "dev",
			SystemPrompt: "d",
			Triggers: agentdef.TriggerSpec{
				Keywords: []string{"code", "function", "bug"},
				Anchors:  []string{".go", "golang"},
			},
		},
		{
			Name:   "qa",
			Body:   "qa",
			SystemPrompt: "q",
			Triggers: agentdef.TriggerSpec{
				Keywords: []string{"test"},
				Anchors:  []string{"pytest"},
			},
		},
	}
}

func newAutoResolveOrchestrator(cacheOn, logOn bool) *Orchestrator {
	return &Orchestrator{
		routing: config.RoutingConfig{
			CacheEnabled:    cacheOn,
			CacheMaxEntries: 16,
			LogProvenance:   logOn,
		},
		routeCache: newRouteCache(16),
	}
}

// resolveAgent must be deterministic for the same task and route a clearly
// specialist task to the specialist, caching exactly one entry.
func TestResolveAgent_DeterministicSpecialistAndCached(t *testing.T) {
	restore := setDefsForTest(autoresolveTestDefs())
	defer restore()

	o := newAutoResolveOrchestrator(true, false)
	task := "fix a bug in the .go file and write a function"

	r1 := o.resolveAgent(context.Background(), task)
	r2 := o.resolveAgent(context.Background(), task)

	if r1 != r2 {
		t.Fatalf("non-deterministic routing: %q then %q", r1, r2)
	}
	if r1.String() != "developer" {
		t.Fatalf("expected developer, got %s", r1)
	}
	if o.routeCache.len() != 1 {
		t.Fatalf("expected exactly 1 cached entry, got %d", o.routeCache.len())
	}
}

// When no specialist clears the threshold, resolveAgent must fall to the
// generalist — identical to the pre-existing ClassifyRole behavior.
func TestResolveAgent_AbstainFallsToGeneralist(t *testing.T) {
	restore := setDefsForTest(autoresolveTestDefs())
	defer restore()

	o := newAutoResolveOrchestrator(true, false)
	r := o.resolveAgent(context.Background(), "what is the capital of france")
	if r.String() != "generalist" {
		t.Fatalf("expected generalist for unmatched task, got %s", r)
	}
}

// A cache hit must return the memoized role even after the agent set changes —
// proving the second call did not re-score.
func TestResolveAgent_CacheHitSkipsRescore(t *testing.T) {
	restore := setDefsForTest(autoresolveTestDefs())
	defer restore()

	o := newAutoResolveOrchestrator(true, false)
	task := "fix the .go bug and write a function"

	r1 := o.resolveAgent(context.Background(), task)
	if r1.String() != "developer" {
		t.Fatalf("expected developer first, got %s", r1)
	}

	// Wipe the agent set. Without the cache this would now return generalist.
	setDefsForTest(nil)
	r2 := o.resolveAgent(context.Background(), task)
	if r2 != r1 {
		t.Fatalf("cache miss: expected cached %s, got %s", r1, r2)
	}
}

// With the cache disabled, routing still resolves correctly and nothing is stored.
func TestResolveAgent_CacheDisabled(t *testing.T) {
	restore := setDefsForTest(autoresolveTestDefs())
	defer restore()

	o := newAutoResolveOrchestrator(false, false)
	r := o.resolveAgent(context.Background(), "fix the .go bug")
	if r.String() != "developer" {
		t.Fatalf("expected developer, got %s", r)
	}
	if o.routeCache.len() != 0 {
		t.Fatalf("cache should be empty when disabled, got %d", o.routeCache.len())
	}
}

// Empty/nil agent set resolves to generalist and never panics.
func TestResolveAgent_NoAgentsLoaded(t *testing.T) {
	restore := setDefsForTest(nil)
	defer restore()

	o := newAutoResolveOrchestrator(true, true) // provenance on; must not panic with nil logger
	r := o.resolveAgent(context.Background(), "anything")
	if r != kyoci.RoleGeneralist {
		t.Fatalf("expected RoleGeneralist with no agents, got %s", r)
	}
}

func TestRouteCache_PutGetEvict(t *testing.T) {
	c := newRouteCache(2)
	c.put(1, kyoci.RoleType("a"))
	c.put(2, kyoci.RoleType("b"))

	if r, ok := c.get(1); !ok || r.String() != "a" {
		t.Fatalf("get(1) = (%s, %v), want (a, true)", r, ok)
	}
	// Exceed cap → evict one arbitrary entry, count stays at cap.
	c.put(3, kyoci.RoleType("c"))
	if c.len() != 2 {
		t.Fatalf("expected 2 entries after eviction, got %d", c.len())
	}
	// Retrievability: exactly two of {1,2,3} must still resolve to their value
	// (guards against a put() that drops the wrong count or fails to insert).
	want := map[uint64]string{1: "a", 2: "b", 3: "c"}
	got := 0
	for k, v := range want {
		if r, ok := c.get(k); ok && r.String() == v {
			got++
		}
	}
	if got != 2 {
		t.Fatalf("expected exactly 2 retrievable entries after eviction, got %d", got)
	}
}

// TestResolveAgent_Concurrent stresses the shared routeCache (and provenance
// logging) under concurrency — Execute/ExecuteStream are called concurrently in
// production. Asserts no panic and a consistent, non-empty decision per task.
// Run with -race to catch data races on the cache.
func TestResolveAgent_Concurrent(t *testing.T) {
	restore := setDefsForTest(autoresolveTestDefs())
	defer restore()

	o := newAutoResolveOrchestrator(true, true) // cache + provenance on (max surface)
	tasks := []string{
		"fix the .go bug and write a function",
		"what is the capital of france",
		"explain how neural networks learn",
		"write a pytest for the calculator",
		"deploy to the kubernetes cluster",
		"  FIX the .GO bug  ", // normalization variant of task 0
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			task := tasks[i%len(tasks)]
			if r := o.resolveAgent(context.Background(), task); r.String() == "" {
				t.Errorf("empty role for task %q", task)
			}
		}(i)
	}
	wg.Wait()
}

func TestRouteCache_DefaultSizeWhenNonPositive(t *testing.T) {
	c := newRouteCache(0)
	if c.maxEntries != 1024 {
		t.Fatalf("expected default 1024, got %d", c.maxEntries)
	}
}

func TestHashTask_Normalization(t *testing.T) {
	a := hashTask("Fix the .GO Bug")
	b := hashTask("  fix   the .go\tbug  ")
	if a != b {
		t.Fatalf("expected normalized hashes to match: %d vs %d", a, b)
	}
	if hashTask("completely different task") == a {
		t.Fatal("expected a different hash for a different task")
	}
	if hashTask("") != hashTask("   ") {
		t.Fatal("empty and whitespace-only tasks should hash identically")
	}
}

func TestTopScoringDef(t *testing.T) {
	restore := setDefsForTest(autoresolveTestDefs())
	defer restore()

	name, score := topScoringDef("fix the .go bug and write a function")
	if name != "developer" || score < agentdef.MinSpecialistScore {
		t.Fatalf("expected developer with score>=%d, got %s/%d", agentdef.MinSpecialistScore, name, score)
	}

	name2, score2 := topScoringDef("hello world nothing matches")
	if score2 != 0 {
		t.Fatalf("expected score 0 for no match, got %d (%s)", score2, name2)
	}
}

func TestTruncateForLog_RuneSafe(t *testing.T) {
	if got := truncateForLog("hello", 10); got != "hello" {
		t.Fatalf("expected no truncation, got %q", got)
	}
	if got := truncateForLog("hello world", 5); got != "hello…" {
		t.Fatalf("expected 'hello…', got %q", got)
	}
	// Must not split a multi-byte rune (each kanji is 3 UTF-8 bytes).
	if got := truncateForLog("日本語テスト", 2); got != "日本…" {
		t.Fatalf("expected rune-safe truncation '日本…', got %q", got)
	}
	if got := truncateForLog("x", 0); got != "" {
		t.Fatalf("expected empty for maxRunes<=0, got %q", got)
	}
}

// Whitespace-only differences must not cause a cache mis-route: the cache key
// (whitespace-collapsed) and the scoring input must use the SAME normalized form.
func TestResolveAgent_WhitespaceNormalizationConsistency(t *testing.T) {
	defs := []agentdef.AgentDef{
		{Name: "generalist"},
		{Name: "developer", Triggers: agentdef.TriggerSpec{Anchors: []string{" go test"}}},
	}
	restore := setDefsForTest(defs)
	defer restore()
	o := newAutoResolveOrchestrator(true, false)

	// " go test" is a whitespace-sensitive anchor. The space and newline variants
	// normalize to the same key AND the same scoring input → both developer.
	space := o.resolveAgent(context.Background(), "run go test here")
	newline := o.resolveAgent(context.Background(), "run go\ntest here")
	if space.String() != "developer" || newline.String() != "developer" {
		t.Errorf("whitespace variants must both resolve developer (normalized); got %q and %q", space, newline)
	}
}
