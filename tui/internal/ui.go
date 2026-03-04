package internal

import (
	"fmt"
	"log"
	"net/url"
	"strings"

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
)

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

	if m.SessionStatus == "Not connected" || m.SessionStatus == "No rooms available" {
		m.SessionStatus = "Rooms loaded"
	}

	return m
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
	m.ServerURL = httpURL
	m.WebsocketURL = wsURL
	m.ServerErr = ""
	m.ServerFormErr = ""
	m.SessionStatus = "Using server " + selected.Name

	return m, tea.Batch(
		LoadRoomsCmd(m.ServerURL.String()),
		SaveConfigCmd(m.ConfigSnapshot()),
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
	}
	return m, nil
}

func handleKeyPress(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
			return m, LoadRoomsCmd(m.ServerURL.String())
		}

	case "n":
		if m.Tab == TabChannels {
			m.RoomFormOpen = true
			m.RoomFormName = ""
			m.RoomFormErr = ""
			return m, nil
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

	roomWSURL, err := websocketURLForRoom(m.WebsocketURL, selectedChannel.ID)
	if err != nil {
		log.Printf("join: websocket url build failed room_id=%s err=%v", selectedChannel.ID, err)
		return err
	}
	log.Printf("join: websocket url=%s", roomWSURL)

	engine := NewAudioEngine()
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
		_ = client.SendPCM16LE(pcm)
	}); err != nil {
		log.Printf("join: audio start failed capture=%s playback=%s err=%v", capture.Name(), playback.Name(), err)
		client.Close()
		return err
	}

	m.WebRTCClient = client
	m.AudioEngine = engine
	m.WebRTCClient.SetMuted(m.MicMuted)
	m.ActiveChannel = &selectedChannel
	m.AudioErr = ""
	m.SessionStatus = "Connected to " + selectedChannel.Name
	log.Printf("join: connected room_id=%s room_name=%s", selectedChannel.ID, selectedChannel.Name)
	return nil
}

func websocketURLForRoom(base *url.URL, roomID string) (string, error) {
	if base == nil {
		return "", fmt.Errorf("websocket url not configured")
	}
	if strings.TrimSpace(roomID) == "" {
		return "", fmt.Errorf("room id is empty")
	}

	u := *base
	q := u.Query()
	q.Set("room", roomID)
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
	m.ActiveChannel = nil
	m.SessionStatus = "Not connected"
}

func handleAudioKeys(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "c":
		m.AudioFocus = AudioFocusCapture
	case "p":
		m.AudioFocus = AudioFocusPlayback
	case "up", "k":
		if m.AudioFocus == AudioFocusCapture {
			if m.AudioCaptureCursor > 0 {
				m.AudioCaptureCursor--
			}
		} else {
			if m.AudioPlaybackCursor > 0 {
				m.AudioPlaybackCursor--
			}
		}
	case "down", "j":
		if m.AudioFocus == AudioFocusCapture {
			if m.AudioCaptureCursor < len(m.AudioCaptureDevices)-1 {
				m.AudioCaptureCursor++
			}
		} else {
			if m.AudioPlaybackCursor < len(m.AudioPlaybackDevices)-1 {
				m.AudioPlaybackCursor++
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
			return m, SaveConfigCmd(m.ConfigSnapshot())
		}
	}
	return m, nil
}

func View(m Model) string {
	var b strings.Builder

	// Header
	b.WriteString(titleStyle.Render("🎙️  Roundtable Audio Chat"))
	b.WriteString("\n\n")

	renderTabs(&b, m)
	b.WriteString("\n")

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
	} else {
		content = renderAudio(m)
	}
	b.WriteString(boxStyle.Render(content))

	// Footer help text
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Press q to quit • tab to switch tabs"))
	b.WriteString("\n")

	return b.String()
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

		active := " "
		if m.ActiveChannel != nil && m.ActiveChannel.ID == ch.ID {
			active = selectedStyle.Render("● ")
		} else {
			active = mutedStyle.Render("○ ")
		}

		name := ch.Name
		if m.Cursor == i {
			name = selectedStyle.Render(name)
		}

		b.WriteString(fmt.Sprintf("%s%s%s\n", cursor, active, name))
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

func renderServers(m Model) string {
	var b strings.Builder

	b.WriteString(sectionTitleStyle.Render("Servers"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ or j/k move • space/enter select • a add • d delete"))
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
	b.WriteString(helpStyle.Render("c=capture • p=playback • ↑/↓ to move • space to select • r to reload"))
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
