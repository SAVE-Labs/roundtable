package main

import (
	"sync"

	"github.com/pion/webrtc/v4"
)

type ConnectionRegistry struct {
	mu          sync.RWMutex
	connections map[string]*webrtc.PeerConnection
}

func NewConnectionRegistry() *ConnectionRegistry {
	return &ConnectionRegistry{
		connections: make(map[string]*webrtc.PeerConnection),
	}
}

func (r *ConnectionRegistry) Add(id string, pc *webrtc.PeerConnection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connections[id] = pc
}

func (r *ConnectionRegistry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.connections, id)
}

func (r *ConnectionRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.connections)
}
