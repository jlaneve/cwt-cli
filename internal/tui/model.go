package tui

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"

	"github.com/jlaneve/cwt-cli/internal/state"
	"github.com/jlaneve/cwt-cli/internal/types"
	"github.com/jlaneve/cwt-cli/internal/utils"
)

// Global logger for debugging
var debugLogger *log.Logger

// Constants for UI behavior
const (
	ScrollAmount = 10 // Number of lines to scroll in diff view
)

func init() {
	// Create debug log file
	logFile, err := os.OpenFile("cwt-tui-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		debugLogger = log.New(logFile, "[TUI-DEBUG] ", log.LstdFlags|log.Lshortfile)
		debugLogger.Println("=== TUI Debug Session Started ===")
	}
}

// Model represents the main TUI state
type Model struct {
	stateManager     *state.Manager
	sessions         []types.Session
	fileWatcher      *fsnotify.Watcher
	showHelp         bool
	confirmDialog    *ConfirmDialog
	newSessionDialog *NewSessionDialog
	lastError        string
	successMessage   string // For success toast notifications
	ready            bool
	attachOnExit     string // Session name to attach to when exiting TUI

	// Terminal dimensions
	width  int
	height int

	// Split-pane state
	selectedIndex int // Which session is selected in the left panel

	// Session creation tracking
	creatingSessions map[string]bool // Track sessions being created

	// Event channel for file watching
	eventChan chan tea.Msg

	// Diff mode state
	diffMode     *DiffMode
	showDiffMode bool
}

// ConfirmDialog represents a yes/no confirmation dialog
type ConfirmDialog struct {
	Message string
	OnYes   func() tea.Cmd
	OnNo    func() tea.Cmd
}

// NewSessionDialog represents a new session creation dialog
type NewSessionDialog struct {
	NameInput string
	Error     string
}

// DiffMode represents the diff viewer state
type DiffMode struct {
	session      types.Session
	diffLines    []DiffLine
	scrollOffset int
	selectedLine int
	target       string // comparison target (branch)
	cached       bool   // show staged changes only
}

// DiffLine represents a single line in the diff view
type DiffLine struct {
	Type     DiffLineType
	Content  string
	OldLine  int
	NewLine  int
	HunkID   int
	FileName string
}

// DiffLineType represents the type of diff line
type DiffLineType int

const (
	DiffLineContext DiffLineType = iota
	DiffLineAdded
	DiffLineRemoved
	DiffLineHeader
	DiffLineFileHeader
	DiffLineHunkHeader
	DiffLineNoNewline
)

// Event messages for BubbleTea
type (
	// Immediate events (fsnotify)
	sessionStateChangedMsg struct{}
	sessionListChangedMsg  struct{}
	gitIndexChangedMsg     struct{ sessionID string }

	// Polling events
	gitStatusRefreshMsg  struct{}
	tmuxStatusRefreshMsg struct{}

	// User actions
	attachMsg        struct{ sessionID string }
	deleteMsg        struct{ sessionID string }
	createSessionMsg struct{ name string }

	// Internal events
	refreshCompleteMsg struct{ sessions []types.Session }
	errorMsg           struct{ err error }
	confirmYesMsg      struct{}
	confirmNoMsg       struct{}

	// Session creation status
	sessionCreatingMsg       struct{ name string }
	sessionCreatedMsg        struct{ name string }
	sessionCreationFailedMsg struct {
		name string
		err  error
	}

	// Toast messages
	clearSuccessMsg struct{}

	// Dialog events
	showConfirmDialogMsg struct {
		message string
		onYes   func() tea.Cmd
		onNo    func() tea.Cmd
	}

	// New session dialog events
	showNewSessionDialogMsg   struct{}
	newSessionDialogInputMsg  struct{ input string }
	newSessionDialogSubmitMsg struct{}
	newSessionDialogCancelMsg struct{}

	// Clear error message after delay
	clearErrorMsg struct{}

	// Attach request (exits TUI and attaches)
	attachRequestMsg struct{ sessionName string }

	// File watcher setup
	fileWatcherSetupMsg struct{ watcher *fsnotify.Watcher }

	// Diff mode events
	showDiffModeMsg   struct{ sessionID string }
	hideDiffModeMsg   struct{}
	diffLoadedMsg     struct{ diffLines []DiffLine }
	diffErrorMsg      struct{ err error }
	diffScrollUpMsg   struct{}
	diffScrollDownMsg struct{}
)

