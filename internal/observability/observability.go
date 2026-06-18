// Package observability wires OpenTelemetry (traces + metrics) and Prometheus
// for the Kyoci-Agent backend, plus graceful-lifecycle and debug-endpoint
// helpers.
//
// Telemetry is OFF by default: when Config.TracesEnabled and MetricsEnabled are
// both false, Setup installs no exporters and returns a no-op shutdown, so the
// app and the benchmark suite incur zero exporter overhead. Call Tracer / Meter
// / Instruments from anywhere; until Setup runs (or with signals off) they
// resolve to OpenTelemetry no-op implementations that are safe to call.
//
// The package keeps OpenTelemetry-conventional process-global providers
// (otel.SetTracerProvider / SetMeterProvider). This is the established pattern
// for telemetry bootstrap and the single legitimate exception to the codebase's
// "no mutable package globals" rule.
package observability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Config configures the telemetry Setup.
type Config struct {
	// OTLPEndpoint is the OTLP collector endpoint (e.g. "localhost:4317").
	// Empty disables exporter-backed export even when a signal is enabled.
	OTLPEndpoint string
	// ServiceName / ServiceVersion identify the emitting service.
	ServiceName    string
	ServiceVersion string
	// ResourceAttrs are extra resource attributes (deployment.environment, etc.).
	ResourceAttrs map[string]any
	// SampleRatio is the trace sampling ratio in [0,1] (ParentBased(TraceIDRatioBased)).
	SampleRatio float64
	// TracesEnabled / MetricsEnabled gate exporter installation.
	TracesEnabled  bool
	MetricsEnabled bool
}

// providers holds the SDK providers created by Setup. Package-level because
// OpenTelemetry is itself process-global; reads after startup are rare.
var (
	tracerProvider atomic.Pointer[sdktrace.TracerProvider]
	meterProvider  atomic.Pointer[sdkmetric.MeterProvider]
	instruments    atomic.Pointer[InstrumentSet]

	setupOnce  sync.Once
	setupState struct {
		shutdown func(context.Context) error
		err      error
	}
)

func init() {
	// Default to no-op instruments backed by the global (no-op) meter so callers
	// can record unconditionally before/without Setup.
	instruments.Store(newInstruments(otel.Meter("kyoci")))
}

// Setup initializes OpenTelemetry providers and exporters. It is idempotent:
// the first call wins and subsequent calls return the cached result. With both
// signals disabled it performs no work and returns a no-op shutdown. The
// returned shutdown flushes providers and must be called on process exit.
func Setup(ctx context.Context, cfg Config) (shutdown func(context.Context) error, err error) {
	setupOnce.Do(func() {
		setupState.shutdown, setupState.err = setup(ctx, cfg)
	})
	return setupState.shutdown, setupState.err
}

