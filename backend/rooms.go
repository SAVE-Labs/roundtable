package main

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"

	"roundtable/backend/db"
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
	return p.ws.WriteJSON(sdp)
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
	mu      sync.RWMutex
	rooms   map[string]*Room
	queries *db.Queries
}

func NewRoomRegistry(queries *db.Queries) *RoomRegistry {
	return &RoomRegistry{
		rooms:   make(map[string]*Room),
		queries: queries,
	}
}

func (rr *RoomRegistry) Create(ctx context.Context, name string) (*Room, error) {
	dbRoom, err := rr.queries.CreateRoom(ctx, db.CreateRoomParams{
		ID:   uuid.New().String(),
		Name: name,
	})
	if err != nil {
		return nil, err
	}
	room := &Room{
		id:          dbRoom.ID,
		name:        dbRoom.Name,
		peers:       make(map[string]*Peer),
		localTracks: make(map[string]*webrtc.TrackLocalStaticRTP),
	}
	rr.mu.Lock()
	rr.rooms[room.id] = room
	rr.mu.Unlock()
	return room, nil
}

// Get returns the in-memory Room for an active session. If the room exists in the
// database but has no active connections yet (e.g. after a server restart), it is
// rehydrated into memory on demand.
func (rr *RoomRegistry) Get(ctx context.Context, id string) (*Room, bool) {
	rr.mu.Lock()
	defer rr.mu.Unlock()

	if r, ok := rr.rooms[id]; ok {
		return r, true
	}

	dbRoom, err := rr.queries.GetRoom(ctx, id)
	if err != nil {
		return nil, false
	}

	room := &Room{
		id:          dbRoom.ID,
		name:        dbRoom.Name,
		peers:       make(map[string]*Peer),
		localTracks: make(map[string]*webrtc.TrackLocalStaticRTP),
	}
	rr.rooms[room.id] = room
	return room, true
}

func (rr *RoomRegistry) Delete(ctx context.Context, id string) error {
	if err := rr.queries.DeleteRoom(ctx, id); err != nil {
		return err
	}
	rr.mu.Lock()
	delete(rr.rooms, id)
	rr.mu.Unlock()
	return nil
}

func (rr *RoomRegistry) List(ctx context.Context) ([]map[string]string, error) {
	dbRooms, err := rr.queries.ListRooms(ctx)
	if err != nil {
		return nil, err
	}
	list := make([]map[string]string, 0, len(dbRooms))
	for _, r := range dbRooms {
		list = append(list, map[string]string{"id": r.ID, "name": r.Name})
	}
	return list, nil
}