// NewModel creates a new TUI model
func NewModel(stateManager *state.Manager) (*Model, error) {
	if debugLogger != nil {
		debugLogger.Println("NewModel: Starting TUI model creation")
	}

	// Load initial sessions
	sessions, err := stateManager.DeriveFreshSessions()
	if err != nil {
		if debugLogger != nil {
			debugLogger.Printf("NewModel: Failed to load sessions: %v", err)
		}
		return nil, fmt.Errorf("failed to load initial sessions: %w", err)
	}

	if debugLogger != nil {
		debugLogger.Printf("NewModel: Loaded %d sessions", len(sessions))
		for i, s := range sessions {
			debugLogger.Printf("NewModel: Session %d: ID=%s, Name=%s, IsAlive=%v", i, s.Core.ID, s.Core.Name, s.IsAlive)
		}
	}

	if debugLogger != nil {
		debugLogger.Printf("NewModel: No table needed for split-pane layout")
	}

	return &Model{
		stateManager:     stateManager,
		sessions:         sessions,
		ready:            false,
		creatingSessions: make(map[string]bool),
		eventChan:        make(chan tea.Msg, 100), // Buffered channel for file events
	}, nil
}

// Init initializes the TUI model with necessary setup
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnableMouseCellMotion, // Enable mouse support including scroll events
		m.setupFileWatching(),
		m.startEventChannelListener(),
		m.startGitPolling(),
		m.startTmuxPolling(),
		func() tea.Msg { return refreshCompleteMsg{sessions: m.sessions} },
	)
}

