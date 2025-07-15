package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/jlaneve/cwt-cli/internal/types"
)

// Minimal styles for the TUI
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Margin(0, 0, 1, 0)

	actionsStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Margin(1, 0, 0, 0)

	helpStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			Padding(1, 2).
			Margin(1, 2)

	confirmStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			Padding(1, 2).
			Margin(2, 4)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Bold(true)

	// Simple status colors
	waitingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	workingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	deadStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	aliveStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	changesStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	cleanStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	idleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// View renders the entire TUI
func (m Model) View() string {
	if !m.ready {
		return "Loading sessions..."
	}

	// HEADER - Dashboard info
	header := m.renderHeader()

	// Calculate exact middle height (no separate status area now)
	middleHeight := m.height - 5 - 1 // header=3, actions=1

	// MIDDLE PANEL - Combined left and right panels
	middle := m.renderMiddlePanel(m.width, middleHeight)

	// ACTIONS BAR - Navigation help
	actions := m.renderActions()

	// Assemble everything
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		middle,
		actions,
	)

	// Overlay dialogs
	if m.confirmDialog != nil {
		return m.renderWithConfirmDialog(content)
	}

	if m.newSessionDialog != nil {
		return m.renderWithNewSessionDialog(content)
	}

	if m.showHelp {
		return m.renderWithHelp(content)
	}

	return content
}

// renderHeader renders the dashboard header with summary info
func (m Model) renderHeader() string {
	totalSessions := len(m.sessions)
	activeSessions := 0
	needsAttention := 0

	for _, session := range m.sessions {
		if session.IsAlive {
			activeSessions++
		}
		if session.ClaudeStatus.State == types.ClaudeWaiting {
			needsAttention++
		}
	}

	summary := fmt.Sprintf("CWT Dashboard - %d sessions, %d active", totalSessions, activeSessions)
	if needsAttention > 0 {
		summary += fmt.Sprintf(", %d need attention", needsAttention)
	}

	// Header with proper styling and natural height
	return lipgloss.NewStyle().
		Bold(true).
		Width(m.width).
		Padding(1).
		Height(3). // one line plus top + bottom padding
		Render(summary)
}

// renderMiddlePanel renders the combined left and right panels
func (m Model) renderMiddlePanel(width int, height int) string {
	statusHeight := 0

	// if we need to render the status area, we need to account for it in the height
	if m.lastError != "" || m.successMessage != "" {
		statusHeight = 2 // Reserve 2 lines for status messages
	}

	height -= statusHeight

	// LEFT PANEL - Session list with border
	leftPanel := m.renderLeftPanel(40, height)

	// RIGHT PANEL - Session details with border (includes status area at bottom)
	rightPanel := m.renderRightPanel(width-40-1, height)

	middleSection := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, " ", rightPanel)

	if statusHeight > 0 {
		// render the status area
		statusArea := m.renderStatusArea()
		middleSection = lipgloss.JoinVertical(lipgloss.Top, middleSection, statusArea)
	}

	// Assemble middle section
	return middleSection
}

