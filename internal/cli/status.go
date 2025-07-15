package cli

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jlaneve/cwt-cli/internal/state"
	"github.com/jlaneve/cwt-cli/internal/types"
)

// newStatusCmd creates the 'cwt status' command
func newStatusCmd() *cobra.Command {
	var summary bool
	var branch bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show comprehensive status of all sessions with change details",
		Long: `Show comprehensive view of changes across all sessions with rich status information.

This command provides detailed information about:
- Session states and activity
- Git changes and commit counts  
- Branch relationships and merge status
- Overall project health

Examples:
  cwt status               # Detailed status for all sessions
  cwt status --summary     # Summary view with statistics
  cwt status --branch      # Include branch relationship info`,
		RunE: func(cmd *cobra.Command, args []string) error {
			sm, err := createStateManager()
			if err != nil {
				return err
			}
			defer sm.Close()

			return showEnhancedStatus(sm, summary, branch)
		},
	}

	cmd.Flags().BoolVar(&summary, "summary", false, "Show summary of all changes across sessions")
	cmd.Flags().BoolVar(&branch, "branch", false, "Include branch relationship information")

	return cmd
}

// showEnhancedStatus displays comprehensive session status
func showEnhancedStatus(sm *state.Manager, summary, showBranch bool) error {
	sessions, err := sm.DeriveFreshSessions()
	if err != nil {
		return fmt.Errorf("failed to load sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		fmt.Println("\nCreate a new session with: cwt new [session-name] [task-description]")
		return nil
	}

	// Sort sessions by last activity (most recent first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastActivity.After(sessions[j].LastActivity)
	})

	if summary {
		return showStatusSummary(sessions)
	}

	return showDetailedStatus(sessions, showBranch)
}

// showStatusSummary shows a high-level summary of all sessions
func showStatusSummary(sessions []types.Session) error {
	fmt.Println("📊 Session Summary")
	fmt.Println(strings.Repeat("=", 50))

	// Calculate statistics
	var alive, dead, hasChanges, published, merged int
	var totalModified, totalAdded, totalDeleted int

	for _, session := range sessions {
		if session.IsAlive {
			alive++
		} else {
			dead++
		}

		if session.GitStatus.HasChanges {
			hasChanges++
			totalModified += len(session.GitStatus.ModifiedFiles)
			totalAdded += len(session.GitStatus.AddedFiles)
			totalDeleted += len(session.GitStatus.DeletedFiles)
		}

		// Check if published (has remote tracking)
		if isSessionPublished(session) {
			published++
		}

		// Check if merged (would need additional logic)
		if isSessionMerged(session) {
			merged++
		}
	}

	// Display statistics
	fmt.Printf("Total Sessions:    %d\n", len(sessions))
	fmt.Printf("  • Active:        %d\n", alive)
	fmt.Printf("  • Inactive:      %d\n", dead)
	fmt.Printf("\n")
	fmt.Printf("Change Summary:\n")
	fmt.Printf("  • With Changes:  %d\n", hasChanges)
	fmt.Printf("  • Clean:         %d\n", len(sessions)-hasChanges)
	fmt.Printf("  • Published:     %d\n", published)
	fmt.Printf("  • Merged:        %d\n", merged)
	fmt.Printf("\n")
	fmt.Printf("File Changes:\n")
	fmt.Printf("  • Modified:      %d\n", totalModified)
	fmt.Printf("  • Added:         %d\n", totalAdded)
	fmt.Printf("  • Deleted:       %d\n", totalDeleted)

	// Show most recent activity
	if len(sessions) > 0 {
		fmt.Printf("\n")
		fmt.Printf("Recent Activity:\n")
		for i, session := range sessions {
			if i >= 3 { // Show top 3 most recent
				break
			}
			fmt.Printf("  • %s: %s\n", session.Core.Name, FormatActivity(session.LastActivity))
		}
	}

	return nil
}

// showDetailedStatus shows detailed information for each session
func showDetailedStatus(sessions []types.Session, showBranch bool) error {
	fmt.Printf("📋 Session Status (%d sessions)\n", len(sessions))
	fmt.Println(strings.Repeat("=", 70))

	for i, session := range sessions {
		if i > 0 {
			fmt.Println()
		}

		renderSessionStatus(session, showBranch)
	}

	return nil
}