// Update handles all TUI events and state changes
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.MouseMsg:
		return m.handleMouseEvent(msg)

	case refreshCompleteMsg:
		// Store old sessions to detect new ones
		oldSessionIDs := make(map[string]bool)
		for _, session := range m.sessions {
			oldSessionIDs[session.Core.ID] = true
		}

		// Update sessions
		m.sessions = msg.sessions

		// Ensure selectedIndex is within bounds
		totalItems := len(m.sessions) + len(m.creatingSessions)
		if m.selectedIndex >= totalItems {
			m.selectedIndex = totalItems - 1
		}
		if m.selectedIndex < 0 {
			m.selectedIndex = 0
		}

		m.ready = true

		// Add watches for any new sessions
		if m.fileWatcher != nil {
			for _, session := range m.sessions {
				if !oldSessionIDs[session.Core.ID] {
					// This is a new session, add watches
					m.addNewSessionWatches(session)
				}
			}
		}

		return m, nil

	case sessionStateChangedMsg:
		// High priority: Claude state changes (hook events)
		return m, tea.Batch(
			m.refreshSessions(),
			m.startEventChannelListener(), // Restart listener
		)

	case sessionListChangedMsg:
		// High priority: Session CRUD operations
		return m, tea.Batch(
			m.refreshSessions(),
			m.startEventChannelListener(), // Restart listener
		)

	case gitIndexChangedMsg:
		// Medium priority: Git staging operations
		return m, tea.Batch(
			m.refreshSessionGitStatus(msg.sessionID),
			m.startEventChannelListener(), // Restart listener
		)

	case gitStatusRefreshMsg:
		// Low priority: Working tree changes (polling)
		return m, m.refreshAllGitStatus()

	case tmuxStatusRefreshMsg:
		// Low priority: Tmux status (polling)
		return m, m.refreshTmuxStatus()

	case errorMsg:
		m.lastError = msg.err.Error()
		// Clear error after a few seconds and restart event listener if it was from file watcher
		return m, tea.Batch(
			tea.Tick(3*time.Second, func(time.Time) tea.Msg {
				return clearErrorMsg{}
			}),
			m.startEventChannelListener(), // Restart listener in case error came from file watcher
		)

	case clearErrorMsg:
		m.lastError = ""
		return m, nil

	case clearSuccessMsg:
		m.successMessage = ""
		return m, nil

	case confirmYesMsg:
		if m.confirmDialog != nil && m.confirmDialog.OnYes != nil {
			cmd := m.confirmDialog.OnYes()
			m.confirmDialog = nil
			return m, cmd
		}
		return m, nil

	case confirmNoMsg:
		if m.confirmDialog != nil && m.confirmDialog.OnNo != nil {
			cmd := m.confirmDialog.OnNo()
			m.confirmDialog = nil
			return m, cmd
		}
		return m, nil

	case showConfirmDialogMsg:
		return m.handleShowConfirmDialog(msg)

	case showNewSessionDialogMsg:
		return m.handleShowNewSessionDialog()

	case newSessionDialogInputMsg:
		return m.handleNewSessionDialogInput(msg.input)

	case newSessionDialogSubmitMsg:
		return m.handleNewSessionDialogSubmit()

	case newSessionDialogCancelMsg:
		return m.handleNewSessionDialogCancel()

	case sessionCreatingMsg:
		// Mark session as being created
		m.creatingSessions[msg.name] = true
		return m, nil

	case sessionCreatedMsg:
		// Remove from creating list, show success message, and refresh
		delete(m.creatingSessions, msg.name)
		m.successMessage = fmt.Sprintf("Session '%s' created successfully", msg.name)
		return m, tea.Batch(
			m.refreshSessions(),
			tea.Tick(3*time.Second, func(time.Time) tea.Msg {
				return clearSuccessMsg{}
			}),
		)

	case sessionCreationFailedMsg:
		// Remove from creating list and show error
		delete(m.creatingSessions, msg.name)
		m.lastError = fmt.Sprintf("Failed to create session '%s': %s", msg.name, msg.err.Error())
		return m, tea.Tick(5*time.Second, func(time.Time) tea.Msg {
			return clearErrorMsg{}
		})

	case attachRequestMsg:
		if debugLogger != nil {
			debugLogger.Printf("Update: Received attachRequestMsg for session: %s", msg.sessionName)
		}
		// Store the session to attach to and quit
		m.attachOnExit = msg.sessionName
		if debugLogger != nil {
			debugLogger.Printf("Update: Set attachOnExit=%s, calling tea.Quit", msg.sessionName)
		}
		return m, tea.Quit

	case fileWatcherSetupMsg:
		// Store the file watcher in the model
		m.fileWatcher = msg.watcher
		return m, m.startEventChannelListener()

	case showDiffModeMsg:
		return m.handleShowDiffMode(msg.sessionID)

	case hideDiffModeMsg:
		m.showDiffMode = false
		m.diffMode = nil
		return m, nil

	case diffLoadedMsg:
		if m.diffMode != nil {
			m.diffMode.diffLines = msg.diffLines
		}
		return m, nil

	case diffErrorMsg:
		m.lastError = fmt.Sprintf("Diff error: %s", msg.err.Error())
		return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return clearErrorMsg{}
		})

	case diffScrollUpMsg:
		return m.handleDiffScrollUp()

	case diffScrollDownMsg:
		return m.handleDiffScrollDown()
	}

	return m, nil
}

