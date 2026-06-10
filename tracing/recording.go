package tracing

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// RecordingTracer records spans in memory for debugging and development
type RecordingTracer struct {
	mu    sync.RWMutex
	spans map[TraceID][]*Span
}

// NewRecordingTracer creates a new recording tracer
func NewRecordingTracer() *RecordingTracer {
	return &RecordingTracer{
		spans: make(map[TraceID][]*Span),
	}
}

// StartSpan creates a new span and stores it
func (t *RecordingTracer) StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	span := &Span{
		Name:   name,
		Start:  time.Now(),
		Status: "ok",
		Attrs:  make(map[string]string),
	}

	// Extract or create trace ID
	if traceID := TraceIDFromContext(ctx); (traceID != TraceID{}) {
		span.TraceID = traceID
	} else {
		span.TraceID = NewTraceID()
		ctx = WithTraceID(ctx, span.TraceID)
	}

	// Extract or create span ID and parent ID
	if parentSpan := SpanFromContext(ctx); parentSpan != nil {
		span.ParentID = parentSpan.SpanID
	}
	span.SpanID = NewSpanID()

	// Store the span
	t.mu.Lock()
	t.spans[span.TraceID] = append(t.spans[span.TraceID], span)
	t.mu.Unlock()

	return WithSpan(ctx, span), span
}

// EndSpan marks a span as completed
func (t *RecordingTracer) EndSpan(span *Span) {
	if span == nil {
		return
	}
	span.mu.Lock()
	span.End = time.Now()
	if span.Status == "" {
		span.Status = "ok"
	}
	span.mu.Unlock()
}

// GetTrace returns all spans for a given trace ID
func (t *RecordingTracer) GetTrace(traceID TraceID) []*Span {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if spans, ok := t.spans[traceID]; ok {
		result := make([]*Span, len(spans))
		copy(result, spans)
		return result
	}
	return nil
}

// AllTraces returns all recorded traces
func (t *RecordingTracer) AllTraces() map[TraceID][]*Span {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[TraceID][]*Span, len(t.spans))
	for traceID, spans := range t.spans {
		result[traceID] = make([]*Span, len(spans))
		copy(result[traceID], spans)
	}
	return result
}

// Export returns a JSON representation of a single trace
func (t *RecordingTracer) Export(traceID TraceID) string {
	spans := t.GetTrace(traceID)
	if spans == nil {
		return "null"
	}

	type spanJSON struct {
		TraceID  string            `json:"trace_id"`
		SpanID   string            `json:"span_id"`
		ParentID string            `json:"parent_id,omitempty"`
		Name     string            `json:"name"`
		Start    time.Time         `json:"start"`
		End      time.Time         `json:"end,omitempty"`
		Status   string            `json:"status"`
		Attrs    map[string]string `json:"attrs,omitempty"`
		Events   []SpanEvent       `json:"events,omitempty"`
		Duration string            `json:"duration,omitempty"`
	}

	exportSpans := make([]spanJSON, len(spans))
	for i, span := range spans {
		span.mu.RLock()
		exportSpans[i] = spanJSON{
			TraceID:  span.TraceID.String(),
			SpanID:   span.SpanID.String(),
			ParentID: span.ParentID.String(),
			Name:     span.Name,
			Start:    span.Start,
			End:      span.End,
			Status:   span.Status,
			Attrs:    span.Attrs,
			Events:   span.Events,
		}
		if !span.End.IsZero() {
			exportSpans[i].Duration = span.End.Sub(span.Start).String()
		}
		span.mu.RUnlock()
	}

	data, _ := json.MarshalIndent(exportSpans, "", "  ")
	return string(data)
}

// Clear removes all recorded spans
func (t *RecordingTracer) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.spans = make(map[TraceID][]*Span)
}