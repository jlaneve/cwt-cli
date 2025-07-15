package operations

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/jlaneve/cwt-cli/internal/state"
	"github.com/jlaneve/cwt-cli/internal/types"
)

// SessionOperations provides business logic for session management
type SessionOperations struct {
	stateManager *state.Manager
}

// NewSessionOperations creates a new SessionOperations instance
func NewSessionOperations(sm *state.Manager) *SessionOperations {
	return &SessionOperations{
		stateManager: sm,
	}
}

// CreateSession creates a new session with the given name
func (s *SessionOperations) CreateSession(name string) error {
	return s.stateManager.CreateSession(name)
}

// DeleteSession deletes the session with the given ID
func (s *SessionOperations) DeleteSession(sessionID string) error {
	return s.stateManager.DeleteSession(sessionID)
}

// FindSessionByName finds a session by its name
// Returns the session and its ID, or an error if not found
func (s *SessionOperations) FindSessionByName(name string) (*types.Session, string, error) {
	sessions, err := s.stateManager.DeriveFreshSessions()
	if err != nil {
		return nil, "", fmt.Errorf("failed to load sessions: %w", err)
	}

	for _, session := range sessions {
		if session.Core.Name == name {
			return &session, session.Core.ID, nil
		}
	}

	return nil, "", fmt.Errorf("session '%s' not found", name)
}

// FindSessionByID finds a session by its ID
func (s *SessionOperations) FindSessionByID(sessionID string) (*types.Session, error) {
	sessions, err := s.stateManager.DeriveFreshSessions()
	if err != nil {
		return nil, fmt.Errorf("failed to load sessions: %w", err)
	}

	for _, session := range sessions {
		if session.Core.ID == sessionID {
			return &session, nil
		}
	}

	return nil, fmt.Errorf("session with ID '%s' not found", sessionID)
}

// GetAllSessions returns all current sessions
func (s *SessionOperations) GetAllSessions() ([]types.Session, error) {
	return s.stateManager.DeriveFreshSessions()
}

// RecreateDeadSession recreates a tmux session for a session that has died
// This handles Claude session resumption if a previous session exists
func (s *SessionOperations) RecreateDeadSession(session *types.Session) error {
	claudeExec := FindClaudeExecutable()
	if claudeExec == "" {
		return fmt.Errorf("claude executable not found in PATH")
	}

	command := claudeExec

	// Check if there's an existing Claude session to resume
	if existingSessionID, err := s.stateManager.GetClaudeChecker().FindSessionID(session.Core.WorktreePath); err == nil && existingSessionID != "" {
		command = fmt.Sprintf("%s -r %s", claudeExec, existingSessionID)
	}

	// Create the tmux session
	tmuxChecker := s.stateManager.GetTmuxChecker()
	return tmuxChecker.CreateSession(session.Core.TmuxSession, session.Core.WorktreePath, command)
}

// FindClaudeExecutable searches for the Claude CLI executable in common locations
func FindClaudeExecutable() string {
	claudePaths := []string{
		"claude",
		os.ExpandEnv("$HOME/.claude/local/claude"),
		os.ExpandEnv("$HOME/.claude/local/node_modules/.bin/claude"),
		"/usr/local/bin/claude",
	}

	for _, path := range claudePaths {
		// Security: Validate expanded paths to prevent directory traversal
		if !isValidExecutablePath(path) {
			continue
		}
		if _, err := exec.LookPath(path); err == nil {
			return path
		}
	}

	return ""
}

// isValidExecutablePath validates that a path is safe to use as an executable
func isValidExecutablePath(path string) bool {
	// Reject paths with directory traversal patterns
	if strings.Contains(path, "..") {
		return false
	}
	// Reject paths with null bytes
	if strings.Contains(path, "\x00") {
		return false
	}
	// Reject paths with shell metacharacters (except legitimate path separators)
	dangerousChars := []string{";", "&", "|", "$", "`", "(", ")", "{", "}", "[", "]", "*", "?", "<", ">", "~"}
	for _, char := range dangerousChars {
		if strings.Contains(path, char) {
			// Allow $HOME in environment expansion, but only at the start
			if char == "$" && strings.HasPrefix(path, "$HOME") {
				continue
			}
			return false
		}
	}
	return true
}

// AttachToTmuxSession attaches to the specified tmux session using exec.
// This function replaces the current process with tmux attach-session,
// so no code after the syscall.Exec call will execute.
func AttachToTmuxSession(sessionName, tmuxSessionName string) error {
	// Validate input
	if tmuxSessionName == "" {
		return fmt.Errorf("tmux session name cannot be empty")
	}

	// Find tmux in PATH
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found in PATH: %w", err)
	}

	// Verify session exists before attempting attach
	if err := verifyTmuxSessionExists(tmuxSessionName); err != nil {
		return fmt.Errorf("tmux session not found: %w", err)
	}

	// Display consistent user feedback
	fmt.Printf("🔗 Attaching to session '%s' (tmux: %s)...\n", sessionName, tmuxSessionName)

	// Use exec to replace current process with tmux attach
	args := []string{"tmux", "attach-session", "-t", tmuxSessionName}
	err = syscall.Exec(tmuxPath, args, os.Environ())
	if err != nil {
		return fmt.Errorf("failed to exec tmux: %w", err)
	}

	// This point should never be reached if exec succeeds
	// Using panic for consistency as this indicates a fundamental system issue
	panic("syscall.Exec returned unexpectedly")
}

// verifyTmuxSessionExists checks if the specified tmux session exists
func verifyTmuxSessionExists(sessionName string) error {
	cmd := exec.Command("tmux", "has-session", "-t", sessionName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("session '%s' does not exist", sessionName)
	}
	return nil
}
