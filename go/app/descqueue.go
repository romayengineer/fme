package app

import (
	"context"
	"sync"
)

// DefaultDescriptionQueueLimit is how many listing ids may wait for a
// description pull while a search runs.
const DefaultDescriptionQueueLimit = 50

// descriptionQueue is a bounded FIFO of listing ids waiting for a PDP fetch.
// The producer pushes while the search runs; one worker pops. It is not safe
// for multiple pushers, which keeps close simple and lock-free.
type descriptionQueue struct {
	ch        chan string
	done      chan struct{}
	closeOnce sync.Once

	mu     sync.Mutex
	queued map[string]struct{}
}

// newDescriptionQueue returns a queue with the given capacity.
func newDescriptionQueue(limit int) *descriptionQueue {
	if limit <= 0 {
		limit = DefaultDescriptionQueueLimit
	}

	return &descriptionQueue{
		ch:     make(chan string, limit),
		done:   make(chan struct{}),
		queued: make(map[string]struct{}, limit),
	}
}

// push enqueues id, blocking while the queue is full. It reports false when the
// queue is closed or ctx is cancelled. Duplicate ids waiting in the queue are
// ignored.
func (q *descriptionQueue) push(ctx context.Context, id string) bool {
	if id == "" {
		return true
	}

	reserve, open := q.reserve(id)
	if !open {
		return false
	}
	if !reserve {
		return true
	}

	if q.send(ctx, id) {
		return true
	}
	q.forget(id)
	return false
}

// reserve checks that the queue is open and records id as waiting. reserve is
// false when id is already queued; open is false when the queue is closed.
func (q *descriptionQueue) reserve(id string) (reserve, open bool) {
	select {
	case <-q.done:
		return false, false
	default:
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if _, exists := q.queued[id]; exists {
		return false, true
	}
	q.queued[id] = struct{}{}
	return true, true
}

// send waits for a free slot and enqueues id.
func (q *descriptionQueue) send(ctx context.Context, id string) bool {
	select {
	case <-q.done:
		return false
	case <-ctx.Done():
		return false
	case q.ch <- id:
		return true
	}
}

// pop returns the oldest queued id, waiting until one is available. It reports
// false once the queue is closed and drained or ctx is cancelled.
func (q *descriptionQueue) pop(ctx context.Context) (string, bool) {
	if id, ok := q.tryPop(); ok {
		return id, true
	}

	select {
	case id := <-q.ch:
		q.forget(id)
		return id, true
	case <-q.done:
		return q.tryPop()
	case <-ctx.Done():
		return "", false
	}
}

// tryPop removes one buffered id without waiting.
func (q *descriptionQueue) tryPop() (string, bool) {
	select {
	case id := <-q.ch:
		q.forget(id)
		return id, true
	default:
		return "", false
	}
}

// close stops accepting new ids. Buffered ids can still be popped.
func (q *descriptionQueue) close() {
	q.closeOnce.Do(func() { close(q.done) })
}

// len returns how many ids are currently buffered.
func (q *descriptionQueue) len() int { return len(q.ch) }

// forget removes id from the duplicate guard.
func (q *descriptionQueue) forget(id string) {
	q.mu.Lock()
	delete(q.queued, id)
	q.mu.Unlock()
}
