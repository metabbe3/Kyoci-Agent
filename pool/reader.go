package pool

import (
	"bytes"
	"io"
)

// PooledReader wraps an io.Reader and stores its contents in a pooled buffer
type PooledReader struct {
	pool *BufferPool
	buf  *bytes.Buffer
}

// NewPooledReader creates a new PooledReader by reading all data from r
// into a buffer from the pool
func NewPooledReader(pool *BufferPool, r io.Reader) (*PooledReader, error) {
	buf := pool.Get()
	_, err := buf.ReadFrom(r)
	if err != nil {
		pool.Put(buf)
		return nil, err
	}
	return &PooledReader{
		pool: pool,
		buf:  buf,
	}, nil
}

// String returns the buffer contents as a string
func (pr *PooledReader) String() string {
	if pr == nil || pr.buf == nil {
		return ""
	}
	return pr.buf.String()
}

// Release returns the buffer to the pool
func (pr *PooledReader) Release() {
	if pr != nil && pr.buf != nil {
		pr.pool.Put(pr.buf)
		pr.buf = nil
	}
}