package gateway

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeGateway is a test Gateway that blocks in Start until ctx is canceled.
type fakeGateway struct {
	name      string
	started   atomic.Bool
	stopped   atomic.Bool
	startErr  error
}

func (f *fakeGateway) Name() string { return f.name }
func (f *fakeGateway) Start(ctx context.Context) error {
	f.started.Store(true)
	if f.startErr != nil {
		return f.startErr
	}
	<-ctx.Done()
	return nil
}
func (f *fakeGateway) Stop(context.Context) error {
	f.stopped.Store(true)
	return nil
}

func TestMultiGatewayRunsAndStopsAll(t *testing.T) {
	t.Parallel()
	a := &fakeGateway{name: "a"}
	b := &fakeGateway{name: "b"}
	mg := NewMultiGateway(nil, a, b)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- mg.Run(ctx) }()

	// Wait for both to start, then cancel.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && (!a.started.Load() || !b.started.Load()) {
		time.Sleep(5 * time.Millisecond)
	}
	if !a.started.Load() || !b.started.Load() {
		t.Fatal("not all gateways started before timeout")
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if !a.stopped.Load() || !b.stopped.Load() {
		t.Error("not all gateways stopped")
	}
}

func TestMultiGatewayPropagatesStartError(t *testing.T) {
	t.Parallel()
	bad := &fakeGateway{name: "bad", startErr: context.Canceled}
	good := &fakeGateway{name: "good"}
	mg := NewMultiGateway(nil, bad, good)

	err := mg.Run(context.Background())
	if err == nil {
		t.Fatal("Run = nil, want error from failing gateway")
	}
	// good should still have been stopped (Stop runs regardless).
	if !good.stopped.Load() {
		t.Error("good gateway was not stopped after sibling failure")
	}
}

func TestMultiGatewayAdd(t *testing.T) {
	t.Parallel()
	mg := NewMultiGateway(nil)
	var mu sync.Mutex
	count := 0
	mg.Add(&fakeGateway{name: "x"})
	mu.Lock()
	count = len(mg.gateways)
	mu.Unlock()
	if count != 1 {
		t.Errorf("after Add, len(gateways) = %d, want 1", count)
	}
}
