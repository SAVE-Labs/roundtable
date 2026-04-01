package internal

import (
	"encoding/json"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

type ServerConfig struct {
	Name    string `json:"name,omitempty"`
	HTTPURL string `json:"http_url,omitempty"`
	WSURL   string `json:"ws_url,omitempty"`
}

type AppConfig struct {
	Version            int            `json:"version,omitempty"`
	CaptureDeviceName  string         `json:"capture_device_name,omitempty"`
	PlaybackDeviceName string         `json:"playback_device_name,omitempty"`
	MicMuted           bool           `json:"mic_muted,omitempty"`
	VoiceActivationThreshold *float64 `json:"voice_activation_threshold_db,omitempty"`
	LastUsedServer     ServerConfig   `json:"last_used_server,omitempty"`
	Servers            []ServerConfig `json:"servers,omitempty"`
}

type ConfigLoadedMsg struct {
	Config AppConfig
	Err    error
}

type ConfigSavedMsg struct {
	Err error
}

func configFilePath() (string, error) {
	baseDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "roundtable", "tui.json"), nil
}

func LoadConfigCmd() tea.Cmd {
	return func() tea.Msg {
		path, err := configFilePath()
		if err != nil {
			return ConfigLoadedMsg{Err: err}
		}

		file, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				return ConfigLoadedMsg{}
			}
			return ConfigLoadedMsg{Err: err}
		}
		defer file.Close()

		// TODO: handle config versioning and migration. For now, we just ignore the version field.
		var cfg AppConfig
		if err := json.NewDecoder(file).Decode(&cfg); err != nil {
			return ConfigLoadedMsg{Err: err}
		}

		return ConfigLoadedMsg{Config: cfg}
	}
}

func SaveConfigCmd(cfg AppConfig) tea.Cmd {
	return func() tea.Msg {
		path, err := configFilePath()
		if err != nil {
			return ConfigSavedMsg{Err: err}
		}

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return ConfigSavedMsg{Err: err}
		}

		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return ConfigSavedMsg{Err: err}
		}

		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			return ConfigSavedMsg{Err: err}
		}

		return ConfigSavedMsg{}
	}
}

func (m Model) ConfigSnapshot() AppConfig {
	cfg := AppConfig{
		Version:  2,
		MicMuted: m.MicMuted,
	}

	voiceActivationThreshold := m.VoiceActivationThresholdDB
	cfg.VoiceActivationThreshold = &voiceActivationThreshold

	if m.ServerSelected >= 0 && m.ServerSelected < len(m.Servers) {
		s := m.Servers[m.ServerSelected]
		cfg.LastUsedServer = ServerConfig{
			Name:    s.Name,
			HTTPURL: s.HTTPURL,
			WSURL:   s.WSURL,
		}
	} else {
		cfg.LastUsedServer = ServerConfig{}
	}

	cfg.Servers = make([]ServerConfig, len(m.Servers))
	for i, s := range m.Servers {
		cfg.Servers[i] = ServerConfig{
			Name:    s.Name,
			HTTPURL: s.HTTPURL,
			WSURL:   s.WSURL,
		}
	}

	if m.AudioCaptureSelected >= 0 && m.AudioCaptureSelected < len(m.AudioCaptureDevices) {
		cfg.CaptureDeviceName = m.AudioCaptureDevices[m.AudioCaptureSelected].Name()
	}
	if m.AudioPlaybackSelected >= 0 && m.AudioPlaybackSelected < len(m.AudioPlaybackDevices) {
		cfg.PlaybackDeviceName = m.AudioPlaybackDevices[m.AudioPlaybackSelected].Name()
	}

	return cfg
}