// handleKeyPress processes keyboard input
func (m Model) handleKeyPress(msg tea.KeyMsg) (Model, tea.Cmd) {
	if debugLogger != nil {
		debugLogger.Printf("handleKeyPress: Key pressed: '%s'", msg.String())
	}

	// Handle confirmation dialog first
	if m.confirmDialog != nil {
		if debugLogger != nil {
			debugLogger.Printf("handleKeyPress: In confirmation dialog, key: '%s'", msg.String())
		}
		switch msg.String() {
		case "y", "Y", "enter":
			if debugLogger != nil {
				debugLogger.Println("handleKeyPress: Confirmation Yes")
			}
			return m, func() tea.Msg { return confirmYesMsg{} }
		case "n", "N", "esc":
			if debugLogger != nil {
				debugLogger.Println("handleKeyPress: Confirmation No")
			}
			return m, func() tea.Msg { return confirmNoMsg{} }
		}
		return m, nil
	}

	// Handle new session dialog
	if m.newSessionDialog != nil {
		return m.handleNewSessionDialogKeys(msg)
	}

	// Handle help overlay
	if m.showHelp {
		if debugLogger != nil {
			debugLogger.Printf("handleKeyPress: In help overlay, key: '%s'", msg.String())
		}
		switch msg.String() {
		case "?", "esc", "q":
			m.showHelp = false
		}
		return m, nil
	}

	// Handle diff mode
	if m.showDiffMode {
		return m.handleDiffModeKeys(msg)
	}

	// Handle action keys first (before table navigation)
	if debugLogger != nil {
		debugLogger.Printf("handleKeyPress: Processing action key: '%s', sessions: %d", msg.String(), len(m.sessions))
	}

	switch msg.String() {
	case "q", "ctrl+c":
		if debugLogger != nil {
			debugLogger.Println("handleKeyPress: Quit requested")
		}
		return m, tea.Quit

	case "enter", "a":
		if debugLogger != nil {
			debugLogger.Printf("handleKeyPress: Attach requested, sessions available: %d", len(m.sessions))
		}
		if len(m.sessions) > 0 {
			sessionID := m.getSelectedSessionID()
			if debugLogger != nil {
				debugLogger.Printf("handleKeyPress: Selected session ID: '%s'", sessionID)
			}
			if sessionID == "" {
				if debugLogger != nil {
					debugLogger.Println("handleKeyPress: No session selected - setting error")
				}
				m.lastError = "No session selected"
				return m, nil
			}
			if debugLogger != nil {
				debugLogger.Printf("handleKeyPress: Processing attach directly for session: %s", sessionID)
			}

			// Handle attach directly instead of through a command
			session := m.findSession(sessionID)
			if session == nil {
				m.lastError = "Session not found"
				return m, nil
			}

			if debugLogger != nil {
				debugLogger.Printf("handleKeyPress: Found session %s, IsAlive: %v", session.Core.Name, session.IsAlive)
			}

			if !session.IsAlive {
				// Show confirmation dialog for dead sessions
				if debugLogger != nil {
					debugLogger.Printf("handleKeyPress: Session %s is dead, showing dialog", session.Core.Name)
				}
				m.confirmDialog = &ConfirmDialog{
					Message: fmt.Sprintf("Session '%s' tmux is not running. Recreate it?", session.Core.Name),
					OnYes: func() tea.Cmd {
						return m.recreateAndAttach(sessionID)
					},
					OnNo: func() tea.Cmd {
						return nil
					},
				}
				return m, nil
			}

			// Alive session - exit and attach
			if debugLogger != nil {
				debugLogger.Printf("handleKeyPress: Session %s is alive, setting attachOnExit", session.Core.Name)
			}
			m.attachOnExit = session.Core.TmuxSession
			return m, tea.Quit
		}
		if debugLogger != nil {
			debugLogger.Println("handleKeyPress: No sessions available")
		}
		m.lastError = "No sessions available"
		return m, nil

	case "n":
		return m, func() tea.Msg { return showNewSessionDialogMsg{} }

	case "d":
		if len(m.sessions) > 0 {
			return m, m.confirmDelete(m.getSelectedSessionID())
		}
		return m, nil

	case "c":
		return m, m.runCleanup()

	case "?":
		m.showHelp = true
		return m, nil

	case "r":
		return m, m.refreshSessions()

	case "s":
		// Switch to session branch
		if len(m.sessions) > 0 {
			return m, m.switchToSessionBranch(m.getSelectedSessionID())
		}
		return m, nil

	case "m":
		// Merge session changes
		if len(m.sessions) > 0 {
			return m, m.mergeSessionChanges(m.getSelectedSessionID())
		}
		return m, nil

	case "u":
		// Publish (commit + push) session
		if len(m.sessions) > 0 {
			return m, m.publishSession(m.getSelectedSessionID())
		}
		return m, nil

	case "v":
		// View diff for selected session
		if len(m.sessions) > 0 {
			sessionID := m.getSelectedSessionID()
			if sessionID != "" {
				session := m.findSession(sessionID)
				if session != nil && session.GitStatus.HasChanges {
					return m, func() tea.Msg { return showDiffModeMsg{sessionID: sessionID} }
				}
				m.lastError = "Session has no changes to view"
				return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
					return clearErrorMsg{}
				})
			}
		}
		return m, nil

	case "t":
		// Toggle between detailed/compact view (placeholder for now)
		return m, nil

	case "/":
		// Search/filter sessions (placeholder for now)
		return m, nil
	}

	// Handle navigation keys for the left panel
	switch msg.String() {
	case "up", "k":
		if m.selectedIndex > 0 {
			m.selectedIndex--
		}
		return m, nil
	case "down", "j":
		totalItems := len(m.sessions) + len(m.creatingSessions)
		if m.selectedIndex < totalItems-1 {
			m.selectedIndex++
		}
		return m, nil
	}

	return m, nil
}

