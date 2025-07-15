package operations

import (
	"path/filepath"
	"testing"

	"github.com/jlaneve/cwt-cli/internal/clients/claude"
	"github.com/jlaneve/cwt-cli/internal/clients/git"
	"github.com/jlaneve/cwt-cli/internal/clients/tmux"
	"github.com/jlaneve/cwt-cli/internal/state"
)

func TestCleanupOperations_FindAndCleanupStaleResources_NoOrphans(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".cwt")

	tmuxChecker := tmux.NewMockChecker()
	config := state.Config{
		DataDir:       dataDir,
		TmuxChecker:   tmuxChecker,
		GitChecker:    git.NewMockChecker(),
		ClaudeChecker: claude.NewMockChecker(),
		BaseBranch:    "main",
	}

	manager := state.NewManager(config)
	defer manager.Close()

	cleanupOps := NewCleanupOperations(manager)

	// Test with no sessions (should find no orphans)
	stats, err := cleanupOps.FindAndCleanupStaleResources(true) // dry run
	if err != nil {
		t.Fatalf("FindAndCleanupStaleResources() error = %v", err)
	}

	if stats.StaleSessions != 0 {
		t.Errorf("Expected 0 stale sessions, got %d", stats.StaleSessions)
	}
	if stats.OrphanedTmux != 0 {
		t.Errorf("Expected 0 orphaned tmux sessions, got %d", stats.OrphanedTmux)
	}
	if stats.OrphanedWorktrees != 0 {
		t.Errorf("Expected 0 orphaned worktrees, got %d", stats.OrphanedWorktrees)
	}
}

func TestCleanupOperations_FindAndCleanupStaleResources_WithStaleSession(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".cwt")

	tmuxChecker := tmux.NewMockChecker()
	config := state.Config{
		DataDir:       dataDir,
		TmuxChecker:   tmuxChecker,
		GitChecker:    git.NewMockChecker(),
		ClaudeChecker: claude.NewMockChecker(),
		BaseBranch:    "main",
	}

	manager := state.NewManager(config)
	defer manager.Close()

	// Create a session
	sessionOps := NewSessionOperations(manager)
	err := sessionOps.CreateSession("stale-session")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Make the tmux session appear dead
	tmuxChecker.SetSessionAlive("cwt-stale-session", false)

	cleanupOps := NewCleanupOperations(manager)

	// Test dry run - should find stale session but not clean it
	stats, err := cleanupOps.FindAndCleanupStaleResources(true)
	if err != nil {
		t.Fatalf("FindAndCleanupStaleResources(dry run) error = %v", err)
	}

	if stats.StaleSessions != 1 {
		t.Errorf("Expected 1 stale session, got %d", stats.StaleSessions)
	}
	if stats.Cleaned != 0 {
		t.Errorf("Expected 0 cleaned in dry run, got %d", stats.Cleaned)
	}

	// Verify session still exists
	sessions, _ := sessionOps.GetAllSessions()
	if len(sessions) != 1 {
		t.Errorf("Expected session to still exist after dry run, got %d sessions", len(sessions))
	}

	// Test actual cleanup
	stats, err = cleanupOps.FindAndCleanupStaleResources(false)
	if err != nil {
		t.Fatalf("FindAndCleanupStaleResources(cleanup) error = %v", err)
	}

	if stats.StaleSessions != 1 {
		t.Errorf("Expected 1 stale session found, got %d", stats.StaleSessions)
	}
	if stats.Cleaned < 1 {
		t.Errorf("Expected at least 1 session cleaned, got %d", stats.Cleaned)
	}

	// Verify session was deleted
	sessions, _ = sessionOps.GetAllSessions()
	if len(sessions) != 0 {
		t.Errorf("Expected session to be deleted after cleanup, got %d sessions", len(sessions))
	}
}

func TestCleanupOperations_FindAndCleanupStaleResources_WithOrphanedTmux(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".cwt")

	tmuxChecker := tmux.NewMockChecker()

	// Add orphaned tmux sessions
	tmuxChecker.SetSessionAlive("cwt-orphaned-1", true)
	tmuxChecker.SetSessionAlive("cwt-orphaned-2", true)
	tmuxChecker.SetSessionAlive("non-cwt-session", true) // Should be ignored

	config := state.Config{
		DataDir:       dataDir,
		TmuxChecker:   tmuxChecker,
		GitChecker:    git.NewMockChecker(),
		ClaudeChecker: claude.NewMockChecker(),
		BaseBranch:    "main",
	}

	manager := state.NewManager(config)
	defer manager.Close()

	cleanupOps := NewCleanupOperations(manager)

	// Test dry run - should find orphaned tmux sessions
	stats, err := cleanupOps.FindAndCleanupStaleResources(true)
	if err != nil {
		t.Fatalf("FindAndCleanupStaleResources(dry run) error = %v", err)
	}

	if stats.OrphanedTmux != 2 {
		t.Errorf("Expected 2 orphaned tmux sessions, got %d", stats.OrphanedTmux)
	}
	if stats.Cleaned != 0 {
		t.Errorf("Expected 0 cleaned in dry run, got %d", stats.Cleaned)
	}

	// Verify tmux sessions still exist
	if len(tmuxChecker.KilledSessions) != 0 {
		t.Errorf("Expected no sessions killed in dry run, got %d", len(tmuxChecker.KilledSessions))
	}

	// Test actual cleanup
	stats, err = cleanupOps.FindAndCleanupStaleResources(false)
	if err != nil {
		t.Fatalf("FindAndCleanupStaleResources(cleanup) error = %v", err)
	}

	if stats.OrphanedTmux != 2 {
		t.Errorf("Expected 2 orphaned tmux sessions found, got %d", stats.OrphanedTmux)
	}
	if stats.Cleaned != 2 {
		t.Errorf("Expected 2 sessions cleaned, got %d", stats.Cleaned)
	}

	// Verify tmux sessions were killed
	if len(tmuxChecker.KilledSessions) != 2 {
		t.Errorf("Expected 2 sessions killed, got %d", len(tmuxChecker.KilledSessions))
	}
}

func TestCleanupOperations_CleanupStats(t *testing.T) {
	stats := &CleanupStats{
		StaleSessions:     2,
		OrphanedTmux:      1,
		OrphanedWorktrees: 0,
		Cleaned:           2,
		Failed:            1,
		Errors:            []string{"Failed to delete session: permission denied"},
	}

	if len(stats.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(stats.Errors))
	}

	if stats.Errors[0] != "Failed to delete session: permission denied" {
		t.Errorf("Unexpected error message: %s", stats.Errors[0])
	}

	totalFound := stats.StaleSessions + stats.OrphanedTmux + stats.OrphanedWorktrees
	if totalFound != 3 {
		t.Errorf("Expected total found = 3, got %d", totalFound)
	}
}
