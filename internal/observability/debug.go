package observability

import (
	"encoding/json"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ReadyFunc reports whether the service is ready to serve traffic.
type ReadyFunc func() bool

// MountDebug registers health, readiness, metrics, version, and pprof endpoints
// on mux. /metrics serves Prometheus (Go runtime metrics always; OTel
// instruments when metrics are enabled). /readyz returns 200 when ready is nil
// or ready() is true, else 503.
func MountDebug(mux *http.ServeMux, version string, ready ReadyFunc) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if ready == nil || ready() {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not ready"})
	})
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"version": version,
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})

	// pprof endpoints under /debug/pprof/.
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
