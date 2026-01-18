package search

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FindGitRoot finds the git repository root directory starting from the given path
func FindGitRoot(startPath string) (string, bool) {
	path := startPath
	for {
		gitDir := filepath.Join(path, ".git")
		if info, err := os.Stat(gitDir); err == nil {
			if info.IsDir() {
				return path, true
			}
		}

		// Check if we've reached the filesystem root
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}

	return "", false
}

// GetCurrentGitRoot finds the git repository root from the current working directory
func GetCurrentGitRoot() (string, bool) {
	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	return FindGitRoot(wd)
}

// IsGitRepository checks if the given path is within a git repository
func IsGitRepository(path string) bool {
	_, ok := FindGitRoot(path)
	return ok
}

// GetGitRootFromCommand uses git command to find repository root
func GetGitRootFromCommand() (string, bool) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", false
	}
	return root, true
}
