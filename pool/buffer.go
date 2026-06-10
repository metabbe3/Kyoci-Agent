package pool

import (
	"bytes"
	"sync"
)

// BufferPool is a memory-efficient pool of bytes.Buffer objects
// with pre-allocated capacity to reduce allocations
type BufferPool struct {
	pool sync.Pool
}

// initialCapacity is the pre-allocated size for buffers (4KB)
const initialCapacity = 4096

// NewBufferPool creates a new buffer pool with 4KB initial capacity
func NewBufferPool() *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, initialCapacity))
			},
		},
	}
}

// Get retrieves a buffer from the pool
func (p *BufferPool) Get() *bytes.Buffer {
	return p.pool.Get().(*bytes.Buffer)
}

// Put returns a buffer to the pool after resetting it
func (p *BufferPool) Put(b *bytes.Buffer) {
	b.Reset()
	p.pool.Put(b)
}