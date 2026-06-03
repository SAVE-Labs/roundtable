package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/SAVE-Labs/roundtable/tui/internal"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gen2brain/malgo"
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

// smokeTest verifies the CGO dependencies (DeepFilterNet + audio) load correctly
// and exits. Used in CI after building to catch missing libraries or arch issues.
func smokeTest() {
	ok := true

	df, err := internal.NewDFEngine()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL deepfilter: %v\n", err)
		ok = false
	} else {
		df.Close()
		fmt.Println("OK   deepfilter")
	}

	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL audio context: %v\n", err)
		ok = false
	} else {
		capture, err := ctx.Devices(malgo.Capture)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL list capture devices: %v\n", err)
			ok = false
		} else {
			fmt.Printf("OK   audio: %d capture device(s)\n", len(capture))
		}
		ctx.Uninit()
		ctx.Free()
	}

	if !ok {
		os.Exit(1)
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--smoke-test" {
		smokeTest()
		return
	}

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
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
