package internal

import (
	"fmt"
	"log"
	"math"
	"net/url"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	// Color scheme
	primaryColor   = lipgloss.Color("#BD93F9") // Dracula Purple
	secondaryColor = lipgloss.Color("#8BE9FD") // Dracula Cyan
	accentColor    = lipgloss.Color("#50FA7B") // Dracula Green
	mutedColor     = lipgloss.Color("#6272A4") // Dracula Comment
	errorColor     = lipgloss.Color("#FF5555") // Dracula Red

	// Styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			MarginBottom(1)

	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F8F8F2")).
			Background(primaryColor).
			Padding(0, 2)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(mutedColor).
				Padding(0, 2)

	selectedStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true)

	cursorStyle = lipgloss.NewStyle().
			Foreground(secondaryColor)

	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginTop(1)

	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 2)

	sectionTitleStyle = lipgloss.NewStyle().
				Foreground(secondaryColor).
				Bold(true).
				MarginBottom(1)

	mutedStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	meterFillStyle = lipgloss.NewStyle().
			Foreground(accentColor)

	meterCutoffStyle = lipgloss.NewStyle().
				Foreground(errorColor).
				Bold(true)
)

type meterTickMsg struct{}

func meterTickCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
		return meterTickMsg{}
	})
}

func Update(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ConfigLoadedMsg:
		return handleConfigLoaded(m, msg)

	case ConfigSavedMsg:
		return handleConfigSaved(m, msg), nil

	case DevicesMsg:
		return handleAudioDevices(m, msg)

	case RoomsMsg:
		return handleRooms(m, msg), nil

	case RoomCreatedMsg:
		return handleRoomCreated(m, msg), nil

	case RoomDeletedMsg:
		return handleRoomDeleted(m, msg), nil

	case ServerInfoMsg:
		return handleServerInfo(m, msg)

	case EventsConnectedMsg:
		if m.EventsClient != nil {
			m.EventsClient.Close()
		}
		m.EventsClient = msg.Client
		return m, m.EventsClient.WaitForEvent()

	case eventsDisconnectedMsg:
		m.EventsClient = nil
		return m, nil

	case RoomEventMsg:
		return handleRoomEvent(m, msg)

	case tea.WindowSizeMsg:
		m.WindowWidth = msg.Width
		m.WindowHeight = msg.Height
		return m, nil

	case meterTickMsg:
		return m, meterTickCmd()

	case tea.KeyMsg:
		return handleKeyPress(m, msg)
	}

	return m, nil
}

func handleConfigLoaded(m Model, msg ConfigLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.AudioErr = msg.Err.Error()
	}

	cfg := msg.Config
	m.MicMuted = cfg.MicMuted
	if cfg.DisplayName != "" {
		m.DisplayName = cfg.DisplayName
	}
	m.VoiceActivationThresholdDB = defaultVoiceActivationThresholdDB
	if cfg.VoiceActivationThreshold != nil {
		m.VoiceActivationThresholdDB = clampVoiceActivationThresholdDB(*cfg.VoiceActivationThreshold)
	}
	m.NoiseSuppressionEnabled = true
	if cfg.NoiseSuppressionEnabled != nil {
		m.NoiseSuppressionEnabled = *cfg.NoiseSuppressionEnabled
	}
	m.MicGainDB = 0.0
	if cfg.MicGainDB != nil {
		m.MicGainDB = *cfg.MicGainDB
	}
	m.ServerSelected = -1
	m.ServerCursor = 0
	m.ServerURL = nil
	m.WebsocketURL = nil

	m.AudioCaptureName = cfg.CaptureDeviceName
	m.AudioPlaybackName = cfg.PlaybackDeviceName

	if len(cfg.Servers) > 0 {
		m.Servers = m.Servers[:0]
		for _, server := range cfg.Servers {
			if strings.TrimSpace(server.HTTPURL) == "" || strings.TrimSpace(server.WSURL) == "" {
				continue
			}
			name := strings.TrimSpace(server.Name)
			if name == "" {
				name = server.HTTPURL
			}
			m.Servers = append(m.Servers, ServerOption{Name: name, HTTPURL: server.HTTPURL, WSURL: server.WSURL})
		}
		if len(m.Servers) == 0 {
			m.ServerSelected = -1
			m.ServerCursor = 0
		}
	}

	if cfg.LastUsedServer.HTTPURL != "" && cfg.LastUsedServer.WSURL != "" {
		httpURL, err := url.Parse(cfg.LastUsedServer.HTTPURL)
		if err != nil {
			m.AudioErr = err.Error()
		} else {
			wsURL, wsErr := url.Parse(cfg.LastUsedServer.WSURL)
			if wsErr != nil {
				m.AudioErr = wsErr.Error()
			} else {
				if !strings.HasSuffix(strings.TrimRight(wsURL.Path, "/"), "/ws") {
					wsURL.Path = strings.TrimRight(wsURL.Path, "/") + "/ws"
				}
				m.ServerURL = httpURL
				m.WebsocketURL = wsURL
				match := -1
				for i, server := range m.Servers {
					if server.HTTPURL == cfg.LastUsedServer.HTTPURL && server.WSURL == cfg.LastUsedServer.WSURL {
						match = i
						break
					}
				}
				if match == -1 {
					name := cfg.LastUsedServer.Name
					if strings.TrimSpace(name) == "" {
						name = cfg.LastUsedServer.HTTPURL
					}
					m.Servers = append(m.Servers, ServerOption{Name: name, HTTPURL: cfg.LastUsedServer.HTTPURL, WSURL: cfg.LastUsedServer.WSURL})
					match = len(m.Servers) - 1
				}
				m.ServerSelected = match
				m.ServerCursor = match
			}
		}
	}

	if m.ServerSelected < 0 || m.ServerSelected >= len(m.Servers) {
		m.Tab = TabServers
	} else {
		m.Tab = TabChannels
	}

	if m.ServerURL == nil {
		return m, LoadDevicesCmd()
	}

	return m, tea.Batch(
		LoadDevicesCmd(),
		LoadRoomsCmd(m.ServerURL.String()),
		ConnectEventsCmd(m.ServerURL.String()),
	)
}

func handleConfigSaved(m Model, msg ConfigSavedMsg) Model {
	if msg.Err != nil {
		m.AudioErr = msg.Err.Error()
	}
	return m
}

