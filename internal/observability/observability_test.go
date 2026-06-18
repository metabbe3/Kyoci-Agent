package observability

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestSetupDisabled exercises the off-by-default path: no exporters, no error,
// and a callable shutdown. Setup is process-global (sync.Once), so this is the
// single test that invokes it.
func TestSetupDisabled(t *testing.T) {
	t.Parallel()
	shutdown, err := Setup(context.Background(), Config{})
	if err != nil {
		t.Fatalf("Setup() error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("Setup() returned nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown() error: %v", err)
	}
}

func TestInstrumentsNeverNil(t *testing.T) {
	t.Parallel()
	ins := Instruments()
	if ins == nil {
		t.Fatal("Instruments() = nil")
	}
	// Recorders must be safe to call before/without metrics enabled (no-op).
	ins.AgentExecutions.Add(context.Background(), 1)
	ins.LLMRequestDuration.Record(context.Background(), 0.123)
	ins.ToolCalls.Add(context.Background(), 1)
	ins.HITLApprovals.Add(context.Background(), 1)
	ins.MemoryOps.Add(context.Background(), 1)
}

func TestTracerMeterNonNil(t *testing.T) {
	t.Parallel()
	if Tracer("test") == nil {
		t.Error("Tracer() = nil")
	}
	if Meter("test") == nil {
		t.Error("Meter() = nil")
	}
}

func TestLoggerNonNil(t *testing.T) {
	t.Parallel()
	if l := Logger("kyoci", 0); l == nil {
		t.Error("Logger() = nil")
	}
}

func TestMountDebugRoutes(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	MountDebug(mux, "test-version", func() bool { return true })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := srv.Client()

	cases := []struct {
		path     string
		wantCode int
	}{
		{"/healthz", http.StatusOK},
		{"/readyz", http.StatusOK},
		{"/version", http.StatusOK},
		{"/metrics", http.StatusOK},
	}
	for _, c := range cases {
		resp, err := client.Get(srv.URL + c.path)
		if err != nil {
			t.Errorf("GET %s: %v", c.path, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != c.wantCode {
			t.Errorf("GET %s: status = %d, want %d", c.path, resp.StatusCode, c.wantCode)
		}
	}

	// /readyz should be 503 when not ready.
	mux2 := http.NewServeMux()
	MountDebug(mux2, "v", func() bool { return false })
	srv2 := httptest.NewServer(mux2)
	defer srv2.Close()
	resp, err := srv2.Client().Get(srv2.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz not-ready: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("/readyz not-ready status = %d, want 503", resp.StatusCode)
	}

	// /version body contains the version string.
	resp, err = srv.Client().Get(srv.URL + "/version")
	if err != nil {
		t.Fatalf("GET /version: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var v map[string]string
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("version body not JSON: %v", err)
	}
	if v["version"] != "test-version" {
		t.Errorf("version body = %v, want test-version", v)
	}
}

func TestManagerShutdownReverseOrder(t *testing.T) {
	t.Parallel()
	var (
		mu   sync.Mutex
		order []string
	)
	rec := func(name string) func(context.Context) error {
		return func(context.Context) error {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return nil
		}
	}
	mgr := NewManager(nil, nil)
	mgr.Add("http", time.Second, rec("http"))
	mgr.Add("grpc", time.Second, rec("grpc"))
	mgr.Add("orchestrator", time.Second, rec("orchestrator"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- mgr.Run(ctx, func(ctx context.Context) error { <-ctx.Done(); return nil }) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	// Reverse registration order, OTel flush is nil here so omitted.
	want := []string{"orchestrator", "grpc", "http"}
	if len(order) != len(want) {
		t.Fatalf("shutdown order = %v, want %v", order, want)
	}
	for i, w := range want {
		if order[i] != w {
			t.Errorf("shutdown order[%d] = %q, want %q (full: %v)", i, order[i], w, order)
		}
	}
}

func TestManagerShutdownErrorPropagation(t *testing.T) {
	t.Parallel()
	mgr := NewManager(nil, nil)
	mgr.Add("fail", time.Second, func(context.Context) error {
		return context.DeadlineExceeded
	})
	mgr.Add("ok", time.Second, func(context.Context) error { return nil })
	err := mgr.shutdown(context.Background())
	if err == nil {
		t.Fatal("shutdown() = nil, want error from failing component")
	}
}

func TestKeyValueCoercion(t *testing.T) {
	t.Parallel()
	cases := []struct {
		v   any
		ok  bool
	}{
		{"s", true}, {true, true}, {int(3), true}, {int64(3), true}, {3.14, true},
		{nil, false}, {[]int{1}, false},
	}
	for _, c := range cases {
		_, ok := toKeyValue("k", c.v)
		if ok != c.ok {
			t.Errorf("toKeyValue(%T) ok = %v, want %v", c.v, ok, c.ok)
		}
	}
}

func TestRatioOr(t *testing.T) {
	t.Parallel()
	if r := ratioOr(0, 0.5); r != 0.5 {
		t.Errorf("ratioOr(0) = %v", r)
	}
	if r := ratioOr(1.5, 0.5); r != 0.5 {
		t.Errorf("ratioOr(1.5) = %v", r)
	}
	if r := ratioOr(0.3, 0.5); r != 0.3 {
		t.Errorf("ratioOr(0.3) = %v", r)
	}
}
