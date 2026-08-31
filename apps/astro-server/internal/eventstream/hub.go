// Package eventstream fans account events out to one replica's SSE connections.
// Producers run in the River worker deployment, so delivery crosses LISTEN/NOTIFY.
package eventstream

import (
	"encoding/json"
	"sync"
	"sync/atomic"
)

// Event carries no build payload: clients refetch what they already own, so no
// second copy of build state can go stale.
type Event struct {
	// The agent_events row id, sent as SSE `id:` so a client can replay from it.
	ID string `json:"id"`
	// Serialized because the same encoding crosses NOTIFY, where routing reads it.
	AccountID string `json:"account_id"`
	Type      string `json:"type"`
	Agent     string `json:"agent"`
	BuildID   string `json:"build_id,omitempty"`
	Status    string `json:"status,omitempty"`
}

// Events are nudges, so a subscriber that cannot keep up gains nothing deeper.
const buffer = 16

type Hub struct {
	mu      sync.RWMutex
	subs    map[string]map[*subscriber]struct{}
	dropped atomic.Uint64
}

type subscriber struct {
	ch chan Event
}

func New() *Hub {
	return &Hub{subs: make(map[string]map[*subscriber]struct{})}
}

// Cancel must run once. It unregisters before closing, so a concurrent Publish
// cannot write to a closed channel.
func (h *Hub) Subscribe(accountID string) (<-chan Event, func()) {
	s := &subscriber{ch: make(chan Event, buffer)}

	h.mu.Lock()
	if h.subs[accountID] == nil {
		h.subs[accountID] = make(map[*subscriber]struct{})
	}
	h.subs[accountID][s] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	return s.ch, func() {
		once.Do(func() {
			h.mu.Lock()
			if set := h.subs[accountID]; set != nil {
				delete(set, s)
				if len(set) == 0 {
					delete(h.subs, accountID)
				}
			}
			h.mu.Unlock()
			close(s.ch)
		})
	}
}

// Never blocks: a full subscriber misses the event and recovers on its next
// refetch, which beats stalling delivery for everyone else.
func (h *Hub) Publish(e Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for s := range h.subs[e.AccountID] {
		select {
		case s.ch <- e:
		default:
			h.dropped.Add(1)
		}
	}
}

// Dropped counts events discarded because a subscriber's buffer was full.
func (h *Hub) Dropped() uint64 { return h.dropped.Load() }

// Subscribers reports the connection count for one account on this replica.
func (h *Hub) Subscribers(accountID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs[accountID])
}

// Encode renders an event as the SSE `data:` payload.
func (e Event) Encode() ([]byte, error) { return json.Marshal(e) }
