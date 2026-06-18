package llm

import (
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// newTestClient is a white-box test helper returning the concrete *OpenAIClient.
// NewOpenAIClient returns the kyoci.Provider interface; tests that exercise
// unexported methods/state (circuitBreaker, convertMessages, convertTools,
// stripToMinimal, parseRetryAfter) need the concrete type.
func newTestClient(name string, cfg kyoci.ProviderConfig) (*OpenAIClient, error) {
	p, err := NewOpenAIClient(name, cfg)
	if err != nil {
		return nil, err
	}
	return AsOpenAIClient(p), nil
}
