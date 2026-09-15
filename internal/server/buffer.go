package server

import (
	"bytes"
	"sync"
)

// BufferPool maintains reusable byte buffers to reduce allocations during high-throughput queries
type BufferPool struct {
	pool sync.Pool
}

// NewBufferPool creates a new pool
func NewBufferPool() *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() any {
				return bytes.NewBuffer(make([]byte, 0, 4096))
			},
		},
	}
}

// Get retrieves a cleared buffer from the pool
func (bp *BufferPool) Get() *bytes.Buffer {
	buf := bp.pool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// Put returns the buffer to the pool
func (bp *BufferPool) Put(buf *bytes.Buffer) {
	if buf.Cap() > 65536 {
		return // Drop excessively large buffers to avoid holding memory
	}
	bp.pool.Put(buf)
}
