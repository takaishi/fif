package tui

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/takaishi/fif/editor"
	"github.com/takaishi/fif/preview"
	"github.com/takaishi/fif/search"
)

const (
	appDebounceDuration = 250 * time.Millisecond
)

// App represents the tview application
type App struct {
	app *tview.Application

	// UI components
	queryInput   *tview.InputField
	maskInput    *tview.InputField
	maskCheckbox *tview.Checkbox
	resultsList  *tview.List
	previewText  *tview.TextView
	statusText   *tview.TextView
	scopeTabs    *tview.TextView
	headerFlex   *tview.Flex
	scopeFlex    *tview.Flex
	flex         *tview.Flex

	// State
	query         string
	mask          string
	maskEnabled   bool
	searcher      *search.Searcher
	searchCancel  context.CancelFunc
	searchResults []*search.SearchResult
	selectedIndex int
	isSearching   bool
	searchError   error
	preview       *preview.Preview
	previewError  error
	editor        editor.Editor
	searchScope   string // "project" or "directory"
	gitRoot       string
	currentDir    string

	// Debounce
	searchTimer *time.Timer
}

// NewApp creates a new App instance
func NewApp() *App {
	ed, _ := editor.DetectEditor()

	// Detect git repository and set initial search scope
	gitRoot, isGitRepo := search.GetCurrentGitRoot()
	currentDir, _ := os.Getwd()

	searchScope := "directory"
	if isGitRepo {
		searchScope = "project"
	}

	app := &App{
		app:           tview.NewApplication(),
		searcher:      search.NewSearcher(),
		editor:        ed,
		searchScope:   searchScope,
		gitRoot:       gitRoot,
		currentDir:    currentDir,
		maskEnabled:   true,
		selectedIndex: -1,
	}

	app.setupUI()
	return app
}

// SetEditor sets the editor to use
func (a *App) SetEditor(ed editor.Editor) {
	a.editor = ed
}

// Start starts the tview application
func (a *App) Start() error {
	return a.app.Run()
}

// setupUI creates and configures all UI components
func (a *App) setupUI() {
	// Query input - styled like JetBrains
	a.queryInput = tview.NewInputField().
		SetLabel("Find in Files ").
		SetFieldWidth(0).
		SetChangedFunc(a.onQueryChanged).
		SetFieldBackgroundColor(tcell.ColorDefault)
	a.queryInput.SetBorder(false) // Explicitly disable border to avoid double border

	// Mask input
	a.maskInput = tview.NewInputField().
		SetLabel("File mask: ").
		SetFieldWidth(15). // Smaller fixed width to prevent overflow
		SetText("*").
		SetChangedFunc(a.onMaskChanged).
		SetFieldBackgroundColor(tcell.ColorDefault)
	a.maskInput.SetBorder(false) // Explicitly disable border to avoid double border

	// Mask checkbox
	a.maskCheckbox = tview.NewCheckbox().
		SetLabel("[x]").
		SetChecked(true).
		SetChangedFunc(a.onMaskCheckboxChanged)
	a.maskCheckbox.SetBorder(false) // Explicitly disable border to avoid double border

	// Results list - styled for code snippets
	a.resultsList = tview.NewList().
		SetSelectedFunc(a.onResultSelected).
		SetChangedFunc(a.onResultChanged).
		SetHighlightFullLine(true).
		SetSelectedBackgroundColor(tcell.ColorBlue).
		SetSelectedTextColor(tcell.ColorWhite).
		ShowSecondaryText(false) // Don't show secondary text to avoid spacing
	a.resultsList.SetBorder(true).
		SetTitle(" Results ").
		SetBorderColor(tcell.ColorWhite)

	// Note: We don't set InputCapture on resultsList directly
	// Instead, we handle it in the global InputCapture to ensure proper event flow

	// Preview text view
	a.previewText = tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(false).
		SetScrollable(true)
	a.previewText.SetBorder(true).
		SetTitle(" Preview ").
		SetBorderColor(tcell.ColorWhite)

	// Status text - shows match count
	a.statusText = tview.NewTextView().
		SetText("Enter a search query...").
		SetTextAlign(tview.AlignLeft)

	// Scope tabs - styled like JetBrains tabs
	a.scopeTabs = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	a.scopeTabs.SetBorder(false) // Explicitly disable border to avoid double border

	// Build layout
	a.buildLayout()

	// Set input capture for global keybindings
	// Note: This captures keys before they reach individual components
	// We need to be careful to not interfere with component-specific keys
	a.app.SetInputCapture(a.handleGlobalKeys)

	// Set input capture on results list to ensure arrow keys work
	// This is called AFTER the application's SetInputCapture
	// So we need to handle Up/Down keys here to ensure they reach the list
	a.resultsList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// CRITICAL: For Up/Down keys, let the list handle them
		// This must return event to allow the list's internal navigation
		if event.Key() == tcell.KeyUp || event.Key() == tcell.KeyDown {
			return event // Let the list handle navigation
		}
		// Handle Enter key to open file
		if event.Key() == tcell.KeyEnter {
			currentIdx := a.resultsList.GetCurrentItem()
			if currentIdx >= 0 && currentIdx < len(a.searchResults) {
				result := a.searchResults[currentIdx]
				if err := editor.OpenFile(a.editor, result.File, result.Line, result.Column); err != nil {
					// Error opening editor
				}
				a.app.Stop()
			}
			return nil // Consume the event
		}
		// Handle j/k keys for vim-style navigation
		if event.Key() == tcell.KeyRune {
			if event.Rune() == 'j' || event.Rune() == 'J' {
				// Move down
				currentIdx := a.resultsList.GetCurrentItem()
				if currentIdx < len(a.searchResults)-1 {
					a.resultsList.SetCurrentItem(currentIdx + 1)
				}
				return nil
			}
			if event.Rune() == 'k' || event.Rune() == 'K' {
				// Move up
				currentIdx := a.resultsList.GetCurrentItem()
				if currentIdx > 0 {
					a.resultsList.SetCurrentItem(currentIdx - 1)
				}
				return nil
			}
		}
		// For other keys, let the global handler process them first
		// But we need to check if global handler wants to consume them
		handled := a.handleGlobalKeys(event)
		// If global handler returns nil, it consumed the event
		// If it returns event, we should also return event to let list handle it
		return handled
	})
}