// renderLeftPanel renders the session list on the left side
func (m Model) renderLeftPanel(width int, height int) string {
	totalItems := len(m.sessions) + len(m.creatingSessions)
	if totalItems == 0 {
		content := "No sessions found.\n\nPress 'n' to create a new session."
		return lipgloss.NewStyle().
			Width(width).
			Height(height).
			Border(lipgloss.NormalBorder()).
			Padding(1).
			Render(content)
	}

	var lines []string
	lines = append(lines, "Sessions:")
	lines = append(lines, "")

	// Track current item index for selection
	itemIndex := 0

	// Show creating sessions first
	for name := range m.creatingSessions {
		// Selection indicator on the far left
		var selectionIndicator string
		if itemIndex == m.selectedIndex {
			selectionIndicator = "▶"
		} else {
			selectionIndicator = " "
		}

		// Creating indicator
		creatingIndicator := workingStyle.Render("●")

		// Session name with creating status
		sessionName := name + " (creating...)"

		// Build the session line
		sessionPart := fmt.Sprintf("%s %s %s", selectionIndicator, creatingIndicator, sessionName)

		// Calculate spacing - no git indicator for creating sessions
		contentWidth := width - 4                             // Account for border and padding
		sessionPartVisual := 1 + 1 + 1 + 1 + len(sessionName) // selection + space + indicator + space + name

		spacesNeeded := contentWidth - sessionPartVisual
		if spacesNeeded < 0 {
			spacesNeeded = 0
		}

		line := sessionPart + strings.Repeat(" ", spacesNeeded)
		lines = append(lines, line)
		itemIndex++
	}

	// Show existing sessions
	for _, session := range m.sessions {
		// Selection indicator on the far left
		var selectionIndicator string
		if itemIndex == m.selectedIndex {
			selectionIndicator = "▶"
		} else {
			selectionIndicator = " "
		}

		// Claude status indicator
		claudeIndicator := getClaudeIndicator(session.ClaudeStatus.State)

		// Session name with tmux status
		name := session.Core.Name
		if !session.IsAlive {
			name += " (closed)"
		}

		// Git changes indicator on the right
		gitIndicator := getGitIndicator(session.GitStatus)

		// Build the session part with selection and claude indicators
		sessionPart := fmt.Sprintf("%s %s %s", selectionIndicator, claudeIndicator, name)

		// Calculate spacing for right-aligned git indicator
		contentWidth := width - 4                      // Account for border and padding
		sessionPartVisual := 1 + 1 + 1 + 1 + len(name) // selection + space + claude + space + name
		gitIndicatorVisual := getGitIndicatorVisualLength(session.GitStatus)

		spacesNeeded := contentWidth - sessionPartVisual - gitIndicatorVisual
		if spacesNeeded < 1 {
			spacesNeeded = 1
		}

		line := sessionPart + strings.Repeat(" ", spacesNeeded) + gitIndicator
		lines = append(lines, line)
		itemIndex++
	}

	content := strings.Join(lines, "\n")

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Border(lipgloss.NormalBorder()).
		Padding(1).
		Render(content)
}

