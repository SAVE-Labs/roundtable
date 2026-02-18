package internal

import (
	"fmt"
	"net/url"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	// Color scheme
	primaryColor   = lipgloss.Color("205") // Pink
	secondaryColor = lipgloss.Color("86")  // Cyan
	accentColor    = lipgloss.Color("220") // Yellow/Gold
	mutedColor     = lipgloss.Color("241") // Gray
	errorColor     = lipgloss.Color("196") // Red

	// Styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			MarginBottom(1)

	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
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

	m.AudioCaptureName = cfg.CaptureDeviceName
	m.AudioPlaybackName = cfg.PlaybackDeviceName

	if cfg.ServerHTTPURL != "" && cfg.ServerWSURL != "" {
		httpURL, err := url.Parse(cfg.ServerHTTPURL)
		if err != nil {
			m.AudioErr = err.Error()
		} else {
			wsURL, wsErr := url.Parse(cfg.ServerWSURL)
			if wsErr != nil {
				m.AudioErr = wsErr.Error()
			} else {
				m.ServerURL = httpURL
				m.WebsocketURL = wsURL
				match := -1
				for i, server := range m.Servers {
					if server.HTTPURL == cfg.ServerHTTPURL && server.WSURL == cfg.ServerWSURL {
						match = i
						break
					}
				}
				if match == -1 {
					name := cfg.ServerName
					if strings.TrimSpace(name) == "" {
						name = cfg.ServerHTTPURL
					}
					m.Servers = append(m.Servers, ServerOption{Name: name, HTTPURL: cfg.ServerHTTPURL, WSURL: cfg.ServerWSURL})
					match = len(m.Servers) - 1
				}
				m.ServerSelected = match
				m.ServerCursor = match
			}
		}
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
		m.AudioErr = msg.Err.Error()
		m.SessionStatus = "Create room failed"
		return m
	}

	m.Channels = append(m.Channels, msg.Channel)
	m.Cursor = len(m.Channels) - 1
	m.AudioErr = ""
	m.SessionStatus = "Created room " + msg.Channel.Name
	return m
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
			if m.ServerURL == nil {
				m.AudioErr = "server url not configured"
				m.SessionStatus = "Create room failed"
				return m, nil
			}
			name := fmt.Sprintf("Room %d", len(m.Channels)+1)
			return m, CreateRoomCmd(m.ServerURL.String(), name)
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

func handleServerKeys(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
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
		httpURL, err := url.Parse(selected.HTTPURL)
		if err != nil {
			m.AudioErr = err.Error()
			m.SessionStatus = "Server selection failed"
			return m, nil
		}
		wsURL, err := url.Parse(selected.WSURL)
		if err != nil {
			m.AudioErr = err.Error()
			m.SessionStatus = "Server selection failed"
			return m, nil
		}

		m.leaveChannel()
		m.ServerURL = httpURL
		m.WebsocketURL = wsURL
		m.ServerSelected = m.ServerCursor
		m.AudioErr = ""
		m.SessionStatus = "Using server " + selected.Name
		return m, tea.Batch(
			LoadRoomsCmd(m.ServerURL.String()),
			SaveConfigCmd(m.ConfigSnapshot()),
		)
	}

	return m, nil
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
	if m.AudioCaptureSelected < 0 || m.AudioCaptureSelected >= len(m.AudioCaptureDevices) {
		return fmt.Errorf("select a capture device first")
	}
	if m.AudioPlaybackSelected < 0 || m.AudioPlaybackSelected >= len(m.AudioPlaybackDevices) {
		return fmt.Errorf("select a playback device first")
	}
	if m.WebsocketURL == nil {
		return fmt.Errorf("websocket url not configured")
	}

	m.leaveChannel()

	roomWSURL, err := websocketURLForRoom(m.WebsocketURL, selectedChannel.ID)
	if err != nil {
		return err
	}

	engine := NewAudioEngine()
	client, err := NewWebRTCClient(roomWSURL, func(pcm []byte) {
		engine.PushPCM16LE(pcm)
	})
	if err != nil {
		return err
	}

	capture := m.AudioCaptureDevices[m.AudioCaptureSelected]
	playback := m.AudioPlaybackDevices[m.AudioPlaybackSelected]
	if err := engine.Start(capture, playback, func(pcm []byte) {
		_ = client.SendPCM16LE(pcm)
	}); err != nil {
		client.Close()
		return err
	}

	m.WebRTCClient = client
	m.AudioEngine = engine
	m.WebRTCClient.SetMuted(m.MicMuted)
	m.ActiveChannel = &selectedChannel
	m.AudioErr = ""
	m.SessionStatus = "Connected to " + selectedChannel.Name
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
	} else if m.Tab == TabServers {
		content = renderServers(m)
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
	b.WriteString(helpStyle.Render("↑/↓ or j/k to move • space/enter join • n create room • r reload • m mute"))
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

func renderServers(m Model) string {
	var b strings.Builder

	b.WriteString(sectionTitleStyle.Render("Servers"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ or j/k to move • space/enter to select"))
	b.WriteString("\n\n")

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

func renderAudio(m Model) string {
	var b strings.Builder

	b.WriteString(sectionTitleStyle.Render("Audio Devices"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("c=capture • p=playback • ↑/↓ to move • space to select • r to reload"))
	b.WriteString("\n\n")

	if m.AudioErr != "" {
		b.WriteString(errorStyle.Render("⚠ Error: " + m.AudioErr))
		b.WriteString("\n\n")
	}

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
