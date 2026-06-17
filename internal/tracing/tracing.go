package tracing

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Tracer manages distributed tracing spans.
type Tracer struct {
	serviceName string
	spans       sync.Map // traceID -> []*Span
}

// Span represents a single trace span.
type Span struct {
	TraceID    string
	SpanID     string
	ParentID   string
	Name       string
	StartTime  time.Time
	EndTime    time.Time
	Attributes map[string]string
	mu         sync.Mutex
}

// contextKey is the type for span context keys.
type contextKey struct{}

// spanContextKey is the context key for storing spans.
var spanContextKey = contextKey{}

// New creates a new Tracer for the given service name.
func New(serviceName string) *Tracer {
	return &Tracer{
		serviceName: serviceName,
	}
}

// StartSpan creates and starts a new span.
// If a parent span exists in the context, it becomes the parent of this span.
// Returns a new context with the span and the span itself.
func (t *Tracer) StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	span := &Span{
		TraceID:    generateTraceID(),
		SpanID:     generateSpanID(),
		Name:       name,
		StartTime:  time.Now(),
		Attributes: make(map[string]string),
	}

	// Check for parent span
	if parentSpan := FromContext(ctx); parentSpan != nil {
		span.ParentID = parentSpan.SpanID
		span.TraceID = parentSpan.TraceID // Child spans share trace ID
	}

	// Store span in trace storage
	value, _ := t.spans.LoadOrStore(span.TraceID, []*Span{})
	spans := value.([]*Span)
	t.spans.Store(span.TraceID, append(spans, span))

	// Add span to context
	newCtx := context.WithValue(ctx, spanContextKey, span)

	return newCtx, span
}

// End marks the span as complete, recording its duration.
func (s *Span) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.EndTime = time.Now()
}

// SetAttribute adds or updates an attribute on the span.
func (s *Span) SetAttribute(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Attributes == nil {
		s.Attributes = make(map[string]string)
	}
	s.Attributes[key] = value
}

// Duration returns the span's duration. Returns 0 if span hasn't ended.
func (s *Span) Duration() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.EndTime.IsZero() {
		return 0
	}
	return s.EndTime.Sub(s.StartTime)
}

// FromContext extracts the span from a context.
// Returns nil if no span exists in the context.
func FromContext(ctx context.Context) *Span {
	if ctx == nil {
		return nil
	}
	span, ok := ctx.Value(spanContextKey).(*Span)
	if !ok {
		return nil
	}
	return span
}

// GetSpans returns all spans for a given trace ID.
func (t *Tracer) GetSpans(traceID string) []*Span {
	value, ok := t.spans.Load(traceID)
	if !ok {
		return nil
	}
	return value.([]*Span)
}

// GetAllTraces returns all trace IDs.
func (t *Tracer) GetAllTraces() []string {
	var traces []string
	t.spans.Range(func(key, value interface{}) bool {
		traces = append(traces, key.(string))
		return true
	})
	return traces
}

// Clear removes all stored spans. Useful for testing or memory management.
func (t *Tracer) Clear() {
	t.spans = sync.Map{}
}

// generateTraceID generates a unique trace ID.
func generateTraceID() string {
	return fmt.Sprintf("trace-%d", time.Now().UnixNano())
}

// generateSpanID generates a unique span ID.
func generateSpanID() string {
	return fmt.Sprintf("span-%d", time.Now().UnixNano())
}