// handleMouseEvent processes mouse input including scroll events
func (m Model) handleMouseEvent(msg tea.MouseMsg) (Model, tea.Cmd) {
	// Handle scroll events in diff mode
	if m.showDiffMode && m.diffMode != nil {
		switch msg.Type {
		case tea.MouseWheelUp:
			// Scroll up in diff view
			return m.handleDiffScrollUp()
		case tea.MouseWheelDown:
			// Scroll down in diff view
			return m.handleDiffScrollDown()
		}
	}

	// Handle scroll events in main session list (optional enhancement)
	if !m.showDiffMode && !m.showHelp && m.confirmDialog == nil && m.newSessionDialog == nil {
		switch msg.Type {
		case tea.MouseWheelUp:
			// Scroll up in session list
			if m.selectedIndex > 0 {
				m.selectedIndex--
			}
			return m, nil
		case tea.MouseWheelDown:
			// Scroll down in session list
			totalItems := len(m.sessions) + len(m.creatingSessions)
			if m.selectedIndex < totalItems-1 {
				m.selectedIndex++
			}
			return m, nil
		}
	}

	return m, nil
}

// Session selection helpers
func (m Model) getSelectedSessionID() string {
	if debugLogger != nil {
		debugLogger.Printf("getSelectedSessionID: Sessions count: %d, Creating: %d", len(m.sessions), len(m.creatingSessions))
	}

	totalItems := len(m.sessions) + len(m.creatingSessions)
	if totalItems == 0 {
		if debugLogger != nil {
			debugLogger.Println("getSelectedSessionID: No sessions available")
		}
		return ""
	}

	selectedIdx := m.selectedIndex
	if debugLogger != nil {
		debugLogger.Printf("getSelectedSessionID: Selected index: %d", selectedIdx)
	}

	// Check if selecting a creating session
	if selectedIdx < len(m.creatingSessions) {
		if debugLogger != nil {
			debugLogger.Println("getSelectedSessionID: Selected creating session, returning empty")
		}
		return ""
	}

	// Adjust for regular sessions
	sessionIndex := selectedIdx - len(m.creatingSessions)
	if sessionIndex >= len(m.sessions) {
		if debugLogger != nil {
			debugLogger.Printf("getSelectedSessionID: Adjusted index %d >= sessions %d", sessionIndex, len(m.sessions))
		}
		return ""
	}

	sessionID := m.sessions[sessionIndex].Core.ID
	if debugLogger != nil {
		debugLogger.Printf("getSelectedSessionID: Returning session ID: %s (name: %s)", sessionID, m.sessions[sessionIndex].Core.Name)
	}

	return sessionID
}

// No longer needed - using custom split-pane layout

// GetAttachOnExit returns the session to attach to when exiting TUI
func (m Model) GetAttachOnExit() string {
	return m.attachOnExit
}

func (m Model) findSession(sessionID string) *types.Session {
	for i := range m.sessions {
		if m.sessions[i].Core.ID == sessionID {
			return &m.sessions[i]
		}
	}
	return nil
}

// handleShowDiffMode initializes diff mode for a session
func (m Model) handleShowDiffMode(sessionID string) (Model, tea.Cmd) {
	session := m.findSession(sessionID)
	if session == nil {
		m.lastError = "Session not found"
		return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return clearErrorMsg{}
		})
	}

	if !session.GitStatus.HasChanges {
		m.lastError = "Session has no changes to view"
		return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return clearErrorMsg{}
		})
	}

	m.diffMode = &DiffMode{
		session:      *session,
		scrollOffset: 0,
		selectedLine: 0,
		target:       "origin/main", // default comparison target
		cached:       false,
	}
	m.showDiffMode = true

	// Load the diff data
	return m, m.loadDiffData()
}

