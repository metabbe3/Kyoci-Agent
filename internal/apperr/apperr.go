// Package apperr provides a centralized, taxonomy-aware error type for the
// Kyoci-Agent backend.
//
// An *apperr.Error carries a stable machine Code, a Kind (taxonomic category),
// a human Message, an optional wrapped Cause, and structured Fields. It
// integrates with the standard library's errors.Is / errors.As / errors.Unwrap
// so callers can branch on a specific sentinel (matched by Code) or on a
// category (via KindOf / IsKind).
//
// apperr is ADDITIVE to the error types already exported by the public kyoci
// package (pkg/types.go: APIError, ConfigError, ValidationError, and the
// ErrTaskFailed / ErrToolExecution / ErrToolNotFound / ErrMaxIterations /
// ErrMemoryCompact sentinels). Those remain in place; apperr adds the Kind
// taxonomy plus HTTP/gRPC status mapping for the observability and API layers.
package apperr

import (
	"errors"
	"fmt"
)

// Kind is the taxonomic category of an error. It drives HTTP/gRPC status
// mapping (see CodeToHTTP / CodeToGRPC) and coarse-grained branching.
type Kind int

const (
	KindUnknown Kind = iota
	KindNotFound
	KindUnavailable
	KindInvalid
	KindUnauthorized
	KindConflict
	KindInternal
	KindCircuitOpen
	KindProviderExhausted
	KindDeadline
	KindCanceled
	KindNotImplemented
)

// Error is the centralized error type. The zero value is not useful; construct
// instances with New, Newf, Wrap, Wrapf, or use a sentinel.
type Error struct {
	// Code is a stable, dotted machine code (e.g. "llm.circuit_open"). Two
	// *Error values are equal under errors.Is when their Codes match, so a
	// copy of a sentinel produced by With still matches the sentinel.
	Code string
	// Kind is the taxonomic category.
	Kind Kind
	// Message is the human-readable description; always non-empty.
	Message string
	// Cause is the wrapped underlying error, or nil. Returned by Unwrap.
	Cause error
	// Fields carries optional structured context. Add entries with With.
	Fields map[string]any
}

// Error implements the error interface. When Cause is non-nil it is appended
// as ": <cause>".
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

// Unwrap returns the wrapped cause, supporting errors.Is / errors.As traversal.
func (e *Error) Unwrap() error { return e.Cause }

// Is reports whether target is an *Error with the same Code. This lets a copy
// of a sentinel (e.g. produced by With) still match the sentinel via errors.Is.
// Non-*Error targets fall through to the standard library's Unwrap handling.
func (e *Error) Is(target error) bool {
	var t *Error
	if errors.As(target, &t) {
		return e.Code != "" && e.Code == t.Code
	}
	return false
}

// With returns a shallow copy of e with the given structured field set. The
// original (including sentinels) is left untouched and safe to reuse.
func (e *Error) With(key string, value any) *Error {
	cp := *e
	fields := make(map[string]any, len(e.Fields)+1)
	for k, v := range e.Fields {
		fields[k] = v
	}
	fields[key] = value
	cp.Fields = fields
	return &cp
}

// New returns a leaf error with the given code, kind, and message.
func New(code string, kind Kind, message string) *Error {
	return &Error{Code: code, Kind: kind, Message: message}
}

// Newf is like New with a fmt.Sprintf-formatted message.
func Newf(code string, kind Kind, format string, args ...any) *Error {
	return &Error{Code: code, Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// Wrap returns an error wrapping cause with the given code, kind, and message.
func Wrap(code string, kind Kind, cause error, message string) *Error {
	return &Error{Code: code, Kind: kind, Message: message, Cause: cause}
}

// Wrapf is like Wrap with a fmt.Sprintf-formatted message.
func Wrapf(code string, kind Kind, cause error, format string, args ...any) *Error {
	return &Error{Code: code, Kind: kind, Message: fmt.Sprintf(format, args...), Cause: cause}
}

// AsError extracts the first *Error in err's chain, or nil.
func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}

// KindOf returns the Kind of the first *Error in err's chain, or KindUnknown.
func KindOf(err error) Kind {
	if e := AsError(err); e != nil {
		return e.Kind
	}
	return KindUnknown
}

// IsKind reports whether err's chain contains an *Error whose Kind matches any
// of the given kinds. Use this for coarse category checks (e.g. "is invalid?").
func IsKind(err error, kinds ...Kind) bool {
	k := KindOf(err)
	for _, want := range kinds {
		if k == want {
			return true
		}
	}
	return false
}

// HasCode reports whether err's chain contains an *Error with the given Code.
func HasCode(err error, code string) bool {
	if e := AsError(err); e != nil {
		return e.Code == code
	}
	return false
}

// IsNotFound is a convenience for IsKind(err, KindNotFound).
func IsNotFound(err error) bool { return IsKind(err, KindNotFound) }
