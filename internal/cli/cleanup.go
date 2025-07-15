package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/jlaneve/cwt-cli/internal/state"
)

func newCleanupCmd() *cobra.Command {
	var dryRun bool
	
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Remove orphaned sessions and resources",
		Long: `Clean up orphaned CWT resources:
- Sessions with dead tmux sessions
- Unused git worktrees
- Stale session metadata

This helps maintain a clean state after crashes or manual tmux session termination.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCleanupCmd(dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be cleaned up without actually doing it")

	return cmd
}

func runCleanupCmd(dryRun bool) error {
	sm, err := createStateManager()
	if err != nil {
		return err
	}
	defer sm.Close()

	fmt.Println("🔍 Scanning for orphaned resources...")

	cleanupStats := struct {
		staleSessions     int
		orphanedTmux      int
		orphanedWorktrees int
		cleaned           int
		failed            int
	}{}

	// 1. Find stale sessions (sessions with dead tmux sessions)
	staleSessions, err := sm.FindStaleSessions()
	if err != nil {
		return fmt.Errorf("failed to find stale sessions: %w", err)
	}
	cleanupStats.staleSessions = len(staleSessions)

	// 2. Find orphaned tmux sessions (cwt-* sessions not in our metadata)
	orphanedTmux, err := findOrphanedTmuxSessions(sm)
	if err != nil {
		fmt.Printf("Warning: failed to scan tmux sessions: %v\n", err)
	} else {
		cleanupStats.orphanedTmux = len(orphanedTmux)
	}

	// 3. Find orphaned worktrees (.cwt/worktrees/* not in our metadata)
	orphanedWorktrees, err := findOrphanedWorktrees(sm)
	if err != nil {
		fmt.Printf("Warning: failed to scan worktrees: %v\n", err)
	} else {
		cleanupStats.orphanedWorktrees = len(orphanedWorktrees)
	}

	// Show what was found
	totalOrphans := cleanupStats.staleSessions + cleanupStats.orphanedTmux + cleanupStats.orphanedWorktrees
	if totalOrphans == 0 {
		fmt.Println("✅ No orphaned resources found. Everything looks clean!")
		return nil
	}

	fmt.Printf("\nFound orphaned resources:\n")
	if cleanupStats.staleSessions > 0 {
		fmt.Printf("  📂 %d stale session(s) with dead tmux\n", cleanupStats.staleSessions)
	}
	if cleanupStats.orphanedTmux > 0 {
		fmt.Printf("  🔧 %d orphaned tmux session(s)\n", cleanupStats.orphanedTmux)
	}
	if cleanupStats.orphanedWorktrees > 0 {
		fmt.Printf("  🌳 %d orphaned git worktree(s)\n", cleanupStats.orphanedWorktrees)
	}
	fmt.Println()

	// Show details
	if cleanupStats.staleSessions > 0 {
		fmt.Printf("Stale sessions:\n")
		for _, session := range staleSessions {
			fmt.Printf("  🗑️  %s (tmux: %s, worktree: %s)\n", 
				session.Core.Name, session.Core.TmuxSession, session.Core.WorktreePath)
		}
		fmt.Println()
	}

	if cleanupStats.orphanedTmux > 0 {
		fmt.Printf("Orphaned tmux sessions:\n")
		for _, tmuxSession := range orphanedTmux {
			fmt.Printf("  🔧 %s\n", tmuxSession)
		}
		fmt.Println()
	}

	if cleanupStats.orphanedWorktrees > 0 {
		fmt.Printf("Orphaned worktrees:\n")
		for _, worktree := range orphanedWorktrees {
			fmt.Printf("  🌳 %s\n", worktree)
		}
		fmt.Println()
	}

	if dryRun {
		fmt.Println("🔍 Dry run mode - no changes made.")
		fmt.Printf("Run 'cwt cleanup' to actually clean up these %d resource(s).\n", totalOrphans)
		return nil
	}

	// Clean up stale sessions
	if cleanupStats.staleSessions > 0 {
		fmt.Printf("Cleaning up %d stale session(s)...\n", cleanupStats.staleSessions)
		for _, session := range staleSessions {
			fmt.Printf("  Cleaning session '%s'...\n", session.Core.Name)
			
			if err := sm.DeleteSession(session.Core.ID); err != nil {
				fmt.Printf("    ❌ Failed: %v\n", err)
				cleanupStats.failed++
			} else {
				fmt.Printf("    ✅ Cleaned\n")
				cleanupStats.cleaned++
			}
		}
		fmt.Println()
	}

	// Clean up orphaned tmux sessions
	if cleanupStats.orphanedTmux > 0 {
		fmt.Printf("Cleaning up %d orphaned tmux session(s)...\n", cleanupStats.orphanedTmux)
		tmuxChecker := sm.GetTmuxChecker()
		for _, tmuxSession := range orphanedTmux {
			fmt.Printf("  Killing tmux session '%s'...\n", tmuxSession)
			
			if err := tmuxChecker.KillSession(tmuxSession); err != nil {
				fmt.Printf("    ❌ Failed: %v\n", err)
				cleanupStats.failed++
			} else {
				fmt.Printf("    ✅ Killed\n")
				cleanupStats.cleaned++
			}
		}
		fmt.Println()
	}

	// Clean up orphaned worktrees
	if cleanupStats.orphanedWorktrees > 0 {
		fmt.Printf("Cleaning up %d orphaned worktree(s)...\n", cleanupStats.orphanedWorktrees)
		for _, worktree := range orphanedWorktrees {
			fmt.Printf("  Removing worktree '%s'...\n", worktree)
			
			if err := removeWorktreeWithFallback(worktree); err != nil {
				fmt.Printf("    ❌ Failed: %v\n", err)
				cleanupStats.failed++
			} else {
				fmt.Printf("    ✅ Removed\n")
				cleanupStats.cleaned++
			}
		}
		fmt.Println()
	}

	fmt.Printf("🧹 Cleanup complete: %d cleaned, %d failed\n", cleanupStats.cleaned, cleanupStats.failed)

	if cleanupStats.failed > 0 {
		return fmt.Errorf("some resources could not be cleaned up")
	}

	return nil
}

// findOrphanedTmuxSessions finds tmux sessions with "cwt-" prefix that aren't tracked by CWT
func findOrphanedTmuxSessions(sm *state.Manager) ([]string, error) {
	// Get all tmux sessions
	tmuxChecker := sm.GetTmuxChecker()
	allSessions, err := tmuxChecker.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("failed to list tmux sessions: %w", err)
	}

	// Get all sessions tracked by CWT
	cwtSessions, err := sm.DeriveFreshSessions()
	if err != nil {
		return nil, fmt.Errorf("failed to get CWT sessions: %w", err)
	}

	// Build map of tracked tmux sessions
	trackedTmux := make(map[string]bool)
	for _, session := range cwtSessions {
		trackedTmux[session.Core.TmuxSession] = true
	}

	// Find orphaned tmux sessions
	var orphaned []string
	for _, tmuxSession := range allSessions {
		if strings.HasPrefix(tmuxSession, "cwt-") && !trackedTmux[tmuxSession] {
			orphaned = append(orphaned, tmuxSession)
		}
	}

	return orphaned, nil
}

// findOrphanedWorktrees finds git worktrees in .cwt/worktrees/ that aren't tracked by CWT
func findOrphanedWorktrees(sm *state.Manager) ([]string, error) {
	// Get all git worktrees
	worktreesDir := filepath.Join(sm.GetDataDir(), "worktrees")
	if _, err := os.Stat(worktreesDir); os.IsNotExist(err) {
		return []string{}, nil // No worktrees directory
	}

	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read worktrees directory: %w", err)
	}

	// Get all sessions tracked by CWT
	cwtSessions, err := sm.DeriveFreshSessions()
	if err != nil {
		return nil, fmt.Errorf("failed to get CWT sessions: %w", err)
	}

	// Build map of tracked worktree paths
	trackedWorktrees := make(map[string]bool)
	for _, session := range cwtSessions {
		// Normalize path for comparison
		absPath, _ := filepath.Abs(session.Core.WorktreePath)
		trackedWorktrees[absPath] = true
	}

	// Find orphaned worktrees
	var orphaned []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		worktreePath := filepath.Join(worktreesDir, entry.Name())
		absPath, _ := filepath.Abs(worktreePath)
		
		if !trackedWorktrees[absPath] {
			orphaned = append(orphaned, worktreePath)
		}
	}

	return orphaned, nil
}

// removeWorktreeWithFallback tries to remove a worktree using git, falling back to directory removal
func removeWorktreeWithFallback(worktreePath string) error {
	// First try to remove with git worktree command
	cmd := exec.Command("git", "worktree", "remove", worktreePath, "--force")
	if err := cmd.Run(); err == nil {
		return nil // Successfully removed with git
	}

	// Fallback to manual directory removal
	if err := os.RemoveAll(worktreePath); err != nil {
		return fmt.Errorf("failed to remove worktree directory: %w", err)
	}

	// Try to clean up git references
	cmd = exec.Command("git", "worktree", "prune")
	cmd.Run() // Ignore errors - this is just cleanup

	return nil
}