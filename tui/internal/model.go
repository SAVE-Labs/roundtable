package internal

import (
	"net/url"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gen2brain/malgo"
)

type Channel struct {
	ID          string
	Name        string
	MemberCount int
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

	DisplayName   string
	NameFormOpen  bool
	NameFormValue string

	EventsClient      *EventsClient
	ActiveRoomMembers []string

	VoiceActivationThresholdDB float64
	NoiseSuppressionEnabled    bool
	MicGainDB                  float64

	SelfTest *AudioEngine // non-nil while mic self-test is active in the Audio tab
}

func New() Model {
	hostname, err := os.Hostname()
	displayName := "User"
	if err == nil && hostname != "" {
		displayName = hostname
	}

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
		DisplayName:                displayName,
		VoiceActivationThresholdDB: defaultVoiceActivationThresholdDB,
		NoiseSuppressionEnabled:    true,
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
