package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/takaishi/fif/search"
)

var (
	// Header styles
	headerStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	searchIconStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Bold(true)

	queryInputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("236"))

	maskLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	scopeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("62")).
			Bold(true).
			Padding(0, 1)

	scopeInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Padding(0, 1)

	// Result styles
	resultStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	selectedResultStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(lipgloss.Color("25")).
				Bold(false)

	highlightStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Background(lipgloss.Color("236")).
			Bold(true)

	fileInfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Align(lipgloss.Right).
			PaddingLeft(1)

	// Preview styles
	previewHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Bold(true)

	previewStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	lineNumberStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Width(6).
			Align(lipgloss.Right)

	hitLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("25")).
			Bold(false)

	hitLineNumberStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(lipgloss.Color("25")).
				Width(6).
				Align(lipgloss.Right)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
)

// renderView renders the entire UI
func renderView(m *Model) string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	// Calculate layout heights
	headerHeight := 3
	statusHeight := 1
	const resultsHeight = 5 // Fixed height for results list
	previewHeight := m.height - headerHeight - statusHeight - resultsHeight - 2
	if previewHeight < 5 {
		previewHeight = 5
	}

	var sections []string

	// Header: Search bar with icon, query, mask, and status
	header := renderHeader(m)
	sections = append(sections, header)

	// Results section
	results := renderResults(m, resultsHeight)
	sections = append(sections, results)

	// Preview section
	preview := renderPreview(m, previewHeight)
	sections = append(sections, preview)

	// Join all sections
	content := lipgloss.JoinVertical(lipgloss.Left, sections...)

	return content
}

// renderHeader renders the search bar with icon, query, mask, and status
func renderHeader(m *Model) string {
	// Search icon
	icon := searchIconStyle.Render("🔍")

	// Query input
	queryValue := m.queryInput.value
	if m.inputMode == InputModeQuery {
		queryValue += "█" // Cursor indicator
	}
	queryDisplay := queryInputStyle.Render(queryValue)

	// File mask with checkbox
	checkbox := "[ ]"
	if m.maskEnabled {
		checkbox = "[x]"
	}
	maskLabel := maskLabelStyle.Render(fmt.Sprintf("%s File mask:", checkbox))
	maskValue := m.maskInput.value
	if maskValue == "" {
		maskValue = "*"
	}
	if m.inputMode == InputModeMask {
		maskValue += "█" // Cursor indicator
	}
	maskDisplay := maskLabelStyle.Render(fmt.Sprintf("%s %s", maskLabel, maskValue))

	// Search scope tabs (In Project / In Directory)
	var projectTab, directoryTab string
	if m.searchScope == "project" {
		projectTab = scopeStyle.Render("In Project")
		directoryTab = scopeInactiveStyle.Render("In Directory")
	} else {
		projectTab = scopeInactiveStyle.Render("In Project")
		directoryTab = scopeStyle.Render("In Directory")
	}

	// Only show project tab if git repository is detected
	scopeTabs := directoryTab
	if m.gitRoot != "" {
		scopeTabs = lipgloss.JoinHorizontal(lipgloss.Left, projectTab, " ", directoryTab)
	}

	// Build header line
	headerLine := lipgloss.JoinHorizontal(lipgloss.Left,
		icon+" ",
		queryDisplay,
		"  ",
		maskDisplay,
		"  ",
		scopeTabs,
	)

	// Status line
	status := renderStatus(m)
	statusLine := statusStyle.Render(status)

	// Combine
	header := lipgloss.JoinVertical(lipgloss.Left, headerLine, statusLine)
	return headerStyle.Width(m.width - 2).Render(header)
}

// renderStatus renders the status information
func renderStatus(m *Model) string {
	if m.isSearching {
		return "Searching..."
	}
	if m.searchError != nil {
		return fmt.Sprintf("Error: %s", m.searchError.Error())
	}
	if len(m.searchResults) == 0 {
		if m.query == "" {
			return "Enter a search query..."
		}
		return "No matches found"
	}

	// Count unique files
	fileMap := make(map[string]bool)
	for _, result := range m.searchResults {
		fileMap[result.File] = true
	}
	fileCount := len(fileMap)
	matchCount := len(m.searchResults)

	if fileCount == 1 {
		return fmt.Sprintf("Find in Files %d match in 1 file", matchCount)
	}
	return fmt.Sprintf("Find in Files %d matches in %d files", matchCount, fileCount)
}

