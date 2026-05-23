package tunnelproxy

import (
	"sync"

	"github.com/hashicorp/yamux"
)

type Hub struct {
	mu       sync.RWMutex
	sessions map[string]*yamux.Session
}

func NewHub() *Hub {
	return &Hub{sessions: make(map[string]*yamux.Session)}
}

func (h *Hub) Register(machineID string, session *yamux.Session) func() {
	h.mu.Lock()
	if old, ok := h.sessions[machineID]; ok && old != session {
		_ = old.Close()
	}
	h.sessions[machineID] = session
	h.mu.Unlock()

	return func() {
		h.mu.Lock()
		if cur, ok := h.sessions[machineID]; ok && cur == session {
			delete(h.sessions, machineID)
		}
		h.mu.Unlock()
	}
}

func (h *Hub) Get(machineID string) (*yamux.Session, bool) {
	h.mu.RLock()
	s, ok := h.sessions[machineID]
	h.mu.RUnlock()
	if !ok || s.IsClosed() {
		return nil, false
	}
	return s, true
}

func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.sessions)
}