func handleRooms(m Model, msg RoomsMsg) Model {
	if msg.Err != nil {
		m.SessionStatus = "Rooms unavailable"
		m.AudioErr = msg.Err.Error()
		return m
	}

	m.Channels = msg.Channels
	if len(m.Channels) == 0 {
		m.Cursor = 0
		m.ActiveChannel = nil
		m.SessionStatus = "No rooms available"
		return m
	}

	if m.Cursor >= len(m.Channels) {
		m.Cursor = len(m.Channels) - 1
	}
	if m.Cursor < 0 {
		m.Cursor = 0
	}

	if m.ActiveChannel != nil {
		found := false
		for i := range m.Channels {
			if m.Channels[i].ID == m.ActiveChannel.ID {
				m.ActiveChannel = &m.Channels[i]
				found = true
				break
			}
		}
		if !found {
			m.leaveChannel()
		}
	}

	if m.SessionStatus == "No rooms available" && len(m.Channels) > 0 {
		m.SessionStatus = "Rooms loaded"
	}

	return m
}

func handleRoomEvent(m Model, msg RoomEventMsg) (tea.Model, tea.Cmd) {
	// Update member count in the room list.
	for i := range m.Channels {
		if m.Channels[i].ID == msg.RoomID {
			m.Channels[i].MemberCount = len(msg.Members)
			break
		}
	}
	// Update the expanded member list if this is the active channel.
	if m.ActiveChannel != nil && m.ActiveChannel.ID == msg.RoomID {
		m.ActiveRoomMembers = msg.Members
	}
	if m.EventsClient != nil {
		return m, m.EventsClient.WaitForEvent()
	}
	return m, nil
}

func handleRoomCreated(m Model, msg RoomCreatedMsg) Model {
	if msg.Err != nil {
		if m.RoomFormOpen {
			m.RoomFormErr = msg.Err.Error()
		} else {
			m.AudioErr = msg.Err.Error()
		}
		m.SessionStatus = "Create room failed"
		return m
	}

	m.Channels = append(m.Channels, msg.Channel)
	m.Cursor = len(m.Channels) - 1
	m.RoomFormOpen = false
	m.RoomFormName = ""
	m.RoomFormErr = ""
	m.AudioErr = ""
	m.SessionStatus = "Created room " + msg.Channel.Name
	return m
}

func handleRoomDeleted(m Model, msg RoomDeletedMsg) Model {
	if msg.Err != nil {
		m.AudioErr = msg.Err.Error()
		m.SessionStatus = "Delete room failed"
		return m
	}

	removedIndex := -1
	for i, ch := range m.Channels {
		if ch.ID == msg.RoomID {
			removedIndex = i
			break
		}
	}

	if removedIndex == -1 {
		m.SessionStatus = "Room deleted"
		return m
	}

	if m.ActiveChannel != nil && m.ActiveChannel.ID == msg.RoomID {
		m.leaveChannel()
	}

	m.Channels = append(m.Channels[:removedIndex], m.Channels[removedIndex+1:]...)
	if len(m.Channels) == 0 {
		m.Cursor = 0
		m.ActiveChannel = nil
		m.SessionStatus = "No rooms available"
		return m
	}

	if m.Cursor >= len(m.Channels) {
		m.Cursor = len(m.Channels) - 1
	}
	if m.Cursor < 0 {
		m.Cursor = 0
	}

	m.SessionStatus = "Room deleted"
	return m
}

func handleServerInfo(m Model, msg ServerInfoMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		friendlyErr := "Error connecting to server, are you sure the URL is correct?"
		if msg.Mode == ServerProbeAdd {
			m.ServerFormErr = friendlyErr
			m.ServerErr = ""
			m.SessionStatus = "Server add failed"
		} else {
			m.ServerErr = friendlyErr
			m.ServerFormErr = ""
			m.SessionStatus = "Server selection failed"
		}
		return m, nil
	}

	selected := msg.Server

	if msg.Mode == ServerProbeAdd {
		m.Servers = append(m.Servers, selected)
		m.ServerCursor = len(m.Servers) - 1
		m.ServerSelected = m.ServerCursor
		m.ServerFormErr = ""
		m.ServerFormOpen = false
		m.ServerFormName = ""
		m.ServerFormURL = ""
	} else {
		if msg.Index < 0 || msg.Index >= len(m.Servers) {
			return m, nil
		}
		m.ServerCursor = msg.Index
		m.ServerSelected = msg.Index
		m.Servers[msg.Index] = selected
	}

	httpURL, err := url.Parse(selected.HTTPURL)
	if err != nil {
		if msg.Mode == ServerProbeAdd {
			m.ServerFormErr = err.Error()
		} else {
			m.ServerErr = err.Error()
		}
		m.SessionStatus = "Server selection failed"
		return m, nil
	}
	wsURL, err := url.Parse(selected.WSURL)
	if err != nil {
		if msg.Mode == ServerProbeAdd {
			m.ServerFormErr = err.Error()
		} else {
			m.ServerErr = err.Error()
		}
		m.SessionStatus = "Server selection failed"
		return m, nil
	}

	m.leaveChannel()
	if m.EventsClient != nil {
		m.EventsClient.Close()
		m.EventsClient = nil
	}
	m.ServerURL = httpURL
	m.WebsocketURL = wsURL
	m.ServerErr = ""
	m.ServerFormErr = ""
	m.SessionStatus = "Using server " + selected.Name

	return m, tea.Batch(
		LoadRoomsCmd(m.ServerURL.String()),
		SaveConfigCmd(m.ConfigSnapshot()),
		ConnectEventsCmd(m.ServerURL.String()),
	)
}

func handleAudioDevices(m Model, msg DevicesMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.AudioErr = msg.Err.Error()
	} else {
		m.AudioErr = ""
		m.AudioCaptureDevices = msg.Capture
		m.AudioPlaybackDevices = msg.Playback

		if m.AudioCaptureName != "" {
			for i, dev := range m.AudioCaptureDevices {
				if dev.Name() == m.AudioCaptureName {
					m.AudioCaptureSelected = i
					m.AudioCaptureCursor = i
					break
				}
			}
		}
		if m.AudioPlaybackName != "" {
			for i, dev := range m.AudioPlaybackDevices {
				if dev.Name() == m.AudioPlaybackName {
					m.AudioPlaybackSelected = i
					m.AudioPlaybackCursor = i
					break
				}
			}
		}

		if m.AudioCaptureCursor >= len(m.AudioCaptureDevices) {
			m.AudioCaptureCursor = 0
		}
		if m.AudioPlaybackCursor >= len(m.AudioPlaybackDevices) {
			m.AudioPlaybackCursor = 0
		}
		m.AudioCaptureName = ""
		m.AudioPlaybackName = ""

		if err := m.ensureMicMonitor(); err != nil {
			m.AudioErr = err.Error()
		}
	}
	return m, nil
}

