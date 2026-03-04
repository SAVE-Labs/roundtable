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

type RoomDeletedMsg struct {
	RoomID string
	Err    error
}

type ServerProbeMode int

const (
	ServerProbeSelect ServerProbeMode = iota
	ServerProbeAdd
)

type ServerInfoMsg struct {
	Mode    ServerProbeMode
	Index   int
	Server  ServerOption
	Version string
	Err     error
}

func ProbeServerInfoCmd(mode ServerProbeMode, index int, server ServerOption) tea.Cmd {
	return func() tea.Msg {
		trimmedServer := strings.TrimSpace(server.HTTPURL)
		if trimmedServer == "" {
			return ServerInfoMsg{Mode: mode, Index: index, Server: server, Err: fmt.Errorf("server url is empty")}
		}

		resp, err := http.Get(strings.TrimRight(trimmedServer, "/") + "/info")
		if err != nil {
			return ServerInfoMsg{Mode: mode, Index: index, Server: server, Err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return ServerInfoMsg{Mode: mode, Index: index, Server: server, Err: fmt.Errorf("info failed: %s", resp.Status)}
		}

		var payload struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return ServerInfoMsg{Mode: mode, Index: index, Server: server, Err: err}
		}

		if strings.TrimSpace(payload.Name) != "" {
			server.Name = strings.TrimSpace(payload.Name)
		}

		return ServerInfoMsg{Mode: mode, Index: index, Server: server, Version: payload.Version}
	}
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

func DeleteRoomCmd(serverURL, roomID string) tea.Cmd {
	return func() tea.Msg {
		trimmedServer := strings.TrimSpace(serverURL)
		trimmedRoomID := strings.TrimSpace(roomID)
		if trimmedServer == "" {
			return RoomDeletedMsg{Err: fmt.Errorf("server url is empty")}
		}
		if trimmedRoomID == "" {
			return RoomDeletedMsg{Err: fmt.Errorf("room id is required")}
		}

		req, err := http.NewRequest(http.MethodDelete, strings.TrimRight(trimmedServer, "/")+"/rooms/"+trimmedRoomID, nil)
		if err != nil {
			return RoomDeletedMsg{RoomID: trimmedRoomID, Err: err}
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return RoomDeletedMsg{RoomID: trimmedRoomID, Err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			return RoomDeletedMsg{RoomID: trimmedRoomID, Err: fmt.Errorf("delete room failed: %s", resp.Status)}
		}

		return RoomDeletedMsg{RoomID: trimmedRoomID}
	}
}
