package gateway

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

// ConnEntry represents a single gRPC connection in the pool
type ConnEntry struct {
	Conn        *grpc.ClientConn
	Target      string // address:port
	ServiceName string
	Healthy     bool
	LastUsed    time.Time
	CreatedAt   time.Time
	opts        []grpc.DialOption
}

// ConnectionPool manages a pool of gRPC connections with health checks and idle cleanup
type ConnectionPool struct {
	conns                map[string]*ConnEntry // key = service_name
	mu                   sync.RWMutex
	maxIdle              time.Duration // default 5 min, close idle connections
	healthCheckInterval time.Duration // default 30s
	done                 chan struct{}
}

// ConnStats returns statistics about a connection
type ConnStats struct {
	ServiceName string
	Target      string
	Healthy     bool
	Age         time.Duration
	IdleFor     time.Duration
}

// NewConnectionPool creates a new connection pool with defaults
func NewConnectionPool() *ConnectionPool {
	return &ConnectionPool{
		conns:                make(map[string]*ConnEntry),
		maxIdle:              5 * time.Minute,
		healthCheckInterval: 30 * time.Second,
		done:                 make(chan struct{}),
	}
}

// Get returns an existing connection or an error if not found
func (cp *ConnectionPool) Get(serviceName string) (*grpc.ClientConn, error) {
	cp.mu.RLock()
	entry, exists := cp.conns[serviceName]
	cp.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no connection found for service %s", serviceName)
	}

	// Update LastUsed timestamp
	cp.mu.Lock()
	entry.LastUsed = time.Now()
	cp.mu.Unlock()

	return entry.Conn, nil
}

// GetOrDial returns an existing connection or creates a new one lazily
func (cp *ConnectionPool) GetOrDial(serviceName, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	// First try to get existing connection
	if conn, err := cp.Get(serviceName); err == nil {
		return conn, nil
	}

	// Connection doesn't exist, create new one
	return cp.registerConnection(serviceName, target, opts...)
}

// Register pre-warms a connection at boot time (non-blocking)
func (cp *ConnectionPool) Register(serviceName, target string, opts ...grpc.DialOption) error {
	_, err := cp.registerConnection(serviceName, target, opts...)
	return err
}

// registerConnection creates and stores a new connection (helper method)
func (cp *ConnectionPool) registerConnection(serviceName, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	// Use grpc.Dial (not DialContext) for non-blocking behavior at boot
	conn, err := grpc.Dial(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s (%s): %w", serviceName, target, err)
	}

	entry := &ConnEntry{
		Conn:        conn,
		Target:      target,
		ServiceName: serviceName,
		Healthy:     true,
		LastUsed:    time.Now(),
		CreatedAt:   time.Now(),
		opts:        opts,
	}

	cp.mu.Lock()
	cp.conns[serviceName] = entry
	cp.mu.Unlock()

	return conn, nil
}

// Close closes a specific connection by service name
func (cp *ConnectionPool) Close(serviceName string) error {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	entry, exists := cp.conns[serviceName]
	if !exists {
		return fmt.Errorf("no connection found for service %s", serviceName)
	}

	err := entry.Conn.Close()
	delete(cp.conns, serviceName)
	return err
}

// CloseAll closes all connections in the pool
func (cp *ConnectionPool) CloseAll() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	for serviceName, entry := range cp.conns {
		entry.Conn.Close()
		delete(cp.conns, serviceName)
	}
}

// Stats returns statistics for all connections in the pool
func (cp *ConnectionPool) Stats() []ConnStats {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	stats := make([]ConnStats, 0, len(cp.conns))
	now := time.Now()

	for _, entry := range cp.conns {
		stats = append(stats, ConnStats{
			ServiceName: entry.ServiceName,
			Target:      entry.Target,
			Healthy:     entry.Healthy,
			Age:         now.Sub(entry.CreatedAt),
			IdleFor:     now.Sub(entry.LastUsed),
		})
	}

	return stats
}

// StartHealthCheck starts the background health check and idle cleanup goroutine
func (cp *ConnectionPool) StartHealthCheck() {
	ticker := time.NewTicker(cp.healthCheckInterval)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				cp.runHealthCheck()
				cp.cleanupIdleConnections()
			case <-cp.done:
				return
			}
		}
	}()
}

// StopHealthCheck stops the background health check goroutine
func (cp *ConnectionPool) StopHealthCheck() {
	close(cp.done)
}

// runHealthCheck performs connectivity checks on all connections
func (cp *ConnectionPool) runHealthCheck() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, entry := range cp.conns {
		healthClient := healthgrpc.NewHealthClient(entry.Conn)

		resp, err := healthClient.Check(ctx, &healthgrpc.HealthCheckRequest{})
		if err != nil {
			// Health check failed, mark as unhealthy
			entry.Healthy = false
			continue
		}

		// Mark healthy if status is SERVING
		entry.Healthy = (resp.Status == healthgrpc.HealthCheckResponse_SERVING)
	}
}

// cleanupIdleConnections closes connections that have been idle longer than maxIdle
func (cp *ConnectionPool) cleanupIdleConnections() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	now := time.Now()
	toClose := []string{}

	for serviceName, entry := range cp.conns {
		idleTime := now.Sub(entry.LastUsed)
		if idleTime > cp.maxIdle {
			toClose = append(toClose, serviceName)
		}
	}

	// Close idle connections outside the map iteration to avoid deadlock
	for _, name := range toClose {
		if entry, exists := cp.conns[name]; exists {
			entry.Conn.Close()
			delete(cp.conns, name)
		}
	}
}