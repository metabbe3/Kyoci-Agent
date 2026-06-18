package apperr

// Sentinel errors for matchable conditions. Match with errors.Is; the match is
// by Code, so a copy produced by (*Error).With still compares equal to the
// sentinel. Messages are chosen to stay substring-compatible with existing
// tests and benchmark grade scripts (e.g. "no available providers").
var (
	// Generic category sentinels.
	ErrNotFound       = New("not_found", KindNotFound, "not found")
	ErrUnavailable    = New("unavailable", KindUnavailable, "unavailable")
	ErrInvalid        = New("invalid", KindInvalid, "invalid")
	ErrUnauthorized   = New("unauthorized", KindUnauthorized, "unauthorized")
	ErrConflict       = New("conflict", KindConflict, "conflict")
	ErrInternal       = New("internal", KindInternal, "internal error")
	ErrDeadline       = New("deadline", KindDeadline, "deadline exceeded")
	ErrCanceled       = New("canceled", KindCanceled, "canceled")
	ErrNotImplemented = New("not_implemented", KindNotImplemented, "not implemented")

	// Domain-specific sentinels.
	ErrCircuitOpen          = New("llm.circuit_open", KindCircuitOpen, "circuit breaker is open")
	ErrNoAvailableProviders = New("llm.no_available_providers", KindProviderExhausted, "no available providers")
	ErrNoProviders          = New("llm.no_providers", KindProviderExhausted, "no providers available")
	ErrNotStarted           = New("orchestrator.not_started", KindInternal, "orchestrator not started — call Start() first")
)
