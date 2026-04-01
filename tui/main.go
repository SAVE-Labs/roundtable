package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SAVE-Labs/roundtable/tui/internal"
	tea "github.com/charmbracelet/bubbletea"
	hook "github.com/robotn/gohook"
)

func defaultLogPath() (string, error) {
	if path := os.Getenv("ROUNDTABLE_TUI_LOG_FILE"); path != "" {
		return path, nil
	}

	baseDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(baseDir, "roundtable", "tui.log"), nil
}

func startGlobalHotkeys(program *tea.Program) func() {
	if strings.EqualFold(os.Getenv("ROUNDTABLE_TUI_DISABLE_GLOBAL_HOTKEYS"), "1") {
		return func() {}
	}

	hook.Register(hook.KeyDown, []string{"m", "ctrl", "shift"}, func(hook.Event) {
		program.Send(internal.GlobalMuteToggleMsg{})
	})

	events := hook.Start()
	done := make(chan struct{})
	go func() {
		<-hook.Process(events)
		close(done)
	}()

	return func() {
		hook.End()
		<-done
	}
}

func main() {
	logPath, err := defaultLogPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not resolve log path: %v\n", err)
	} else {
		if mkErr := os.MkdirAll(filepath.Dir(logPath), 0o755); mkErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create log directory: %v\n", mkErr)
		} else {
			logFile, logErr := tea.LogToFile(logPath, "roundtable-tui")
			if logErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not enable log file %s: %v\n", logPath, logErr)
			} else {
				defer logFile.Close()
			}
		}
	}

	m := internal.New()

	p := tea.NewProgram(m, tea.WithAltScreen())
	stopHotkeys := startGlobalHotkeys(p)
	defer stopHotkeys()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
