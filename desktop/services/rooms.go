package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Room struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MemberCount int    `json:"member_count"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type RoomsService struct{}

func NewRoomsService() *RoomsService {
	return &RoomsService{}
}

func (s *RoomsService) ListRooms(serverURL string) ([]Room, error) {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if serverURL == "" {
		return nil, fmt.Errorf("server URL is required")
	}

	resp, err := http.Get(serverURL + "/rooms")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list rooms: %s", resp.Status)
	}

	var payload []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		MemberCount int    `json:"member_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	rooms := make([]Room, 0, len(payload))
	for _, r := range payload {
		if r.ID != "" {
			rooms = append(rooms, Room{ID: r.ID, Name: r.Name, MemberCount: r.MemberCount})
		}
	}
	return rooms, nil
}

func (s *RoomsService) CreateRoom(serverURL, name string) (Room, error) {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	name = strings.TrimSpace(name)
	if serverURL == "" {
		return Room{}, fmt.Errorf("server URL is required")
	}
	if name == "" {
		return Room{}, fmt.Errorf("room name is required")
	}

	body, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return Room{}, err
	}

	resp, err := http.Post(serverURL+"/rooms", "application/json", bytes.NewReader(body))
	if err != nil {
		return Room{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return Room{}, fmt.Errorf("create room: %s", resp.Status)
	}

	var room struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&room); err != nil {
		return Room{}, err
	}
	if room.ID == "" {
		return Room{}, fmt.Errorf("backend returned room without id")
	}
	return Room{ID: room.ID, Name: room.Name}, nil
}

func (s *RoomsService) DeleteRoom(serverURL, roomID string) error {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	roomID = strings.TrimSpace(roomID)
	if serverURL == "" {
		return fmt.Errorf("server URL is required")
	}
	if roomID == "" {
		return fmt.Errorf("room ID is required")
	}

	req, err := http.NewRequest(http.MethodDelete, serverURL+"/rooms/"+roomID, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete room: %s", resp.Status)
	}
	return nil
}

func (s *RoomsService) ProbeServer(serverURL string) (ServerInfo, error) {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if serverURL == "" {
		return ServerInfo{}, fmt.Errorf("server URL is required")
	}

	resp, err := http.Get(serverURL + "/info")
	if err != nil {
		return ServerInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ServerInfo{}, fmt.Errorf("probe server: %s", resp.Status)
	}

	var info ServerInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return ServerInfo{}, err
	}
	return info, nil
}
