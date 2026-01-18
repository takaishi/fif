package config

import (
	"flag"
	"os"
	"github.com/takaishi/fif/editor"
)

// Config holds application configuration
type Config struct {
	Editor editor.Editor
}

// ParseFlags parses command line flags and returns configuration
func ParseFlags() (*Config, error) {
	editorFlag := flag.String("editor", "", "Editor to use (cursor or code)")
	flag.Parse()

	cfg := &Config{}

	// Determine editor
	if *editorFlag != "" {
		cfg.Editor = editor.Editor(*editorFlag)
	} else if envEditor := getEnvEditor(); envEditor != "" {
		cfg.Editor = editor.Editor(envEditor)
	} else {
		// Auto-detect
		ed, err := editor.DetectEditor()
		if err != nil {
			return nil, err
		}
		cfg.Editor = ed
	}

	return cfg, nil
}

// getEnvEditor gets editor from FIF_EDITOR environment variable
func getEnvEditor() string {
	return os.Getenv("FIF_EDITOR")
}