// buildLayout creates the UI layout
func (a *App) buildLayout() {
	// Header: Combine search and tabs with border
	// Note: Border takes 2 lines (top and bottom), so we need to account for that
	// We use a single Flex with FlexRow direction to avoid double borders
	// Row 1: Query input and mask input
	// Row 2: Scope tabs
	// Header: Find in Files section (search input only)
	// Use a simple Flex without nested Flex to avoid double border
	// Add components directly to headerFlex, similar to resultsList and previewText
	a.headerFlex = tview.NewFlex().
		AddItem(a.queryInput, 0, 1, true).
		AddItem(nil, 0, 1, false). // Spacer to push maskCheckbox and maskInput to the right
		AddItem(a.maskCheckbox, 3, 0, false).
		AddItem(a.maskInput, 26, 0, false) // Fixed width (15 for field + 11 for "File mask: " label) to prevent overflow
	a.headerFlex.SetBorder(true).
		// SetTitle(" Find in Files ").
		SetBorderColor(tcell.ColorWhite)

	// Scope: In Project/In Directory section (separate border)
	// Use a simple Flex without nested Flex to avoid double border
	// Add scopeTabs directly to scopeFlex, similar to resultsList and previewText
	a.scopeFlex = tview.NewFlex().
		AddItem(a.scopeTabs, 0, 1, false)
	a.scopeFlex.SetBorder(true).
		SetTitle(" Scope ").
		SetBorderColor(tcell.ColorWhite)

	// Root: 4-section layout (Header, Scope, Results, Preview, Status)
	// Results list is fixed at 5 lines, scrollable
	// Border adds 2 lines (top and bottom), so adjust heights accordingly
	// headerFlex: 1 line content + 2 lines for border = 3 lines total
	// scopeFlex: 1 line content + 2 lines for border = 3 lines total
	// resultsList: 5 lines content + 2 lines for border = 7 lines total
	// previewText: remaining space + 2 lines for border
	a.flex = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(a.headerFlex, 3, 0, false).  // Section 1: Search input (1 line content + 2 for border)
		AddItem(a.scopeFlex, 3, 0, false).   // Section 2: Scope tabs (1 line content + 2 for border)
		AddItem(a.resultsList, 7, 0, true).  // Section 3: Results list (5 lines + 2 for border)
		AddItem(a.previewText, 0, 1, false). // Section 4: Preview (not focusable, border included)
		AddItem(a.statusText, 1, 0, false)   // Status bar (not focusable)

	a.app.SetRoot(a.flex, true)
	a.updateScopeTabs()

	// Set initial focus to query input (inside headerFlex)
	// This must be done after SetRoot, but before Run()
	a.app.SetFocus(a.queryInput)
}