// renderResults renders the search results list
func renderResults(m *Model, maxHeight int) string {
	if len(m.searchResults) == 0 {
		if m.query == "" {
			return ""
		}
		return "No results found"
	}

	const visibleResults = 5
	availableWidth := m.width - 4 // Reserve space for borders

	// Calculate which results to display based on scroll offset
	startIdx := m.resultsOffset
	endIdx := startIdx + visibleResults
	if endIdx > len(m.searchResults) {
		endIdx = len(m.searchResults)
	}

	var lines []string
	for i := startIdx; i < endIdx; i++ {
		result := m.searchResults[i]
		// Format result with 2-column layout: code snippet | file:line
		line := formatResultJetBrains(m, result, availableWidth)

		if i == m.selectedIndex {
			line = selectedResultStyle.Render(line)
		} else {
			line = resultStyle.Render(line)
		}
		lines = append(lines, line)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// formatResultJetBrains formats a result in JetBrains style: code snippet | file:line
func formatResultJetBrains(m *Model, result *search.SearchResult, width int) string {
	// Extract filename from path
	fileParts := strings.Split(result.File, "/")
	fileName := fileParts[len(fileParts)-1]
	fileInfo := fmt.Sprintf("%s %d", fileName, result.Line)

	// Reserve space for file info on the right (minimum 25 chars for filename + line number)
	fileInfoAreaWidth := 30
	if fileInfoAreaWidth > width/3 {
		fileInfoAreaWidth = width / 3
	}
	if fileInfoAreaWidth < 25 {
		fileInfoAreaWidth = 25
	}

	codeWidth := width - fileInfoAreaWidth
	if codeWidth < 10 {
		codeWidth = 10
		fileInfoAreaWidth = width - codeWidth
	}

	// Format code snippet with query highlight (left-aligned, fixed width)
	codeSnippet := highlightQuery(m.query, result.Text, codeWidth)
	// Ensure code snippet doesn't exceed its allocated width
	codeSnippetStyled := lipgloss.NewStyle().Width(codeWidth).Render(codeSnippet)

	// Format file info (right-aligned within its area, then placed at right edge)
	fileInfoFormatted := fileInfoStyle.Width(fileInfoAreaWidth).Align(lipgloss.Right).Render(fileInfo)

	// Combine: code snippet (left, fixed width) + file info (right, fixed width)
	// This ensures file info is always at the right edge
	resultLine := lipgloss.JoinHorizontal(lipgloss.Left, codeSnippetStyled, fileInfoFormatted)

	// Ensure the entire line is exactly the specified width
	resultLineStyled := lipgloss.NewStyle().Width(width).Render(resultLine)

	return resultLineStyled
}

// highlightQuery highlights the search query in the text
func highlightQuery(query, text string, maxWidth int) string {
	if query == "" {
		// Truncate if needed
		if len(text) > maxWidth {
			return text[:maxWidth-3] + "..."
		}
		return text
	}

	// Simple case-insensitive highlighting
	queryLower := strings.ToLower(query)
	textLower := strings.ToLower(text)

	// Find all occurrences
	var parts []string
	lastIndex := 0
	searchIndex := 0

	for {
		idx := strings.Index(textLower[searchIndex:], queryLower)
		if idx == -1 {
			break
		}
		actualIdx := searchIndex + idx

		// Add text before match
		if actualIdx > lastIndex {
			parts = append(parts, text[lastIndex:actualIdx])
		}

		// Add highlighted match
		matchText := text[actualIdx : actualIdx+len(query)]
		parts = append(parts, highlightStyle.Render(matchText))

		lastIndex = actualIdx + len(query)
		searchIndex = lastIndex
	}

	// Add remaining text
	if lastIndex < len(text) {
		parts = append(parts, text[lastIndex:])
	}

	// Join parts
	highlighted := strings.Join(parts, "")

	// Truncate if needed
	if len(highlighted) > maxWidth {
		// Try to truncate while preserving ANSI codes
		truncated := highlighted
		// Simple truncation (could be improved to handle ANSI codes properly)
		visibleLen := 0
		inAnsi := false
		for i, r := range highlighted {
			if r == '\x1b' {
				inAnsi = true
			} else if inAnsi && r == 'm' {
				inAnsi = false
			} else if !inAnsi {
				visibleLen++
				if visibleLen >= maxWidth-3 {
					truncated = highlighted[:i] + "..."
					break
				}
			}
		}
		return truncated
	}

	return highlighted
}

// renderPreview renders the code preview
func renderPreview(m *Model, maxHeight int) string {
	if m.previewError != nil {
		return errorStyle.Render("Error loading preview: " + m.previewError.Error())
	}

	if m.preview == nil {
		return ""
	}

	// Preview header with file path
	filePath := m.preview.File
	header := previewHeaderStyle.Render(filePath)

	var lines []string
	lines = append(lines, header)

	// Code lines
	availableWidth := m.width - 10 // Reserve space for line numbers and borders
	for i, line := range m.preview.Lines {
		if len(lines) >= maxHeight-1 {
			break
		}

		lineNum := m.preview.StartLine + i
		lineNumStr := fmt.Sprintf("%4d", lineNum)

		// Highlight the hit line
		if i+1 == m.preview.HitLine {
			lineNumStr = hitLineNumberStyle.Render(lineNumStr)
			// Highlight query in the hit line
			line = highlightQueryInPreview(m.query, line, availableWidth)
			line = hitLineStyle.Render(line)
		} else {
			lineNumStr = lineNumberStyle.Render(lineNumStr)
			// Truncate long lines
			if len(line) > availableWidth {
				line = line[:availableWidth-3] + "..."
			}
		}

		lines = append(lines, fmt.Sprintf("%s | %s", lineNumStr, line))
	}

	previewContent := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return previewStyle.Width(m.width - 2).Render(previewContent)
}

// highlightQueryInPreview highlights query in preview line
func highlightQueryInPreview(query, line string, maxWidth int) string {
	if query == "" {
		if len(line) > maxWidth {
			return line[:maxWidth-3] + "..."
		}
		return line
	}
	return highlightQuery(query, line, maxWidth)
}