func handleKeyPress(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.Tab == TabServers && m.NameFormOpen {
		switch msg.String() {
		case "ctrl+c", "q":
			m.leaveChannel()
			return m, tea.Quit
		default:
			return handleNameFormKeys(m, msg)
		}
	}

	if m.Tab == TabChannels && m.RoomFormOpen {
		switch msg.String() {
		case "ctrl+c", "q":
			m.leaveChannel()
			return m, tea.Quit
		default:
			return handleRoomFormKeys(m, msg)
		}
	}

	if m.Tab == TabServers && m.ServerFormOpen {
		switch msg.String() {
		case "ctrl+c", "q":
			m.leaveChannel()
			return m, tea.Quit
		default:
			return handleServerKeys(m, msg)
		}
	}

	switch msg.String() {
	case "ctrl+c", "q":
		m.leaveChannel()
		return m, tea.Quit

	case "tab", "right", "l":
		m.Tab = (m.Tab + 1) % TabCount

	case "left", "h":
		m.Tab = (m.Tab - 1 + TabCount) % TabCount

	case "r":
		if m.Tab == TabAudio {
			return m, LoadDevicesCmd()
		}
		if m.Tab == TabChannels && m.ServerURL != nil {
			var cmds []tea.Cmd
			cmds = append(cmds, LoadRoomsCmd(m.ServerURL.String()))
			if m.EventsClient == nil {
				cmds = append(cmds, ConnectEventsCmd(m.ServerURL.String()))
			}
			return m, tea.Batch(cmds...)
		}

	case "e":
		if m.Tab == TabServers {
			m.NameFormOpen = true
			m.NameFormValue = m.DisplayName
			return m, nil
		}

	case "n":
		if m.Tab == TabChannels {
			m.RoomFormOpen = true
			m.RoomFormName = ""
			m.RoomFormErr = ""
			return m, nil
		}
		if m.Tab == TabAudio {
			return handleAudioKeys(m, msg)
		}
	case ",", ".":
		if m.Tab == TabAudio {
			return handleAudioKeys(m, msg)
		}

	case "m":
		m.MicMuted = !m.MicMuted
		if m.WebRTCClient != nil {
			m.WebRTCClient.SetMuted(m.MicMuted)
		}
		return m, SaveConfigCmd(m.ConfigSnapshot())

	default:
		switch m.Tab {
		case TabChannels:
			return handleChannelsKeys(m, msg)
		case TabServers:
			return handleServerKeys(m, msg)
		default:
			return handleAudioKeys(m, msg)
		}
	}

	return m, nil
}

func handleRoomFormKeys(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.RoomFormOpen = false
		m.RoomFormName = ""
		m.RoomFormErr = ""
		return m, nil
	case tea.KeyEnter:
		if m.ServerURL == nil {
			m.RoomFormErr = "server url not configured"
			m.SessionStatus = "Create room failed"
			return m, nil
		}

		name := strings.TrimSpace(m.RoomFormName)
		if name == "" {
			m.RoomFormErr = "room name is required"
			m.SessionStatus = "Create room failed"
			return m, nil
		}

		m.RoomFormErr = ""
		m.SessionStatus = "Creating room " + name
		return m, CreateRoomCmd(m.ServerURL.String(), name)
	case tea.KeyBackspace, tea.KeyDelete:
		if len(m.RoomFormName) > 0 {
			m.RoomFormName = m.RoomFormName[:len(m.RoomFormName)-1]
		}
		return m, nil
	case tea.KeyRunes:
		m.RoomFormName += string(msg.Runes)
		return m, nil
	}

	return m, nil
}

func handleNameFormKeys(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.NameFormOpen = false
		m.NameFormValue = ""
		return m, nil
	case tea.KeyEnter:
		name := strings.TrimSpace(m.NameFormValue)
		if name == "" {
			m.NameFormOpen = false
			m.NameFormValue = ""
			return m, nil
		}
		m.DisplayName = name
		m.NameFormOpen = false
		m.NameFormValue = ""
		return m, SaveConfigCmd(m.ConfigSnapshot())
	case tea.KeyBackspace, tea.KeyDelete:
		if len(m.NameFormValue) > 0 {
			m.NameFormValue = m.NameFormValue[:len(m.NameFormValue)-1]
		}
		return m, nil
	case tea.KeyRunes:
		m.NameFormValue += string(msg.Runes)
		return m, nil
	}
	return m, nil
}

func handleServerKeys(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.ServerFormOpen {
		return handleServerFormKeys(m, msg)
	}

	switch msg.String() {
	case "a":
		m.ServerFormOpen = true
		m.ServerFormField = ServerFormFieldName
		m.ServerFormName = ""
		m.ServerFormURL = ""
		m.ServerFormErr = ""
		return m, nil
	case "d":
		if len(m.Servers) == 0 {
			return m, nil
		}

		deleteIndex := m.ServerCursor
		if deleteIndex < 0 || deleteIndex >= len(m.Servers) {
			return m, nil
		}

		deletedSelected := m.ServerSelected == deleteIndex
		m.Servers = append(m.Servers[:deleteIndex], m.Servers[deleteIndex+1:]...)

		if len(m.Servers) == 0 {
			m.leaveChannel()
			if m.EventsClient != nil {
				m.EventsClient.Close()
				m.EventsClient = nil
			}
			m.ServerSelected = -1
			m.ServerCursor = 0
			m.ServerURL = nil
			m.WebsocketURL = nil
			m.SessionStatus = "No server selected"
			m.ServerErr = ""
			return m, SaveConfigCmd(m.ConfigSnapshot())
		}

		if m.ServerCursor >= len(m.Servers) {
			m.ServerCursor = len(m.Servers) - 1
		}
		if m.ServerCursor < 0 {
			m.ServerCursor = 0
		}

		if m.ServerSelected > deleteIndex {
			m.ServerSelected--
		}

		if deletedSelected {
			m.leaveChannel()
			if m.EventsClient != nil {
				m.EventsClient.Close()
				m.EventsClient = nil
			}
			m.ServerSelected = m.ServerCursor
			selected := m.Servers[m.ServerSelected]

			httpURL, err := url.Parse(selected.HTTPURL)
			if err != nil {
				m.ServerErr = err.Error()
				m.SessionStatus = "Server selection failed"
				return m, SaveConfigCmd(m.ConfigSnapshot())
			}
			wsURL, err := url.Parse(selected.WSURL)
			if err != nil {
				m.ServerErr = err.Error()
				m.SessionStatus = "Server selection failed"
				return m, SaveConfigCmd(m.ConfigSnapshot())
			}

			m.ServerURL = httpURL
			m.WebsocketURL = wsURL
			m.ServerErr = ""
			m.SessionStatus = "Using server " + selected.Name
			return m, tea.Batch(
				LoadRoomsCmd(m.ServerURL.String()),
				SaveConfigCmd(m.ConfigSnapshot()),
				ConnectEventsCmd(m.ServerURL.String()),
			)
		}

		return m, SaveConfigCmd(m.ConfigSnapshot())
	case "up", "k":
		if m.ServerCursor > 0 {
			m.ServerCursor--
		}
	case "down", "j":
		if m.ServerCursor < len(m.Servers)-1 {
			m.ServerCursor++
		}
	case " ", "enter":
		if len(m.Servers) == 0 {
			return m, nil
		}
		selected := m.Servers[m.ServerCursor]
		m.ServerErr = ""
		m.ServerFormErr = ""
		m.SessionStatus = "Checking server " + selected.Name
		return m, ProbeServerInfoCmd(ServerProbeSelect, m.ServerCursor, selected)
	}

	return m, nil
}

