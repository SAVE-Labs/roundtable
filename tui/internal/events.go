package internal

import (
	"encoding/json"
	"log"
	"net/url"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/net/websocket"
)

// RoomEventMsg is dispatched to the bubbletea runtime whenever a room's
// membership changes.
type RoomEventMsg struct {
	RoomID  string
	Members []string
}

// EventsConnectedMsg is returned by ConnectEventsCmd on success.
type EventsConnectedMsg struct {
	Client *EventsClient
}

// eventsDisconnectedMsg signals that the events WebSocket has closed.
type eventsDisconnectedMsg struct{}

// EventsClient maintains a persistent WebSocket connection to /events and
// forwards incoming room-membership events into a channel that bubbletea
// can consume one-at-a-time via WaitForEvent.
type EventsClient struct {
	ws    *websocket.Conn
	msgCh chan RoomEventMsg
}

func (e *EventsClient) Close() {
	if e == nil {
		return
	}
	if e.ws != nil {
		e.ws.Close()
	}
}

// WaitForEvent returns a Cmd that blocks until the next event arrives on the
// channel, then delivers it as a tea.Msg.  Re-register this after every
// RoomEventMsg to keep the stream flowing.
func (e *EventsClient) WaitForEvent() tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-e.msgCh
		if !ok {
			return eventsDisconnectedMsg{}
		}
		return msg
	}
}

// ConnectEventsCmd dials the server's /events WebSocket endpoint and starts
// a background goroutine that feeds decoded events into the client's channel.
// It returns EventsConnectedMsg on success or eventsDisconnectedMsg on failure.
func ConnectEventsCmd(serverURL string) tea.Cmd {
	return func() tea.Msg {
		eventsWS, origin := deriveEventsURL(serverURL)
		log.Printf("events: connecting url=%s origin=%s", eventsWS, origin)

		ws, err := websocket.Dial(eventsWS, "", origin)
		if err != nil {
			log.Printf("events: connect failed url=%s err=%v", eventsWS, err)
			return eventsDisconnectedMsg{}
		}

		ch := make(chan RoomEventMsg, 64)
		client := &EventsClient{ws: ws, msgCh: ch}

		go func() {
			defer close(ch)
			for {
				var raw []byte
				if err := websocket.Message.Receive(ws, &raw); err != nil {
					log.Printf("events: receive failed err=%v", err)
					return
				}
				var event struct {
					Type    string `json:"type"`
					RoomID  string `json:"room_id"`
					Members []struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"members"`
				}
				if err := json.Unmarshal(raw, &event); err != nil {
					log.Printf("events: decode failed err=%v", err)
					continue
				}
				if event.Type != "members" {
					continue
				}
				names := make([]string, 0, len(event.Members))
				for _, m := range event.Members {
					names = append(names, m.Name)
				}
				select {
				case ch <- RoomEventMsg{RoomID: event.RoomID, Members: names}:
				default:
					log.Printf("events: dropped event for room=%s (channel full)", event.RoomID)
				}
			}
		}()

		log.Printf("events: connected url=%s", eventsWS)
		return EventsConnectedMsg{Client: client}
	}
}

// deriveEventsURL converts the HTTP server URL (e.g. "http://host:1323") to the
// WebSocket events URL ("ws://host:1323/events") and a matching origin header.
func deriveEventsURL(httpURL string) (eventsWS, origin string) {
	trimmed := strings.TrimRight(strings.TrimSpace(httpURL), "/")
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return "ws://" + trimmed + "/events", "http://" + trimmed
	}
	wsScheme := "ws"
	if parsed.Scheme == "https" {
		wsScheme = "wss"
	}
	return wsScheme + "://" + parsed.Host + "/events", parsed.Scheme + "://" + parsed.Host
}
