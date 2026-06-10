package tracing

import (
	"encoding/hex"
	"net/http"

	"google.golang.org/grpc/metadata"
)

const (
	traceIDHeader = "x-trace-id"
	spanIDHeader  = "x-span-id"
)

// InjectHTTP injects trace and span IDs into HTTP headers
func InjectHTTP(span *Span, header http.Header) {
	if span == nil {
		return
	}
	header.Set(traceIDHeader, span.TraceID.String())
	header.Set(spanIDHeader, span.SpanID.String())
}

// ExtractHTTP extracts trace and span IDs from HTTP headers
func ExtractHTTP(header http.Header) (TraceID, SpanID) {
	traceID, _ := hex.DecodeString(header.Get(traceIDHeader))
	spanID, _ := hex.DecodeString(header.Get(spanIDHeader))

	var tid TraceID
	var sid SpanID

	copy(tid[:], traceID)
	copy(sid[:], spanID)

	return tid, sid
}

// InjectGRPC injects trace and span IDs into gRPC metadata
func InjectGRPC(span *Span, md *metadata.MD) {
	if span == nil || md == nil {
		return
	}
	(*md)[traceIDHeader] = []string{span.TraceID.String()}
	(*md)[spanIDHeader] = []string{span.SpanID.String()}
}

// ExtractGRPC extracts trace and span IDs from gRPC metadata
func ExtractGRPC(md metadata.MD) (TraceID, SpanID) {
	traceIDs := md.Get(traceIDHeader)
	spanIDs := md.Get(spanIDHeader)

	var tid TraceID
	var sid SpanID

	if len(traceIDs) > 0 {
		traceID, _ := hex.DecodeString(traceIDs[0])
		copy(tid[:], traceID)
	}

	if len(spanIDs) > 0 {
		spanID, _ := hex.DecodeString(spanIDs[0])
		copy(sid[:], spanID)
	}

	return tid, sid
}

// ExtractFromPayload extracts trace and span IDs from a JSON-like payload map
func ExtractFromPayload(payload map[string]interface{}) (TraceID, SpanID) {
	var tid TraceID
	var sid SpanID

	if traceIDVal, ok := payload[traceIDHeader].(string); ok {
		traceID, _ := hex.DecodeString(traceIDVal)
		copy(tid[:], traceID)
	}

	if spanIDVal, ok := payload[spanIDHeader].(string); ok {
		spanID, _ := hex.DecodeString(spanIDVal)
		copy(sid[:], spanID)
	}

	return tid, sid
}

// InjectToPayload injects trace and span IDs into a JSON-like payload map
func InjectToPayload(span *Span, payload map[string]interface{}) {
	if span == nil || payload == nil {
		return
	}
	payload[traceIDHeader] = span.TraceID.String()
	payload[spanIDHeader] = span.SpanID.String()
}