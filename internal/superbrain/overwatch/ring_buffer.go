// Package overwatch provides real-time execution monitoring capabilities for the Superbrain system.
// It implements the Overwatch Layer which wraps CLI executions with monitoring, heartbeat detection,
// and silence threshold detection to identify hung processes.
package overwatch

import (
	"sync"
)

// RingBuffer is a thread-safe circular buffer for storing log lines.
// It maintains a fixed-size buffer that overwrites the oldest entries when full,
// ensuring memory-bounded log retention for diagnostic purposes.
type RingBuffer struct {
	mu       sync.RWMutex
	buffer   []string
	size     int  // Maximum capacity of the buffer
	head     int  // Index where next write will occur
	count    int  // Current number of elements in buffer
	isFull   bool // Whether the buffer has wrapped around
}

// NewRingBuffer creates a new RingBuffer with the specified capacity.
// If size is <= 0, a default size of 50 is used.
func NewRingBuffer(size int) *RingBuffer {
	if size <= 0 {
		size = 50 // Default buffer size
	}
	return &RingBuffer{
		buffer: make([]string, size),
		size:   size,
		head:   0,
		count:  0,
		isFull: false,
	}
}

// Write adds a line to the buffer. If the buffer is full, the oldest line is overwritten.
// This operation is thread-safe.
func (rb *RingBuffer) Write(line string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.buffer[rb.head] = line
	rb.head = (rb.head + 1) % rb.size

	if rb.count < rb.size {
		rb.count++
	} else {
		rb.isFull = true
	}
}

// GetLast returns the last n lines from the buffer in chronological order.
// If n is greater than the number of stored lines, all stored lines are returned.
// If n is <= 0, an empty slice is returned.
// This operation is thread-safe.
func (rb *RingBuffer) GetLast(n int) []string {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if n <= 0 {
		return []string{}
	}

	// Limit n to the actual number of elements
	if n > rb.count {
		n = rb.count
	}

	if n == 0 {
		return []string{}
	}

	result := make([]string, n)

	// Calculate the starting position for reading
	// We need to read the last n elements in chronological order
	startIdx := (rb.head - n + rb.size) % rb.size

	for i := 0; i < n; i++ {
		idx := (startIdx + i) % rb.size
		result[i] = rb.buffer[idx]
	}

	return result
}

// Clear removes all lines from the buffer.
// This operation is thread-safe.
func (rb *RingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	// Reset the buffer state
	rb.buffer = make([]string, rb.size)
	rb.head = 0
	rb.count = 0
	rb.isFull = false
}

// Len returns the current number of lines in the buffer.
// This operation is thread-safe.
func (rb *RingBuffer) Len() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}

// Cap returns the maximum capacity of the buffer.
func (rb *RingBuffer) Cap() int {
	return rb.size
}

// IsFull returns true if the buffer has reached capacity and is overwriting old entries.
// This operation is thread-safe.
func (rb *RingBuffer) IsFull() bool {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.isFull
}

// GetAll returns all lines currently in the buffer in chronological order.
// This operation is thread-safe.
func (rb *RingBuffer) GetAll() []string {
	return rb.GetLast(rb.Len())
}
