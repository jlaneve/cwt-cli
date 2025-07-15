package cli

import (
	"fmt"

	"github.com/jlaneve/cwt-cli/internal/operations"
	"github.com/spf13/cobra"
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

	// Use operations layer for cleanup
	cleanupOps := operations.NewCleanupOperations(sm)
	stats, err := cleanupOps.FindAndCleanupStaleResources(dryRun)
	if err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	// Show what was found
	totalOrphans := stats.StaleSessions + stats.OrphanedTmux + stats.OrphanedWorktrees
	if totalOrphans == 0 {
		fmt.Println("✅ No orphaned resources found. Everything looks clean!")
		return nil
	}

	fmt.Printf("\nFound orphaned resources:\n")
	if stats.StaleSessions > 0 {
		fmt.Printf("  📂 %d stale session(s) with dead tmux\n", stats.StaleSessions)
	}
	if stats.OrphanedTmux > 0 {
		fmt.Printf("  🔧 %d orphaned tmux session(s)\n", stats.OrphanedTmux)
	}
	if stats.OrphanedWorktrees > 0 {
		fmt.Printf("  🌳 %d orphaned git worktree(s)\n", stats.OrphanedWorktrees)
	}
	fmt.Println()

	if dryRun {
		fmt.Println("🔍 Dry run mode - no changes made.")
		fmt.Printf("Run 'cwt cleanup' to actually clean up these %d resource(s).\n", totalOrphans)
		return nil
	}

	// Show cleanup results
	fmt.Printf("🧹 Cleanup complete!\n")
	fmt.Printf("  ✅ Cleaned: %d\n", stats.Cleaned)
	if stats.Failed > 0 {
		fmt.Printf("  ❌ Failed: %d\n", stats.Failed)
		for _, errMsg := range stats.Errors {
			fmt.Printf("    - %s\n", errMsg)
		}
	}

	return nil
}