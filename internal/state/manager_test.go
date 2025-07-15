package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jlaneve/cwt-cli/internal/clients/claude"
	"github.com/jlaneve/cwt-cli/internal/clients/git"
	"github.com/jlaneve/cwt-cli/internal/clients/tmux"
)

func TestManager_CreateSession(t *testing.T) {
	// Create temp directory for testing
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".cwt")

	// Create manager with mocks
	config := Config{
		DataDir:       dataDir,
		TmuxChecker:   tmux.NewMockChecker(),
		GitChecker:    git.NewMockChecker(),
		ClaudeChecker: claude.NewMockChecker(),
		BaseBranch:    "main",
	}

	manager := NewManager(config)

	// Test creating a session
	err := manager.CreateSession("test-session")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Verify session was saved
	sessions, err := manager.DeriveFreshSessions()
	if err != nil {
		t.Fatalf("DeriveFreshSessions() error = %v", err)
	}

	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}

	if sessions[0].Core.Name != "test-session" {
		t.Errorf("Expected session name 'test-session', got %v", sessions[0].Core.Name)
	}
}

func TestManager_CreateSession_InvalidName(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".cwt")

	config := Config{
		DataDir:       dataDir,
		TmuxChecker:   tmux.NewMockChecker(),
		GitChecker:    git.NewMockChecker(),
		ClaudeChecker: claude.NewMockChecker(),
		BaseBranch:    "main",
	}

	manager := NewManager(config)

	// Test invalid session names
	invalidNames := []string{
		"",
		"session with spaces",
		"session~invalid",
		"123",
		"main",
	}

	for _, name := range invalidNames {
		err := manager.CreateSession(name)
		if err == nil {
			t.Errorf("CreateSession(%q) should return error", name)
		}
	}
}

func TestManager_DeleteSession(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".cwt")

	config := Config{
		DataDir:       dataDir,
		TmuxChecker:   tmux.NewMockChecker(),
		GitChecker:    git.NewMockChecker(),
		ClaudeChecker: claude.NewMockChecker(),
		BaseBranch:    "main",
	}

	manager := NewManager(config)

	// Create a session first
	err := manager.CreateSession("test-delete")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Get session ID
	sessions, _ := manager.DeriveFreshSessions()
	if len(sessions) != 1 {
		t.Fatal("Expected 1 session after creation")
	}
	sessionID := sessions[0].Core.ID

	// Delete the session
	err = manager.DeleteSession(sessionID)
	if err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}

	// Verify session was deleted
	sessions, _ = manager.DeriveFreshSessions()
	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions after deletion, got %d", len(sessions))
	}
}

func TestManager_FindStaleSessions(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".cwt")

	tmuxChecker := tmux.NewMockChecker()
	gitChecker := git.NewMockChecker()
	config := Config{
		DataDir:       dataDir,
		TmuxChecker:   tmuxChecker,
		GitChecker:    gitChecker,
		ClaudeChecker: claude.NewMockChecker(),
		BaseBranch:    "main",
	}

	manager := NewManager(config)

	// Create sessions
	err := manager.CreateSession("alive-session")
	if err != nil {
		t.Fatalf("CreateSession(alive-session) error = %v", err)
	}

	err = manager.CreateSession("dead-session")
	if err != nil {
		t.Fatalf("CreateSession(dead-session) error = %v", err)
	}

	// Mock tmux to report one session as dead
	tmuxChecker.SetSessionAlive("cwt-alive-session", true)
	tmuxChecker.SetSessionAlive("cwt-dead-session", false)

	// Find stale sessions
	staleSessions, err := manager.FindStaleSessions()
	if err != nil {
		t.Fatalf("FindStaleSessions() error = %v", err)
	}

	if len(staleSessions) != 1 {
		t.Errorf("Expected 1 stale session, got %d", len(staleSessions))
	}
	if len(staleSessions) > 0 && staleSessions[0].Core.Name != "dead-session" {
		t.Errorf("Expected 'dead-session' to be stale, got %v", staleSessions[0].Core.Name)
	}
}

func TestManager_LoadCoreSessions_CorruptedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".cwt")

	// Create directory
	os.MkdirAll(dataDir, 0755)

	// Write corrupted JSON
	sessionsFile := filepath.Join(dataDir, "sessions.json")
	os.WriteFile(sessionsFile, []byte("invalid json{"), 0644)

	config := Config{
		DataDir:       dataDir,
		TmuxChecker:   tmux.NewMockChecker(),
		GitChecker:    git.NewMockChecker(),
		ClaudeChecker: claude.NewMockChecker(),
		BaseBranch:    "main",
	}

	manager := NewManager(config)

	sessions, err := manager.DeriveFreshSessions()
	if err == nil {
		t.Error("Expected error for corrupted JSON")
	}
	if sessions != nil {
		t.Error("Expected nil sessions for corrupted JSON")
	}
}
