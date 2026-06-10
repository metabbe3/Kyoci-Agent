package pool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrQueueFull is returned when the job queue is full
var ErrQueueFull = errors.New("queue full")

// Job represents a unit of work to be processed
type Job struct {
	Ctx         context.Context
	Task        interface{}
	ResultCh    chan interface{}
	SubmittedAt time.Time
}

// WorkerPool manages concurrent task execution with bounded queue
type WorkerPool struct {
	maxWorkers     int
	queueSize      int
	jobQueue       chan Job
	wg             sync.WaitGroup
	active         int64
	totalProcessed int64
	totalFailed    int64
	quit           chan struct{}
	handler        func(ctx context.Context, task interface{}) (interface{}, error)
}

// PoolStats contains current worker pool statistics
type PoolStats struct {
	ActiveWorkers int64
	TotalProcessed int64
	TotalFailed    int64
	QueueLength    int
	QueueCapacity  int
}

// NewWorkerPool creates a new bounded worker pool
func NewWorkerPool(maxWorkers, queueSize int, handler func(ctx context.Context, task interface{}) (interface{}, error)) *WorkerPool {
	return &WorkerPool{
		maxWorkers:     maxWorkers,
		queueSize:      queueSize,
		jobQueue:       make(chan Job, queueSize),
		quit:           make(chan struct{}),
		handler:        handler,
	}
}

// Start initializes the worker goroutines
func (wp *WorkerPool) Start() {
	for i := 0; i < wp.maxWorkers; i++ {
		wp.wg.Add(1)
		go wp.worker()
	}
}

// Stop gracefully shuts down the worker pool
func (wp *WorkerPool) Stop() {
	close(wp.quit)
	wp.wg.Wait()
}

// worker processes tasks from the queue
func (wp *WorkerPool) worker() {
	defer wp.wg.Done()
	for {
		select {
		case job := <-wp.jobQueue:
			atomic.AddInt64(&wp.active, 1)
			wp.processJob(job)
			atomic.AddInt64(&wp.active, -1)
		case <-wp.quit:
			return
		}
	}
}

// processJob executes a single job
func (wp *WorkerPool) processJob(job Job) {
	result, err := wp.handler(job.Ctx, job.Task)
	
	if err != nil {
		atomic.AddInt64(&wp.totalFailed, 1)
	}
	atomic.AddInt64(&wp.totalProcessed, 1)

	select {
	case job.ResultCh <- result:
	case <-job.Ctx.Done():
		// Context canceled before we could send result
	}
	close(job.ResultCh)
}

// Submit submits a job and blocks until result or timeout
func (wp *WorkerPool) Submit(ctx context.Context, task interface{}) (interface{}, error) {
	resultCh := make(chan interface{}, 1)
	job := Job{
		Ctx:         ctx,
		Task:        task,
		ResultCh:    resultCh,
		SubmittedAt: time.Now(),
	}

	select {
	case wp.jobQueue <- job:
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, ErrQueueFull
	}

	select {
	case result := <-resultCh:
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// TrySubmit attempts to submit a job non-blocking, returns success status
func (wp *WorkerPool) TrySubmit(ctx context.Context, task interface{}) (interface{}, bool) {
	resultCh := make(chan interface{}, 1)
	job := Job{
		Ctx:         ctx,
		Task:        task,
		ResultCh:    resultCh,
		SubmittedAt: time.Now(),
	}

	select {
	case wp.jobQueue <- job:
		// Job submitted, wait for result
		select {
		case result := <-resultCh:
			return result, true
		case <-ctx.Done():
			return nil, false
		}
	default:
		return nil, false
	}
}

// Stats returns current pool statistics
func (wp *WorkerPool) Stats() PoolStats {
	return PoolStats{
		ActiveWorkers:  atomic.LoadInt64(&wp.active),
		TotalProcessed: atomic.LoadInt64(&wp.totalProcessed),
		TotalFailed:    atomic.LoadInt64(&wp.totalFailed),
		QueueLength:    len(wp.jobQueue),
		QueueCapacity:  cap(wp.jobQueue),
	}
}