// updateScopeTabs updates the scope tabs display
func (a *App) updateScopeTabs() {
	var projectTab, directoryTab string
	if a.searchScope == "project" {
		projectTab = "[white:blue]In Project[white:black]"
		directoryTab = "In Directory"
	} else {
		projectTab = "In Project"
		directoryTab = "[white:blue]In Directory[white:black]"
	}

	scopeText := directoryTab
	if a.gitRoot != "" {
		scopeText = projectTab + "  " + directoryTab
	}
	a.scopeTabs.SetText(scopeText)
}

// handleGlobalKeys handles global keyboard shortcuts
func (a *App) handleGlobalKeys(event *tcell.EventKey) *tcell.EventKey {
	// Get current focus
	currentFocus := a.app.GetFocus()

	// If focus is on headerFlex, redirect to queryInput
	// This ensures that when headerFlex has focus, we can still handle input
	if currentFocus == a.headerFlex {
		// Try to set focus to queryInput
		a.app.SetFocus(a.queryInput)
		// Then handle the event as if queryInput has focus
		currentFocus = a.queryInput
	}

	// CRITICAL: If focus is on results list, handle Up/Down keys FIRST
	// This must be checked before any other processing to ensure arrow keys work
	// tviewのListコンポーネントは上下キーで自動的に選択を移動するので、
	// アプリケーション全体のInputCaptureで上下キーをそのまま返す必要がある
	if currentFocus == a.resultsList {
		// For Up/Down keys, MUST return event to let list handle them
		// This is the most important check - must be first
		if event.Key() == tcell.KeyUp || event.Key() == tcell.KeyDown {
			return event // Let the list handle navigation
		}
		// Handle Enter key to open file
		if event.Key() == tcell.KeyEnter {
			currentIdx := a.resultsList.GetCurrentItem()
			if currentIdx >= 0 && currentIdx < len(a.searchResults) {
				result := a.searchResults[currentIdx]
				if err := editor.OpenFile(a.editor, result.File, result.Line, result.Column); err != nil {
					// Error opening editor
				}
				a.app.Stop()
			}
			return nil // Consume the event
		}
		// Handle j/k keys for vim-style navigation
		if event.Key() == tcell.KeyRune {
			if event.Rune() == 'j' || event.Rune() == 'J' {
				// Move down
				currentIdx := a.resultsList.GetCurrentItem()
				if currentIdx < len(a.searchResults)-1 {
					a.resultsList.SetCurrentItem(currentIdx + 1)
				}
				return nil
			}
			if event.Rune() == 'k' || event.Rune() == 'K' {
				// Move up
				currentIdx := a.resultsList.GetCurrentItem()
				if currentIdx > 0 {
					a.resultsList.SetCurrentItem(currentIdx - 1)
				}
				return nil
			}
		}
		// Allow Tab to move to query input
		if event.Key() == tcell.KeyTab {
			a.app.SetFocus(a.queryInput)
			return nil
		}
		// Allow Esc and Ctrl+C to quit
		if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyCtrlC {
			if a.searchCancel != nil {
				a.searchCancel()
			}
			a.app.Stop()
			return nil
		}
		// Check for Alt+P/D
		if event.Key() == tcell.KeyRune {
			if event.Rune() == 'π' {
				if a.gitRoot != "" && a.searchScope != "project" {
					a.searchScope = "project"
					a.updateScopeTabs()
					a.triggerSearch()
				}
				return nil
			}
			if event.Rune() == '∂' {
				if a.searchScope != "directory" {
					a.searchScope = "directory"
					a.updateScopeTabs()
					a.triggerSearch()
				}
				return nil
			}
		}
		// For all other keys, let the list handle them
		return event
	}

	// If focus is on InputField, allow normal input processing
	// Only intercept specific global shortcuts
	if currentFocus == a.queryInput || currentFocus == a.maskInput {
		// CRITICAL: If there are search results and user presses Up/Down,
		// move selection in results list WITHOUT changing focus
		// This allows users to continue typing while navigating results
		if (event.Key() == tcell.KeyUp || event.Key() == tcell.KeyDown) && len(a.searchResults) > 0 {
			// Get current selection index
			currentIdx := a.selectedIndex
			if currentIdx < 0 {
				currentIdx = 0
			}

			// Move selection
			var newIdx int
			if event.Key() == tcell.KeyUp {
				if currentIdx > 0 {
					newIdx = currentIdx - 1
				} else {
					newIdx = 0
				}
			} else { // KeyDown
				if currentIdx < len(a.searchResults)-1 {
					newIdx = currentIdx + 1
				} else {
					newIdx = len(a.searchResults) - 1
				}
			}

			// Update selection in results list
			a.resultsList.SetCurrentItem(newIdx)
			a.selectedIndex = newIdx

			// Load preview for selected item
			if newIdx >= 0 && newIdx < len(a.searchResults) {
				a.loadPreview(a.searchResults[newIdx])
			}

			// Consume the event so InputField doesn't process it
			return nil
		}
		// Handle Enter key to open file when queryInput has focus
		if event.Key() == tcell.KeyEnter && len(a.searchResults) > 0 {
			currentIdx := a.selectedIndex
			if currentIdx < 0 {
				currentIdx = 0
			}
			if currentIdx >= 0 && currentIdx < len(a.searchResults) {
				result := a.searchResults[currentIdx]
				if err := editor.OpenFile(a.editor, result.File, result.Line, result.Column); err != nil {
					// Error opening editor
				}
				a.app.Stop()
			}
			return nil // Consume the event
		}
		// Handle j/k keys for vim-style navigation when queryInput has focus
		if event.Key() == tcell.KeyRune && len(a.searchResults) > 0 {
			if event.Rune() == 'j' || event.Rune() == 'J' {
				// Move down
				currentIdx := a.selectedIndex
				if currentIdx < 0 {
					currentIdx = 0
				}
				if currentIdx < len(a.searchResults)-1 {
					newIdx := currentIdx + 1
					a.resultsList.SetCurrentItem(newIdx)
					a.selectedIndex = newIdx
					if newIdx >= 0 && newIdx < len(a.searchResults) {
						a.loadPreview(a.searchResults[newIdx])
					}
				}
				return nil // Consume the event
			}
			if event.Rune() == 'k' || event.Rune() == 'K' {
				// Move up
				currentIdx := a.selectedIndex
				if currentIdx < 0 {
					currentIdx = 0
				}
				if currentIdx > 0 {
					newIdx := currentIdx - 1
					a.resultsList.SetCurrentItem(newIdx)
					a.selectedIndex = newIdx
					if newIdx >= 0 && newIdx < len(a.searchResults) {
						a.loadPreview(a.searchResults[newIdx])
					}
				}
				return nil // Consume the event
			}
		}
		// Allow Tab to switch between inputs
		if event.Key() == tcell.KeyTab {
			if currentFocus == a.queryInput {
				a.app.SetFocus(a.maskInput)
			} else {
				a.app.SetFocus(a.queryInput)
			}
			return nil
		}
		// Allow Esc and Ctrl+C to quit
		if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyCtrlC {
			if a.searchCancel != nil {
				a.searchCancel()
			}
			a.app.Stop()
			return nil
		}
		// Check for Alt+P/D only if they are special characters (π/∂)
		if event.Key() == tcell.KeyRune {
			if event.Rune() == 'π' {
				if a.gitRoot != "" && a.searchScope != "project" {
					a.searchScope = "project"
					a.updateScopeTabs()
					a.triggerSearch()
				}
				return nil
			}
			if event.Rune() == '∂' {
				if a.searchScope != "directory" {
					a.searchScope = "directory"
					a.updateScopeTabs()
					a.triggerSearch()
				}
				return nil
			}
		}
		// For all other keys, let InputField handle them
		return event
	}

	// For other components, handle global shortcuts
	// Alt+P: Switch to project scope
	if event.Key() == tcell.KeyRune {
		if event.Rune() == 'π' || (event.Modifiers()&tcell.ModAlt != 0 && event.Rune() == 'p') {
			if a.gitRoot != "" && a.searchScope != "project" {
				a.searchScope = "project"
				a.updateScopeTabs()
				a.triggerSearch()
			}
			return nil
		}
		// Alt+D: Switch to directory scope
		if event.Rune() == '∂' || (event.Modifiers()&tcell.ModAlt != 0 && event.Rune() == 'd') {
			if a.searchScope != "directory" {
				a.searchScope = "directory"
				a.updateScopeTabs()
				a.triggerSearch()
			}
			return nil
		}
	}

	// Tab: Switch between query and mask input, or move to results list
	if event.Key() == tcell.KeyTab {
		if currentFocus == a.queryInput {
			a.app.SetFocus(a.maskInput)
		} else if currentFocus == a.maskInput {
			// Move to results list if there are results
			if len(a.searchResults) > 0 {
				a.app.SetFocus(a.resultsList)
			} else {
				a.app.SetFocus(a.queryInput)
			}
		} else {
			a.app.SetFocus(a.queryInput)
		}
		return nil
	}

	// Esc or Ctrl+C: Quit
	if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyCtrlC {
		if a.searchCancel != nil {
			a.searchCancel()
		}
		a.app.Stop()
		return nil
	}

	return event
}

