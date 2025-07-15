package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jlaneve/cwt-cli/internal/clients/tmux"
	"github.com/jlaneve/cwt-cli/internal/types"
)

// Checker defines the interface for Claude status operations
type Checker interface {
	GetStatus(worktreePath string) types.ClaudeStatus
	FindSessionID(worktreePath string) (string, error)
}

// RealChecker implements Checker using actual Claude session detection
type RealChecker struct {
	tmuxChecker tmux.Checker
	scanner     *SessionScanner
}

// NewRealChecker creates a new RealChecker
func NewRealChecker(tmuxChecker tmux.Checker) *RealChecker {
	return &RealChecker{
		tmuxChecker: tmuxChecker,
		scanner:     NewSessionScanner(),
	}
}

// GetStatus analyzes Claude activity in a worktree
func (r *RealChecker) GetStatus(worktreePath string) types.ClaudeStatus {
	status := types.ClaudeStatus{
		State:        types.ClaudeUnknown,
		Availability: types.AvailVeryStale,
	}

	// Use scanner to find the most recent Claude session
	claudeSession, err := r.scanner.GetMostRecentSession(worktreePath)
	if err != nil || claudeSession == nil {
		return status
	}

	status.SessionID = claudeSession.SessionID
	status.LastMessage = claudeSession.LastSeen

	// Parse last message from JSONL file
	lastMessage, err := r.parseLastMessage(claudeSession.FilePath)
	if err != nil {
		// Fallback to session metadata if JSONL parsing fails
		status.LastMessage = claudeSession.LastSeen
		status.Availability = r.calculateAvailability(claudeSession.LastSeen)
		return status
	}

	// Update timestamp from parsed message
	status.LastMessage = lastMessage.Timestamp

	// Determine state from message content
	status.State = r.determineStateFromMessage(lastMessage)

	// Check tmux for waiting prompts (overrides JSONL state)
	if status.State == types.ClaudeWorking {
		tmuxSession := r.deriveTmuxSessionName(worktreePath)
		if r.checkTmuxForWaitingPrompt(tmuxSession) {
			status.State = types.ClaudeWaiting
		}
	}

	// Calculate availability from timestamp
	status.Availability = r.calculateAvailability(lastMessage.Timestamp)

	return status
}

// FindSessionID finds the Claude session ID for a worktree
func (r *RealChecker) FindSessionID(worktreePath string) (string, error) {
	// Use scanner to find session
	claudeSession, err := r.scanner.GetMostRecentSession(worktreePath)
	if err != nil {
		return "", err
	}
	
	if claudeSession == nil {
		return "", fmt.Errorf("no Claude session found for worktree %s", worktreePath)
	}
	
	return claudeSession.SessionID, nil
}


func (r *RealChecker) parseLastMessage(jsonlPath string) (types.ClaudeMessage, error) {
	file, err := os.Open(jsonlPath)
	if err != nil {
		return types.ClaudeMessage{}, err
	}
	defer file.Close()

	var lastMessage types.ClaudeMessage
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var rawMessage map[string]interface{}
		if err := json.Unmarshal([]byte(line), &rawMessage); err != nil {
			continue
		}

		// Parse message structure
		if msg, ok := rawMessage["message"].(map[string]interface{}); ok {
			claudeMsg := types.ClaudeMessage{}

			if role, ok := msg["role"].(string); ok {
				claudeMsg.Role = role
			}

			if content, ok := msg["content"].([]interface{}); ok {
				for _, c := range content {
					if contentMap, ok := c.(map[string]interface{}); ok {
						contentItem := types.Content{}
						if contentType, ok := contentMap["type"].(string); ok {
							contentItem.Type = contentType
						}
						if text, ok := contentMap["text"].(string); ok {
							contentItem.Text = text
						}
						if name, ok := contentMap["name"].(string); ok {
							contentItem.Name = name
						}
						claudeMsg.Content = append(claudeMsg.Content, contentItem)
					}
				}
			}

			// Parse timestamp
			if timestamp, ok := rawMessage["timestamp"].(string); ok {
				if parsed, err := time.Parse(time.RFC3339, timestamp); err == nil {
					claudeMsg.Timestamp = parsed
				}
			}

			if claudeMsg.Role == "assistant" {
				lastMessage = claudeMsg
			}
		}
	}

	if lastMessage.Role == "" {
		return types.ClaudeMessage{}, fmt.Errorf("no assistant messages found in JSONL")
	}

	return lastMessage, nil
}

