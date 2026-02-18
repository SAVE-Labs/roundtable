package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type RoomsMsg struct {
	Channels []Channel
	Err      error
}

type RoomCreatedMsg struct {
	Channel Channel
	Err     error
}

func LoadRoomsCmd(serverURL string) tea.Cmd {
	return func() tea.Msg {
		if strings.TrimSpace(serverURL) == "" {
			return RoomsMsg{Err: fmt.Errorf("server url is empty")}
		}

		resp, err := http.Get(strings.TrimRight(serverURL, "/") + "/rooms")
		if err != nil {
			return RoomsMsg{Err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return RoomsMsg{Err: fmt.Errorf("list rooms failed: %s", resp.Status)}
		}

		var payload []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return RoomsMsg{Err: err}
		}

		channels := make([]Channel, 0, len(payload))
		for _, room := range payload {
			if room.ID == "" {
				continue
			}
			channels = append(channels, Channel{ID: room.ID, Name: room.Name})
		}

		return RoomsMsg{Channels: channels}
	}
}

func CreateRoomCmd(serverURL, name string) tea.Cmd {
	return func() tea.Msg {
		trimmedServer := strings.TrimSpace(serverURL)
		trimmedName := strings.TrimSpace(name)
		if trimmedServer == "" {
			return RoomCreatedMsg{Err: fmt.Errorf("server url is empty")}
		}
		if trimmedName == "" {
			return RoomCreatedMsg{Err: fmt.Errorf("room name is required")}
		}

		body, err := json.Marshal(map[string]string{"name": trimmedName})
		if err != nil {
			return RoomCreatedMsg{Err: err}
		}

		resp, err := http.Post(strings.TrimRight(trimmedServer, "/")+"/rooms", "application/json", bytes.NewReader(body))
		if err != nil {
			return RoomCreatedMsg{Err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			return RoomCreatedMsg{Err: fmt.Errorf("create room failed: %s", resp.Status)}
		}

		var room struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&room); err != nil {
			return RoomCreatedMsg{Err: err}
		}
		if room.ID == "" {
			return RoomCreatedMsg{Err: fmt.Errorf("backend returned room without id")}
		}

		return RoomCreatedMsg{Channel: Channel{ID: room.ID, Name: room.Name}}
	}
}
