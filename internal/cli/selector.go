package cli

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jlaneve/cwt-cli/internal/types"
)

// SessionSelectorResult represents the result of session selection
type SessionSelectorResult struct {
	Session  *types.Session
	Canceled bool
}

// sessionSelectorModel represents the session selector state
type sessionSelectorModel struct {
	sessions []types.Session
	cursor   int
	selected bool
	canceled bool
	title    string
	width    int
	height   int
}

// SessionSelectorOption configures the session selector
type SessionSelectorOption func(*sessionSelectorModel)

// WithTitle sets the selector title
func WithTitle(title string) SessionSelectorOption {
	return func(m *sessionSelectorModel) {
		m.title = title
	}
}

// WithSessionFilter filters sessions based on a predicate
func WithSessionFilter(filter func(types.Session) bool) SessionSelectorOption {
	return func(m *sessionSelectorModel) {
		filtered := make([]types.Session, 0)
		for _, session := range m.sessions {
			if filter(session) {
				filtered = append(filtered, session)
			}
		}
		m.sessions = filtered
	}
}

// SelectSession shows an interactive session selector
func SelectSession(sessions []types.Session, options ...SessionSelectorOption) (*types.Session, error) {
	if len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions available")
	}

	if len(sessions) == 1 {
		// If only one session, return it directly
		return &sessions[0], nil
	}

	model := &sessionSelectorModel{
		sessions: sessions,
		cursor:   0,
		title:    "Select a session:",
	}

	// Apply options
	for _, opt := range options {
		opt(model)
	}

	// Check if we have an interactive terminal
	if !hasInteractiveTerminal() {
		// Fallback to simple number-based selection
		return selectSessionFallback(model.sessions, model.title)
	}

	// Try interactive mode, fallback on any error
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		// Fallback to simple number-based selection
		return selectSessionFallback(model.sessions, model.title)
	}

	result := finalModel.(*sessionSelectorModel)
	if result.canceled {
		return nil, nil // User canceled
	}

	if result.cursor >= 0 && result.cursor < len(result.sessions) {
		return &result.sessions[result.cursor], nil
	}

	return nil, fmt.Errorf("invalid selection")
}

// hasInteractiveTerminal checks if we're running in an interactive terminal
func hasInteractiveTerminal() bool {
	// Check if stdin and stdout are terminals
	_, stdinErr := os.Stdin.Stat()
	_, stdoutErr := os.Stdout.Stat()
	return stdinErr == nil && stdoutErr == nil
}

// selectSessionFallback provides a simple number-based fallback when TTY is not available
func selectSessionFallback(sessions []types.Session, title string) (*types.Session, error) {
	fmt.Println(title)
	fmt.Println()

	for i, session := range sessions {
		status := getSessionStatusIndicator(session)
		activity := FormatActivity(session.LastActivity)
		fmt.Printf("  %d. %s %s (%s)\n", i+1, session.Core.Name, status, activity)
	}

	fmt.Print("\nEnter session number (or 0 to cancel): ")
	var choice int
	if _, err := fmt.Scanf("%d", &choice); err != nil {
		return nil, fmt.Errorf("invalid input")
	}

	if choice == 0 {
		return nil, nil // User canceled
	}

	if choice < 1 || choice > len(sessions) {
		return nil, fmt.Errorf("invalid session number")
	}

	return &sessions[choice-1], nil
}

// Init initializes the selector model
func (m *sessionSelectorModel) Init() tea.Cmd {
	return nil
}

// Update handles user input
func (m *sessionSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.canceled = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
			}

		case "enter", " ":
			m.selected = true
			return m, tea.Quit
		}
	}

	return m, nil
}

// View renders the selector inline
func (m *sessionSelectorModel) View() string {
	if len(m.sessions) == 0 {
		return "No sessions available. Press q to quit."
	}

	var b strings.Builder

	// Title (more compact)
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205"))

	b.WriteString(titleStyle.Render(m.title))
	b.WriteString("\n")

	// Session list (more compact)
	for i, session := range m.sessions {
		prefix := "  "
		if i == m.cursor {
			prefix = "→ "
		}

		// Session info
		status := getSessionStatusIndicator(session)
		activity := FormatActivity(session.LastActivity)

		line := fmt.Sprintf("%s%s %s (%s)",
			prefix,
			session.Core.Name,
			status,
			activity)

		// Style based on selection (less intrusive highlighting)
		if i == m.cursor {
			selectedStyle := lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("cyan"))
			line = selectedStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	// Compact instructions
	instructionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true)

	instructions := "↑/↓: navigate • enter: select • q/esc: cancel"
	b.WriteString(instructionStyle.Render(instructions))

	// Show what will be selected for clarity
	if m.selected {
		b.WriteString("\n\n✓ Selected: " + m.sessions[m.cursor].Core.Name)
	}

	return b.String()
}

// getSessionStatusIndicator returns a compact status indicator for a session
func getSessionStatusIndicator(session types.Session) string {
	var indicators []string

	// Tmux status
	if session.IsAlive {
		indicators = append(indicators, "🟢")
	} else {
		indicators = append(indicators, "🔴")
	}

	// Claude status
	switch session.ClaudeStatus.State {
	case types.ClaudeWorking:
		indicators = append(indicators, "🔄")
	case types.ClaudeWaiting:
		indicators = append(indicators, "⏸️")
	case types.ClaudeComplete:
		indicators = append(indicators, "✅")
	case types.ClaudeIdle:
		indicators = append(indicators, "💤")
	default:
		indicators = append(indicators, "❓")
	}

	// Git status
	if session.GitStatus.HasChanges {
		total := len(session.GitStatus.ModifiedFiles) + len(session.GitStatus.AddedFiles) +
			len(session.GitStatus.DeletedFiles) + len(session.GitStatus.UntrackedFiles)
		indicators = append(indicators, fmt.Sprintf("📝%d", total))
	} else {
		indicators = append(indicators, "✨")
	}

	return strings.Join(indicators, " ")
}