// handleDiffModeKeys handles keyboard input in diff mode
func (m Model) handleDiffModeKeys(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.diffMode == nil {
		return m, nil
	}

	switch msg.String() {
	case "esc", "q":
		return m, func() tea.Msg { return hideDiffModeMsg{} }

	case "up", "k":
		return m, func() tea.Msg { return diffScrollUpMsg{} }

	case "down", "j":
		return m, func() tea.Msg { return diffScrollDownMsg{} }

	case "r":
		// Refresh diff
		return m, m.loadDiffData()

	case "c":
		// Toggle cached/working tree view
		m.diffMode.cached = !m.diffMode.cached
		return m, m.loadDiffData()

	case "pgup":
		if m.diffMode.scrollOffset > ScrollAmount {
			m.diffMode.scrollOffset -= ScrollAmount
		} else {
			m.diffMode.scrollOffset = 0
		}
		return m, nil

	case "pgdn":
		maxScroll := len(m.diffMode.diffLines) - (m.height - 6)
		if maxScroll < 0 {
			maxScroll = 0
		}
		if m.diffMode.scrollOffset+ScrollAmount < maxScroll {
			m.diffMode.scrollOffset += ScrollAmount
		} else {
			m.diffMode.scrollOffset = maxScroll
		}
		return m, nil
	}

	return m, nil
}

// handleDiffScrollUp scrolls up in diff view
func (m Model) handleDiffScrollUp() (Model, tea.Cmd) {
	if m.diffMode != nil && m.diffMode.scrollOffset > 0 {
		m.diffMode.scrollOffset--
	}
	return m, nil
}

// handleDiffScrollDown scrolls down in diff view
func (m Model) handleDiffScrollDown() (Model, tea.Cmd) {
	if m.diffMode != nil {
		maxScroll := len(m.diffMode.diffLines) - (m.height - 6)
		if maxScroll < 0 {
			maxScroll = 0
		}
		if m.diffMode.scrollOffset < maxScroll {
			m.diffMode.scrollOffset++
		}
	}
	return m, nil
}

// handleShowConfirmDialog sets up a confirmation dialog
func (m Model) handleShowConfirmDialog(msg showConfirmDialogMsg) (Model, tea.Cmd) {
	m.confirmDialog = &ConfirmDialog{
		Message: msg.message,
		OnYes:   msg.onYes,
		OnNo:    msg.onNo,
	}
	return m, nil
}

// handleShowNewSessionDialog sets up a new session dialog
func (m Model) handleShowNewSessionDialog() (Model, tea.Cmd) {
	m.newSessionDialog = &NewSessionDialog{
		NameInput: "",
		Error:     "",
	}
	return m, nil
}

// handleNewSessionDialogKeys handles keyboard input for the new session dialog
func (m Model) handleNewSessionDialogKeys(msg tea.KeyMsg) (Model, tea.Cmd) {
	dialog := m.newSessionDialog

	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return newSessionDialogCancelMsg{} }

	case "enter":
		return m, func() tea.Msg { return newSessionDialogSubmitMsg{} }

	case "backspace":
		if len(dialog.NameInput) > 0 {
			dialog.NameInput = dialog.NameInput[:len(dialog.NameInput)-1]
		}
		// Clear error when user starts typing
		dialog.Error = ""
		return m, nil

	default:
		// Handle regular character input
		if len(msg.String()) == 1 {
			char := msg.String()
			dialog.NameInput += char
			// Clear error when user starts typing
			dialog.Error = ""
		}
		return m, nil
	}
}

// handleNewSessionDialogInput handles text input for the dialog
func (m Model) handleNewSessionDialogInput(input string) (Model, tea.Cmd) {
	if m.newSessionDialog != nil {
		m.newSessionDialog.NameInput = input
		// Clear error when user types
		m.newSessionDialog.Error = ""
	}
	return m, nil
}

