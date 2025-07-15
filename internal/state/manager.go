package state

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jlaneve/cwt-cli/internal/clients/claude"
	"github.com/jlaneve/cwt-cli/internal/clients/git"
	"github.com/jlaneve/cwt-cli/internal/clients/tmux"
	"github.com/jlaneve/cwt-cli/internal/events"
	"github.com/jlaneve/cwt-cli/internal/types"
)

// Config holds configuration for the StateManager
type Config struct {
	DataDir       string         // Directory for storing session data (e.g., ".cwt")
	TmuxChecker   tmux.Checker   // Injectable tmux operations
	ClaudeChecker claude.Checker // Injectable Claude operations
	GitChecker    git.Checker    // Injectable git operations
	BaseBranch    string         // Base branch for creating worktrees (default: "main")
}

// Manager handles all session state operations
type Manager struct {
	config   Config
	eventBus *events.Bus
	mu       sync.RWMutex
	dataFile string
}

// NewManager creates a new StateManager with the given configuration
func NewManager(config Config) *Manager {
	if config.DataDir == "" {
		config.DataDir = ".cwt"
	}
	if config.BaseBranch == "" {
		config.BaseBranch = "main"
	}

	// Use real checkers if not provided
	if config.TmuxChecker == nil {
		config.TmuxChecker = tmux.NewRealChecker()
	}
	if config.GitChecker == nil {
		config.GitChecker = git.NewRealChecker(config.BaseBranch)
	}
	if config.ClaudeChecker == nil {
		config.ClaudeChecker = claude.NewRealChecker(config.TmuxChecker)
	}

	return &Manager{
		config:   config,
		eventBus: events.NewBus(),
		dataFile: filepath.Join(config.DataDir, "sessions.json"),
	}
}

// EventBus returns the event bus for subscribing to events
func (m *Manager) EventBus() <-chan types.Event {
	return m.eventBus.Subscribe()
}

// DeriveFreshSessions loads core sessions and derives complete state from external systems
func (m *Manager) DeriveFreshSessions() ([]types.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cores, err := m.loadCoreSessions()
	if err != nil {
		return nil, fmt.Errorf("failed to load core sessions: %w", err)
	}

	sessions := make([]types.Session, len(cores))
	for i, core := range cores {
		sessions[i] = m.deriveSession(core)
	}

	return sessions, nil
}

// CreateSession creates a new session with all required resources
func (m *Manager) CreateSession(name string) error {
	// Validate session name
	if err := validateSessionName(name); err != nil {
		return fmt.Errorf("invalid session name: %w", err)
	}

	// Emit immediate event for UI feedback
	m.eventBus.Publish(types.SessionCreationStarted{
		Name: name,
	})

	// Generate core session
	core := types.CoreSession{
		ID:           generateSessionID(),
		Name:         name,
		WorktreePath: filepath.Join(m.config.DataDir, "worktrees", name),
		TmuxSession:  fmt.Sprintf("cwt-%s", name),
		CreatedAt:    time.Now(),
	}

	// Check for duplicate session name
	if err := m.checkDuplicateName(name); err != nil {
		m.eventBus.Publish(types.SessionCreationFailed{
			Name:  name,
			Error: err.Error(),
		})
		return err
	}

	// Create external resources with rollback on failure
	if err := m.createExternalResources(core); err != nil {
		m.eventBus.Publish(types.SessionCreationFailed{
			Name:  name,
			Error: err.Error(),
		})
		return err
	}

	// Save to persistent storage
	if err := m.addCoreSession(core); err != nil {
		// Rollback external resources
		m.cleanupExternalResources(core)
		m.eventBus.Publish(types.SessionCreationFailed{
			Name:  name,
			Error: err.Error(),
		})
		return fmt.Errorf("failed to save session: %w", err)
	}

	// Emit success event with derived session
	session := m.deriveSession(core)
	m.eventBus.Publish(types.SessionCreated{Session: session})

	return nil
}

// DeleteSession removes a session and all its resources
func (m *Manager) DeleteSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cores, err := m.loadCoreSessions()
	if err != nil {
		return fmt.Errorf("failed to load sessions: %w", err)
	}

	// Find session to delete
	var sessionToDelete *types.CoreSession
	var newCores []types.CoreSession

	for _, core := range cores {
		if core.ID == sessionID {
			sessionToDelete = &core
		} else {
			newCores = append(newCores, core)
		}
	}

	if sessionToDelete == nil {
		err := fmt.Errorf("session with ID %s not found", sessionID)
		m.eventBus.Publish(types.SessionDeletionFailed{
			SessionID: sessionID,
			Error:     err.Error(),
		})
		return err
	}

	// Clean up external resources
	m.cleanupExternalResources(*sessionToDelete)

	// Save updated session list
	if err := m.saveCoreSessions(newCores); err != nil {
		err := fmt.Errorf("failed to save updated sessions: %w", err)
		m.eventBus.Publish(types.SessionDeletionFailed{
			SessionID: sessionID,
			Error:     err.Error(),
		})
		return err
	}

	// Emit success event
	m.eventBus.Publish(types.SessionDeleted{SessionID: sessionID})

	return nil
}