func handleServerFormKeys(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.ServerFormOpen = false
		m.ServerFormName = ""
		m.ServerFormURL = ""
		m.ServerFormErr = ""
		return m, nil
	case tea.KeyTab:
		if m.ServerFormField == ServerFormFieldName {
			m.ServerFormField = ServerFormFieldURL
		} else {
			m.ServerFormField = ServerFormFieldName
		}
		return m, nil
	case tea.KeyEnter:
		if m.ServerFormField == ServerFormFieldName {
			m.ServerFormField = ServerFormFieldURL
			return m, nil
		}

		httpURL, wsURL, err := normalizeServerURLs(m.ServerFormURL)
		if err != nil {
			m.ServerFormErr = err.Error()
			m.SessionStatus = "Server add failed"
			return m, nil
		}

		name := strings.TrimSpace(m.ServerFormName)
		if name == "" {
			name = httpURL
		}

		m.ServerFormErr = ""
		m.SessionStatus = "Checking server " + name
		candidate := ServerOption{Name: name, HTTPURL: httpURL, WSURL: wsURL}
		return m, ProbeServerInfoCmd(ServerProbeAdd, -1, candidate)
	case tea.KeyBackspace, tea.KeyDelete:
		if m.ServerFormField == ServerFormFieldName {
			if len(m.ServerFormName) > 0 {
				m.ServerFormName = m.ServerFormName[:len(m.ServerFormName)-1]
			}
		} else {
			if len(m.ServerFormURL) > 0 {
				m.ServerFormURL = m.ServerFormURL[:len(m.ServerFormURL)-1]
			}
		}
		return m, nil
	case tea.KeyRunes:
		text := string(msg.Runes)
		if m.ServerFormField == ServerFormFieldName {
			m.ServerFormName += text
		} else {
			m.ServerFormURL += text
		}
		return m, nil
	}

	if msg.String() == "left" || msg.String() == "h" {
		m.ServerFormField = ServerFormFieldName
		return m, nil
	}
	if msg.String() == "right" || msg.String() == "l" {
		m.ServerFormField = ServerFormFieldURL
		return m, nil
	}

	return m, nil
}

func normalizeServerURLs(raw string) (string, string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", fmt.Errorf("server url is required")
	}

	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", "", err
	}
	if parsed.Host == "" {
		return "", "", fmt.Errorf("invalid server url")
	}

	httpURL := *parsed
	wsURL := *parsed

	switch strings.ToLower(parsed.Scheme) {
	case "http":
		httpURL.Scheme = "http"
		wsURL.Scheme = "ws"
	case "https":
		httpURL.Scheme = "https"
		wsURL.Scheme = "wss"
	case "ws":
		httpURL.Scheme = "http"
		wsURL.Scheme = "ws"
	case "wss":
		httpURL.Scheme = "https"
		wsURL.Scheme = "wss"
	default:
		return "", "", fmt.Errorf("unsupported url scheme: %s", parsed.Scheme)
	}

	if !strings.HasSuffix(strings.TrimRight(wsURL.Path, "/"), "/ws") {
		wsURL.Path = strings.TrimRight(wsURL.Path, "/") + "/ws"
	}

	return httpURL.String(), wsURL.String(), nil
}

func handleChannelsKeys(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.Cursor > 0 {
			m.Cursor--
		}
	case "down", "j":
		if m.Cursor < len(m.Channels)-1 {
			m.Cursor++
		}
	case "d":
		if m.ServerURL == nil {
			m.AudioErr = "server url not configured"
			m.SessionStatus = "Delete room failed"
			return m, nil
		}
		if len(m.Channels) == 0 || m.Cursor < 0 || m.Cursor >= len(m.Channels) {
			return m, nil
		}
		room := m.Channels[m.Cursor]
		m.AudioErr = ""
		m.SessionStatus = "Deleting room " + room.Name
		return m, DeleteRoomCmd(m.ServerURL.String(), room.ID)
	case " ", "enter":
		if len(m.Channels) > 0 {
			if m.ActiveChannel != nil && m.ActiveChannel.ID == m.Channels[m.Cursor].ID {
				m.leaveChannel()
			} else {
				m.ActiveChannel = &m.Channels[m.Cursor]
				if err := m.joinActiveChannel(); err != nil {
					m.SessionStatus = "Join failed"
					m.AudioErr = err.Error()
					m.ActiveChannel = nil
				}
			}
		}
	}
	return m, nil
}

