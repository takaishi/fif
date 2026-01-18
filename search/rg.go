package search

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Searcher handles ripgrep search execution
type Searcher struct {
	searchID int64
}

// NewSearcher creates a new Searcher instance
func NewSearcher() *Searcher {
	return &Searcher{}
}

// SearchResultMsg is sent when search results are available
type SearchResultMsg struct {
	SearchID int64
	Results  []*SearchResult
	Error    error
}

// Search executes a ripgrep search with the given query and glob pattern
// It returns a channel that will receive search results as they come in
// searchPath specifies the directory to search in (empty means current directory)
func (s *Searcher) Search(ctx context.Context, query, globPattern, searchPath string) <-chan SearchResultMsg {
	s.searchID++
	currentID := s.searchID
	resultChan := make(chan SearchResultMsg, 1)

	go func() {
		defer close(resultChan)

		// Build ripgrep command
		args := []string{
			"--vimgrep",
			"--no-heading",
			"--color=never",
		}

		if globPattern != "" {
			args = append(args, "--glob", globPattern)
		}

		args = append(args, query)

		// Set search path (directory to search in)
		// If empty, ripgrep will search from current directory
		var cmd *exec.Cmd
		if searchPath != "" {
			cmd = exec.CommandContext(ctx, "rg", args...)
			cmd.Dir = searchPath
		} else {
			cmd = exec.CommandContext(ctx, "rg", args...)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			resultChan <- SearchResultMsg{
				SearchID: currentID,
				Error:    fmt.Errorf("failed to create stdout pipe: %w", err),
			}
			return
		}

		if err := cmd.Start(); err != nil {
			resultChan <- SearchResultMsg{
				SearchID: currentID,
				Error:    fmt.Errorf("failed to start ripgrep: %w", err),
			}
			return
		}

		// Read output line by line
		scanner := bufio.NewScanner(stdout)
		results := make([]*SearchResult, 0)

		for scanner.Scan() {
			// Check if context was cancelled
			select {
			case <-ctx.Done():
				cmd.Process.Kill()
				cmd.Wait()
				return
			default:
			}

			line := scanner.Text()
			result, err := ParseVimgrepLine(line)
			if err != nil {
				// Skip invalid lines
				continue
			}
			results = append(results, result)
		}

		if err := scanner.Err(); err != nil {
			resultChan <- SearchResultMsg{
				SearchID: currentID,
				Error:    fmt.Errorf("failed to read output: %w", err),
			}
			return
		}

		if err := cmd.Wait(); err != nil {
			// ripgrep returns non-zero exit code when no matches found
			// This is not an error, just empty results
			if strings.Contains(err.Error(), "exit status 1") {
				resultChan <- SearchResultMsg{
					SearchID: currentID,
					Results:  []*SearchResult{},
				}
				return
			}
			resultChan <- SearchResultMsg{
				SearchID: currentID,
				Error:    fmt.Errorf("ripgrep failed: %w", err),
			}
			return
		}

		resultChan <- SearchResultMsg{
			SearchID: currentID,
			Results:  results,
		}
	}()

	return resultChan
}
