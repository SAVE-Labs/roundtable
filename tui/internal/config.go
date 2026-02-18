package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

type AppConfig struct {
	ServerName         string `json:"server_name,omitempty"`
	ServerHTTPURL      string `json:"server_http_url,omitempty"`
	ServerWSURL        string `json:"server_ws_url,omitempty"`
	CaptureDeviceName  string `json:"capture_device_name,omitempty"`
	PlaybackDeviceName string `json:"playback_device_name,omitempty"`
	MicMuted           bool   `json:"mic_muted,omitempty"`
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
		MicMuted: m.MicMuted,
	}

	if m.ServerSelected >= 0 && m.ServerSelected < len(m.Servers) {
		s := m.Servers[m.ServerSelected]
		cfg.ServerName = s.Name
		cfg.ServerHTTPURL = s.HTTPURL
		cfg.ServerWSURL = s.WSURL
	} else {
		if m.ServerURL != nil {
			cfg.ServerHTTPURL = m.ServerURL.String()
		}
		if m.WebsocketURL != nil {
			cfg.ServerWSURL = m.WebsocketURL.String()
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

func configFromSelectedServer(server ServerOption) AppConfig {
	return AppConfig{
		ServerName:    server.Name,
		ServerHTTPURL: server.HTTPURL,
		ServerWSURL:   server.WSURL,
	}
}

func validateConfigServer(cfg AppConfig) error {
	if cfg.ServerHTTPURL == "" || cfg.ServerWSURL == "" {
		return fmt.Errorf("server urls are incomplete")
	}
	return nil
}
