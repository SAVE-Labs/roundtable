package main

import (
	"sync"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
	"golang.org/x/net/websocket"
)

type Peer struct {
	id       string
	pc       *webrtc.PeerConnection
	ws       *websocket.Conn
	wsMu     sync.Mutex
	answerCh chan webrtc.SessionDescription
}

func (p *Peer) SendSDP(sdp webrtc.SessionDescription) error {
	p.wsMu.Lock()
	defer p.wsMu.Unlock()
	return websocket.JSON.Send(p.ws, sdp)
}

type Room struct {
	id          string
	name        string
	mu          sync.RWMutex
	peers       map[string]*Peer
	localTracks map[string]*webrtc.TrackLocalStaticRTP
}

func (r *Room) AddPeer(p *Peer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.peers[p.id] = p
}

func (r *Room) RemovePeer(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.peers, id)
	delete(r.localTracks, id)
}

func (r *Room) GetPeers() map[string]*Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	snapshot := make(map[string]*Peer, len(r.peers))
	for k, v := range r.peers {
		snapshot[k] = v
	}
	return snapshot
}

type RoomRegistry struct {
	mu    sync.RWMutex
	rooms map[string]*Room
}

func NewRoomRegistry() *RoomRegistry {
	return &RoomRegistry{rooms: make(map[string]*Room)}
}

func (rr *RoomRegistry) Create(name string) *Room {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	room := &Room{
		id:          uuid.New().String(),
		name:        name,
		peers:       make(map[string]*Peer),
		localTracks: make(map[string]*webrtc.TrackLocalStaticRTP),
	}
	rr.rooms[room.id] = room
	return room
}

func (rr *RoomRegistry) Get(id string) (*Room, bool) {
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	r, ok := rr.rooms[id]
	return r, ok
}
