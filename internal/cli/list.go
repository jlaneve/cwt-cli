package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"

	"github.com/jlaneve/cwt-cli/internal/operations"
	"github.com/jlaneve/cwt-cli/internal/types"
)

func newListCmd() *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all sessions with their current status",
		Long: `List all CWT sessions with derived status from:
- Tmux session alive status
- Git working tree changes
- Claude activity and availability

Status is derived fresh from external systems for accuracy.`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListCmd(verbose)
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed information")

	return cmd
}

func runListCmd(verbose bool) error {
	sm, err := createStateManager()
	if err != nil {
		return err
	}
	defer sm.Close()

	// Use operations layer for session retrieval and formatting
	sessionOps := operations.NewSessionOperations(sm)
	sessions, err := sessionOps.GetAllSessions()
	if err != nil {
		return fmt.Errorf("failed to load sessions: %w", err)
	}

	formatter := operations.NewStatusFormat()

	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		fmt.Println("\nCreate a new session with: cwt new [session-name]")
		return nil
	}

	// Sort sessions by creation time (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Core.CreatedAt.After(sessions[j].Core.CreatedAt)
	})

	if verbose {
		renderVerboseSessionList(sessions, formatter)
	} else {
		renderCompactSessionList(sessions, formatter)
	}

	return nil
}

func renderCompactSessionList(sessions []types.Session, formatter *operations.StatusFormat) {
	fmt.Printf("Found %d session(s):\n\n", len(sessions))

	// Calculate max widths for each column based on content
	maxNameLen := 4     // "NAME"
	maxTmuxLen := 4     // "TMUX"
	maxClaudeLen := 6   // "CLAUDE"
	maxGitLen := 3      // "GIT"
	maxActivityLen := 8 // "ACTIVITY"

	// Pre-format all data to calculate actual widths
	type rowData struct {
		name     string
		tmux     string
		claude   string
		git      string
		activity string
	}

	rows := make([]rowData, len(sessions))
	for i, session := range sessions {
		rows[i] = rowData{
			name:     truncate(session.Core.Name, 30),
			tmux:     formatter.FormatTmuxStatus(session.IsAlive),
			claude:   formatter.FormatClaudeStatus(session.ClaudeStatus),
			git:      formatter.FormatGitStatus(session.GitStatus),
			activity: formatter.FormatActivity(session.LastActivity),
		}

		// Update max lengths (using visual length)
		if l := visualLength(rows[i].name); l > maxNameLen {
			maxNameLen = l
		}
		if l := visualLength(rows[i].tmux); l > maxTmuxLen {
			maxTmuxLen = l
		}
		if l := visualLength(rows[i].claude); l > maxClaudeLen {
			maxClaudeLen = l
		}
		if l := visualLength(rows[i].git); l > maxGitLen {
			maxGitLen = l
		}
		if l := visualLength(rows[i].activity); l > maxActivityLen {
			maxActivityLen = l
		}
	}

	// Add padding
	maxNameLen += 2
	maxTmuxLen += 2
	maxClaudeLen += 2
	maxGitLen += 2
	maxActivityLen += 2

	// Print header
	fmt.Printf("%s  %s  %s  %s  %s\n",
		padRight("NAME", maxNameLen),
		padRight("TMUX", maxTmuxLen),
		padRight("CLAUDE", maxClaudeLen),
		padRight("GIT", maxGitLen),
		padRight("ACTIVITY", maxActivityLen))

	fmt.Printf("%s  %s  %s  %s  %s\n",
		strings.Repeat("-", maxNameLen),
		strings.Repeat("-", maxTmuxLen),
		strings.Repeat("-", maxClaudeLen),
		strings.Repeat("-", maxGitLen),
		strings.Repeat("-", maxActivityLen))

	// Print rows
	for _, row := range rows {
		fmt.Printf("%s  %s  %s  %s  %s\n",
			padRight(row.name, maxNameLen),
			padRight(row.tmux, maxTmuxLen),
			padRight(row.claude, maxClaudeLen),
			padRight(row.git, maxGitLen),
			padRight(row.activity, maxActivityLen))
	}
}

func renderVerboseSessionList(sessions []types.Session, formatter *operations.StatusFormat) {
	fmt.Printf("Found %d session(s):\n\n", len(sessions))

	for i, session := range sessions {
		if i > 0 {
			fmt.Println()
		}

		fmt.Printf("🏷️  %s\n", session.Core.Name)
		fmt.Printf("   ID: %s\n", session.Core.ID)
		fmt.Printf("   Created: %s\n", session.Core.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("   Worktree: %s\n", session.Core.WorktreePath)
		fmt.Printf("   \n")

		// Tmux status
		fmt.Printf("   🖥️  Tmux: %s (session: %s)\n",
			formatter.FormatTmuxStatus(session.IsAlive), session.Core.TmuxSession)

		// Git status
		gitDetails := ""
		if session.GitStatus.HasChanges {
			changes := []string{}
			if len(session.GitStatus.ModifiedFiles) > 0 {
				changes = append(changes, fmt.Sprintf("%d modified", len(session.GitStatus.ModifiedFiles)))
			}
			if len(session.GitStatus.AddedFiles) > 0 {
				changes = append(changes, fmt.Sprintf("%d added", len(session.GitStatus.AddedFiles)))
			}
			if len(session.GitStatus.DeletedFiles) > 0 {
				changes = append(changes, fmt.Sprintf("%d deleted", len(session.GitStatus.DeletedFiles)))
			}
			gitDetails = fmt.Sprintf(" (%s)", strings.Join(changes, ", "))
		}
		fmt.Printf("   📁 Git: %s%s\n", formatter.FormatGitStatus(session.GitStatus), gitDetails)

		// Claude status
		claudeDetails := ""
		if session.ClaudeStatus.SessionID != "" {
			claudeDetails = fmt.Sprintf(" (session: %s)", session.ClaudeStatus.SessionID)
		}
		if !session.ClaudeStatus.LastMessage.IsZero() {
			age := time.Since(session.ClaudeStatus.LastMessage)
			claudeDetails += fmt.Sprintf(" (last: %s ago)", formatter.FormatDuration(age))
		}
		fmt.Printf("   🤖 Claude: %s%s\n", formatter.FormatClaudeStatus(session.ClaudeStatus), claudeDetails)

		// Show full message in verbose mode if available
		if session.ClaudeStatus.StatusMessage != "" {
			fmt.Printf("      Message: %s\n", session.ClaudeStatus.StatusMessage)
		}

		// Last activity
		fmt.Printf("   ⏰ Activity: %s\n", formatter.FormatActivity(session.LastActivity))
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// visualLength calculates the visual display width of a string using runewidth
func visualLength(s string) int {
	return runewidth.StringWidth(s)
}

// padRight pads a string to the specified visual width
func padRight(s string, width int) string {
	currentWidth := runewidth.StringWidth(s)
	if currentWidth >= width {
		return s
	}
	return s + strings.Repeat(" ", width-currentWidth)
}