// renderRightPanel renders the detailed view of the selected session
func (m Model) renderRightPanel(width int, height int) string {
	totalItems := len(m.sessions) + len(m.creatingSessions)
	if totalItems == 0 || m.selectedIndex >= totalItems {
		var lines []string
		lines = append(lines, "No session selected")

		// Add status area at the bottom
		if m.lastError != "" {
			lines = append(lines, "")
			lines = append(lines, "---")
			// Use the same 2-line error rendering logic
			errorLines := m.renderErrorMessageForPanel(width - 6)
			lines = append(lines, errorLines...)
		} else if m.successMessage != "" {
			lines = append(lines, "")
			lines = append(lines, "---")
			successMsg := "✓ " + sanitizeMessage(m.successMessage)
			maxWidth := width - 6 // Account for border, padding, and prefix
			if len(successMsg) > maxWidth {
				successMsg = successMsg[:maxWidth-3] + "..."
			}
			successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // Green
			lines = append(lines, successStyle.Render(successMsg))
		}

		content := strings.Join(lines, "\n")
		return lipgloss.NewStyle().
			Width(width).
			Height(height).
			Border(lipgloss.NormalBorder()).
			Padding(1).
			Render(content)
	}

	// Check if we're selecting a creating session
	if m.selectedIndex < len(m.creatingSessions) {
		// Get the creating session name (map iteration order isn't guaranteed, but for display it's okay)
		var creatingName string
		i := 0
		for name := range m.creatingSessions {
			if i == m.selectedIndex {
				creatingName = name
				break
			}
			i++
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Session: %s", creatingName))
		lines = append(lines, "")
		lines = append(lines, "Status: Creating session...")
		lines = append(lines, "")
		lines = append(lines, "Please wait while the session is being set up with:")
		lines = append(lines, "• Git worktree")
		lines = append(lines, "• Claude configuration")
		lines = append(lines, "• Tmux session")

		// Add status area at the bottom
		if m.lastError != "" {
			lines = append(lines, "")
			lines = append(lines, "---")
			// Use the same 2-line error rendering logic
			errorLines := m.renderErrorMessageForPanel(width - 6)
			lines = append(lines, errorLines...)
		} else if m.successMessage != "" {
			lines = append(lines, "")
			lines = append(lines, "---")
			successMsg := "✓ " + sanitizeMessage(m.successMessage)
			maxWidth := width - 6 // Account for border, padding, and prefix
			if len(successMsg) > maxWidth {
				successMsg = successMsg[:maxWidth-3] + "..."
			}
			successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // Green
			lines = append(lines, successStyle.Render(successMsg))
		}

		content := strings.Join(lines, "\n")
		return lipgloss.NewStyle().
			Width(width).
			Height(height).
			Border(lipgloss.NormalBorder()).
			Padding(1).
			Render(content)
	}

	// Regular session - adjust index to account for creating sessions
	sessionIndex := m.selectedIndex - len(m.creatingSessions)
	if sessionIndex >= len(m.sessions) {
		var lines []string
		lines = append(lines, "Session not found")

		// Add status area at the bottom
		if m.lastError != "" {
			lines = append(lines, "")
			lines = append(lines, "---")
			// Use the same 2-line error rendering logic
			errorLines := m.renderErrorMessageForPanel(width - 6)
			lines = append(lines, errorLines...)
		} else if m.successMessage != "" {
			lines = append(lines, "")
			lines = append(lines, "---")
			successMsg := "✓ " + sanitizeMessage(m.successMessage)
			maxWidth := width - 6 // Account for border, padding, and prefix
			if len(successMsg) > maxWidth {
				successMsg = successMsg[:maxWidth-3] + "..."
			}
			successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // Green
			lines = append(lines, successStyle.Render(successMsg))
		}

		content := strings.Join(lines, "\n")
		return lipgloss.NewStyle().
			Width(width).
			Height(height).
			Border(lipgloss.NormalBorder()).
			Padding(1).
			Render(content)
	}

	session := m.sessions[sessionIndex]

	var lines []string
	lines = append(lines, fmt.Sprintf("Session: %s", session.Core.Name))
	lines = append(lines, fmt.Sprintf("ID: %s", session.Core.ID))
	lines = append(lines, fmt.Sprintf("Created: %s", session.Core.CreatedAt.Format("2006-01-02 15:04:05")))
	lines = append(lines, "")

	// Tmux status
	tmuxStatus := "alive"
	if !session.IsAlive {
		tmuxStatus = deadStyle.Render("dead")
	} else {
		tmuxStatus = aliveStyle.Render("alive")
	}
	lines = append(lines, fmt.Sprintf("Tmux: %s (%s)", tmuxStatus, session.Core.TmuxSession))
	lines = append(lines, "")

	// Claude status
	claudeStatus := formatClaudeStatusDetail(session.ClaudeStatus)
	lines = append(lines, fmt.Sprintf("Claude: %s", claudeStatus))
	if session.ClaudeStatus.StatusMessage != "" {
		lines = append(lines, fmt.Sprintf("Message: %s", session.ClaudeStatus.StatusMessage))
	}
	if !session.ClaudeStatus.LastMessage.IsZero() {
		lines = append(lines, fmt.Sprintf("Last activity: %s", formatActivity(session.ClaudeStatus.LastMessage)))
	}
	lines = append(lines, "")

	// Git status
	gitStatus := "clean"
	if session.GitStatus.HasChanges {
		gitStatus = changesStyle.Render("has changes")
	} else {
		gitStatus = cleanStyle.Render("clean")
	}
	lines = append(lines, fmt.Sprintf("Git: %s", gitStatus))

	if session.GitStatus.HasChanges {
		// Calculate available width for file names (account for border, padding, and git prefix)
		availableWidth := width - 10 // Border(2) + Padding(2) + Indentation(4) + GitPrefix(2)

		if len(session.GitStatus.ModifiedFiles) > 0 {
			lines = append(lines, fmt.Sprintf("  Modified (%d):", len(session.GitStatus.ModifiedFiles)))
			for _, file := range session.GitStatus.ModifiedFiles {
				displayFile := truncateFileName(file, availableWidth)
				lines = append(lines, fmt.Sprintf("    M %s", displayFile))
			}
		}
		if len(session.GitStatus.AddedFiles) > 0 {
			lines = append(lines, fmt.Sprintf("  Added (%d):", len(session.GitStatus.AddedFiles)))
			for _, file := range session.GitStatus.AddedFiles {
				displayFile := truncateFileName(file, availableWidth)
				lines = append(lines, fmt.Sprintf("    A %s", displayFile))
			}
		}
		if len(session.GitStatus.DeletedFiles) > 0 {
			lines = append(lines, fmt.Sprintf("  Deleted (%d):", len(session.GitStatus.DeletedFiles)))
			for _, file := range session.GitStatus.DeletedFiles {
				displayFile := truncateFileName(file, availableWidth)
				lines = append(lines, fmt.Sprintf("    D %s", displayFile))
			}
		}
		if len(session.GitStatus.UntrackedFiles) > 0 {
			lines = append(lines, fmt.Sprintf("  Untracked (%d):", len(session.GitStatus.UntrackedFiles)))
			for _, file := range session.GitStatus.UntrackedFiles {
				displayFile := truncateFileName(file, availableWidth)
				lines = append(lines, fmt.Sprintf("    ? %s", displayFile))
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Worktree: %s", session.Core.WorktreePath))

	content := strings.Join(lines, "\n")

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Border(lipgloss.NormalBorder()).
		Padding(1).
		Render(content)
}

// renderStatusArea renders the status/notification area between main content and actions
// Now supports up to 2 lines for error messages
func (m Model) renderStatusArea() string {
	if m.lastError != "" {
		return m.renderErrorMessage()
	} else if m.successMessage != "" {
		return m.renderSuccessMessage()
	}
	// Return empty string to maintain spacing
	return ""
}

// renderErrorMessage handles error message rendering with 2-line support
func (m Model) renderErrorMessage() string {
	maxWidth := m.width - 10       // Leave some margin
	errorMsg := "✗ " + m.lastError // Don't sanitize - preserve newlines for wrapping

	// Split message into words for intelligent wrapping
	words := strings.Fields(errorMsg)
	if len(words) == 0 {
		return errorStyle.Height(2).Render("✗ Error")
	}

	var lines []string
	currentLine := ""

	for _, word := range words {
		// Check if adding this word would exceed the width
		testLine := currentLine
		if testLine != "" {
			testLine += " "
		}
		testLine += word

		if len(testLine) <= maxWidth {
			currentLine = testLine
		} else {
			// Start new line if we have room for 2 lines
			if len(lines) < 1 {
				lines = append(lines, currentLine)
				currentLine = word
			} else {
				// Truncate if we're already at 2 lines
				if len(currentLine)+4 <= maxWidth { // +4 for "..."
					currentLine += "..."
				} else {
					currentLine = currentLine[:maxWidth-3] + "..."
				}
				break
			}
		}
	}

	// Add the last line if it has content
	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	// Ensure we have exactly 2 lines for consistent spacing
	for len(lines) < 2 {
		lines = append(lines, "")
	}

	errorText := strings.Join(lines, "\n")
	return errorStyle.Height(2).Render(errorText)
}

// renderSuccessMessage handles success message rendering (still 1 line)
func (m Model) renderSuccessMessage() string {
	// Keep success messages as single line
	maxWidth := m.width - 10 // Leave some margin
	successMsg := "✓ " + sanitizeMessage(m.successMessage)
	if len(successMsg) > maxWidth {
		successMsg = successMsg[:maxWidth-3] + "..."
	}
	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // Green
	// Use 2 lines for consistent spacing with error messages
	return successStyle.Height(2).Render(successMsg)
}

// renderErrorMessageForPanel renders error message for right panel with 2-line support
func (m Model) renderErrorMessageForPanel(maxWidth int) []string {
	errorMsg := "✗ " + m.lastError

	// Split message into words for intelligent wrapping
	words := strings.Fields(errorMsg)
	if len(words) == 0 {
		return []string{errorStyle.Render("✗ Error"), ""}
	}

	var lines []string
	currentLine := ""

	for _, word := range words {
		// Check if adding this word would exceed the width
		testLine := currentLine
		if testLine != "" {
			testLine += " "
		}
		testLine += word

		if len(testLine) <= maxWidth {
			currentLine = testLine
		} else {
			// Start new line if we have room for 2 lines
			if len(lines) < 1 {
				lines = append(lines, errorStyle.Render(currentLine))
				currentLine = word
			} else {
				// Truncate if we're already at 2 lines
				if len(currentLine)+4 <= maxWidth { // +4 for "..."
					currentLine += "..."
				} else {
					currentLine = currentLine[:maxWidth-3] + "..."
				}
				break
			}
		}
	}

	// Add the last line if it has content
	if currentLine != "" {
		lines = append(lines, errorStyle.Render(currentLine))
	}

	// Ensure we have exactly 2 lines for consistent spacing
	for len(lines) < 2 {
		lines = append(lines, "")
	}

	return lines
}

// sanitizeMessage removes newlines and other problematic characters for single-line display
func sanitizeMessage(msg string) string {
	// Replace newlines and carriage returns with spaces
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")

	// Replace multiple spaces with single space
	msg = strings.Join(strings.Fields(msg), " ")

	return msg
}

// renderActions renders the action bar at the bottom
func (m Model) renderActions() string {
	content := "↑↓: navigate  a/enter: attach  s: switch  m: merge  u: publish  n: new  d: delete  c: cleanup  r: refresh  ?: help  q: quit"
	return lipgloss.NewStyle().
		Height(1).
		Width(m.width).
		Foreground(lipgloss.Color("240")).
		Render(content)
}

// renderWithConfirmDialog renders content with a confirmation dialog overlay
func (m Model) renderWithConfirmDialog(content string) string {
	dialog := fmt.Sprintf("%s\n\n[Y]es / [Enter] / [N]o", m.confirmDialog.Message)
	dialogBox := confirmStyle.Render(dialog)

	// Center the dialog on a clean screen
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		dialogBox,
	)
}

// renderWithNewSessionDialog renders content with a new session dialog on clean screen
func (m Model) renderWithNewSessionDialog(content string) string {
	dialog := m.newSessionDialog

	var lines []string
	lines = append(lines, "Create New Session")
	lines = append(lines, "")

	// Name field
	lines = append(lines, "Name:")

	nameValue := dialog.NameInput + "_" // Show cursor
	lines = append(lines, nameValue)
	lines = append(lines, "")

	// Show error if present
	if dialog.Error != "" {
		lines = append(lines, errorStyle.Render("Error: "+dialog.Error))
		lines = append(lines, "")
	}

	// Instructions
	lines = append(lines, "Enter: create  Esc: cancel")

	dialogText := strings.Join(lines, "\n")
	dialogBox := confirmStyle.Render(dialogText)

	// Center the dialog on a clean screen
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		dialogBox,
	)
}

// Removed complex toast overlay system in favor of simpler status area

// renderWithHelp renders content with help overlay
func (m Model) renderWithHelp(content string) string {
	helpText := `CWT Dashboard Help

Navigation:
  ↑/k       Move up
  ↓/j       Move down
  Enter/a   Attach to session
  
Session Actions:
  s         Switch to session branch
  m         Merge session into current branch
  u         Publish session (commit + push)
  
Management:
  n         Create new session
  d         Delete session
  c         Cleanup orphaned resources
  r         Refresh session list
  ?         Toggle this help
  q         Quit

Session Status:
  🟢 alive    Tmux session running
  🔴 dead     Tmux session stopped
  🔔 needs input  Claude waiting for response
  🔄 working  Claude actively processing
  ✅ complete Claude task finished
  📝 changes  Git working tree has changes
  ✨ clean    Git working tree clean

Press ? or Esc to close help`

	helpBox := helpStyle.Render(helpText)

	// Center the help on a clean screen
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		helpBox,
	)
}

// Status formatting functions
func formatTmuxStatus(isAlive bool) string {
	if isAlive {
		return aliveStyle.Render("alive")
	}
	return deadStyle.Render("dead")
}

func formatClaudeStatus(status types.ClaudeStatus) string {
	switch status.State {
	case types.ClaudeWorking:
		return workingStyle.Render("working")
	case types.ClaudeWaiting:
		// Show truncated message if available
		if status.StatusMessage != "" {
			msg := status.StatusMessage
			if len(msg) > 30 {
				msg = msg[:27] + "..."
			}
			return waitingStyle.Render(msg)
		}
		return waitingStyle.Render("waiting")
	case types.ClaudeComplete:
		return "complete"
	case types.ClaudeIdle:
		return idleStyle.Render("idle")
	default:
		return idleStyle.Render("unknown")
	}
}

func formatGitStatus(status types.GitStatus) string {
	if status.HasChanges {
		return changesStyle.Render("changes")
	}
	return cleanStyle.Render("clean")
}

func formatActivity(lastActivity time.Time) string {
	if lastActivity.IsZero() {
		return "unknown"
	}

	age := time.Since(lastActivity)
	if age < time.Minute {
		return "just now"
	}
	if age < time.Hour {
		minutes := int(age.Minutes())
		return fmt.Sprintf("%dm ago", minutes)
	}
	if age < 24*time.Hour {
		hours := int(age.Hours())
		return fmt.Sprintf("%dh ago", hours)
	}
	days := int(age.Hours() / 24)
	return fmt.Sprintf("%dd ago", days)
}

// Helper functions for split-pane layout

func getClaudeIndicator(state types.ClaudeState) string {
	switch state {
	case types.ClaudeWorking:
		return workingStyle.Render("●")
	case types.ClaudeWaiting:
		return waitingStyle.Render("◐")
	case types.ClaudeComplete:
		return "◉"
	case types.ClaudeIdle:
		return idleStyle.Render("○")
	default:
		return idleStyle.Render("○")
	}
}

func getGitIndicator(status types.GitStatus) string {
	if !status.HasChanges {
		return cleanStyle.Render("◦")
	}

	// Calculate total changes
	total := len(status.ModifiedFiles) + len(status.AddedFiles) + len(status.DeletedFiles) + len(status.UntrackedFiles)
	if total == 0 {
		return cleanStyle.Render("◦")
	}

	return changesStyle.Render(fmt.Sprintf("+%d", total))
}

func formatClaudeStatusDetail(status types.ClaudeStatus) string {
	switch status.State {
	case types.ClaudeWorking:
		return workingStyle.Render("working")
	case types.ClaudeWaiting:
		return waitingStyle.Render("waiting for input")
	case types.ClaudeComplete:
		return "complete"
	case types.ClaudeIdle:
		return idleStyle.Render("idle")
	default:
		return idleStyle.Render("unknown")
	}
}

func getGitIndicatorVisualLength(status types.GitStatus) int {
	if !status.HasChanges {
		return 1 // "◦"
	}

	// Calculate total changes
	total := len(status.ModifiedFiles) + len(status.AddedFiles) + len(status.DeletedFiles) + len(status.UntrackedFiles)
	if total == 0 {
		return 1 // "◦"
	}

	// "+N" where N is the number
	return len(fmt.Sprintf("+%d", total))
}

// truncateFileName intelligently truncates file names to fit within available width
func truncateFileName(filename string, maxWidth int) string {
	if len(filename) <= maxWidth {
		return filename
	}

	// If the filename is too long, show the beginning and end with "..." in the middle
	if maxWidth < 10 {
		// If very narrow, just truncate with ...
		if maxWidth < 4 {
			return "..."
		}
		return filename[:maxWidth-3] + "..."
	}

	// For longer filenames, show beginning and end
	prefixLen := (maxWidth - 3) / 2
	suffixLen := maxWidth - 3 - prefixLen

	return filename[:prefixLen] + "..." + filename[len(filename)-suffixLen:]
}
