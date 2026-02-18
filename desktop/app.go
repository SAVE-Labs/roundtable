package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gen2brain/malgo"
)

type AudioDeviceInfo struct {
	Name string `json:"name"`
}

type AudioDevices struct {
	Capture  []AudioDeviceInfo `json:"capture"`
	Playback []AudioDeviceInfo `json:"playback"`
}

type RoomInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type App struct {
	ctx             context.Context
	engine          *AudioEngine
	client          *WebRTCClient
	status          string
	captureDevices  []malgo.DeviceInfo
	playbackDevices []malgo.DeviceInfo
}

func NewApp() *App {
	return &App{status: "Not connected"}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) ListAudioDevices() (AudioDevices, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return AudioDevices{}, fmt.Errorf("init audio context: %w", err)
	}
	defer ctx.Uninit()

	capture, err := ctx.Devices(malgo.Capture)
	if err != nil {
		return AudioDevices{}, fmt.Errorf("list capture devices: %w", err)
	}
	playback, err := ctx.Devices(malgo.Playback)
	if err != nil {
		return AudioDevices{}, fmt.Errorf("list playback devices: %w", err)
	}

	a.captureDevices = capture
	a.playbackDevices = playback

	result := AudioDevices{}
	for _, d := range capture {
		result.Capture = append(result.Capture, AudioDeviceInfo{Name: d.Name()})
	}
	for _, d := range playback {
		result.Playback = append(result.Playback, AudioDeviceInfo{Name: d.Name()})
	}
	return result, nil
}

func (a *App) ListRooms(serverURL string) ([]RoomInfo, error) {
	resp, err := http.Get(strings.TrimRight(serverURL, "/") + "/rooms")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var rooms []RoomInfo
	if err := json.NewDecoder(resp.Body).Decode(&rooms); err != nil {
		return nil, err
	}
	return rooms, nil
}

func (a *App) CreateRoom(serverURL, name string) (RoomInfo, error) {
	body, _ := json.Marshal(map[string]string{"name": name})
	resp, err := http.Post(
		strings.TrimRight(serverURL, "/")+"/rooms",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return RoomInfo{}, err
	}
	defer resp.Body.Close()
	var room RoomInfo
	if err := json.NewDecoder(resp.Body).Decode(&room); err != nil {
		return RoomInfo{}, err
	}
	return room, nil
}

// JoinRoom connects to the given WebSocket URL using the selected audio devices.
// captureIndex and playbackIndex refer to the positions returned by ListAudioDevices.
func (a *App) JoinRoom(wsURL string, captureIndex, playbackIndex int) error {
	if captureIndex < 0 || captureIndex >= len(a.captureDevices) {
		return fmt.Errorf("invalid capture device index %d (call ListAudioDevices first)", captureIndex)
	}
	if playbackIndex < 0 || playbackIndex >= len(a.playbackDevices) {
		return fmt.Errorf("invalid playback device index %d (call ListAudioDevices first)", playbackIndex)
	}

	capDev := a.captureDevices[captureIndex]
	pbDev := a.playbackDevices[playbackIndex]

	a.LeaveRoom()

	engine := NewAudioEngine()
	client, err := NewWebRTCClient(wsURL, func(pcm []byte) {
		engine.PushPCM16LE(pcm)
	})
	if err != nil {
		return err
	}

	if err := engine.Start(capDev, pbDev, func(pcm []byte) {
		_ = client.SendPCM16LE(pcm)
	}); err != nil {
		client.Close()
		return err
	}

	a.engine = engine
	a.client = client
	a.status = "Connected"
	return nil
}

func (a *App) LeaveRoom() {
	if a.client != nil {
		a.client.Close()
		a.client = nil
	}
	if a.engine != nil {
		a.engine.Close()
		a.engine = nil
	}
	a.status = "Not connected"
}

func (a *App) SessionStatus() string {
	return a.status
}