// FindStaleSessions returns sessions that have dead tmux sessions
func (m *Manager) FindStaleSessions() ([]types.Session, error) {
	sessions, err := m.DeriveFreshSessions()
	if err != nil {
		return nil, err
	}

	var stale []types.Session
	for _, session := range sessions {
		if !session.IsAlive {
			stale = append(stale, session)
		}
	}

	return stale, nil
}

// Private methods

func (m *Manager) deriveSession(core types.CoreSession) types.Session {
	session := types.Session{
		Core:      core,
		IsAlive:   m.config.TmuxChecker.IsSessionAlive(core.TmuxSession),
		GitStatus: m.config.GitChecker.GetStatus(core.WorktreePath),
	}

	// Load Claude status from session state file (preferred) or fallback to checker
	if sessionState, err := types.LoadSessionState(m.config.DataDir, core.ID); err == nil && sessionState != nil {
		session.ClaudeStatus = types.GetClaudeStatusFromState(sessionState)
	} else {
		// Fallback to old JSONL scanning if no session state
		session.ClaudeStatus = m.config.ClaudeChecker.GetStatus(core.WorktreePath)
	}

	// Calculate last activity from available timestamps
	session.LastActivity = m.calculateLastActivity(session)

	return session
}

func (m *Manager) calculateLastActivity(session types.Session) time.Time {
	lastActivity := session.Core.CreatedAt

	// Consider Claude activity
	if !session.ClaudeStatus.LastMessage.IsZero() && session.ClaudeStatus.LastMessage.After(lastActivity) {
		lastActivity = session.ClaudeStatus.LastMessage
	}

	// Consider git activity (would need file stat times, simplified for now)
	// In a full implementation, you'd check git log timestamps

	return lastActivity
}

func (m *Manager) loadCoreSessions() ([]types.CoreSession, error) {
	if _, err := os.Stat(m.dataFile); os.IsNotExist(err) {
		return []types.CoreSession{}, nil
	}

	data, err := os.ReadFile(m.dataFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read sessions file: %w", err)
	}

	var sessionData types.SessionData
	if err := json.Unmarshal(data, &sessionData); err != nil {
		return nil, fmt.Errorf("sessions file corrupted: %w", err)
	}

	return sessionData.Sessions, nil
}

