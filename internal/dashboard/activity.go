package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// ActivityBroker — process-wide fan-out of agent activity events.
//
// Workers/delegations publish ActivityEvents here as they run. The broker
// multiplexes each event to every active subscriber (the Live Activity panel
// SSE endpoint, plus any future debug listeners).
//
// Design:
//   - Subscribers get a buffered channel (cap 64). When full, old events are
//     dropped — the panel is best-effort and we never block a worker on UI
//     telemetry.
//   - Unsubscribe on context cancellation or HTTP close. Leaked subscribers
//     get garbage-collected when their goroutine exits.
//   - No persistence. Past runs vanish on refresh. (DB-backed history is
//     explicitly out of scope per the plan.)
// =====================================================================================

// ActivityBroker fans out agent activity events to N subscribers. Safe for
// concurrent use. Zero-value is NOT usable — use NewActivityBroker.
type ActivityBroker struct {
	mu          sync.RWMutex
	subscribers map[int]chan kyoci.ActivityEvent
	nextID      int
}

// NewActivityBroker returns a ready-to-use broker with no subscribers.
func NewActivityBroker() *ActivityBroker {
	return &ActivityBroker{
		subscribers: make(map[int]chan kyoci.ActivityEvent),
	}
}

// Subscribe returns a channel that receives every subsequent event. The
// caller MUST call the returned cancel() when done (typically via defer in an
// HTTP handler) to avoid leaking the channel.
//
// The channel is buffered (cap 64). On overflow, oldest events are dropped
// to make room — the panel catches up via subsequent events.
func (b *ActivityBroker) Subscribe() (<-chan kyoci.ActivityEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.nextID
	b.nextID++
	ch := make(chan kyoci.ActivityEvent, 64)
	b.subscribers[id] = ch
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if c, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(c)
		}
	}
}

// Publish ships an event to every active subscriber. Non-blocking: full
// subscribers get the oldest event dropped. No subscribers = no-op.
func (b *ActivityBroker) Publish(evt kyoci.ActivityEvent) {
	if evt.Timestamp == 0 {
		evt.Timestamp = time.Now().UnixMilli()
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- evt:
		default:
			// Channel full — drop oldest, then retry. This keeps the latest
			// events visible at the cost of historical ones, which is the
			// right tradeoff for a live panel.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- evt:
			default:
			}
		}
	}
}

// SubscriberCount returns the current number of active subscribers. Useful
// for diagnostics.
func (b *ActivityBroker) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}

// =====================================================================================
// HTTP handler — GET /api/dashboard/activity
//
// Opens an SSE stream that receives every activity event broker-wide. Used by
// the Live Activity panel. Closes when the client disconnects.
//
// Wire format (one event per line):
//
//	event: activity
//	data: {"type":"task_start","task_id":"step-1","task_name":"...","timestamp":...}
//
// =====================================================================================

// handleActivityStream is the SSE endpoint for the Live Activity panel.
func (s *Server) handleActivityStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if s.activityBroker == nil {
		http.Error(w, `{"error":"activity broker not initialized"}`, http.StatusInternalServerError)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // nginx: don't buffer SSE

	ch, cancel := s.activityBroker.Subscribe()
	defer cancel()

	// Heartbeat every 15s so proxies don't kill idle connections. The data
	// line is a no-op comment that the client ignores.
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if _, err := w.Write([]byte(": heartbeat\n\n")); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			payload, err := json.Marshal(map[string]any{
				"activity": evt,
			})
			if err != nil {
				continue
			}
			if _, err := w.Write([]byte("event: activity\ndata: " + string(payload) + "\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// Server.PublishActivity is the orchestrator-facing publish entry point. It's
// a method on Server so the orchestrator can call it via the dashboard
// reference it holds. Safe to call when broker is nil (no-op).
func (s *Server) PublishActivity(evt kyoci.ActivityEvent) {
	if s.activityBroker == nil {
		return
	}
	s.activityBroker.Publish(evt)
}

// ensure context import is used even if future edits remove the only ref.
var _ = context.Background
