package internal

import (
	"net/url"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gen2brain/malgo"
)

type Channel struct {
	ID   string
	Name string
}

type ServerOption struct {
	Name    string
	HTTPURL string
	WSURL   string
}

const (
	TabChannels = iota
	TabServers
	TabAudio
	TabCount
)

const (
	AudioFocusCapture = iota
	AudioFocusPlayback
)

type Model struct {
	Username     string
	ServerInput  string
	ServerURL    *url.URL
	WebsocketURL *url.URL

	Cursor        int
	Channels      []Channel
	ActiveChannel *Channel

	Servers        []ServerOption
	ServerCursor   int
	ServerSelected int

	Tab int

	AudioFocus            int
	AudioCaptureDevices   []malgo.DeviceInfo
	AudioPlaybackDevices  []malgo.DeviceInfo
	AudioCaptureCursor    int
	AudioPlaybackCursor   int
	AudioCaptureSelected  int
	AudioPlaybackSelected int
	AudioErr              string

	AudioEngine   *AudioEngine
	WebRTCClient  *WebRTCClient
	SessionStatus string
}

func New() Model {
	serverURL, _ := url.Parse(defaultBackendBaseURL)
	wsURL, _ := url.Parse(defaultBackendWS)

	return Model{
		ServerURL:    serverURL,
		WebsocketURL: wsURL,
		Channels: []Channel{
			{ID: "general", Name: "General"},
			{ID: "offtopic", Name: "Off Topic"},
		},
		Servers: []ServerOption{
			{
				Name:    "Localhost",
				HTTPURL: defaultBackendBaseURL,
				WSURL:   defaultBackendWS,
			},
			{
				Name:    "santing.net:8654",
				HTTPURL: "http://santing.net:8654",
				WSURL:   "ws://santing.net:8654/ws",
			},
		},
		ServerSelected:        0,
		Tab:                   TabChannels,
		AudioFocus:            AudioFocusCapture,
		AudioCaptureSelected:  -1,
		AudioPlaybackSelected: -1,
		SessionStatus:         "Not connected",
	}
}

func (m Model) Init() tea.Cmd {
	if m.ServerURL == nil {
		return LoadDevicesCmd()
	}
	return tea.Batch(
		LoadDevicesCmd(),
		LoadRoomsCmd(m.ServerURL.String()),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return Update(m, msg)
}

func (m Model) View() string {
	return View(m)
}
