package observability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/signal"
	"syscall"
	"time"
)

// Component is a named, shutdown-able process component.
type Component struct {
	Name     string
	Shutdown func(ctx context.Context) error
	Timeout  time.Duration
}

// Manager owns a root context canceled on SIGINT/SIGTERM and coordinates
// ordered, timeout-bounded shutdown of registered components. Components shut
// down in reverse registration order (stop front-door traffic before backend
// subsystems); OpenTelemetry providers flush last.
type Manager struct {
	log        *slog.Logger
	components []Component
	otelStop   func(context.Context) error
}

// NewManager builds a Manager. otelShutdown is the shutdown func returned by
// Setup (nil if telemetry was not configured).
func NewManager(log *slog.Logger, otelShutdown func(context.Context) error) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{log: log, otelStop: otelShutdown}
}

// Add registers a component to be shut down on exit.
func (m *Manager) Add(name string, timeout time.Duration, fn func(context.Context) error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	m.components = append(m.components, Component{Name: name, Shutdown: fn, Timeout: timeout})
}

// Run installs a SIGINT/SIGTERM handler that cancels the context, then blocks on
// run. When run returns (e.g. fatal startup error) or a signal arrives, all
// components are shut down in reverse order, each under its own timeout.
func (m *Manager) Run(ctx context.Context, run func(ctx context.Context) error) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runErr := make(chan error, 1)
	go func() { runErr <- run(ctx) }()

	select {
	case err := <-runErr:
		return errors.Join(err, m.shutdown(ctx))
	case <-ctx.Done():
		m.log.Info("shutdown signal received, draining")
		stop() // restore default behavior so a second signal force-exits
		return m.shutdown(context.Background())
	}
}

func (m *Manager) shutdown(ctx context.Context) error {
	var errs []error
	for i := len(m.components) - 1; i >= 0; i-- {
		c := m.components[i]
		start := time.Now()
		shutCtx, cancel := context.WithTimeout(ctx, c.Timeout)
		err := c.Shutdown(shutCtx)
		cancel()
		if err != nil {
			m.log.Error("component shutdown error", "component", c.Name, "error", err, "duration", time.Since(start))
			errs = append(errs, fmt.Errorf("%s: %w", c.Name, err))
			continue
		}
		m.log.Info("component stopped", "component", c.Name, "duration", time.Since(start))
	}
	if m.otelStop != nil {
		shutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := m.otelStop(shutCtx); err != nil {
			m.log.Error("otel shutdown error", "error", err)
			errs = append(errs, fmt.Errorf("otel: %w", err))
		}
		cancel()
	}
	return errors.Join(errs...)
}
