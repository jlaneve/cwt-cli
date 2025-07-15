package operations

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jlaneve/cwt-cli/internal/clients/claude"
	"github.com/jlaneve/cwt-cli/internal/clients/git"
	"github.com/jlaneve/cwt-cli/internal/clients/tmux"
	"github.com/jlaneve/cwt-cli/internal/state"
)

func TestSessionOperations_CreateSession(t *testing.T) {
	// Create temp directory for testing
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".cwt")

	// Create manager with mocks
	config := state.Config{
		DataDir:       dataDir,
		TmuxChecker:   tmux.NewMockChecker(),
		GitChecker:    git.NewMockChecker(),
		ClaudeChecker: claude.NewMockChecker(),
		BaseBranch:    "main",
	}

	manager := state.NewManager(config)
	defer manager.Close()

	sessionOps := NewSessionOperations(manager)

	// Test creating a session
	err := sessionOps.CreateSession("test-session")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Verify session was created
	sessions, err := sessionOps.GetAllSessions()
	if err != nil {
		t.Fatalf("GetAllSessions() error = %v", err)
	}

	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}

	if sessions[0].Core.Name != "test-session" {
		t.Errorf("Expected session name 'test-session', got %v", sessions[0].Core.Name)
	}
}

func TestSessionOperations_FindSessionByName(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".cwt")

	config := state.Config{
		DataDir:       dataDir,
		TmuxChecker:   tmux.NewMockChecker(),
		GitChecker:    git.NewMockChecker(),
		ClaudeChecker: claude.NewMockChecker(),
		BaseBranch:    "main",
	}

	manager := state.NewManager(config)
	defer manager.Close()

	sessionOps := NewSessionOperations(manager)

	// Create a session first
	err := sessionOps.CreateSession("findme-session")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Test finding existing session
	session, sessionID, err := sessionOps.FindSessionByName("findme-session")
	if err != nil {
		t.Fatalf("FindSessionByName() error = %v", err)
	}

	if session.Core.Name != "findme-session" {
		t.Errorf("Expected session name 'findme-session', got %v", session.Core.Name)
	}

	if sessionID == "" {
		t.Error("Expected non-empty session ID")
	}

	// Test finding non-existent session
	_, _, err = sessionOps.FindSessionByName("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent session")
	}
}

func TestSessionOperations_FindSessionByID(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".cwt")

	config := state.Config{
		DataDir:       dataDir,
		TmuxChecker:   tmux.NewMockChecker(),
		GitChecker:    git.NewMockChecker(),
		ClaudeChecker: claude.NewMockChecker(),
		BaseBranch:    "main",
	}

	manager := state.NewManager(config)
	defer manager.Close()

	sessionOps := NewSessionOperations(manager)

	// Create a session first
	err := sessionOps.CreateSession("findbyid-session")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Get the session ID
	sessions, err := sessionOps.GetAllSessions()
	if err != nil {
		t.Fatalf("GetAllSessions() error = %v", err)
	}
	sessionID := sessions[0].Core.ID

	// Test finding by ID
	session, err := sessionOps.FindSessionByID(sessionID)
	if err != nil {
		t.Fatalf("FindSessionByID() error = %v", err)
	}

	if session.Core.Name != "findbyid-session" {
		t.Errorf("Expected session name 'findbyid-session', got %v", session.Core.Name)
	}

	// Test finding non-existent ID
	_, err = sessionOps.FindSessionByID("nonexistent-id")
	if err == nil {
		t.Error("Expected error for non-existent session ID")
	}
}

func TestSessionOperations_DeleteSession(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".cwt")

	config := state.Config{
		DataDir:       dataDir,
		TmuxChecker:   tmux.NewMockChecker(),
		GitChecker:    git.NewMockChecker(),
		ClaudeChecker: claude.NewMockChecker(),
		BaseBranch:    "main",
	}

	manager := state.NewManager(config)
	defer manager.Close()

	sessionOps := NewSessionOperations(manager)

	// Create a session first
	err := sessionOps.CreateSession("delete-me")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Get session ID
	sessions, _ := sessionOps.GetAllSessions()
	sessionID := sessions[0].Core.ID

	// Delete the session
	err = sessionOps.DeleteSession(sessionID)
	if err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}

	// Verify session was deleted
	sessions, _ = sessionOps.GetAllSessions()
	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions after deletion, got %d", len(sessions))
	}
}

func TestFindClaudeExecutable(t *testing.T) {
	// This test checks that the function doesn't crash
	// We can't reliably test the actual finding logic without 
	// modifying PATH or creating fake executables
	result := FindClaudeExecutable()
	
	// Should return a string (empty if not found)
	if result == "" {
		t.Log("Claude executable not found in PATH (this is expected in test environment)")
	} else {
		t.Logf("Found Claude executable at: %s", result)
		
		// If we found something, verify it's actually executable
		if _, err := exec.LookPath(result); err != nil {
			t.Errorf("FindClaudeExecutable() returned %q but it's not in PATH: %v", result, err)
		}
	}
}

func TestSessionOperations_RecreateDeadSession(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".cwt")

	tmuxChecker := tmux.NewMockChecker()
	claudeChecker := claude.NewMockChecker()
	
	config := state.Config{
		DataDir:       dataDir,
		TmuxChecker:   tmuxChecker,
		GitChecker:    git.NewMockChecker(),
		ClaudeChecker: claudeChecker,
		BaseBranch:    "main",
	}

	manager := state.NewManager(config)
	defer manager.Close()

	sessionOps := NewSessionOperations(manager)

	// Create a session first
	err := sessionOps.CreateSession("recreate-test")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Get the session
	session, _, err := sessionOps.FindSessionByName("recreate-test")
	if err != nil {
		t.Fatalf("FindSessionByName() error = %v", err)
	}

	// Test recreating session (will only work if claude executable is available)
	err = sessionOps.RecreateDeadSession(session)
	
	// If claude is not available, expect specific error
	claudeExec := FindClaudeExecutable()
	if claudeExec == "" {
		if err == nil {
			t.Error("Expected error when claude executable not found")
		}
		return
	}

	// If claude is available, the operation should succeed
	if err != nil {
		t.Errorf("RecreateDeadSession() error = %v", err)
	}

	// Verify tmux session was created
	if len(tmuxChecker.CreatedSessions) != 2 { // One from initial creation, one from recreation
		t.Errorf("Expected 2 tmux sessions created, got %d", len(tmuxChecker.CreatedSessions))
	}
}