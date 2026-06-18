package gateway

import (
	"context"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"
)

// Gateway is a chat-platform adapter (Telegram today; Discord/Slack later). A
// gateway runs until its context is canceled; Stop drains in-flight per-chat
// work (e.g. pending approvals) so callers do not block on shutdown.
type Gateway interface {
	// Name is the gateway identifier (e.g. "telegram").
	Name() string
	// Start runs the gateway until ctx is canceled or an error occurs.
	Start(ctx context.Context) error
	// Stop drains in-flight work after Start's ctx is canceled.
	Stop(ctx context.Context) error
}

// Compile-time check that TelegramGateway satisfies Gateway.
var _ Gateway = (*TelegramGateway)(nil)

// MultiGateway runs a set of gateways concurrently and shuts them all down
// together. Run blocks until ctx is canceled or any gateway returns an error;
// on exit it Stops every gateway in parallel under a deadline.
type MultiGateway struct {
	gateways []Gateway
	logger   *slog.Logger
}

// NewMultiGateway builds a MultiGateway from the given gateways.
func NewMultiGateway(logger *slog.Logger, gateways ...Gateway) *MultiGateway {
	if logger == nil {
		logger = slog.Default()
	}
	return &MultiGateway{gateways: gateways, logger: logger}
}

// Add appends a gateway to the set.
func (m *MultiGateway) Add(gw Gateway) { m.gateways = append(m.gateways, gw) }

// Run starts every gateway and blocks until ctx is canceled or any gateway
// errors. It then Stops all gateways in parallel under a fresh 5s deadline
// (the run ctx is canceled by then).
func (m *MultiGateway) Run(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)
	for _, gw := range m.gateways {
		gw := gw
		g.Go(func() error {
			m.logger.Info("gateway starting", "gateway", gw.Name())
			if err := gw.Start(gctx); err != nil {
				m.logger.Error("gateway exited with error", "gateway", gw.Name(), "error", err)
				return err
			}
			return nil
		})
	}
	err := g.Wait()
	if stopErr := m.Stop(context.Background()); stopErr != nil && err == nil {
		return stopErr
	}
	return err
}

// Stop stops all gateways in parallel under a 5s deadline, logging and
// returning the first error (if any).
func (m *MultiGateway) Stop(ctx context.Context) error {
	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	g, gctx := errgroup.WithContext(stopCtx)
	for _, gw := range m.gateways {
		gw := gw
		g.Go(func() error {
			if err := gw.Stop(gctx); err != nil {
				m.logger.Error("gateway stop error", "gateway", gw.Name(), "error", err)
				return err
			}
			m.logger.Info("gateway stopped", "gateway", gw.Name())
			return nil
		})
	}
	return g.Wait()
}
