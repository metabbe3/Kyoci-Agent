package promptskill

import "strings"

// CompositeInjector chains multiple Injectors and concatenates their non-empty
// outputs with a blank-line separator. It implements Injector itself, so it
// can be passed wherever a single Injector is expected (e.g. to the agent's
// SetIntelligenceHooks).
//
// Typical wiring in the orchestrator:
//
//	composite := promptskill.CompositeInjector{
//	    Injectors: []promptskill.Injector{memoryInjector, skillInjector},
//	}
//	registry.SetIntelligenceHooks(composite, recorder)
type CompositeInjector struct {
	Injectors []Injector
}

// Inject runs every wrapped injector in order and joins their outputs.
// Empty outputs are skipped to avoid stray separators.
func (c CompositeInjector) Inject(task string) string {
	if c.Injectors == nil {
		return ""
	}
	var parts []string
	for _, inj := range c.Injectors {
		if inj == nil {
			continue
		}
		if t := inj.Inject(task); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n\n")
}