// onQueryChanged is called when query input changes
func (a *App) onQueryChanged(text string) {
	// Update query only if it actually changed
	if a.query != text {
		a.query = text
		a.triggerSearch()
	}
}

// onMaskChanged is called when mask input changes
func (a *App) onMaskChanged(text string) {
	// Update mask only if it actually changed
	if a.mask != text {
		a.mask = text
		a.triggerSearch()
	}
}

// onMaskCheckboxChanged is called when mask checkbox changes
func (a *App) onMaskCheckboxChanged(checked bool) {
	a.maskEnabled = checked
	if checked {
		a.maskCheckbox.SetLabel("[x]")
	} else {
		a.maskCheckbox.SetLabel("[ ]")
	}
	a.triggerSearch()
}

// onResultSelected is called when a result is selected (Enter)
func (a *App) onResultSelected(index int, mainText, secondaryText string, shortcut rune) {
	if index >= 0 && index < len(a.searchResults) {
		result := a.searchResults[index]
		if err := editor.OpenFile(a.editor, result.File, result.Line, result.Column); err != nil {
			// Error opening editor
		}
		a.app.Stop()
	}
}

// onResultChanged is called when result selection changes
func (a *App) onResultChanged(index int, mainText, secondaryText string, shortcut rune) {
	a.selectedIndex = index
	if index >= 0 && index < len(a.searchResults) {
		a.loadPreview(a.searchResults[index])
	} else {
		a.selectedIndex = -1
		a.previewText.Clear()
		a.preview = nil
	}
}

