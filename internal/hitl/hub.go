package hitl

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	pb "github.com/metabbe3/Kyoci-Agent/internal/hitl/pb"
)

// Hub is the in-process broker between agents (which emit HelpRequests via
// RequestHelp) and operator subscribers (which receive them over gRPC).
//
// Agents call RequestHelp, which registers a waiter channel for the task ID,
// broadcasts the request to every subscriber, and blocks until either:
//   - a hint arrives via DeliverHint (called by the gRPC SubmitHint handler)
//   - the request timeout elapses (ErrNoHint)
//   - the caller's context is cancelled
//
// Thread-safe: all access is guarded by RWMutex.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[int64]chan *pb.HelpRequest // operator streams
	waiters     map[string]chan string          // taskID -> hint delivery channel
	nextSubID   int64
	timeout     time.Duration
	logger      *slog.Logger
}

// NewHub constructs a Hub with the given request timeout (fallback 5min).
// The timeout bounds how long RequestHelp blocks waiting for an operator hint.
func NewHub(timeout time.Duration, logger *slog.Logger) *Hub {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Hub{
		subscribers: make(map[int64]chan *pb.HelpRequest),
		waiters:     make(map[string]chan string),
		timeout:     timeout,
		logger:      logger.With("component", "hitl-hub"),
	}
}

// NewTaskID returns a fresh unique task ID for use in HelpRequest.TaskID.
// Used by the orchestrator at the start of every Execute call so retries
// can be correlated in operator logs.
func NewTaskID() string {
	return "task-" + uuid.NewString()
}

// RequestHelp implements HITLHook. It registers a waiter for req.TaskID,
// broadcasts the HelpRequest to all subscribers, then blocks for a hint.
//
// Returns ErrNoSubscriber immediately if no operator is connected — the
// orchestrator can then degrade cleanly instead of blocking the full timeout.
func (h *Hub) RequestHelp(ctx context.Context, req HelpRequest) (string, error) {
	if req.TaskID == "" {
		req.TaskID = NewTaskID()
	}

	// Register the waiter BEFORE broadcasting so a fast operator response in
	// the millisecond between broadcast and the wait select can still deliver.
	hintCh := make(chan string, 1)
	h.mu.Lock()
	h.waiters[req.TaskID] = hintCh
	subs := make([]chan *pb.HelpRequest, 0, len(h.subscribers))
	for _, ch := range h.subscribers {
		subs = append(subs, ch)
	}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.waiters, req.TaskID)
		h.mu.Unlock()
	}()

	if len(subs) == 0 {
		h.logger.Warn("HITL request emitted but no operator subscribed",
			"task_id", req.TaskID, "role", req.Role)
		return "", ErrNoSubscriber
	}

	pbReq := &pb.HelpRequest{
		TaskId:         req.TaskID,
		Role:           req.Role,
		Attempt:        int32(req.Attempt),
		Question:       req.Question,
		LastError:      req.LastError,
		AttemptedFixes: req.AttemptedFixes,
		EmittedAtUnix:  time.Now().Unix(),
	}
	for _, ch := range subs {
		select {
		case ch <- pbReq:
		default:
			h.logger.Warn("HITL subscriber queue full, dropping",
				"task_id", req.TaskID)
		}
	}
	h.logger.Info("HITL request emitted",
		"task_id", req.TaskID, "role", req.Role, "subscribers", len(subs))

	waitCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	select {
	case hint := <-hintCh:
		h.logger.Info("HITL hint received",
			"task_id", req.TaskID, "hint_len", len(hint))
		return hint, nil
	case <-waitCtx.Done():
		h.logger.Warn("HITL hint timed out", "task_id", req.TaskID, "timeout", h.timeout)
		return "", ErrNoHint
	}
}

// DeliverHint is called by the gRPC SubmitHint handler to deliver an operator
// hint to the waiting agent goroutine. Returns false if no waiter exists for
// taskID (e.g. the orchestrator gave up, or the task ID was never registered).
func (h *Hub) DeliverHint(taskID, hint string) bool {
	h.mu.RLock()
	ch, ok := h.waiters[taskID]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	select {
	case ch <- hint:
		return true
	default:
		return false
	}
}

// Subscribe registers a new operator subscriber and returns:
//   - a channel that receives every new pb.HelpRequest
//   - an unsubscribe function that MUST be called when the stream ends
//
// Used by the gRPC server to bridge stream RPCs.
func (h *Hub) Subscribe() (<-chan *pb.HelpRequest, func()) {
	h.mu.Lock()
	h.nextSubID++
	id := h.nextSubID
	ch := make(chan *pb.HelpRequest, 16)
	h.subscribers[id] = ch
	count := len(h.subscribers)
	h.mu.Unlock()

	h.logger.Info("operator subscribed", "sub_id", id, "total_subs", count)

	return ch, func() {
		h.mu.Lock()
		delete(h.subscribers, id)
		count := len(h.subscribers)
		h.mu.Unlock()
		h.logger.Info("operator unsubscribed", "sub_id", id, "total_subs", count)
	}
}

// HasSubscriber reports whether at least one operator is currently subscribed.
// Useful for health checks and orchestrator decisions about whether HITL is
// even available.
func (h *Hub) HasSubscriber() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers) > 0
}
