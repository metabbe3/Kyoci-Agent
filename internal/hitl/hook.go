// Package hitl implements Human-In-The-Loop assistance for the orchestrator.
//
// When the orchestrator exhausts its retry budget on a failing task, it emits
// a HelpRequest via the Hub. The HelpRequest is streamed to any operator
// subscribed over gRPC (cmd/hitlctl). The operator's hint is delivered back
// to the waiting agent goroutine and injected into the next pipeline pass.
//
// Components:
//   - HITLHook     — the interface the orchestrator depends on
//   - HelpRequest  — the in-process request struct (translated to pb.HelpRequest)
//   - Hub          — in-process broker between agents and subscribers
//   - Server       — gRPC adapter over Hub (pb.HITLServiceServer impl)
package hitl

import (
	"context"
	"errors"
)

// HITLHook lets the orchestrator pause and request human assistance when it
// has exhausted its retry budget. The hook blocks until a hint arrives or the
// context/timeout expires.
//
// Implementations MUST be safe for concurrent use: orchestrator goroutines
// may emit HelpRequests from multiple tasks in parallel.
type HITLHook interface {
	// RequestHelp emits a HelpRequest to any subscribed operators and blocks
	// until a hint is returned. Returns:
	//   - (hint, nil)     — operator supplied a hint
	//   - ("", ErrNoHint) — no hint arrived before the deadline
	//   - ("", ErrNoSubscriber) — no operator is connected; orchestrator degrades
	//   - (other non-nil err) — transport-level failure
	RequestHelp(ctx context.Context, req HelpRequest) (hint string, err error)
}

// HelpRequest is the in-process request emitted by the orchestrator. It is
// translated to pb.HelpRequest at the gRPC boundary by the Hub.
//
// Fields mirror operator-facing concerns: what task, what role, how many
// attempts so far, what's the question, what was the last error, and what
// has been tried.
type HelpRequest struct {
	// TaskID is a unique ID for this Execute call. If empty, the Hub mints one.
	TaskID string
	// Role is the agent role handling the task (developer, qa, etc.).
	Role string
	// Attempt is the retry number that triggered this request (1-indexed).
	Attempt int
	// Question is a human-readable ask. e.g. "I cannot fix calculator.go —
	// test expects 5 but got 6. Can you point me at the bug?".
	Question string
	// LastError is the verification output from the most recent failed attempt.
	LastError string
	// AttemptedFixes is a short summary of fixes already tried (so the operator
	// doesn't suggest the same thing).
	AttemptedFixes []string
}

// ErrNoHint is returned when no operator supplies a hint before the deadline.
var ErrNoHint = errors.New("hitl: no hint received before timeout")

// ErrNoSubscriber is returned when there is no operator client connected to
// receive the HelpRequest. Callers should treat this as "HITL unavailable"
// and fall through to a hard failure rather than blocking the full timeout.
var ErrNoSubscriber = errors.New("hitl: no operator subscriber connected")
