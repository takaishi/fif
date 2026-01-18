package main

import (
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/takaishi/fif/config"
	"github.com/takaishi/fif/tui"
)

func main() {
	// Check if ripgrep is installed
	if _, err := exec.LookPath("rg"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: ripgrep (rg) is not installed or not in PATH\n")
		fmt.Fprintf(os.Stderr, "Please install ripgrep: https://github.com/BurntSushi/ripgrep\n")
		os.Exit(1)
	}

	// Parse flags and configuration
	cfg, err := config.ParseFlags()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Create and start TUI with Bubble Tea
	model := tui.New()
	model.SetEditor(cfg.Editor)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseAllMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
