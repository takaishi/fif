package search

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseVimgrepLine parses a single line of ripgrep vimgrep output
// Format: file:line:column:text
func ParseVimgrepLine(line string) (*SearchResult, error) {
	// Find the last colon before the text content
	// We need to split on colons, but the text part may contain colons
	parts := strings.SplitN(line, ":", 4)
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid vimgrep format: %s", line)
	}

	file := parts[0]
	lineNum, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid line number: %s", parts[1])
	}

	columnNum, err := strconv.Atoi(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid column number: %s", parts[2])
	}

	text := parts[3]

	return &SearchResult{
		File:   file,
		Line:   lineNum,
		Column: columnNum,
		Text:   text,
	}, nil
}

// ParseVimgrepOutput parses multiple lines of ripgrep vimgrep output
func ParseVimgrepOutput(output string) []*SearchResult {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	results := make([]*SearchResult, 0, len(lines))

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		result, err := ParseVimgrepLine(line)
		if err != nil {
			// Skip invalid lines
			continue
		}
		results = append(results, result)
	}

	return results
}
