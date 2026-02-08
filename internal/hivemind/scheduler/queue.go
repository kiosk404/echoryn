package scheduler

import (
	"container/heap"
	"sync"
)

// --------------------------------------------------------------------------
// Queue interface
// --------------------------------------------------------------------------

// Queue defines the interface for a task queue with priority support.
// Implementations must be goroutine-safe.
type Queue interface {
	// Enqueue adds a scheduling request to the queue.
	Enqueue(req *ScheduleRequest) error

	// Dequeue removes and returns the highest-priority request.
	// Returns nil if the queue is empty.
	Dequeue() *ScheduleRequest

	// Peek returns the highest-priority request without removing it.
	// Returns nil if the queue is empty.
	Peek() *ScheduleRequest

	// Len returns the number of requests in the queue.
	Len() int

	// Remove removes a specific request by task ID.
	Remove(taskID string) bool

	// Drain returns all queued requests and empties the queue.
	Drain() []*ScheduleRequest
}

// --------------------------------------------------------------------------
// PriorityQueue — heap-based implementation
// --------------------------------------------------------------------------

// PriorityQueue is a thread-safe priority queue backed by container/heap.
// Higher-priority tasks are dequeued first; among equal priorities, FIFO order is maintained.
type PriorityQueue struct {
	mu   sync.Mutex
	heap *requestHeap
	seq  int64 // monotonically increasing insertion counter for FIFO tiebreaking
}

// NewPriorityQueue creates a new empty PriorityQueue.
func NewPriorityQueue() *PriorityQueue {
	h := &requestHeap{}
	heap.Init(h)
	return &PriorityQueue{heap: h}
}

// Enqueue adds a request to the queue.
func (q *PriorityQueue) Enqueue(req *ScheduleRequest) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.seq++
	item := &heapItem{
		request:  req,
		priority: taskPriorityToInt(req.Task.Priority),
		seq:      q.seq,
	}
	heap.Push(q.heap, item)
	return nil
}

// Dequeue removes and returns the highest-priority request.
func (q *PriorityQueue) Dequeue() *ScheduleRequest {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.heap.Len() == 0 {
		return nil
	}
	item := heap.Pop(q.heap).(*heapItem)
	return item.request
}

// Peek returns the highest-priority request without removing it.
func (q *PriorityQueue) Peek() *ScheduleRequest {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.heap.Len() == 0 {
		return nil
	}
	return (*q.heap)[0].request
}

// Len returns the number of requests in the queue.
func (q *PriorityQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.heap.Len()
}

// Remove removes a request by task ID.
func (q *PriorityQueue) Remove(taskID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, item := range *q.heap {
		if item.request.Task.ID == taskID {
			heap.Remove(q.heap, i)
			return true
		}
	}
	return false
}

// Drain returns all queued requests in priority order and empties the queue.
func (q *PriorityQueue) Drain() []*ScheduleRequest {
	q.mu.Lock()
	defer q.mu.Unlock()

	result := make([]*ScheduleRequest, 0, q.heap.Len())
	for q.heap.Len() > 0 {
		item := heap.Pop(q.heap).(*heapItem)
		result = append(result, item.request)
	}
	return result
}

// --------------------------------------------------------------------------
// Heap internals
// --------------------------------------------------------------------------

type heapItem struct {
	request  *ScheduleRequest
	priority int   // higher = more urgent
	seq      int64 // lower = inserted earlier (FIFO tiebreaker)
	index    int   // managed by container/heap
}

type requestHeap []*heapItem

func (h requestHeap) Len() int { return len(h) }

// Less returns true if item i should be dequeued before item j.
// Higher priority first; on tie, earlier insertion first (FIFO).
func (h requestHeap) Less(i, j int) bool {
	if h[i].priority != h[j].priority {
		return h[i].priority > h[j].priority
	}
	return h[i].seq < h[j].seq
}

func (h requestHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *requestHeap) Push(x interface{}) {
	item := x.(*heapItem)
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *requestHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	item.index = -1
	*h = old[:n-1]
	return item
}

// taskPriorityToInt converts a protocol.TaskPriority to an integer for heap ordering.
func taskPriorityToInt(p protocol.TaskPriority) int {
	return int(p)
}
