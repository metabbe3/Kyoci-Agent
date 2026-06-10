package tracing

import (
	"time"
)

// TraceStats provides statistics about recorded traces
type TraceStats struct {
	ActiveTraces int
	TotalSpans   int
	ErrorSpans   int
	AvgDuration  time.Duration
}

// Stats returns statistics about recorded traces
func (t *RecordingTracer) Stats() TraceStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := TraceStats{}

	for _, spans := range t.spans {
		stats.TotalSpans += len(spans)

		for _, span := range spans {
			span.mu.RLock()

			if span.Status == "error" {
				stats.ErrorSpans++
			}

			if !span.End.IsZero() {
				duration := span.End.Sub(span.Start)
				stats.AvgDuration += duration
			}

			span.mu.RUnlock()
		}
	}

	// Count active traces (traces with at least one span)
	stats.ActiveTraces = len(t.spans)

	// Calculate average duration
	if stats.TotalSpans > 0 {
		stats.AvgDuration = time.Duration(int64(stats.AvgDuration) / int64(stats.TotalSpans))
	}

	return stats
}