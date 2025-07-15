package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jlaneve/cwt-cli/internal/state"
	"github.com/jlaneve/cwt-cli/internal/types"
)

// newMergeCmd creates the 'cwt merge' command
func newMergeCmd() *cobra.Command {
	var target string
	var squash bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "merge <session-name>",
		Short: "Merge session changes back to target branch",
		Long: `Safely integrate session changes back to target branches with conflict resolution.

Examples:
  cwt merge my-session              # Interactive merge to current branch
  cwt merge my-session --target main  # Merge to specific target branch
  cwt merge my-session --squash     # Squash merge for clean history
  cwt merge my-session --dry-run    # Preview merge without executing`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sm, err := createStateManager()
			if err != nil {
				return err
			}
			defer sm.Close()

			sessionName := args[0]
			return mergeSession(sm, sessionName, target, squash, dryRun)
		},
	}

	cmd.Flags().StringVar(&target, "target", "", "Target branch to merge into (default: current branch)")
	cmd.Flags().BoolVar(&squash, "squash", false, "Squash merge for clean history")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview merge without executing")

	return cmd
}

// mergeSession merges a session's changes into the target branch
func mergeSession(sm *state.Manager, sessionName, target string, squash, dryRun bool) error {
	sessions, err := sm.DeriveFreshSessions()
	if err != nil {
		return fmt.Errorf("failed to load sessions: %w", err)
	}

	// Find the session
	var targetSession *types.Session
	for _, session := range sessions {
		if session.Core.Name == sessionName {
			targetSession = &session
			break
		}
	}

	if targetSession == nil {
		return fmt.Errorf("session '%s' not found", sessionName)
	}

	// Determine target branch
	if target == "" {
		currentBranch, err := getCurrentBranch()
		if err != nil {
			return fmt.Errorf("failed to get current branch: %w", err)
		}
		target = currentBranch
	}

	sessionBranch := fmt.Sprintf("cwt-%s", sessionName)

	// Validate pre-merge conditions
	if err := validateMergeConditions(target, sessionBranch); err != nil {
		return err
	}

	// Show merge preview
	if err := showMergePreview(sessionBranch, target); err != nil {
		return fmt.Errorf("failed to show merge preview: %w", err)
	}

	if dryRun {
		fmt.Println("\nDry run completed. No changes were made.")
		return nil
	}

	// Confirm merge unless dry run
	if !confirmMerge(sessionName, target, squash) {
		fmt.Println("Merge cancelled")
		return nil
	}

	// Perform the merge
	if err := performMerge(sessionBranch, target, squash); err != nil {
		return fmt.Errorf("merge failed: %w", err)
	}

	fmt.Printf("Successfully merged session '%s' into '%s'\n", sessionName, target)

	// Update session status (this would require extending the Session type)
	// For now, just print success message

	return nil
}

// validateMergeConditions checks if merge can proceed safely
func validateMergeConditions(targetBranch, sessionBranch string) error {
	// Check if target branch exists
	if !branchExists(targetBranch) {
		return fmt.Errorf("target branch '%s' does not exist", targetBranch)
	}

	// Check if session branch exists
	if !branchExists(sessionBranch) {
		return fmt.Errorf("session branch '%s' does not exist", sessionBranch)
	}

	// Check for uncommitted changes in target branch
	currentBranch, err := getCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	if currentBranch == targetBranch && hasUncommittedChanges() {
		return fmt.Errorf("target branch '%s' has uncommitted changes. Please commit or stash them first", targetBranch)
	}

	// Check if session branch is ahead of target
	if !branchIsAhead(sessionBranch, targetBranch) {
		return fmt.Errorf("session branch '%s' is not ahead of target branch '%s'", sessionBranch, targetBranch)
	}

	return nil
}

// showMergePreview displays what will be merged
func showMergePreview(sessionBranch, targetBranch string) error {
	fmt.Printf("Merge Preview: %s -> %s\n", sessionBranch, targetBranch)
	fmt.Println(strings.Repeat("=", 50))

	// Show commit summary
	cmd := exec.Command("git", "log", "--oneline", fmt.Sprintf("%s..%s", targetBranch, sessionBranch))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to show commit summary: %w", err)
	}

	fmt.Println(strings.Repeat("=", 50))

	// Show file changes summary
	cmd = exec.Command("git", "diff", "--stat", targetBranch, sessionBranch)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to show file changes: %w", err)
	}

	return nil
}

// confirmMerge asks user for confirmation
func confirmMerge(sessionName, target string, squash bool) bool {
	mergeType := "merge"
	if squash {
		mergeType = "squash merge"
	}

	fmt.Printf("\nProceed with %s of session '%s' into '%s'? (y/N): ", mergeType, sessionName, target)
	
	var response string
	fmt.Scanln(&response)
	
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// performMerge executes the actual merge
func performMerge(sessionBranch, targetBranch string, squash bool) error {
	// Switch to target branch first
	if err := switchBranch(targetBranch); err != nil {
		return fmt.Errorf("failed to switch to target branch '%s': %w", targetBranch, err)
	}

	// Prepare merge command
	var cmd *exec.Cmd
	if squash {
		cmd = exec.Command("git", "merge", "--squash", sessionBranch)
	} else {
		cmd = exec.Command("git", "merge", "--no-ff", sessionBranch, "-m", fmt.Sprintf("Merge session branch %s", sessionBranch))
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// If merge failed, try to provide helpful error message
		if exitError, ok := err.(*exec.ExitError); ok {
			if exitError.ExitCode() == 1 {
				return fmt.Errorf("merge conflicts detected. Please resolve conflicts and run 'git commit' to complete the merge")
			}
		}
		return fmt.Errorf("merge command failed: %w", err)
	}

	// If squash merge, we need to commit the changes
	if squash {
		commitMsg := fmt.Sprintf("Squash merge session %s", strings.TrimPrefix(sessionBranch, "cwt-"))
		cmd = exec.Command("git", "commit", "-m", commitMsg)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to commit squash merge: %w", err)
		}
	}

	return nil
}

// Helper functions for git operations

func branchExists(branch string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", branch)
	return cmd.Run() == nil
}

func branchIsAhead(sourceBranch, targetBranch string) bool {
	// Check if source branch has commits that target doesn't have
	cmd := exec.Command("git", "rev-list", "--count", fmt.Sprintf("%s..%s", targetBranch, sourceBranch))
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	
	count := strings.TrimSpace(string(output))
	return count != "0"
}