// handleNewSessionDialogSubmit processes the dialog submission
func (m Model) handleNewSessionDialogSubmit() (Model, tea.Cmd) {
	dialog := m.newSessionDialog
	if dialog == nil {
		return m, nil
	}

	// Validate input
	if strings.TrimSpace(dialog.NameInput) == "" {
		dialog.Error = "Session name is required"
		return m, nil
	}

	// Check for duplicate session names
	for _, session := range m.sessions {
		if session.Core.Name == strings.TrimSpace(dialog.NameInput) {
			dialog.Error = "Session name already exists"
			return m, nil
		}
	}

	// Create the session
	name := strings.TrimSpace(dialog.NameInput)

	// Clear the dialog
	m.newSessionDialog = nil

	// Create session using state manager
	return m, tea.Batch(
		// Show immediate "creating" status
		func() tea.Msg {
			return sessionCreatingMsg{name: name}
		},
		// Create session in background
		func() tea.Msg {
			err := m.stateManager.CreateSession(name)
			if err != nil {
				return sessionCreationFailedMsg{name: name, err: err}
			}

			// Signal completion
			return sessionCreatedMsg{name: name}
		},
	)
}

// handleNewSessionDialogCancel cancels the dialog
func (m Model) handleNewSessionDialogCancel() (Model, tea.Cmd) {
	m.newSessionDialog = nil
	return m, nil
}

// switchToSessionBranch switches to a session's branch
func (m Model) switchToSessionBranch(sessionID string) tea.Cmd {
	return func() tea.Msg {
		session := m.findSession(sessionID)
		if session == nil {
			return errorMsg{err: fmt.Errorf("session not found")}
		}

		// Show confirmation dialog
		return showConfirmDialogMsg{
			message: fmt.Sprintf("Switch to session '%s' branch?", session.Core.Name),
			onYes: func() tea.Cmd {
				return func() tea.Msg {
					// Execute cwt switch command
					if err := utils.ExecuteCWTCommand("switch", session.Core.Name); err != nil {
						return errorMsg{err: fmt.Errorf("failed to switch: %w", err)}
					}
					m.successMessage = fmt.Sprintf("Switched to session '%s' branch", session.Core.Name)
					return clearSuccessMsg{}
				}
			},
			onNo: func() tea.Cmd { return nil },
		}
	}
}

// mergeSessionChanges merges a session's changes
func (m Model) mergeSessionChanges(sessionID string) tea.Cmd {
	return func() tea.Msg {
		session := m.findSession(sessionID)
		if session == nil {
			return errorMsg{err: fmt.Errorf("session not found")}
		}

		if !session.GitStatus.HasChanges {
			return errorMsg{err: fmt.Errorf("session '%s' has no changes to merge", session.Core.Name)}
		}

		// Show confirmation dialog
		return showConfirmDialogMsg{
			message: fmt.Sprintf("Merge session '%s' into current branch?", session.Core.Name),
			onYes: func() tea.Cmd {
				return func() tea.Msg {
					// Execute cwt merge command
					if err := utils.ExecuteCWTCommand("merge", session.Core.Name); err != nil {
						return errorMsg{err: fmt.Errorf("failed to merge: %w", err)}
					}
					m.successMessage = fmt.Sprintf("Merged session '%s'", session.Core.Name)
					return clearSuccessMsg{}
				}
			},
			onNo: func() tea.Cmd { return nil },
		}
	}
}

// publishSession publishes a session (commit + push)
func (m Model) publishSession(sessionID string) tea.Cmd {
	return func() tea.Msg {
		session := m.findSession(sessionID)
		if session == nil {
			return errorMsg{err: fmt.Errorf("session not found")}
		}

		if !session.GitStatus.HasChanges {
			return errorMsg{err: fmt.Errorf("session '%s' has no changes to publish", session.Core.Name)}
		}

		// Show confirmation dialog
		return showConfirmDialogMsg{
			message: fmt.Sprintf("Publish session '%s' (commit + push)?", session.Core.Name),
			onYes: func() tea.Cmd {
				return func() tea.Msg {
					// Execute cwt publish command
					if err := utils.ExecuteCWTCommand("publish", session.Core.Name); err != nil {
						return errorMsg{err: fmt.Errorf("failed to publish: %w", err)}
					}
					m.successMessage = fmt.Sprintf("Published session '%s'", session.Core.Name)
					return clearSuccessMsg{}
				}
			},
			onNo: func() tea.Cmd { return nil },
		}
	}
}

// executeCommand executes a shell command
func executeCommand(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	return cmd.Run()
}
