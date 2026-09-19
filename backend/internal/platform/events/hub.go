// Package events is a tiny in-process publish/subscribe hub used to stream
// optimization progress to SSE clients.
//
// Publishing never blocks: a subscriber that cannot keep up drops events rather
// than stalling a solver. Subscribers are never closed by the hub (that would
// race with a publisher), they simply stop being sent to when they unsubscribe.
package events

import "sync"

// Event is one message on a job's stream. Type is one of: running, progress,
// done, failed, cancelled, snapshot.
type Event struct {
	Type  string `json:"type"`
	JobID string `json:"jobId,omitempty"`
	Seq   int64  `json:"seq,omitempty"`
	Data  any    `json:"data,omitempty"`
}

// subscriberBuffer is how many events a slow subscriber may fall behind before
// events are dropped.
const subscriberBuffer = 32

type Hub struct {
	mu     sync.Mutex
	nextID int
	subs   map[string]map[int]chan Event
}

func NewHub() *Hub {
	return &Hub{subs: map[string]map[int]chan Event{}}
}

// Subscribe returns a channel of events for one job and an unsubscribe
// function. The channel is never closed by the hub.
func (h *Hub) Subscribe(jobID string) (<-chan Event, func()) {
	h.mu.Lock()
	id := h.nextID
	h.nextID++
	ch := make(chan Event, subscriberBuffer)
	if h.subs[jobID] == nil {
		h.subs[jobID] = make(map[int]chan Event)
	}
	h.subs[jobID][id] = ch
	h.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs[jobID], id)
			if len(h.subs[jobID]) == 0 {
				delete(h.subs, jobID)
			}
			h.mu.Unlock()
		})
	}
	return ch, unsubscribe
}

// Publish delivers an event to every subscriber of a job.
func (h *Hub) Publish(jobID string, event Event) {
	h.mu.Lock()
	channels := make([]chan Event, 0, len(h.subs[jobID]))
	for _, ch := range h.subs[jobID] {
		channels = append(channels, ch)
	}
	h.mu.Unlock()

	for _, ch := range channels {
		select {
		case ch <- event:
		default: // subscriber too slow: drop rather than block the solver
		}
	}
}

// SubscriberCount reports how many subscribers a job currently has, for tests
// and metrics.
func (h *Hub) SubscriberCount(jobID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[jobID])
}
