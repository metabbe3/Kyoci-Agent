// Package tracing is a thin backward-compatibility shim that routes the
// orchestrator's legacy StartSpan / SetAttribute / End API onto OpenTelemetry.
// New code should use internal/observability directly.
//
// The previous in-process span store (sync.Map keyed by trace ID, with
// GetSpans/GetAllTraces/FromContext) is removed: it had no external consumers
// and produced no observable signal. Spans now flow to the configured OTel
// exporter (no-op until observability.Setup runs), preserving the orchestrator's
// behavior of creating a span per Execute / ExecuteStream / ExecuteDirect.
package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// Tracer wraps an OpenTelemetry tracer under the legacy API.
type Tracer struct {
	tracer oteltrace.Tracer
}

// Span wraps an OpenTelemetry span under the legacy API.
type Span struct {
	otel oteltrace.Span
}

// New returns a Tracer backed by the named OpenTelemetry tracer.
func New(serviceName string) *Tracer {
	return &Tracer{tracer: otel.Tracer(serviceName)}
}

// StartSpan starts a span and returns a context carrying it plus the span.
func (t *Tracer) StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	ctx, span := t.tracer.Start(ctx, name)
	return ctx, &Span{otel: span}
}

// SetAttribute adds a string attribute to the span (the legacy API is
// string-only, matching the orchestrator's existing call sites).
func (s *Span) SetAttribute(key, value string) {
	if s == nil || s.otel == nil {
		return
	}
	s.otel.SetAttributes(attribute.String(key, value))
}

// SetError marks the span as failed and records err.
func (s *Span) SetError(err error) {
	if s == nil || s.otel == nil || err == nil {
		return
	}
	s.otel.SetStatus(codes.Error, err.Error())
	s.otel.RecordError(err)
}

// End completes the span.
func (s *Span) End() {
	if s == nil || s.otel == nil {
		return
	}
	s.otel.End()
}

// Clear is a no-op: OpenTelemetry exports spans via the configured exporter and
// does not retain them in-process. Kept for API compatibility with callers that
// flush at shutdown.
func (t *Tracer) Clear() {}
