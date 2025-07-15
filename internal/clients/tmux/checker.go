package tmux

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Checker defines the interface for tmux operations
type Checker interface {
	IsSessionAlive(sessionName string) bool
	CaptureOutput(sessionName string) (string, error)
	CreateSession(name, workdir, command string) error
	KillSession(sessionName string) error
	ListSessions() ([]string, error)
}

// RealChecker implements Checker using actual tmux commands
type RealChecker struct{}

// NewRealChecker creates a new RealChecker
func NewRealChecker() *RealChecker {
	return &RealChecker{}
}

// IsSessionAlive checks if a tmux session exists and is running
func (r *RealChecker) IsSessionAlive(sessionName string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", sessionName)
	err := cmd.Run()
	return err == nil
}

// CaptureOutput captures the current pane output from a tmux session
func (r *RealChecker) CaptureOutput(sessionName string) (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to capture tmux output for session %s: %w", sessionName, err)
	}
	return string(output), nil
}

// CreateSession creates a new tmux session with the specified command
func (r *RealChecker) CreateSession(name, workdir, command string) error {
	args := []string{
		"new-session",
		"-d",       // detached
		"-s", name, // session name
		"-c", workdir, // working directory
	}

	if command != "" {
		args = append(args, command)
	}

	cmd := exec.Command("tmux", args...)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to create tmux session %s: %w", name, err)
	}

	// Enable mouse mode for the session to allow scrolling
	mouseCmd := exec.Command("tmux", "set-option", "-t", name, "mouse", "on")
	if err := mouseCmd.Run(); err != nil {
		// Non-fatal error - session still usable without mouse mode
		fmt.Printf("Warning: Failed to enable mouse mode for session %s: %v\n", name, err)
	}

	return nil
}

// KillSession terminates a tmux session
func (r *RealChecker) KillSession(sessionName string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", sessionName)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to kill tmux session %s: %w", sessionName, err)
	}
	return nil
}

// ListSessions returns a list of all active tmux sessions
func (r *RealChecker) ListSessions() ([]string, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		// tmux returns exit code 1 when no sessions exist
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to list tmux sessions: %w", err)
	}

	sessions := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(sessions) == 1 && sessions[0] == "" {
		return []string{}, nil
	}
	return sessions, nil
}

// MockChecker implements Checker for testing
type MockChecker struct {
	AliveSessions    map[string]bool
	Output           map[string]string
	CreatedSessions  []string
	KilledSessions   []string
	ShouldFailCreate bool
	Delay            time.Duration
}

// NewMockChecker creates a new MockChecker
func NewMockChecker() *MockChecker {
	return &MockChecker{
		AliveSessions:   make(map[string]bool),
		Output:          make(map[string]string),
		CreatedSessions: []string{},
		KilledSessions:  []string{},
	}
}

// IsSessionAlive returns the mocked status
func (m *MockChecker) IsSessionAlive(sessionName string) bool {
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
	return m.AliveSessions[sessionName]
}

// CaptureOutput returns the mocked output
func (m *MockChecker) CaptureOutput(sessionName string) (string, error) {
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
	output, exists := m.Output[sessionName]
	if !exists {
		return "", fmt.Errorf("no output configured for session %s", sessionName)
	}
	return output, nil
}

// CreateSession mocks session creation
func (m *MockChecker) CreateSession(name, workdir, command string) error {
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
	if m.ShouldFailCreate {
		return fmt.Errorf("mock create failure for session %s", name)
	}
	m.CreatedSessions = append(m.CreatedSessions, name)
	m.AliveSessions[name] = true
	return nil
}

// KillSession mocks session termination
func (m *MockChecker) KillSession(sessionName string) error {
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
	m.KilledSessions = append(m.KilledSessions, sessionName)
	m.AliveSessions[sessionName] = false
	return nil
}

// SetSessionAlive sets the alive status for a session
func (m *MockChecker) SetSessionAlive(sessionName string, alive bool) {
	m.AliveSessions[sessionName] = alive
}

// ListSessions returns all alive sessions
func (m *MockChecker) ListSessions() ([]string, error) {
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
	sessions := make([]string, 0)
	for name, alive := range m.AliveSessions {
		if alive {
			sessions = append(sessions, name)
		}
	}
	return sessions, nil
}

// SetAlive sets the alive status for testing
func (m *MockChecker) SetAlive(sessionName string, alive bool) {
	m.AliveSessions[sessionName] = alive
}

// SetOutput sets the output for testing
func (m *MockChecker) SetOutput(sessionName, output string) {
	m.Output[sessionName] = output
}

// SetDelay sets a delay for all operations (for performance testing)
func (m *MockChecker) SetDelay(delay time.Duration) {
	m.Delay = delay
}
