package simulation

import "container/heap"

// eventHeap implements heap.Interface for Event, ordered by virtual time.
type eventHeap []Event

func (h eventHeap) Len() int            { return len(h) }
func (h eventHeap) Less(i, j int) bool  { return h[i].Time < h[j].Time }
func (h eventHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *eventHeap) Push(x interface{}) { *h = append(*h, x.(Event)) }
func (h *eventHeap) Pop() interface{} {
	old := *h
	n := len(old)
	e := old[n-1]
	*h = old[:n-1]
	return e
}

// Scheduler is a virtual-time priority queue of simulation events.
type Scheduler struct {
	h     eventHeap
	Clock float64 // current virtual time in ms
}

// NewScheduler creates an empty scheduler at virtual time 0.
func NewScheduler() *Scheduler {
	s := &Scheduler{}
	heap.Init(&s.h)
	return s
}

// Schedule inserts an event at the given virtual time.
func (s *Scheduler) Schedule(e Event) {
	if e.Time < s.Clock {
		e.Time = s.Clock // no backward time travel
	}
	heap.Push(&s.h, e)
}

// Next pops and returns the earliest event.
// Returns false if the queue is empty.
func (s *Scheduler) Next() (Event, bool) {
	if len(s.h) == 0 {
		return Event{}, false
	}
	e := heap.Pop(&s.h).(Event)
	s.Clock = e.Time
	return e, true
}

// Len returns the number of pending events.
func (s *Scheduler) Len() int { return len(s.h) }
