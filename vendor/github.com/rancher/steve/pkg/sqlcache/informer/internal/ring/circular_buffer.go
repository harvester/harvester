package ring

import (
	"container/ring"
	"context"
	"errors"
	"fmt"
	"sync"
)

// ErrSlowReader is returned when a reader has been lapped by the writer.
var (
	ErrSlowReader   = errors.New("slow reader detected")
	ErrBufferClosed = errors.New("buffer was closed")
)

// ringItem wraps the stored value with a sequence number for lap detection.
type ringItem[T any] struct {
	seq   uint64
	value T
}

// CircularBuffer is a generic, lock-based, fixed-size circular buffer.
// It supports one writer and multiple readers.
type CircularBuffer[T any] struct {
	cond     *sync.Cond
	sequence uint64 // Monotonically increasing write counter.
	data     *ring.Ring
	head     *ring.Ring
	size     int
	closed   bool
}

// NewCircularBuffer creates a new generic circular buffer.
func NewCircularBuffer[T any](size int) *CircularBuffer[T] {
	if size <= 0 {
		return nil
	}
	cb := &CircularBuffer[T]{
		data: ring.New(size),
		size: size,
	}
	cb.head = cb.data
	cb.cond = sync.NewCond(&sync.Mutex{})
	return cb
}

// Write adds a new value to the buffer.
func (b *CircularBuffer[T]) Write(v T) error {
	b.cond.L.Lock()
	defer b.cond.L.Unlock()

	if b.closed {
		return ErrBufferClosed
	}

	b.sequence++
	b.head.Value = ringItem[T]{
		seq:   b.sequence,
		value: v,
	}
	b.head = b.head.Next()

	b.cond.Broadcast()
	return nil
}

// Close marks the buffer as closed, so readers will no longer block on reading.
// Readers can still consume remaining items in the buffer, after which they'll receive an ErrBufferClosed error.
func (b *CircularBuffer[T]) Close() {
	b.cond.L.Lock()
	defer b.cond.L.Unlock()

	b.closed = true
	b.cond.Broadcast()
}

// Reader represents a single consumer of the CircularBuffer.
type Reader[T any] struct {
	buffer  *CircularBuffer[T]
	cursor  *ring.Ring
	lastSeq uint64 // Sequence number of the last item read.
}

// NewReader creates a new reader associated with the buffer.
func (b *CircularBuffer[T]) NewReader() *Reader[T] {
	b.cond.L.Lock()
	defer b.cond.L.Unlock()

	// A new reader is considered up-to-date with the writer.
	return &Reader[T]{
		buffer:  b,
		cursor:  b.head,
		lastSeq: b.sequence,
	}
}

// newDataAvailableLocked checks if this reader has already consumed all the data that was written
// assumes the lock is held by the caller
func (r *Reader[T]) newDataAvailableLocked() bool {
	return r.lastSeq != r.buffer.sequence
}

// shouldWaitLocked is the condition for waiting on the buffer's sync.Cond
// assumes the lock is held by the caller
func (r *Reader[T]) shouldWaitLocked() bool {
	return !r.buffer.closed && !r.newDataAvailableLocked()
}

// waitForDataAvailableLocked and will wait (if needed) until new data is available, or the context is canceled, whatever happens first
// assumes the lock is held by the caller
func (r *Reader[T]) waitForDataAvailableLocked(ctx context.Context) (err error) {
	if ctx.Err() != nil || !r.shouldWaitLocked() {
		return ctx.Err()
	}

	done := make(chan struct{})
	go func() {
		// Wait in a loop until data is available OR the context is cancelled.
		for ctx.Err() == nil && r.shouldWaitLocked() {
			r.buffer.cond.Wait()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		// we cannot selectively wake up this goroutine, so we need to broadcast, even if that means waking all the other waiters
		// every waiter should use wait within a loop that checks if the condition is met
		r.buffer.cond.Broadcast()
		// we cannot leave before goroutine finishes: this function was entered holding the lock, and the caller expects to still hold it when it finishes
		<-done
	}
	return ctx.Err()
}

// Read blocks until a new value is available and returns it.
// It returns an error if the context is cancelled or if the reader is lapped.
func (r *Reader[T]) Read(ctx context.Context) (T, error) {
	var zero T
	b := r.buffer
	b.cond.L.Lock()
	defer b.cond.L.Unlock()

	// If data is not yet available, we need to wait.
	if err := r.waitForDataAvailableLocked(ctx); err != nil {
		return zero, err
	}

	if !r.newDataAvailableLocked() && r.buffer.closed {
		return zero, ErrBufferClosed
	}

	// At this point, data is available and the context is still valid.
	// Proceed with the normal read and lap detection logic.
	item, ok := r.cursor.Value.(ringItem[T])
	if !ok {
		// this should never happen
		return zero, fmt.Errorf("slot is empty or has wrong type")
	}

	// Detect jumps in the sequence, meaning this is a slow reader
	if item.seq > r.lastSeq+1 {
		missedItems := item.seq - (r.lastSeq + 1)
		return zero, fmt.Errorf("%w; items missed: %d", ErrSlowReader, missedItems)
	}

	// Advance reader for next run
	r.lastSeq = item.seq
	r.cursor = r.cursor.Next()

	return item.value, nil
}

// Rewind moves the reader's cursor backward in the buffer to the first element
// that satisfies the predicate. Returns true if a matching element was found.
func (r *Reader[T]) Rewind(predicate func(v T) bool) bool {
	b := r.buffer
	b.cond.L.Lock()
	defer b.cond.L.Unlock()

	searchCursor := r.cursor

	// Iterate backwards up to the buffer size.
	for i := 0; i < b.size; i++ {
		searchCursor = searchCursor.Prev()

		// Stop if we've checked all available data and reached the writer's head.
		if searchCursor == b.head {
			return false
		}

		// Extract the item and check the predicate using its value.
		if item, ok := searchCursor.Value.(ringItem[T]); ok {
			if predicate(item.value) {
				r.cursor = searchCursor
				r.lastSeq = item.seq - 1
				return true
			}
		}
	}

	return false
}
