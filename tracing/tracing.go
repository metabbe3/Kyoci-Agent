package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// TraceID is a 16-byte identifier for a trace
type TraceID [16]byte

// SpanID is an 8-byte identifier for a span
type SpanID [8]byte

// Span represents a unit of work in a distributed trace
type Span struct {
	TraceID  TraceID
	SpanID   SpanID
	ParentID SpanID
	Name     string
	Start    time.Time
	End      time.Time
	Status   string // "ok", "error"
	Attrs    map[string]string
	Events   []SpanEvent
	mu       sync.RWMutex
}

// SpanEvent represents a timestamped event within a span
type SpanEvent struct {
	Name  string
	TS    time.Time
	Attrs map[string]string
}

// Tracer is the interface for creating and managing spans
type Tracer interface {
	StartSpan(ctx context.Context, name string) (context.Context, *Span)
	EndSpan(span *Span)
}

// NoopTracer is a zero-overhead tracer that does nothing
type NoopTracer struct{}

// StartSpan creates a no-op span
func (t *NoopTracer) StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	return ctx, nil
}

// EndSpan does nothing for no-op spans
func (t *NoopTracer) EndSpan(span *Span) {
	// No-op
}

// Global no-op tracer instance
var DefaultTracer Tracer = &NoopTracer{}

// contextKey is the type for context keys
type contextKey struct{}

var (
	spanContextKey    = contextKey{}
	traceIDContextKey = contextKey{}
)

// WithSpan stores a span in the context
func WithSpan(ctx context.Context, span *Span) context.Context {
	return context.WithValue(ctx, spanContextKey, span)
}

// SpanFromContext retrieves a span from the context
func SpanFromContext(ctx context.Context) *Span {
	if span, ok := ctx.Value(spanContextKey).(*Span); ok {
		return span
	}
	return nil
}

// WithTraceID stores a trace ID in the context
func WithTraceID(ctx context.Context, traceID TraceID) context.Context {
	return context.WithValue(ctx, traceIDContextKey, traceID)
}

// TraceIDFromContext retrieves a trace ID from the context
func TraceIDFromContext(ctx context.Context) TraceID {
	if traceID, ok := ctx.Value(traceIDContextKey).(TraceID); ok {
		return traceID
	}
	return TraceID{}
}

// NewTraceID generates a new random trace ID
func NewTraceID() TraceID {
	var id TraceID
	_, _ = rand.Read(id[:])
	return id
}

// NewSpanID generates a new random span ID
func NewSpanID() SpanID {
	var id SpanID
	_, _ = rand.Read(id[:])
	return id
}

// String returns the hex representation of the trace ID
func (t TraceID) String() string {
	return hex.EncodeToString(t[:])
}

// String returns the hex representation of the span ID
func (s SpanID) String() string {
	return hex.EncodeToString(s[:])
}

// SetAttribute sets a key-value attribute on the span
func (s *Span) SetAttribute(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Attrs == nil {
		s.Attrs = make(map[string]string)
	}
	s.Attrs[key] = value
}

// AddEvent adds an event to the span
func (s *Span) AddEvent(name string, attrs map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	event := SpanEvent{
		Name:  name,
		TS:    time.Now(),
		Attrs: attrs,
	}
	s.Events = append(s.Events, event)
}

// Duration returns the span's duration
func (s *Span) Duration() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.End.IsZero() {
		return time.Since(s.Start)
	}
	return s.End.Sub(s.Start)
}