package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
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
		log.Printf("probe server info mode=%d index=%d url=%s", mode, index, trimmedServer)
		if trimmedServer == "" {
			log.Printf("probe server info failed: empty server url")
			return ServerInfoMsg{Mode: mode, Index: index, Server: server, Err: fmt.Errorf("server url is empty")}
		}

		resp, err := http.Get(strings.TrimRight(trimmedServer, "/") + "/info")
		if err != nil {
			log.Printf("probe server info failed url=%s err=%v", trimmedServer, err)
			return ServerInfoMsg{Mode: mode, Index: index, Server: server, Err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("probe server info failed url=%s status=%s", trimmedServer, resp.Status)
			return ServerInfoMsg{Mode: mode, Index: index, Server: server, Err: fmt.Errorf("info failed: %s", resp.Status)}
		}

		var payload struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return ServerInfoMsg{Mode: mode, Index: index, Server: server, Err: err}
		}

		if strings.TrimSpace(server.Name) == "" && strings.TrimSpace(payload.Name) != "" {
			server.Name = strings.TrimSpace(payload.Name)
		}
		log.Printf("probe server info ok mode=%d index=%d url=%s name=%s version=%s", mode, index, trimmedServer, server.Name, payload.Version)

		return ServerInfoMsg{Mode: mode, Index: index, Server: server, Version: payload.Version}
	}
}

func LoadRoomsCmd(serverURL string) tea.Cmd {
	return func() tea.Msg {
		log.Printf("load rooms server=%s", strings.TrimSpace(serverURL))
		if strings.TrimSpace(serverURL) == "" {
			log.Printf("load rooms failed: empty server url")
			return RoomsMsg{Err: fmt.Errorf("server url is empty")}
		}

		resp, err := http.Get(strings.TrimRight(serverURL, "/") + "/rooms")
		if err != nil {
			log.Printf("load rooms failed server=%s err=%v", serverURL, err)
			return RoomsMsg{Err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("load rooms failed server=%s status=%s", serverURL, resp.Status)
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
		log.Printf("load rooms ok server=%s count=%d", serverURL, len(channels))

		return RoomsMsg{Channels: channels}
	}
}

func CreateRoomCmd(serverURL, name string) tea.Cmd {
	return func() tea.Msg {
		trimmedServer := strings.TrimSpace(serverURL)
		trimmedName := strings.TrimSpace(name)
		log.Printf("create room server=%s name=%s", trimmedServer, trimmedName)
		if trimmedServer == "" {
			log.Printf("create room failed: empty server url")
			return RoomCreatedMsg{Err: fmt.Errorf("server url is empty")}
		}
		if trimmedName == "" {
			log.Printf("create room failed: empty room name")
			return RoomCreatedMsg{Err: fmt.Errorf("room name is required")}
		}

		body, err := json.Marshal(map[string]string{"name": trimmedName})
		if err != nil {
			return RoomCreatedMsg{Err: err}
		}

		resp, err := http.Post(strings.TrimRight(trimmedServer, "/")+"/rooms", "application/json", bytes.NewReader(body))
		if err != nil {
			log.Printf("create room failed server=%s name=%s err=%v", trimmedServer, trimmedName, err)
			return RoomCreatedMsg{Err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			log.Printf("create room failed server=%s name=%s status=%s", trimmedServer, trimmedName, resp.Status)
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
			log.Printf("create room failed server=%s name=%s backend returned empty id", trimmedServer, trimmedName)
			return RoomCreatedMsg{Err: fmt.Errorf("backend returned room without id")}
		}
		log.Printf("create room ok server=%s id=%s name=%s", trimmedServer, room.ID, room.Name)

		return RoomCreatedMsg{Channel: Channel{ID: room.ID, Name: room.Name}}
	}
}

func DeleteRoomCmd(serverURL, roomID string) tea.Cmd {
	return func() tea.Msg {
		trimmedServer := strings.TrimSpace(serverURL)
		trimmedRoomID := strings.TrimSpace(roomID)
		log.Printf("delete room server=%s id=%s", trimmedServer, trimmedRoomID)
		if trimmedServer == "" {
			log.Printf("delete room failed: empty server url")
			return RoomDeletedMsg{Err: fmt.Errorf("server url is empty")}
		}
		if trimmedRoomID == "" {
			log.Printf("delete room failed: empty room id")
			return RoomDeletedMsg{Err: fmt.Errorf("room id is required")}
		}

		req, err := http.NewRequest(http.MethodDelete, strings.TrimRight(trimmedServer, "/")+"/rooms/"+trimmedRoomID, nil)
		if err != nil {
			log.Printf("delete room failed server=%s id=%s err=%v", trimmedServer, trimmedRoomID, err)
			return RoomDeletedMsg{RoomID: trimmedRoomID, Err: err}
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("delete room failed server=%s id=%s err=%v", trimmedServer, trimmedRoomID, err)
			return RoomDeletedMsg{RoomID: trimmedRoomID, Err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			log.Printf("delete room failed server=%s id=%s status=%s", trimmedServer, trimmedRoomID, resp.Status)
			return RoomDeletedMsg{RoomID: trimmedRoomID, Err: fmt.Errorf("delete room failed: %s", resp.Status)}
		}
		log.Printf("delete room ok server=%s id=%s", trimmedServer, trimmedRoomID)

		return RoomDeletedMsg{RoomID: trimmedRoomID}
	}
}
