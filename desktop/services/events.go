package services

import (
	"context"
	"encoding/json"
	"log"
	"net/url"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/net/websocket"
)

// EventsService subscribes to the backend /events WebSocket and re-emits
// room membership updates as Wails events. The frontend can listen with:
//
//	window.runtime.EventsOn("rooms:members", handler)
type EventsService struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

func NewEventsService() *EventsService {
	return &EventsService{}
}

// Subscribe connects to serverURL/events and starts forwarding membership
// events to the frontend. Calling Subscribe again replaces any active connection.
func (s *EventsService) Subscribe(serverURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}

	wsURL := deriveEventsWSURL(serverURL)
	origin := deriveOrigin(wsURL)

	ws, err := websocket.Dial(wsURL, "", origin)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	go s.readLoop(ctx, ws)
	return nil
}

// Unsubscribe stops the active events connection.
func (s *EventsService) Unsubscribe() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

func (s *EventsService) readLoop(ctx context.Context, ws *websocket.Conn) {
	defer ws.Close()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var raw []byte
		if err := websocket.Message.Receive(ws, &raw); err != nil {
			log.Printf("events: receive error: %v", err)
			return
		}

		var payload struct {
			Type    string   `json:"type"`
			RoomID  string   `json:"room_id"`
			Members []string `json:"members"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			continue
		}
		if payload.Type != "members" {
			continue
		}

		application.Get().Event.Emit("rooms:members", map[string]any{
			"room_id": payload.RoomID,
			"members": payload.Members,
		})
	}
}

func deriveEventsWSURL(serverURL string) string {
	base := strings.TrimRight(strings.TrimSpace(serverURL), "/")
	wsBase := strings.Replace(base, "https://", "wss://", 1)
	wsBase = strings.Replace(wsBase, "http://", "ws://", 1)
	u, err := url.Parse(wsBase + "/events")
	if err != nil {
		return wsBase + "/events"
	}
	return u.String()
}