func (r *RealChecker) determineStateFromMessage(message types.ClaudeMessage) types.ClaudeState {
	// Check if message contains tool usage
	for _, content := range message.Content {
		if content.Type == "tool_use" {
			return types.ClaudeWorking
		}
	}

	// If no tools, Claude is waiting for user input
	return types.ClaudeWaiting
}

func (r *RealChecker) checkTmuxForWaitingPrompt(tmuxSession string) bool {
	if r.tmuxChecker == nil {
		return false
	}

	if !r.tmuxChecker.IsSessionAlive(tmuxSession) {
		return false
	}

	output, err := r.tmuxChecker.CaptureOutput(tmuxSession)
	if err != nil {
		return false
	}

	// Check for common waiting prompts
	waitingPatterns := []*regexp.Regexp{
		regexp.MustCompile(`Do you want to.*\?`),
		regexp.MustCompile(`\d+\.\s+(Yes|No|Cancel)`),
		regexp.MustCompile(`❯\s*\d+\.\s+(Yes|No|Cancel)`),
		regexp.MustCompile(`Continue\?\s*\(y/n\)`),
		regexp.MustCompile(`Press.*to continue`),
	}

	for _, pattern := range waitingPatterns {
		if pattern.MatchString(output) {
			return true
		}
	}

	return false
}

func (r *RealChecker) deriveTmuxSessionName(worktreePath string) string {
	// Extract session name from worktree path
	// Assumes path like: .cwt/worktrees/{session-name}
	base := filepath.Base(worktreePath)
	return fmt.Sprintf("cwt-%s", base)
}

func (r *RealChecker) calculateAvailability(timestamp time.Time) types.Availability {
	if timestamp.IsZero() {
		return types.AvailVeryStale
	}

	age := time.Since(timestamp)

	switch {
	case age < 5*time.Minute:
		return types.AvailCurrent
	case age < 1*time.Hour:
		return types.AvailRecent
	case age < 24*time.Hour:
		return types.AvailStale
	default:
		return types.AvailVeryStale
	}
}


// MockChecker implements Checker for testing
type MockChecker struct {
	Statuses map[string]types.ClaudeStatus
	Delay    time.Duration
}

// NewMockChecker creates a new MockChecker
func NewMockChecker() *MockChecker {
	return &MockChecker{
		Statuses: make(map[string]types.ClaudeStatus),
	}
}

// GetStatus returns the mocked status
func (m *MockChecker) GetStatus(worktreePath string) types.ClaudeStatus {
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
	status, exists := m.Statuses[worktreePath]
	if !exists {
		return types.ClaudeStatus{
			State:        types.ClaudeUnknown,
			Availability: types.AvailVeryStale,
		}
	}
	return status
}

// FindSessionID returns a mock session ID
func (m *MockChecker) FindSessionID(worktreePath string) (string, error) {
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
	// Return a mock session ID based on path
	return fmt.Sprintf("mock-session-%s", filepath.Base(worktreePath)), nil
}

// SetStatus sets the Claude status for testing
func (m *MockChecker) SetStatus(worktreePath string, status types.ClaudeStatus) {
	m.Statuses[worktreePath] = status
}

// SetDelay sets a delay for all operations
func (m *MockChecker) SetDelay(delay time.Duration) {
	m.Delay = delay
}