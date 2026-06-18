package observability

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
)

// HTTPMiddleware wraps h with OpenTelemetry HTTP tracing, panic recovery (logged
// and returned as 500), and structured access logging. It uses the global
// TracerProvider, so it is a no-op for tracing until Setup installs exporters.
func HTTPMiddleware(h http.Handler, log *slog.Logger) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	traced := otelhttp.NewHandler(h, "http.server")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if rv := recover(); rv != nil {
				rec.status = http.StatusInternalServerError
				log.Error("http panic recovered",
					"method", r.Method, "path", r.URL.Path,
					"panic", rv, "stack", string(debug.Stack()))
				http.Error(rec, "internal server error", http.StatusInternalServerError)
			}
			log.Info("http request",
				"method", r.Method, "path", r.URL.Path,
				"status", rec.status, "duration_ms", time.Since(start).Milliseconds(),
				"trace_id", traceID(r.Context()))
		}()
		traced.ServeHTTP(rec, r)
	})
}

// statusRecorder captures the response status code for access logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush delegates to the underlying ResponseWriter when it implements
// http.Flusher. This is REQUIRED so that SSE/streaming handlers (which assert
// w.(http.Flusher)) keep working through this middleware wrapper — without it
// they fail with "streaming not supported".
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack delegates to the underlying ResponseWriter when it implements
// http.Hijacker (WebSocket / connection upgrades). Same transparency rationale
// as Flush.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := r.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("observability.statusRecorder: underlying ResponseWriter does not implement http.Hijacker")
}

func traceID(ctx context.Context) string {
	if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
		return sc.TraceID().String()
	}
	return ""
}
