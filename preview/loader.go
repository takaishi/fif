package preview

import (
	"bufio"
	"fmt"
	"os"
)

const (
	previewBefore = 5
	previewAfter  = 10
)

// LoadPreview loads a preview for the given file and line number
func LoadPreview(file string, lineNum int) (*Preview, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	allLines := make([]string, 0)
	lineNumInFile := 1

	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
		lineNumInFile++
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Calculate preview range
	startLine := lineNum - previewBefore
	if startLine < 1 {
		startLine = 1
	}

	endLine := lineNum + previewAfter
	if endLine > len(allLines) {
		endLine = len(allLines)
	}

	// Extract preview lines (1-based to 0-based conversion)
	previewLines := make([]string, 0, endLine-startLine+1)
	for i := startLine - 1; i < endLine; i++ {
		if i >= 0 && i < len(allLines) {
			previewLines = append(previewLines, allLines[i])
		}
	}

	// Calculate hit line relative to preview start
	hitLineInPreview := lineNum - startLine + 1

	return &Preview{
		File:      file,
		StartLine: startLine,
		Lines:     previewLines,
		HitLine:   hitLineInPreview,
	}, nil
}
