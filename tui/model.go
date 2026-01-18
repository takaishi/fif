package tui

import (
	"context"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/takaishi/fif/editor"
	"github.com/takaishi/fif/preview"
	"github.com/takaishi/fif/search"
)

const (
	debounceDuration   = 250 * time.Millisecond
	escSequenceTimeout = 100 * time.Millisecond // Timeout for ESC sequence detection
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

	// Search scope
	searchScope string // "project" or "directory"
	gitRoot     string // Git repository root path
	currentDir  string // Current working directory

	// ESC sequence handling (for Alt key detection in some terminals)
	waitingForEscSequence bool

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

	// Detect git repository and set initial search scope
	gitRoot, isGitRepo := search.GetCurrentGitRoot()
	currentDir, _ := os.Getwd()

	searchScope := "directory" // Default to current directory
	if isGitRepo {
		searchScope = "project" // If in git repo, default to project scope
	}

	return &Model{
		searcher:      search.NewSearcher(),
		editor:        ed,
		inputMode:     InputModeQuery,
		selectedIndex: -1,
		searchScope:   searchScope,
		gitRoot:       gitRoot,
		currentDir:    currentDir,
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

	case escTimeoutMsg:
		// ESC sequence timeout - treat as ESC key (quit)
		if m.waitingForEscSequence {
			m.waitingForEscSequence = false
			if m.searchCancel != nil {
				m.searchCancel()
			}
			return m, tea.Quit
		}
		return m, nil

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
	keyStr := msg.String()

	// FIRST: Check for special characters that represent Alt key sequences
	// macOS sends Option+P as π (U+03C0) and Option+D as ∂ (U+2202)
	// This must be checked BEFORE any other processing
	//
	// IMPORTANT: On macOS, when Option+P is pressed, the terminal sends
	// the π character (U+03C0) as a regular rune WITHOUT the Alt modifier flag.
	// This is macOS's standard behavior - Option key acts as a character modifier,
	// not as a Meta key. We must intercept this character before it reaches
	// the text input handler.
	if len(msg.Runes) > 0 {
		runeChar := msg.Runes[0]
		// π (U+03C0) = Option+P on macOS
		if runeChar == 'π' || runeChar == 0x03C0 {
			// Alt+P: Switch to project scope
			if m.gitRoot != "" {
				if m.searchScope != "project" {
					m.searchScope = "project"
					return m, m.triggerSearch()
				}
			}
			// Even if already in project scope, return to prevent text input
			return m, nil
		}
		// ∂ (U+2202) = Option+D on macOS
		if runeChar == '∂' || runeChar == 0x2202 {
			// Alt+D: Switch to directory scope
			if m.searchScope != "directory" {
				m.searchScope = "directory"
				return m, m.triggerSearch()
			}
			// Even if already in directory scope, return to prevent text input
			return m, nil
		}
	}

	// Also check keyStr for π/∂ (in case Runes is empty but String contains it)
	if keyStr == "π" || keyStr == "∂" {
		if keyStr == "π" {
			if m.gitRoot != "" {
				if m.searchScope != "project" {
					m.searchScope = "project"
					return m, m.triggerSearch()
				}
			}
			return m, nil
		}
		if keyStr == "∂" {
			if m.searchScope != "directory" {
				m.searchScope = "directory"
				return m, m.triggerSearch()
			}
			return m, nil
		}
	}

	// Check for Alt key combinations
	// Some terminals send Alt+P as ESC followed by 'p', so we check both
	// the string representation and the Alt modifier
	if msg.Alt {
		// Alt key is pressed, check which key
		if len(msg.Runes) > 0 {
			runeChar := msg.Runes[0]
			if runeChar == 'p' || runeChar == 'P' {
				// Alt+P: Switch to project scope
				if m.gitRoot != "" && m.searchScope != "project" {
					m.searchScope = "project"
					return m, m.triggerSearch()
				}
				return m, nil
			}
			if runeChar == 'd' || runeChar == 'D' {
				// Alt+D: Switch to directory scope
				if m.searchScope != "directory" {
					m.searchScope = "directory"
					return m, m.triggerSearch()
				}
				return m, nil
			}
		}
	}

	switch keyStr {
	case "ctrl+c":
		if m.searchCancel != nil {
			m.searchCancel()
		}
		return m, tea.Quit

	case "esc":
		// ESC key might be the start of an Alt key sequence
		// Set flag to wait for next key with timeout
		m.waitingForEscSequence = true
		// Set timeout - if no key comes within timeout, treat as ESC (quit)
		return m, tea.Tick(escSequenceTimeout, func(time.Time) tea.Msg {
			return escTimeoutMsg{}
		})

	case "tab":
		// Switch between query and mask input
		if m.inputMode == InputModeQuery {
			m.inputMode = InputModeMask
		} else {
			m.inputMode = InputModeQuery
		}
		return m, nil

	case "alt+p", "alt+P":
		// Switch to project scope (git repository)
		if m.gitRoot != "" && m.searchScope != "project" {
			m.searchScope = "project"
			// Trigger new search with new scope
			return m, m.triggerSearch()
		}
		return m, nil

	case "alt+d", "alt+D":
		// Switch to directory scope (current directory)
		if m.searchScope != "directory" {
			m.searchScope = "directory"
			// Trigger new search with new scope
			return m, m.triggerSearch()
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
		// Check if we're waiting for ESC sequence (Alt key)
		if m.waitingForEscSequence {
			m.waitingForEscSequence = false
			// Check if this is Alt+P or Alt+D
			if len(msg.Runes) > 0 {
				runeChar := msg.Runes[0]
				if runeChar == 'p' || runeChar == 'P' {
					// Alt+P: Switch to project scope
					if m.gitRoot != "" && m.searchScope != "project" {
						m.searchScope = "project"
						return m, m.triggerSearch()
					}
					return m, nil
				}
				if runeChar == 'd' || runeChar == 'D' {
					// Alt+D: Switch to directory scope
					if m.searchScope != "directory" {
						m.searchScope = "directory"
						return m, m.triggerSearch()
					}
					return m, nil
				}
			}
			// If it's not P or D, ignore (ESC was part of sequence but not our command)
			return m, nil
		}

		// Handle text input
		return m.handleTextInput(msg)
	}
}

// handleTextInput processes text input for query and mask fields
func (m *Model) handleTextInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// FIRST: Check for special characters that might be Alt key sequences
	// macOS sends Option+P as π (U+03C0) and Option+D as ∂ (U+2202)
	// This must be checked BEFORE any other processing to prevent text input
	keyStr := msg.String()

	if len(msg.Runes) > 0 {
		runeChar := msg.Runes[0]

		// π (U+03C0) = Option+P on macOS
		if runeChar == 'π' || runeChar == 0x03C0 {
			// Alt+P: Switch to project scope
			if m.gitRoot != "" {
				if m.searchScope != "project" {
					m.searchScope = "project"
					return m, m.triggerSearch()
				}
			}
			return m, nil
		}
		// ∂ (U+2202) = Option+D on macOS
		if runeChar == '∂' || runeChar == 0x2202 {
			// Alt+D: Switch to directory scope
			if m.searchScope != "directory" {
				m.searchScope = "directory"
				return m, m.triggerSearch()
			}
			return m, nil
		}
	}

	// Also check keyStr for π/∂ (in case Runes is empty but String contains it)
	if keyStr == "π" || keyStr == "∂" {
		if keyStr == "π" {
			if m.gitRoot != "" {
				if m.searchScope != "project" {
					m.searchScope = "project"
					return m, m.triggerSearch()
				}
			}
			return m, nil
		}
		if keyStr == "∂" {
			if m.searchScope != "directory" {
				m.searchScope = "directory"
				return m, m.triggerSearch()
			}
			return m, nil
		}
	}

	// Check for Alt key combinations - prevent them from being treated as text input
	// keyStr is already defined above
	if keyStr == "alt+p" || keyStr == "alt+P" {
		// Alt+P: Switch to project scope
		if m.gitRoot != "" && m.searchScope != "project" {
			m.searchScope = "project"
			return m, m.triggerSearch()
		}
		return m, nil
	}
	if keyStr == "alt+d" || keyStr == "alt+D" {
		// Alt+D: Switch to directory scope
		if m.searchScope != "directory" {
			m.searchScope = "directory"
			return m, m.triggerSearch()
		}
		return m, nil
	}

	// Check Alt modifier and runes
	if msg.Alt {
		if len(msg.Runes) > 0 {
			runeChar := msg.Runes[0]
			if runeChar == 'p' || runeChar == 'P' {
				// Alt+P: Switch to project scope
				if m.gitRoot != "" && m.searchScope != "project" {
					m.searchScope = "project"
					return m, m.triggerSearch()
				}
				return m, nil
			}
			if runeChar == 'd' || runeChar == 'D' {
				// Alt+D: Switch to directory scope
				if m.searchScope != "directory" {
					m.searchScope = "directory"
					return m, m.triggerSearch()
				}
				return m, nil
			}
		}
		// Alt key is pressed but not P or D - don't process as text input
		return m, nil
	}

	var input *textInput
	if m.inputMode == InputModeQuery {
		input = &m.queryInput
	} else {
		input = &m.maskInput
	}

	switch keyStr {
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

// escTimeoutMsg is sent when ESC sequence timeout occurs
type escTimeoutMsg struct{}

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

	// Determine search path based on scope
	searchPath := ""
	if m.searchScope == "project" && m.gitRoot != "" {
		searchPath = m.gitRoot
	} else {
		searchPath = m.currentDir
	}

	return m, func() tea.Msg {
		resultChan := m.searcher.Search(ctx, msg.Query, msg.Mask, searchPath)
		msg := <-resultChan
		return msg
	}
}
