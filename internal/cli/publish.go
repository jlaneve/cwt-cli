package cli

import (
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

// newPublishCmd creates the 'cwt publish' command
func newPublishCmd() *cobra.Command {
	var draft bool
	var pr bool
	var localOnly bool
	var message string

	cmd := &cobra.Command{
		Use:   "publish <session-name>",
		Short: "Commit all session changes and publish the branch",
		Long: `Commit all session changes and publish the branch for collaboration or backup.

Examples:
  cwt publish my-session                # Commit all changes + push branch
  cwt publish my-session --draft        # Push as draft PR (if GitHub CLI available)
  cwt publish my-session --pr           # Create PR automatically
  cwt publish my-session --local        # Commit only, no push
  cwt publish my-session -m "Custom commit message"  # Use custom commit message`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sm, err := createStateManager()
			if err != nil {
				return err
			}
			defer sm.Close()

			sessionName := args[0]
			return publishSession(sm, sessionName, message, draft, pr, localOnly)
		},
	}

	cmd.Flags().BoolVar(&draft, "draft", false, "Push as draft PR (requires GitHub CLI)")
	cmd.Flags().BoolVar(&pr, "pr", false, "Create PR automatically (requires GitHub CLI)")
	cmd.Flags().BoolVar(&localOnly, "local", false, "Commit only, no push")
	cmd.Flags().StringVarP(&message, "message", "m", "", "Custom commit message")

	return cmd
}

// publishSession commits and publishes a session's changes
func publishSession(sm *state.Manager, sessionName, customMessage string, draft, pr, localOnly bool) error {
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

	if !targetSession.IsAlive {
		fmt.Printf("Warning: Session '%s' is not currently active\n", sessionName)
	}

	worktreePath := targetSession.Core.WorktreePath
	sessionBranch := fmt.Sprintf("cwt-%s", sessionName)

	// Switch to the session's worktree directory
	originalDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(worktreePath); err != nil {
		return fmt.Errorf("failed to change to worktree directory: %w", err)
	}

	// Check if there are changes to commit
	if !hasChangesToCommit() {
		fmt.Printf("No changes to commit in session '%s'\n", sessionName)
		if !localOnly {
			// Still try to push in case there are unpushed commits
			return pushBranch(sessionBranch, draft, pr)
		}
		return nil
	}

	// Generate commit message
	commitMessage := customMessage
	if commitMessage == "" {
		commitMessage = generateCommitMessage(sessionName, worktreePath)
	}

	// Stage and commit changes
	if err := stageAndCommit(commitMessage); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	fmt.Printf("Committed changes in session '%s'\n", sessionName)

	// Push if not local-only
	if !localOnly {
		if err := pushBranch(sessionBranch, draft, pr); err != nil {
			return fmt.Errorf("failed to push branch: %w", err)
		}
	}

	return nil
}

// hasChangesToCommit checks if there are changes to commit
func hasChangesToCommit() bool {
	// Check for staged changes
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	if cmd.Run() != nil {
		return true // Has staged changes
	}

	// Check for unstaged changes
	cmd = exec.Command("git", "diff", "--quiet")
	if cmd.Run() != nil {
		return true // Has unstaged changes
	}

	// Check for untracked files
	cmd = exec.Command("git", "ls-files", "--others", "--exclude-standard")
	output, err := cmd.Output()
	if err == nil && len(strings.TrimSpace(string(output))) > 0 {
		return true // Has untracked files
	}

	return false
}

// generateCommitMessage creates an intelligent commit message
func generateCommitMessage(sessionName, worktreePath string) string {
	// Try to read Claude's recent activity to understand what was done
	claudeMessage := extractClaudeWorkSummary(worktreePath)
	if claudeMessage != "" {
		return fmt.Sprintf("feat(%s): %s\n\n🤖 Generated with Claude Code\n\nCo-Authored-By: Claude <noreply@anthropic.com>", sessionName, claudeMessage)
	}

	// Fallback to generic message with file analysis
	changes := analyzeChanges()
	if changes != "" {
		return fmt.Sprintf("feat(%s): %s\n\n🤖 Generated with Claude Code\n\nCo-Authored-By: Claude <noreply@anthropic.com>", sessionName, changes)
	}

	// Final fallback
	return fmt.Sprintf("feat(%s): Update session changes\n\n🤖 Generated with Claude Code\n\nCo-Authored-By: Claude <noreply@anthropic.com>", sessionName)
}

