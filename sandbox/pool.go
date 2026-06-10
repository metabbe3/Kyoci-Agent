package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// PoolStats represents statistics about the container pool
type PoolStats struct {
	Available int
	InUse     int
	Total     int
}

// ContainerPool manages a pool of pre-warmed Docker containers
type ContainerPool struct {
	sandbox    *DockerSandbox
	size       int
	containers []string
	available  chan string
	mu         sync.Mutex
	closed     bool
	workDir    string
}

// NewContainerPool creates a new container pool
func NewContainerPool(sandbox *DockerSandbox, size int, workDir string) (*ContainerPool, error) {
	if size <= 0 {
		return nil, fmt.Errorf("pool size must be positive")
	}
	if sandbox == nil {
		return nil, fmt.Errorf("sandbox cannot be nil")
	}

	pool := &ContainerPool{
		sandbox:    sandbox,
		size:       size,
		containers: make([]string, 0, size),
		available:  make(chan string, size),
		closed:     false,
		workDir:    workDir,
	}

	return pool, nil
}

// WarmUp pre-starts containers and adds them to the pool
func (p *ContainerPool) WarmUp(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return fmt.Errorf("pool is closed")
	}

	// Images to pre-warm
	images := []string{
		"alpine:latest",
		"python:3.11-alpine",
		"node:20-alpine",
	}

	for i := 0; i < p.size; i++ {
		// Rotate through different images
		image := images[i%len(images)]

		containerID, err := p.startWarmContainer(ctx, image)
		if err != nil {
			return fmt.Errorf("failed to start warm container %d: %w", i, err)
		}

		p.containers = append(p.containers, containerID)
		p.available <- containerID
	}

	return nil
}

// startWarmContainer starts a warm container and returns its ID
func (p *ContainerPool) startWarmContainer(ctx context.Context, image string) (string, error) {
	// Pull image if needed
	if err := p.sandbox.PullImage(image); err != nil {
		return "", fmt.Errorf("failed to pull image %s: %w", image, err)
	}

	// Start container with long-running process
	cmdArgs := []string{
		"run", "-d",
		"--network=none",
		"--memory=128m",
		"--cpus=0.5",
		"--pids-limit=64",
		"--read-only",
		"--tmpfs", "/tmp:size=10m",
		"--user=nobody",
		"--security-opt=no-new-privileges",
		"-v", p.workDir + ":/workspace:ro",
		image,
		"/bin/sh", "-c", "tail -f /dev/null", // Keep container running
	}

	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to start container: %s: %w", string(output), err)
	}

	containerID := string(output)
	containerID = trimContainerID(containerID)

	return containerID, nil
}

// trimContainerID trims whitespace from container ID
func trimContainerID(id string) string {
	// Remove any trailing newlines or spaces
	for len(id) > 0 && (id[len(id)-1] == '\n' || id[len(id)-1] == '\r' || id[len(id)-1] == ' ') {
		id = id[:len(id)-1]
	}
	return id
}

// Get returns an available container from the pool
func (p *ContainerPool) Get(ctx context.Context) (string, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return "", fmt.Errorf("pool is closed")
	}
	p.mu.Unlock()

	select {
	case containerID := <-p.available:
		return containerID, nil
	case <-ctx.Done():
		// Create a new container if pool is empty and not closed
		p.mu.Lock()
		defer p.mu.Unlock()

		if p.closed {
			return "", fmt.Errorf("pool is closed")
		}

		// Create a new container on demand
		newID, err := p.startWarmContainer(ctx, "alpine:latest")
		if err != nil {
			return "", fmt.Errorf("failed to create new container: %w", err)
		}

		p.containers = append(p.containers, newID)
		return newID, nil
	}
}

// Put returns a container to the pool
func (p *ContainerPool) Put(containerID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return // Don't return to closed pool
	}

	select {
	case p.available <- containerID:
		// Successfully returned to pool
	default:
		// Pool is full, just discard reference
	}
}

// Drain destroys all containers in the pool
func (p *ContainerPool) Drain() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	p.closed = true
	close(p.available)

	// Stop all containers
	for _, containerID := range p.containers {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		exec.CommandContext(ctx, "docker", "stop", containerID).Run()
		exec.CommandContext(ctx, "docker", "rm", "-f", containerID).Run()
		cancel()
	}

	p.containers = nil
}

// Stats returns current pool statistics
func (p *ContainerPool) Stats() PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()

	availableCount := len(p.available)
	inUseCount := len(p.containers) - availableCount

	return PoolStats{
		Available: availableCount,
		InUse:     inUseCount,
		Total:     len(p.containers),
	}
}

// ExecInContainer executes a command in a specific container
func (p *ContainerPool) ExecInContainer(ctx context.Context, containerID, script, lang string) (*ExecResult, error) {
	// Build docker exec command
	cmdArgs := []string{
		"exec",
		containerID,
	}

	// Add language-specific command
	cmdParts := GetCommandForLang(lang, "/workspace/script")
	if len(cmdParts) == 0 {
		cmdArgs = append(cmdArgs, "/bin/sh", "-c", script)
	} else {
		cmdArgs = append(cmdArgs, cmdParts...)
	}

	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)

	// Add timeout if not already set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.sandbox.timeout)
		defer cancel()
	}

	// Execute
	output, err := cmd.CombinedOutput()
	duration := time.Since(ctx.Value("start_time").(time.Time))

	result := &ExecResult{
		Stdout:   string(output),
		Stderr:   "",
		Duration: duration,
		TimedOut: ctx.Err() == context.DeadlineExceeded,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}

	return result, nil
}

// IsClosed returns whether the pool is closed
func (p *ContainerPool) IsClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

// GetSize returns the configured pool size
func (p *ContainerPool) GetSize() int {
	return p.size
}

// Resize resizes the pool (can be larger or smaller)
func (p *ContainerPool) Resize(ctx context.Context, newSize int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return fmt.Errorf("pool is closed")
	}

	if newSize <= 0 {
		return fmt.Errorf("new size must be positive")
	}

	if newSize == p.size {
		return nil // No change
	}

	// If growing, start new containers
	if newSize > p.size {
		images := []string{"alpine:latest", "python:3.11-alpine", "node:20-alpine"}
		for i := p.size; i < newSize; i++ {
			image := images[i%len(images)]
			containerID, err := p.startWarmContainer(ctx, image)
			if err != nil {
				return fmt.Errorf("failed to start container during resize: %w", err)
			}
			p.containers = append(p.containers, containerID)
			p.available <- containerID
		}
	} else {
		// If shrinking, remove excess containers
		excess := len(p.containers) - newSize
		for i := 0; i < excess; i++ {
			select {
			case containerID := <-p.available:
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				exec.CommandContext(ctx, "docker", "stop", containerID).Run()
				exec.CommandContext(ctx, "docker", "rm", "-f", containerID).Run()
				cancel()
			default:
				// No more available containers to remove
				break
			}
		}
		// Truncate containers list
		p.containers = p.containers[:min(len(p.containers), newSize)]
	}

	p.size = newSize
	return nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}