func (m *Model) joinActiveChannel() error {
	if m.ActiveChannel == nil {
		return fmt.Errorf("no channel selected")
	}
	selectedChannel := *m.ActiveChannel
	log.Printf("join: begin room_id=%s room_name=%s", selectedChannel.ID, selectedChannel.Name)
	if m.AudioCaptureSelected < 0 || m.AudioCaptureSelected >= len(m.AudioCaptureDevices) {
		log.Printf("join: missing capture device selection")
		return fmt.Errorf("select a capture device first")
	}
	if m.AudioPlaybackSelected < 0 || m.AudioPlaybackSelected >= len(m.AudioPlaybackDevices) {
		log.Printf("join: missing playback device selection")
		return fmt.Errorf("select a playback device first")
	}
	if m.WebsocketURL == nil {
		log.Printf("join: websocket url not configured")
		return fmt.Errorf("websocket url not configured")
	}

	m.leaveChannel()
	m.stopSelfTest()
	m.stopMicMonitor()

	roomWSURL, err := websocketURLForRoom(m.WebsocketURL, selectedChannel.ID, m.DisplayName)
	if err != nil {
		log.Printf("join: websocket url build failed room_id=%s err=%v", selectedChannel.ID, err)
		return err
	}
	log.Printf("join: websocket url=%s", roomWSURL)

	engine := NewAudioEngine()
	voiceActivation := NewVoiceActivation(
		audioSampleRate,
		audioFrameSamples,
		m.VoiceActivationThresholdDB,
		defaultVoiceActivationAttackMs,
		defaultVoiceActivationReleaseMs,
		defaultVoiceActivationHoldMs,
	)
	client, err := NewWebRTCClient(roomWSURL, func(pcm []byte) {
		engine.PushPCM16LE(pcm)
	})
	if err != nil {
		log.Printf("join: webrtc client init failed room_id=%s ws=%s err=%v", selectedChannel.ID, roomWSURL, err)
		return err
	}

	capture := m.AudioCaptureDevices[m.AudioCaptureSelected]
	playback := m.AudioPlaybackDevices[m.AudioPlaybackSelected]
	if err := engine.Start(capture, playback, func(pcm []byte) {
		voiceActivation.ProcessPCM16LE(pcm)
		_ = client.SendPCM16LE(pcm)
	}, m.NoiseSuppressionEnabled); err != nil {
		log.Printf("join: audio start failed capture=%s playback=%s err=%v", capture.Name(), playback.Name(), err)
		client.Close()
		return err
	}
	engine.SetMicGainDB(m.MicGainDB)

	m.WebRTCClient = client
	m.AudioEngine = engine
	m.VoiceActivation = voiceActivation
	m.WebRTCClient.SetMuted(m.MicMuted)
	m.ActiveChannel = &selectedChannel
	m.AudioErr = ""
	m.SessionStatus = "Connected to " + selectedChannel.Name
	log.Printf("join: connected room_id=%s room_name=%s", selectedChannel.ID, selectedChannel.Name)
	return nil
}

