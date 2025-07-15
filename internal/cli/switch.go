package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jlaneve/cwt-cli/internal/state"
	"github.com/jlaneve/cwt-cli/internal/types"
)

// newSwitchCmd creates the 'cwt switch' command
func newSwitchCmd() *cobra.Command {
	var back bool

	cmd := &cobra.Command{
		Use:   "switch [session-name]",
		Short: "Switch to a session's branch for testing or manual work",
		Long: `Switch your main workspace to a session's branch temporarily.
This allows you to test changes or do manual work on the session branch.

Examples:
  cwt switch my-session     # Switch to my-session branch
  cwt switch --back         # Return to previous branch
  cwt switch                # Interactive session selector`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sm, err := createStateManager()
			if err != nil {
				return err
			}
			defer sm.Close()

			if back {
				return switchBack()
			}

			if len(args) == 0 {
				return interactiveSwitch(sm)
			}

			sessionName := args[0]
			return switchToSession(sm, sessionName)
		},
	}

	cmd.Flags().BoolVar(&back, "back", false, "Return to previous branch")

	return cmd
}

// switchToSession switches to a session's branch
func switchToSession(sm *state.Manager, sessionName string) error {
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

	// Get current branch
	currentBranch, err := getCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// Check for uncommitted changes and handle them interactively
	if hasUncommittedChanges() {
		if err := handleUncommittedChanges(); err != nil {
			return err
		}
	}

	// Save current branch for --back functionality
	if err := savePreviousBranch(currentBranch); err != nil {
		fmt.Printf("Warning: failed to save previous branch: %v\n", err)
	}

	// Switch to session branch
	sessionBranch := fmt.Sprintf("cwt-%s", sessionName)
	if err := switchBranch(sessionBranch); err != nil {
		return fmt.Errorf("failed to switch to branch '%s': %w", sessionBranch, err)
	}

	fmt.Printf("Switched to session branch: %s\n", sessionBranch)
	fmt.Printf("Use 'cwt switch --back' to return to %s\n", currentBranch)

	return nil
}

// switchBack returns to the previous branch
func switchBack() error {
	previousBranch, err := loadPreviousBranch()
	if err != nil {
		return fmt.Errorf("no previous branch saved: %w", err)
	}

	// Check for uncommitted changes
	if hasUncommittedChanges() {
		return fmt.Errorf("cannot switch: you have uncommitted changes. Please commit or stash them first")
	}

	if err := switchBranch(previousBranch); err != nil {
		return fmt.Errorf("failed to switch back to '%s': %w", previousBranch, err)
	}

	fmt.Printf("Switched back to: %s\n", previousBranch)

	// Clear the saved previous branch
	if err := clearPreviousBranch(); err != nil {
		fmt.Printf("Warning: failed to clear previous branch: %v\n", err)
	}

	return nil
}

// interactiveSwitch provides an interactive session selector
func interactiveSwitch(sm *state.Manager) error {
	sessions, err := sm.DeriveFreshSessions()
	if err != nil {
		return fmt.Errorf("failed to load sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions available to switch to")
		return nil
	}

	selectedSession, err := SelectSession(sessions, WithTitle("Select a session to switch to:"))
	if err != nil {
		return fmt.Errorf("failed to select session: %w", err)
	}

	if selectedSession == nil {
		fmt.Println("Cancelled")
		return nil
	}

	return switchToSession(sm, selectedSession.Core.Name)
}

// Helper functions for git operations

func getCurrentBranch() (string, error) {
	cmd := exec.Command("git", "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func hasUncommittedChanges() bool {
	cmd := exec.Command("git", "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return false // Assume no changes if we can't check
	}
	return len(strings.TrimSpace(string(output))) > 0
}

func switchBranch(branch string) error {
	cmd := exec.Command("git", "checkout", branch)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Helper functions for previous branch management

func savePreviousBranch(branch string) error {
	sm, err := createStateManager()
	if err != nil {
		return err
	}
	defer sm.Close()

	dataDir := sm.GetDataDir()
	previousBranchFile := filepath.Join(dataDir, "previous_branch")

	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}

	return os.WriteFile(previousBranchFile, []byte(branch), 0644)
}

func loadPreviousBranch() (string, error) {
	sm, err := createStateManager()
	if err != nil {
		return "", err
	}
	defer sm.Close()

	dataDir := sm.GetDataDir()
	previousBranchFile := filepath.Join(dataDir, "previous_branch")

	data, err := os.ReadFile(previousBranchFile)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}

func clearPreviousBranch() error {
	sm, err := createStateManager()
	if err != nil {
		return err
	}
	defer sm.Close()

	dataDir := sm.GetDataDir()
	previousBranchFile := filepath.Join(dataDir, "previous_branch")

	return os.Remove(previousBranchFile)
}

// handleUncommittedChanges provides options for dealing with uncommitted changes
func handleUncommittedChanges() error {
	fmt.Println("⚠️  You have uncommitted changes that need to be handled before switching branches.")
	fmt.Println()

	// Show what changes exist
	if err := showUncommittedChanges(); err != nil {
		fmt.Printf("Warning: failed to show changes: %v\n", err)
	}

	fmt.Println()
	fmt.Println("How would you like to handle these changes?")
	fmt.Println("  1. 📦 Stash changes (recommended - easily recoverable)")
	fmt.Println("  2. 💾 Commit changes (permanent)")
	fmt.Println("  3. ❌ Cancel switch")
	fmt.Println()

	for {
		fmt.Print("Enter your choice (1-3) [1]: ")

		var input string
		fmt.Scanln(&input)

		// Default to stash if no input
		if input == "" {
			input = "1"
		}

		switch input {
		case "1":
			return stashChanges()
		case "2":
			return commitChanges()
		case "3":
			return fmt.Errorf("switch cancelled by user")
		default:
			fmt.Println("Invalid choice. Please enter 1, 2, or 3.")
			continue
		}
	}
}

// showUncommittedChanges displays what changes exist
func showUncommittedChanges() error {
	cmd := exec.Command("git", "status", "--short")
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	if len(output) > 0 {
		fmt.Println("Changes that will be affected:")
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			if line != "" {
				fmt.Printf("  %s\n", line)
			}
		}
	}

	return nil
}

// stashChanges stashes the current changes
func stashChanges() error {
	fmt.Println("📦 Stashing changes...")

	// Create a meaningful stash message
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf("CWT auto-stash before branch switch - %s", timestamp)

	cmd := exec.Command("git", "stash", "push", "-m", message)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stash changes: %w", err)
	}

	fmt.Println("✅ Changes stashed successfully!")
	fmt.Println("💡 Use 'git stash pop' to restore them later")
	return nil
}

// commitChanges prompts for a commit message and commits changes
func commitChanges() error {
	fmt.Print("Enter commit message: ")

	reader := bufio.NewReader(os.Stdin)
	message, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read commit message: %w", err)
	}

	message = strings.TrimSpace(message)
	if message == "" {
		return fmt.Errorf("commit message cannot be empty")
	}

	fmt.Println("💾 Committing changes...")

	// Add all changes
	if err := exec.Command("git", "add", ".").Run(); err != nil {
		return fmt.Errorf("failed to stage changes: %w", err)
	}

	// Commit changes
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	fmt.Println("✅ Changes committed successfully!")
	return nil
}