// extractClaudeWorkSummary tries to extract what Claude was working on
func extractClaudeWorkSummary(worktreePath string) string {
	// Look for Claude's session state or recent JSONL activity
	sessionStateDir := filepath.Join(worktreePath, ".claude", "session_state")

	// This is a simplified implementation - in a full version,
	// you'd parse Claude's actual activity logs
	if _, err := os.Stat(sessionStateDir); err == nil {
		return "implement new features and improvements"
	}

	return ""
}

// analyzeChanges analyzes git diff to create a descriptive commit message
func analyzeChanges() string {
	// Get list of modified files
	cmd := exec.Command("git", "diff", "--name-only", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "update files"
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(files) == 0 || files[0] == "" {
		// Check staged files
		cmd = exec.Command("git", "diff", "--cached", "--name-only")
		output, err = cmd.Output()
		if err != nil {
			return "update files"
		}
		files = strings.Split(strings.TrimSpace(string(output)), "\n")
	}

	if len(files) == 0 || files[0] == "" {
		return "update files"
	}

	// Analyze file types and create descriptive message
	var goFiles, jsFiles, pyFiles, otherFiles int

	for _, file := range files {
		if file == "" {
			continue
		}
		ext := filepath.Ext(file)
		switch ext {
		case ".go":
			goFiles++
		case ".js", ".ts", ".jsx", ".tsx":
			jsFiles++
		case ".py":
			pyFiles++
		default:
			otherFiles++
		}
	}

	if len(files) == 1 {
		return fmt.Sprintf("update %s", files[0])
	}

	var parts []string
	if goFiles > 0 {
		parts = append(parts, fmt.Sprintf("%d Go files", goFiles))
	}
	if jsFiles > 0 {
		parts = append(parts, fmt.Sprintf("%d JS/TS files", jsFiles))
	}
	if pyFiles > 0 {
		parts = append(parts, fmt.Sprintf("%d Python files", pyFiles))
	}
	if otherFiles > 0 {
		parts = append(parts, fmt.Sprintf("%d other files", otherFiles))
	}

	if len(parts) > 0 {
		return fmt.Sprintf("update %s", strings.Join(parts, ", "))
	}

	return fmt.Sprintf("update %d files", len(files))
}

// stageAndCommit stages all changes and commits them
func stageAndCommit(message string) error {
	// Add all changes (including untracked files)
	cmd := exec.Command("git", "add", ".")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stage changes: %w", err)
	}

	// Commit changes
	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// pushBranch pushes the branch and optionally creates PR
func pushBranch(branch string, draft, pr bool) error {
	// Check if remote exists
	if !hasRemote() {
		fmt.Println("No remote repository configured, skipping push")
		return nil
	}

	// Push branch with upstream tracking
	fmt.Printf("Pushing branch '%s'...\n", branch)
	cmd := exec.Command("git", "push", "-u", "origin", branch)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to push branch: %w", err)
	}

	fmt.Printf("Successfully pushed branch '%s'\n", branch)

	// Create PR if requested and GitHub CLI is available
	if (draft || pr) && hasGitHubCLI() {
		return createPullRequest(branch, draft)
	} else if draft || pr {
		fmt.Println("GitHub CLI not found, skipping PR creation")
		fmt.Printf("You can manually create a PR for branch '%s'\n", branch)
	}

	return nil
}

// hasRemote checks if a remote repository is configured
func hasRemote() bool {
	cmd := exec.Command("git", "remote")
	output, err := cmd.Output()
	return err == nil && len(strings.TrimSpace(string(output))) > 0
}

// hasGitHubCLI checks if GitHub CLI is available
func hasGitHubCLI() bool {
	cmd := exec.Command("gh", "--version")
	return cmd.Run() == nil
}

// createPullRequest creates a pull request using GitHub CLI
func createPullRequest(branch string, draft bool) error {
	sessionName := strings.TrimPrefix(branch, "cwt-")
	title := fmt.Sprintf("feat(%s): Session changes", sessionName)

	body := fmt.Sprintf(`## Summary
Changes from CWT session: %s

## Generated Context
- Session branch: %s
- Created: %s

🤖 Generated with [Claude Code](https://claude.ai/code)`,
		sessionName,
		branch,
		time.Now().Format("2006-01-02 15:04:05"))

	args := []string{"pr", "create", "--title", title, "--body", body}
	if draft {
		args = append(args, "--draft")
	}

	fmt.Printf("Creating pull request for branch '%s'...\n", branch)
	cmd := exec.Command("gh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create pull request: %w", err)
	}

	return nil
}