func websocketURLForRoom(base *url.URL, roomID, peerName string) (string, error) {
	if base == nil {
		return "", fmt.Errorf("websocket url not configured")
	}
	if strings.TrimSpace(roomID) == "" {
		return "", fmt.Errorf("room id is empty")
	}

	u := *base
	q := u.Query()
	q.Set("room", roomID)
	if name := strings.TrimSpace(peerName); name != "" {
		q.Set("peer_name", name)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (m *Model) leaveChannel() {
	if m.WebRTCClient != nil {
		m.WebRTCClient.Close()
		m.WebRTCClient = nil
	}
	if m.AudioEngine != nil {
		m.AudioEngine.Close()
		m.AudioEngine = nil
	}
	m.VoiceActivation = nil
	m.ActiveChannel = nil
	m.ActiveRoomMembers = nil
	if err := m.ensureMicMonitor(); err != nil {
		m.AudioErr = err.Error()
	}
	m.SessionStatus = "Not connected"
}

func handleAudioKeys(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "[", "-":
		m.VoiceActivationThresholdDB = clampVoiceActivationThresholdDB(m.VoiceActivationThresholdDB - voiceActivationThresholdStepDB)
		if m.VoiceActivation != nil {
			m.VoiceActivation.SetThresholdDB(m.VoiceActivationThresholdDB)
		}
		m.SessionStatus = fmt.Sprintf("Voice activation threshold %.1f dB", m.VoiceActivationThresholdDB)
		return m, SaveConfigCmd(m.ConfigSnapshot())
	case "]", "=":
		m.VoiceActivationThresholdDB = clampVoiceActivationThresholdDB(m.VoiceActivationThresholdDB + voiceActivationThresholdStepDB)
		if m.VoiceActivation != nil {
			m.VoiceActivation.SetThresholdDB(m.VoiceActivationThresholdDB)
		}
		m.SessionStatus = fmt.Sprintf("Voice activation threshold %.1f dB", m.VoiceActivationThresholdDB)
		return m, SaveConfigCmd(m.ConfigSnapshot())
	case ",":
		if m.MicGainDB > -12.0 {
			m.MicGainDB -= 1.0
		}
		if m.AudioEngine != nil {
			m.AudioEngine.SetMicGainDB(m.MicGainDB)
		}
		if m.SelfTest != nil {
			m.SelfTest.SetMicGainDB(m.MicGainDB)
		}
		m.SessionStatus = fmt.Sprintf("Mic gain: %+.0f dB", m.MicGainDB)
		return m, SaveConfigCmd(m.ConfigSnapshot())
	case ".":
		if m.MicGainDB < 24.0 {
			m.MicGainDB += 1.0
		}
		if m.AudioEngine != nil {
			m.AudioEngine.SetMicGainDB(m.MicGainDB)
		}
		if m.SelfTest != nil {
			m.SelfTest.SetMicGainDB(m.MicGainDB)
		}
		m.SessionStatus = fmt.Sprintf("Mic gain: %+.0f dB", m.MicGainDB)
		return m, SaveConfigCmd(m.ConfigSnapshot())
	case "n":
		m.NoiseSuppressionEnabled = !m.NoiseSuppressionEnabled
		if m.AudioEngine != nil {
			m.AudioEngine.SetNoiseSuppression(m.NoiseSuppressionEnabled)
		}
		if m.SelfTest != nil {
			m.SelfTest.SetNoiseSuppression(m.NoiseSuppressionEnabled)
		}
		if m.NoiseSuppressionEnabled {
			m.SessionStatus = "Noise suppression: on"
		} else {
			m.SessionStatus = "Noise suppression: off"
		}
		return m, SaveConfigCmd(m.ConfigSnapshot())
	case "L":
		if m.SelfTest != nil {
			m.stopSelfTest()
			if err := m.ensureMicMonitor(); err != nil {
				m.AudioErr = err.Error()
			}
			m.SessionStatus = "Mic self-test off"
		} else {
			if err := m.startSelfTest(); err != nil {
				m.AudioErr = err.Error()
			} else {
				m.SessionStatus = "Mic self-test on — you are hearing yourself"
			}
		}
		return m, nil
	case "c":
		m.AudioFocus = AudioFocusCapture
	case "p":
		m.AudioFocus = AudioFocusPlayback
	case "up", "k":
		if m.AudioFocus == AudioFocusCapture {
			if len(m.AudioCaptureDevices) > 0 && m.AudioCaptureCursor > 0 {
				m.AudioCaptureCursor--
			} else if len(m.AudioPlaybackDevices) > 0 {
				m.AudioFocus = AudioFocusPlayback
				m.AudioPlaybackCursor = len(m.AudioPlaybackDevices) - 1
			}
		} else {
			if len(m.AudioPlaybackDevices) > 0 && m.AudioPlaybackCursor > 0 {
				m.AudioPlaybackCursor--
			} else if len(m.AudioCaptureDevices) > 0 {
				m.AudioFocus = AudioFocusCapture
				m.AudioCaptureCursor = len(m.AudioCaptureDevices) - 1
			}
		}
	case "down", "j":
		if m.AudioFocus == AudioFocusCapture {
			if len(m.AudioCaptureDevices) > 0 && m.AudioCaptureCursor < len(m.AudioCaptureDevices)-1 {
				m.AudioCaptureCursor++
			} else if len(m.AudioPlaybackDevices) > 0 {
				m.AudioFocus = AudioFocusPlayback
				m.AudioPlaybackCursor = 0
			} else if len(m.AudioCaptureDevices) > 0 {
				m.AudioCaptureCursor = 0
			}
		} else {
			if len(m.AudioPlaybackDevices) > 0 && m.AudioPlaybackCursor < len(m.AudioPlaybackDevices)-1 {
				m.AudioPlaybackCursor++
			} else if len(m.AudioCaptureDevices) > 0 {
				m.AudioFocus = AudioFocusCapture
				m.AudioCaptureCursor = 0
			} else if len(m.AudioPlaybackDevices) > 0 {
				m.AudioPlaybackCursor = 0
			}
		}
	case " ":
		saved := false
		if m.AudioFocus == AudioFocusCapture && len(m.AudioCaptureDevices) > 0 {
			m.AudioCaptureSelected = m.AudioCaptureCursor
			saved = true
		}
		if m.AudioFocus == AudioFocusPlayback && len(m.AudioPlaybackDevices) > 0 {
			m.AudioPlaybackSelected = m.AudioPlaybackCursor
			saved = true
		}
		if saved {
			if m.AudioFocus == AudioFocusCapture {
				if err := m.ensureMicMonitor(); err != nil {
					m.AudioErr = err.Error()
				}
			}
			return m, SaveConfigCmd(m.ConfigSnapshot())
		}
	}
	return m, nil
}

func View(m Model) string {
	var b strings.Builder

	var tabsBuilder strings.Builder
	renderTabs(&tabsBuilder, m)

	header := titleStyle.Render("🎙️  Roundtable Audio Chat") + "\n\n" + tabsBuilder.String() + "\n"
	footer := "\n" + helpStyle.Render("Press q to quit • tab to switch tabs") + "\n"

	// Content
	var content string
	if m.Tab == TabChannels {
		content = renderChannels(m)
		if m.RoomFormOpen {
			modal := renderRoomFormModal(m, lipgloss.Width(content))
			contentWidth := lipgloss.Width(content)
			modalWidth := lipgloss.Width(modal)
			if modalWidth > contentWidth {
				contentWidth = modalWidth
			}
			content = lipgloss.JoinVertical(
				lipgloss.Left,
				content,
				"",
				lipgloss.Place(contentWidth, lipgloss.Height(modal), lipgloss.Center, lipgloss.Top, modal),
			)
		}
	} else if m.Tab == TabServers {
		content = renderServers(m)
		if m.ServerFormOpen {
			modal := renderServerFormModal(m, lipgloss.Width(content))
			contentWidth := lipgloss.Width(content)
			modalWidth := lipgloss.Width(modal)
			if modalWidth > contentWidth {
				contentWidth = modalWidth
			}
			content = lipgloss.JoinVertical(
				lipgloss.Left,
				content,
				"",
				lipgloss.Place(contentWidth, lipgloss.Height(modal), lipgloss.Center, lipgloss.Top, modal),
			)
		}
		if m.NameFormOpen {
			modal := renderNameFormModal(m, lipgloss.Width(content))
			contentWidth := lipgloss.Width(content)
			modalWidth := lipgloss.Width(modal)
			if modalWidth > contentWidth {
				contentWidth = modalWidth
			}
			content = lipgloss.JoinVertical(
				lipgloss.Left,
				content,
				"",
				lipgloss.Place(contentWidth, lipgloss.Height(modal), lipgloss.Center, lipgloss.Top, modal),
			)
		}
	} else {
		content = renderAudio(m)
	}
	renderedBoxStyle := boxStyle
	if m.WindowWidth > 0 {
		contentWidth := m.WindowWidth - renderedBoxStyle.GetHorizontalFrameSize()
		if contentWidth > 0 {
			renderedBoxStyle = renderedBoxStyle.Width(contentWidth)
		}
	}
	if m.WindowHeight > 0 {
		chromeHeight := lipgloss.Height(header) + lipgloss.Height(footer)
		contentHeight := m.WindowHeight - chromeHeight - renderedBoxStyle.GetVerticalFrameSize()
		if contentHeight > 0 {
			renderedBoxStyle = renderedBoxStyle.Height(contentHeight)
		}
	}

	b.WriteString(header)
	b.WriteString(renderedBoxStyle.Render(content))
	b.WriteString(footer)

	view := b.String()
	if m.WindowWidth > 0 && m.WindowHeight > 0 {
		return lipgloss.NewStyle().
			Width(m.WindowWidth).
			Height(m.WindowHeight).
			Render(view)
	}

	return view
}

func renderTabs(b *strings.Builder, m Model) {
	var tabs []string

	if m.Tab == TabChannels {
		tabs = append(tabs, tabActiveStyle.Render("📢 Channels"))
		tabs = append(tabs, tabInactiveStyle.Render("🖧 Servers"))
		tabs = append(tabs, tabInactiveStyle.Render("🎧 Audio"))
	} else if m.Tab == TabServers {
		tabs = append(tabs, tabInactiveStyle.Render("📢 Channels"))
		tabs = append(tabs, tabActiveStyle.Render("🖧 Servers"))
		tabs = append(tabs, tabInactiveStyle.Render("🎧 Audio"))
	} else {
		tabs = append(tabs, tabInactiveStyle.Render("📢 Channels"))
		tabs = append(tabs, tabInactiveStyle.Render("🖧 Servers"))
		tabs = append(tabs, tabActiveStyle.Render("🎧 Audio"))
	}

	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, tabs...))
}

