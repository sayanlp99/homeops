// Package realtime fans out change events to connected SSE clients of a household.
package realtime

import (
	"encoding/json"
	"sync"
)

// Event tells clients which data changed so they can refetch it.
// Origin is the client ID that caused the change; that client skips the event.
type Event struct {
	Topic  string `json:"topic"`            // e.g. "tasks", "sprints", "notifications"
	ID     string `json:"id,omitempty"`     // entity ID when relevant
	Origin string `json:"origin,omitempty"` // X-Client-Id of the writer
	UserID string `json:"userId,omitempty"` // restrict delivery to one user
}

type client struct {
	userID string
	ch     chan []byte
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*client]struct{} // household ID -> clients
}

func NewHub() *Hub {
	return &Hub{clients: make(map[string]map[*client]struct{})}
}

// Subscribe registers a listener; call the returned func to unsubscribe.
func (h *Hub) Subscribe(householdID, userID string) (<-chan []byte, func()) {
	c := &client{userID: userID, ch: make(chan []byte, 64)}
	h.mu.Lock()
	if h.clients[householdID] == nil {
		h.clients[householdID] = make(map[*client]struct{})
	}
	h.clients[householdID][c] = struct{}{}
	h.mu.Unlock()
	return c.ch, func() {
		h.mu.Lock()
		delete(h.clients[householdID], c)
		if len(h.clients[householdID]) == 0 {
			delete(h.clients, householdID)
		}
		h.mu.Unlock()
	}
}

func (h *Hub) Publish(householdID string, ev Event) {
	msg, _ := json.Marshal(ev)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients[householdID] {
		if ev.UserID != "" && c.userID != ev.UserID {
			continue
		}
		select {
		case c.ch <- msg:
		default: // slow client; it will resync on reconnect
		}
	}
}
