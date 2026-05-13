package services

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type ServerConfig struct {
	Name    string `json:"name,omitempty"`
	HTTPURL string `json:"http_url,omitempty"`
	WSURL   string `json:"ws_url,omitempty"`
}

type AppConfig struct {
	Version                  int            `json:"version,omitempty"`
	DisplayName              string         `json:"display_name,omitempty"`
	CaptureDeviceName        string         `json:"capture_device_name,omitempty"`
	PlaybackDeviceName       string         `json:"playback_device_name,omitempty"`
	MicMuted                 bool           `json:"mic_muted,omitempty"`
	VoiceActivationThreshold *float64       `json:"voice_activation_threshold_db,omitempty"`
	MicGainDB                *float64       `json:"mic_gain_db,omitempty"`
	LastUsedServer           ServerConfig   `json:"last_used_server,omitempty"`
	Servers                  []ServerConfig `json:"servers,omitempty"`
}

type ConfigService struct{}

func NewConfigService() *ConfigService {
	return &ConfigService{}
}

func (s *ConfigService) Load() (AppConfig, error) {
	path, err := configPath()
	if err != nil {
		return AppConfig{}, err
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return AppConfig{}, nil
		}
		return AppConfig{}, err
	}
	defer f.Close()

	var cfg AppConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return AppConfig{}, err
	}
	return cfg, nil
}

func (s *ConfigService) Save(cfg AppConfig) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func configPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "roundtable", "desktop.json"), nil
}
