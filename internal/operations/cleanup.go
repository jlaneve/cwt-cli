package operations

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jlaneve/cwt-cli/internal/state"
)

// CleanupStats tracks the results of a cleanup operation
type CleanupStats struct {
	StaleSessions     int
	OrphanedTmux      int
	OrphanedWorktrees int
	Cleaned           int
	Failed            int
	Errors            []string
}

// CleanupOperations provides business logic for cleanup operations
type CleanupOperations struct {
	stateManager *state.Manager
}

// NewCleanupOperations creates a new CleanupOperations instance
func NewCleanupOperations(sm *state.Manager) *CleanupOperations {
	return &CleanupOperations{
		stateManager: sm,
	}
}

// FindAndCleanupStaleResources finds and optionally cleans up stale CWT resources
func (c *CleanupOperations) FindAndCleanupStaleResources(dryRun bool) (*CleanupStats, error) {
	stats := &CleanupStats{
		Errors: make([]string, 0),
	}

	// Find stale sessions
	staleSessions, err := c.stateManager.FindStaleSessions()
	if err != nil {
		return nil, fmt.Errorf("failed to find stale sessions: %w", err)
	}
	stats.StaleSessions = len(staleSessions)

	// Clean up stale sessions
	for _, session := range staleSessions {
		if dryRun {
			fmt.Printf("Would clean up stale session: %s (tmux: %s, worktree: %s)\n",
				session.Core.Name, session.Core.TmuxSession, session.Core.WorktreePath)
			continue
		}

		if err := c.stateManager.DeleteSession(session.Core.ID); err != nil {
			stats.Failed++
			errMsg := fmt.Sprintf("Failed to delete session %s: %v", session.Core.Name, err)
			stats.Errors = append(stats.Errors, errMsg)
		} else {
			stats.Cleaned++
		}
	}

	// Find orphaned tmux sessions
	orphanedTmux, err := c.findOrphanedTmuxSessions()
	if err != nil {
		return stats, fmt.Errorf("failed to find orphaned tmux sessions: %w", err)
	}
	stats.OrphanedTmux = len(orphanedTmux)

	// Clean up orphaned tmux sessions
	for _, tmuxSession := range orphanedTmux {
		if dryRun {
			fmt.Printf("Would kill orphaned tmux session: %s\n", tmuxSession)
			continue
		}

		if err := c.killTmuxSession(tmuxSession); err != nil {
			stats.Failed++
			errMsg := fmt.Sprintf("Failed to kill tmux session %s: %v", tmuxSession, err)
			stats.Errors = append(stats.Errors, errMsg)
		} else {
			stats.Cleaned++
		}
	}

	// Find orphaned worktrees
	orphanedWorktrees, err := c.findOrphanedWorktrees()
	if err != nil {
		return stats, fmt.Errorf("failed to find orphaned worktrees: %w", err)
	}
	stats.OrphanedWorktrees = len(orphanedWorktrees)

	// Clean up orphaned worktrees
	for _, worktree := range orphanedWorktrees {
		if dryRun {
			fmt.Printf("Would remove orphaned worktree: %s\n", worktree)
			continue
		}

		if err := c.removeWorktree(worktree); err != nil {
			stats.Failed++
			errMsg := fmt.Sprintf("Failed to remove worktree %s: %v", worktree, err)
			stats.Errors = append(stats.Errors, errMsg)
		} else {
			stats.Cleaned++
		}
	}

	return stats, nil
}

// findOrphanedTmuxSessions finds tmux sessions that start with "cwt-" but don't have corresponding CWT sessions
func (c *CleanupOperations) findOrphanedTmuxSessions() ([]string, error) {
	// Get all tmux sessions
	tmuxSessions, err := c.stateManager.GetTmuxChecker().ListSessions()
	if err != nil {
		return nil, fmt.Errorf("failed to list tmux sessions: %w", err)
	}

	// Get current CWT sessions
	sessions, err := c.stateManager.DeriveFreshSessions()
	if err != nil {
		return nil, fmt.Errorf("failed to get current sessions: %w", err)
	}

	// Create a map of active CWT tmux session names
	activeTmux := make(map[string]bool)
	for _, session := range sessions {
		activeTmux[session.Core.TmuxSession] = true
	}

	// Find orphaned sessions
	var orphaned []string
	for _, tmuxSession := range tmuxSessions {
		if strings.HasPrefix(tmuxSession, "cwt-") && !activeTmux[tmuxSession] {
			orphaned = append(orphaned, tmuxSession)
		}
	}

	return orphaned, nil
}

