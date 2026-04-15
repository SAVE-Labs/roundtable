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
	id          string
	displayName string
	pc          *webrtc.PeerConnection
	ws          *websocket.Conn
	wsMu        sync.Mutex
	answerCh    chan webrtc.SessionDescription
}

type PeerInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RoomInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MemberCount int    `json:"member_count"`
}

type RoomEvent struct {
	Type    string     `json:"type"`
	RoomID  string     `json:"room_id"`
	Members []PeerInfo `json:"members"`
}

type EventBus struct {
	mu          sync.RWMutex
	subscribers map[chan RoomEvent]struct{}
}

func newEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[chan RoomEvent]struct{}),
	}
}

func (b *EventBus) subscribe() chan RoomEvent {
	ch := make(chan RoomEvent, 64)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *EventBus) unsubscribe(ch chan RoomEvent) {
	b.mu.Lock()
	delete(b.subscribers, ch)
	b.mu.Unlock()
	close(ch)
}

func (b *EventBus) publish(event RoomEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subscribers {
		select {
		case ch <- event:
		default: // slow subscriber; drop rather than block
		}
	}
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
	bus         *EventBus
}

func (r *Room) AddPeer(p *Peer) {
	r.mu.Lock()
	r.peers[p.id] = p
	members := r.membersLocked()
	r.mu.Unlock()
	if r.bus != nil {
		r.bus.publish(RoomEvent{Type: "members", RoomID: r.id, Members: members})
	}
}

func (r *Room) RemovePeer(id string) {
	r.mu.Lock()
	delete(r.peers, id)
	delete(r.localTracks, id)
	members := r.membersLocked()
	r.mu.Unlock()
	if r.bus != nil {
		r.bus.publish(RoomEvent{Type: "members", RoomID: r.id, Members: members})
	}
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

func (r *Room) MemberCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.peers)
}

func (r *Room) membersLocked() []PeerInfo {
	members := make([]PeerInfo, 0, len(r.peers))
	for _, p := range r.peers {
		name := p.displayName
		if name == "" {
			if len(p.id) >= 8 {
				name = p.id[:8]
			} else {
				name = p.id
			}
		}
		members = append(members, PeerInfo{ID: p.id, Name: name})
	}
	return members
}

func (r *Room) Members() []PeerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.membersLocked()
}

type RoomRegistry struct {
	mu      sync.RWMutex
	rooms   map[string]*Room
	queries *db.Queries
	bus     *EventBus
}

func NewRoomRegistry(queries *db.Queries) *RoomRegistry {
	return &RoomRegistry{
		rooms:   make(map[string]*Room),
		queries: queries,
		bus:     newEventBus(),
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
		bus:         rr.bus,
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
		bus:         rr.bus,
	}
	rr.rooms[room.id] = room
	return room, true
}

func (rr *RoomRegistry) forEachRoom(fn func(*Room)) {
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	for _, room := range rr.rooms {
		fn(room)
	}
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

func (rr *RoomRegistry) List(ctx context.Context) ([]RoomInfo, error) {
	dbRooms, err := rr.queries.ListRooms(ctx)
	if err != nil {
		return nil, err
	}
	list := make([]RoomInfo, 0, len(dbRooms))
	for _, r := range dbRooms {
		count := 0
		rr.mu.RLock()
		if room, ok := rr.rooms[r.ID]; ok {
			count = room.MemberCount()
		}
		rr.mu.RUnlock()
		list = append(list, RoomInfo{ID: r.ID, Name: r.Name, MemberCount: count})
	}
	return list, nil
}