// renderSessionStatus renders detailed status for a single session
func renderSessionStatus(session types.Session, showBranch bool) {
	// Session header
	fmt.Printf("🏷️  %s", session.Core.Name)

	// Show main status indicators
	statusIndicators := []string{}

	if session.IsAlive {
		statusIndicators = append(statusIndicators, "🟢 active")
	} else {
		statusIndicators = append(statusIndicators, "🔴 inactive")
	}

	if session.GitStatus.HasChanges {
		changeCount := len(session.GitStatus.ModifiedFiles) + len(session.GitStatus.AddedFiles) + len(session.GitStatus.DeletedFiles)
		statusIndicators = append(statusIndicators, fmt.Sprintf("📝 %d changes", changeCount))
	} else {
		statusIndicators = append(statusIndicators, "✨ clean")
	}

	if isSessionPublished(session) {
		statusIndicators = append(statusIndicators, "📤 published")
	}

	fmt.Printf(" (%s)\n", strings.Join(statusIndicators, ", "))

	// Show activity timing
	fmt.Printf("   ⏰ Last activity: %s\n", FormatActivity(session.LastActivity))

	// Show Claude status
	claudeIcon := getClaudeIcon(session.ClaudeStatus.State)
	fmt.Printf("   %s Claude: %s", claudeIcon, string(session.ClaudeStatus.State))

	if session.ClaudeStatus.StatusMessage != "" {
		fmt.Printf(" - %s", session.ClaudeStatus.StatusMessage)
	}

	if !session.ClaudeStatus.LastMessage.IsZero() {
		age := time.Since(session.ClaudeStatus.LastMessage)
		fmt.Printf(" (last: %s ago)", FormatDuration(age))
	}
	fmt.Println()

	// Show detailed git status
	if session.GitStatus.HasChanges {
		fmt.Printf("   📁 Git changes:\n")

		if len(session.GitStatus.ModifiedFiles) > 0 {
			fmt.Printf("      📝 Modified: %s\n",
				formatFileList(session.GitStatus.ModifiedFiles, 3))
		}

		if len(session.GitStatus.AddedFiles) > 0 {
			fmt.Printf("      ➕ Added: %s\n",
				formatFileList(session.GitStatus.AddedFiles, 3))
		}

		if len(session.GitStatus.DeletedFiles) > 0 {
			fmt.Printf("      ➖ Deleted: %s\n",
				formatFileList(session.GitStatus.DeletedFiles, 3))
		}

		if len(session.GitStatus.UntrackedFiles) > 0 {
			fmt.Printf("      ❓ Untracked: %s\n",
				formatFileList(session.GitStatus.UntrackedFiles, 3))
		}
	}

	// Show commit count if available
	if session.GitStatus.CommitCount > 0 {
		fmt.Printf("   📊 Commits ahead: %d\n", session.GitStatus.CommitCount)
	}

	// Show branch information if requested
	if showBranch {
		branchName := fmt.Sprintf("cwt-%s", session.Core.Name)
		if branchInfo := getBranchInfo(session.Core.WorktreePath, branchName); branchInfo != "" {
			fmt.Printf("   🌿 Branch: %s\n", branchInfo)
		}
	}

	// Show path for easy access
	fmt.Printf("   📂 Path: %s\n", session.Core.WorktreePath)
}

// Helper functions

func getClaudeIcon(state types.ClaudeState) string {
	switch state {
	case types.ClaudeWorking:
		return "🔄"
	case types.ClaudeWaiting:
		return "⏸️"
	case types.ClaudeComplete:
		return "✅"
	case types.ClaudeIdle:
		return "💤"
	default:
		return "❓"
	}
}

func formatFileList(files []string, maxShow int) string {
	if len(files) == 0 {
		return ""
	}

	if len(files) <= maxShow {
		return strings.Join(files, ", ")
	}

	shown := files[:maxShow]
	remaining := len(files) - maxShow
	return fmt.Sprintf("%s... (+%d more)", strings.Join(shown, ", "), remaining)
}

func isSessionPublished(session types.Session) bool {
	// This is a simplified check - in a full implementation,
	// you'd check if the branch has been pushed to remote
	branchName := fmt.Sprintf("cwt-%s", session.Core.Name)

	// Change to worktree directory to check remote tracking
	originalDir, err := os.Getwd()
	if err != nil {
		return false
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(session.Core.WorktreePath); err != nil {
		return false
	}

	// Check if branch has remote tracking
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", fmt.Sprintf("%s@{upstream}", branchName))
	return cmd.Run() == nil
}

func isSessionMerged(session types.Session) bool {
	// This is a simplified check - in a full implementation,
	// you'd check if the session branch has been merged into main
	return false // Placeholder for now
}

func getBranchInfo(worktreePath, branchName string) string {
	// Change to worktree directory
	originalDir, err := os.Getwd()
	if err != nil {
		return ""
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(worktreePath); err != nil {
		return ""
	}

	// Get branch status relative to main
	cmd := exec.Command("git", "rev-list", "--count", "--left-right", "main..."+branchName)
	output, err := cmd.Output()
	if err != nil {
		return branchName
	}

	parts := strings.Fields(strings.TrimSpace(string(output)))
	if len(parts) == 2 {
		behind := parts[0]
		ahead := parts[1]
		return fmt.Sprintf("%s (↓%s ↑%s)", branchName, behind, ahead)
	}

	return branchName
}

// Helper functions are imported from list.go - removed duplicates