// findOrphanedWorktrees finds git worktrees in .cwt/worktrees/ that don't have corresponding CWT sessions
func (c *CleanupOperations) findOrphanedWorktrees() ([]string, error) {
	worktreesDir := filepath.Join(c.stateManager.GetDataDir(), "worktrees")

	// Check if worktrees directory exists
	if _, err := os.Stat(worktreesDir); os.IsNotExist(err) {
		return nil, nil // No worktrees directory means no orphaned worktrees
	}

	// Get all worktree directories
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read worktrees directory: %w", err)
	}

	// Get current CWT sessions
	sessions, err := c.stateManager.DeriveFreshSessions()
	if err != nil {
		return nil, fmt.Errorf("failed to get current sessions: %w", err)
	}

	// Create a map of active session names
	activeNames := make(map[string]bool)
	for _, session := range sessions {
		activeNames[session.Core.Name] = true
	}

	// Find orphaned worktrees
	var orphaned []string
	for _, entry := range entries {
		if entry.IsDir() && !activeNames[entry.Name()] {
			orphaned = append(orphaned, entry.Name())
		}
	}

	return orphaned, nil
}

// killTmuxSession kills a tmux session
func (c *CleanupOperations) killTmuxSession(sessionName string) error {
	return c.stateManager.GetTmuxChecker().KillSession(sessionName)
}

// removeWorktree removes a git worktree
func (c *CleanupOperations) removeWorktree(name string) error {
	// Security: Validate worktree name to prevent command injection
	if !isValidWorktreeName(name) {
		return fmt.Errorf("invalid worktree name: %s", name)
	}

	worktreePath := filepath.Join(c.stateManager.GetDataDir(), "worktrees", name)

	// Security: Validate that the path is within our data directory
	if !isPathWithinDataDir(worktreePath, c.stateManager.GetDataDir()) {
		return fmt.Errorf("worktree path outside data directory: %s", worktreePath)
	}

	// Use git worktree remove command
	cmd := exec.Command("git", "worktree", "remove", worktreePath)
	if err := cmd.Run(); err != nil {
		// If git worktree remove fails, try force removal
		cmd = exec.Command("git", "worktree", "remove", "--force", worktreePath)
		if err := cmd.Run(); err != nil {
			// If that also fails, remove the directory manually
			return os.RemoveAll(worktreePath)
		}
	}

	return nil
}

// isValidWorktreeName validates that a worktree name is safe
func isValidWorktreeName(name string) bool {
	// Reject empty names
	if name == "" {
		return false
	}
	// Reject paths with directory traversal patterns
	if strings.Contains(name, "..") {
		return false
	}
	// Reject paths with null bytes
	if strings.Contains(name, "\x00") {
		return false
	}
	// Reject names with shell metacharacters
	dangerousChars := []string{";", "&", "|", "$", "`", "(", ")", "{", "}", "[", "]", "*", "?", "<", ">", "~", " ", "\t", "\n", "\r"}
	for _, char := range dangerousChars {
		if strings.Contains(name, char) {
			return false
		}
	}
	// Reject names starting with dash (could be interpreted as flags)
	if strings.HasPrefix(name, "-") {
		return false
	}
	return true
}

// isPathWithinDataDir validates that a path is within the expected data directory
func isPathWithinDataDir(path, dataDir string) bool {
	// Clean and resolve both paths
	cleanPath := filepath.Clean(path)
	cleanDataDir := filepath.Clean(dataDir)

	// Make sure the path starts with the data directory
	expectedPrefix := filepath.Join(cleanDataDir, "worktrees") + string(filepath.Separator)
	cleanPathWithSep := cleanPath + string(filepath.Separator)

	return strings.HasPrefix(cleanPathWithSep, expectedPrefix) || cleanPath == filepath.Join(cleanDataDir, "worktrees")
}
