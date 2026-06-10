package llm

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// OllamaQueue serializes Ollama API requests so the single-GPU inference
// server processes one request at a time while callers wait asynchronously.
//
// Flow:  caller → Submit() → channel → worker → Ollama API → result channel → caller
//
// Features:
//   - Serial execution (1 concurrent Ollama call)
//   - Async wait with context cancellation
//   - Queue depth limit (back-pressure)
//   - Metrics (queue depth, avg wait time, total processed)
type OllamaQueue struct {
	requests chan *queueRequest
	wg       sync.WaitGroup

	// Metrics
	processed   atomic.Int64
	dropped     atomic.Int64
	totalWaitMs atomic.Int64

	mu   sync.Mutex
	done bool
}

// queueRequest is a queued Ollama API call.
type queueRequest struct {
	ctx    context.Context
	chatFn func(ctx context.Context) (*Response, error)
	result chan<- queueResult
}

// queueResult holds the outcome of a queued request.
type queueResult struct {
	resp *Response
	err  error
}

// NewOllamaQueue creates a new serial queue with the given buffer depth.
// maxQueueDepth controls how many requests can be waiting before Submit() returns error.
func NewOllamaQueue(maxQueueDepth int) *OllamaQueue {
	q := &OllamaQueue{
		requests: make(chan *queueRequest, maxQueueDepth),
	}

	// Single worker — serializes all Ollama calls
	q.wg.Add(1)
	go q.worker()

	slog.Info("Ollama queue started", "max_depth", maxQueueDepth)
	return q
}

// Submit enqueues a Chat function for serial execution.
// Returns the response or error when the request is processed.
// Blocks if queue is full (back-pressure). Respects context cancellation.
func (q *OllamaQueue) Submit(ctx context.Context, chatFn func(ctx context.Context) (*Response, error)) (*Response, error) {
	resultCh := make(chan queueResult, 1)

	req := &queueRequest{
		ctx:    ctx,
		chatFn: chatFn,
		result: resultCh,
	}

	start := time.Now()

	// Try to enqueue (respect context)
	select {
	case q.requests <- req:
		// Enqueued successfully
	case <-ctx.Done():
		q.dropped.Add(1)
		return nil, ctx.Err()
	}

	// Wait for result (respect context)
	select {
	case result := <-resultCh:
		waitMs := time.Since(start).Milliseconds()
		q.totalWaitMs.Add(waitMs)
		q.processed.Add(1)
		return result.resp, result.err
	case <-ctx.Done():
		q.dropped.Add(1)
		return nil, ctx.Err()
	}
}

// Close stops the queue worker. Blocks until in-flight request completes.
func (q *OllamaQueue) Close() {
	q.mu.Lock()
	if q.done {
		q.mu.Unlock()
		return
	}
	q.done = true
	q.mu.Unlock()

	close(q.requests)
	q.wg.Wait()
	slog.Info("Ollama queue stopped",
		"processed", q.processed.Load(),
		"dropped", q.dropped.Load(),
		"avg_wait_ms", q.AvgWaitMs(),
	)
}

// Stats returns current queue metrics.
func (q *OllamaQueue) Stats() map[string]interface{} {
	return map[string]interface{}{
		"queue_depth":  len(q.requests),
		"processed":    q.processed.Load(),
		"dropped":      q.dropped.Load(),
		"avg_wait_ms":  q.AvgWaitMs(),
	}
}

// AvgWaitMs returns average wait time in milliseconds.
func (q *OllamaQueue) AvgWaitMs() int64 {
	p := q.processed.Load()
	if p == 0 {
		return 0
	}
	return q.totalWaitMs.Load() / p
}

func (q *OllamaQueue) worker() {
	defer q.wg.Done()

	for req := range q.requests {
		// Check if context already cancelled
		if err := req.ctx.Err(); err != nil {
			req.result <- queueResult{err: err}
			continue
		}

		start := time.Now()
		resp, err := req.chatFn(req.ctx)
		elapsed := time.Since(start)

		if err != nil {
			slog.Warn("Ollama request failed",
				"error", err,
				"elapsed_ms", elapsed.Milliseconds(),
			)
		} else {
			slog.Debug("Ollama request completed",
				"elapsed_ms", elapsed.Milliseconds(),
			)
		}

		req.result <- queueResult{resp: resp, err: err}
	}
}
