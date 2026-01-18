package editor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// Editor represents an editor type
type Editor string

const (
	EditorCursor Editor = "cursor"
	EditorCode   Editor = "code"
)

// DetectEditor detects which editor is available
func DetectEditor() (Editor, error) {
	// Check for cursor first
	if _, err := exec.LookPath("cursor"); err == nil {
		return EditorCursor, nil
	}

	// Then check for code
	if _, err := exec.LookPath("code"); err == nil {
		return EditorCode, nil
	}

	return "", fmt.Errorf("no editor found (cursor or code)")
}

// OpenFile opens a file in the specified editor at the given line and column
func OpenFile(editor Editor, file string, line, column int) error {
	// Check if we're running inside editor or if existing instance exists
	hasExistingInstance, _ := findExistingInstance(editor)
	isInEditor := isRunningInEditor()

	// On macOS, try using URL scheme first if we're in the editor
	// This is more reliable for opening in existing instance
	if runtime.GOOS == "darwin" && (hasExistingInstance || isInEditor) && editor == EditorCursor {
		// Try using cursor:// URL scheme
		absPath, err := filepath.Abs(file)
		if err == nil {
			url := fmt.Sprintf("cursor://file/%s:%d:%d", absPath, line, column)
			cmd := exec.Command("open", "-u", url)
			cmd.Stdout = nil
			cmd.Stderr = nil

			// Try URL scheme first
			if err := cmd.Run(); err == nil {
				return nil
			}
			// If URL scheme fails, fall back to CLI
		}
	}

	// Fall back to CLI command
	editorCmd := string(editor)
	args := []string{
		"--goto",
		fmt.Sprintf("%s:%d:%d", file, line, column),
	}

	if hasExistingInstance || isInEditor {
		// Use --reuse-window to prevent new window from opening
		args = append([]string{"--reuse-window"}, args...)
	}

	cmd := exec.Command(editorCmd, args...)
	// Discard output to prevent any interference
	cmd.Stdout = nil
	cmd.Stderr = nil

	// Start the command in background to prevent blocking
	// This minimizes the chance of a new window flashing
	if err := cmd.Start(); err != nil {
		return err
	}

	// Don't wait for the command to complete - let it run in background
	// This prevents the TUI from blocking and reduces window flashing
	go cmd.Wait()

	return nil
}

// isRunningInEditor checks if the process is running inside Cursor or VS Code terminal
func isRunningInEditor() bool {
	// Check for IPC socket - most reliable indicator of existing instance
	ipcHook := os.Getenv("VSCODE_IPC_HOOK")
	if ipcHook != "" {
		// Check if IPC socket file exists
		if _, err := os.Stat(ipcHook); err == nil {
			return true
		}
		// Also check if it's a socket file pattern (even if file doesn't exist yet, the env var indicates we're in the editor)
		if strings.HasSuffix(ipcHook, ".sock") {
			return true
		}
	}

	// Check for Cursor/VS Code environment variables and verify the process exists
	cursorPID := os.Getenv("CURSOR_PID")
	vscodePID := os.Getenv("VSCODE_PID")

	if cursorPID != "" {
		if processExists(cursorPID) {
			return true
		}
	}

	if vscodePID != "" {
		if processExists(vscodePID) {
			return true
		}
	}

	// Check for Cursor agent (indicates we're running inside Cursor)
	if os.Getenv("CURSOR_AGENT") != "" {
		return true
	}

	// Check parent process name (macOS/Linux)
	if runtime.GOOS != "windows" {
		ppid := os.Getppid()
		if ppid > 0 {
			// Try to read parent process info
			cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", ppid), "-o", "comm=")
			output, err := cmd.Output()
			if err == nil {
				parentName := strings.TrimSpace(string(output))
				// Check if parent is Cursor or VS Code
				if contains(parentName, "Cursor") || contains(parentName, "cursor") ||
					contains(parentName, "Code") || contains(parentName, "code") {
					return true
				}
			}
		}
	}

	return false
}

// findExistingInstance attempts to find an existing Cursor/VS Code instance via IPC
func findExistingInstance(editor Editor) (bool, string) {
	// Check for IPC socket in environment
	ipcHook := os.Getenv("VSCODE_IPC_HOOK")
	if ipcHook != "" {
		// Verify socket file exists
		if info, err := os.Stat(ipcHook); err == nil {
			// Check if it's a socket file
			if info.Mode()&os.ModeSocket != 0 {
				return true, ipcHook
			}
		}
		// Even if file doesn't exist, the env var indicates we're in the editor
		if strings.HasSuffix(ipcHook, ".sock") {
			return true, ipcHook
		}
	}

	// Try to find IPC socket in common locations
	homeDir, err := os.UserHomeDir()
	if err == nil {
		var ipcPaths []string
		if editor == EditorCursor {
			ipcPaths = []string{
				filepath.Join(homeDir, "Library", "Application Support", "Cursor", "*.sock"),
				filepath.Join(homeDir, ".cursor", "*.sock"),
			}
		} else {
			ipcPaths = []string{
				filepath.Join(homeDir, "Library", "Application Support", "Code", "*.sock"),
				filepath.Join(homeDir, ".vscode", "*.sock"),
			}
		}

		for _, pattern := range ipcPaths {
			matches, err := filepath.Glob(pattern)
			if err == nil && len(matches) > 0 {
				return true, matches[0]
			}
		}
	}

	return false, ""
}

// processExists checks if a process with the given PID exists
func processExists(pidStr string) bool {
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return false
	}

	// On Unix systems, use kill(pid, 0) to check if process exists
	// Signal 0 doesn't kill the process, just checks if it exists
	if runtime.GOOS != "windows" {
		err := syscall.Kill(pid, 0)
		return err == nil
	}

	// On Windows, try to find the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process != nil
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