func setup(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }

	// W3C TraceContext + Baggage propagation, always.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	res, rerr := buildResource(ctx, cfg)
	if rerr != nil {
		return noop, fmt.Errorf("observability: build resource: %w", rerr)
	}

	var (
		tExp sdktrace.SpanExporter
		mExp sdkmetric.Exporter
		err  error
	)

	if cfg.TracesEnabled && cfg.OTLPEndpoint != "" {
		if tExp, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
			otlptracegrpc.WithInsecure(),
		); err != nil {
			return noop, fmt.Errorf("observability: trace exporter: %w", err)
		}
	}
	if cfg.MetricsEnabled && cfg.OTLPEndpoint != "" {
		if mExp, err = otlpmetricgrpc.New(ctx,
			otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint),
			otlpmetricgrpc.WithInsecure(),
		); err != nil {
			return noop, fmt.Errorf("observability: metric exporter: %w", err)
		}
	}

	// Tracer provider: always install so spans propagate; only batch-export when
	// an exporter exists. ParentBased(TraceIDRatioBased) honors parent decisions.
	tpOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(
			sdktrace.TraceIDRatioBased(ratioOr(cfg.SampleRatio, 1.0)),
		)),
	}
	if tExp != nil {
		tpOpts = append(tpOpts, sdktrace.WithBatcher(tExp))
	}
	tp := sdktrace.NewTracerProvider(tpOpts...)
	tracerProvider.Store(tp)
	otel.SetTracerProvider(tp)

	// Meter provider: always install; add the Prometheus reader (serves /metrics)
	// and an OTLP periodic reader when metrics are enabled. The Prometheus reader
	// registers with prometheus.DefaultRegisterer so promhttp.Handler() serves it.
	mpOpts := []sdkmetric.Option{sdkmetric.WithResource(res)}
	if cfg.MetricsEnabled {
		if promReader, perr := newPrometheusReader(); perr == nil {
			mpOpts = append(mpOpts, sdkmetric.WithReader(promReader))
		}
		if mExp != nil {
			mpOpts = append(mpOpts, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(mExp,
				sdkmetric.WithInterval(15*time.Second),
			)))
		}
	}
	mp := sdkmetric.NewMeterProvider(mpOpts...)
	meterProvider.Store(mp)
	otel.SetMeterProvider(mp)

	// Recreate instruments on the real meter so recorders emit real data.
	instruments.Store(newInstruments(mp.Meter("kyoci")))

	return func(ctx context.Context) error {
		var errs []error
		if e := tp.Shutdown(ctx); e != nil {
			errs = append(errs, fmt.Errorf("tracer shutdown: %w", e))
		}
		if e := mp.Shutdown(ctx); e != nil {
			errs = append(errs, fmt.Errorf("meter shutdown: %w", e))
		}
		return errors.Join(errs...)
	}, nil
}

// newPrometheusReader creates the OTel Prometheus exporter, which implements
// sdkmetric.Reader and self-registers with the default Prometheus registerer.
func newPrometheusReader() (sdkmetric.Reader, error) {
	return otelprom.New(otelprom.WithRegisterer(prometheus.DefaultRegisterer))
}

// Tracer returns a named tracer from the active provider (no-op before Setup).
func Tracer(name string) trace.Tracer {
	if tp := tracerProvider.Load(); tp != nil {
		return tp.Tracer(name)
	}
	return otel.Tracer(name)
}

// Meter returns a named meter from the active provider (no-op before Setup).
func Meter(name string) otelmetric.Meter {
	if mp := meterProvider.Load(); mp != nil {
		return mp.Meter(name)
	}
	return otel.Meter(name)
}

// Instruments returns the process-wide instrument set (never nil; no-op until
// Setup configures metrics).
func Instruments() *InstrumentSet { return instruments.Load() }

// Logger returns a slog logger writing JSON to stdout (text on an interactive
// TTY) at the given level.
func Logger(name string, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler = slog.NewJSONHandler(os.Stdout, opts)
	if isTerminal() {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	if name != "" {
		return slog.New(h).With("service", name)
	}
	return slog.New(h)
}

func ratioOr(r, def float64) float64 {
	if r <= 0 || r > 1 {
		return def
	}
	return r
}

// buildResource assembles the OTel Resource from service identity + extra attrs.
func buildResource(ctx context.Context, cfg Config) (*sdkresource.Resource, error) {
	attrs := []attribute.KeyValue{
		attribute.String("service.name", valueOr(cfg.ServiceName, "kyoci-agent")),
		attribute.String("service.version", valueOr(cfg.ServiceVersion, "dev")),
	}
	for k, v := range cfg.ResourceAttrs {
		if kv, ok := toKeyValue(k, v); ok {
			attrs = append(attrs, kv)
		}
	}
	return sdkresource.New(ctx, sdkresource.WithAttributes(attrs...))
}

func valueOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func toKeyValue(k string, v any) (attribute.KeyValue, bool) {
	switch t := v.(type) {
	case string:
		return attribute.String(k, t), true
	case bool:
		return attribute.Bool(k, t), true
	case int:
		return attribute.Int64(k, int64(t)), true
	case int64:
		return attribute.Int64(k, t), true
	case float64:
		return attribute.Float64(k, t), true
	default:
		return attribute.KeyValue{}, false
	}
}
