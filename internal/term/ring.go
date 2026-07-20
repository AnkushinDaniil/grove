// Package term runs and multiplexes a single interactive PTY process: a
// fixed-capacity scrollback ring, a fan-out hub letting many viewers attach to
// the live byte stream, and lifecycle control (resize, stop, exit reaping).
package term

import "sync"

// DefaultRingSize is the scrollback ring capacity used when WithRingSize is not
// supplied.
const DefaultRingSize = 512 * 1024

// Ring is a fixed-capacity byte ring holding the most recent output. Once full
// it overwrites the oldest bytes. It is safe for concurrent use.
type Ring struct {
	mu    sync.Mutex
	buf   []byte
	head  int   // next write index; also the oldest byte once full
	total int64 // total bytes ever written
}

// NewRing returns a ring of the given capacity. A non-positive size falls back
// to DefaultRingSize.
func NewRing(size int) *Ring {
	if size <= 0 {
		size = DefaultRingSize
	}
	return &Ring{buf: make([]byte, size)}
}

// Write appends p, discarding the oldest bytes once the ring is full. It never
// fails and always consumes all of p.
func (r *Ring) Write(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(p)
	r.total += int64(n)
	size := len(r.buf)
	if n >= size {
		// Only the final size bytes survive; they fill the buffer from 0.
		copy(r.buf, p[n-size:])
		r.head = 0
		return
	}
	first := copy(r.buf[r.head:], p)
	if first < n {
		copy(r.buf, p[first:])
	}
	r.head = (r.head + n) % size
}

// Bytes returns a linearized copy of the ring's current contents, oldest first.
func (r *Ring) Bytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	size := len(r.buf)
	if r.total < int64(size) {
		out := make([]byte, r.head)
		copy(out, r.buf[:r.head])
		return out
	}
	out := make([]byte, size)
	n := copy(out, r.buf[r.head:])
	copy(out[n:], r.buf[:r.head])
	return out
}

// Total reports the number of bytes ever written to the ring.
func (r *Ring) Total() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.total
}