// triggerSearch starts a new search with debounce
func (a *App) triggerSearch() {
	// Cancel previous timer
	if a.searchTimer != nil {
		a.searchTimer.Stop()
	}

	// Cancel previous search
	if a.searchCancel != nil {
		a.searchCancel()
	}

	// Reset state
	a.selectedIndex = -1
	a.resultsList.Clear()
	a.previewText.Clear()
	a.preview = nil
	a.previewError = nil

	// If query is empty, clear results
	if a.query == "" {
		a.searchResults = nil
		a.isSearching = false
		a.updateStatus()
		return
	}

	// Start debounced search
	a.searchTimer = time.AfterFunc(appDebounceDuration, func() {
		a.performSearch()
	})
}

// performSearch executes the actual search
func (a *App) performSearch() {
	a.isSearching = true
	a.updateStatus()

	ctx, cancel := context.WithCancel(context.Background())
	a.searchCancel = cancel

	// Determine search path
	searchPath := a.currentDir
	if a.searchScope == "project" && a.gitRoot != "" {
		searchPath = a.gitRoot
	}

	// Determine mask
	mask := a.mask
	if !a.maskEnabled {
		mask = ""
	}

	// Start search
	resultChan := a.searcher.Search(ctx, a.query, mask, searchPath)

	// Process results
	go func() {
		var results []*search.SearchResult
		for resultMsg := range resultChan {
			if resultMsg.Error != nil {
				a.app.QueueUpdateDraw(func() {
					a.searchError = resultMsg.Error
					a.isSearching = false
					a.updateStatus()
				})
				return
			}
			results = append(results, resultMsg.Results...)
			a.app.QueueUpdateDraw(func() {
				a.searchResults = results
				a.updateResultsList()
				a.updateStatus()
				// Keep focus on queryInput so users can continue typing
				// Arrow keys will move focus to resultsList when pressed
			})
		}

		a.app.QueueUpdateDraw(func() {
			a.isSearching = false
			a.updateStatus()
		})
	}()
}

