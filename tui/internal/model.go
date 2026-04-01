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

const (
	ServerFormFieldName = iota
	ServerFormFieldURL
)

type Model struct {
	WindowWidth  int
	WindowHeight int

	ServerURL    *url.URL
	WebsocketURL *url.URL

	Cursor        int
	Channels      []Channel
	ActiveChannel *Channel
	RoomFormOpen  bool
	RoomFormName  string
	RoomFormErr   string

	Servers         []ServerOption
	ServerCursor    int
	ServerSelected  int
	ServerErr       string
	ServerFormErr   string
	ServerFormOpen  bool
	ServerFormField int
	ServerFormName  string
	ServerFormURL   string

	Tab int

	AudioFocus            int
	AudioCaptureDevices   []malgo.DeviceInfo
	AudioPlaybackDevices  []malgo.DeviceInfo
	AudioCaptureCursor    int
	AudioPlaybackCursor   int
	AudioCaptureSelected  int
	AudioPlaybackSelected int
	AudioCaptureName      string
	AudioPlaybackName     string
	AudioErr              string

	AudioEngine     *AudioEngine
	MicMonitor      *MicLevelMonitor
	VoiceActivation *VoiceActivation
	WebRTCClient    *WebRTCClient
	SessionStatus   string
	MicMuted        bool

	VoiceActivationThresholdDB float64
}

func New() Model {
	return Model{
		ServerURL:    nil,
		WebsocketURL: nil,
		Channels:     []Channel{},
		Servers: []ServerOption{
			{
				Name:    "Localhost",
				HTTPURL: defaultBackendBaseURL,
				WSURL:   defaultBackendWS,
			},
		},
		ServerSelected:             0,
		Tab:                        TabChannels,
		AudioFocus:                 AudioFocusCapture,
		AudioCaptureSelected:       -1,
		AudioPlaybackSelected:      -1,
		SessionStatus:              "Not connected",
		VoiceActivationThresholdDB: defaultVoiceActivationThresholdDB,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		LoadConfigCmd(),
		meterTickCmd(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return Update(m, msg)
}

func (m Model) View() string {
	return View(m)
}