func renderChannels(m Model) string {
	var b strings.Builder

	b.WriteString(sectionTitleStyle.Render("Channels"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ or j/k to move • space/enter join • n create room • d delete room • r reload • m mute"))
	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("Status: " + m.SessionStatus))
	b.WriteString("\n")
	micStatus := "unmuted"
	if m.MicMuted {
		micStatus = "muted"
	}
	b.WriteString(mutedStyle.Render("Mic: " + micStatus + " (m)"))
	b.WriteString("\n\n")

	if len(m.Channels) == 0 {
		b.WriteString(helpStyle.Render("No channels available"))
		return b.String()
	}

	for i, ch := range m.Channels {
		cursor := "  "
		if m.Cursor == i {
			cursor = cursorStyle.Render("❯ ")
		}

		isActive := m.ActiveChannel != nil && m.ActiveChannel.ID == ch.ID
		activeIndicator := mutedStyle.Render("○ ")
		if isActive {
			activeIndicator = selectedStyle.Render("● ")
		}

		name := ch.Name
		if m.Cursor == i {
			name = selectedStyle.Render(name)
		}

		memberCount := ""
		if ch.MemberCount > 0 {
			memberCount = mutedStyle.Render(fmt.Sprintf(" (%d)", ch.MemberCount))
		}

		b.WriteString(fmt.Sprintf("%s%s%s%s\n", cursor, activeIndicator, name, memberCount))

		if isActive && len(m.ActiveRoomMembers) > 0 {
			for _, member := range m.ActiveRoomMembers {
				b.WriteString(fmt.Sprintf("     %s %s\n", mutedStyle.Render("↳"), mutedStyle.Render(member)))
			}
		}
	}

	return b.String()
}

func renderRoomFormModal(m Model, totalWidth int) string {
	var b strings.Builder

	b.WriteString(sectionTitleStyle.Render("Create Room"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Type name • enter create • esc cancel"))
	b.WriteString("\n\n")

	if m.RoomFormErr != "" {
		b.WriteString(errorStyle.Render("⚠ Error: " + m.RoomFormErr))
		b.WriteString("\n\n")
	}

	name := m.RoomFormName
	if name == "" {
		name = mutedStyle.Render("(required)")
	}

	b.WriteString(cursorStyle.Render("❯ Name: ") + name)

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(secondaryColor).
		Padding(1, 2)

	if totalWidth > 6 {
		contentWidth := totalWidth - 6
		if contentWidth > lipgloss.Width(b.String()) {
			modalStyle = modalStyle.Width(contentWidth)
		}
	}

	return modalStyle.Render(b.String())
}

func renderNameFormModal(m Model, totalWidth int) string {
	var b strings.Builder

	b.WriteString(sectionTitleStyle.Render("Set Display Name"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Type name • enter save • esc cancel"))
	b.WriteString("\n\n")

	name := m.NameFormValue
	if name == "" {
		name = mutedStyle.Render("(required)")
	}

	b.WriteString(cursorStyle.Render("❯ Name: ") + name)

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(secondaryColor).
		Padding(1, 2)

	if totalWidth > 6 {
		contentWidth := totalWidth - 6
		if contentWidth > lipgloss.Width(b.String()) {
			modalStyle = modalStyle.Width(contentWidth)
		}
	}

	return modalStyle.Render(b.String())
}

func renderServers(m Model) string {
	var b strings.Builder

	b.WriteString(sectionTitleStyle.Render("Servers"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ or j/k move • space/enter select • a add • d delete • e change user name • r reload"))
	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("Name: " + m.DisplayName))
	b.WriteString("\n\n")

	if m.ServerErr != "" {
		b.WriteString(errorStyle.Render("⚠ Error: " + m.ServerErr))
		b.WriteString("\n\n")
	}

	if m.ServerURL != nil {
		b.WriteString(mutedStyle.Render("Current: " + m.ServerURL.String()))
		b.WriteString("\n\n")
	}

	if len(m.Servers) == 0 {
		b.WriteString(helpStyle.Render("No servers configured"))
		return b.String()
	}

	for i, server := range m.Servers {
		cursor := "  "
		if m.ServerCursor == i {
			cursor = cursorStyle.Render("❯ ")
		}

		selected := " "
		if m.ServerSelected == i {
			selected = selectedStyle.Render("● ")
		} else {
			selected = mutedStyle.Render("○ ")
		}

		name := server.Name
		if m.ServerCursor == i {
			name = selectedStyle.Render(name)
		}

		b.WriteString(fmt.Sprintf("%s%s%s\n", cursor, selected, name))
		b.WriteString(mutedStyle.Render("    " + server.HTTPURL))
		b.WriteString("\n")
	}

	return b.String()
}

func renderServerFormModal(m Model, totalWidth int) string {
	var b strings.Builder

	b.WriteString(sectionTitleStyle.Render("Add Server"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Type values • tab switch field • enter save • esc cancel"))
	b.WriteString("\n\n")

	if m.ServerFormErr != "" {
		b.WriteString(errorStyle.Render("⚠ Error: " + m.ServerFormErr))
		b.WriteString("\n\n")
	}

	nameLabel := "  Name: "
	urlLabel := "  URL:  "
	if m.ServerFormField == ServerFormFieldName {
		nameLabel = cursorStyle.Render("❯ Name: ")
	}
	if m.ServerFormField == ServerFormFieldURL {
		urlLabel = cursorStyle.Render("❯ URL:  ")
	}

	name := m.ServerFormName
	if name == "" {
		name = mutedStyle.Render("(optional)")
	}
	serverURL := m.ServerFormURL
	if serverURL == "" {
		serverURL = mutedStyle.Render("(required, e.g. localhost:8080)")
	}

	b.WriteString(nameLabel + name + "\n")
	b.WriteString(urlLabel + serverURL)

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(secondaryColor).
		Padding(1, 2)

	if totalWidth > 6 {
		contentWidth := totalWidth - 6
		if contentWidth > lipgloss.Width(b.String()) {
			modalStyle = modalStyle.Width(contentWidth)
		}
	}

	return modalStyle.Render(b.String())
}

func renderAudio(m Model) string {
	var b strings.Builder

	b.WriteString(sectionTitleStyle.Render("Audio Devices"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ devices • c/p capture/playback • space select • r reload • [/] voice threshold • ,/. mic gain • n noise suppression • L self-test"))
	b.WriteString("\n\n")

	b.WriteString(mutedStyle.Render("Voice activation: on"))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Threshold: %.1f dBFS ([/])", m.VoiceActivationThresholdDB)))
	b.WriteString("\n\n")
	b.WriteString(renderVoiceActivationMeter(m))
	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Mic gain: %+.0f dB (,/.)", m.MicGainDB)))
	b.WriteString("\n")
	nsLabel := "on"
	if !m.NoiseSuppressionEnabled {
		nsLabel = "off"
	}
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Noise suppression: %s (n)", nsLabel)))
	b.WriteString("\n")
	selfTestLabel := "off"
	selfTestStyle := mutedStyle
	if m.SelfTest != nil {
		selfTestLabel = "ON — you are hearing yourself"
		selfTestStyle = selectedStyle
	}
	b.WriteString(selfTestStyle.Render(fmt.Sprintf("Mic self-test: %s (L)", selfTestLabel)))
	b.WriteString("\n\n")

	// Capture devices
	captureTitle := "🎤 Capture Devices"
	if m.AudioFocus == AudioFocusCapture {
		captureTitle = selectedStyle.Render(captureTitle)
	} else {
		captureTitle = mutedStyle.Render(captureTitle)
	}
	b.WriteString(captureTitle)
	b.WriteString("\n")

	if len(m.AudioCaptureDevices) == 0 {
		b.WriteString(helpStyle.Render("  No capture devices found"))
		b.WriteString("\n")
	} else {
		for i, dev := range m.AudioCaptureDevices {
			cursor := "  "
			if m.AudioFocus == AudioFocusCapture && m.AudioCaptureCursor == i {
				cursor = cursorStyle.Render("❯ ")
			}

			selected := " "
			if m.AudioCaptureSelected == i {
				selected = selectedStyle.Render("● ")
			} else {
				selected = mutedStyle.Render("○ ")
			}

			name := dev.Name()
			if m.AudioFocus == AudioFocusCapture && m.AudioCaptureCursor == i {
				name = selectedStyle.Render(name)
			}

			b.WriteString(fmt.Sprintf("%s%s%s\n", cursor, selected, name))
		}
	}

	b.WriteString("\n")

	// Playback devices
	playbackTitle := "🔊 Playback Devices"
	if m.AudioFocus == AudioFocusPlayback {
		playbackTitle = selectedStyle.Render(playbackTitle)
	} else {
		playbackTitle = mutedStyle.Render(playbackTitle)
	}
	b.WriteString(playbackTitle)
	b.WriteString("\n")

	if len(m.AudioPlaybackDevices) == 0 {
		b.WriteString(helpStyle.Render("  No playback devices found"))
		b.WriteString("\n")
	} else {
		for i, dev := range m.AudioPlaybackDevices {
			cursor := "  "
			if m.AudioFocus == AudioFocusPlayback && m.AudioPlaybackCursor == i {
				cursor = cursorStyle.Render("❯ ")
			}

			selected := " "
			if m.AudioPlaybackSelected == i {
				selected = selectedStyle.Render("● ")
			} else {
				selected = mutedStyle.Render("○ ")
			}

			name := dev.Name()
			if m.AudioFocus == AudioFocusPlayback && m.AudioPlaybackCursor == i {
				name = selectedStyle.Render(name)
			}

			b.WriteString(fmt.Sprintf("%s%s%s\n", cursor, selected, name))
		}
	}

	return b.String()
}

func renderVoiceActivationMeter(m Model) string {
	const meterWidth = 34
	const meterMaxDB = 0.0

	levelDB := voiceActivationMinThresholdDB
	if m.VoiceActivation != nil {
		levelDB = m.VoiceActivation.InputLevelDB()
	} else if m.MicMonitor != nil {
		levelDB = m.MicMonitor.LevelDB()
	}

	if levelDB < voiceActivationMinThresholdDB {
		levelDB = voiceActivationMinThresholdDB
	}
	if levelDB > meterMaxDB {
		levelDB = meterMaxDB
	}

	thresholdDB := clampVoiceActivationThresholdDB(m.VoiceActivationThresholdDB)

	levelPos := meterPosition(levelDB, voiceActivationMinThresholdDB, meterMaxDB, meterWidth)
	thresholdPos := meterPosition(thresholdDB, voiceActivationMinThresholdDB, meterMaxDB, meterWidth)

	var meter strings.Builder
	for i := 0; i < meterWidth; i++ {
		switch {
		case i == thresholdPos:
			meter.WriteString(meterCutoffStyle.Render("|"))
		case i <= levelPos:
			meter.WriteString(meterFillStyle.Render("█"))
		default:
			meter.WriteString(mutedStyle.Render("░"))
		}
	}

	return strings.Join([]string{
		"Mic level:  [" + meter.String() + "]",
		mutedStyle.Render(fmt.Sprintf("           %.1f dBFS  cutoff %.1f dBFS", levelDB, thresholdDB)),
	}, "\n")
}

func (m *Model) ensureMicMonitor() error {
	if m.ActiveChannel != nil {
		m.stopMicMonitor()
		return nil
	}

	if m.AudioCaptureSelected < 0 || m.AudioCaptureSelected >= len(m.AudioCaptureDevices) {
		m.stopMicMonitor()
		return nil
	}

	selected := m.AudioCaptureDevices[m.AudioCaptureSelected]
	selectedName := selected.Name()
	if m.MicMonitor != nil && m.MicMonitor.DeviceName() == selectedName {
		return nil
	}

	m.stopMicMonitor()
	monitor, err := NewMicLevelMonitor(selected)
	if err != nil {
		return err
	}
	m.MicMonitor = monitor
	return nil
}

func (m *Model) stopMicMonitor() {
	if m.MicMonitor == nil {
		return
	}
	m.MicMonitor.Close()
	m.MicMonitor = nil
}

func (m *Model) startSelfTest() error {
	m.stopSelfTest()
	m.stopMicMonitor() // can't share the capture device

	if m.AudioCaptureSelected < 0 || m.AudioCaptureSelected >= len(m.AudioCaptureDevices) {
		return fmt.Errorf("select a capture device first")
	}
	if m.AudioPlaybackSelected < 0 || m.AudioPlaybackSelected >= len(m.AudioPlaybackDevices) {
		return fmt.Errorf("select a playback device first")
	}

	engine := NewAudioEngine()
	capture := m.AudioCaptureDevices[m.AudioCaptureSelected]
	playback := m.AudioPlaybackDevices[m.AudioPlaybackSelected]

	// Apply the same voice activation gate as the room session so DTLN artifacts
	// during silence don't leak through to the speaker. VA modifies PCM in-place,
	// so the loopback path automatically gets the gated output.
	va := NewVoiceActivation(
		audioSampleRate, audioFrameSamples,
		m.VoiceActivationThresholdDB,
		defaultVoiceActivationAttackMs,
		defaultVoiceActivationReleaseMs,
		defaultVoiceActivationHoldMs,
	)
	if err := engine.Start(capture, playback, func(pcm []byte) {
		va.ProcessPCM16LE(pcm)
	}, m.NoiseSuppressionEnabled); err != nil {
		return err
	}
	engine.SetMicGainDB(m.MicGainDB)
	engine.SetLoopback(true)
	m.SelfTest = engine
	return nil
}

func (m *Model) stopSelfTest() {
	if m.SelfTest == nil {
		return
	}
	m.SelfTest.Close()
	m.SelfTest = nil
}

func meterPosition(db, minDB, maxDB float64, width int) int {
	if width <= 1 {
		return 0
	}
	if db < minDB {
		db = minDB
	}
	if db > maxDB {
		db = maxDB
	}
	norm := (db - minDB) / (maxDB - minDB)
	pos := int(math.Round(norm * float64(width-1)))
	if pos < 0 {
		return 0
	}
	if pos >= width {
		return width - 1
	}
	return pos
}
