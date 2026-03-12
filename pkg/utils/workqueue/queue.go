// Package workqueue provides a rate-limited work queue modeled after
// k8s.io/client-go/util/workqueue. It uses Go generics for type safety.
//
// The core pattern is the dirty/processing dual-set dedup mechanism from
// Kubernetes, which ensures that:
//   - Each item is processed at most once at a time.
//   - If an item is re-added while being processed, it will be re-queued
//     after the current processing completes (via Done).
//   - Duplicate Adds are coalesced (dirty set dedup).
//
// Usage pattern (K8s controller style):
//
//	q := workqueue.New[string]()
//	defer q.ShutDown()
//
//	// Producer:
//	q.Add("my-key")
//
//	// Consumer (worker loop):
//	for {
//	    item, shutdown := q.Get()
//	    if shutdown { break }
//	    // process item...
//	    q.Done(item)
//	}
package workqueue

import (
	"sync"
)

// Interface defines the contract for a work queue.
// Modeled after k8s.io/client-go/util/workqueue.TypedInterface[T].
type Interface[T comparable] interface {
	// Add marks an item as needing processing. If the item is not already
	// in the dirty set, it is added to the queue. If the item is currently
	// being processed, it is added to the dirty set so it will be re-queued
	// when Done is called.
	Add(item T)

	// Len returns the current number of items in the queue.
	Len() int

	// Get blocks until an item is available, then returns it along with a
	// shutdown indicator. If shutdown is true, the caller should exit.
	// The caller MUST call Done with the item when processing is complete.
	Get() (item T, shutdown bool)

	// Done marks an item as finished processing. If the item was re-added
	// (dirty) while being processed, it is re-enqueued.
	// This MUST be called for every item returned by Get.
	Done(item T)

	// ShutDown signals the queue to shut down. All blocked Get calls will
	// return with shutdown=true. Further Add calls are ignored.
	ShutDown()

	// ShutDownWithDrain signals shutdown but continues to process existing
	// items until the queue is empty. New items added after this call are
	// accepted only if the queue hasn't fully drained yet.
	ShutDownWithDrain()

	// ShuttingDown returns true if ShutDown or ShutDownWithDrain has been called.
	ShuttingDown() bool
}

// typed implements Interface[T] with the dirty/processing dual-set pattern
// from k8s.io/client-go/util/workqueue/queue.go.
type typed[T comparable] struct {
	// queue holds the ordered list of items to be processed.
	// Items in this list are guaranteed to be in the dirty set.
	queue []T

	// dirty tracks all items that need to be processed.
	// An item can be in dirty but not in queue if it was re-added
	// while being processed (it will be re-queued in Done).
	dirty set[T]

	// processing tracks items currently being processed by consumers.
	// An item is in processing from Get() until Done().
	processing set[T]

	cond *sync.Cond

	shuttingDown bool
	drain        bool
}

// set is a simple set implementation backed by a map.
type set[T comparable] map[T]struct{}

func (s set[T]) has(item T) bool {
	_, exists := s[item]
	return exists
}

func (s set[T]) insert(item T) {
	s[item] = struct{}{}
}

func (s set[T]) delete(item T) {
	delete(s, item)
}

func (s set[T]) len() int {
	return len(s)
}

// New creates a new work queue.
// This is the equivalent of k8s.io/client-go/util/workqueue.NewTyped[T]().
func New[T comparable]() Interface[T] {
	return newQueue[T]()
}

func newQueue[T comparable]() *typed[T] {
	t := &typed[T]{
		dirty:      set[T]{},
		processing: set[T]{},
	}
	t.cond = sync.NewCond(&sync.Mutex{})
	return t
}

// Add implements Interface[T].Add.
func (q *typed[T]) Add(item T) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	if q.shuttingDown && !q.drain {
		return
	}

	// If already dirty (either in queue or re-added during processing), skip.
	if q.dirty.has(item) {
		return
	}

	q.dirty.insert(item)

	// If the item is currently being processed, don't enqueue it yet.
	// It will be re-enqueued when Done() is called.
	if q.processing.has(item) {
		return
	}

	q.queue = append(q.queue, item)
	q.cond.Signal()
}

// Len implements Interface[T].Len.
func (q *typed[T]) Len() int {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	return len(q.queue)
}

// Get implements Interface[T].Get.
func (q *typed[T]) Get() (item T, shutdown bool) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	for len(q.queue) == 0 && !q.shuttingDown {
		q.cond.Wait()
	}

	if len(q.queue) == 0 {
		// Shutting down and queue is empty.
		var zero T
		return zero, true
	}

	item = q.queue[0]
	// Use copy to avoid memory leak from holding references in the
	// underlying array (same pattern as k8s queue.go).
	q.queue[0] = *new(T) // zero value
	q.queue = q.queue[1:]

	q.processing.insert(item)
	q.dirty.delete(item)

	return item, false
}

// Done implements Interface[T].Done.
func (q *typed[T]) Done(item T) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.processing.delete(item)

	// If the item was re-added while being processed, re-enqueue it.
	if q.dirty.has(item) {
		q.queue = append(q.queue, item)
		q.cond.Signal()
	} else if q.drain && q.processing.len() == 0 && len(q.queue) == 0 {
		// All processing complete during drain — signal shutdown.
		q.shuttingDown = true
		q.cond.Broadcast()
	}
}

// ShutDown implements Interface[T].ShutDown.
func (q *typed[T]) ShutDown() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.drain = false
	q.shuttingDown = true
	q.cond.Broadcast()
}

// ShutDownWithDrain implements Interface[T].ShutDownWithDrain.
func (q *typed[T]) ShutDownWithDrain() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.drain = true

	// If nothing is pending, immediately shut down.
	if q.processing.len() == 0 && len(q.queue) == 0 {
		q.shuttingDown = true
	}

	q.cond.Broadcast()
}

// ShuttingDown implements Interface[T].ShuttingDown.
func (q *typed[T]) ShuttingDown() bool {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	return q.shuttingDown
}
