package internal

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gen2brain/malgo"
)

type DevicesMsg struct {
	Capture  []malgo.DeviceInfo
	Playback []malgo.DeviceInfo
	Err      error
}

func LoadDevicesCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
		if err != nil {
			return DevicesMsg{Err: err}
		}
		defer ctx.Uninit()

		capture, err := ctx.Devices(malgo.Capture)
		if err != nil {
			return DevicesMsg{Err: err}
		}
		playback, err := ctx.Devices(malgo.Playback)
		if err != nil {
			return DevicesMsg{Err: err}
		}

		return DevicesMsg{
			Capture:  capture,
			Playback: playback,
		}
	}
}