func (m *Manager) saveCoreSessions(sessions []types.CoreSession) error {
	// Ensure data directory exists
	if err := os.MkdirAll(m.config.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	sessionData := types.SessionData{Sessions: sessions}
	data, err := json.MarshalIndent(sessionData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sessions: %w", err)
	}

	// Atomic write using temporary file
	tempFile := m.dataFile + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tempFile, m.dataFile); err != nil {
		os.Remove(tempFile) // Cleanup temp file
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func (m *Manager) addCoreSession(core types.CoreSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessions, err := m.loadCoreSessions()
	if err != nil {
		return err
	}

	sessions = append(sessions, core)
	return m.saveCoreSessions(sessions)
}

func (m *Manager) checkDuplicateName(name string) error {
	sessions, err := m.loadCoreSessions()
	if err != nil {
		return err
	}

	for _, session := range sessions {
		if session.Name == name {
			return fmt.Errorf("session with name '%s' already exists", name)
		}
	}

	return nil
}

func (m *Manager) createExternalResources(core types.CoreSession) error {
	// Validate git repository first
	if err := m.config.GitChecker.IsValidRepository(""); err != nil {
		return fmt.Errorf("git repository validation failed: %w", err)
	}

	// Create git worktree
	if err := m.config.GitChecker.CreateWorktree(core.Name, core.WorktreePath); err != nil {
		return fmt.Errorf("failed to create git worktree: %w", err)
	}

	// Create Claude settings with hooks in the worktree
	if err := m.createClaudeSettings(core.WorktreePath, core.ID); err != nil {
		// Rollback git worktree
		m.config.GitChecker.RemoveWorktree(core.WorktreePath)
		return fmt.Errorf("failed to create Claude settings: %w", err)
	}

	// Create tmux session
	// Check if claude is available, otherwise create session without it
	var command string
	if claudeExec := m.findClaudeExecutable(); claudeExec != "" {
		command = claudeExec
	}

	err := m.config.TmuxChecker.CreateSession(core.TmuxSession, core.WorktreePath, command)
	if err != nil {
		// Rollback git worktree
		m.config.GitChecker.RemoveWorktree(core.WorktreePath)
		return fmt.Errorf("failed to create tmux session: %w", err)
	}

	return nil
}

func (m *Manager) cleanupExternalResources(core types.CoreSession) {
	// Kill tmux session (ignore errors)
	m.config.TmuxChecker.KillSession(core.TmuxSession)

	// Remove git worktree (ignore errors)
	m.config.GitChecker.RemoveWorktree(core.WorktreePath)

	// Remove session state file (ignore errors)
	types.RemoveSessionState(m.config.DataDir, core.ID)
}

func generateSessionID() string {
	return fmt.Sprintf("session-%d", time.Now().UnixNano())
}

// createClaudeSettings creates a settings.json file in the worktree with CWT hooks configured
func (m *Manager) createClaudeSettings(worktreePath, sessionID string) error {
	claudeDir := filepath.Join(worktreePath, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Ensure .claude directory exists
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("failed to create .claude directory: %w", err)
	}

	// Get the current cwt executable path
	cwtPath := m.getCwtExecutablePath()

	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"Notification": []map[string]interface{}{
				{
					"matcher": "",
					"hooks": []map[string]interface{}{
						{
							"type":    "command",
							"command": fmt.Sprintf("%s __hook %s notification", cwtPath, sessionID),
						},
					},
				},
			},
			"Stop": []map[string]interface{}{
				{
					"matcher": "",
					"hooks": []map[string]interface{}{
						{
							"type":    "command",
							"command": fmt.Sprintf("%s __hook %s stop", cwtPath, sessionID),
						},
					},
				},
			},
			"PreToolUse": []map[string]interface{}{
				{
					"matcher": "",
					"hooks": []map[string]interface{}{
						{
							"type":    "command",
							"command": fmt.Sprintf("%s __hook %s pre_tool_use", cwtPath, sessionID),
						},
					},
				},
			},
			"PostToolUse": []map[string]interface{}{
				{
					"matcher": "",
					"hooks": []map[string]interface{}{
						{
							"type":    "command",
							"command": fmt.Sprintf("%s __hook %s post_tool_use", cwtPath, sessionID),
						},
					},
				},
			},
			"SubagentStop": []map[string]interface{}{
				{
					"matcher": "",
					"hooks": []map[string]interface{}{
						{
							"type":    "command",
							"command": fmt.Sprintf("%s __hook %s subagent_stop", cwtPath, sessionID),
						},
					},
				},
			},
			"PreCompact": []map[string]interface{}{
				{
					"matcher": "",
					"hooks": []map[string]interface{}{
						{
							"type":    "command",
							"command": fmt.Sprintf("%s __hook %s pre_compact", cwtPath, sessionID),
						},
					},
				},
			},
		},
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal Claude settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write Claude settings file: %w", err)
	}

	return nil
}

// getCwtExecutablePath determines the best path to use for cwt executable
func (m *Manager) getCwtExecutablePath() string {
	// First, try to find cwt in PATH (most reliable for installed binaries)
	if path, err := exec.LookPath("cwt"); err == nil {
		return path
	}

	// Check if we're running from go run (has temp executable path)
	if execPath, err := os.Executable(); err == nil {
		// If it's a temp path from go run, use absolute path to "go run cmd/cwt/main.go"
		if strings.Contains(execPath, "go-build") || strings.Contains(execPath, "/tmp/") {
			// Get current working directory to build absolute path
			if wd, err := os.Getwd(); err == nil {
				// Check if we're in the cwt project directory
				mainGoPath := filepath.Join(wd, "cmd/cwt/main.go")
				if _, err := os.Stat(mainGoPath); err == nil {
					return fmt.Sprintf("cd %s && go run cmd/cwt/main.go", wd)
				}
			}
		} else {
			// It's a real executable path
			return execPath
		}
	}

	// Final fallback to "cwt" in PATH
	return "cwt"
}

// findClaudeExecutable searches for claude in common installation paths
func (m *Manager) findClaudeExecutable() string {
	// Check common installation paths
	claudePaths := []string{
		"claude",
		os.ExpandEnv("$HOME/.claude/local/claude"),
		os.ExpandEnv("$HOME/.claude/local/node_modules/.bin/claude"),
		"/usr/local/bin/claude",
	}

	for _, path := range claudePaths {
		cmd := exec.Command(path, "--version")
		if err := cmd.Run(); err == nil {
			return path
		}
	}

	return ""
}


// GetDataDir returns the data directory path
func (m *Manager) GetDataDir() string {
	return m.config.DataDir
}

// GetTmuxChecker returns the tmux checker for direct access
func (m *Manager) GetTmuxChecker() tmux.Checker {
	return m.config.TmuxChecker
}

// GetClaudeChecker returns the claude checker for direct access
func (m *Manager) GetClaudeChecker() claude.Checker {
	return m.config.ClaudeChecker
}

// Close cleans up the manager resources
func (m *Manager) Close() {
	m.eventBus.Close()
}