// updateResultsList updates the results list display
func (a *App) updateResultsList() {
	a.resultsList.Clear()

	// Get terminal width for formatting
	_, _, width, _ := a.resultsList.GetRect()
	if width == 0 {
		// Fallback if width not available
		width = 80
	}

	for _, result := range a.searchResults {
		// Format: code snippet | file:line (JetBrains style)
		// Extract filename from path
		fileParts := strings.Split(result.File, "/")
		fileName := fileParts[len(fileParts)-1]
		fileInfo := fileName + ":" + strconv.Itoa(result.Line)

		// Calculate the actual width needed for file info
		fileInfoWidth := len(fileInfo)

		// Calculate available width for code snippet
		// Reserve space for separator " | " (3 chars) and file info
		codeWidth := width - fileInfoWidth - 3
		if codeWidth < 10 {
			codeWidth = 10
			fileInfoWidth = width - codeWidth - 3
		}

		// Format code snippet (truncate if needed)
		codeSnippet := result.Text
		if len(codeSnippet) > codeWidth {
			codeSnippet = codeSnippet[:codeWidth-3] + "..."
		}

		// Calculate padding to align file info to the right edge
		codeSnippetLen := len(codeSnippet)
		separatorLen := 3 // " | "
		totalUsed := codeSnippetLen + separatorLen + fileInfoWidth
		padding := width - totalUsed
		if padding < 0 {
			padding = 0
		}

		// Combine: code snippet + separator + padding + file info
		// Padding ensures file info is right-aligned to the edge
		mainText := codeSnippet + " | " + strings.Repeat(" ", padding) + fileInfo

		a.resultsList.AddItem(mainText, "", 0, nil)
	}
	// Set selection if valid
	if len(a.searchResults) > 0 {
		if a.selectedIndex >= 0 && a.selectedIndex < len(a.searchResults) {
			a.resultsList.SetCurrentItem(a.selectedIndex)
		} else {
			// Auto-select first item if no selection
			a.selectedIndex = 0
			a.resultsList.SetCurrentItem(0)
			// Load preview for first item
			a.loadPreview(a.searchResults[0])
		}
	} else {
		a.selectedIndex = -1
	}
}

// updateStatus updates the status text
func (a *App) updateStatus() {
	if a.isSearching {
		a.statusText.SetText("Searching...")
		return
	}
	if a.searchError != nil {
		a.statusText.SetText("Error: " + a.searchError.Error())
		return
	}
	if a.query == "" {
		a.statusText.SetText("Enter a search query...")
		return
	}
	if len(a.searchResults) == 0 {
		a.statusText.SetText("No matches found")
		return
	}

	// Count unique files
	fileMap := make(map[string]bool)
	for _, result := range a.searchResults {
		fileMap[result.File] = true
	}
	fileCount := len(fileMap)
	matchCount := len(a.searchResults)

	// Format like JetBrains: "100+ matches in 41+ files"
	statusText := strconv.Itoa(matchCount)
	if matchCount >= 100 {
		statusText = "100+"
	}
	statusText += " matches"

	if fileCount > 0 {
		fileText := strconv.Itoa(fileCount)
		if fileCount >= 100 {
			fileText = "100+"
		}
		statusText += " in " + fileText
		if fileCount == 1 {
			statusText += " file"
		} else {
			statusText += " files"
		}
	}

	a.statusText.SetText(statusText)
}

// loadPreview loads preview for the selected result
func (a *App) loadPreview(result *search.SearchResult) {
	preview, err := preview.LoadPreview(result.File, result.Line)
	if err != nil {
		a.previewError = err
		a.previewText.SetText("Error loading preview: " + err.Error())
		return
	}

	a.preview = preview
	a.renderPreview()
}

// renderPreview renders the preview content
func (a *App) renderPreview() {
	if a.preview == nil {
		a.previewText.Clear()
		return
	}

	var lines []string
	// File path header
	lines = append(lines, "[yellow:black:b]"+a.preview.File+"[white:black]")
	lines = append(lines, "")

	// Code lines with line numbers
	for i, line := range a.preview.Lines {
		lineNum := a.preview.StartLine + i
		lineNumStr := fmt.Sprintf("%4d", lineNum)
		if i+1 == a.preview.HitLine {
			// Highlight the hit line
			lines = append(lines, "[white:blue]"+lineNumStr+"[white:black] | [yellow:black]"+line+"[white:black]")
		} else {
			lines = append(lines, "[gray:black]"+lineNumStr+"[white:black] | "+line)
		}
	}

	a.previewText.SetText(strings.Join(lines, "\n"))
}
