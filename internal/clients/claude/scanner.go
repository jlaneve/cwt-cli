package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ClaudeSession represents a Claude Code session from JSONL
type ClaudeSession struct {
	SessionID    string    `json:"sessionId"`
	CWD          string    `json:"cwd"`
	LastSeen     time.Time `json:"lastSeen"`
	FilePath     string    `json:"filePath"`
	MessageCount int       `json:"messageCount"`
}

// SessionScanner discovers Claude Code sessions
type SessionScanner struct {
	claudeDir string
}

// NewSessionScanner creates a new Claude session scanner
func NewSessionScanner() *SessionScanner {
	homeDir, _ := os.UserHomeDir()
	return &SessionScanner{
		claudeDir: filepath.Join(homeDir, ".claude"),
	}
}

// FindSessionsForDirectory finds Claude sessions that match a given directory
func (s *SessionScanner) FindSessionsForDirectory(targetDir string) ([]*ClaudeSession, error) {
	// Convert to absolute path for matching
	absTargetDir, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Build project directory path from target directory
	// Example: /Users/julian/Astronomer/cwt-cli/.cwt/worktrees/test -> -Users-julian-Astronomer-cwt-cli--cwt-worktrees-test
	// Claude converts /.cwt/ to --cwt- (double dash for hidden dirs)
	projectName := strings.ReplaceAll(absTargetDir, "/", "-")
	projectName = strings.ReplaceAll(projectName, "-.cwt-", "--cwt-")
	projectDir := filepath.Join(s.claudeDir, "projects", projectName)

	// Check if project directory exists
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return []*ClaudeSession{}, nil // No Claude sessions for this directory
	}

	// Scan all .jsonl files in the project directory
	files, err := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("failed to scan claude sessions: %w", err)
	}

	var sessions []*ClaudeSession
	for _, file := range files {
		session, err := s.parseSessionFile(file, absTargetDir)
		if err != nil {
			continue // Skip problematic files
		}
		if session != nil {
			sessions = append(sessions, session)
		}
	}

	// Sort by most recent activity first
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastSeen.After(sessions[j].LastSeen)
	})

	return sessions, nil
}

// GetMostRecentSession returns the most recently active Claude session for a directory
func (s *SessionScanner) GetMostRecentSession(targetDir string) (*ClaudeSession, error) {
	sessions, err := s.FindSessionsForDirectory(targetDir)
	if err != nil {
		return nil, err
	}

	if len(sessions) == 0 {
		return nil, nil // No sessions found
	}

	return sessions[0], nil // Most recent is first due to sorting
}

// parseSessionFile extracts session metadata from a JSONL file
func (s *SessionScanner) parseSessionFile(filePath, targetDir string) (*ClaudeSession, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var sessionID string
	var cwd string
	var lastSeen time.Time
	var messageCount int

	// Read each line of JSONL
	for scanner.Scan() {
		var line map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue // Skip invalid JSON lines
		}

		// Extract session metadata from first message
		if sessionID == "" {
			if sid, ok := line["sessionId"].(string); ok {
				sessionID = sid
			}
			if c, ok := line["cwd"].(string); ok {
				cwd = c
			}
		}

		// Track last activity timestamp
		if timestampStr, ok := line["timestamp"].(string); ok {
			if t, err := time.Parse(time.RFC3339, timestampStr); err == nil {
				if t.After(lastSeen) {
					lastSeen = t
				}
			}
		}

		messageCount++
	}

	// Only return session if it matches our target directory exactly
	if cwd != targetDir {
		return nil, nil
	}

	// Must have valid session ID and recent activity
	if sessionID == "" || lastSeen.IsZero() {
		return nil, nil
	}

	return &ClaudeSession{
		SessionID:    sessionID,
		CWD:          cwd,
		LastSeen:     lastSeen,
		FilePath:     filePath,
		MessageCount: messageCount,
	}, nil
}

// IsClaudeAvailable checks if claude command is available
func (s *SessionScanner) IsClaudeAvailable() bool {
	// Check common installation paths
	claudePaths := []string{
		"/usr/local/bin/claude",
		os.ExpandEnv("$HOME/.claude/local/claude"),
		os.ExpandEnv("$HOME/.claude/local/node_modules/.bin/claude"),
	}

	for _, path := range claudePaths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	return false
}
