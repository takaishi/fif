package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/takaishi/fif/editor"
	"github.com/takaishi/fif/preview"
	"github.com/takaishi/fif/search"
)

const (
	debounceDuration = 250 * time.Millisecond
)

// InputMode represents which input field is active
type InputMode int

const (
	InputModeQuery InputMode = iota
	InputModeMask
)

// Model represents the application state
type Model struct {
	// Input fields
	query      string
	mask       string
	inputMode  InputMode
	queryInput textInput
	maskInput  textInput

	// Search state
	searcher      *search.Searcher
	searchCancel  context.CancelFunc
	searchResults []*search.SearchResult
	selectedIndex int
	resultsOffset int // Scroll offset for results list
	isSearching   bool
	searchError   error

	// Preview state
	preview      *preview.Preview
	previewError error

	// Editor
	editor editor.Editor

	// UI dimensions
	width  int
	height int
}

// textInput represents a simple text input field
type textInput struct {
	value string
}

// New creates a new Model instance
func New() *Model {
	ed, _ := editor.DetectEditor()
	return &Model{
		searcher:      search.NewSearcher(),
		editor:        ed,
		inputMode:     InputModeQuery,
		selectedIndex: -1,
	}
}

// SetEditor sets the editor to use
func (m *Model) SetEditor(ed editor.Editor) {
	m.editor = ed
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update handles messages and updates the model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case search.SearchResultMsg:
		return m.handleSearchResult(msg)

	case previewLoadedMsg:
		return m.handlePreviewLoaded(msg)

	case startSearchMsg:
		return m.handleStartSearch(msg)

	default:
		return m, nil
	}
}

// View renders the UI
func (m *Model) View() string {
	return renderView(m)
}

// Start starts the Bubble Tea program
func (m *Model) Start() error {
	program := tea.NewProgram(m, tea.WithAltScreen())
	_, err := program.Run()
	return err
}

// handleKey processes keyboard input
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		if m.searchCancel != nil {
			m.searchCancel()
		}
		return m, tea.Quit

	case "tab":
		// Switch between query and mask input
		if m.inputMode == InputModeQuery {
			m.inputMode = InputModeMask
		} else {
			m.inputMode = InputModeQuery
		}
		return m, nil

	case "up", "k":
		if m.selectedIndex > 0 {
			m.selectedIndex--
			m.adjustScroll()
			return m, m.loadPreview()
		}
		return m, nil

	case "down", "j":
		if m.selectedIndex < len(m.searchResults)-1 {
			m.selectedIndex++
			m.adjustScroll()
			return m, m.loadPreview()
		}
		return m, nil

	case "enter":
		if m.selectedIndex >= 0 && m.selectedIndex < len(m.searchResults) {
			result := m.searchResults[m.selectedIndex]
			if err := editor.OpenFile(m.editor, result.File, result.Line, result.Column); err != nil {
				// Error opening editor - could show a message, but for now just continue
			}
			return m, tea.Quit
		}
		return m, nil

	default:
		// Handle text input
		return m.handleTextInput(msg)
	}
}

// handleTextInput processes text input for query and mask fields
func (m *Model) handleTextInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var input *textInput
	if m.inputMode == InputModeQuery {
		input = &m.queryInput
	} else {
		input = &m.maskInput
	}

	switch msg.String() {
	case "backspace":
		if len(input.value) > 0 {
			input.value = input.value[:len(input.value)-1]
		}
	case " ":
		input.value += " "
	default:
		if len(msg.Runes) > 0 {
			input.value += string(msg.Runes)
		}
	}

	// Update the corresponding field
	if m.inputMode == InputModeQuery {
		m.query = input.value
	} else {
		m.mask = input.value
	}

	// Trigger search with debounce
	return m, m.triggerSearch()
}

// triggerSearch starts a new search with debounce
func (m *Model) triggerSearch() tea.Cmd {
	// Cancel previous search if any
	if m.searchCancel != nil {
		m.searchCancel()
	}

	// Reset selection and scroll
	m.selectedIndex = -1
	m.resultsOffset = 0
	m.preview = nil
	m.previewError = nil

	// If query is empty, clear results
	if m.query == "" {
		m.searchResults = nil
		m.isSearching = false
		return nil
	}

	// Start search after debounce
	query := m.query
	mask := m.mask
	return tea.Tick(debounceDuration, func(time.Time) tea.Msg {
		return startSearchMsg{Query: query, Mask: mask}
	})
}

// startSearchMsg is sent after debounce to start the actual search
type startSearchMsg struct {
	Query string
	Mask  string
}

// handleSearchResult processes search results
func (m *Model) handleSearchResult(msg search.SearchResultMsg) (tea.Model, tea.Cmd) {
	m.isSearching = false
	m.searchCancel = nil

	if msg.Error != nil {
		m.searchError = msg.Error
		m.searchResults = nil
		return m, nil
	}

	m.searchResults = msg.Results
	m.searchError = nil

	// Auto-select first result if available
	if len(m.searchResults) > 0 && m.selectedIndex < 0 {
		m.selectedIndex = 0
		m.resultsOffset = 0
		return m, m.loadPreview()
	}

	return m, nil
}

// adjustScroll adjusts the scroll offset to keep selected item visible
func (m *Model) adjustScroll() {
	const visibleResults = 5

	if len(m.searchResults) <= visibleResults {
		m.resultsOffset = 0
		return
	}

	// If selected item is above visible area, scroll up
	if m.selectedIndex < m.resultsOffset {
		m.resultsOffset = m.selectedIndex
	}

	// If selected item is below visible area, scroll down
	if m.selectedIndex >= m.resultsOffset+visibleResults {
		m.resultsOffset = m.selectedIndex - visibleResults + 1
	}

	// Ensure offset doesn't go negative
	if m.resultsOffset < 0 {
		m.resultsOffset = 0
	}

	// Ensure offset doesn't exceed bounds
	maxOffset := len(m.searchResults) - visibleResults
	if m.resultsOffset > maxOffset {
		m.resultsOffset = maxOffset
	}
}

// loadPreview loads preview for the currently selected result
func (m *Model) loadPreview() tea.Cmd {
	if m.selectedIndex < 0 || m.selectedIndex >= len(m.searchResults) {
		return nil
	}

	result := m.searchResults[m.selectedIndex]
	return func() tea.Msg {
		preview, err := preview.LoadPreview(result.File, result.Line)
		return previewLoadedMsg{Preview: preview, Error: err}
	}
}

// previewLoadedMsg is sent when preview is loaded
type previewLoadedMsg struct {
	Preview *preview.Preview
	Error   error
}

// handlePreviewLoaded processes loaded preview
func (m *Model) handlePreviewLoaded(msg previewLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.Error != nil {
		m.previewError = msg.Error
		m.preview = nil
	} else {
		m.preview = msg.Preview
		m.previewError = nil
	}
	return m, nil
}

// handleStartSearch starts the actual search
func (m *Model) handleStartSearch(msg startSearchMsg) (tea.Model, tea.Cmd) {
	// Only start if query hasn't changed
	if m.query != msg.Query || m.mask != msg.Mask {
		return m, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.searchCancel = cancel
	m.isSearching = true
	m.searchError = nil

	return m, func() tea.Msg {
		resultChan := m.searcher.Search(ctx, msg.Query, msg.Mask)
		msg := <-resultChan
		return msg
	}
}
